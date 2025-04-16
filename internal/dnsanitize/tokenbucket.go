package dnsanitize

import (
	"math"
	"sync"
	"time"
)

// tokenBucket limits the consumption of "tokens" (or requests) over time.
// We define a "globalRateLimit" in RPS (requests per second)
// and a desired "burstTime" which indicates how large a burst we might want.
// However, to keep an integer number of tokens (no fractions), we must:
// 1) Round the product (globalRateLimit * burstTime.Seconds()) to get refillAmount.
// 2) Recompute the actual refill interval so that the average is exactly the desired RPS.
//    realIntervalSec = refillAmount / globalRateLimit.
//    That way, we add 'refillAmount' tokens every 'realIntervalSec' seconds,
//    giving an exact average of 'globalRateLimit' tokens/s.
// 3) We override the stored "burstTime" to match the new refillInterval,
//    so that we can see the actual timing in the struct if needed.

// Example: if globalRateLimit=3 and burstTime=500ms,
// floatTokens = 3 * 0.5 = 1.5 => rounding => 2 tokens.
// Then realIntervalSec = 2 / 3 = ~0.666..., so we add 2 tokens every ~666ms.
// That yields an average of exactly 3 tokens per second.

type tokenBucket struct {
	mu             sync.Mutex
	tokens         int           // current number of available tokens
	maxTokens      int           // maximum number of allowed tokens
	refillInterval time.Duration // actual interval between refills
	refillAmount   int           // number of tokens added each refill

	stopCh   chan struct{}
	stopped  bool
}

// NewTokenBucket calculates an integer-based token bucket.
// globalRateLimit = RPS, burstTime is the desired burst.
// Steps:
//  1) round the product float64(RPS) * burstTime.Seconds() to find refillAmount.
//  2) compute realIntervalSec = refillAmount / float64(RPS).
//  3) set refillInterval to that real value, override burstTime.
//  4) the bucket is initialized full (tokens = maxTokens), where maxTokens = refillAmount.
func NewTokenBucket(globalRateLimit int, burstTime time.Duration) *tokenBucket {
	if globalRateLimit < 1 {
		globalRateLimit = 1
	}
	if burstTime <= 0 {
		// fallback to avoid zero or negative
		burstTime = time.Second
	}

	// 1) compute floatTokens and round
	floatTokens := float64(globalRateLimit) * burstTime.Seconds()
	refillAmount := int(math.Round(floatTokens))
	if refillAmount < 1 {
		refillAmount = 1
	}

	// 2) compute the actual interval so that average is exactly globalRateLimit
	realIntervalSec := float64(refillAmount) / float64(globalRateLimit)
	realInterval := time.Duration(realIntervalSec * float64(time.Second))

	// We'll store refillAmount as both "maxTokens" and "refillAmount".
	// 3) we override the user-specified burstTime with realInterval
	//    (so that StartRefiller uses the exact interval for the average RPS)
	tb := &tokenBucket{
		tokens:         refillAmount,     // start bucket full
		maxTokens:      refillAmount,
		refillInterval: realInterval,
		refillAmount:   refillAmount,
		stopCh:         nil,
		stopped:        false,
	}
	return tb
}

// StartRefiller launches a goroutine that, at each refillInterval,
// adds refillAmount tokens (capped at maxTokens).
func (tb *tokenBucket) StartRefiller() {
	tb.stopCh = make(chan struct{})
	go func() {
		ticker := time.NewTicker(tb.refillInterval)
		defer ticker.Stop()

		for {
			select {
			case <-tb.stopCh:
				return
			case <-ticker.C:
				tb.mu.Lock()
				tb.tokens += tb.refillAmount
				if tb.tokens > tb.maxTokens {
					tb.tokens = tb.maxTokens
				}
				tb.mu.Unlock()
			}
		}
	}()
}

// StopRefiller signals the refill goroutine to terminate.
func (tb *tokenBucket) StopRefiller() {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	if !tb.stopped {
		tb.stopped = true
		close(tb.stopCh)
	}
}

// consumeOne tries to remove one token from the bucket. Returns true if successful.
func (tb *tokenBucket) consumeOne() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	if tb.tokens > 0 {
		tb.tokens--
		return true
	}
	return false
}

// giveBackOne re-injects one token, e.g. if we reserved it but didn't use it.
func (tb *tokenBucket) giveBackOne() {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	if tb.tokens < tb.maxTokens {
		tb.tokens++
	}
}
