package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BrandonMager/CacheProxyfy/internal/db"
	"github.com/BrandonMager/CacheProxyfy/internal/metrics"
	"github.com/BrandonMager/CacheProxyfy/internal/ratelimit"
	"github.com/BrandonMager/CacheProxyfy/internal/security"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestIntegration_GoPackage_CVEAlertsRecorded verifies the full CVE scanning
// pipeline for the Go ecosystem. It proxies a known-vulnerable Go module
// (github.com/gin-gonic/gin@v1.6.0) against a real Postgres container and
// asserts that CVE alerts are written to the database with correct severities:
//   - No UNKNOWN-severity alerts (all GHSA grades must be resolved)
//   - No GO-* CVE IDs (duplicates filtered by the scanner)
//   - At least one MEDIUM-severity alert (from MODERATE-rated advisories)
func TestIntegration_GoPackage_CVEAlertsRecorded(t *testing.T) {
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

	database, err := db.Open(dbConfigFromConnStr(t, connStr))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	defer database.Close()

	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("db migrate: %v", err)
	}

	// Real scanner hitting the live OSV API; warn at LOW so all resolved
	// severities (LOW, MEDIUM, HIGH, CRITICAL) trigger alert recording.
	checker := security.NewChecker(true, "CRITICAL", "LOW")

	router := NewRouter([]string{"go"})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := metrics.New(prometheus.NewRegistry(), []string{})
	p := New(router, &mockStorage{}, logger, &mockCache{}, database, checker, m, ratelimit.New(false, 0, 0, nil))

	// Intercept the upstream module fetch — the test only cares about CVE alerts,
	// not the actual zip content.
	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("fake zip bytes")),
		}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/go/github.com/gin-gonic/gin/@v/v1.6.0.zip", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Verify the package bytes are returned to the caller — not just a 200 with an empty body.
	if got := w.Body.String(); got != "fake zip bytes" {
		t.Fatalf("body: got %q, want %q", got, "fake zip bytes")
	}

	// recordCVEAlerts runs in a goroutine — poll until alerts land in Postgres.
	const (
		eco     = "go"
		name    = "github.com/gin-gonic/gin"
		version = "v1.6.0"
	)

	var alerts []db.CVEAlert
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		alerts, err = database.ListPackageCVEAlerts(ctx, eco, name, version)
		if err != nil {
			t.Fatalf("list cve alerts: %v", err)
		}
		if len(alerts) > 0 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if len(alerts) == 0 {
		t.Fatal("no CVE alerts recorded within 10s — CVE scanning pipeline may be broken")
	}

	for _, a := range alerts {
		if a.Severity == "UNKNOWN" {
			t.Errorf("alert %s has UNKNOWN severity — severity classification is broken", a.CVEID)
		}
		if strings.HasPrefix(a.CVEID, "GO-") {
			t.Errorf("GO-* duplicate %s was not filtered and appears in alerts", a.CVEID)
		}
	}

	hasMedium := false
	for _, a := range alerts {
		if a.Severity == "MEDIUM" {
			hasMedium = true
			break
		}
	}
	if !hasMedium {
		t.Errorf("expected at least one MEDIUM alert (from MODERATE-rated advisories) — got severities: %v",
			severitiesOf(alerts))
	}
}

