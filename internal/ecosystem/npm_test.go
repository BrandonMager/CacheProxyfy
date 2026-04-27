package ecosystem

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestNPMParse(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		wantName    string
		wantVersion string
		wantFile    string
		wantErr     error
	}{
		{
			name:        "unscoped package",
			path:        "/npm/lodash/-/lodash-4.17.21.tgz",
			wantName:    "lodash",
			wantVersion: "4.17.21",
			wantFile:    "lodash-4.17.21.tgz",
		},
		{
			name:        "scoped package",
			path:        "/npm/@babel/core/-/core-7.0.0.tgz",
			wantName:    "@babel/core",
			wantVersion: "7.0.0",
			wantFile:    "core-7.0.0.tgz",
		},
		{
			name:    "metadata request, not a tarball",
			path:    "/npm/lodash",
			wantErr: ErrNotPackageRequest,
		},
		{
			name:    "missing separator",
			path:    "/npm/lodash/lodash-4.17.21.tgz",
			wantErr: ErrNotPackageRequest,
		},
	}

	n := NewNPM()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := http.NewRequest(http.MethodGet, tc.path, nil)
			pkg, err := n.Parse(r)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("expected error %v, got %v", tc.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
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
			if pkg.Ecosystem != "npm" {
				t.Errorf("expected ecosystem=npm, got %q", pkg.Ecosystem)
			}
		})
	}
}

func TestNPMUpstreamURL(t *testing.T) {
	n := NewNPM()
	pkg := &Package{Name: "lodash", Filename: "lodash-4.17.21.tgz"}
	got := n.UpstreamURL(pkg)
	want := "https://registry.npmjs.org/lodash/-/lodash-4.17.21.tgz"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestNPMIsMetadataRequest(t *testing.T) {
	n := NewNPM()
	cases := []struct {
		path string
		want bool
	}{
		{"/npm/lodash", true},
		{"/npm/@babel/core", true},
		{"/npm/lodash/-/lodash-4.17.21.tgz", false},
		{"/npm/@babel/core/-/core-7.0.0.tgz", false},
		{"/pypi/simple/requests/", false},
	}

	for _, tc := range cases {
		r, _ := http.NewRequest(http.MethodGet, tc.path, nil)
		if got := n.IsMetadataRequest(r); got != tc.want {
			t.Errorf("IsMetadataRequest(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestNPMMetadataUpstreamURL(t *testing.T) {
	n := NewNPM()
	cases := []struct {
		path string
		want string
	}{
		{"/npm/lodash", "https://registry.npmjs.org/lodash"},
		{"/npm/@babel/core", "https://registry.npmjs.org/@babel/core"},
	}

	for _, tc := range cases {
		r, _ := http.NewRequest(http.MethodGet, tc.path, nil)
		if got := n.MetadataUpstreamURL(r); got != tc.want {
			t.Errorf("MetadataUpstreamURL(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestNPMRewriteMetadata(t *testing.T) {
	n := NewNPM()
	proxyBase := "http://localhost:8080"

	body := []byte(`{
		"name": "lodash",
		"dist": {
			"tarball": "https://registry.npmjs.org/lodash/-/lodash-4.17.21.tgz"
		}
	}`)

	got, err := n.RewriteMetadata(body, proxyBase)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rewritten := string(got)
	if strings.Contains(rewritten, "https://registry.npmjs.org") {
		t.Error("expected registry.npmjs.org to be rewritten, but it still appears")
	}
	if !strings.Contains(rewritten, "http://localhost:8080/npm/lodash/-/lodash-4.17.21.tgz") {
		t.Errorf("expected proxy tarball URL, got: %s", rewritten)
	}
}
