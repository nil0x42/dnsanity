package dnsanitize

// All tests below aim for near‑complete coverage of the dnsanitize package.
// Comments are deliberately kept in **English** as required by the project
// guidelines, while normal assistant explanations remain in French.

import (
    "context"
    "sync"
    "sync/atomic"
    "testing"
    "time"

    dnst "github.com/nil0x42/dnsanity/internal/dns"
    "github.com/nil0x42/dnsanity/internal/display"
)

// ---------------------------------------------------------------------------
// TokenBucket ----------------------------------------------------------------
// ---------------------------------------------------------------------------

// TestTokenBucketBasic exercises creation, consume / give‑back semantics, the
// periodic refiller, and idempotent StartRefiller.
func TestTokenBucketBasic(t *testing.T) {
    t.Parallel()

    tb := NewTokenBucket(100, 10*time.Millisecond)
    if tb.Remaining() != 1 {
        t.Fatalf("expected 1 token at start, got %d", tb.Remaining())
    }
    // Consume initial token – bucket is now empty
    if !tb.ConsumeOne() {
        t.Fatal("ConsumeOne should have succeeded on non-empty bucket")
    }
    if tb.Remaining() != 0 {
        t.Fatalf("expected 0 tokens after consume, got %d", tb.Remaining())
    }

    // Start the background refiller and wait for at least one tick.
    tb.StartRefiller()
    time.Sleep(25 * time.Millisecond) // ≥ 2×10 ms → guaranteed one refill

    if tb.Remaining() != 1 {
        t.Fatalf("expected bucket to be refilled back to 1 token, got %d", tb.Remaining())
    }

    // Idempotency of StartRefiller / StopRefiller
    tb.StartRefiller()
    tb.StopRefiller()

    if !tb.ConsumeOne() {
        t.Fatal("ConsumeOne should have succeeded on non‑empty bucket")
    }
    if tb.Remaining() != 0 {
        t.Fatalf("expected 0 tokens after consume, got %d", tb.Remaining())
    }
    if tb.ConsumeOne() {
        t.Fatal("ConsumeOne should fail when the bucket is empty")
    }

    tb.GiveBackOne()
    if tb.Remaining() != 1 {
        t.Fatalf("GiveBackOne failed to return a token, remaining=%d", tb.Remaining())
    }

    tb.StartRefiller()
    // Second call must be harmless (sync.Once protection).
    tb.StartRefiller()

    // Wait long enough for at least one tick (10 ms interval * 2).
    time.Sleep(25 * time.Millisecond)
    if tb.Remaining() != 1 {      // ✅  Le seau doit rester à 1
        t.Fatalf("expected 1 token after refill, got %d", tb.Remaining())
    }

    tb.StopRefiller()
}

// TestTokenBucketConcurrency hammers ConsumeOne from many goroutines to ensure
// lock‑free CAS loops hold under contention.
func TestTokenBucketConcurrency(t *testing.T) {
    t.Parallel()

    tb := NewTokenBucket(1000, time.Millisecond) // 1 ms interval, 1 token
    tb.StartRefiller()
    defer tb.StopRefiller()

    var successes int32
    var wg sync.WaitGroup
    for i := 0; i < 200; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            if tb.ConsumeOne() {
                atomic.AddInt32(&successes, 1)
            }
        }()
    }
    wg.Wait()
    if successes == 0 {
        t.Fatal("concurrency test: no goroutine managed to consume a token")
    }
}

// TestGiveBackOverflow verifies that GiveBackOne never exceeds MaxTokens.
func TestGiveBackOverflow(t *testing.T) {
    t.Parallel()

    tb := NewTokenBucket(10, time.Second) // 10 tokens burst
    // Bucket starts full – attempting to give back must keep it capped.
    tb.GiveBackOne()
    if tb.Remaining() != int(tb.maxTokens.Load()) {
        t.Fatalf("cap exceeded: remaining=%d, cap=%d", tb.Remaining(), tb.maxTokens.Load())
    }
}

// TestNewTokenBucketValidation expects a panic on invalid parameters.
func TestNewTokenBucketValidation(t *testing.T) {
    t.Parallel()

    defer func() {
        if r := recover(); r == nil {
            t.Fatal("expected panic for rate=0, got none")
        }
    }()
    _ = NewTokenBucket(0, time.Second)
}

