package proxy

import (
	"bytes"
	"context"
	"encoding/json"
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

	"github.com/BrandonMager/CacheProxyfy/internal/db"
	"github.com/BrandonMager/CacheProxyfy/internal/metrics"
	"github.com/BrandonMager/CacheProxyfy/internal/security"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// -- mock implementations --

type mockCache struct {
	getFunc  func(ctx context.Context, ecosystem, name, version string) (string, error)
	setFunc  func(ctx context.Context, ecosystem, name, version, checksum string) error
	pingFunc func(ctx context.Context) error
}

func (m *mockCache) Get(ctx context.Context, ecosystem, name, version string) (string, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, ecosystem, name, version)
	}
	return "", errors.New("cache: not found")
}

func (m *mockCache) Set(ctx context.Context, ecosystem, name, version, checksum string) error {
	if m.setFunc != nil {
		return m.setFunc(ctx, ecosystem, name, version, checksum)
	}
	return nil
}

func (m *mockCache) Ping(ctx context.Context) error {
	if m.pingFunc != nil {
		return m.pingFunc(ctx)
	}
	return nil
}

type mockDB struct {
	getPackageCalled    atomic.Bool
	getPackageFunc      func(ctx context.Context, ecosystem, name, version string) (db.Package, error)
	upsertPackageCalled atomic.Bool
	recordEventCalled   atomic.Bool
	onRecordEvent       func()                         // called when RecordEvent runs, used to signal goroutine completion
	onRecordEventArgs   func(event string, bytes int64) // called with the actual arguments for assertion
	recordCVEAlertCalled atomic.Bool
	onRecordCVEAlert     func()                                                          // called when RecordCVEAlert runs
	onRecordCVEAlertArgs func(ecosystem, name, version, cveID, severity, outcome string) // called with the actual arguments for assertion
}

func (m *mockDB) GetPackage(ctx context.Context, ecosystem, name, version string) (db.Package, error) {
	m.getPackageCalled.Store(true)
	if m.getPackageFunc != nil {
		return m.getPackageFunc(ctx, ecosystem, name, version)
	}
	return db.Package{}, errors.New("db: not found")
}

func (m *mockDB) TouchPackage(_ context.Context, _, _, _ string) error { return nil }

func (m *mockDB) UpsertPackage(_ context.Context, _ db.Package) (string, error) {
	m.upsertPackageCalled.Store(true)
	return "", nil
}

func (m *mockDB) RecordCVEAlert(_ context.Context, ecosystem, name, version, cveID, severity, outcome string) error {
	m.recordCVEAlertCalled.Store(true)
	if m.onRecordCVEAlertArgs != nil {
		m.onRecordCVEAlertArgs(ecosystem, name, version, cveID, severity, outcome)
	}
	if m.onRecordCVEAlert != nil {
		m.onRecordCVEAlert()
	}
	return nil
}

func (m *mockDB) RecordEvent(_ context.Context, _, _, _, event string, bytes int64) error {
	m.recordEventCalled.Store(true)
	if m.onRecordEventArgs != nil {
		m.onRecordEventArgs(event, bytes)
	}
	if m.onRecordEvent != nil {
		m.onRecordEvent()
	}
	return nil
}

type mockStorage struct {
	getFunc func(ctx context.Context, checksum string) (io.ReadCloser, error)
	putFunc func(ctx context.Context, checksum string, r io.Reader, size int64) error
	putCalled atomic.Bool
}

func (m *mockStorage) Get(ctx context.Context, checksum string) (io.ReadCloser, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, checksum)
	}
	return nil, errors.New("storage: not found")
}

func (m *mockStorage) Put(ctx context.Context, checksum string, r io.Reader, size int64) error {
	m.putCalled.Store(true)
	if m.putFunc != nil {
		return m.putFunc(ctx, checksum, r, size)
	}
	return nil
}

func (m *mockStorage) Exists(_ context.Context, _ string) (bool, error) { return false, nil }
func (m *mockStorage) Delete(_ context.Context, _ string) error          { return nil }
func (m *mockStorage) Name() string                                       { return "mock" }

