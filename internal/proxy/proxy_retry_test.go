package proxy

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BrandonMager/CacheProxyfy/internal/metrics"
	"github.com/BrandonMager/CacheProxyfy/internal/ratelimit"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// zeroDelayRetry is a retryConfig with zero delays so retry tests run instantly.
var zeroDelayRetry = retryConfig{
	maxAttempts: 3,
	baseDelay:   0,
	maxDelay:    0,
}

// TestRetry_SucceedsFirstAttempt_NoRetriesRecorded verifies that a successful first
// attempt does not increment the retry counter and does not log a retry warning.
func TestRetry_SucceedsFirstAttempt_NoRetriesRecorded(t *testing.T) {
	cache := &mockCache{}
	database := &mockDB{}
	goroutineDone := make(chan struct{})
	database.onRecordEvent = func() { close(goroutineDone) }
	store := &mockStorage{}

	reg := prometheus.NewRegistry()
	m := metrics.New(reg, []string{})
	p := New(NewRouter([]string{"npm"}), store, discardLogger(), cache, database, &mockSecurityChecker{}, m, ratelimit.New(false, 0, 0, nil))
	p.retry = zeroDelayRetry

	var callCount atomic.Int32
	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		callCount.Add(1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("data")),
		}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := callCount.Load(); got != 1 {
		t.Errorf("upstream called %d times, want 1", got)
	}
	if got := testutil.ToFloat64(m.UpstreamRetriesTotal.WithLabelValues("npm")); got != 0 {
		t.Errorf("upstream_retries_total{npm} = %v, want 0", got)
	}

	select {
	case <-goroutineDone:
	case <-time.After(time.Second):
		t.Fatal("write-back goroutine did not complete")
	}
}

// TestRetry_NetworkError_RetrySucceeds verifies that transient network errors trigger
// retries and that the proxy recovers successfully when a later attempt succeeds.
func TestRetry_NetworkError_RetrySucceeds(t *testing.T) {
	cache := &mockCache{}
	database := &mockDB{}
	goroutineDone := make(chan struct{})
	database.onRecordEvent = func() { close(goroutineDone) }
	store := &mockStorage{}

	reg := prometheus.NewRegistry()
	m := metrics.New(reg, []string{})
	p := New(NewRouter([]string{"npm"}), store, discardLogger(), cache, database, &mockSecurityChecker{}, m, ratelimit.New(false, 0, 0, nil))
	p.retry = zeroDelayRetry

	var callCount atomic.Int32
	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		n := callCount.Add(1)
		if n < 3 {
			return nil, errors.New("connection refused")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("data")),
		}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := callCount.Load(); got != 3 {
		t.Errorf("upstream called %d times, want 3", got)
	}
	// Two retries fired (after attempt 1 and attempt 2).
	if got := testutil.ToFloat64(m.UpstreamRetriesTotal.WithLabelValues("npm")); got != 2 {
		t.Errorf("upstream_retries_total{npm} = %v, want 2", got)
	}

	select {
	case <-goroutineDone:
	case <-time.After(time.Second):
		t.Fatal("write-back goroutine did not complete")
	}
}

// TestRetry_503_RetrySucceeds verifies that a 503 Service Unavailable triggers a
// retry and the proxy serves the package successfully on the next attempt.
func TestRetry_503_RetrySucceeds(t *testing.T) {
	cache := &mockCache{}
	database := &mockDB{}
	goroutineDone := make(chan struct{})
	database.onRecordEvent = func() { close(goroutineDone) }
	store := &mockStorage{}

	reg := prometheus.NewRegistry()
	m := metrics.New(reg, []string{})
	p := New(NewRouter([]string{"npm"}), store, discardLogger(), cache, database, &mockSecurityChecker{}, m, ratelimit.New(false, 0, 0, nil))
	p.retry = zeroDelayRetry

	var callCount atomic.Int32
	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		n := callCount.Add(1)
		if n == 1 {
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("data")),
		}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := callCount.Load(); got != 2 {
		t.Errorf("upstream called %d times, want 2", got)
	}
	if got := testutil.ToFloat64(m.UpstreamRetriesTotal.WithLabelValues("npm")); got != 1 {
		t.Errorf("upstream_retries_total{npm} = %v, want 1", got)
	}

	select {
	case <-goroutineDone:
	case <-time.After(time.Second):
		t.Fatal("write-back goroutine did not complete")
	}
}

