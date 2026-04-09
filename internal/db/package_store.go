package db 

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrNotFound = errors.New("db: not found")

type Package struct {
	ID int64
	Ecosystem string
	Name string
	Version string
	Checksum string
	SizeBytes int64
	CachedAt time.Time
	LastHitAt *time.Time // or nil if never hit
}

/* Inserts new package row or updates checksum and size 
if the (ecosystem, name, version) already exists */

func (db *DB) UpsertPackage(ctx context.Context, pkg Package) (string, error) {
	const q = `
		INSERT INTO packages (ecosystem, name, version, checksum, size_bytes)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (ecosystem, name, version)
		DO UPDATE SET 
			checksum = EXCLUDED.checksum,
			size_bytes = EXCLUDED.size_bytes
		RETURNING checksum
	`

	var checksum string
	err := db.QueryRowContext(ctx, q, 
		pkg.Ecosystem,
		pkg.Name,
		pkg.Version,
		pkg.Checksum,
		pkg.SizeBytes,
	).Scan(&checksum)

	if err != nil {
		return "", fmt.Errorf("db: upsert package: %w", err)
	}

	return checksum, nil
}

func (db *DB) GetPackage(ctx Context.context, ecosystem, name, version string) (Package, error) {
	const q = `
		SELECT id, ecosystem, name, version, checksum, size_bytes, cached_at, last_hit_at
		FROM packages
		WHERE ecosystem = $1 AND name = $2 AND version = $3
	`

	var pkg Package
	err := db.QueryRowContext(ctx, q, ecosystem, name, version).Scan(
		&pkg.ID,
		&pkg.Ecosystem,
		&pkg.Name,
		&pkg.Version,
		&pkg.Checksum,
		&pkg.SizeBytes,
		&pkg.CachedAt,
		&pkg.LastHitAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return Package{}, ErrNotFound
	}

	if err != nil {
		return Package{}, fmt.Errorf("db: get package: %w", err)
	}
	
	return pkg, nil
}

func (db *DB) TouchPackage(ctx context.Context, ecosystem, name, version string) error {
	const q = `
		UPDATE packages
		SET last_hit_at = NOW()
		WHERE ecosystem = $1 AND name = $2 AND version = $3
	`

	_, err := db.ExecContext(ctx, q, ecosystem, name, version)
	if err != nil {
		return fmt.Errorf("db: touch package: %w", err)
	}

	return nil
}

func (db *DB) ListVersions(ctx context.Context, ecosystem, name string) ([]Package, error){
	const q = `
		SELECT id, ecosystem, name, version, checksum, size_bytes, cached_at, last_hit_at
		FROM packages
		WHERE ecosystem = $1 AND name = $2
		ORDER BY cached_at DESC
	`

	rows, err := db.QueryContext(ctx, q, ecosystem, name)
	if err != nil {
		return nil, fmt.Errorf("db: list versions: %w", err)
	}

	defer rows.Close()
	var pkgs []Package
	for rows.Next() {
		var pkg Package
		if err := rows.Scan(
			&pkg.ID,
			&pkg.Ecosystem, 
			&pkg.Name,
			&pkg.Version,
			&pkg.Checksum,
			&pkg.SizeBytes,
			&pkg.CachedAt,
			&pkg.LastHitAt,
		); err != nil {
			return nil, fmt.Errorf("db: list versions scan: %w", err)
		}

		pkgs = append(pkgs, pkg)
	}

	return pkgs, rows.Err()
}

func (db *DB) RecordEvent(ctx context.Context, ecosystem, name, version, event string, bytes int64) error {
	const q = `
		INSERT INTO cache_events (ecosystem, name, version, event, bytes)
		VALUES ($1, $2, $3, $4, $5)
	`

	_, err := db.ExecContext(ctx, q, ecosystem, name, version, event, bytes)
	if err != nil {
		return fmt.Errorf("db: record event: %w", err)
	}

	return nil
} 