// mockSecurityChecker always allows — security behavior is tested separately.
type mockSecurityChecker struct {
	checkCalled atomic.Bool
	checkFunc   func(ctx context.Context, ecosystem, name, version string) (security.Outcome, []security.CVERecord, error)
}

func (m *mockSecurityChecker) Check(ctx context.Context, ecosystem, name, version string) (security.Outcome, []security.CVERecord, error) {
	m.checkCalled.Store(true)
	if m.checkFunc != nil {
		return m.checkFunc(ctx, ecosystem, name, version)
	}
	return security.Allow, nil, nil
}

// roundTripFunc lets a test intercept outbound HTTP requests without a real server.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func newProxy(t *testing.T, cache CacheClient, store *mockStorage, database *mockDB) (*Proxy, *metrics.Metrics) {
	t.Helper()
	router := NewRouter([]string{"npm"})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := metrics.New(prometheus.NewRegistry(), []string{})
	return New(router, store, logger, cache, database, &mockSecurityChecker{}, m), m
}

// -- tests --

func TestServeHTTP_RedisHit(t *testing.T) {
	const (
		testChecksum = "abc123"
		testData     = "fake package bytes"
	)

	cache := &mockCache{
		getFunc: func(_ context.Context, _, _, _ string) (string, error) {
			return testChecksum, nil
		},
	}

	store := &mockStorage{
		getFunc: func(_ context.Context, checksum string) (io.ReadCloser, error) {
			if checksum != testChecksum {
				t.Errorf("storage.Get called with unexpected checksum %q, want %q", checksum, testChecksum)
			}
			return io.NopCloser(strings.NewReader(testData)), nil
		},
	}

	database := &mockDB{}
	p, _ := newProxy(t, cache, store, database)

	req := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, req)

	resp := w.Result()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	if got := resp.Header.Get("X-Cache"); got != "hit" {
		t.Errorf("X-Cache: got %q, want %q", got, "hit")
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != testData {
		t.Errorf("body: got %q, want %q", string(body), testData)
	}

	// The DB lookup path (GetPackage) should be bypassed entirely on a Redis hit.
	// The upstream fetch is also implicitly skipped — serve() returns early after
	// reading from storage.
	if database.getPackageCalled.Load() {
		t.Error("db.GetPackage should not be called on a Redis hit")
	}
}

func TestServeHTTP_DBHit_RedisMiss(t *testing.T) {
	const (
		testChecksum = "def456"
		testData     = "fake package bytes"
		testEco      = "npm"
		testName     = "lodash"
		testVersion  = "4.17.21"
	)

	// Redis miss
	cache := &mockCache{
		getFunc: func(_ context.Context, _, _, _ string) (string, error) {
			return "", errors.New("cache: not found")
		},
	}

	// Signal when cache.Set is called so the test can wait for the goroutine
	redisBackfilled := make(chan struct{}, 1)
	cache.setFunc = func(_ context.Context, ecosystem, name, version, checksum string) error {
		if ecosystem != testEco || name != testName || version != testVersion || checksum != testChecksum {
			t.Errorf("cache.Set called with unexpected args: eco=%q name=%q version=%q checksum=%q",
				ecosystem, name, version, checksum)
		}
		redisBackfilled <- struct{}{}
		return nil
	}

	store := &mockStorage{
		getFunc: func(_ context.Context, checksum string) (io.ReadCloser, error) {
			if checksum != testChecksum {
				t.Errorf("storage.Get called with unexpected checksum %q, want %q", checksum, testChecksum)
			}
			return io.NopCloser(strings.NewReader(testData)), nil
		},
	}

	database := &mockDB{}
	database.getPackageFunc = func(_ context.Context, _, _, _ string) (db.Package, error) {
		return db.Package{
			Ecosystem: testEco,
			Name:      testName,
			Version:   testVersion,
			Checksum:  testChecksum,
		}, nil
	}

	p, _ := newProxy(t, cache, store, database)

	req := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, req)

	resp := w.Result()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	if got := resp.Header.Get("X-Cache"); got != "hit" {
		t.Errorf("X-Cache: got %q, want %q", got, "hit")
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != testData {
		t.Errorf("body: got %q, want %q", string(body), testData)
	}

	if !database.getPackageCalled.Load() {
		t.Error("db.GetPackage should have been called on a Redis miss")
	}

	// Wait for the backfill goroutine to call cache.Set
	select {
	case <-redisBackfilled:
		// Redis was backfilled as expected
	case <-time.After(time.Second):
		t.Error("cache.Set was not called within 1s — Redis backfill goroutine may not have run")
	}
}