// TestRetry_429_RetrySucceeds verifies that a 429 Too Many Requests triggers a
// retry. When Retry-After is present it is respected (capped by maxDelay).
func TestRetry_429_RetrySucceeds(t *testing.T) {
	cache := &mockCache{}
	database := &mockDB{}
	goroutineDone := make(chan struct{})
	database.onRecordEvent = func() { close(goroutineDone) }
	store := &mockStorage{}

	reg := prometheus.NewRegistry()
	m := metrics.New(reg, []string{})
	p := New(NewRouter([]string{"npm"}), store, discardLogger(), cache, database, &mockSecurityChecker{}, m, ratelimit.New(false, 0, 0, nil))
	// Use a tiny maxDelay so Retry-After: 9999 is capped and tests stay fast.
	p.retry = retryConfig{maxAttempts: 3, baseDelay: 0, maxDelay: 0}

	var callCount atomic.Int32
	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		n := callCount.Add(1)
		if n == 1 {
			hdr := http.Header{}
			hdr.Set("Retry-After", "9999") // will be capped by maxDelay=0
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     hdr,
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("data")),
		}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := callCount.Load(); got != 2 {
		t.Errorf("upstream called %d times, want 2", got)
	}
	if got := testutil.ToFloat64(m.UpstreamRetriesTotal.WithLabelValues("npm")); got != 1 {
		t.Errorf("upstream_retries_total{npm} = %v, want 1", got)
	}

	select {
	case <-goroutineDone:
	case <-time.After(time.Second):
		t.Fatal("write-back goroutine did not complete")
	}
}

// TestRetry_ExhaustsAllAttempts_Returns502 verifies that when all retry attempts are
// exhausted the proxy returns 502 Bad Gateway and records two retry increments
// (one per retry fired, not per attempt).
func TestRetry_ExhaustsAllAttempts_Returns502(t *testing.T) {
	cache := &mockCache{}
	store := &mockStorage{}
	database := &mockDB{}

	reg := prometheus.NewRegistry()
	m := metrics.New(reg, []string{})
	p := New(NewRouter([]string{"npm"}), store, discardLogger(), cache, database, &mockSecurityChecker{}, m, ratelimit.New(false, 0, 0, nil))
	p.retry = zeroDelayRetry

	var callCount atomic.Int32
	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		callCount.Add(1)
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", w.Code)
	}
	if got := callCount.Load(); got != 3 {
		t.Errorf("upstream called %d times, want 3 (all attempts exhausted)", got)
	}
	// Two retries fired: after attempt 1 and after attempt 2.
	if got := testutil.ToFloat64(m.UpstreamRetriesTotal.WithLabelValues("npm")); got != 2 {
		t.Errorf("upstream_retries_total{npm} = %v, want 2", got)
	}
}

// TestRetry_404_NeverRetried verifies that a 404 Not Found is treated as a permanent
// failure and the upstream is called exactly once with no retry counter increment.
func TestRetry_404_NeverRetried(t *testing.T) {
	cache := &mockCache{}
	store := &mockStorage{}
	database := &mockDB{}

	reg := prometheus.NewRegistry()
	m := metrics.New(reg, []string{})
	p := New(NewRouter([]string{"npm"}), store, discardLogger(), cache, database, &mockSecurityChecker{}, m, ratelimit.New(false, 0, 0, nil))
	p.retry = zeroDelayRetry

	var callCount atomic.Int32
	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		callCount.Add(1)
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", w.Code)
	}
	if got := callCount.Load(); got != 1 {
		t.Errorf("upstream called %d times for a 404, want 1 (no retries)", got)
	}
	if got := testutil.ToFloat64(m.UpstreamRetriesTotal.WithLabelValues("npm")); got != 0 {
		t.Errorf("upstream_retries_total{npm} = %v, want 0 for a 404", got)
	}
}

