package dnsanitize

import (
	"sync"
	"time"

	"github.com/nil0x42/dnsanity/internal/config"
	"github.com/nil0x42/dnsanity/internal/report"
	"github.com/nil0x42/dnsanity/internal/dns"
)


// ---------------------------------------------------------------------------
// Worker & scheduler plumbing -----------------------------------------------
// ---------------------------------------------------------------------------

// WorkerResult is used by goroutines to send back the final DNSAnswer.
type WorkerResult struct {
    SrvID		int					// srv id in pool
    CheckID		int					// check index
    Answer		*dns.DNSAnswer		// received answer
    Passed		bool				// equals? result
}

type QueryScheduler struct {
	waitGroup	sync.WaitGroup
	JobLimiter	chan struct{}
	RateLimiter	*RateLimiter
	Results		chan WorkerResult // worker results are sent here
}

func runDNSWorker(
	srv			*dns.ServerContext,	// server context
	check		*dns.TemplateEntry,	// template check
	srvID		int,				// server ID (in pool)
	checkID		int,				// check ID (template index)
	timeout		time.Duration,		// DNS query timeout
	sched		*QueryScheduler,	// scheduler
) {
	defer sched.waitGroup.Done()
	answer := dns.ResolveDNS(check.Domain, srv.IPAddress, timeout, srv.Ctx)
	sched.Results <- WorkerResult{
		SrvID:			srvID,
		CheckID:		checkID,
		Answer:			answer,
		Passed:			check.Matches(answer),
	}
	<- sched.JobLimiter
}

// ---------------------------------------------------------------------------
//  PUBLIC ENTRYPOINT --------------------------------------------------------
// ---------------------------------------------------------------------------
func DNSanitize(
	s		*config.Settings,
	status	*report.StatusReporter,
) {
	maxAttempts := max(s.PerCheckMaxAttempts, 1)
	maxThreads := min(s.MaxThreads, len(s.ServerIPs) * len(s.Template))
	srvReqInterval := time.Duration(0)
	if s.PerSrvRateLimit > 0 {
		srvReqInterval = time.Second / time.Duration(s.PerSrvRateLimit)
	}
	globRateLimit := s.GlobRateLimit
	if (globRateLimit <= 0) {
		globRateLimit = int(^uint(0) >> 1) // INT_MAX
	}
	srvMaxFailures := s.PerSrvMaxFailures
	if srvMaxFailures < 0 {
		srvMaxFailures = int(^uint(0) >> 1) // INT_MAX
	}
	qryTimeout := time.Duration(s.PerQueryTimeout) * time.Second

    // init server pool
	idealPoolSz := min(maxThreads * 2, globRateLimit, 10000)
	pool := NewServerPool(s.ServerIPs, s.Template, idealPoolSz, maxAttempts)
	// init scheduler
	sched := &QueryScheduler{
		JobLimiter:  make(chan struct{}, maxThreads),
		Results:     make(chan WorkerResult, maxThreads),
		RateLimiter: NewRateLimiter(globRateLimit, time.Second),
	}
	// Run the scheduling loop to fill out servers
	scheduleChecks(
		pool, s.Template, sched, status,
		qryTimeout, srvReqInterval, srvMaxFailures,
	)
	// stop gobal ratelimiter
	sched.RateLimiter.StopRefiller()
}

