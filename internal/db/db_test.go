package db

import (
	"context"
	"errors"
	"testing"

	"fmt"
	"os/exec"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestMigrate_Success(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer sqlDB.Close()

	mock.ExpectExec("CREATE TABLE").WillReturnResult(sqlmock.NewResult(0, 0))

	db := &DB{sqlDB}
	if err := db.Migrate(context.Background()); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestMigrate_SentinelError(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer sqlDB.Close()

	mock.ExpectExec("CREATE TABLE").WillReturnError(errors.New("exec failed"))

	db := &DB{sqlDB}
	err = db.Migrate(context.Background())
	if !errors.Is(err, ErrMigrate) {
		t.Errorf("expected ErrMigrate, got: %v", err)
	}
}

func TestMigrate_ErrorMessage(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer sqlDB.Close()

	mock.ExpectExec("CREATE TABLE").WillReturnError(errors.New("exec failed"))

	db := &DB{sqlDB}
	err = db.Migrate(context.Background())
	want := "db: migrate failed: exec failed"
	if err.Error() != want {
		t.Errorf("expected %q, got %q", want, err.Error())
	}
}

func TestMigrate_Idempotent(t *testing.T) {
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
		t.Fatalf("first Migrate failed: %v", err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate failed (not idempotent): %v", err)
	}
}

// configFromConnStr parses a postgres connection string of the form
// "host=... port=... user=... password=... dbname=... sslmode=..."
// as returned by testcontainers.
func configFromConnStr(t *testing.T, connStr string) Config {
	t.Helper()
	cfg := Config{SSLMode: "disable"}
	_, err := fmt.Sscanf(connStr,
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		&cfg.Host, &cfg.Port, &cfg.User, &cfg.Password, &cfg.DBName, &cfg.SSLMode,
	)
	if err != nil {
		// connStr format may vary; fall back to known test values
		cfg = Config{
			Host:     "localhost",
			User:     "user",
			Password: "pass",
			DBName:   "testdb",
			SSLMode:  "disable",
		}
		// parse port from the connection string
		fmt.Sscanf(connStr, "postgres://user:pass@localhost:%d/testdb", &cfg.Port)
	}
	return cfg
}
