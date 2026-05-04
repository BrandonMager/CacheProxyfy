package proxy

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/BrandonMager/CacheProxyfy/internal/db"
	"github.com/BrandonMager/CacheProxyfy/internal/metrics"
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
	p := New(router, &mockStorage{}, logger, &mockCache{}, database, checker, m)

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
