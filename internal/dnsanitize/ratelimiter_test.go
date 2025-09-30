package dnsanitize

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// expectPanic executes fn and ensures it panics – helper for invalid constructor parameters.
func expectPanic(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("%s: expected panic, but none occurred", name)
		}
	}()
	fn()
}

func TestRateLimiterInvalidParamsPanics(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		rate  int
		burst time.Duration
	}{
		{"rate zero", 0, time.Second},
		{"burst zero", 10, 0},
		{"burst too small", 1000, time.Nanosecond},
	}
	for _, tc := range cases {
		expectPanic(t, tc.name, func() {
			_ = NewRateLimiter(tc.rate, tc.burst)
		})
	}
}

func TestRateLimiterConsumeAndGiveBack(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(10, time.Second) // 10 tokens, 1s interval
	defer rl.StopRefiller()

	if got := rl.Remaining(); got != 10 {
		t.Fatalf("initial Remaining() = %d, want 10", got)
	}

	// consume all tokens
	for i := 0; i < 10; i++ {
		if !rl.ConsumeOne() {
			t.Fatalf("ConsumeOne() returned false at iteration %d", i)
		}
	}
	if rl.ConsumeOne() {
		t.Fatalf("ConsumeOne() should fail when bucket is empty")
	}
	if got := rl.Remaining(); got != 0 {
		t.Fatalf("Remaining() after drain = %d, want 0", got)
	}

	// give back a single token
	rl.GiveBackOne()
	if got := rl.Remaining(); got != 1 {
		t.Fatalf("Remaining() after single GiveBackOne() = %d, want 1", got)
	}

	// spam GiveBackOne – should never exceed max (10)
	for i := 0; i < 20; i++ {
		rl.GiveBackOne()
	}
	if got := rl.Remaining(); got != 10 {
		t.Fatalf("Remaining() after excessive GiveBackOne() = %d, want 10", got)
	}
}

func TestRateLimiterRefill(t *testing.T) {
	t.Parallel()
	// rate = 200 rps, burst = 5ms ➜ 1 token, interval ≈5ms
	rl := NewRateLimiter(200, 5*time.Millisecond)
	defer rl.StopRefiller()

	// consume the single token
	if !rl.ConsumeOne() {
		t.Fatalf("failed to consume initial token")
	}
	if rl.Remaining() != 0 {
		t.Fatalf("bucket not empty after consume")
	}

	// wait long enough for at least one refill tick
	time.Sleep(15 * time.Millisecond)
	if got := rl.Remaining(); got == 0 {
		t.Fatalf("bucket did not refill after interval")
	}
}

func TestRateLimiterStopRefiller(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(100, 10*time.Millisecond)

	// drain bucket and stop refiller quickly
	rl.ConsumeOne()
	rl.StopRefiller()
	tokensBefore := rl.Remaining()

	// wait several intervals to verify no further refills
	time.Sleep(40 * time.Millisecond)
	if got := rl.Remaining(); got != tokensBefore {
		t.Fatalf("tokens changed after StopRefiller(): before=%d after=%d", tokensBefore, got)
	}
}

func TestRateLimiterConcurrencySafety(t *testing.T) {
	t.Parallel()
	const tokens = 1000
	rl := NewRateLimiter(tokens, time.Second) // 1s burst gives tokens tokens
	rl.StopRefiller()                         // lock bucket size – no new tokens

	var consumed int64
	var wg sync.WaitGroup

	// concurrent consumption
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if rl.ConsumeOne() {
					atomic.AddInt64(&consumed, 1)
				} else {
					return
				}
			}
		}()
	}
	wg.Wait()
	if consumed != tokens {
		t.Fatalf("concurrent consumed=%d, want %d", consumed, tokens)
	}
	if rl.Remaining() != 0 {
		t.Fatalf("Remaining() after concurrent drain = %d, want 0", rl.Remaining())
	}

	// concurrent GiveBackOne up to capacity (and beyond)
	var retWG sync.WaitGroup
	for i := 0; i < tokens*2; i++ { // try to overflow bucket
		retWG.Add(1)
		go func() {
			rl.GiveBackOne()
			retWG.Done()
		}()
	}
	retWG.Wait()
	if rl.Remaining() != tokens {
		t.Fatalf("Remaining() after concurrent GiveBackOne = %d, want %d", rl.Remaining(), tokens)
	}
}
