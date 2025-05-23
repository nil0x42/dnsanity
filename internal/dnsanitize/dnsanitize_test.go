package dnsanitize

// All tests below aim for near‑complete coverage of the dnsanitize package.
// Comments are deliberately kept in **English** as required by the project
// guidelines, while normal assistant explanations remain in French.

import (
    "bytes"
    "context"
    "reflect"
    "sync"
    "sync/atomic"
    "testing"
    "time"
    "unsafe"

    "github.com/nil0x42/dnsanity/internal/config"
    "github.com/nil0x42/dnsanity/internal/dns"
    "github.com/nil0x42/dnsanity/internal/report"
)

// ---------------------------------------------------------------------------
// Helpers -------------------------------------------------------------------
// ---------------------------------------------------------------------------

// newIOFiles returns an empty IOFiles ready to be embedded inside a
// StatusReporter. Every file writer is nil, which is fine because the
// implementation checks for nil before writing.
func newIOFiles() *report.IOFiles {
    return &report.IOFiles{
        OutputFile:  bytes.NewBuffer(nil),
        VerboseFile: nil,
        DebugFile:   nil,
        TTYFile:     nil,
    }
}

// newStatus creates a *report.StatusReporter with its private `io` field set
// to a minimal instance so that calls like ReportFinishedServer() never panic.
// We must reach into the unexported field using reflect + unsafe, which is
// perfectly acceptable inside tests.
func newStatus() *report.StatusReporter {
    sr := &report.StatusReporter{}

    // Sneak‑set the private `io` field.
    rv := reflect.ValueOf(sr).Elem()
    ioField := rv.FieldByName("io") // unexported – use unsafe
    if ioField.IsValid() {
        p := unsafe.Pointer(ioField.UnsafeAddr())
        reflect.NewAt(ioField.Type(), p).Elem().Set(reflect.ValueOf(newIOFiles()))
    }
    return sr
}

// helperServer creates a minimal ServerContext with the desired AttemptsLeft.
func helperServer(attempts int) *dns.ServerContext {
    ctx, cancel := context.WithCancel(context.Background())
    return &dns.ServerContext{
        Ctx:           ctx,
        CancelCtx:     cancel,
        IPAddress:     "192.0.2.123", // TEST‑NET‑1: guaranteed unroutable
        PendingChecks: []int{},
        Checks:        []dns.CheckContext{{AttemptsLeft: attempts, MaxAttempts: attempts}},
    }
}

// dummyTemplate returns a one‑line template entry targeting an RFC‑2606 domain.
func dummyTemplate() dns.Template {
    entry := dns.TemplateEntry{
        Domain: "invalid.test", // will never resolve
        ValidAnswers: []dns.DNSAnswerData{{Status: "TIMEOUT"}},
    }
    return dns.Template{entry}
}

// ---------------------------------------------------------------------------
// min / max helpers ----------------------------------------------------------
// ---------------------------------------------------------------------------

func TestMinMaxHelpers(t *testing.T) {
    t.Parallel()

    if max(1, 2) != 2 {
        t.Fatalf("max(1,2) should be 2")
    }
    if min(1, 2) != 1 {
        t.Fatalf("min(1,2) should be 1")
    }
}

// ---------------------------------------------------------------------------
// applyResults --------------------------------------------------------------
// ---------------------------------------------------------------------------

func TestApplyResultsPaths(t *testing.T) {
    t.Parallel()

    st := newStatus()
    res := WorkerResult{SrvID: 0, CheckID: 0, Passed: true}

    // Success path ---------------------------------------------------------
    srv := helperServer(1)
    applyResults(srv, &res, 1, st)
    if srv.CompletedCount != 1 || srv.FailedCount != 0 || !srv.Checks[0].Passed {
        t.Fatal("applyResults success path failed")
    }

    // Retry path -----------------------------------------------------------
    srv = helperServer(2)
    res.Passed = false
    applyResults(srv, &res, 2, st)
    if len(srv.PendingChecks) != 1 || srv.Checks[0].AttemptsLeft != 1 {
        t.Fatal("applyResults retry path incorrect")
    }

    // Final failure → server disabled when maxFailures reached -------------
    srv = helperServer(1)
    applyResults(srv, &res, 0, st) // maxFailures==0 → immediate drop
    if !srv.Disabled || srv.FailedCount != 1 {
        t.Fatal("applyResults final failure logic incorrect")
    }
}

// ---------------------------------------------------------------------------
// runDNSWorker --------------------------------------------------------------
// ---------------------------------------------------------------------------

func TestRunDNSWorker(t *testing.T) {
    t.Parallel()

    tmpl := dummyTemplate()
    srv := dns.NewServerContext("192.0.2.45", tmpl, 1)
    // Cancel ctx to make ResolveDNS return immediately
    srv.CancelCtx()

    sched := &QueryScheduler{
        JobLimiter: make(chan struct{}, 1),
        RateLimiter: NewRateLimiter(1, time.Second),
        Results: make(chan WorkerResult, 1),
    }
    sched.JobLimiter <- struct{}{} // occupy one slot

    sched.waitGroup.Add(1)
    go runDNSWorker(srv, &tmpl[0], 0, 0, time.Millisecond*5, sched)
    sched.waitGroup.Wait()

    res := <-sched.Results
    if res.SrvID != 0 || res.CheckID != 0 || res.Answer.Domain != "invalid.test" {
        t.Fatal("runDNSWorker produced unexpected result data")
    }
}

// ---------------------------------------------------------------------------
// RateLimiter sanity & concurrency -----------------------------------------
// ---------------------------------------------------------------------------

func TestRateLimiterBasic(t *testing.T) {
    t.Parallel()

    rl := NewRateLimiter(10, 50*time.Millisecond)
    if rl.Remaining() == 0 {
        t.Fatal("expected some initial tokens in RateLimiter")
    }
    if !rl.ConsumeOne() {
        t.Fatal("ConsumeOne should succeed when tokens are available")
    }
    rl.GiveBackOne()
    if rl.Remaining() == 0 {
        t.Fatal("GiveBackOne failed to return token")
    }
    rl.StopRefiller() // idempotent
}

func TestRateLimiterConcurrency(t *testing.T) {
    t.Parallel()

    rl := NewRateLimiter(100, 10*time.Millisecond)
    defer rl.StopRefiller()

    var ok int32
    var wg sync.WaitGroup
    for i := 0; i < 200; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            if rl.ConsumeOne() {
                atomic.AddInt32(&ok, 1)
            }
        }()
    }
    wg.Wait()
    if ok == 0 {
        t.Fatal("no goroutine managed to consume a token – concurrency broken")
    }
}

// ---------------------------------------------------------------------------
// Mini end‑to‑end run through DNSanitize() ----------------------------------
// ---------------------------------------------------------------------------

func TestDNSanitizeEndToEnd(t *testing.T) {
    t.Parallel()

    settings := &config.Settings{
        ServerIPs:           []string{"192.0.2.99"}, // unroutable
        Template:            dummyTemplate(),
        MaxThreads:          2,
		MaxPoolSize:         2,
        GlobRateLimit:       50,
        PerSrvRateLimit:     1,
        PerSrvMaxFailures:   -1, // never drop
        PerCheckMaxAttempts: 1,
        PerQueryTimeout:     1,
    }

    st := newStatus()

    done := make(chan struct{})
    go func() {
        DNSanitize(settings, st)
        close(done)
    }()

    select {
    case <-done:
        // success – function returned
    case <-time.After(5 * time.Second):
        t.Fatal("DNSanitize did not finish within expected time")
    }
}
