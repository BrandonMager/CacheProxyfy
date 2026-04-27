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
	ID        int64      `json:"id"`
	Ecosystem string     `json:"ecosystem"`
	Name      string     `json:"name"`
	Version   string     `json:"version"`
	Checksum  string     `json:"checksum"`
	SizeBytes int64      `json:"size_bytes"`
	CachedAt  time.Time  `json:"cached_at"`
	LastHitAt *time.Time `json:"last_hit_at"`
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

func (db *DB) GetPackage(ctx context.Context, ecosystem, name, version string) (Package, error) {
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

// PackageSummary represents one unique (ecosystem, name) pair, aggregated
// across all cached versions.
type PackageSummary struct {
	Ecosystem      string     `json:"ecosystem"`
	Name           string     `json:"name"`
	LatestVersion  string     `json:"latest_version"`
	VersionCount   int        `json:"version_count"`
	TotalSizeBytes int64      `json:"total_size_bytes"`
	LastCachedAt   time.Time  `json:"last_cached_at"`
	LastHitAt      *time.Time `json:"last_hit_at"`
}

// ListPackageSummaries returns one row per unique (ecosystem, name), with the
// most recently cached version labelled as LatestCached. Optionally filtered
// by ecosystem.
func (db *DB) ListPackageSummaries(ctx context.Context, ecosystem string) ([]PackageSummary, error) {
	const qAll = `
		SELECT
			ecosystem,
			name,
			(
				SELECT version FROM packages p2
				WHERE p2.ecosystem = p.ecosystem AND p2.name = p.name
				ORDER BY cached_at DESC LIMIT 1
			) AS latest_version,
			COUNT(*)           AS version_count,
			SUM(size_bytes)    AS total_size_bytes,
			MAX(cached_at)     AS last_cached_at,
			MAX(last_hit_at)   AS last_hit_at
		FROM packages p
		GROUP BY ecosystem, name
		ORDER BY MAX(cached_at) DESC
	`
	const qEco = `
		SELECT
			ecosystem,
			name,
			(
				SELECT version FROM packages p2
				WHERE p2.ecosystem = p.ecosystem AND p2.name = p.name
				ORDER BY cached_at DESC LIMIT 1
			) AS latest_version,
			COUNT(*)           AS version_count,
			SUM(size_bytes)    AS total_size_bytes,
			MAX(cached_at)     AS last_cached_at,
			MAX(last_hit_at)   AS last_hit_at
		FROM packages p
		WHERE ecosystem = $1
		GROUP BY ecosystem, name
		ORDER BY MAX(cached_at) DESC
	`

	var (
		rows *sql.Rows
		err  error
	)
	if ecosystem == "" {
		rows, err = db.QueryContext(ctx, qAll)
	} else {
		rows, err = db.QueryContext(ctx, qEco, ecosystem)
	}
	if err != nil {
		return nil, fmt.Errorf("db: list package summaries: %w", err)
	}
	defer rows.Close()

	var summaries []PackageSummary
	for rows.Next() {
		var s PackageSummary
		if err := rows.Scan(
			&s.Ecosystem,
			&s.Name,
			&s.LatestVersion,
			&s.VersionCount,
			&s.TotalSizeBytes,
			&s.LastCachedAt,
			&s.LastHitAt,
		); err != nil {
			return nil, fmt.Errorf("db: list package summaries scan: %w", err)
		}
		summaries = append(summaries, s)
	}

	return summaries, rows.Err()
}

// ListPackages returns all cached packages ordered by most recently cached.
// If ecosystem is non-empty, results are filtered to that ecosystem.
func (db *DB) ListPackages(ctx context.Context, ecosystem string) ([]Package, error) {
	const qAll = `
		SELECT id, ecosystem, name, version, checksum, size_bytes, cached_at, last_hit_at
		FROM packages
		ORDER BY cached_at DESC
	`
	const qEco = `
		SELECT id, ecosystem, name, version, checksum, size_bytes, cached_at, last_hit_at
		FROM packages
		WHERE ecosystem = $1
		ORDER BY cached_at DESC
	`

	var (
		rows *sql.Rows
		err  error
	)
	if ecosystem == "" {
		rows, err = db.QueryContext(ctx, qAll)
	} else {
		rows, err = db.QueryContext(ctx, qEco, ecosystem)
	}
	if err != nil {
		return nil, fmt.Errorf("db: list packages: %w", err)
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
			return nil, fmt.Errorf("db: list packages scan: %w", err)
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

func (db *DB) RecordCVEAlert(ctx context.Context, ecosystem, name, version, cveID, severity, outcome string) error {
	const q = `
		INSERT INTO cve_alerts (ecosystem, name, version, cve_id, severity, outcome)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err := db.ExecContext(ctx, q, ecosystem, name, version, cveID, severity, outcome)
	if err != nil {
		return fmt.Errorf("db: record cve alert: %w", err)
	}

	return nil
}

type CVEAlert struct {
	ID         int64     `json:"id"`
	Ecosystem  string    `json:"ecosystem"`
	Name       string    `json:"name"`
	Version    string    `json:"version"`
	CVEID      string    `json:"cve_id"`
	Severity   string    `json:"severity"`
	Outcome    string    `json:"outcome"`
	RecordedAt time.Time `json:"recorded_at"`
}

// ListPackageCVEAlerts returns all CVE alerts ever recorded for a specific
// package version, ordered from most to least recent.
func (db *DB) ListPackageCVEAlerts(ctx context.Context, ecosystem, name, version string) ([]CVEAlert, error) {
	const q = `
		SELECT id, ecosystem, name, version, cve_id, severity, outcome, recorded_at
		FROM cve_alerts
		WHERE ecosystem = $1 AND name = $2 AND version = $3
		ORDER BY recorded_at DESC
	`

	rows, err := db.QueryContext(ctx, q, ecosystem, name, version)
	if err != nil {
		return nil, fmt.Errorf("db: list package cve alerts: %w", err)
	}
	defer rows.Close()

	var alerts []CVEAlert
	for rows.Next() {
		var a CVEAlert
		if err := rows.Scan(&a.ID, &a.Ecosystem, &a.Name, &a.Version, &a.CVEID, &a.Severity, &a.Outcome, &a.RecordedAt); err != nil {
			return nil, fmt.Errorf("db: list package cve alerts scan: %w", err)
		}
		alerts = append(alerts, a)
	}

	return alerts, rows.Err()
}

// ListCVEAlerts returns CVE alerts since the given time.
// If ecosystem is non-empty, results are filtered to that ecosystem.
func (db *DB) ListCVEAlerts(ctx context.Context, since time.Time, ecosystem string) ([]CVEAlert, error) {
	const qAll = `
		SELECT id, ecosystem, name, version, cve_id, severity, outcome, recorded_at
		FROM cve_alerts
		WHERE recorded_at >= $1
		ORDER BY recorded_at DESC
	`
	const qEco = `
		SELECT id, ecosystem, name, version, cve_id, severity, outcome, recorded_at
		FROM cve_alerts
		WHERE recorded_at >= $1 AND ecosystem = $2
		ORDER BY recorded_at DESC
	`

	var (
		rows *sql.Rows
		err  error
	)
	if ecosystem == "" {
		rows, err = db.QueryContext(ctx, qAll, since)
	} else {
		rows, err = db.QueryContext(ctx, qEco, since, ecosystem)
	}
	if err != nil {
		return nil, fmt.Errorf("db: list cve alerts: %w", err)
	}
	defer rows.Close()

	var alerts []CVEAlert
	for rows.Next() {
		var a CVEAlert
		if err := rows.Scan(&a.ID, &a.Ecosystem, &a.Name, &a.Version, &a.CVEID, &a.Severity, &a.Outcome, &a.RecordedAt); err != nil {
			return nil, fmt.Errorf("db: list cve alerts scan: %w", err)
		}
		alerts = append(alerts, a)
	}

	return alerts, rows.Err()
}

type Stats struct {
	TotalPackages int64   `json:"total_packages"`
	TotalHits     int64   `json:"total_hits"`
	TotalMisses   int64   `json:"total_misses"`
	BytesSaved    int64   `json:"bytes_saved"`
	HitRate       float64 `json:"hit_rate"`
}

func (db *DB) GetStats(ctx context.Context, since time.Time) (Stats, error) {
	const q = `
		SELECT 
			(SELECT COUNT(*) FROM packages) AS total_packages,
			COUNT(*) FILTER (WHERE event = 'hit') AS total_hits,
			COUNT(*) FILTER (WHERE event = 'miss') AS total_misses,
			COALESCE(SUM(bytes) FILTER (WHERE event = 'hit'), 0) AS bytes_saved

		FROM cache_events
		WHERE recorded_at >= $1
	`
	var s Stats
	err := db.QueryRowContext(ctx, q, since).Scan(
		&s.TotalPackages,
		&s.TotalHits,
		&s.TotalMisses,
		&s.BytesSaved,
	)

	if err != nil {
		return Stats{}, fmt.Errorf("db: get stats: %w", err)
	}

	total := s.TotalHits + s.TotalMisses
	if total > 0 {
		s.HitRate = float64(s.TotalHits) / float64(total)
	}

	return s, nil
}