func TestServeHTTP_UpstreamFetch_RedisMiss_DBMiss(t *testing.T) {
	const testData = "fake package bytes"

	// Both Redis and DB miss
	cache := &mockCache{}
	database := &mockDB{}

	// Signal when the goroutine finishes — RecordEvent is the last call in it
	goroutineDone := make(chan struct{})
	database.onRecordEvent = func() { close(goroutineDone) }

	store := &mockStorage{}

	p, _ := newProxy(t, cache, store, database)

	// Intercept the outbound HTTP call — p.client is accessible from the same package
	upstreamCalled := false
	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		upstreamCalled = true
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(testData)),
		}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, req)

	resp := w.Result()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	if got := resp.Header.Get("X-Cache"); got != "miss" {
		t.Errorf("X-Cache: got %q, want %q", got, "miss")
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != testData {
		t.Errorf("body: got %q, want %q", string(body), testData)
	}

	if !upstreamCalled {
		t.Error("upstream was not called on a full cache miss")
	}

	// storage.Put is synchronous — no need to wait
	if !store.putCalled.Load() {
		t.Error("storage.Put was not called after upstream fetch")
	}

	// cache.Set, db.UpsertPackage, db.RecordEvent are in a goroutine — wait for it
	select {
	case <-goroutineDone:
	case <-time.After(time.Second):
		t.Fatal("goroutine did not complete within 1s")
	}

	if !database.upsertPackageCalled.Load() {
		t.Error("db.UpsertPackage was not called after upstream fetch")
	}

	if !database.recordEventCalled.Load() {
		t.Error("db.RecordEvent was not called after upstream fetch")
	}

	// cache.Set is verified indirectly — it runs before UpsertPackage in the same goroutine,
	// and we already confirmed the goroutine completed via RecordEvent
}

