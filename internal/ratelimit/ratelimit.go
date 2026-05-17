package ratelimit

import (
	"context"
	"sync"

	"golang.org/x/time/rate"
)

// EcosystemLimit holds per-ecosystem rate limit parameters that override the global defaults.
type EcosystemLimit struct {
	RPS   float64
	Burst int
}

// Limiter throttles upstream fetch rates per ecosystem using independent token buckets.
// When disabled, Wait is a no-op and all requests pass through immediately.
type Limiter struct {
	enabled   bool
	rps       float64
	burst     int
	overrides map[string]EcosystemLimit
	mu        sync.Mutex
	buckets   map[string]*rate.Limiter
}

// New returns a Limiter. When enabled is false the limiter is inert.
// rps and burst are the global defaults applied to any ecosystem without an override.
// overrides maps ecosystem names to per-ecosystem limits; a nil map means no overrides.
func New(enabled bool, rps float64, burst int, overrides map[string]EcosystemLimit) *Limiter {
	return &Limiter{
		enabled:   enabled,
		rps:       rps,
		burst:     burst,
		overrides: overrides,
		buckets:   make(map[string]*rate.Limiter),
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
	rps, burst := l.rps, l.burst
	if ov, ok := l.overrides[ecosystem]; ok {
		rps, burst = ov.RPS, ov.Burst
	}
	b := rate.NewLimiter(rate.Limit(rps), burst)
	l.buckets[ecosystem] = b
	return b
}
