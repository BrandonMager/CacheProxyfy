package ecosystem

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

var pypiArtifact = regexp.MustCompile(
	`/pypi/packages/[^/]+/[^/]+/([^/]+)/(([^/]+?)[-_](\d[^/]*?)(?:\.tar\.gz|\.whl|\.zip))$`,
)

type PyPI struct {
	UpstreamBase string
}

func NewPyPI() *PyPI {
	return &PyPI{UpstreamBase: "https://files.pythonhosted.org"}
}

func (p *PyPI) Parse(r *http.Request) (*Package, error){
	if strings.Contains(r.URL.Path, "/simple/") {
		return nil, ErrNotPackageRequest
	}

	m := pypiArtifact.FindStringSubmatch(r.URL.Path)
	if m == nil {
		return nil, ErrNotPackageRequest
	}

	name := normalise(m[3])
	filename := m[2]
	version := m[4]
	upstreamPath := strings.TrimPrefix(r.URL.Path, "/pypi/packages/")

	return &Package{
		Ecosystem:    "pypi",
		Name:         name,
		Version:      version,
		Filename:     filename,
		UpstreamPath: upstreamPath,
	}, nil
}


func (p *PyPI) UpstreamURL(pkg *Package) string {
	return fmt.Sprintf("%s/packages/%s", p.UpstreamBase, pkg.UpstreamPath)
}

func (p *PyPI) RewriteResponse(_ context.Context, body []byte, _ *Package) ([]byte, error) {
	return body, nil
}

// IsMetadataRequest reports whether r is a PyPI simple index or wheel metadata request.
func (p *PyPI) IsMetadataRequest(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/pypi/simple/") ||
		strings.HasSuffix(r.URL.Path, ".whl.metadata")
}

// MetadataUpstreamURL returns the upstream URL for simple index and wheel metadata requests.
// Simple index requests are proxied to pypi.org; wheel metadata files are proxied to
// files.pythonhosted.org to match where PyPI actually serves them.
func (p *PyPI) MetadataUpstreamURL(r *http.Request) string {
	path := strings.TrimPrefix(r.URL.Path, "/pypi")
	if strings.HasSuffix(r.URL.Path, ".whl.metadata") {
		return "https://files.pythonhosted.org" + path
	}
	return "https://pypi.org" + path
}

// RewriteMetadata replaces pythonhosted.org download URLs with proxy URLs
// so pip fetches artifacts through the proxy instead of directly.
func (p *PyPI) RewriteMetadata(body []byte, proxyBase string) ([]byte, error) {
	rewritten := strings.ReplaceAll(
		string(body),
		"https://files.pythonhosted.org",
		proxyBase+"/pypi",
	)
	return []byte(rewritten), nil
}

func normalise(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ReplaceAll(name, ".", "-")
	return name
}
