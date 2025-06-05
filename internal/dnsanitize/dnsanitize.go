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
	qryTimeout := time.Duration(s.PerQueryTimeout) * time.Second
	srvReqInterval := time.Duration(0)
	if s.PerSrvRateLimit > 0 {
		srvReqInterval = time.Duration(float64(time.Second) / s.PerSrvRateLimit)
	}

    // init server pool
	pool := NewServerPool(
		s.MaxPoolSize, s.ServerIPs, s.Template, s.PerCheckMaxAttempts)

	// init scheduler
	maxThreads := min(s.MaxThreads, len(s.ServerIPs) * len(s.Template))
	sched := &QueryScheduler{
		JobLimiter:  make(chan struct{}, maxThreads),
		Results:     make(chan WorkerResult, maxThreads),
		RateLimiter: NewRateLimiter(s.GlobRateLimit, time.Second),
	}
	// Run the scheduling loop to fill out servers
	scheduleChecks(
		pool, s.Template, sched, status,
		qryTimeout, srvReqInterval, s.PerSrvMaxFailures,
	)
	// stop gobal ratelimiter
	sched.RateLimiter.StopRefiller()
}

// scheduleChecks is the core scheduler that dispatches DNS queries,
// observes concurrency limits, rate limits, and failure thresholds.
func scheduleChecks(
	pool            *ServerPool,
	template        dns.Template,
	sched           *QueryScheduler,
	status          *report.StatusReporter,
	qryTimeout      time.Duration,
	srvReqInterval  time.Duration,
	srvMaxFailures  int,
) {
	inFlight := make(map[int]int)
	for {
		// 1) async collection of worker results -----------------------------
		collectLoop:
		for {
			select {
			case res := <-sched.Results:
				// request done -> decrement inFlight count
				if n := inFlight[res.SrvID]; n > 1 {
					inFlight[res.SrvID] = n - 1
				} else { // <=0
					delete(inFlight, res.SrvID)
				}
				srv, srvExists := pool.Get(res.SrvID)
				if !srvExists { // server already dropped
					continue
				}
				applyResults(srv, &res, srvMaxFailures, status)
				if srv.Finished() {
					status.ReportFinishedServer(srv) // report server
					pool.Unload(res.SrvID)           // drop server from pool
				}
			default:
				break collectLoop
			}
		}

		// 2) scheduling new queries -----------------------------------------
		now := time.Now()
		numScheduled, numScheduledBusy, numScheduledIdle := 0, 0, 0
		poolCanGrow := pool.CanGrow()
		busyJobs := len(sched.JobLimiter)
		freeJobs := cap(sched.JobLimiter) - busyJobs
		// Two passes : first IDLE servers, then BUSY (inFlight).
		// Second pass is allowed ONLY when the pool cannot grow anymore.
		for pass := 0; pass < 2 && freeJobs > 0; pass++ {
			if pass == 1 && poolCanGrow {
				break // skip BUSY pass if pool can grow
			}
			for srvID, srv := range pool.pool {
				idle := inFlight[srvID] == 0 // not in inFlight: srv is idle
				if ///////////////////////////////// SKIP SERVER IF:
				(pass == 0 && !idle) ||           // - BUSY srv on IDLE pass
				(pass == 1 && idle) ||            // - IDLE srv on BUSY pass
				len(srv.PendingChecks) == 0 ||    // - nothing to do now
				srv.NextQueryAt.After(now) ||     // - per‑server rate‑limit
				!sched.RateLimiter.ConsumeOne() { // - global RPS exceeded
					continue
				}
				select {
				case sched.JobLimiter <- struct{}{}:
					// We can schedule a new test for this server
					inFlight[srvID]++
					busyJobs = max(busyJobs, len(sched.JobLimiter))
					checkID := srv.PendingChecks[0]
					srv.PendingChecks = srv.PendingChecks[1:]
					sched.waitGroup.Add(1)
					go runDNSWorker(
						srv, &template[checkID],
						srvID, checkID, qryTimeout, sched,
					)
					srv.NextQueryAt = now.Add(srvReqInterval)
					freeJobs--
					numScheduled++
					if idle {
						numScheduledIdle++
					} else {
						numScheduledBusy++
					}
				default:
					// No free worker slot: give back the consumed token
					sched.RateLimiter.GiveBackOne()
				}
			}
		}
		// notify num of requests just scheduled (for RPS count)
		if numScheduled > 0 {
			status.LogRequests(now, numScheduledIdle, numScheduledBusy)
			status.UpdateBusyJobs(busyJobs)
		}
		// 3) termination condition ------------------------------------------
		if pool.IsDrained() {
			break // every server processed and pool emptied
		}
		// 4) refill pool if we have job/req budget
		poolGrowth := 0
		if poolCanGrow && freeJobs != 0 {
			freeReqs := sched.RateLimiter.Remaining()
			toLoad := min(freeReqs, freeJobs)
			if toLoad > 0 {
				poolGrowth = pool.LoadN(toLoad)
				if poolGrowth > 0 {
					status.UpdatePoolSize(pool.Len())
					status.Debug("expand pool by %d. newsz=%d", poolGrowth, pool.Len())
				}
			}
		}
		if numScheduled == 0 && poolGrowth == 0 {
			status.UpdatePoolSize(pool.Len())
			time.Sleep(13 * time.Millisecond) // avoid busy‑wait
		}
	}
	// END) Once all works are done, wait for remaining workers
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
	if chk.AttemptsLeft > 0 && res.Answer.IsWorthRetrying() {
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
