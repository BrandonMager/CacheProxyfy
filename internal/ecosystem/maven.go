package ecosystem

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

var mavenArtifact = regexp.MustCompile(
	`^/maven/(.+)/([^/]+)/([^/]+)/([^/]+\.jar)$`,
)

// mavenMetadataPath matches maven-metadata.xml, .pom, .md5, and .sha1 requests
// — all are proxied transparently without caching as artifacts.
var mavenMetadataPath = regexp.MustCompile(
	`^/maven/.+(/maven-metadata\.xml|\.pom|\.md5|\.sha1|\.sha256)$`,
)

type Maven struct {
	UpstreamBase string
}

func NewMaven() *Maven {
	return &Maven{UpstreamBase: "https://repo1.maven.org/maven2"}
}

func (m *Maven) Parse(r *http.Request) (*Package, error) {
	path := r.URL.Path
	if mavenMetadataPath.MatchString(path) {
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

	if !strings.HasPrefix(filename, artifactID+"-"+version) {
		return nil, ErrNotPackageRequest
	}

	name := fmt.Sprintf("%s:%s", groupID, artifactID)

	return &Package{
		Ecosystem: "maven",
		Name:      name,
		Version:   version,
		Filename:  filename,
	}, nil
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

func (m *Maven) RewriteResponse(_ context.Context, body []byte, _ *Package) ([]byte, error) {
	return body, nil
}

// IsMetadataRequest reports whether r is a Maven metadata request:
// maven-metadata.xml, .pom, or checksum files (.md5, .sha1, .sha256).
func (m *Maven) IsMetadataRequest(r *http.Request) bool {
	return mavenMetadataPath.MatchString(r.URL.Path)
}

// MetadataUpstreamURL returns the upstream Maven Central URL for the metadata request.
func (m *Maven) MetadataUpstreamURL(r *http.Request) string {
	path := strings.TrimPrefix(r.URL.Path, "/maven")
	return m.UpstreamBase + path
}

// RewriteMetadata is a no-op for Maven — metadata files (XML, POM, checksums)
// do not contain URLs that need rewriting.
func (m *Maven) RewriteMetadata(body []byte, _ string) ([]byte, error) {
	return body, nil
}