func TestServeHTTP_WriteBack_SecondRequestIsHit(t *testing.T) {
	const testData = "fake package bytes"

	// in-memory cache: empty until Set is called, then Get returns the stored checksum
	var cacheMu sync.Mutex
	cacheStore := make(map[string]string)
	setCalled := make(chan struct{}, 1)

	cache := &mockCache{
		getFunc: func(_ context.Context, eco, name, ver string) (string, error) {
			cacheMu.Lock()
			defer cacheMu.Unlock()
			if v, ok := cacheStore[eco+"/"+name+"/"+ver]; ok {
				return v, nil
			}
			return "", errors.New("cache: not found")
		},
		setFunc: func(_ context.Context, eco, name, ver, checksum string) error {
			cacheMu.Lock()
			cacheStore[eco+"/"+name+"/"+ver] = checksum
			cacheMu.Unlock()
			select {
			case setCalled <- struct{}{}:
			default:
			}
			return nil
		},
	}

	// in-memory storage: Put stores bytes, Get retrieves them by checksum
	var storageMu sync.Mutex
	storageData := make(map[string][]byte)

	store := &mockStorage{
		getFunc: func(_ context.Context, checksum string) (io.ReadCloser, error) {
			storageMu.Lock()
			defer storageMu.Unlock()
			if data, ok := storageData[checksum]; ok {
				return io.NopCloser(bytes.NewReader(data)), nil
			}
			return nil, errors.New("storage: not found")
		},
		putFunc: func(_ context.Context, checksum string, r io.Reader, _ int64) error {
			data, err := io.ReadAll(r)
			if err != nil {
				return err
			}
			storageMu.Lock()
			storageData[checksum] = data
			storageMu.Unlock()
			return nil
		},
	}

	database := &mockDB{}
	p, _ := newProxy(t, cache, store, database)

	var upstreamCallCount atomic.Int32
	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		upstreamCallCount.Add(1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(testData)),
		}, nil
	})

	// First request: full cache miss → upstream fetch
	req1 := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
	w1 := httptest.NewRecorder()
	p.ServeHTTP(w1, req1)

	resp1 := w1.Result()
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("first request: expected status 200, got %d", resp1.StatusCode)
	}
	if got := resp1.Header.Get("X-Cache"); got != "miss" {
		t.Errorf("first request: X-Cache: got %q, want %q", got, "miss")
	}

	// Wait for the write-back goroutine to populate the cache before the second request
	select {
	case <-setCalled:
	case <-time.After(time.Second):
		t.Fatal("cache.Set was not called within 1s — write-back goroutine may not have run")
	}

	// Second request: should be served from Redis cache (write-back already ran)
	req2 := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
	w2 := httptest.NewRecorder()
	p.ServeHTTP(w2, req2)

	resp2 := w2.Result()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("second request: expected status 200, got %d", resp2.StatusCode)
	}
	if got := resp2.Header.Get("X-Cache"); got != "hit" {
		t.Errorf("second request: X-Cache: got %q, want %q", got, "hit")
	}

	body2, _ := io.ReadAll(resp2.Body)
	if string(body2) != testData {
		t.Errorf("second request: body: got %q, want %q", string(body2), testData)
	}

	// Upstream must only be called once — the second request is served from cache
	if got := upstreamCallCount.Load(); got != 1 {
		t.Errorf("upstream called %d times, want exactly 1", got)
	}
}

func TestServeHTTP_SingleflightDeduplication(t *testing.T) {
	const (
		testData    = "fake package bytes"
		concurrency = 5
	)

	var upstreamCallCount atomic.Int32

	// release blocks the upstream response until all goroutines have queued in sf.Do
	release := make(chan struct{})

	cache := &mockCache{}
	database := &mockDB{}
	store := &mockStorage{}

	// RecordEvent is the last call in the write-back goroutine — use it to know when it's done
	goroutineDone := make(chan struct{})
	database.onRecordEvent = func() { close(goroutineDone) }

	p, m := newProxy(t, cache, store, database)
	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		upstreamCallCount.Add(1)
		<-release // block until all goroutines have queued up
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

	// started signals when each goroutine has launched; done signals when ServeHTTP returns
	var started, done sync.WaitGroup
	started.Add(concurrency)
	done.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func(i int) {
			started.Done() // signal: goroutine is running
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

	// Wait for all goroutines to be running, then give them time to reach sf.Do.
	// The mock cache and DB return instantly, so the path to sf.Do is pure in-memory.
	started.Wait()
	time.Sleep(20 * time.Millisecond)

	// Release the upstream — all goroutines should now be sitting in sf.Do
	close(release)
	done.Wait()

	if got := upstreamCallCount.Load(); got != 1 {
		t.Errorf("upstream called %d times, want exactly 1", got)
	}

	if got := testutil.ToFloat64(m.UpstreamFetchesTotal.WithLabelValues("npm", "ok")); got != 1 {
		t.Errorf("upstream_fetches_total{npm,ok} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.UpstreamFetchesTotal.WithLabelValues("npm", "error")); got != 0 {
		t.Errorf("upstream_fetches_total{npm,error} = %v, want 0", got)
	}

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

	// Verify the write-back goroutine ran exactly once
	select {
	case <-goroutineDone:
	case <-time.After(time.Second):
		t.Error("write-back goroutine did not complete within 1s")
	}
}

func TestRecordEvent_CacheHit(t *testing.T) {
	const (
		testChecksum = "abc123"
		testData     = "fake package bytes"
	)

	cache := &mockCache{
		getFunc: func(_ context.Context, _, _, _ string) (string, error) {
			return testChecksum, nil
		},
	}

	store := &mockStorage{
		getFunc: func(_ context.Context, _ string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(testData)), nil
		},
	}

	// goroutineDone is closed when RecordEvent is called, signalling the goroutine has finished
	goroutineDone := make(chan struct{})
	var gotEvent string
	var gotBytes int64
	database := &mockDB{
		onRecordEventArgs: func(event string, bytes int64) {
			gotEvent = event
			gotBytes = bytes
		},
		onRecordEvent: func() { close(goroutineDone) },
	}

	p, _ := newProxy(t, cache, store, database)

	req := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, req)

	if got := w.Result().Header.Get("X-Cache"); got != "hit" {
		t.Fatalf("X-Cache: got %q, want %q", got, "hit")
	}

	select {
	case <-goroutineDone:
	case <-time.After(time.Second):
		t.Fatal("RecordEvent was not called within 1s")
	}

	if gotEvent != "hit" {
		t.Errorf("RecordEvent event: got %q, want %q", gotEvent, "hit")
	}
	if gotBytes != int64(len(testData)) {
		t.Errorf("RecordEvent bytes: got %d, want %d", gotBytes, int64(len(testData)))
	}
}

