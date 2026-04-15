package ecosystem

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

/* maven-metadata.xml is not included */
var mavenArtifact = regexp.MustCompile(
	`^/maven/(.+)/([^/]+)/([^/]+)/(\2-\3(?:-[^.]+)?\.(?:jar|pom))$`,
)

type Maven struct {
	UpstreamBase string
}

func NewMaven() *Maven {
	return &Maven{UpstreamBase: "https://repo1.maven.org/maven2"}
}

func (m *Maven) Parse(r *http.Request) (*Package, error) {
	path := r.URL.Path
	if strings.HasSuffix(path, ".md5") || strings.HasSuffix(path, ".sha1") || strings.Contains(path, "maven-metadata") {
		return nil, ErrNotPackageRequest
	}

	matches := mavenArtifact.FindStringSubmatch(path)
	if matches == nil {
		return nil, ErrNotPackageRequest
	}

	groupID := strings.ReplaceAll(matches[1], "/", ".") // change to URL form
	artifactID := matches[2]
	version := matches[3]
	filename := matches[4]

	name := fmt.Sprintf("%s:%s", groupID, artifactID)

	return &Package{
		Ecosystem: "maven",
		Name: name,
		Version: version,
		Filename: filename,
	}
}

func (m *Maven) UpstreamURL(pkg *Package) string {
	parts := strings.SplitN(pkg.Name, ":", 2)
	groupPath := strings.ReplaceAll(parts[0], ".", "/")
	artifactID := parts[1]

	return fmt.Sprintf("%s/%s/%s/%s/%s",
		m.UpstreamBase,
		groupPath,
		artifactID,
		pkg.Version,
		pkg.Filename,
	)
}

func (m *Maven) RewriteResponse(_ context.Context, body []byte, _ *Package) ([]byte, error){
	return body, nil
}