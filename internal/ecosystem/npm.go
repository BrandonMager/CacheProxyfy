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

// npmManifest matches GET /npm/<name> and GET /npm/@scope/name (no trailing slash path segments beyond the name).
var npmManifest = regexp.MustCompile(
	`^/npm/(@[^/]+/[^/]+|[^@/][^/]*)$`,
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

// IsMetadataRequest reports whether r is an npm manifest request (package metadata JSON).
func (n *NPM) IsMetadataRequest(r *http.Request) bool {
	return npmManifest.MatchString(r.URL.Path)
}

// MetadataUpstreamURL returns the upstream registry URL for the manifest request.
func (n *NPM) MetadataUpstreamURL(r *http.Request) string {
	name := strings.TrimPrefix(r.URL.Path, "/npm/")
	return n.UpstreamBase + "/" + name
}

// RewriteMetadata replaces registry.npmjs.org tarball URLs with proxy URLs
// so npm fetches tarballs through the proxy instead of directly.
func (n *NPM) RewriteMetadata(body []byte, proxyBase string) ([]byte, error) {
	rewritten := strings.ReplaceAll(
		string(body),
		n.UpstreamBase,
		proxyBase+"/npm",
	)
	return []byte(rewritten), nil
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