package proxy

// smoke_test.go boots the full infrastructure stack — Redis, Postgres, and local
// storage — and fires real HTTP requests through a live httptest.Server.
//
// Unlike the other integration tests (which call ServeHTTP directly with mock
// storage or a mock cache), this file validates all three cache tiers working
// together as a unit:
//
//	Tier 1 — Redis   : fast in-memory checksum lookup
//	Tier 2 — Postgres: authoritative metadata / checksum store
//	Tier 3 — Local FS: artifact bytes on disk
//
// Scenarios covered in order:
//  1. Full cache miss  — upstream fetched, artifact stored, DB row + Redis key written
//  2. Redis hit        — served from local FS via Redis checksum; upstream not called again
//  3. DB hit           — Redis key manually evicted; DB provides checksum, FS serves bytes,
//                        Redis is backfilled asynchronously
//  4. Health check     — /healthz returns 200 with redis=true and storage="local"

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/BrandonMager/CacheProxyfy/internal/cache"
	"github.com/BrandonMager/CacheProxyfy/internal/db"
	"github.com/BrandonMager/CacheProxyfy/internal/metrics"
	"github.com/BrandonMager/CacheProxyfy/internal/ratelimit"
	"github.com/BrandonMager/CacheProxyfy/internal/security"
	"github.com/BrandonMager/CacheProxyfy/internal/storage"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// startRedisForSmoke spins up a Redis container and returns its host:port address.
func startRedisForSmoke(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "redis:7-alpine",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForLog("Ready to accept connections"),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start redis: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(ctr); err != nil {
			t.Errorf("terminate redis: %v", err)
		}
	})
	host, err := ctr.Host(ctx)
	if err != nil {
		t.Fatalf("redis host: %v", err)
	}
	port, err := ctr.MappedPort(ctx, "6379/tcp")
	if err != nil {
		t.Fatalf("redis port: %v", err)
	}
	return host + ":" + port.Port()
}

