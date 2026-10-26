package remote

import (
	"sync"
	"time"
)

// maxBuckets bounds the limiter's memory. Idle buckets are swept when the map
// grows past it; busy ones are limited by the global bucket upstream.
const maxBuckets = 10_000

// limiter is a set of token buckets keyed by string (a client IP, or "" for the
// global bucket). Each bucket holds up to burst attempts and earns one back
// every refill. An empty bucket is a lockout that lifts by itself: the caller
// is told how long to wait.
type limiter struct {
	now    func() time.Time
	burst  float64
	refill time.Duration

	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens float64
	at     time.Time
}

func newLimiter(now func() time.Time, burst int, refill time.Duration) *limiter {
	return &limiter{now: now, burst: float64(burst), refill: refill, buckets: map[string]*bucket{}}
}

// take spends one attempt for key. When none is left it reports how long until
// one is.
func (l *limiter) take(key string) (ok bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	b := l.refilledLocked(key)
	if b.tokens < 1 {
		return false, time.Duration((1 - b.tokens) * float64(l.refill))
	}
	b.tokens--
	return true, 0
}

// give returns one attempt, for a request that was counted and then turned out
// not to be a failure.
func (l *limiter) give(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.refilledLocked(key)
	b.tokens = min(b.tokens+1, l.burst)
}

// reset forgets key, as after a successful sign-in.
func (l *limiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.buckets, key)
}

// refilledLocked returns key's bucket brought up to date, creating it full.
func (l *limiter) refilledLocked(key string) *bucket {
	now := l.now()
	b, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) >= maxBuckets {
			l.sweepLocked(now)
		}
		b = &bucket{tokens: l.burst, at: now}
		l.buckets[key] = b
		return b
	}
	if elapsed := now.Sub(b.at); elapsed > 0 {
		b.tokens = min(b.tokens+float64(elapsed)/float64(l.refill), l.burst)
		b.at = now
	}
	return b
}

// sweepLocked drops buckets that have refilled completely: forgetting them
// changes nothing, since a new bucket starts full.
func (l *limiter) sweepLocked(now time.Time) {
	for key, b := range l.buckets {
		if b.tokens+float64(now.Sub(b.at))/float64(l.refill) >= l.burst {
			delete(l.buckets, key)
		}
	}
}
