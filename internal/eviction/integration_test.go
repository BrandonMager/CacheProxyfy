package eviction

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BrandonMager/CacheProxyfy/internal/db"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestIntegration_Eviction_SkipsBlockedPackages verifies that the eviction job
// never attempts to delete a blocked package from storage, Redis, or the DB.
//
// The protection lives in ListExpiredPackages (WHERE status = 'cached'), so this
// test exercises the full pipeline — real SQL query + real eviction loop — rather
// than a mock that could drift from the actual query.
//
// Setup:
//   - One cached package (checksum "abc", size 1 KB) — must be evicted.
//   - One blocked package (checksum "", size 0)      — must be left alone.
//
// Both rows are inserted fresh; the worker uses a negative TTL so the cutoff
// lands one hour in the future, making all 'cached' rows appear expired.
func TestIntegration_Eviction_SkipsBlockedPackages(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("user"),
		postgres.WithPassword("pass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Errorf("terminate postgres: %v", err)
		}
	}()

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("container connection string: %v", err)
	}

	database, err := db.Open(evictionDBConfig(t, connStr))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	defer database.Close()

	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("db migrate: %v", err)
	}

	const cachedChecksum = "sha256:deadbeef"

	// Insert the cached package.
	if _, err := database.UpsertPackage(ctx, db.Package{
		Ecosystem: "npm",
		Name:      "lodash",
		Version:   "4.17.21",
		Checksum:  cachedChecksum,
		SizeBytes: 1024,
	}); err != nil {
		t.Fatalf("UpsertPackage: %v", err)
	}

	// Insert the blocked package (empty checksum, no artifact).
	if err := database.UpsertBlockedPackage(ctx, "npm", "ejs", "2.7.4"); err != nil {
		t.Fatalf("UpsertBlockedPackage: %v", err)
	}

	// Track every checksum passed to storage.Delete.
	var storageDeleteChecksums []string
	var storageDeleteCalls atomic.Int32
	store := &mockStorage{
		deleteFunc: func(_ context.Context, checksum string) error {
			storageDeleteCalls.Add(1)
			storageDeleteChecksums = append(storageDeleteChecksums, checksum)
			return nil
		},
	}

	var cacheDeleteCalls atomic.Int32
	cache := &mockCache{
		deleteFunc: func(_ context.Context, _, _, _ string) error {
			cacheDeleteCalls.Add(1)
			return nil
		},
	}

	var dbDeleteCalls atomic.Int32
	mockDatabase := &mockDB{
		// Use the real DB for listing — this is what the test is proving.
		listExpiredFunc: func(c context.Context, olderThan time.Time) ([]db.Package, error) {
			return database.ListExpiredPackages(c, olderThan)
		},
		deleteFunc: func(_ context.Context, _ int64) error {
			dbDeleteCalls.Add(1)
			return nil
		},
	}

	// Negative TTL → cutoff = time.Now().Add(+1h), making the just-inserted
	// cached package appear expired. The blocked package is excluded by the SQL
	// regardless of the cutoff.
	w := New(mockDatabase, cache, store, -1*time.Hour, time.Hour,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	if err := w.evict(ctx); err != nil {
		t.Fatalf("evict: %v", err)
	}

	// Exactly one storage delete — the cached package's checksum.
	if got := int(storageDeleteCalls.Load()); got != 1 {
		t.Errorf("storage.Delete call count = %d, want 1", got)
	}
	if len(storageDeleteChecksums) == 1 && storageDeleteChecksums[0] != cachedChecksum {
		t.Errorf("storage.Delete called with checksum %q, want %q", storageDeleteChecksums[0], cachedChecksum)
	}
	// The blocked package's empty checksum must never appear.
	for _, cs := range storageDeleteChecksums {
		if cs == "" {
			t.Error("storage.Delete called with empty checksum — blocked package was incorrectly evicted")
		}
	}

	// Redis and DB each deleted exactly once (cached package only).
	if got := int(cacheDeleteCalls.Load()); got != 1 {
		t.Errorf("cache.Delete call count = %d, want 1", got)
	}
	if got := int(dbDeleteCalls.Load()); got != 1 {
		t.Errorf("db.DeletePackage call count = %d, want 1", got)
	}

	// The blocked package row must still exist in the DB.
	blocked, err := database.GetPackage(ctx, "npm", "ejs", "2.7.4")
	if err != nil {
		t.Fatalf("GetPackage (blocked): %v", err)
	}
	if blocked.Status != "blocked" {
		t.Errorf("blocked package status = %q, want %q", blocked.Status, "blocked")
	}
}

func evictionDBConfig(t *testing.T, connStr string) db.Config {
	t.Helper()
	cfg := db.Config{SSLMode: "disable"}
	_, err := fmt.Sscanf(connStr,
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		&cfg.Host, &cfg.Port, &cfg.User, &cfg.Password, &cfg.DBName, &cfg.SSLMode,
	)
	if err != nil {
		cfg = db.Config{Host: "localhost", User: "user", Password: "pass", DBName: "testdb", SSLMode: "disable"}
		fmt.Sscanf(connStr, "postgres://user:pass@localhost:%d/testdb", &cfg.Port)
	}
	return cfg
}
