package dnsanitize

import (
	"sync"
	"time"

	"github.com/nil0x42/dnsanity/internal/dns"
)

type serverTest struct {
	Answer				dns.DNSAnswer
	IsOk				bool
	RemainingAttempts	int
	MaxAttempts			int
}

type ServerState struct {
	// IP is the server address (IPv4).
	IP				string
	// Disabled indicates whether the server was disabled at some point
	// (due to reaching the maxFailures threshold), so the remaining tests
	// were not executed.
	Disabled		bool
	// NumFailed is how many tests failed for this server.
	NumFailed		int
	// NumCompleted: how many tests are completed (failed + succeeded)
	NumCompleted	int
	// at which time a new dns query can be made:
	NextAllowedTime	time.Time
	// a queue of test ids that must still be completed:
	TestsTodo		[]int
	// tests results per server:
	Tests			[]serverTest
}
// ServerState.Finished():
func (srv *ServerState) Finished() bool {
	return srv.Disabled || srv.NumCompleted == len(srv.Tests)
}


// resultMsg is used by worker goroutines to send back the final DNSAnswer.
type resultMsg struct {
	srvIdx		int
	testIdx		int
	answer		dns.DNSAnswer
	isOk		bool
}


type schedulerStruct struct {
	waitGroup	sync.WaitGroup	// to wait for remaining workers at the end
	semaphore	chan struct{}	// concurrency limiter
	results		chan resultMsg	// worker results are sent here:
}


func runDNSWorker(
	srvIp string, test *dns.DNSAnswer, srvIdx int, testIdx int,
	timeout time.Duration, sched *schedulerStruct,
) {
	defer sched.waitGroup.Done()
	answer := dns.ResolveDNS(test.Domain, srvIp, timeout)
	sched.results <- resultMsg{
		srvIdx:  srvIdx,
		testIdx: testIdx,
		answer:  *answer,
		isOk:	 test.Equals(answer),
	}
	<- sched.semaphore
}

