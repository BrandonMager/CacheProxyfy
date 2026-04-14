package ecosystem

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestPyPIParse(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		wantName    string
		wantVersion string
		wantFile    string
	}{
		{
			name:        "wheel file",
			path:        "/pypi/packages/ab/cd/requests/requests-2.28.0-py3-none-any.whl",
			wantName:    "requests",
			wantVersion: "2.28.0-py3-none-any",
			wantFile:    "requests-2.28.0-py3-none-any.whl",
		},
		{
			name:        "sdist tar.gz file",
			path:        "/pypi/packages/ab/cd/numpy/numpy-1.24.0.tar.gz",
			wantName:    "numpy",
			wantVersion: "1.24.0",
			wantFile:    "numpy-1.24.0.tar.gz",
		},
		{
			name:        "zip file",
			path:        "/pypi/packages/ab/cd/some-package/some_package-3.0.0.zip",
			wantName:    "some-package",
			wantVersion: "3.0.0",
			wantFile:    "some_package-3.0.0.zip",
		},
	}

	p := NewPyPI()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := http.NewRequest(http.MethodGet, tc.path, nil)
			pkg, err := p.Parse(r)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pkg.Ecosystem != "pypi" {
				t.Errorf("expected ecosystem=%q, got %q", "pypi", pkg.Ecosystem)
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

func TestPyPIParseErrors(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{
			name: "simple index path",
			path: "/pypi/simple/requests/",
		},
		{
			name: "metadata request, not an artifact",
			path: "/pypi/packages/ab/cd/requests/",
		},
		{
			name: "unsupported extension",
			path: "/pypi/packages/ab/cd/requests/requests-2.28.0.egg",
		},
		{
			name: "missing version segment",
			path: "/pypi/packages/ab/cd/requests/requests.whl",
		},
		{
			name: "root path",
			path: "/",
		},
		{
			name: "unrelated path",
			path: "/health",
		},
		{
			name: "wrong ecosystem prefix",
			path: "/npm/packages/ab/cd/requests/requests-2.28.0-py3-none-any.whl",
		},
	}

	p := NewPyPI()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := http.NewRequest(http.MethodGet, tc.path, nil)
			_, err := p.Parse(r)
			if !errors.Is(err, ErrNotPackageRequest) {
				t.Errorf("expected ErrNotPackageRequest, got %v", err)
			}
		})
	}
}

func TestPyPIUpstreamURL(t *testing.T) {
	p := NewPyPI()
	pkg := &Package{Filename: "requests-2.28.0-py3-none-any.whl"}
	got := p.UpstreamURL(pkg)
	want := "https://files.pythonhosted.org/packages/requests-2.28.0-py3-none-any.whl"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestPyPIRewriteResponse(t *testing.T) {
	p := NewPyPI()
	body := []byte("package contents")
	got, err := p.RewriteResponse(context.Background(), body, &Package{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("expected body to be unchanged, got %q", got)
	}
}

func TestNormalise(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"requests", "requests"},
		{"Pillow", "pillow"},
		{"my_package", "my-package"},
		{"My.Package", "my-package"},
		{"My_Cool.Package", "my-cool-package"},
	}

	for _, tc := range tests {
		got := normalise(tc.input)
		if got != tc.want {
			t.Errorf("normalise(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
