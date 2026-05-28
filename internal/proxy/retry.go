package proxy

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"time"
)

// httpStatusError is returned by doFetch when the upstream registry responds with a
// non-2xx HTTP status. Carrying the code lets isRetryable distinguish transient
// failures (5xx, 429) from permanent ones (404, 403) without string matching.
type httpStatusError struct {
	code       int
	url        string
	retryAfter time.Duration // parsed from Retry-After header; zero if absent
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("upstream returned %d for %s", e.code, e.url)
}

// retryConfig controls exponential-backoff behaviour for upstream artifact fetches.
type retryConfig struct {
	maxAttempts int           // total attempts including the initial one (minimum 1)
	baseDelay   time.Duration // delay before the first retry
	maxDelay    time.Duration // upper bound on any single delay
}

var defaultRetryConfig = retryConfig{
	maxAttempts: 3,
	baseDelay:   100 * time.Millisecond,
	maxDelay:    2 * time.Second,
}

// isRetryable reports whether err represents a transient upstream failure that is
// safe to retry. Context cancellation and permanent HTTP errors (404, 403, 400,
// etc.) return false immediately.
func isRetryable(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var httpErr *httpStatusError
	if errors.As(err, &httpErr) {
		switch httpErr.code {
		case http.StatusTooManyRequests,    // 429
			http.StatusInternalServerError, // 500
			http.StatusBadGateway,          // 502
			http.StatusServiceUnavailable,  // 503
			http.StatusGatewayTimeout:      // 504
			return true
		default:
			return false
		}
	}
	// Network-level errors (connection reset, DNS failure, etc.) are retryable.
	return true
}

// retryDelay returns the backoff duration before retry number attempt (0-indexed).
// It applies exponential growth with ±25% jitter, capped at maxDelay.
// If the failing request included a Retry-After hint, that value is used instead.
func (p *Proxy) retryDelay(attempt int, err error) time.Duration {
	var httpErr *httpStatusError
	if errors.As(err, &httpErr) && httpErr.retryAfter > 0 {
		if httpErr.retryAfter > p.retry.maxDelay {
			return p.retry.maxDelay
		}
		return httpErr.retryAfter
	}
	delay := p.retry.baseDelay * (1 << uint(attempt))
	if delay > p.retry.maxDelay {
		delay = p.retry.maxDelay
	}
	// ±25% jitter: multiply by a factor in [0.75, 1.25)
	factor := 0.75 + rand.Float64()*0.5
	return time.Duration(float64(delay) * factor)
}