// DNSanitize prepares the data structures and invokes the scheduling loop.
//
// servers:      DNS servers to test, each one is an IP address or validated host.
// tests:        DNS tests to execute (in order 0..len(tests)-1).
// maxThreads:   Global limit of concurrent goroutines (all servers combined).
// rateLimit:    Max number of DNS requests per second, per server. (0 or negative = no limit.)
// maxFailures:  Failure threshold. If >= 0, a server is disabled after that many failures.
//               If 0 => disable on first failure. If -1 => never disable the server.
//
// Returns a ServerState slice, one entry per server, with each DNSAnswer's outcome.
func DNSanitize(
	servers []string,
	tests []dns.DNSAnswer,
	globRateLimit int,
	maxThreads int,
	rateLimit int,
	timeout int,
	maxFailures int,
	maxAttempts int,
	onTestDone func(int, int, int),
) []ServerState {

	if maxAttempts < 1 {
		maxAttempts = 1 // can't perform less than 1 attempt !
	}

	// Initialize slice of ServerStates:
	srvStates := make([]ServerState, len(servers))
	for i, srv := range servers {
		srvStates[i] = ServerState{
			IP:					srv,
			Disabled:			false,
			NumFailed:			0,
			NextAllowedTime:	time.Time{},
			TestsTodo:			make([]int, len(tests)),
			Tests:				make([]serverTest, len(tests)),
		}
		for j := range tests {
			srvStates[i].TestsTodo[j] = j;
			srvStates[i].Tests[j].Answer.Domain = tests[j].Domain
			// defaults to 'SKIPPED' before being overridden (or not):
			srvStates[i].Tests[j].Answer.Status = "SKIPPED"
			srvStates[i].Tests[j].RemainingAttempts = maxAttempts
			srvStates[i].Tests[j].MaxAttempts = maxAttempts
		}
	}
	// If total tests < maxThreads, limit maxThreads to total test count
	totalTests := len(servers) * len(tests)
	if maxThreads > totalTests {
		maxThreads = totalTests
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
	// callback is nil -> dummy func
    if onTestDone == nil {
        onTestDone = func(_ int, __ int, ___ int) {}
    }
	// init scheduler objects
	sched := schedulerStruct{
		waitGroup:		sync.WaitGroup{},
		semaphore:		make(chan struct{}, maxThreads),
		results:		make(chan resultMsg, maxThreads),
	}
	// Run the scheduling loop to fill out srvStates
	scheduleTests(
		srvStates, tests, sched,
		globRateLimit, reqInterval, timeoutDuration, maxFailures,
		onTestDone,
	)
	return srvStates
}

// scheduleTests is the core scheduler that dispatches DNS queries, observes
// concurrency limits, rate limits, and failure thresholds.
func scheduleTests(
	servers []ServerState,
	tests []dns.DNSAnswer,
	sched schedulerStruct,
	globRateLimit int,
	reqInterval time.Duration,
	timeout time.Duration,
	maxFailures int,
	onTestDone func(int, int, int),
) {
	globRateLimit_tokenBucket := NewTokenBucket(globRateLimit, time.Second)
	globRateLimit_tokenBucket.StartRefiller()

	for {
		// 1) Collect newly completed results (non-blocking).
		collectLoop:
		for {
			select {
			case res := <- sched.results:
				applyResults(
					&servers[res.srvIdx], &res, maxFailures, onTestDone,
				)
			default:
				break collectLoop
			}
		}
		// 2) Attempt to schedule new tests if resources are available
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
			// skip servers whose NextAllowedTime is in the future
			// skip servers with an empty queue (TestsTodo)
			if srv.NextAllowedTime.After(now) || len(srv.TestsTodo) == 0 {
				continue
			}
			// consume a globRateLimit_tokenbucket token if available
			if ! globRateLimit_tokenBucket.consumeOne() {
				continue
			}
			select {
			case sched.semaphore <- struct{}{}:
				// We can schedule a new test for this server
				testIdx := srv.TestsTodo[0]
				srv.TestsTodo = srv.TestsTodo[1:]
				sched.waitGroup.Add(1)
				go runDNSWorker(
					srv.IP, &tests[testIdx],
					i, testIdx, timeout, &sched,
				)
				srv.NextAllowedTime = now.Add(reqInterval)
				numScheduled++
			default:
				// No free slot right now
				// give back consumed globRateLimit_tokenbucket token
				globRateLimit_tokenBucket.giveBackOne()
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
	close(sched.results)
	for res := range sched.results {
		applyResults(&servers[res.srvIdx], &res, maxFailures, onTestDone)
	}
	globRateLimit_tokenBucket.StopRefiller()
}

// update results from scheduler (main thread, no concurrency)
func applyResults(
	srv *ServerState, res *resultMsg,
	maxFailures int, onTestDone func(int, int, int),
) {

	srv.Tests[res.testIdx].RemainingAttempts-- // decrement remaining attempts
	srv.Tests[res.testIdx].Answer = res.answer // update answer
	// test succeeded:
	if res.isOk {
		srv.NumCompleted++ // this testId is now completed
		srv.Tests[res.testIdx].IsOk = true
		if !srv.Disabled {
			onTestDone(1, 0, 0)
		} else {
			onTestDone(1, 1, 0)
		}
	// test failed WITHOUT remaining attempts:
	} else if srv.Tests[res.testIdx].RemainingAttempts <= 0 {
		srv.NumCompleted++ // this testId is now completed
		srv.NumFailed++
		// triggered max-mismatches:
		if srv.NumFailed >= maxFailures {
			if !srv.Disabled {
				leftover := (len(srv.Tests) - srv.NumCompleted)
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
		srv.TestsTodo = append([]int{res.testIdx}, srv.TestsTodo...)
		if !srv.Disabled {
			onTestDone(1, 1, 0)
		} else {
			onTestDone(1, 1, 0)
		}
	}
}
