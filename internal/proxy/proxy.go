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

	"github.com/BrandonMager/CacheProxyfy/internal/ecosystem"
	"github.com/BrandonMager/CacheProxyfy/internal/storage"
)

type Proxy struct {
	router *Router
	storage storage.StorageBackend
	client *http.Client
	logger *slog.Logger
}

func New(router *Router, store storage.StorageBackend, logger *slog.Logger) *Proxy {
	return &Proxy {
		router: router,
		storage: store,
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
	checksum := pkg.CacheKey()
	rc, err := p.storage.Get(ctx, checksum)
	if err == nil {
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			return nil, "", fmt.Errorf("reading from storage: %w", err)
		}

		return data, "hit", nil
	}

	if err != storage.ErrNotFound {
		return nil, "", fmt.Errorf("storage lookup: %w", err)
	}

	/* Not in cache so check upstream */

	data, err := p.fetchFromUpstream(ctx, handler, pkg)
	if err != nil {
		return nil, "", err
	}

	data, err = handler.RewriteResponse(ctx, data, pkg)
	if err != nil {
		return nil, "", fmt.Errorf("rewriting response: %w", err)
	}

	if err := p.storage.Put(ctx, checksum, bytes.NewReader(data), int64(len(data))); err != nil {
		p.logger.Warn("failed to store artifact", 
			"package", pkg.Name,
			"error", err,
		)
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

func (p *Proxy) handleHealth(w http.ResponseWriter, _ *http.Request){
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok", "storage":%q, "ecosystems":%q}`, p.storage.Name(), strings.Join(p.router.Ecosystems(), ","))
}