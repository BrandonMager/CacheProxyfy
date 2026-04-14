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

	name := normalise(m[1])
	filename := m[2]
	version := m[4]

	return &Package{
		Ecosystem: "pypi",
		Name: name,
		Version: version,
		Filename: filename,
	}, nil
}


func (p *PyPI) UpstreamURL(pkg *Package) string {
	return fmt.Sprintf("%s/packages/%s", p.UpstreamBase, pkg.Filename)
}

func (p *PyPI) RewriteResponse(_ context.Context, body []byte, _ *Package) ([]byte, error){
	return body, nil
}

func normalise(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ReplaceAll(name, ".", "-")
	return name
}
