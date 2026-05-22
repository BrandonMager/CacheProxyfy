package proxy

// integration_migration_test.go validates that the proxy handles a live
// backend switch from local FS → S3 correctly, without data loss or downtime.
//
// Why this matters:
//   When the storage backend is swapped at runtime (e.g. operator changes
//   cache.backend: local → s3 and restarts), Redis and Postgres still hold
//   checksums for previously cached artifacts — but those artifacts do not
//   exist in the new S3 bucket yet. The proxy must detect the storage miss
//   and transparently re-fetch from upstream, then serve all subsequent
//   requests from S3.
//
// Sequence:
//  Phase 1 — local FS backend
//    1a. Request lodash → X-Cache: miss, artifact stored on local disk,
//        DB row + Redis key written asynchronously.
//    1b. Second request → X-Cache: hit (Redis checksum → local FS read).
//
//  Switch  — rebuild proxy with real S3 backend; local FS is no longer
//            accessible, but Redis and Postgres still have the checksums.
//
//  Phase 2 — S3 backend (Redis + Postgres checksums intact)
//    2a. First request → Redis hit → S3 miss → DB hit → S3 miss →
//        upstream re-fetched → artifact written to S3. X-Cache: miss.
//    2b. S3 Exists() confirms the artifact is now in the bucket.
//    2c. Second request → Redis hit → S3 hit. X-Cache: hit.
//        Upstream not called again.

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/BrandonMager/CacheProxyfy/internal/cache"
	"github.com/BrandonMager/CacheProxyfy/internal/db"
	"github.com/BrandonMager/CacheProxyfy/internal/metrics"
	"github.com/BrandonMager/CacheProxyfy/internal/ratelimit"
	"github.com/BrandonMager/CacheProxyfy/internal/security"
	"github.com/BrandonMager/CacheProxyfy/internal/storage"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/localstack"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestIntegration_BackendMigration_LocalToS3(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()

	// --- Infrastructure ---

	lsCtr, err := localstack.Run(ctx, "localstack/localstack:3")
	testcontainers.CleanupContainer(t, lsCtr)
	if err != nil {
		t.Fatalf("start localstack: %v", err)
	}

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

	redisAddr := startRedisForSmoke(t)

	artifactDir, err := os.MkdirTemp("", "cacheproxyfy-migration-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(artifactDir) })

	// --- Shared components (survive the backend switch) ---

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

	// --- S3 setup (bucket created before Phase 2 starts) ---

	mappedPort, err := lsCtr.MappedPort(ctx, "4566/tcp")
	if err != nil {
		t.Fatalf("localstack port: %v", err)
	}
	provider, err := testcontainers.NewDockerProvider()
	if err != nil {
		t.Fatalf("docker provider: %v", err)
	}
	defer provider.Close()
	lsHost, err := provider.DaemonHost(ctx)
	if err != nil {
		t.Fatalf("daemon host: %v", err)
	}
	endpoint := "http://" + net.JoinHostPort(lsHost, mappedPort.Port())

	const bucket = "migration-artifacts"
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	if err != nil {
		t.Fatalf("load aws config: %v", err)
	}
	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})
	if _, err := s3Client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatalf("create bucket: %v", err)
	}

	checker := security.NewChecker(false, "CRITICAL", "HIGH")

	const (
		eco     = "npm"
		name    = "lodash"
		version = "4.17.21"
		path    = "/npm/lodash/-/lodash-4.17.21.tgz"
		body    = "fake-lodash-tarball-bytes"
	)

	var upstreamCalls int
	fakeUpstream := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		upstreamCalls++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})

	newProxy := func(store storage.StorageBackend) (*Proxy, *httptest.Server) {
		router := NewRouter([]string{"npm"})
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		m := metrics.New(prometheus.NewRegistry(), []string{})
		p := New(router, store, logger, redisClient, database, checker, m, ratelimit.New(false, 0, 0, nil))
		p.client.Transport = fakeUpstream
		return p, httptest.NewServer(p)
	}

	// =======================================================================
	// Phase 1 — local FS backend
	// =======================================================================

	localStore, err := storage.NewLocal(artifactDir)
	if err != nil {
		t.Fatalf("storage.NewLocal: %v", err)
	}

	_, srv1 := newProxy(localStore)
	client := srv1.Client()

	// 1a. Full miss: upstream fetched, artifact written to local FS.
	resp1a, err := client.Get(srv1.URL + path)
	if err != nil {
		t.Fatalf("phase 1a: request: %v", err)
	}
	io.Copy(io.Discard, resp1a.Body)
	resp1a.Body.Close()

	if resp1a.StatusCode != http.StatusOK {
		t.Fatalf("phase 1a: expected 200, got %d", resp1a.StatusCode)
	}
	if got := resp1a.Header.Get("X-Cache"); got != "miss" {
		t.Errorf("phase 1a: X-Cache = %q, want %q", got, "miss")
	}
	if upstreamCalls != 1 {
		t.Errorf("phase 1a: upstream calls = %d, want 1", upstreamCalls)
	}

	// Wait for DB row and Redis key to be written by the async goroutine.
	pkg1 := pollDB(t, database, eco, name, version)
	if pkg1.Status != "cached" {
		t.Errorf("phase 1a: DB status = %q, want %q", pkg1.Status, "cached")
	}
	pollRedis(t, redisClient, eco, name, version)

	// Confirm artifact is on local disk.
	if exists, err := localStore.Exists(ctx, pkg1.Checksum); err != nil || !exists {
		t.Fatalf("phase 1a: artifact not on local disk (exists=%v, err=%v)", exists, err)
	}

	// 1b. Redis hit: served from local FS via Redis checksum.
	resp1b, err := client.Get(srv1.URL + path)
	if err != nil {
		t.Fatalf("phase 1b: request: %v", err)
	}
	io.Copy(io.Discard, resp1b.Body)
	resp1b.Body.Close()

	if got := resp1b.Header.Get("X-Cache"); got != "hit" {
		t.Errorf("phase 1b: X-Cache = %q, want %q", got, "hit")
	}
	if upstreamCalls != 1 {
		t.Errorf("phase 1b: upstream called again — total=%d, want 1", upstreamCalls)
	}

	srv1.Close()

	// =======================================================================
	// Switch: rebuild proxy with S3 backend.
	// Redis and Postgres checksums are intact; S3 bucket is empty.
	// =======================================================================

	s3Store, err := storage.NewS3(ctx, storage.S3Config{
		Bucket:          bucket,
		Region:          "us-east-1",
		Endpoint:        endpoint,
		AccessKeyID:     "test",
		SecretAccessKey: "test",
	})
	if err != nil {
		t.Fatalf("storage.NewS3: %v", err)
	}

	// Confirm S3 bucket is empty before Phase 2 — the migration is the event under test.
	if exists, err := s3Store.Exists(ctx, pkg1.Checksum); err != nil || exists {
		t.Fatalf("pre-phase-2: artifact should not be in S3 yet (exists=%v, err=%v)", exists, err)
	}

	_, srv2 := newProxy(s3Store)
	defer srv2.Close()

	// =======================================================================
	// Phase 2 — S3 backend (Redis + Postgres checksums intact)
	// =======================================================================

	// 2a. First request after switch.
	//     Redis hit → S3 miss → DB hit → S3 miss → upstream re-fetched → stored in S3.
	resp2a, err := client.Get(srv2.URL + path)
	if err != nil {
		t.Fatalf("phase 2a: request: %v", err)
	}
	body2a, _ := io.ReadAll(resp2a.Body)
	resp2a.Body.Close()

	if resp2a.StatusCode != http.StatusOK {
		t.Fatalf("phase 2a: expected 200, got %d", resp2a.StatusCode)
	}
	if got := resp2a.Header.Get("X-Cache"); got != "miss" {
		t.Errorf("phase 2a: X-Cache = %q, want %q — proxy should re-fetch when S3 is empty", got, "miss")
	}
	if string(body2a) != body {
		t.Errorf("phase 2a: body = %q, want %q", string(body2a), body)
	}
	if upstreamCalls != 2 {
		t.Errorf("phase 2a: upstream calls = %d, want 2 (one per phase)", upstreamCalls)
	}

	// 2b. Artifact must now be in S3.
	// Allow the async write-back goroutine to finish before checking.
	pollRedis(t, redisClient, eco, name, version)
	if exists, err := s3Store.Exists(ctx, pkg1.Checksum); err != nil || !exists {
		t.Fatalf("phase 2b: artifact not in S3 after re-fetch (exists=%v, err=%v)", exists, err)
	}

	// 2c. Second request: Redis hit → S3 hit. Upstream must not be called again.
	resp2c, err := client.Get(srv2.URL + path)
	if err != nil {
		t.Fatalf("phase 2c: request: %v", err)
	}
	body2c, _ := io.ReadAll(resp2c.Body)
	resp2c.Body.Close()

	if resp2c.StatusCode != http.StatusOK {
		t.Fatalf("phase 2c: expected 200, got %d", resp2c.StatusCode)
	}
	if got := resp2c.Header.Get("X-Cache"); got != "hit" {
		t.Errorf("phase 2c: X-Cache = %q, want %q", got, "hit")
	}
	if string(body2c) != body {
		t.Errorf("phase 2c: body = %q, want %q", string(body2c), body)
	}
	if upstreamCalls != 2 {
		t.Errorf("phase 2c: upstream called again — total=%d, want 2", upstreamCalls)
	}
}
