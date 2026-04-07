package ecosystem

import (
	"context"
	"errors"
	"net/http"
)

var ErrNotPackageRequest = errors.New("Not a Package Request")

type Package struct {
	Ecosystem string
	Name string
	Version string 
	Filename string
}

func (p *Package) CacheKey() string {
	return p.Ecosystem + ":" + p.Name + ":" + p.Version + ":" + p.Filename
}

type Handler interface {
	Parse(r *http.Request) (*Package, error)
	UpstreamURL(pkg *Package) string
	RewriteResponse(ctx context.Context, body []byte, pkg *Package) ([] byte, error)
}