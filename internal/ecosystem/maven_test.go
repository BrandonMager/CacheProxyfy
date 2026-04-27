package ecosystem

import (
	"errors"
	"net/http"
	"testing"
)

func TestMavenParse(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		wantName    string
		wantVersion string
		wantFile    string
	}{
		{
			name:        "jar with classifier in version",
			path:        "/maven/com/google/guava/guava/32.1.2-jre/guava-32.1.2-jre.jar",
			wantName:    "com.google.guava:guava",
			wantVersion: "32.1.2-jre",
			wantFile:    "guava-32.1.2-jre.jar",
		},
		{
			name:        "multi-segment groupId",
			path:        "/maven/org/apache/commons/commons-lang3/3.13.0/commons-lang3-3.13.0.jar",
			wantName:    "org.apache.commons:commons-lang3",
			wantVersion: "3.13.0",
			wantFile:    "commons-lang3-3.13.0.jar",
		},
	}

	m := NewMaven()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := http.NewRequest(http.MethodGet, tc.path, nil)
			pkg, err := m.Parse(r)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pkg.Ecosystem != "maven" {
				t.Errorf("expected ecosystem=%q, got %q", "maven", pkg.Ecosystem)
			}
			if pkg.Name != tc.wantName {
				t.Errorf("expected name=%q, got %q", tc.wantName, pkg.Name)
			}
			if pkg.Version != tc.wantVersion {
				t.Errorf("expected version=%q, got %q", tc.wantVersion, pkg.Version)
			}
			if pkg.Filename != tc.wantFile {
				t.Errorf("expected filename=%q, got %q", tc.wantFile, pkg.Filename)
			}
		})
	}
}

func TestMavenUpstreamURL(t *testing.T) {
	tests := []struct {
		name string
		pkg  *Package
		want string
	}{
		{
			name: "two-segment groupId",
			pkg: &Package{
				Name:     "com.google.guava:guava",
				Version:  "32.1.2-jre",
				Filename: "guava-32.1.2-jre.jar",
			},
			want: "https://repo1.maven.org/maven2/com/google/guava/guava/32.1.2-jre/guava-32.1.2-jre.jar",
		},
		{
			name: "three-segment groupId",
			pkg: &Package{
				Name:     "org.apache.commons:commons-lang3",
				Version:  "3.13.0",
				Filename: "commons-lang3-3.13.0.jar",
			},
			want: "https://repo1.maven.org/maven2/org/apache/commons/commons-lang3/3.13.0/commons-lang3-3.13.0.jar",
		},
	}

	m := NewMaven()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := m.UpstreamURL(tc.pkg)
			if got != tc.want {
				t.Errorf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestMavenParseErrors(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{
			name: "md5 checksum file",
			path: "/maven/com/google/guava/guava/32.1.2-jre/guava-32.1.2-jre.jar.md5",
		},
		{
			name: "sha1 checksum file",
			path: "/maven/com/google/guava/guava/32.1.2-jre/guava-32.1.2-jre.jar.sha1",
		},
		{
			name: "maven metadata file",
			path: "/maven/com/google/guava/guava/maven-metadata.xml",
		},
		{
			name: "pom file",
			path: "/maven/org/springframework/spring-core/6.0.11/spring-core-6.0.11.pom",
		},
		{
			name: "sha256 checksum file",
			path: "/maven/com/google/guava/guava/32.1.2-jre/guava-32.1.2-jre.jar.sha256",
		},
		{
			name: "unsupported extension",
			path: "/maven/com/google/guava/guava/32.1.2-jre/guava-32.1.2-jre.zip",
		},
		{
			name: "missing version segment",
			path: "/maven/com/google/guava/guava/guava-32.1.2-jre.jar",
		},
	}

	m := NewMaven()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := http.NewRequest(http.MethodGet, tc.path, nil)
			_, err := m.Parse(r)
			if !errors.Is(err, ErrNotPackageRequest) {
				t.Errorf("expected ErrNotPackageRequest, got %v", err)
			}
		})
	}
}

func TestMavenIsMetadataRequest(t *testing.T) {
	m := NewMaven()
	cases := []struct {
		path string
		want bool
	}{
		{"/maven/com/google/guava/guava/maven-metadata.xml", true},
		{"/maven/org/springframework/spring-core/6.0.11/spring-core-6.0.11.pom", true},
		{"/maven/com/google/guava/guava/32.1.2-jre/guava-32.1.2-jre.jar.md5", true},
		{"/maven/com/google/guava/guava/32.1.2-jre/guava-32.1.2-jre.jar.sha1", true},
		{"/maven/com/google/guava/guava/32.1.2-jre/guava-32.1.2-jre.jar.sha256", true},
		{"/maven/com/google/guava/guava/32.1.2-jre/guava-32.1.2-jre.jar", false},
		{"/npm/lodash", false},
	}

	for _, tc := range cases {
		r, _ := http.NewRequest(http.MethodGet, tc.path, nil)
		if got := m.IsMetadataRequest(r); got != tc.want {
			t.Errorf("IsMetadataRequest(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestMavenMetadataUpstreamURL(t *testing.T) {
	m := NewMaven()
	cases := []struct {
		path string
		want string
	}{
		{
			"/maven/com/google/guava/guava/maven-metadata.xml",
			"https://repo1.maven.org/maven2/com/google/guava/guava/maven-metadata.xml",
		},
		{
			"/maven/org/springframework/spring-core/6.0.11/spring-core-6.0.11.pom",
			"https://repo1.maven.org/maven2/org/springframework/spring-core/6.0.11/spring-core-6.0.11.pom",
		},
		{
			"/maven/com/google/guava/guava/32.1.2-jre/guava-32.1.2-jre.jar.sha1",
			"https://repo1.maven.org/maven2/com/google/guava/guava/32.1.2-jre/guava-32.1.2-jre.jar.sha1",
		},
	}

	for _, tc := range cases {
		r, _ := http.NewRequest(http.MethodGet, tc.path, nil)
		if got := m.MetadataUpstreamURL(r); got != tc.want {
			t.Errorf("MetadataUpstreamURL(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestMavenRewriteMetadata(t *testing.T) {
	m := NewMaven()
	body := []byte(`<?xml version="1.0"?><metadata><groupId>com.google.guava</groupId></metadata>`)
	got, err := m.RewriteMetadata(body, "http://localhost:8080")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("expected body unchanged, got %q", string(got))
	}
}
