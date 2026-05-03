package ecosystem

import (
	"errors"
	"net/http"
	"testing"
)

func TestGoModParse(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		wantName     string
		wantVersion  string
		wantFile     string
		wantUpstream string
	}{
		{
			name:         "simple module",
			path:         "/go/github.com/gin-gonic/gin/@v/v1.9.1.zip",
			wantName:     "github.com/gin-gonic/gin",
			wantVersion:  "v1.9.1",
			wantFile:     "v1.9.1.zip",
			wantUpstream: "github.com/gin-gonic/gin/@v/v1.9.1.zip",
		},
		{
			name:         "case-encoded module",
			path:         "/go/github.com/!burnt!sushi/toml/@v/v1.3.2.zip",
			wantName:     "github.com/BurntSushi/toml",
			wantVersion:  "v1.3.2",
			wantFile:     "v1.3.2.zip",
			wantUpstream: "github.com/!burnt!sushi/toml/@v/v1.3.2.zip",
		},
		{
			name:         "stdlib-style module with major version suffix",
			path:         "/go/github.com/stretchr/testify/v2/@v/v2.0.0.zip",
			wantName:     "github.com/stretchr/testify/v2",
			wantVersion:  "v2.0.0",
			wantFile:     "v2.0.0.zip",
			wantUpstream: "github.com/stretchr/testify/v2/@v/v2.0.0.zip",
		},
		{
			name:         "golang.org module",
			path:         "/go/golang.org/x/net/@v/v0.20.0.zip",
			wantName:     "golang.org/x/net",
			wantVersion:  "v0.20.0",
			wantFile:     "v0.20.0.zip",
			wantUpstream: "golang.org/x/net/@v/v0.20.0.zip",
		},
	}

	g := NewGoMod()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := http.NewRequest(http.MethodGet, tc.path, nil)
			pkg, err := g.Parse(r)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pkg.Ecosystem != "go" {
				t.Errorf("expected ecosystem=%q, got %q", "go", pkg.Ecosystem)
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
			if pkg.UpstreamPath != tc.wantUpstream {
				t.Errorf("expected upstreamPath=%q, got %q", tc.wantUpstream, pkg.UpstreamPath)
			}
		})
	}
}

func TestGoModUpstreamURL(t *testing.T) {
	tests := []struct {
		name string
		pkg  *Package
		want string
	}{
		{
			name: "simple module",
			pkg: &Package{
				UpstreamPath: "github.com/gin-gonic/gin/@v/v1.9.1.zip",
			},
			want: "https://proxy.golang.org/github.com/gin-gonic/gin/@v/v1.9.1.zip",
		},
		{
			name: "case-encoded module preserves encoding",
			pkg: &Package{
				UpstreamPath: "github.com/!burnt!sushi/toml/@v/v1.3.2.zip",
			},
			want: "https://proxy.golang.org/github.com/!burnt!sushi/toml/@v/v1.3.2.zip",
		},
	}

	g := NewGoMod()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := g.UpstreamURL(tc.pkg)
			if got != tc.want {
				t.Errorf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestGoModParseErrors(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "info file", path: "/go/github.com/gin-gonic/gin/@v/v1.9.1.info"},
		{name: "mod file", path: "/go/github.com/gin-gonic/gin/@v/v1.9.1.mod"},
		{name: "version list", path: "/go/github.com/gin-gonic/gin/@v/list"},
		{name: "latest endpoint", path: "/go/github.com/gin-gonic/gin/@latest"},
		{name: "npm path", path: "/npm/lodash/-/lodash-4.17.21.tgz"},
		{name: "no version segment", path: "/go/github.com/gin-gonic/gin/v1.9.1.zip"},
	}

	g := NewGoMod()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := http.NewRequest(http.MethodGet, tc.path, nil)
			_, err := g.Parse(r)
			if !errors.Is(err, ErrNotPackageRequest) {
				t.Errorf("expected ErrNotPackageRequest, got %v", err)
			}
		})
	}
}

func TestGoModIsMetadataRequest(t *testing.T) {
	g := NewGoMod()
	cases := []struct {
		path string
		want bool
	}{
		{"/go/github.com/gin-gonic/gin/@v/list", true},
		{"/go/github.com/gin-gonic/gin/@v/v1.9.1.info", true},
		{"/go/github.com/gin-gonic/gin/@v/v1.9.1.mod", true},
		{"/go/github.com/gin-gonic/gin/@latest", true},
		{"/go/golang.org/x/net/@v/v0.20.0.info", true},
		{"/go/github.com/gin-gonic/gin/@v/v1.9.1.zip", false},
		{"/npm/lodash", false},
	}

	for _, tc := range cases {
		r, _ := http.NewRequest(http.MethodGet, tc.path, nil)
		if got := g.IsMetadataRequest(r); got != tc.want {
			t.Errorf("IsMetadataRequest(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestGoModMetadataUpstreamURL(t *testing.T) {
	g := NewGoMod()
	cases := []struct {
		path string
		want string
	}{
		{
			"/go/github.com/gin-gonic/gin/@v/list",
			"https://proxy.golang.org/github.com/gin-gonic/gin/@v/list",
		},
		{
			"/go/github.com/gin-gonic/gin/@v/v1.9.1.info",
			"https://proxy.golang.org/github.com/gin-gonic/gin/@v/v1.9.1.info",
		},
		{
			"/go/github.com/gin-gonic/gin/@v/v1.9.1.mod",
			"https://proxy.golang.org/github.com/gin-gonic/gin/@v/v1.9.1.mod",
		},
		{
			"/go/github.com/gin-gonic/gin/@latest",
			"https://proxy.golang.org/github.com/gin-gonic/gin/@latest",
		},
	}

	for _, tc := range cases {
		r, _ := http.NewRequest(http.MethodGet, tc.path, nil)
		if got := g.MetadataUpstreamURL(r); got != tc.want {
			t.Errorf("MetadataUpstreamURL(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestGoModRewriteMetadata(t *testing.T) {
	g := NewGoMod()
	body := []byte(`{"Version":"v1.9.1","Time":"2023-06-22T08:26:42Z"}`)
	got, err := g.RewriteMetadata(body, "http://localhost:8080")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("expected body unchanged, got %q", string(got))
	}
}

func TestDecodeCaseEncoding(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"github.com/gin-gonic/gin", "github.com/gin-gonic/gin"},
		{"github.com/!burnt!sushi/toml", "github.com/BurntSushi/toml"},
		{"github.com/!azure/azure-sdk-for-go", "github.com/Azure/azure-sdk-for-go"},
		{"github.com/!p-a-u-l-f/foo", "github.com/Paul-F/foo"},
	}

	for _, tc := range cases {
		got := decodeCaseEncoding(tc.input)
		if got != tc.want {
			t.Errorf("decodeCaseEncoding(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
