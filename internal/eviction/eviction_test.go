package eviction

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/BrandonMager/CacheProxyfy/internal/db"
)

// -- mock implementations --

type mockDB struct {
	listExpiredFunc func(ctx context.Context, olderThan time.Time) ([]db.Package, error)
	deleteFunc      func(ctx context.Context, id int64) error
}

func (m *mockDB) ListExpiredPackages(ctx context.Context, olderThan time.Time) ([]db.Package, error) {
	if m.listExpiredFunc != nil {
		return m.listExpiredFunc(ctx, olderThan)
	}
	return nil, nil
}

func (m *mockDB) DeletePackage(ctx context.Context, id int64) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id)
	}
	return nil
}

type mockCache struct {
	deleteFunc func(ctx context.Context, ecosystem, name, version string) error
}

func (m *mockCache) Delete(ctx context.Context, ecosystem, name, version string) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, ecosystem, name, version)
	}
	return nil
}

type mockStorage struct {
	deleteFunc func(ctx context.Context, checksum string) error
}

func (m *mockStorage) Delete(ctx context.Context, checksum string) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, checksum)
	}
	return nil
}

func (m *mockStorage) Get(_ context.Context, _ string) (io.ReadCloser, error) { return nil, nil }
func (m *mockStorage) Put(_ context.Context, _ string, _ io.Reader, _ int64) error {
	return nil
}
func (m *mockStorage) Exists(_ context.Context, _ string) (bool, error) { return false, nil }
func (m *mockStorage) Name() string                                      { return "mock" }

// logBuffer captures slog output for assertions.
func logBuffer() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	return logger, &buf
}

// callOrder records the sequence of delete operations across all layers.
type callOrder struct {
	calls []string
}

func (c *callOrder) record(label string) {
	c.calls = append(c.calls, label)
}

// -- tests --

func TestEvict_NothingExpired(t *testing.T) {
	logger, buf := logBuffer()

	w := New(
		&mockDB{
			listExpiredFunc: func(_ context.Context, _ time.Time) ([]db.Package, error) {
				return nil, nil
			},
		},
		&mockCache{},
		&mockStorage{},
		720*time.Hour,
		time.Hour,
		logger,
	)

	if err := w.evict(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "nothing to evict") {
		t.Errorf("expected debug log 'nothing to evict', got: %s", buf.String())
	}
}