// ---------------------------------------------------------------------------
// ServerPool ----------------------------------------------------------------
// ---------------------------------------------------------------------------

// TestServerPoolLifecycle covers LoadN/Unload/NumPending/IsDrained paths.
func TestServerPoolLifecycle(t *testing.T) {
    t.Parallel()

    tmpl := []dnst.DNSAnswer{{Domain: "example.com", Status: "NXDOMAIN"}}
    ips := []string{"192.0.2.1", "192.0.2.2", "192.0.2.3"}

    sp := NewServerPool(ips, tmpl, 2, 5, 0, 1)

    if len(sp.pool) != 2 {
        t.Fatalf("expected initial pool size 2, got %d", len(sp.pool))
    }
    if sp.NumPending() != 1 {
        t.Fatalf("pending count mismatch: want 1, got %d", sp.NumPending())
    }

    inserted := sp.LoadN(5) // only one remains
    if inserted != 1 || sp.NumPending() != 0 {
        t.Fatalf("LoadN or NumPending incorrect: inserted=%d pending=%d", inserted, sp.NumPending())
    }

    // Unload everything and ensure IsDrained toggles.
    for slot := range sp.pool {
        sp.Unload(slot)
    }
    if !sp.IsDrained() {
        t.Fatal("server pool should be fully drained after unloads")
    }
}

// ---------------------------------------------------------------------------
// applyResults --------------------------------------------------------------
// ---------------------------------------------------------------------------

// helperServer creates a minimal ServerContext with the desired attempts.
func helperServer(attempts int) *dnst.ServerContext {
    ctx, cancel := context.WithCancel(context.Background())
    return &dnst.ServerContext{
        Ctx:            ctx,
        CancelCtx:      cancel,
        IPAddress:      "192.0.2.9",
        PendingChecks:  []int{},
        Checks:         []dnst.CheckContext{{AttemptsLeft: attempts, MaxAttempts: attempts}},
    }
}

// TestApplyResults exercises success, retry and final‑failure paths.
func TestApplyResults(t *testing.T) {
    t.Parallel()

    st := &display.Status{}
    dummyRes := WorkerResult{
        SlotID:  0,
        CheckID: 0,
        Answer:  dnst.DNSAnswer{Domain: "example.com", Status: "NXDOMAIN"},
        Passed:  true,
    }

    // Success path
    srv := helperServer(1)
    applyResults(srv, &dummyRes, 1, st)
    if srv.CompletedCount != 1 || srv.FailedCount != 0 || !srv.Checks[0].Passed {
        t.Fatal("applyResults success path failed")
    }

    // Retry path (AttemptsLeft>0, first failure)
    srv = helperServer(2)
    dummyRes.Passed = false
    applyResults(srv, &dummyRes, 2, st)
    if len(srv.PendingChecks) != 1 || srv.Checks[0].AttemptsLeft != 1 {
        t.Fatal("applyResults retry logic incorrect")
    }

    // Final failure → server disabled when maxFailures reached.
    srv = helperServer(1)
    applyResults(srv, &dummyRes, 0, st) // maxFailures==0 → immediate drop
    if !srv.Disabled || srv.FailedCount != 1 {
        t.Fatal("applyResults final failure logic incorrect")
    }
}

// ---------------------------------------------------------------------------
// Mini end‑to‑end scheduler run ---------------------------------------------
// ---------------------------------------------------------------------------

// TestDNSanitizeEndToEnd executes a single DNS query against an unreachable
// TEST‑NET‑1 address (RFC 5737). The query should fail fast (<1 s) and drive
// the entire scheduler, covering runDNSWorker & scheduleChecks paths.
func TestDNSanitizeEndToEnd(t *testing.T) {
    t.Parallel()

    status := &display.Status{}
    checks := []dnst.DNSAnswer{{Domain: "example.invalid", Status: "NXDOMAIN"}}
    servers := []string{"192.0.2.123"} // guaranteed unroutable

    DNSanitize(
        servers,
        checks,
        5,  // global RPS
        1,  // max threads
        1,  // per‑server RPS
        1,  // timeout (s)
        0,  // max mismatches → drop immediately
        1,  // attempts
        status,
    )

    if status.InvalidServers != 1 {
        t.Fatalf("expected exactly 1 invalid server, got %d", status.InvalidServers)
    }
}
