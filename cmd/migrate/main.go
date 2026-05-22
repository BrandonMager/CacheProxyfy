// migrate copies all cached artifacts from local FS → S3, pre-populating the
// S3 bucket before the operator switches cache.backend from "local" to "s3".
//
// Run this tool while the proxy is still on the local backend, then flip the
// config and restart — no upstream re-fetches, no downtime.
//
// Usage:
//
//	migrate [--dry-run] [--workers N]
//	  --dry-run   Print what would be uploaded without touching S3 (default: false)
//	  --workers   Number of concurrent S3 upload goroutines          (default: 4)
//
// Configuration is read from cacheproxyfy.yaml (or CACHEPROXYFY_* env vars),
// exactly as the main proxy binary does. Both [database] and [s3] sections
// must be populated before running.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"

	"github.com/BrandonMager/CacheProxyfy/internal/config"
	"github.com/BrandonMager/CacheProxyfy/internal/db"
	"github.com/BrandonMager/CacheProxyfy/internal/storage"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "Print what would be migrated without uploading to S3")
	workers := flag.Int("workers", 4, "Number of concurrent S3 upload goroutines")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("loading config", "error", err)
		os.Exit(1)
	}

	if err := run(context.Background(), cfg, logger, *dryRun, *workers); err != nil {
		logger.Error("migration failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg *config.Config, logger *slog.Logger, dryRun bool, workers int) error {
	if cfg.Cache.Backend != "local" {
		logger.Warn("cache.backend is not 'local' — run this tool before switching to S3",
			"backend", cfg.Cache.Backend,
		)
	}
	if cfg.S3.Bucket == "" {
		return fmt.Errorf("s3.bucket is not configured — set it in cacheproxyfy.yaml or CACHEPROXYFY_S3_BUCKET")
	}

	// --- Open Postgres to enumerate all cached packages ---
	database, err := db.Open(db.Config{
		Host:     cfg.Database.Host,
		Port:     cfg.Database.Port,
		User:     cfg.Database.User,
		Password: cfg.Database.Password,
		DBName:   cfg.Database.DBName,
		SSLMode:  cfg.Database.SSLMode,
	})
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer database.Close()

	packages, err := database.ListPackages(ctx, "")
	if err != nil {
		return fmt.Errorf("listing packages: %w", err)
	}

	// Deduplicate by checksum — multiple package versions can share the same
	// artifact bytes (and therefore the same content-addressed key).
	type entry struct {
		checksum  string
		sizeBytes int64
		label     string // "ecosystem/name@version" for logging
	}
	seen := make(map[string]struct{}, len(packages))
	var work []entry
	for _, pkg := range packages {
		if pkg.Status != "cached" || pkg.Checksum == "" {
			continue
		}
		if _, ok := seen[pkg.Checksum]; ok {
			continue
		}
		seen[pkg.Checksum] = struct{}{}
		work = append(work, entry{
			checksum:  pkg.Checksum,
			sizeBytes: pkg.SizeBytes,
			label:     fmt.Sprintf("%s/%s@%s", pkg.Ecosystem, pkg.Name, pkg.Version),
		})
	}

	total := len(work)
	logger.Info("migration starting",
		"packages", len(packages),
		"unique_artifacts", total,
		"dry_run", dryRun,
		"workers", workers,
		"source", cfg.Cache.LocalDir,
		"destination", fmt.Sprintf("s3://%s", cfg.S3.Bucket),
	)

	if total == 0 {
		logger.Info("nothing to migrate")
		return nil
	}

	// --- Build storage backends ---
	localStore, err := storage.NewLocal(cfg.Cache.LocalDir)
	if err != nil {
		return fmt.Errorf("opening local storage %q: %w", cfg.Cache.LocalDir, err)
	}

	var s3Store *storage.S3
	if !dryRun {
		s3Store, err = storage.NewS3(ctx, storage.S3Config{
			Bucket:          cfg.S3.Bucket,
			Region:          cfg.S3.Region,
			Endpoint:        cfg.S3.Endpoint,
			KeyPrefix:       cfg.S3.KeyPrefix,
			AccessKeyID:     cfg.S3.AccessKeyID,
			SecretAccessKey: cfg.S3.SecretAccessKey,
		})
		if err != nil {
			return fmt.Errorf("connecting to S3: %w", err)
		}
	}

	// --- Worker pool ---
	var (
		uploaded  atomic.Int64
		skipped   atomic.Int64
		notFound  atomic.Int64
		errCount  atomic.Int64
		mu        sync.Mutex
		index     int
	)

	workCh := make(chan entry)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for e := range workCh {
				mu.Lock()
				index++
				n := index
				mu.Unlock()

				prefix := fmt.Sprintf("[%*d/%d] %s (%s)",
					len(fmt.Sprintf("%d", total)), n, total,
					e.checksum[:12],
					e.label,
				)

				if dryRun {
					fmt.Printf("%s → would upload (%s)\n", prefix, formatBytes(e.sizeBytes))
					uploaded.Add(1)
					continue
				}

				// Skip if already in S3 (idempotent — safe to re-run).
				exists, err := s3Store.Exists(ctx, e.checksum)
				if err != nil {
					fmt.Printf("%s → ERROR checking S3: %v\n", prefix, err)
					errCount.Add(1)
					continue
				}
				if exists {
					fmt.Printf("%s → skipped (already in S3)\n", prefix)
					skipped.Add(1)
					continue
				}

				// Read from local FS.
				rc, err := localStore.Get(ctx, e.checksum)
				if err == storage.ErrNotFound {
					fmt.Printf("%s → WARNING: not found on local disk, skipping\n", prefix)
					notFound.Add(1)
					continue
				}
				if err != nil {
					fmt.Printf("%s → ERROR reading local storage: %v\n", prefix, err)
					errCount.Add(1)
					continue
				}

				// Stream directly to S3.
				if err := s3Store.Put(ctx, e.checksum, rc, e.sizeBytes); err != nil {
					rc.Close()
					fmt.Printf("%s → ERROR uploading to S3: %v\n", prefix, err)
					errCount.Add(1)
					continue
				}
				rc.Close()

				fmt.Printf("%s → uploaded (%s)\n", prefix, formatBytes(e.sizeBytes))
				uploaded.Add(1)
			}
		}()
	}

	for _, e := range work {
		workCh <- e
	}
	close(workCh)
	wg.Wait()

	// --- Summary ---
	fmt.Println()
	if dryRun {
		logger.Info("dry run complete — no data was written",
			"would_upload", uploaded.Load(),
		)
		return nil
	}

	errs := errCount.Load()
	logger.Info("migration complete",
		"uploaded", uploaded.Load(),
		"skipped", skipped.Load(),
		"not_found_on_disk", notFound.Load(),
		"errors", errs,
	)

	if errs > 0 {
		return fmt.Errorf("%d artifact(s) failed to upload — check output above", errs)
	}
	return nil
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
