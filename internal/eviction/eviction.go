package eviction

import (
	"context"
	"log/slog"
	"time"

	"github.com/BrandonMager/CacheProxyfy/internal/db"
	"github.com/BrandonMager/CacheProxyfy/internal/storage"
)

type DBClient interface {
	ListExpiredPackages(ctx context.Context, olderThan time.Time) ([]db.Package, error)
	DeletePackage(ctx context.Context, id int64) error
}

type CacheClient interface {
	Delete(ctx context.Context, ecosystem, name, version string) error
}

type Worker struct {
	db       DBClient
	cache    CacheClient
	storage  storage.StorageBackend
	ttl      time.Duration
	interval time.Duration
	logger   *slog.Logger
}

func New(
	database DBClient,
	cache CacheClient,
	store storage.StorageBackend,
	ttl time.Duration,
	interval time.Duration,
	logger *slog.Logger,
) *Worker {
	return &Worker{
		db:       database,
		cache:    cache,
		storage:  store,
		ttl:      ttl,
		interval: interval,
		logger:   logger,
	}
}

// Run blocks until ctx is cancelled, running an eviction cycle on each tick.
func (w *Worker) Run(ctx context.Context) {
	if w.ttl <= 0 {
		w.logger.Warn("eviction disabled: ttl_hours must be > 0")
		<-ctx.Done()
		return
	}

	interval := w.interval
	if interval <= 0 {
		w.logger.Warn("eviction_interval_hours invalid, defaulting to 1h")
		interval = time.Hour
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.evict(ctx); err != nil {
				w.logger.Error("eviction cycle failed", "error", err)
			}
		}
	}
}

func (w *Worker) evict(ctx context.Context) error {
	cutoff := time.Now().Add(-w.ttl)
	pkgs, err := w.db.ListExpiredPackages(ctx, cutoff)
	if err != nil {
		return err
	}

	if len(pkgs) == 0 {
		w.logger.Debug("eviction cycle: nothing to evict")
		return nil
	}

	start := time.Now()
	var evicted, failed int
	var bytesFreed int64

	for _, pkg := range pkgs {
		if err := w.evictOne(ctx, pkg); err != nil {
			w.logger.Error("failed to evict package",
				"ecosystem", pkg.Ecosystem,
				"name", pkg.Name,
				"version", pkg.Version,
				"error", err,
			)
			failed++
			continue
		}
		evicted++
		bytesFreed += pkg.SizeBytes
	}

	w.logger.Info("eviction cycle complete",
		"attempted", len(pkgs),
		"evicted", evicted,
		"failed", failed,
		"bytes_freed", bytesFreed,
		"duration_ms", time.Since(start).Milliseconds(),
	)

	return nil
}

func (w *Worker) evictOne(ctx context.Context, pkg db.Package) error {
	// 1. Storage first — if this fails the artifact still exists, so don't
	//    touch Redis or Postgres.
	if err := w.storage.Delete(ctx, pkg.Checksum); err != nil {
		return err
	}

	// 2. Redis — a stale key is worse than a missing one; log but continue.
	if err := w.cache.Delete(ctx, pkg.Ecosystem, pkg.Name, pkg.Version); err != nil {
		w.logger.Warn("eviction: failed to delete redis key",
			"ecosystem", pkg.Ecosystem,
			"name", pkg.Name,
			"version", pkg.Version,
			"error", err,
		)
	}

	// 3. Postgres last — authoritative record removed only after storage is clean.
	if err := w.db.DeletePackage(ctx, pkg.ID); err != nil {
		return err
	}

	w.logger.Info("evicted package",
		"ecosystem", pkg.Ecosystem,
		"name", pkg.Name,
		"version", pkg.Version,
		"checksum", pkg.Checksum,
		"size_bytes", pkg.SizeBytes,
	)

	return nil
}