// dbConfigFromConnStr parses a Testcontainers postgres connection string into a db.Config.
func dbConfigFromConnStr(t *testing.T, connStr string) db.Config {
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

// TestIntegration_NpmPackage_CriticalCVE_Blocked verifies the end-to-end blocking
// pipeline when security.cve_scanning=true and security.block_severity=CRITICAL.
// It requests ejs@2.7.4, which has a CRITICAL server-side template injection
// vulnerability (GHSA-phwq-j96m-2c2q, CVSS 9.8), against a real Postgres
// container and asserts:
//   - The proxy returns 403 Forbidden (not 502)
//   - The package row in the DB has status = 'blocked'
func TestIntegration_NpmPackage_CriticalCVE_Blocked(t *testing.T) {
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

	database, err := db.Open(dbConfigFromConnStr(t, connStr))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	defer database.Close()

	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("db migrate: %v", err)
	}

	// cve_scanning=true, block_severity=CRITICAL — mirrors the documented config.
	checker := security.NewChecker(true, "CRITICAL", "HIGH")

	router := NewRouter([]string{"npm"})

	var logBuf strings.Builder
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	store := &mockStorage{}
	m := metrics.New(prometheus.NewRegistry(), []string{})
	p := New(router, store, logger, &mockCache{}, database, checker, m, ratelimit.New(false, 0, 0, nil))

	// The upstream should never be reached for a blocked package.
	upstreamCalled := false
	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		upstreamCalled = true
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("should not be reached")),
		}, nil
	})

	const (
		eco     = "npm"
		name    = "ejs"
		version = "2.7.4"
	)

	// --- First request: OSV scan → block → DB persisted ---
	req := httptest.NewRequest(http.MethodGet, "/npm/ejs/-/ejs-2.7.4.tgz", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("first request: expected 403 Forbidden, got %d", w.Code)
	}
	if upstreamCalled {
		t.Error("upstream should not be called for a blocked package")
	}

	// UpsertBlockedPackage runs in a goroutine — poll the DB until the row lands.
	var pkg db.Package
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		pkg, err = database.GetPackage(ctx, eco, name, version)
		if err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("package row not found in DB within 10s: %v", err)
	}
	if pkg.Status != "blocked" {
		t.Errorf("expected package status %q, got %q", "blocked", pkg.Status)
	}

	// --- Second request: DB short-circuit, no storage or OSV scan ---
	store.getCalled.Store(false) // reset between requests
	logBuf.Reset()

	req2 := httptest.NewRequest(http.MethodGet, "/npm/ejs/-/ejs-2.7.4.tgz", nil)
	w2 := httptest.NewRecorder()
	p.ServeHTTP(w2, req2)

	if w2.Code != http.StatusForbidden {
		t.Fatalf("second request: expected 403 Forbidden, got %d", w2.Code)
	}
	if store.getCalled.Load() {
		t.Error("second request: storage.Get was called — blocked packages must not touch object storage")
	}
	if log := logBuf.String(); !strings.Contains(log, "previously blocked") {
		t.Errorf("second request: expected log line containing %q; got: %s", "previously blocked", log)
	}
}

// TestIntegration_CachedPackage_MissThenHit exercises the full miss→cached→hit
// lifecycle against a real Postgres container:
//   - First request: upstream is fetched, X-Cache: miss, DB row status = 'cached'
//   - Second request: served from storage via DB checksum, X-Cache: hit, upstream never called again
func TestIntegration_CachedPackage_MissThenHit(t *testing.T) {
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

	database, err := db.Open(dbConfigFromConnStr(t, connStr))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	defer database.Close()

	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("db migrate: %v", err)
	}

	// CVE scanning disabled — package is clean and always allowed.
	checker := security.NewChecker(false, "CRITICAL", "HIGH")

	// In-memory storage so the test controls Put/Get without a real object store.
	var storageMu sync.Mutex
	storageData := make(map[string][]byte)
	store := &mockStorage{
		getFunc: func(_ context.Context, checksum string) (io.ReadCloser, error) {
			storageMu.Lock()
			defer storageMu.Unlock()
			if data, ok := storageData[checksum]; ok {
				return io.NopCloser(bytes.NewReader(data)), nil
			}
			return nil, fmt.Errorf("storage: not found")
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

	const fakeBody = "fake-lodash-bytes"

	router := NewRouter([]string{"npm"})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := metrics.New(prometheus.NewRegistry(), []string{})
	// mockCache always misses — forces every request through the DB path.
	p := New(router, store, logger, &mockCache{}, database, checker, m, ratelimit.New(false, 0, 0, nil))

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

	// --- First request: cache miss ---
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
		t.Errorf("first request: upstream call count = %d, want 1", upstreamCalls)
	}

	// The write-back goroutine (UpsertPackage) runs async — poll until the DB row lands.
	var pkg db.Package
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		pkg, err = database.GetPackage(ctx, eco, name, version)
		if err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("DB row not found within 10s: %v", err)
	}
	if pkg.Status != "cached" {
		t.Errorf("DB status = %q, want %q", pkg.Status, "cached")
	}

	// --- Second request: DB hit → storage hit ---
	store.getCalled.Store(false)

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
	if !store.getCalled.Load() {
		t.Error("second request: storage.Get was never called — expected DB-checksum lookup to hit object storage")
	}
}

func severitiesOf(alerts []db.CVEAlert) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, a := range alerts {
		if _, ok := seen[a.Severity]; !ok {
			seen[a.Severity] = struct{}{}
			out = append(out, a.Severity)
		}
	}
	return out
}
