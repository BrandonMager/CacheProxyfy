package ratelimit

import (
	"context"
	"sync"

	"golang.org/x/time/rate"
)

// Limiter throttles upstream fetch rates per ecosystem using independent token buckets.
// When disabled, Wait is a no-op and all requests pass through immediately.
type Limiter struct {
	enabled bool
	rps     float64
	burst   int
	mu      sync.Mutex
	buckets map[string]*rate.Limiter
}

// New returns a Limiter. When enabled is false the limiter is inert.
// rps is the sustained token refill rate (requests per second) per ecosystem.
// burst is the maximum number of tokens that can accumulate.
func New(enabled bool, rps float64, burst int) *Limiter {
	return &Limiter{
		enabled: enabled,
		rps:     rps,
		burst:   burst,
		buckets: make(map[string]*rate.Limiter),
	}
}

// Wait blocks until a token is available for the given ecosystem or ctx is cancelled.
// Returns nil immediately when the limiter is disabled.
func (l *Limiter) Wait(ctx context.Context, ecosystem string) error {
	if !l.enabled {
		return nil
	}
	return l.bucket(ecosystem).Wait(ctx)
}

func (l *Limiter) bucket(ecosystem string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	if b, ok := l.buckets[ecosystem]; ok {
		return b
	}
	b := rate.NewLimiter(rate.Limit(l.rps), l.burst)
	l.buckets[ecosystem] = b
	return b
}
