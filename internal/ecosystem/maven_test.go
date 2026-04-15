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
			name:        "pom file",
			path:        "/maven/org/springframework/spring-core/6.0.11/spring-core-6.0.11.pom",
			wantName:    "org.springframework:spring-core",
			wantVersion: "6.0.11",
			wantFile:    "spring-core-6.0.11.pom",
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
