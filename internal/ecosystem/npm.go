package ecosystem

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)


var npmTarball = regexp.MustCompile(
	`^/npm/(@[^/]+/[^/]+|[^@/][^/]*)/-/([^/]+\.tgz)$`,
)

type NPM struct {
	UpstreamBase string
}

func NewNPM() *NPM {
	return &NPM{ UpstreamBase: "https://registry.npmjs.org" }
}


func (n *NPM) Parse(r *http.Request) (*Package, error){
	m := npmTarball.FindStringSubmatch(r.URL.Path)
	if m == nil {
		return nil, ErrNotPackageRequest
	}

	name := m[1]
	filename := m[2]
	version := extractNPMVersion(name, filename)

	return &Package{
		Ecosystem: "npm",
		Name: name,
		Version: version,
		Filename: filename,
	}, nil
}

func (n *NPM) UpstreamURL(pkg *Package) string {
	return fmt.Sprintf("%s/%s/-/%s", n.UpstreamBase, pkg.Name, pkg.Filename)
}

func (n *NPM) RewriteResponse(_ context.Context, body []byte, _ *Package) ([]byte, error){
	return body, nil
}

func extractNPMVersion(name, filename string) string {
	base := name
	if i := strings.LastIndex(name, "/"); i >= 0 {
		base = name[i+1:]
	}

	version := strings.TrimPrefix(filename, base + "-")
	version = strings.TrimSuffix(version, ".tgz")
	return version
}