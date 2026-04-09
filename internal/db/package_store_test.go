package db

import (
	"context"
	"database/sql"
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// ── UpsertPackage ────────────────────────────────────────────────────────────

func TestUpsertPackage_ReturnsChecksum(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer sqlDB.Close()

	mock.ExpectQuery("INSERT INTO packages").
		WithArgs("npm", "lodash", "4.17.21", "abc123", int64(500)).
		WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow("abc123"))

	db := &DB{sqlDB}
	got, err := db.UpsertPackage(context.Background(), Package{
		Ecosystem: "npm",
		Name:      "lodash",
		Version:   "4.17.21",
		Checksum:  "abc123",
		SizeBytes: 500,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "abc123" {
		t.Errorf("checksum: got %q, want %q", got, "abc123")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestUpsertPackage_DBError(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer sqlDB.Close()

	mock.ExpectQuery("INSERT INTO packages").
		WillReturnError(errors.New("connection reset"))

	db := &DB{sqlDB}
	_, err = db.UpsertPackage(context.Background(), Package{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "db: upsert package: connection reset" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ── GetPackage ───────────────────────────────────────────────────────────────

func TestGetPackage_Found(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer sqlDB.Close()

	now := time.Now().UTC().Truncate(time.Second)
	mock.ExpectQuery("SELECT id, ecosystem").
		WithArgs("npm", "lodash", "4.17.21").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "ecosystem", "name", "version",
			"checksum", "size_bytes", "cached_at", "last_hit_at",
		}).AddRow(int64(1), "npm", "lodash", "4.17.21", "abc123", int64(500), now, nil))

	db := &DB{sqlDB}
	pkg, err := db.GetPackage(context.Background(), "npm", "lodash", "4.17.21")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pkg.Name != "lodash" || pkg.Checksum != "abc123" {
		t.Errorf("unexpected package: %+v", pkg)
	}
	if pkg.LastHitAt != nil {
		t.Errorf("expected nil LastHitAt, got %v", pkg.LastHitAt)
	}
}

func TestGetPackage_NotFound(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer sqlDB.Close()

	mock.ExpectQuery("SELECT id, ecosystem").
		WillReturnError(sql.ErrNoRows)

	db := &DB{sqlDB}
	_, err = db.GetPackage(context.Background(), "npm", "lodash", "4.17.21")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestGetPackage_DBError(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer sqlDB.Close()

	mock.ExpectQuery("SELECT id, ecosystem").
		WillReturnError(errors.New("timeout"))

	db := &DB{sqlDB}
	_, err = db.GetPackage(context.Background(), "npm", "lodash", "4.17.21")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "db: get package: timeout" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ── TouchPackage ─────────────────────────────────────────────────────────────

func TestTouchPackage_Success(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer sqlDB.Close()

	mock.ExpectExec("UPDATE packages").
		WithArgs("npm", "lodash", "4.17.21").
		WillReturnResult(sqlmock.NewResult(1, 1))

	db := &DB{sqlDB}
	if err := db.TouchPackage(context.Background(), "npm", "lodash", "4.17.21"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestTouchPackage_DBError(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer sqlDB.Close()

	mock.ExpectExec("UPDATE packages").
		WillReturnError(errors.New("deadlock"))

	db := &DB{sqlDB}
	err = db.TouchPackage(context.Background(), "npm", "lodash", "4.17.21")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "db: touch package: deadlock" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ── ListVersions ─────────────────────────────────────────────────────────────

func TestListVersions_ReturnsPackages(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer sqlDB.Close()

	now := time.Now().UTC().Truncate(time.Second)
	rows := sqlmock.NewRows([]string{
		"id", "ecosystem", "name", "version",
		"checksum", "size_bytes", "cached_at", "last_hit_at",
	}).
		AddRow(int64(1), "npm", "lodash", "4.17.21", "abc123", int64(500), now, nil).
		AddRow(int64(2), "npm", "lodash", "4.17.20", "def456", int64(480), now, nil)

	mock.ExpectQuery("SELECT id, ecosystem").
		WithArgs("npm", "lodash").
		WillReturnRows(rows)

	db := &DB{sqlDB}
	pkgs, err := db.ListVersions(context.Background(), "npm", "lodash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pkgs) != 2 {
		t.Errorf("expected 2 packages, got %d", len(pkgs))
	}
	if pkgs[0].Version != "4.17.21" || pkgs[1].Version != "4.17.20" {
		t.Errorf("unexpected versions: %v, %v", pkgs[0].Version, pkgs[1].Version)
	}
}

func TestListVersions_Empty(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer sqlDB.Close()

	mock.ExpectQuery("SELECT id, ecosystem").
		WithArgs("npm", "nonexistent").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "ecosystem", "name", "version",
			"checksum", "size_bytes", "cached_at", "last_hit_at",
		}))

	db := &DB{sqlDB}
	pkgs, err := db.ListVersions(context.Background(), "npm", "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pkgs) != 0 {
		t.Errorf("expected empty slice, got %d packages", len(pkgs))
	}
}

func TestListVersions_QueryError(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer sqlDB.Close()

	mock.ExpectQuery("SELECT id, ecosystem").
		WillReturnError(errors.New("query failed"))

	db := &DB{sqlDB}
	_, err = db.ListVersions(context.Background(), "npm", "lodash")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "db: list versions: query failed" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestListVersions_ScanError(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer sqlDB.Close()

	// Return a row with a bad value for id (string instead of int64)
	rows := sqlmock.NewRows([]string{
		"id", "ecosystem", "name", "version",
		"checksum", "size_bytes", "cached_at", "last_hit_at",
	}).AddRow("not-an-int", "npm", "lodash", "4.17.21", "abc123", int64(500), time.Now(), nil)

	mock.ExpectQuery("SELECT id, ecosystem").
		WillReturnRows(rows)

	db := &DB{sqlDB}
	_, err = db.ListVersions(context.Background(), "npm", "lodash")
	if err == nil {
		t.Fatal("expected scan error, got nil")
	}
}

// ── RecordEvent ──────────────────────────────────────────────────────────────

func TestRecordEvent_Success(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer sqlDB.Close()

	mock.ExpectExec("INSERT INTO cache_events").
		WithArgs("npm", "lodash", "4.17.21", "hit", int64(500)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	db := &DB{sqlDB}
	if err := db.RecordEvent(context.Background(), "npm", "lodash", "4.17.21", "hit", 500); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestRecordEvent_DBError(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer sqlDB.Close()

	mock.ExpectExec("INSERT INTO cache_events").
		WillReturnError(errors.New("insert failed"))

	db := &DB{sqlDB}
	err = db.RecordEvent(context.Background(), "npm", "lodash", "4.17.21", "hit", 500)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "db: record event: insert failed" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ── GetStats ─────────────────────────────────────────────────────────────────

func TestGetStats_HitRateComputed(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer sqlDB.Close()

	mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows([]string{
			"total_packages", "total_hits", "total_misses", "bytes_saved",
		}).AddRow(int64(10), int64(8), int64(2), int64(4096)))

	db := &DB{sqlDB}
	stats, err := db.GetStats(context.Background(), time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.TotalPackages != 10 {
		t.Errorf("TotalPackages: got %d, want 10", stats.TotalPackages)
	}
	if stats.TotalHits != 8 {
		t.Errorf("TotalHits: got %d, want 8", stats.TotalHits)
	}
	if stats.TotalMisses != 2 {
		t.Errorf("TotalMisses: got %d, want 2", stats.TotalMisses)
	}
	if stats.BytesSaved != 4096 {
		t.Errorf("BytesSaved: got %d, want 4096", stats.BytesSaved)
	}
	want := 0.8
	if stats.HitRate != want {
		t.Errorf("HitRate: got %f, want %f", stats.HitRate, want)
	}
}

func TestGetStats_NoEvents_HitRateZero(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer sqlDB.Close()

	mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows([]string{
			"total_packages", "total_hits", "total_misses", "bytes_saved",
		}).AddRow(int64(0), int64(0), int64(0), int64(0)))

	db := &DB{sqlDB}
	stats, err := db.GetStats(context.Background(), time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.HitRate != 0 {
		t.Errorf("HitRate: got %f, want 0", stats.HitRate)
	}
}

func TestGetStats_DBError(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer sqlDB.Close()

	mock.ExpectQuery("SELECT").
		WillReturnError(errors.New("query failed"))

	db := &DB{sqlDB}
	_, err = db.GetStats(context.Background(), time.Now())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "db: get stats: query failed" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ── Integration ──────────────────────────────────────────────────────────────

func TestPackageStore_Integration(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("user"),
		postgres.WithPassword("pass"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp")),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Errorf("failed to terminate container: %v", err)
		}
	}()

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	db, err := Open(configFromConnStr(t, connStr))
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}

	pkg := Package{
		Ecosystem: "npm",
		Name:      "lodash",
		Version:   "4.17.21",
		Checksum:  "abc123",
		SizeBytes: 500,
	}

	t.Run("UpsertPackage inserts", func(t *testing.T) {
		checksum, err := db.UpsertPackage(ctx, pkg)
		if err != nil {
			t.Fatalf("UpsertPackage: %v", err)
		}
		if checksum != pkg.Checksum {
			t.Errorf("checksum: got %q, want %q", checksum, pkg.Checksum)
		}
	})

	t.Run("UpsertPackage updates checksum", func(t *testing.T) {
		updated := pkg
		updated.Checksum = "xyz999"
		checksum, err := db.UpsertPackage(ctx, updated)
		if err != nil {
			t.Fatalf("UpsertPackage update: %v", err)
		}
		if checksum != "xyz999" {
			t.Errorf("updated checksum: got %q, want %q", checksum, "xyz999")
		}
	})

	t.Run("GetPackage returns row", func(t *testing.T) {
		got, err := db.GetPackage(ctx, pkg.Ecosystem, pkg.Name, pkg.Version)
		if err != nil {
			t.Fatalf("GetPackage: %v", err)
		}
		if got.Name != pkg.Name || got.Ecosystem != pkg.Ecosystem {
			t.Errorf("unexpected package: %+v", got)
		}
	})

	t.Run("GetPackage missing returns ErrNotFound", func(t *testing.T) {
		_, err := db.GetPackage(ctx, "npm", "lodash", "0.0.0")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got: %v", err)
		}
	})

	t.Run("TouchPackage sets last_hit_at", func(t *testing.T) {
		if err := db.TouchPackage(ctx, pkg.Ecosystem, pkg.Name, pkg.Version); err != nil {
			t.Fatalf("TouchPackage: %v", err)
		}
		got, err := db.GetPackage(ctx, pkg.Ecosystem, pkg.Name, pkg.Version)
		if err != nil {
			t.Fatalf("GetPackage after touch: %v", err)
		}
		if got.LastHitAt == nil {
			t.Error("expected LastHitAt to be set after TouchPackage")
		}
	})

	t.Run("ListVersions returns all versions", func(t *testing.T) {
		_, err := db.UpsertPackage(ctx, Package{
			Ecosystem: "npm", Name: "lodash", Version: "4.17.20",
			Checksum: "old111", SizeBytes: 480,
		})
		if err != nil {
			t.Fatalf("UpsertPackage v2: %v", err)
		}

		pkgs, err := db.ListVersions(ctx, "npm", "lodash")
		if err != nil {
			t.Fatalf("ListVersions: %v", err)
		}
		if len(pkgs) != 2 {
			t.Errorf("expected 2 versions, got %d", len(pkgs))
		}
	})

	t.Run("ListVersions unknown package returns empty", func(t *testing.T) {
		pkgs, err := db.ListVersions(ctx, "npm", "nonexistent")
		if err != nil {
			t.Fatalf("ListVersions: %v", err)
		}
		if len(pkgs) != 0 {
			t.Errorf("expected empty, got %d packages", len(pkgs))
		}
	})

	t.Run("RecordEvent and GetStats", func(t *testing.T) {
		since := time.Now().Add(-time.Minute)

		if err := db.RecordEvent(ctx, pkg.Ecosystem, pkg.Name, pkg.Version, "hit", 500); err != nil {
			t.Fatalf("RecordEvent hit: %v", err)
		}
		if err := db.RecordEvent(ctx, pkg.Ecosystem, pkg.Name, pkg.Version, "miss", 0); err != nil {
			t.Fatalf("RecordEvent miss: %v", err)
		}

		stats, err := db.GetStats(ctx, since)
		if err != nil {
			t.Fatalf("GetStats: %v", err)
		}
		if stats.TotalHits != 1 {
			t.Errorf("TotalHits: got %d, want 1", stats.TotalHits)
		}
		if stats.TotalMisses != 1 {
			t.Errorf("TotalMisses: got %d, want 1", stats.TotalMisses)
		}
		if stats.BytesSaved != 500 {
			t.Errorf("BytesSaved: got %d, want 500", stats.BytesSaved)
		}
		if stats.HitRate != 0.5 {
			t.Errorf("HitRate: got %f, want 0.5", stats.HitRate)
		}
	})
}
