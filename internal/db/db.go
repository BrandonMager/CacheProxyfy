package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)


type DB struct {
	*sql.DB
}

type Config struct {
	Host string
	Port int
	User string 
	Password string
	DBName string 
	SSLMode string
}

func (c Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode,
	)
}

func Open(cfg Config) (*DB, error) {
	sqlDB, err := sql.Open("postgres", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("db: open: %w", err)
	}

	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("db: ping: %w", err)
	}

	return &DB{sqlDB}, nil
}

var ErrMigrate = errors.New("db: migrate failed")

// Runs schema idempotently
func (db *DB) Migrate(ctx context.Context) error {
	_, err := db.ExecContext(ctx, schema)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrMigrate, err)
	}

	return nil
}

const schema = `
	CREATE TABLE IF NOT EXISTS packages (
		id BIGSERIAL PRIMARY KEY,
		ecosystem TEXT NOT NULL,
		name TEXT NOT NULL,
		version TEXT NOT NULL,
		checksum TEXT NOT NULL,
		size_bytes BIGINT NOT NULL DEFAULT 0,
		cached_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		last_hit_at TIMESTAMPTZ,
		CONSTRAINT packages_unique UNIQUE (ecosystem, name, version)
	);

	CREATE INDEX IF NOT EXISTS package_ecosystem_name
		ON packages (ecosystem, name);

	CREATE TABLE IF NOT EXISTS cache_events (
		id BIGSERIAL PRIMARY KEY,
		ecosystem TEXT NOT NULL,
		name TEXT NOT NULL,
		version TEXT NOT NULL,
		event TEXT NOT NULL, -- "hit" | "miss"
		bytes BIGINT NOT NULL DEFAULT 0,
		recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS cache_events_recorded_at
		ON cache_events (recorded_at DESC);

	CREATE TABLE IF NOT EXISTS cve_alerts (
		id BIGSERIAL PRIMARY KEY,
		ecosystem TEXT NOT NULL,
		name TEXT NOT NULL,
		version TEXT NOT NULL,
		cve_id TEXT NOT NULL,
		severity TEXT NOT NULL,
		outcome TEXT NOT NULL,
		recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS cve_alerts_recorded_at
		ON cve_alerts (recorded_at DESC);

	CREATE INDEX IF NOT EXISTS cve_alerts_ecosystem_name
		ON cve_alerts (ecosystem, name);

	CREATE INDEX IF NOT EXISTS packages_last_accessed
		ON packages (COALESCE(last_hit_at, cached_at));
`