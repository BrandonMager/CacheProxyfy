package proxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/BrandonMager/CacheProxyfy/internal/db"
	"github.com/BrandonMager/CacheProxyfy/internal/ecosystem"
	"github.com/BrandonMager/CacheProxyfy/internal/metrics"
	"github.com/BrandonMager/CacheProxyfy/internal/ratelimit"
	"github.com/BrandonMager/CacheProxyfy/internal/security"
	"github.com/BrandonMager/CacheProxyfy/internal/singleflight"
	"github.com/BrandonMager/CacheProxyfy/internal/storage"
)

// errPackageBlocked is returned by serve when a security policy blocks a package.
// ServeHTTP uses it to return 403 Forbidden instead of the generic 502.
var errPackageBlocked = errors.New("package blocked by security policy")

// errRateLimited is returned by serve when the per-ecosystem token bucket is exhausted
// and the request context expires before a token becomes available.
// ServeHTTP uses it to return 429 Too Many Requests instead of the generic 502.
var errRateLimited = errors.New("rate limit exceeded")

type Proxy struct {
	router   *Router
	storage  storage.StorageBackend
	cache    CacheClient
	db       DBClient
	security SecurityChecker
	limiter  *ratelimit.Limiter
	sf       *singleflight.Group
	client   *http.Client
	logger   *slog.Logger
	metrics  *metrics.Metrics
	retry    retryConfig
}

