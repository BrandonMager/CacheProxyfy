package proxy

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

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
	getPackageCalled atomic.Bool
}

func (m *mockDB) GetPackage(_ context.Context, _, _, _ string) (db.Package, error) {
	m.getPackageCalled.Store(true)
	return db.Package{}, errors.New("db: not found")
}

func (m *mockDB) TouchPackage(_ context.Context, _, _, _ string) error        { return nil }
func (m *mockDB) UpsertPackage(_ context.Context, _ db.Package) (string, error) { return "", nil }
func (m *mockDB) RecordEvent(_ context.Context, _, _, _, _ string, _ int64) error { return nil }

type mockStorage struct {
	getFunc func(ctx context.Context, checksum string) (io.ReadCloser, error)
}

func (m *mockStorage) Get(ctx context.Context, checksum string) (io.ReadCloser, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, checksum)
	}
	return nil, errors.New("storage: not found")
}

func (m *mockStorage) Put(_ context.Context, _ string, _ io.Reader, _ int64) error { return nil }
func (m *mockStorage) Exists(_ context.Context, _ string) (bool, error)            { return false, nil }
func (m *mockStorage) Delete(_ context.Context, _ string) error                    { return nil }
func (m *mockStorage) Name() string                                                 { return "mock" }

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

	router := NewRouter([]string{"npm"})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	p := New(router, store, logger, cache, database)

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
