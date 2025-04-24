package dnsanitize

import (
	"sync"
	"time"

	"github.com/nil0x42/dnsanity/internal/dns"
)

type CheckContext struct {
	Answer				dns.DNSAnswer
	Passed				bool
	AttemptsLeft		int
	MaxAttempts			int
}

type ServerContext struct {
	IPAddress			string // server's IPv4
	Disabled			bool // True if reached maxFailures
	FailedCount			int // failed checks.
	CompletedCount		int // completed checks (failed + succeeded)
	NextQueryAt			time.Time // to honour per-server ratelimit
	PendingChecks		[]int // list of pending Check IDs
	Checks				[]CheckContext // results of checks
}
// ServerContext.Finished():
func (srv *ServerContext) Finished() bool {
	return srv.Disabled || srv.CompletedCount == len(srv.Checks)
}


// WorkerResult is used by worker goroutines to send back the final DNSAnswer.
type WorkerResult struct {
	ServerID			int
	CheckID				int
	Answer				dns.DNSAnswer
	Passed				bool
}


type QueryScheduler struct {
	waitGroup			sync.WaitGroup
	ConcurrencyLimiter	chan struct{}
	Results				chan WorkerResult // worker results are sent here:
}


func runDNSWorker(
	serverIP	string, // IP address
	check		*dns.DNSAnswer, // template check
	serverID	int, // server ID (index within pool)
	checkID		int, // check ID (index)
	timeout		time.Duration, // DNS query timeout
	sched		*QueryScheduler, // query scheduler
) {
	defer sched.waitGroup.Done()
	answer := dns.ResolveDNS(check.Domain, serverIP, timeout)
	sched.Results <- WorkerResult{
		ServerID:		serverID,
		CheckID:		checkID,
		Answer:			*answer,
		Passed:			check.Equals(answer),
	}
	<- sched.ConcurrencyLimiter
}

// DNSanitize prepares the data structures and invokes the scheduling loop.
// Returns a ServerContext slice, one entry per server, with each DNSAnswer's outcome.
func DNSanitize(
	serverIPs		[]string, // IP of DNS servers to test
	checks			[]dns.DNSAnswer, // checks from template
	globRateLimit	int, // global rate limit (max rps)
	maxThreads		int, // global limit of concurrent goroutines
	rateLimit		int, // per-server rate limit (max rps)
	timeout			int, // max seconds before DNS query timeout
	maxFailures		int, // max allowed non-passing checks per server
	maxAttempts		int, // max attempts per check before failure
	onTestDone		func(int, int, int), // callback
) []ServerContext {

	// can't perform less than 1 attempt !
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	// Initialize ServerContexts:
	servers := make([]ServerContext, len(serverIPs))
	for i, serverIP := range serverIPs {
		servers[i] = ServerContext{
			IPAddress:		serverIP,
			Disabled:		false,
			FailedCount:	0,
			NextQueryAt:	time.Time{},
			PendingChecks:	make([]int, len(checks)),
			Checks:			make([]CheckContext, len(checks)),
		}
		for j := range checks {
			servers[i].PendingChecks[j] = j;
			servers[i].Checks[j].Answer.Domain = checks[j].Domain
			// defaults to 'SKIPPED' before being overridden (or not):
			servers[i].Checks[j].Answer.Status = "SKIPPED"
			servers[i].Checks[j].AttemptsLeft = maxAttempts
			servers[i].Checks[j].MaxAttempts = maxAttempts
		}
	}
	// If total checks < maxThreads, limit maxThreads to total test count
	totalChecks := len(serverIPs) * len(checks)
	if maxThreads > totalChecks {
		maxThreads = totalChecks
	}
	// create reqInterval => min interval between 2 reqs per server
	reqInterval := time.Duration(0)
	if rateLimit > 0 {
		reqInterval = time.Second / time.Duration(rateLimit)
	}
	// if maxFailures is -1 (unlimited) then set it to INT_MAX
	if maxFailures < 0 {
		maxFailures = int(^uint(0) >> 1)
	}
	// create timeout duration (convert timeout to time.Duration)
	timeoutDuration := time.Duration(timeout) * time.Second
	// init scheduler objects
	sched := QueryScheduler{
		waitGroup:				sync.WaitGroup{},
		ConcurrencyLimiter:		make(chan struct{}, maxThreads),
		Results:				make(chan WorkerResult, maxThreads),
	}
	// init global ratelimiter
	globRateLimiter := NewTokenBucket(uint64(globRateLimit), time.Second)
	globRateLimiter.StartRefiller()
	// Run the scheduling loop to fill out servers
	scheduleChecks(
		servers, checks, sched,
		globRateLimiter, reqInterval,
		timeoutDuration, maxFailures,
		onTestDone,
	)
	globRateLimiter.StopRefiller() // stop gobal ratelimiter
	return servers
}