func New(router *Router, store storage.StorageBackend, logger *slog.Logger,
	cache CacheClient, db DBClient, security SecurityChecker, m *metrics.Metrics,
	limiter *ratelimit.Limiter,
) *Proxy {
	return &Proxy{
		router:   router,
		storage:  store,
		cache:    cache,
		db:       db,
		security: security,
		limiter:  limiter,
		sf:       singleflight.NewGroup(),
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
		logger:  logger,
		metrics: m,
		retry:   defaultRetryConfig,
	}
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	if r.URL.Path == "/healthz" {
		p.handleHealth(w, r)
		return
	}

	ecoName, handler, ok := p.router.Match(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	if mh, ok := handler.(ecosystem.MetadataHandler); ok && mh.IsMetadataRequest(r) {
		p.handleMetadata(w, r, mh)
		return
	}

	pkg, err := handler.Parse(r)
	if err == ecosystem.ErrNotPackageRequest {
		http.NotFound(w, r)
		return
	}

	if err != nil {
		p.logger.Error("parse failed", "path", r.URL.Path, "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	p.metrics.InflightRequests.Inc()
	defer p.metrics.InflightRequests.Dec()

	data, cacheStatus, err := p.serve(r.Context(), handler, pkg)

	elapsed := time.Since(start).Seconds()
	result := cacheStatus
	if err != nil {
		result = "error"
	}
	p.metrics.RequestsTotal.WithLabelValues(ecoName, result).Inc()
	p.metrics.RequestDuration.WithLabelValues(ecoName, result).Observe(elapsed)

	if err != nil {
		if errors.Is(err, errPackageBlocked) {
			p.logger.Warn("package blocked",
				"ecosystem", ecoName, "package", pkg.Name,
			)
			http.Error(w, "package blocked by security policy", http.StatusForbidden)
			return
		}
		if errors.Is(err, errRateLimited) {
			p.logger.Warn("rate limit exceeded",
				"ecosystem", ecoName, "package", pkg.Name,
			)
			http.Error(w, "rate limit exceeded — retry later", http.StatusTooManyRequests)
			return
		}
		p.logger.Error("serve failed",
			"ecosystem", ecoName, "package", pkg.Name, "error", err,
		)
		http.Error(w, "upstream fetch failed", http.StatusBadGateway)
		return
	}

	p.metrics.BytesServedTotal.WithLabelValues(ecoName, cacheStatus).Add(float64(len(data)))
	p.metrics.PackageSizeBytes.WithLabelValues(ecoName).Observe(float64(len(data)))

	w.Header().Set("X-Cache", cacheStatus)
	w.Header().Set("x-CacheProxyfy-Ecosystem", ecoName)
	w.WriteHeader(http.StatusOK)
	w.Write(data)

	p.logger.Info("served",
		"ecosystem", ecoName,
		"package", pkg.Name,
		"version", pkg.Version,
		"cache", cacheStatus,
		"ms", time.Since(start).Milliseconds(),
	)
}

func (p *Proxy) serve(ctx context.Context, handler ecosystem.Handler, pkg *ecosystem.Package) ([]byte, string, error) {
	// Check redis checksum
	if checksum, err := p.cache.Get(ctx, pkg.Ecosystem, pkg.Name, pkg.Version); err == nil {
		rc, err := p.storage.Get(ctx, checksum)
		if err == nil {
			defer rc.Close()
			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, "", fmt.Errorf("reading from storage: %w", err)
			}

			go func() {
				p.db.TouchPackage(context.Background(), pkg.Ecosystem, pkg.Name, pkg.Version)
				p.recordEvent(pkg, "hit", int64(len(data)))
			}()
			return data, "hit", nil
		}
	}

	if dbPkg, err := p.db.GetPackage(ctx, pkg.Ecosystem, pkg.Name, pkg.Version); err == nil {
		if dbPkg.Status == "blocked" {
			p.logger.Info("package previously blocked, skipping storage and security scan",
				"ecosystem", pkg.Ecosystem, "package", pkg.Name, "version", pkg.Version,
			)
			return nil, "", errPackageBlocked
		}
		rc, err := p.storage.Get(ctx, dbPkg.Checksum)
		if err == nil {
			defer rc.Close()
			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, "", fmt.Errorf("reading from storage: %w", err)
			}
			go func() {
				p.cache.Set(context.Background(), pkg.Ecosystem, pkg.Name, pkg.Version, dbPkg.Checksum)
				p.recordEvent(pkg, "hit", int64(len(data)))
			}()
			return data, "hit", nil
		}
	}

	outcome, records, err := p.security.Check(ctx, pkg.Ecosystem, pkg.Name, pkg.Version)
	if err != nil {
		p.logger.Warn("security check failed", "package", pkg.Name, "error", err)
		p.metrics.CVEScansTotal.WithLabelValues(pkg.Ecosystem, "error").Inc()
	} else {
		p.metrics.CVEScansTotal.WithLabelValues(pkg.Ecosystem, outcome.String()).Inc()
	}

	go p.recordCVEAlerts(pkg, outcome, records)

	if outcome == security.Block {
		go func() {
			if err := p.db.UpsertBlockedPackage(context.Background(), pkg.Ecosystem, pkg.Name, pkg.Version); err != nil {
				p.logger.Warn("upsert blocked package failed", "package", pkg.Name, "error", err)
			}
		}()
		return nil, "", errPackageBlocked
	}

	if outcome == security.Warn {
		p.logger.Warn("package has known vulnerabilities",
			"ecosystem", pkg.Ecosystem,
			"package", pkg.Name,
			"version", pkg.Version,
			"cves", len(records),
		)
	}

	var fetchDuration time.Duration
	data, shared, err := p.sf.Do(pkg.Ecosystem, pkg.Name, pkg.Version, func() ([]byte, error) {
		waitStart := time.Now()
		if err := p.limiter.Wait(ctx, pkg.Ecosystem); err != nil {
			return nil, fmt.Errorf("%w: %w", errRateLimited, err)
		}
		p.metrics.RateLimitWaitDuration.WithLabelValues(pkg.Ecosystem).Observe(time.Since(waitStart).Seconds())
		fetchStart := time.Now()
		result, fetchErr := p.fetchFromUpstream(ctx, handler, pkg)
		fetchDuration = time.Since(fetchStart)
		return result, fetchErr
	})

	// Only the singleflight leader performs a real upstream fetch — record it once.
	if shared {
		if err != nil {
			p.metrics.UpstreamFetchesTotal.WithLabelValues(pkg.Ecosystem, "error").Inc()
		} else {
			p.metrics.UpstreamFetchesTotal.WithLabelValues(pkg.Ecosystem, "ok").Inc()
			p.metrics.UpstreamFetchDuration.WithLabelValues(pkg.Ecosystem).Observe(fetchDuration.Seconds())
		}
	}

	if err != nil {
		return nil, "", err
	}

	data, err = handler.RewriteResponse(ctx, data, pkg)
	if err != nil {
		return nil, "", fmt.Errorf("rewriting response: %w", err)
	}
	if shared {
		checksum := pkg.CacheKey()
		if err := p.storage.Put(ctx, checksum, bytes.NewReader(data), int64(len(data))); err != nil {
			p.logger.Warn("storage put failed", "package", pkg.Name, "error", err)
		}
		go func() {
			p.cache.Set(context.Background(), pkg.Ecosystem, pkg.Name, pkg.Version, checksum)
			if _, err := p.db.UpsertPackage(context.Background(), db.Package{
				Ecosystem: pkg.Ecosystem,
				Name:      pkg.Name,
				Version:   pkg.Version,
				Checksum:  checksum,
				SizeBytes: int64(len(data)),
			}); err != nil {
				p.logger.Warn("upsert package failed", "package", pkg.Name, "error", err)
			}

			p.recordEvent(pkg, "miss", int64(len(data)))
		}()
	}

	return data, "miss", nil
}

// fetchFromUpstream fetches pkg from the upstream registry with exponential backoff
// retry on transient failures (network errors, 5xx, 429). Non-retryable errors
// (404, 403, context cancellation) are returned immediately.
func (p *Proxy) fetchFromUpstream(ctx context.Context, handler ecosystem.Handler, pkg *ecosystem.Package) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < p.retry.maxAttempts; attempt++ {
		data, err := p.doFetch(ctx, handler, pkg)
		if err == nil {
			return data, nil
		}
		lastErr = err
		if attempt == p.retry.maxAttempts-1 || !isRetryable(err) {
			break
		}
		delay := p.retryDelay(attempt, err)
		p.logger.Warn("upstream fetch failed, retrying",
			"ecosystem", pkg.Ecosystem,
			"package", pkg.Name,
			"version", pkg.Version,
			"attempt", attempt+1,
			"of", p.retry.maxAttempts,
			"delay_ms", delay.Milliseconds(),
			"error", err,
		)
		p.metrics.UpstreamRetriesTotal.WithLabelValues(pkg.Ecosystem).Inc()
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, lastErr
}

// doFetch performs a single upstream HTTP GET for pkg with no retry logic.
func (p *Proxy) doFetch(ctx context.Context, handler ecosystem.Handler, pkg *ecosystem.Package) ([]byte, error) {
	url := handler.UpstreamURL(pkg)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building upstream request: %w", err)
	}

	req.Header.Set("User-Agent", "CacheProxyfy/0.1")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		httpErr := &httpStatusError{code: resp.StatusCode, url: url}
		if resp.StatusCode == http.StatusTooManyRequests {
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, parseErr := strconv.Atoi(ra); parseErr == nil && secs > 0 {
					httpErr.retryAfter = time.Duration(secs) * time.Second
				}
			}
		}
		return nil, httpErr
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading upstream body: %w", err)
	}
	return data, nil
}