func TestRecordEvent_UpstreamMiss(t *testing.T) {
	const testData = "fake package bytes"

	// Both Redis and DB miss
	cache := &mockCache{}

	goroutineDone := make(chan struct{})
	var gotEvent string
	var gotBytes int64
	database := &mockDB{
		onRecordEventArgs: func(event string, bytes int64) {
			gotEvent = event
			gotBytes = bytes
		},
		onRecordEvent: func() { close(goroutineDone) },
	}

	store := &mockStorage{}

	p, _ := newProxy(t, cache, store, database)
	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(testData)),
		}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, req)

	if got := w.Result().Header.Get("X-Cache"); got != "miss" {
		t.Fatalf("X-Cache: got %q, want %q", got, "miss")
	}

	select {
	case <-goroutineDone:
	case <-time.After(time.Second):
		t.Fatal("RecordEvent was not called within 1s")
	}

	if gotEvent != "miss" {
		t.Errorf("RecordEvent event: got %q, want %q", gotEvent, "miss")
	}
	if gotBytes != int64(len(testData)) {
		t.Errorf("RecordEvent bytes: got %d, want %d", gotBytes, int64(len(testData)))
	}
}

func TestRecordEvent_DBError_WarnsAndResponseUnaffected(t *testing.T) {
	const (
		testChecksum = "abc123"
		testData     = "fake package bytes"
	)

	cache := &mockCache{
		getFunc: func(_ context.Context, _, _, _ string) (string, error) {
			return testChecksum, nil
		},
	}

	store := &mockStorage{
		getFunc: func(_ context.Context, _ string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(testData)), nil
		},
	}

	database := &mockDB{}

	// warnLogged is closed by the slog handler after it writes the warning, guaranteeing
	// the buffer is populated before the test reads it.
	warnLogged := make(chan struct{})
	var logBuf bytes.Buffer
	baseHandler := slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn})
	logger := slog.New(&warnSignalHandler{Handler: baseHandler, signal: func() { close(warnLogged) }})

	router := NewRouter([]string{"npm"})
	p := New(router, store, logger, cache, &errRecordEventDB{mockDB: database}, &mockSecurityChecker{}, metrics.New(prometheus.NewRegistry(), []string{}))

	req := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Cache"); got != "hit" {
		t.Errorf("X-Cache: got %q, want %q", got, "hit")
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != testData {
		t.Errorf("body: got %q, want %q", string(body), testData)
	}

	// Wait until the warning has been written — the handler closes the channel after Handle returns
	select {
	case <-warnLogged:
	case <-time.After(time.Second):
		t.Fatal("warning was not logged within 1s")
	}

	if !strings.Contains(logBuf.String(), "record event failed") {
		t.Errorf("expected warning %q in log output, got: %s", "record event failed", logBuf.String())
	}
}