// scheduleChecks is the core scheduler that dispatches DNS queries,
// observes concurrency limits, rate limits, and failure thresholds.
func scheduleChecks(
	servers			[]ServerContext, // servers
	checks			[]dns.DNSAnswer, // template checks
	sched			QueryScheduler, // query scheduler
	globRateLimiter	*TokenBucket, // global rate limiter
	reqInterval		time.Duration, // min interval between 2 reqs per server
	timeout			time.Duration, // DNS query timeout
	maxFailures		int, // max allowed non-passing checks per server
	onTestDone		func(int, int, int), // callback
) {
	for {
		// 1) Collect newly completed results (non-blocking).
		collectLoop:
		for {
			select {
			case res := <- sched.Results:
				applyResults(
					&servers[res.ServerID], &res, maxFailures, onTestDone,
				)
			default:
				break collectLoop
			}
		}
		// 2) Attempt to schedule new checks if resources are available
		now := time.Now()
		numScheduled := 0
		numFinished := 0
		for i := range servers {
			srv := &servers[i]
			// skip finished servers:
			if srv.Finished() {
				numFinished++
				continue
			}
			// skip servers whose NextQueryAt is in the future
			// skip servers with an empty queue (PendingChecks)
			if srv.NextQueryAt.After(now) || len(srv.PendingChecks) == 0 {
				continue
			}
			// consume a 'request' token if available
			if ! globRateLimiter.ConsumeOne() {
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
					i, CheckID, timeout, &sched,
				)
				srv.NextQueryAt = now.Add(reqInterval)
				numScheduled++
			default:
				// No free slot: give back consumed ratelimit token:
				globRateLimiter.GiveBackOne()
			}
		}
		// Check if we're done
		if numFinished == len(servers) {
			break
		}
		// If we didn't schedule anything, sleep briefly to avoid busy-wait
		if numScheduled == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}
	// END) Once all works are done, close chan & read remaining msgs:
	sched.waitGroup.Wait()
	close(sched.Results)
	for res := range sched.Results {
		applyResults(
			&servers[res.ServerID], &res, maxFailures, onTestDone,
		)
	}
}

// update results from scheduler (main thread, no concurrency)
func applyResults(
	srv				*ServerContext, // server
	res				*WorkerResult, // worker result
	maxFailures		int, // max allowed non-passing checks per server
	onTestDone		func(int, int, int), // callback
) {
	srv.Checks[res.CheckID].AttemptsLeft--
	srv.Checks[res.CheckID].Answer = res.Answer
	// test succeeded:
	if res.Passed {
		srv.CompletedCount++
		srv.Checks[res.CheckID].Passed = true
		if !srv.Disabled {
			onTestDone(1, 0, 0)
		} else {
			onTestDone(1, 1, 0)
		}
	// test failed WITHOUT remaining attempts:
	} else if srv.Checks[res.CheckID].AttemptsLeft <= 0 {
		srv.CompletedCount++ // this testId is now completed
		srv.FailedCount++
		// triggered max-mismatches:
		if srv.FailedCount >= maxFailures {
			if !srv.Disabled {
				leftover := (len(srv.Checks) - srv.CompletedCount)
				onTestDone(1, -leftover, 1)
				srv.Disabled = true
			} else {
				onTestDone(1, 1, 0)
			}
		// did NOT trigger max-mismatches:
		} else {
			if !srv.Disabled {
				onTestDone(1, 0, 0)
			} else {
				onTestDone(1, 1, 0)
			}
		}
	// test failed WITH remaining attempts
	} else {
		srv.PendingChecks = append([]int{res.CheckID}, srv.PendingChecks...)
		if !srv.Disabled {
			onTestDone(1, 1, 0)
		} else {
			onTestDone(1, 1, 0)
		}
	}
}
