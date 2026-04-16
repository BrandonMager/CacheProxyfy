package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/BrandonMager/CacheProxyfy/internal/db"
	"github.com/BrandonMager/CacheProxyfy/internal/ecosystem"
	"github.com/BrandonMager/CacheProxyfy/internal/security"
	"github.com/BrandonMager/CacheProxyfy/internal/singleflight"
	"github.com/BrandonMager/CacheProxyfy/internal/storage"
)

type Proxy struct {
	router *Router
	storage storage.StorageBackend
	cache CacheClient
	db DBClient
	security SecurityChecker
	sf *singleflight.Group
	client *http.Client
	logger *slog.Logger
}

func New(router *Router, store storage.StorageBackend, logger *slog.Logger,
	cache CacheClient, db DBClient, security SecurityChecker,
) *Proxy {
	return &Proxy {
		router: router,
		storage: store,
		cache: cache,
		db: db,
		security: security,
		sf: singleflight.NewGroup(),
		client: &http.Client {
			Timeout: 5 * time.Minute,
		},
		logger: logger,
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

	data, cacheStatus, err := p.serve(r.Context(), handler, pkg)
	if err != nil {
		p.logger.Error("serve failed",
			"ecosystem", ecoName, "package", pkg.Name, "error", err,
		)

		http.Error(w, "upstream fetch failed", http.StatusBadGateway)
		return
	}

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
	//Check redis checksum 
	if checksum, err := p.cache.Get(ctx, pkg.Ecosystem, pkg.Name, pkg.Version); err == nil {
		rc, err := p.storage.Get(ctx, checksum)
		if err == nil {
			defer rc.Close()
			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, "", fmt.Errorf("reading from storage: %w", err)
			}

			go func() {
				p.db.TouchPackage(context.Background(), pkg.Ecosystem, pkg.Name, pkg.Version); 
				p.recordEvent(pkg, "hit", int64(len(data)))
			}()
			return data, "hit", nil
		}
	}

	if dbPkg, err := p.db.GetPackage(ctx, pkg.Ecosystem, pkg.Name, pkg.Version); err == nil {
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
	}

	go p.recordCVEAlerts(pkg, outcome, records)

	if outcome == security.Block {
		return nil, "", fmt.Errorf("package blocked by security policy: %s@%s", pkg.Name, pkg.Version)
	}

	if outcome == security.Warn {
		p.logger.Warn("package has known vulnerabilities",
			"ecosystem", pkg.Ecosystem,
			"package", pkg.Name,
			"version", pkg.Version,
			"cves", len(records),
		)
	}

	data, shared, err := p.sf.Do(pkg.Ecosystem, pkg.Name, pkg.Version, func() ([]byte, error) {
		return p.fetchFromUpstream(ctx, handler, pkg)
	})

	if err != nil {
		return nil, "", err
	}

	data, err = handler.RewriteResponse(ctx, data, pkg)
	if err != nil {
		return nil, "", fmt.Errorf("rewriting response: %w", err)
	}
	if shared {
		checksum := pkg.CacheKey()
		p.storage.Put(ctx, checksum, bytes.NewReader(data), int64(len(data)))
		go func() {
			p.cache.Set(context.Background(), pkg.Ecosystem, pkg.Name, pkg.Version, checksum)
			p.db.UpsertPackage(context.Background(), db.Package{
				Ecosystem: pkg.Ecosystem,
				Name: pkg.Name,
				Version: pkg.Version,
				Checksum: checksum,
				SizeBytes: int64(len(data)),
			})

			p.recordEvent(pkg, "miss", int64(len(data)))
		}()
	}

	return data, "miss", nil
}

func (p *Proxy) fetchFromUpstream(ctx context.Context, handler ecosystem.Handler, pkg *ecosystem.Package) ([]byte, error) {
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
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("package not found upstream: %s", url)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned %d for %s", resp.StatusCode, url)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading upstream body: %w", err)
	}

	return data, nil
}

func (p *Proxy) recordEvent(pkg *ecosystem.Package, event string, bytes int64){
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

func (p *Proxy) handleHealth(w http.ResponseWriter, _ *http.Request){
	redisOk := p.cache.Ping(context.Background()) == nil
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok", "storage":%q, "ecosystems":%q, "redis":%t}`, p.storage.Name(), strings.Join(p.router.Ecosystems(), ","), redisOk)
}