// warnSignalHandler wraps a slog.Handler and calls signal once after it handles any Warn record.
type warnSignalHandler struct {
	slog.Handler
	signal func()
	once   sync.Once
}

func (h *warnSignalHandler) Handle(ctx context.Context, r slog.Record) error {
	err := h.Handler.Handle(ctx, r)
	if r.Level == slog.LevelWarn {
		h.once.Do(h.signal)
	}
	return err
}

// errRecordEventDB wraps mockDB and makes RecordEvent always return an error.
type errRecordEventDB struct {
	*mockDB
}

func (e *errRecordEventDB) RecordEvent(_ context.Context, _, _, _, _ string, _ int64) error {
	e.mockDB.recordEventCalled.Store(true)
	return errors.New("db: record event failed")
}

func TestHandleHealth(t *testing.T) {
	tests := []struct {
		name        string
		pingErr     error
		wantRedis   bool
	}{
		{
			name:      "redis reachable",
			pingErr:   nil,
			wantRedis: true,
		},
		{
			name:      "redis unreachable",
			pingErr:   errors.New("connection refused"),
			wantRedis: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cache := &mockCache{
				pingFunc: func(_ context.Context) error { return tc.pingErr },
			}
			store := &mockStorage{}
			database := &mockDB{}
			p, _ := newProxy(t, cache, store, database)

			req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
			w := httptest.NewRecorder()
			p.ServeHTTP(w, req)

			resp := w.Result()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected status 200, got %d", resp.StatusCode)
			}

			if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
			}

			var body map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("response is not valid JSON: %v", err)
			}

			if got := body["status"]; got != "ok" {
				t.Errorf("status: got %v, want %q", got, "ok")
			}

			// storage name comes from mockStorage.Name() == "mock"
			if got := body["storage"]; got != "mock" {
				t.Errorf("storage: got %v, want %q", got, "mock")
			}

			// newProxy registers only "npm"
			if got := body["ecosystems"]; got != "npm" {
				t.Errorf("ecosystems: got %v, want %q", got, "npm")
			}

			if got := body["redis"]; got != tc.wantRedis {
				t.Errorf("redis: got %v, want %v", got, tc.wantRedis)
			}
		})
	}
}

func TestServeHTTP_CVEScanningDisabled_PackagePassesThrough(t *testing.T) {
	const testData = "fake package bytes"

	// Security checker simulates cve_scanning: false — returns Allow with no records.
	checker := &mockSecurityChecker{}

	cache := &mockCache{}
	database := &mockDB{}
	goroutineDone := make(chan struct{})
	database.onRecordEvent = func() { close(goroutineDone) }

	store := &mockStorage{}

	router := NewRouter([]string{"npm"})

	// Capture warn-level logs to assert no security warning is emitted.
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	p := New(router, store, logger, cache, database, checker, metrics.New(prometheus.NewRegistry(), []string{}))
	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(testData)),
		}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, req)

	resp := w.Result()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	if got := resp.Header.Get("X-Cache"); got != "miss" {
		t.Errorf("X-Cache: got %q, want %q", got, "miss")
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != testData {
		t.Errorf("body: got %q, want %q", string(body), testData)
	}

	if !checker.checkCalled.Load() {
		t.Error("security.Check was not called")
	}

	// Wait for the write-back goroutine to finish before reading the log buffer.
	select {
	case <-goroutineDone:
	case <-time.After(time.Second):
		t.Fatal("write-back goroutine did not complete within 1s")
	}

	if log := logBuf.String(); strings.Contains(log, "vulnerabilities") || strings.Contains(log, "blocked") {
		t.Errorf("unexpected security warning in log output: %s", log)
	}
}

