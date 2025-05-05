package dnsanitize

import (
	"sync"
	"time"
	"context"

	"github.com/nil0x42/dnsanity/internal/dns"
	"github.com/nil0x42/dnsanity/internal/display"
)


// ---------------------------------------------------------------------------
// Worker & scheduler plumbing -----------------------------------------------
// ---------------------------------------------------------------------------

// WorkerResult is used by goroutines to send back the final DNSAnswer.
type WorkerResult struct {
    SlotID              int           // server pool slot identifier
    CheckID             int           // check index
    Answer              dns.DNSAnswer // received answer
    Passed              bool          // equals? result
}

type QueryScheduler struct {
	waitGroup           sync.WaitGroup
	ConcurrencyLimiter  chan struct{}
	Results             chan WorkerResult // worker results are sent here
}


func runDNSWorker(
	serverIP	string, // IP address
	check		*dns.DNSAnswer, // template check
	slotID		int, // server ID (index within pool)
	checkID		int, // check ID (index)
	timeout		time.Duration, // DNS query timeout
	sched		*QueryScheduler, // query scheduleu
	srvCtx		context.Context,
) {
	defer sched.waitGroup.Done()
	answer := dns.ResolveDNS(check.Domain, serverIP, timeout, srvCtx)
	sched.Results <- WorkerResult{
		SlotID:			slotID,
		CheckID:		checkID,
		Answer:			*answer,
		Passed:			check.Equals(answer),
	}
	<- sched.ConcurrencyLimiter
}

// ---------------------------------------------------------------------------
//  PUBLIC ENTRYPOINT --------------------------------------------------------
// ---------------------------------------------------------------------------
func DNSanitize(
	serverIPs		[]string, // IP of DNS servers to test
	checks			[]dns.DNSAnswer, // checks from template
	globRateLimit	int, // global rate limit (max rps)
	maxThreads		int, // global limit of concurrent goroutines
	rateLimit		int, // per-server rate limit (max rps)
	timeout			int, // max seconds before DNS query timeout
	maxFailures		int, // max allowed non-passing checks per server
	maxAttempts		int, // max attempts per check before failure
	status			*display.Status,
) {

	maxAttempts = max(maxAttempts, 1)

	// limit maxThreads to total tests
	maxThreads = min(maxThreads, len(serverIPs) * len(checks))

	// create reqInterval => min interval between 2 reqs per server
	reqInterval := time.Duration(0)
	if rateLimit > 0 {
		reqInterval = time.Second / time.Duration(rateLimit)
	}

	// if maxFailures is -1 (unlimited) then set it to INT_MAX
	if maxFailures < 0 {
		maxFailures = int(^uint(0) >> 1) // INT_MAX
	}

	// create timeout duration (convert timeout to time.Duration)
	timeoutDuration := time.Duration(timeout) * time.Second

	// init scheduler
	sched := QueryScheduler{
		ConcurrencyLimiter:     make(chan struct{}, maxThreads),
		Results:                make(chan WorkerResult, maxThreads),
	}

    // init server pool
	poolSize := min(len(serverIPs), max(1, min(globRateLimit, maxThreads*2)))
	maxPoolSize := poolSize * 3
	pool := NewServerPool(serverIPs, checks, poolSize, maxPoolSize, reqInterval, maxAttempts)
	status.WithLock(func () {
		status.PoolSize = poolSize
		status.NumServersInPool = len(pool.pool)
	})


	// init global ratelimiter
	globRateLimiter := NewTokenBucket(uint64(globRateLimit), time.Second)
	globRateLimiter.StartRefiller()

	// Run the scheduling loop to fill out servers
	scheduleChecks(
		pool, checks, sched,
		globRateLimiter, reqInterval,
		timeoutDuration, maxFailures,
		status,
	)

	// stop gobal ratelimiter
	globRateLimiter.StopRefiller()
}