func (p *Proxy) recordEvent(pkg *ecosystem.Package, event string, bytes int64) {
	if err := p.db.RecordEvent(context.Background(), pkg.Ecosystem, pkg.Name, pkg.Version, event, bytes); err != nil {
		p.logger.Warn("record event failed", "package", pkg.Name, "error", err)
	}
}

func (p *Proxy) recordCVEAlerts(pkg *ecosystem.Package, outcome security.Outcome, records []security.CVERecord) {
	for _, r := range records {
		if err := p.db.RecordCVEAlert(context.Background(), pkg.Ecosystem, pkg.Name, pkg.Version, r.ID, r.Severity.String(), outcome.String()); err != nil {
			p.logger.Warn("record cve alert failed", "package", pkg.Name, "cve", r.ID, "error", err)
		}
	}
}

func (p *Proxy) handleMetadata(w http.ResponseWriter, r *http.Request, mh ecosystem.MetadataHandler) {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	proxyBase := scheme + "://" + r.Host

	upstreamURL := mh.MetadataUpstreamURL(r)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, upstreamURL, nil)
	if err != nil {
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}
	req.Header.Set("User-Agent", "CacheProxyfy/0.1")

	resp, err := p.client.Do(req)
	if err != nil {
		http.Error(w, "upstream fetch failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "reading upstream response failed", http.StatusBadGateway)
		return
	}

	rewritten, err := mh.RewriteMetadata(body, proxyBase)
	if err != nil {
		http.Error(w, "rewriting metadata failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	w.Write(rewritten) //nolint:errcheck
}

func (p *Proxy) handleHealth(w http.ResponseWriter, _ *http.Request) {
	redisOk := p.cache.Ping(context.Background()) == nil
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok", "storage":%q, "ecosystems":%q, "redis":%t}`, p.storage.Name(), strings.Join(p.router.Ecosystems(), ","), redisOk)
}