func TestServeHTTP_CVEScanningEnabled_WarnPolicy_PackagePassesThrough(t *testing.T) {
	const testData = "fake package bytes"

	// Security checker simulates cve_scanning: true with policy: warn —
	// returns Warn with one CVE record, as the OSV scanner would for a vulnerable package.
	cveRecords := []security.CVERecord{
		{ID: "GHSA-jf85-cpcp-j695", Summary: "Prototype pollution in lodash", Severity: security.SeverityHigh},
	}
	checker := &mockSecurityChecker{
		checkFunc: func(_ context.Context, _, _, _ string) (security.Outcome, []security.CVERecord, error) {
			return security.Warn, cveRecords, nil
		},
	}

	cache := &mockCache{}
	database := &mockDB{}
	goroutineDone := make(chan struct{})
	database.onRecordEvent = func() { close(goroutineDone) }

	store := &mockStorage{}

	router := NewRouter([]string{"npm"})

	warnLogged := make(chan struct{})
	var logBuf bytes.Buffer
	baseHandler := slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn})
	logger := slog.New(&warnSignalHandler{Handler: baseHandler, signal: func() { close(warnLogged) }})

	p := New(router, store, logger, cache, database, checker, metrics.New(prometheus.NewRegistry(), []string{}))
	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(testData)),
		}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, req)

	resp := w.Result()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	if got := resp.Header.Get("X-Cache"); got != "miss" {
		t.Errorf("X-Cache: got %q, want %q", got, "miss")
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != testData {
		t.Errorf("body: got %q, want %q", string(body), testData)
	}

	// Wait for the security warning to be logged before inspecting the buffer.
	select {
	case <-warnLogged:
	case <-time.After(time.Second):
		t.Fatal("security warning was not logged within 1s")
	}

	if log := logBuf.String(); !strings.Contains(log, "known vulnerabilities") {
		t.Errorf("expected security warning in log output, got: %s", log)
	}

	// Wait for the write-back goroutine to finish.
	select {
	case <-goroutineDone:
	case <-time.After(time.Second):
		t.Fatal("write-back goroutine did not complete within 1s")
	}
}

func TestServeHTTP_CVEScanningEnabled_BlockPolicy_RequestRejected(t *testing.T) {
	// Security checker simulates cve_scanning: true with a block-level policy —
	// returns Block with one critical CVE record.
	cveRecords := []security.CVERecord{
		{ID: "GHSA-jf85-cpcp-j695", Summary: "Prototype pollution in lodash", Severity: security.SeverityCritical},
	}
	checker := &mockSecurityChecker{
		checkFunc: func(_ context.Context, _, _, _ string) (security.Outcome, []security.CVERecord, error) {
			return security.Block, cveRecords, nil
		},
	}

	cache := &mockCache{}
	database := &mockDB{}

	// recordCVEAlerts runs in a goroutine — wait for it before the test exits.
	cveAlertDone := make(chan struct{})
	database.onRecordEvent = func() { close(cveAlertDone) }

	store := &mockStorage{}

	router := NewRouter([]string{"npm"})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	upstreamCalled := false
	p := New(router, store, logger, cache, database, checker, metrics.New(prometheus.NewRegistry(), []string{}))
	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		upstreamCalled = true
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("should not reach client")),
		}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, req)

	resp := w.Result()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("expected status 502, got %d", resp.StatusCode)
	}

	if upstreamCalled {
		t.Error("upstream should not be called when package is blocked")
	}

	if !checker.checkCalled.Load() {
		t.Error("security.Check was not called")
	}
}

