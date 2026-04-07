package ecosystem

import (
	"errors"
	"net/http"
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
			// regex `@[^/]+[^/]+` does not cross the `/` in @scope/name
			name:    "scoped package (regex does not support @scope/name)",
			path:    "/npm/@babel/core/-/core-7.0.0.tgz",
			wantErr: ErrNotPackageRequest,
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