// TestRetry_403_NeverRetried verifies that a 403 Forbidden is treated as a permanent
// failure and is not retried.
func TestRetry_403_NeverRetried(t *testing.T) {
	cache := &mockCache{}
	store := &mockStorage{}
	database := &mockDB{}

	reg := prometheus.NewRegistry()
	m := metrics.New(reg, []string{})
	p := New(NewRouter([]string{"npm"}), store, discardLogger(), cache, database, &mockSecurityChecker{}, m, ratelimit.New(false, 0, 0, nil))
	p.retry = zeroDelayRetry

	var callCount atomic.Int32
	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		callCount.Add(1)
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", w.Code)
	}
	if got := callCount.Load(); got != 1 {
		t.Errorf("upstream called %d times for a 403, want 1 (no retries)", got)
	}
	if got := testutil.ToFloat64(m.UpstreamRetriesTotal.WithLabelValues("npm")); got != 0 {
		t.Errorf("upstream_retries_total{npm} = %v, want 0 for a 403", got)
	}
}

// TestRetry_ContextCancelledDuringBackoff_StopsRetrying verifies that when the
// request context is cancelled while the proxy is waiting between retry attempts,
// the retry loop exits promptly and returns 502 (not hanging until backoff expires).
func TestRetry_ContextCancelledDuringBackoff_StopsRetrying(t *testing.T) {
	cache := &mockCache{}
	store := &mockStorage{}
	database := &mockDB{}

	reg := prometheus.NewRegistry()
	m := metrics.New(reg, []string{})
	p := New(NewRouter([]string{"npm"}), store, discardLogger(), cache, database, &mockSecurityChecker{}, m, ratelimit.New(false, 0, 0, nil))
	// Use a non-trivial delay so the context cancellation races against the timer —
	// the select in fetchFromUpstream must pick ctx.Done() first.
	p.retry = retryConfig{maxAttempts: 3, baseDelay: 500 * time.Millisecond, maxDelay: 2 * time.Second}

	ctx, cancel := context.WithCancel(context.Background())

	var callCount atomic.Int32
	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		n := callCount.Add(1)
		if n == 1 {
			// Cancel the context as soon as the first attempt fails, so the
			// backoff select picks ctx.Done() instead of time.After.
			cancel()
			return nil, errors.New("connection refused")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("data")),
		}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	start := time.Now()
	p.ServeHTTP(w, req)
	elapsed := time.Since(start)

	// Must return well before the 500ms backoff delay fires.
	if elapsed > 200*time.Millisecond {
		t.Errorf("ServeHTTP took %v — context cancellation did not short-circuit the backoff", elapsed)
	}
	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", w.Code)
	}
	// Only the first attempt should have been made before the context was cancelled.
	if got := callCount.Load(); got != 1 {
		t.Errorf("upstream called %d times, want 1", got)
	}
	// The retry metric was incremented once (the retry was triggered, then cancelled).
	if got := testutil.ToFloat64(m.UpstreamRetriesTotal.WithLabelValues("npm")); got != 1 {
		t.Errorf("upstream_retries_total{npm} = %v, want 1", got)
	}
}