func TestRecordCVEAlert_InsertedAfterVulnerableScan(t *testing.T) {
	const (
		testEco     = "npm"
		testName    = "lodash"
		testVersion = "4.17.21"
		testCVEID   = "GHSA-jf85-cpcp-j695"
	)

	cveRecords := []security.CVERecord{
		{ID: testCVEID, Summary: "Prototype pollution in lodash", Severity: security.SeverityHigh},
	}
	checker := &mockSecurityChecker{
		checkFunc: func(_ context.Context, _, _, _ string) (security.Outcome, []security.CVERecord, error) {
			return security.Warn, cveRecords, nil
		},
	}

	// Signal when RecordCVEAlert is called and capture its arguments for assertion.
	cveAlertDone := make(chan struct{})
	var gotEco, gotName, gotVersion, gotCVEID, gotSeverity, gotOutcome string
	database := &mockDB{
		onRecordCVEAlertArgs: func(ecosystem, name, version, cveID, severity, outcome string) {
			gotEco, gotName, gotVersion, gotCVEID, gotSeverity, gotOutcome = ecosystem, name, version, cveID, severity, outcome
		},
		onRecordCVEAlert: func() { close(cveAlertDone) },
	}

	// Also wait for the write-back goroutine (RecordEvent is its last call).
	writeDone := make(chan struct{})
	database.onRecordEvent = func() { close(writeDone) }

	cache := &mockCache{}
	store := &mockStorage{}
	router := NewRouter([]string{"npm"})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	p := New(router, store, logger, cache, database, checker, metrics.New(prometheus.NewRegistry(), []string{}))
	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("fake package bytes")),
		}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, req)

	// Wait for RecordCVEAlert goroutine to complete.
	select {
	case <-cveAlertDone:
	case <-time.After(time.Second):
		t.Fatal("RecordCVEAlert was not called within 1s")
	}

	if !database.recordCVEAlertCalled.Load() {
		t.Error("db.RecordCVEAlert was not called")
	}
	if gotEco != testEco {
		t.Errorf("ecosystem: got %q, want %q", gotEco, testEco)
	}
	if gotName != testName {
		t.Errorf("name: got %q, want %q", gotName, testName)
	}
	if gotVersion != testVersion {
		t.Errorf("version: got %q, want %q", gotVersion, testVersion)
	}
	if gotCVEID != testCVEID {
		t.Errorf("cveID: got %q, want %q", gotCVEID, testCVEID)
	}
	if gotSeverity != "HIGH" {
		t.Errorf("severity: got %q, want %q", gotSeverity, "HIGH")
	}
	if gotOutcome != "warn" {
		t.Errorf("outcome: got %q, want %q", gotOutcome, "warn")
	}

	// Wait for write-back goroutine to finish cleanly.
	select {
	case <-writeDone:
	case <-time.After(time.Second):
		t.Fatal("write-back goroutine did not complete within 1s")
	}
}

func TestServeHTTP_OSVRequestFailed_WarnsAndPackageServed(t *testing.T) {
	const testData = "fake package bytes"

	// Security checker simulates a failed OSV request — returns Allow + error (fail-open).
	checker := &mockSecurityChecker{
		checkFunc: func(_ context.Context, _, _, _ string) (security.Outcome, []security.CVERecord, error) {
			return security.Allow, nil, errors.New("security: osv request: connection refused")
		},
	}

	cache := &mockCache{}
	database := &mockDB{}
	writeDone := make(chan struct{})
	database.onRecordEvent = func() { close(writeDone) }

	store := &mockStorage{}
	router := NewRouter([]string{"npm"})

	warnLogged := make(chan struct{})
	var logBuf bytes.Buffer
	baseHandler := slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn})
	logger := slog.New(&warnSignalHandler{Handler: baseHandler, signal: func() { close(warnLogged) }})

	p := New(router, store, logger, cache, database, checker, metrics.New(prometheus.NewRegistry(), []string{}))
	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(testData)),
		}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, req)

	resp := w.Result()

	// Package is still served despite the OSV failure (fail-open).
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	if got := resp.Header.Get("X-Cache"); got != "miss" {
		t.Errorf("X-Cache: got %q, want %q", got, "miss")
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != testData {
		t.Errorf("body: got %q, want %q", string(body), testData)
	}

	// Wait for the warning to be written before inspecting the log buffer.
	select {
	case <-warnLogged:
	case <-time.After(time.Second):
		t.Fatal("security warning was not logged within 1s")
	}

	if log := logBuf.String(); !strings.Contains(log, "security check failed") {
		t.Errorf("expected %q in log output, got: %s", "security check failed", log)
	}

	// Wait for the write-back goroutine to finish cleanly.
	select {
	case <-writeDone:
	case <-time.After(time.Second):
		t.Fatal("write-back goroutine did not complete within 1s")
	}
}
