package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BrandonMager/CacheProxyfy/internal/api"
	"github.com/BrandonMager/CacheProxyfy/internal/cache"
	"github.com/BrandonMager/CacheProxyfy/internal/config"
	"github.com/BrandonMager/CacheProxyfy/internal/db"
	"github.com/BrandonMager/CacheProxyfy/internal/metrics"
	"github.com/BrandonMager/CacheProxyfy/internal/proxy"
	"github.com/BrandonMager/CacheProxyfy/internal/security"
	"github.com/BrandonMager/CacheProxyfy/internal/storage"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	if err := run(logger); err != nil {
		logger.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	logger.Info("CacheProxyfy starting",
		"port", cfg.Proxy.Port,
		"backend", cfg.Cache.Backend,
		"ecosystems", cfg.Proxy.Ecosystems,
	)

	// Build a dedicated Prometheus registry so we don't pollute the global one.
	// Register the standard Go runtime and process collectors for free CPU/memory/GC metrics.
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	m := metrics.New(reg, cfg.Proxy.Ecosystems)

	store, err := buildStorage(cfg)
	if err != nil {
		return fmt.Errorf("building storage: %w", err)
	}
	logger.Info("storage ready", "backend", store.Name())

	redisClient, err := cache.New(cache.Config{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
		TTL:      time.Duration(cfg.Cache.TTLHours) * time.Hour,
	})
	if err != nil {
		return fmt.Errorf("connecting to redis: %w", err)
	}
	defer redisClient.Close()
	logger.Info("cache ready", "addr", cfg.Redis.Addr)

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

	if err := database.Migrate(context.Background()); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}
	logger.Info("database ready", "host", cfg.Database.Host)

	checker := security.NewChecker(cfg.Security.CVEScanning, cfg.Security.BlockSeverity, cfg.Security.WarnSeverity)

	router := proxy.NewRouter(cfg.Proxy.Ecosystems)
	p := proxy.New(router, store, logger, redisClient, database, checker, m)

	// Separate mux: proxy traffic on the main port, /metrics and /api/* on a
	// dedicated internal port (9090) so they are never accidentally exposed publicly.
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		EnableOpenMetrics: true, // Enables OpenMetrics text format (superset of Prometheus format)
	}))
	api.NewHandler(database, cfg).RegisterRoutes(metricsMux)

	metricsSrv := &http.Server{
		Addr:         "127.0.0.1:9090",
		Handler:      metricsMux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	proxySrv := http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Proxy.Port),
		Handler:      p,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 10 * time.Minute,
		IdleTimeout:  60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	// Forward SIGINT (Ctrl+C) and SIGTERM (OS shutdown) into the quit channel
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("metrics listening", "addr", metricsSrv.Addr)
		if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("metrics server error", "error", err)
			quit <- syscall.SIGTERM
		}
	}()

	// Start the HTTP server in a background goroutine so the main goroutine
	// is free to block on <-quit below. If the server crashes unexpectedly,
	// send SIGTERM into quit to trigger graceful shutdown.
	go func() {
		logger.Info("proxy listening", "addr", proxySrv.Addr)
		if err := proxySrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			quit <- syscall.SIGTERM
		}
	}()

	// Block here until a shutdown signal is received
	<-quit

	// Signal received — begin graceful shutdown with a 15-second deadline
	logger.Info("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	metricsSrv.Shutdown(ctx) //nolint:errcheck
	return proxySrv.Shutdown(ctx)
}

func buildStorage(cfg *config.Config) (storage.StorageBackend, error) {
	switch cfg.Cache.Backend {
	case "local":
		return storage.NewLocal(cfg.Cache.LocalDir)
	case "s3":
		return storage.NewS3(context.Background(), storage.S3Config{
			Bucket:          cfg.S3.Bucket,
			Region:          cfg.S3.Region,
			Endpoint:        cfg.S3.Endpoint,
			KeyPrefix:       cfg.S3.KeyPrefix,
			AccessKeyID:     cfg.S3.AccessKeyID,
			SecretAccessKey: cfg.S3.SecretAccessKey,
		})
	default:
		return nil, fmt.Errorf("Unknown storage backend: %q", cfg.Cache.Backend)
	}
}
