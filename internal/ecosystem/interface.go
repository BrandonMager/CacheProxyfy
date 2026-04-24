package ecosystem

import (
	"context"
	"errors"
	"net/http"
)

var ErrNotPackageRequest = errors.New("Not a Package Request")

type Package struct {
	Ecosystem    string
	Name         string
	Version      string
	Filename     string
	UpstreamPath string // optional: full relative path to use in upstream URL (e.g. PyPI hash path)
}

func (p *Package) CacheKey() string {
	return p.Ecosystem + ":" + p.Name + ":" + p.Version + ":" + p.Filename
}

type Handler interface {
	Parse(r *http.Request) (*Package, error)
	UpstreamURL(pkg *Package) string
	RewriteResponse(ctx context.Context, body []byte, pkg *Package) ([]byte, error)
}

// MetadataHandler is an optional interface for ecosystems that need to proxy
// index/metadata requests (e.g. PyPI simple index) in addition to artifacts.
// Handlers that don't implement this only serve direct file downloads.
type MetadataHandler interface {
	IsMetadataRequest(r *http.Request) bool
	MetadataUpstreamURL(r *http.Request) string
	RewriteMetadata(body []byte, proxyBase string) ([]byte, error)
}