// scheduleChecks is the core scheduler that dispatches DNS queries,
// observes concurrency limits, rate limits, and failure thresholds.
func scheduleChecks(
	pool			*ServerPool,
	template		dns.Template,
	sched			*QueryScheduler,
	status			*report.StatusReporter,
	qryTimeout		time.Duration,
	srvReqInterval	time.Duration,
	srvMaxFailures	int,
) {
	for {
		// 1) async collection of worker results -----------------------------
		collectLoop:
		for {
			select {
			case res := <- sched.Results:
				srv, ok := pool.pool[res.SrvID]
				if !ok {
				    continue // already unloaded -> drop silently
				}
				applyResults(srv, &res, srvMaxFailures, status)
				if srv.Finished() {
					status.ReportFinishedServer(srv) // report server
					pool.Unload(res.SrvID) // drop server from pool
					if pool.IsOverLoaded() || pool.LoadN(1) == 0 {
						status.UpdatePoolSize(pool.Len())
					}
					// status.UpdatePoolSize(pool.Len())
				}
			default:
				break collectLoop
			}
		}
		// 2) scheduling new queries -----------------------------------------
		now := time.Now()
		numScheduled := 0
		for srvID, srv := range pool.pool {
			if                             // SKIP SERVER IF:
			len(srv.PendingChecks) == 0 || //  - nothing to do at the moment
			srv.NextQueryAt.After(now) ||  //  - per-serv ratelimit not honored
			!sched.RateLimiter.ConsumeOne() {//  - global ratelimit not honored
				continue
			}
			select {
			case sched.JobLimiter <- struct{}{}:
				// We can schedule a new test for this server
				checkID := srv.PendingChecks[0]
				srv.PendingChecks = srv.PendingChecks[1:]
				sched.waitGroup.Add(1)
				go runDNSWorker(
					srv, &template[checkID],
					srvID, checkID, qryTimeout, sched,
				)
				srv.NextQueryAt = now.Add(srvReqInterval)
				numScheduled++
			default:
				// No free slot: give back consumed ratelimit token:
				sched.RateLimiter.GiveBackOne()
			}
		}
		// notify num of requests just scheduled (for rps count)
		if numScheduled > 0 {
			status.LogRequests(now, numScheduled)
		}
		// 3) termination condition ------------------------------------------
		if pool.IsDrained() {
			break // every server processed and pool emptied
		}
		// 4) refill pool if we have RPS budget and nothing scheduled --------
		if numScheduled == 0 {
			availableReqs := sched.RateLimiter.Remaining()
			if availableReqs > 0 && pool.NumPending() > 0 && !pool.IsFull() {
				inserted := pool.LoadN(availableReqs)
				status.UpdatePoolSize(pool.Len())
				status.Debug(
					"expand pool by %d. newsz=%d",
					inserted, pool.Len())
			} else {
				time.Sleep(13 * time.Millisecond) // avoid busy-wait
			}
		}
	}
	// END) Once all works are done, close chan & ignore remaining msgs:
	sched.waitGroup.Wait()
}

// applyResults updates a ServerContext after one DNS query
// and reflects the change into the shared Status struct.
// This runs in the scheduler goroutine (single-threaded),
func applyResults(
	srv				*dns.ServerContext, // server
	res				*WorkerResult, // worker result
	srvMaxFailures	int, // max allowed non-passing checks per server
	status			*report.StatusReporter,
) {
	chk := &srv.Checks[res.CheckID]
	chk.AttemptsLeft--
	chk.Answer = res.Answer
	/* ---------- success ------------------------------------------------ */
	if res.Passed {
		chk.Passed = true
		srv.CompletedCount++
		status.AddDoneChecks(+1, +0) // +1 done, +0 total
		return
	}
	/* ---------- failure, retry remaining ------------------------------- */
	if chk.AttemptsLeft > 0 {
		// re-queue the check at the front
		srv.PendingChecks = append([]int{res.CheckID}, srv.PendingChecks...)
		status.AddDoneChecks(+1, +1) // +1 done, +1 total
		return
	}
	/* ---------- failure, no retry left --------------------------------- */
	srv.CompletedCount++
	srv.FailedCount++
	// reached drop threshold?
	if srv.FailedCount >= srvMaxFailures {
		// how many planned checks are immediately cancelled
		cancelledChecks := len(srv.Checks) - srv.CompletedCount
		status.AddDoneChecks(+1, -cancelledChecks)
		srv.Disabled = true
		srv.CancelCtx()
	} else {
		status.AddDoneChecks(+1, +0) // +1 done, +0 total
	}
}
