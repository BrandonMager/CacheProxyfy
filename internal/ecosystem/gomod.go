package ecosystem

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// goArtifact matches GET /go/<module>/@v/<version>.zip
// Module paths may be case-encoded (e.g., !burnt!sushi for BurntSushi).
var goArtifact = regexp.MustCompile(
	`^/go/(.+)/@v/([^/]+)\.zip$`,
)

// goMetadataPath matches GOPROXY metadata endpoints:
// /@v/list, /@v/<version>.info, /@v/<version>.mod, and /@latest
var goMetadataPath = regexp.MustCompile(
	`^/go/(.+)/(@v/(?:list|[^/]+\.(?:info|mod))|@latest)$`,
)

type GoMod struct {
	UpstreamBase string
}

func NewGoMod() *GoMod {
	return &GoMod{UpstreamBase: "https://proxy.golang.org"}
}

func (g *GoMod) Parse(r *http.Request) (*Package, error) {
	m := goArtifact.FindStringSubmatch(r.URL.Path)
	if m == nil {
		return nil, ErrNotPackageRequest
	}

	encodedModule := m[1] // e.g., "github.com/!burnt!sushi/toml"
	version := m[2]       // e.g., "v1.9.1"

	return &Package{
		Ecosystem:    "go",
		Name:         decodeCaseEncoding(encodedModule),
		Version:      version,
		Filename:     version + ".zip",
		UpstreamPath: encodedModule + "/@v/" + version + ".zip",
	}, nil
}

func (g *GoMod) UpstreamURL(pkg *Package) string {
	return fmt.Sprintf("%s/%s", g.UpstreamBase, pkg.UpstreamPath)
}

func (g *GoMod) RewriteResponse(_ context.Context, body []byte, _ *Package) ([]byte, error) {
	return body, nil
}

func (g *GoMod) IsMetadataRequest(r *http.Request) bool {
	return goMetadataPath.MatchString(r.URL.Path)
}

func (g *GoMod) MetadataUpstreamURL(r *http.Request) string {
	path := strings.TrimPrefix(r.URL.Path, "/go")
	return g.UpstreamBase + path
}

func (g *GoMod) RewriteMetadata(body []byte, _ string) ([]byte, error) {
	return body, nil
}

// decodeCaseEncoding converts a GOPROXY case-encoded module path to its original form.
// The GOPROXY protocol encodes each uppercase letter as '!' followed by the lowercase letter.
// For example, "github.com/!burnt!sushi/toml" decodes to "github.com/BurntSushi/toml".
func decodeCaseEncoding(module string) string {
	if !strings.ContainsRune(module, '!') {
		return module
	}
	var b strings.Builder
	b.Grow(len(module))
	i := 0
	for i < len(module) {
		if module[i] == '!' && i+1 < len(module) {
			b.WriteByte(module[i+1] - 32) // convert to uppercase
			i += 2
		} else {
			b.WriteByte(module[i])
			i++
		}
	}
	return b.String()
}