// pollDB retries database.GetPackage until the row appears or the deadline is reached.
func pollDB(t *testing.T, database *db.DB, eco, name, version string) db.Package {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		pkg, err := database.GetPackage(ctx, eco, name, version)
		if err == nil {
			return pkg
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("DB row for %s/%s@%s not found within 10s", eco, name, version)
	return db.Package{}
}

// pollRedis retries cache.Get until the key appears or the deadline is reached.
func pollRedis(t *testing.T, redisClient *cache.Client, eco, name, version string) string {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		checksum, err := redisClient.Get(ctx, eco, name, version)
		if err == nil {
			return checksum
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("Redis key for %s/%s@%s not found within 10s", eco, name, version)
	return ""
}

// TestSmoke_FullStack_AllCacheTiers validates the full proxy request lifecycle
// against real Redis, Postgres, and local-FS storage, exercising every cache
// tier in sequence within a single test run.
func TestSmoke_FullStack_AllCacheTiers(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()

	// --- Infrastructure ---

	redisAddr := startRedisForSmoke(t)

	pgCtr, err := postgres.Run(ctx, "postgres:16-alpine",
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
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(pgCtr); err != nil {
			t.Errorf("terminate postgres: %v", err)
		}
	})

	artifactDir, err := os.MkdirTemp("", "cacheproxyfy-smoke-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(artifactDir) })

	// --- Components ---

	redisClient, err := cache.New(cache.Config{Addr: redisAddr, TTL: time.Hour})
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { redisClient.Close() })

	connStr, err := pgCtr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	database, err := db.Open(dbConfigFromConnStr(t, connStr))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	store, err := storage.NewLocal(artifactDir)
	if err != nil {
		t.Fatalf("storage.NewLocal: %v", err)
	}

	checker := security.NewChecker(false, "CRITICAL", "HIGH")
	router := NewRouter([]string{"npm"})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := metrics.New(prometheus.NewRegistry(), []string{})
	p := New(router, store, logger, redisClient, database, checker, m, ratelimit.New(false, 0, 0, nil))

	const fakeBody = "fake-lodash-tarball-bytes"
	var upstreamCalls int
	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		upstreamCalls++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(fakeBody)),
		}, nil
	})

	srv := httptest.NewServer(p)
	t.Cleanup(srv.Close)

	client := srv.Client()

	const (
		eco     = "npm"
		name    = "lodash"
		version = "4.17.21"
		path    = "/npm/lodash/-/lodash-4.17.21.tgz"
	)

	// -----------------------------------------------------------------------
	// Scenario 1: Full cache miss
	//   Redis miss → DB miss → upstream fetch → stored to local FS
	//   DB row and Redis key written asynchronously after response
	// -----------------------------------------------------------------------

	resp1, err := client.Get(srv.URL + path)
	if err != nil {
		t.Fatalf("scenario 1: request: %v", err)
	}
	defer resp1.Body.Close()
	body1, _ := io.ReadAll(resp1.Body)

	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("scenario 1: expected 200, got %d", resp1.StatusCode)
	}
	if got := resp1.Header.Get("X-Cache"); got != "miss" {
		t.Errorf("scenario 1: X-Cache = %q, want %q", got, "miss")
	}
	if string(body1) != fakeBody {
		t.Errorf("scenario 1: body = %q, want %q", string(body1), fakeBody)
	}
	if upstreamCalls != 1 {
		t.Errorf("scenario 1: upstream calls = %d, want 1", upstreamCalls)
	}

	// Wait for the async write-back goroutine to populate both DB and Redis.
	pkg := pollDB(t, database, eco, name, version)
	if pkg.Status != "cached" {
		t.Errorf("scenario 1: DB status = %q, want %q", pkg.Status, "cached")
	}
	pollRedis(t, redisClient, eco, name, version)

	// Confirm the artifact landed on local FS.
	if exists, err := store.Exists(ctx, pkg.Checksum); err != nil || !exists {
		t.Fatalf("scenario 1: artifact not on disk (exists=%v, err=%v)", exists, err)
	}

	// -----------------------------------------------------------------------
	// Scenario 2: Redis hit
	//   Redis has the checksum → serve bytes directly from local FS.
	//   Upstream must not be called again.
	// -----------------------------------------------------------------------

	resp2, err := client.Get(srv.URL + path)
	if err != nil {
		t.Fatalf("scenario 2: request: %v", err)
	}
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)

	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("scenario 2: expected 200, got %d", resp2.StatusCode)
	}
	if got := resp2.Header.Get("X-Cache"); got != "hit" {
		t.Errorf("scenario 2: X-Cache = %q, want %q", got, "hit")
	}
	if string(body2) != fakeBody {
		t.Errorf("scenario 2: body = %q, want %q", string(body2), fakeBody)
	}
	if upstreamCalls != 1 {
		t.Errorf("scenario 2: upstream called again (total=%d), want 1", upstreamCalls)
	}

	// -----------------------------------------------------------------------
	// Scenario 3: DB hit after Redis eviction
	//   Manually delete the Redis key to simulate TTL expiry.
	//   Redis miss → DB hit → read artifact from local FS → serve bytes.
	//   Redis is backfilled asynchronously; upstream still must not be called.
	// -----------------------------------------------------------------------

	if err := redisClient.Delete(ctx, eco, name, version); err != nil {
		t.Fatalf("scenario 3: redis delete: %v", err)
	}

	resp3, err := client.Get(srv.URL + path)
	if err != nil {
		t.Fatalf("scenario 3: request: %v", err)
	}
	defer resp3.Body.Close()
	body3, _ := io.ReadAll(resp3.Body)

	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("scenario 3: expected 200, got %d", resp3.StatusCode)
	}
	if got := resp3.Header.Get("X-Cache"); got != "hit" {
		t.Errorf("scenario 3: X-Cache = %q, want %q", got, "hit")
	}
	if string(body3) != fakeBody {
		t.Errorf("scenario 3: body = %q, want %q", string(body3), fakeBody)
	}
	if upstreamCalls != 1 {
		t.Errorf("scenario 3: upstream called again (total=%d), want 1", upstreamCalls)
	}

	// Confirm Redis was backfilled after the DB hit.
	pollRedis(t, redisClient, eco, name, version)

	// -----------------------------------------------------------------------
	// Scenario 4: Health check
	//   /healthz must return 200 JSON with redis=true and storage="local".
	// -----------------------------------------------------------------------

	resp4, err := client.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("scenario 4: request: %v", err)
	}
	defer resp4.Body.Close()

	if resp4.StatusCode != http.StatusOK {
		t.Fatalf("scenario 4: expected 200, got %d", resp4.StatusCode)
	}
	if ct := resp4.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("scenario 4: Content-Type = %q, want %q", ct, "application/json")
	}

	var health map[string]any
	if err := json.NewDecoder(resp4.Body).Decode(&health); err != nil {
		t.Fatalf("scenario 4: decode JSON: %v", err)
	}
	if got := health["status"]; got != "ok" {
		t.Errorf("scenario 4: status = %v, want %q", got, "ok")
	}
	if got := health["redis"]; got != true {
		t.Errorf("scenario 4: redis = %v, want true", got)
	}
	if got := health["storage"]; got != "local" {
		t.Errorf("scenario 4: storage = %v, want %q", got, "local")
	}
}
