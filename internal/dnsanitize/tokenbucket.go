package dnsanitize

import (
	"context"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// TokenBucket implements a high-performance token bucket rate limiter
// using Go 1.19 atomic wrappers. It guarantees zero mutex usage,
// idempotent start/stop, and strict parameter validation.
type TokenBucket struct {
	tokens         atomic.Uint64  // current number of available tokens
	maxTokens      atomic.Uint64  // maximum number of tokens (burst capacity)
	refillAmount   atomic.Uint64  // tokens added each interval
	refillInterval time.Duration // interval between refills

	startOnce sync.Once          // ensures StartRefiller is called only once
	ctx       context.Context    // context for cancellation
	cancel    context.CancelFunc // cancellation function
}

// NewTokenBucket creates a new TokenBucket given a desired rate (RPS) and burst duration.
// It panics if parameters are invalid or lead to impossible internal values.
func NewTokenBucket(globalRateLimit uint64, burstTime time.Duration) *TokenBucket {
	// validate inputs
	if globalRateLimit < 1 {
		panic("dnsanitize: globalRateLimit must be >= 1")
	}
	if burstTime <= 0 {
		panic("dnsanitize: burstTime must be > 0")
	}

	// calculate desired tokens as float and round
	floatTokens := float64(globalRateLimit) * burstTime.Seconds()
	refillAmount := uint64(math.Round(floatTokens))
	if refillAmount < 1 {
		panic("dnsanitize: computed refillAmount < 1; burstTime too small relative to rate")
	}

	// derive interval to ensure exact average rate
	realInterval := time.Duration(
		float64(refillAmount)/float64(globalRateLimit)*float64(time.Second),
	)
	if realInterval < time.Nanosecond {
		panic("dnsanitize: computed refillInterval < 1ns; parameters too extreme")
	}

	// initialize context for cancellation
	ctx, cancel := context.WithCancel(context.Background())

	// build bucket
	tb := &TokenBucket{
		refillInterval: realInterval,
		ctx:             ctx,
		cancel:          cancel,
	}
	tb.tokens.Store(refillAmount)
	tb.maxTokens.Store(refillAmount)
	tb.refillAmount.Store(refillAmount)
	return tb
}

// StartRefiller begins a background goroutine that refills tokens at fixed intervals.
// It is safe to call multiple times; the refiller will only start once.
func (tb *TokenBucket) StartRefiller() {
	tb.startOnce.Do(func() {
		ticker := time.NewTicker(tb.refillInterval)
		go func() {
			defer ticker.Stop()
			for {
				select {
				case <-tb.ctx.Done():
					return
				case <-ticker.C:
					tb.refillOnce()
				}
			}
		}()
	})
}

// StopRefiller stops the background refill goroutine. It is safe to call multiple times.
func (tb *TokenBucket) StopRefiller() {
	// cancel is idempotent and thread-safe
	tb.cancel()
}

// refillOnce performs a single refill operation with CAS loop and minimal backoff.
func (tb *TokenBucket) refillOnce() {
	const maxSpins = 10
	for spins := 0; ; spins++ {
		old := tb.tokens.Load()
		want := old + tb.refillAmount.Load()
		max := tb.maxTokens.Load()
		if want > max {
			want = max
		}
		if tb.tokens.CompareAndSwap(old, want) {
			return
		}
		if spins >= maxSpins {
			runtime.Gosched()
			spins = 0
		}
	}
}

// ConsumeOne attempts to remove one token. Returns true if successful.
// This method is lock-free and non-blocking.
func (tb *TokenBucket) ConsumeOne() bool {
	const maxSpins = 10
	for spins := 0; ; spins++ {
		old := tb.tokens.Load()
		if old == 0 {
			return false
		}
		if tb.tokens.CompareAndSwap(old, old-1) {
			return true
		}
		if spins >= maxSpins {
			runtime.Gosched()
			spins = 0
		}
	}
}

// GiveBackOne returns one token back into the bucket if not already full.
// This method is lock-free and non-blocking.
func (tb *TokenBucket) GiveBackOne() {
	const maxSpins = 10
	for spins := 0; ; spins++ {
		old := tb.tokens.Load()
		max := tb.maxTokens.Load()
		if old >= max {
			return
		}
		if tb.tokens.CompareAndSwap(old, old+1) {
			return
		}
		if spins >= maxSpins {
			runtime.Gosched()
			spins = 0
		}
	}
}

// Remaining returns the number of remaining reqs that can be done right now
func (tb *TokenBucket) Remaining() int {
	return int(tb.tokens.Load())
}