// scheduleChecks is the core scheduler that dispatches DNS queries,
// observes concurrency limits, rate limits, and failure thresholds.
func scheduleChecks(
	pool			*ServerPool,
	checks			[]dns.DNSAnswer, // template checks
	sched			QueryScheduler, // query scheduler
	globRateLimiter	*TokenBucket, // global rate limiter
	reqInterval		time.Duration, // min interval between 2 reqs per server
	timeout			time.Duration, // DNS query timeout
	maxFailures		int, // max allowed non-passing checks per server
	status			*display.Status,
) {
	for {
		// 1) async collection of worker results -----------------------------
		collectLoop:
		for {
			select {
			case res := <- sched.Results:
				srv, ok := pool.pool[res.SlotID]
				if !ok {
				    continue // already unloaded -> drop silently
				}
				applyResults(
					srv, &res, maxFailures, status,
				)
				if srv.Finished() {
					status.ReportFinishedServer(srv) // report server
					pool.Unload(res.SlotID) // drop server from pool
					// if
					// len(pool.pool) >= pool.defaultPoolSize ||
					// pool.LoadN(1) == 0 {
					if
					len(pool.pool) >= pool.maxPoolSize ||
					pool.LoadN(1) == 0 {
						status.WithLock(func () {
							status.NumServersInPool = len(pool.pool)
						})
					}
				}
			default:
				break collectLoop
			}
		}
		// 2) scheduling new queries -----------------------------------------
		now := time.Now()
		numScheduled := 0
		for slotID, srv := range pool.pool {
			if                             // SKIP SERVER IF:
			len(srv.PendingChecks) == 0 || //  - nothing to do at the moment
			srv.NextQueryAt.After(now) ||  //  - per-serv ratelimit not honored
			!globRateLimiter.ConsumeOne() {//  - global ratelimit not honored
				continue
			}
			select {
			case sched.ConcurrencyLimiter <- struct{}{}:
				// We can schedule a new test for this server
				CheckID := srv.PendingChecks[0]
				srv.PendingChecks = srv.PendingChecks[1:]
				sched.waitGroup.Add(1)
				go runDNSWorker(
					srv.IPAddress, &checks[CheckID],
					slotID, CheckID, timeout, &sched, srv.Ctx,
				)
				srv.NextQueryAt = now.Add(reqInterval)
				numScheduled++
			default:
				// No free slot: give back consumed ratelimit token:
				globRateLimiter.GiveBackOne()
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
			availableReqs := globRateLimiter.Remaining()
			if availableReqs > 0 && pool.NumPending() > 0 && !pool.IsFull() {
				inserted := pool.LoadN(availableReqs)
				status.Debug(
					"expand pool by %d. newsz=%d",
					inserted, len(pool.pool))
				status.WithLock(func () {
					status.PoolSize = max(status.PoolSize, len(pool.pool))
					status.NumServersInPool = len(pool.pool)
				})
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
// Status.WithLock() is only for the progress-bar goroutine.
func applyResults(
	srv				*dns.ServerContext, // server
	res				*WorkerResult, // worker result
	maxFailures		int, // max allowed non-passing checks per server
	status			*display.Status,
) {
	chk := &srv.Checks[res.CheckID]
	chk.AttemptsLeft--
	chk.Answer = res.Answer
	/* ---------- success ------------------------------------------------ */
	if res.Passed {
		chk.Passed = true
		srv.CompletedCount++
		status.WithLock(func () {
			status.DoneChecks++
		})
		return
	}
	/* ---------- failure, retry remaining ------------------------------- */
	if chk.AttemptsLeft > 0 {
		// re-queue the check at the front
		srv.PendingChecks = append([]int{res.CheckID}, srv.PendingChecks...)
		status.WithLock(func() {
			status.DoneChecks++
			status.TotalChecks++
		})
		return
	}
	/* ---------- failure, no retry left --------------------------------- */
	srv.CompletedCount++
	srv.FailedCount++
	// reached drop threshold?
	if srv.FailedCount >= maxFailures {
		// how many planned checks are immediately cancelled
		cancelled := len(srv.Checks) - srv.CompletedCount
		status.WithLock(func () {
			status.DoneChecks++
			status.TotalChecks -= cancelled
		})
		srv.Disabled = true
		srv.CancelCtx()
	} else {
		status.WithLock(func () {
			status.DoneChecks++
		})
	}
}
