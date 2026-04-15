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

	"github.com/BrandonMager/CacheProxyfy/internal/db"
)

// -- mock implementations --

type mockCache struct {
	getFunc func(ctx context.Context, ecosystem, name, version string) (string, error)
	setFunc func(ctx context.Context, ecosystem, name, version, checksum string) error
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

func (m *mockCache) Ping(_ context.Context) error { return nil }

type mockDB struct {
	getPackageCalled    atomic.Bool
	getPackageFunc      func(ctx context.Context, ecosystem, name, version string) (db.Package, error)
	upsertPackageCalled atomic.Bool
	recordEventCalled   atomic.Bool
	onRecordEvent       func() // called when RecordEvent runs, used to signal goroutine completion
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

func (m *mockDB) RecordEvent(_ context.Context, _, _, _, _ string, _ int64) error {
	m.recordEventCalled.Store(true)
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

// roundTripFunc lets a test intercept outbound HTTP requests without a real server.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func newProxy(t *testing.T, cache CacheClient, store *mockStorage, database *mockDB) *Proxy {
	t.Helper()
	router := NewRouter([]string{"npm"})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(router, store, logger, cache, database)
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
	p := newProxy(t, cache, store, database)

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

	p := newProxy(t, cache, store, database)

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

	p := newProxy(t, cache, store, database)

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

	p := newProxy(t, cache, store, database)
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
