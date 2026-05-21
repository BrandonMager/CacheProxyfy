package proxy

// integration_s3_test.go exercises the full proxy request lifecycle when S3
// (via LocalStack) is the active storage backend. These tests complement
// integration_test.go — which uses mock storage — by validating that the proxy
// correctly stores and retrieves real artifacts from S3.

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

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

// s3Endpoint resolves the LocalStack container's host+port into an HTTP endpoint
// string that the S3 backend can use as its BaseEndpoint override.
func s3Endpoint(t *testing.T, ctx context.Context, ctr *localstack.LocalStackContainer) string {
	t.Helper()
	mappedPort, err := ctr.MappedPort(ctx, "4566/tcp")
	if err != nil {
		t.Fatalf("localstack port: %v", err)
	}
	provider, err := testcontainers.NewDockerProvider()
	if err != nil {
		t.Fatalf("docker provider: %v", err)
	}
	defer provider.Close()
	host, err := provider.DaemonHost(ctx)
	if err != nil {
		t.Fatalf("daemon host: %v", err)
	}
	return "http://" + net.JoinHostPort(host, mappedPort.Port())
}

// createBucket creates an S3 bucket in LocalStack using a raw AWS SDK client.
// Bucket provisioning is outside the scope of StorageBackend, so we do it
// directly in the test using the same credentials/endpoint as the backend.
func createBucket(t *testing.T, ctx context.Context, endpoint, bucket string) {
	t.Helper()
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	if err != nil {
		t.Fatalf("load aws config: %v", err)
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})
	if _, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatalf("create bucket %q: %v", bucket, err)
	}
}

// TestIntegration_S3Backend_MissThenHit verifies the full artifact lifecycle
// when the proxy is wired with a real S3 backend (LocalStack) and a real
// Postgres database:
//
//   - First request (full cache miss): upstream bytes are fetched, the artifact
//     is stored in S3, and a DB row is created with status='cached'.
//     X-Cache: miss is returned to the caller.
//   - S3 object verified: Exists() confirms the artifact key is in the bucket.
//   - Second request (DB hit → S3 read): the DB provides the checksum, the
//     artifact is read back from S3, upstream is never called again, and
//     X-Cache: hit is returned.
func TestIntegration_S3Backend_MissThenHit(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()

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
	defer func() {
		if err := testcontainers.TerminateContainer(pgCtr); err != nil {
			t.Errorf("terminate postgres: %v", err)
		}
	}()

	endpoint := s3Endpoint(t, ctx, lsCtr)
	const bucket = "test-artifacts"
	createBucket(t, ctx, endpoint, bucket)

	store, err := storage.NewS3(ctx, storage.S3Config{
		Bucket:          bucket,
		Region:          "us-east-1",
		Endpoint:        endpoint,
		AccessKeyID:     "test",
		SecretAccessKey: "test",
	})
	if err != nil {
		t.Fatalf("NewS3: %v", err)
	}

	connStr, err := pgCtr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	database, err := db.Open(dbConfigFromConnStr(t, connStr))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	defer database.Close()
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("db migrate: %v", err)
	}

	checker := security.NewChecker(false, "CRITICAL", "HIGH")
	router := NewRouter([]string{"npm"})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := metrics.New(prometheus.NewRegistry(), []string{})
	// mockCache always misses — forces every request through the DB path so the
	// test exercises the full DB-checksum → S3-read path on the second request.
	p := New(router, store, logger, &mockCache{}, database, checker, m, ratelimit.New(false, 0, 0, nil))

	const fakeBody = "fake-lodash-bytes"
	var upstreamCalls int
	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		upstreamCalls++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(fakeBody)),
		}, nil
	})

	const (
		eco     = "npm"
		name    = "lodash"
		version = "4.17.21"
	)

	// --- First request: cache miss → upstream fetch → S3 store ---
	req1 := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
	w1 := httptest.NewRecorder()
	p.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", w1.Code)
	}
	if got := w1.Header().Get("X-Cache"); got != "miss" {
		t.Errorf("first request: X-Cache = %q, want %q", got, "miss")
	}
	if w1.Body.String() != fakeBody {
		t.Errorf("first request: body = %q, want %q", w1.Body.String(), fakeBody)
	}
	if upstreamCalls != 1 {
		t.Errorf("first request: upstream calls = %d, want 1", upstreamCalls)
	}

	// UpsertPackage runs in a goroutine — poll until the DB row lands.
	var (
		pkg    db.Package
		dbErr  error
	)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		pkg, dbErr = database.GetPackage(ctx, eco, name, version)
		if dbErr == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if dbErr != nil {
		t.Fatalf("DB row not found within 10s: %v", dbErr)
	}
	if pkg.Status != "cached" {
		t.Errorf("DB status = %q, want %q", pkg.Status, "cached")
	}

	// Confirm the artifact is present in S3.
	exists, err := store.Exists(ctx, pkg.Checksum)
	if err != nil {
		t.Fatalf("S3 Exists: %v", err)
	}
	if !exists {
		t.Fatal("artifact not found in S3 after first request — storage.Put may not have been called with the S3 backend")
	}

	// --- Second request: DB hit → S3 read (upstream must not be called again) ---
	req2 := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
	w2 := httptest.NewRecorder()
	p.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("second request: expected 200, got %d", w2.Code)
	}
	if got := w2.Header().Get("X-Cache"); got != "hit" {
		t.Errorf("second request: X-Cache = %q, want %q", got, "hit")
	}
	if w2.Body.String() != fakeBody {
		t.Errorf("second request: body = %q, want %q", w2.Body.String(), fakeBody)
	}
	if upstreamCalls != 1 {
		t.Errorf("second request: upstream called again (total=%d) — expected exactly 1 upstream call", upstreamCalls)
	}
}