func TestEvict_EvictsAllPackages(t *testing.T) {
	logger, buf := logBuffer()

	expired := []db.Package{
		{ID: 1, Ecosystem: "npm", Name: "lodash", Version: "4.17.21", Checksum: "aaa", SizeBytes: 1000},
		{ID: 2, Ecosystem: "maven", Name: "com.google.guava:guava", Version: "33.0.0-jre", Checksum: "bbb", SizeBytes: 2000},
		{ID: 3, Ecosystem: "pypi", Name: "requests", Version: "2.31.0", Checksum: "ccc", SizeBytes: 500},
	}

	var order callOrder

	w := New(
		&mockDB{
			listExpiredFunc: func(_ context.Context, _ time.Time) ([]db.Package, error) {
				return expired, nil
			},
			deleteFunc: func(_ context.Context, id int64) error {
				order.record("db:" + string(rune('0'+id)))
				return nil
			},
		},
		&mockCache{
			deleteFunc: func(_ context.Context, ecosystem, name, version string) error {
				order.record("redis:" + name)
				return nil
			},
		},
		&mockStorage{
			deleteFunc: func(_ context.Context, checksum string) error {
				order.record("storage:" + checksum)
				return nil
			},
		},
		720*time.Hour,
		time.Hour,
		logger,
	)

	if err := w.evict(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify storage → redis → db order for each package.
	wantOrder := []string{
		"storage:aaa", "redis:lodash", "db:1",
		"storage:bbb", "redis:com.google.guava:guava", "db:2",
		"storage:ccc", "redis:requests", "db:3",
	}
	if len(order.calls) != len(wantOrder) {
		t.Fatalf("expected %d delete calls, got %d: %v", len(wantOrder), len(order.calls), order.calls)
	}
	for i, want := range wantOrder {
		if order.calls[i] != want {
			t.Errorf("call[%d]: want %q, got %q", i, want, order.calls[i])
		}
	}

	// Verify summary log contains correct counts and total bytes_freed (3500).
	log := buf.String()
	for _, want := range []string{"attempted=3", "evicted=3", "failed=0", "bytes_freed=3500"} {
		if !strings.Contains(log, want) {
			t.Errorf("expected log to contain %q, got:\n%s", want, log)
		}
	}
}

func TestEvictOne_StorageFailure_SkipsRedisAndDB(t *testing.T) {
	logger, _ := logBuffer()
	storageErr := errors.New("storage: disk full")

	var redisCalled, dbCalled bool

	pkg := db.Package{ID: 1, Ecosystem: "npm", Name: "lodash", Version: "4.17.21", Checksum: "aaa", SizeBytes: 1000}

	w := New(
		&mockDB{
			deleteFunc: func(_ context.Context, _ int64) error {
				dbCalled = true
				return nil
			},
		},
		&mockCache{
			deleteFunc: func(_ context.Context, _, _, _ string) error {
				redisCalled = true
				return nil
			},
		},
		&mockStorage{
			deleteFunc: func(_ context.Context, _ string) error {
				return storageErr
			},
		},
		720*time.Hour,
		time.Hour,
		logger,
	)

	err := w.evictOne(context.Background(), pkg)
	if !errors.Is(err, storageErr) {
		t.Fatalf("expected storage error, got: %v", err)
	}
	if redisCalled {
		t.Error("Redis Delete should not have been called after storage failure")
	}
	if dbCalled {
		t.Error("Postgres Delete should not have been called after storage failure")
	}
}

func TestEvictOne_RedisFailure_StillDeletesFromDB(t *testing.T) {
	logger, _ := logBuffer()
	redisErr := errors.New("redis: connection refused")

	var dbCalled bool

	pkg := db.Package{ID: 1, Ecosystem: "npm", Name: "lodash", Version: "4.17.21", Checksum: "aaa", SizeBytes: 1000}

	w := New(
		&mockDB{
			deleteFunc: func(_ context.Context, _ int64) error {
				dbCalled = true
				return nil
			},
		},
		&mockCache{
			deleteFunc: func(_ context.Context, _, _, _ string) error {
				return redisErr
			},
		},
		&mockStorage{},
		720*time.Hour,
		time.Hour,
		logger,
	)

	if err := w.evictOne(context.Background(), pkg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !dbCalled {
		t.Error("Postgres Delete should have been called despite Redis failure")
	}
}

func TestRun_ZeroTTL_WarnsAndNeverTicks(t *testing.T) {
	logger, buf := logBuffer()

	var listCalled bool

	w := New(
		&mockDB{
			listExpiredFunc: func(_ context.Context, _ time.Time) ([]db.Package, error) {
				listCalled = true
				return nil, nil
			},
		},
		&mockCache{},
		&mockStorage{},
		0, // ttl <= 0
		time.Hour,
		logger,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	w.Run(ctx) // blocks until ctx expires

	if !strings.Contains(buf.String(), "eviction disabled") {
		t.Errorf("expected warning log 'eviction disabled', got:\n%s", buf.String())
	}
	if listCalled {
		t.Error("ListExpiredPackages should not have been called when ttl <= 0")
	}
}

func TestRun_ZeroInterval_DefaultsTo1h(t *testing.T) {
	logger, buf := logBuffer()

	var callCount int

	w := New(
		&mockDB{
			listExpiredFunc: func(_ context.Context, _ time.Time) ([]db.Package, error) {
				callCount++
				return nil, nil
			},
		},
		&mockCache{},
		&mockStorage{},
		720*time.Hour,
		0, // interval <= 0
		logger,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	w.Run(ctx) // blocks until ctx expires; a 1h ticker will not fire in 50ms

	if !strings.Contains(buf.String(), "defaulting to 1h") {
		t.Errorf("expected warning log 'defaulting to 1h', got:\n%s", buf.String())
	}
	// The startup cycle fires once immediately; the 1h ticker must not fire a
	// second call within 50ms.
	if callCount != 1 {
		t.Errorf("expected exactly 1 call (startup cycle), got %d", callCount)
	}
}

func TestEvict_DBError(t *testing.T) {
	logger, _ := logBuffer()
	dbErr := errors.New("connection refused")

	w := New(
		&mockDB{
			listExpiredFunc: func(_ context.Context, _ time.Time) ([]db.Package, error) {
				return nil, dbErr
			},
		},
		&mockCache{},
		&mockStorage{},
		720*time.Hour,
		time.Hour,
		logger,
	)

	if err := w.evict(context.Background()); !errors.Is(err, dbErr) {
		t.Errorf("expected db error, got: %v", err)
	}
}