// TestServeHTTP_SingleflightDeduplication_WithRetry verifies that when concurrent
// requests are coalesced by singleflight and the leader's first upstream attempt
// returns a transient error, the retry fires inside the singleflight callback and
// all waiting callers receive the eventual successful result — not an error.
//
// Without this guarantee, a 503 during a thundering-herd burst would return errors
// to every waiting caller even though a single retry would have recovered.
func TestServeHTTP_SingleflightDeduplication_WithRetry(t *testing.T) {
	const (
		testData    = "fake package bytes"
		concurrency = 5
	)

	cache := &mockCache{}
	database := &mockDB{}
	store := &mockStorage{}

	goroutineDone := make(chan struct{})
	database.onRecordEvent = func() { close(goroutineDone) }

	p, m := newProxy(t, cache, store, database)
	p.retry = zeroDelayRetry

	var upstreamCallCount atomic.Int32
	// release blocks the first upstream call until all goroutines are queued in sf.Do.
	release := make(chan struct{})

	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		n := upstreamCallCount.Add(1)
		if n == 1 {
			<-release // hold here until all goroutines are waiting in sf.Do
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}
		// Second call: the singleflight leader's retry — succeeds immediately.
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(testData)),
		}, nil
	})

	type result struct {
		status   int
		cacheHdr string
		body     string
	}
	results := make([]result, concurrency)

	var started, done sync.WaitGroup
	started.Add(concurrency)
	done.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func(i int) {
			started.Done()
			defer done.Done()
			req := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
			w := httptest.NewRecorder()
			p.ServeHTTP(w, req)
			resp := w.Result()
			body, _ := io.ReadAll(resp.Body)
			results[i] = result{
				status:   resp.StatusCode,
				cacheHdr: resp.Header.Get("X-Cache"),
				body:     string(body),
			}
		}(i)
	}

	// Wait for all goroutines to be running then give them time to reach sf.Do.
	started.Wait()
	time.Sleep(20 * time.Millisecond)

	// Unblock the first upstream call — it returns 503, triggering a retry inside
	// the singleflight callback while all 4 other goroutines wait for the leader.
	close(release)
	done.Wait()

	// Upstream was called exactly twice: the initial 503 and the successful retry.
	if got := upstreamCallCount.Load(); got != 2 {
		t.Errorf("upstream called %d times, want 2 (initial 503 + one retry)", got)
	}

	// All concurrent callers must receive the successful retry result — not the 503.
	for i, r := range results {
		if r.status != http.StatusOK {
			t.Errorf("goroutine %d: expected status 200, got %d", i, r.status)
		}
		if r.cacheHdr != "miss" {
			t.Errorf("goroutine %d: X-Cache: got %q, want %q", i, r.cacheHdr, "miss")
		}
		if r.body != testData {
			t.Errorf("goroutine %d: body: got %q, want %q", i, r.body, testData)
		}
	}

	// Fetch and retry counters reflect one logical upstream fetch (leader only).
	if got := testutil.ToFloat64(m.UpstreamFetchesTotal.WithLabelValues("npm", "ok")); got != 1 {
		t.Errorf("upstream_fetches_total{npm,ok} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.UpstreamRetriesTotal.WithLabelValues("npm")); got != 1 {
		t.Errorf("upstream_retries_total{npm} = %v, want 1", got)
	}

	select {
	case <-goroutineDone:
	case <-time.After(time.Second):
		t.Error("write-back goroutine did not complete within 1s")
	}
}

// TestIsRetryable covers the classification logic in isolation.
func TestIsRetryable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"network error", errors.New("connection refused"), true},
		{"429", &httpStatusError{code: http.StatusTooManyRequests}, true},
		{"500", &httpStatusError{code: http.StatusInternalServerError}, true},
		{"502", &httpStatusError{code: http.StatusBadGateway}, true},
		{"503", &httpStatusError{code: http.StatusServiceUnavailable}, true},
		{"504", &httpStatusError{code: http.StatusGatewayTimeout}, true},
		{"404", &httpStatusError{code: http.StatusNotFound}, false},
		{"403", &httpStatusError{code: http.StatusForbidden}, false},
		{"400", &httpStatusError{code: http.StatusBadRequest}, false},
		{"context cancelled", context.Canceled, false},
		{"context deadline exceeded", context.DeadlineExceeded, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRetryable(tc.err); got != tc.want {
				t.Errorf("isRetryable(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
