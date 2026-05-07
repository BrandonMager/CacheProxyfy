package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"

	"github.com/BrandonMager/CacheProxyfy/internal/db"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestIntegration_Stats_BlockedPackagesCount verifies that GET /api/stats reflects
// the correct blocked_packages count after packages are blocked in the database.
// This exercises the real SQL COUNT query in GetStats, not a stub.
func TestIntegration_Stats_BlockedPackagesCount(t *testing.T) {
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

	mux := http.NewServeMux()
	NewHandler(database, nil).RegisterRoutes(mux)

	// Before any blocks: blocked_packages must be 0.
	stats := getStats(t, mux)
	if stats.BlockedPackages != 0 {
		t.Fatalf("baseline: blocked_packages = %d, want 0", stats.BlockedPackages)
	}

	// Block two distinct packages.
	if err := database.UpsertBlockedPackage(ctx, "npm", "ejs", "2.7.4"); err != nil {
		t.Fatalf("UpsertBlockedPackage ejs: %v", err)
	}
	if err := database.UpsertBlockedPackage(ctx, "npm", "lodash", "4.17.4"); err != nil {
		t.Fatalf("UpsertBlockedPackage lodash: %v", err)
	}

	stats = getStats(t, mux)
	if stats.BlockedPackages != 2 {
		t.Errorf("after 2 blocks: blocked_packages = %d, want 2", stats.BlockedPackages)
	}

	// Blocking the same package again (upsert) must not double-count.
	if err := database.UpsertBlockedPackage(ctx, "npm", "ejs", "2.7.4"); err != nil {
		t.Fatalf("UpsertBlockedPackage ejs (repeat): %v", err)
	}

	stats = getStats(t, mux)
	if stats.BlockedPackages != 2 {
		t.Errorf("after repeat upsert: blocked_packages = %d, want 2 (no double-count)", stats.BlockedPackages)
	}
}

// getStats calls GET /api/stats and decodes the response body into db.Stats.
func getStats(t *testing.T, mux *http.ServeMux) db.Stats {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/stats: status %d, body: %s", w.Code, w.Body.String())
	}
	var stats db.Stats
	if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
		t.Fatalf("decode stats: %v", err)
	}
	return stats
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
