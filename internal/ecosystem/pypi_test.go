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
		{
			name:        "real pythonhosted hash path",
			path:        "/pypi/packages/5f/a4/98b9c7c6428a668bf7e42ebb7c79d576a1c3c1e3ae2d47e674b468388871/requests-2.33.1.tar.gz",
			wantName:    "requests",
			wantVersion: "2.33.1",
			wantFile:    "requests-2.33.1.tar.gz",
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
	tests := []struct {
		name         string
		upstreamPath string
		want         string
	}{
		{
			name:         "wheel file",
			upstreamPath: "ab/cd/requests/requests-2.28.0-py3-none-any.whl",
			want:         "https://files.pythonhosted.org/packages/ab/cd/requests/requests-2.28.0-py3-none-any.whl",
		},
		{
			name:         "sdist tar.gz",
			upstreamPath: "ab/cd/numpy/numpy-1.24.0.tar.gz",
			want:         "https://files.pythonhosted.org/packages/ab/cd/numpy/numpy-1.24.0.tar.gz",
		},
		{
			name:         "zip file",
			upstreamPath: "ab/cd/some-package/some_package-3.0.0.zip",
			want:         "https://files.pythonhosted.org/packages/ab/cd/some-package/some_package-3.0.0.zip",
		},
	}

	p := NewPyPI()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pkg := &Package{UpstreamPath: tc.upstreamPath}
			got := p.UpstreamURL(pkg)
			if got != tc.want {
				t.Errorf("expected %q, got %q", tc.want, got)
			}
		})
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

func TestPyPIIsMetadataRequest(t *testing.T) {
	p := NewPyPI()
	tests := []struct {
		path string
		want bool
	}{
		{"/pypi/simple/requests/", true},
		{"/pypi/simple/numpy/", true},
		{"/pypi/packages/ab/cd/requests/requests-2.31.0-py3-none-any.whl", false},
		{"/npm/lodash/-/lodash-4.17.21.tgz", false},
	}
	for _, tc := range tests {
		r, _ := http.NewRequest(http.MethodGet, tc.path, nil)
		if got := p.IsMetadataRequest(r); got != tc.want {
			t.Errorf("IsMetadataRequest(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestPyPIIsMetadataRequestWhlMetadata(t *testing.T) {
	p := NewPyPI()
	tests := []struct {
		path string
		want bool
	}{
		{"/pypi/packages/ab/cd/requests/requests-2.31.0-py3-none-any.whl.metadata", true},
		{"/pypi/packages/ab/cd/numpy/numpy-1.24.0-cp311-cp311-manylinux_2_17_x86_64.whl.metadata", true},
		{"/pypi/packages/ab/cd/requests/requests-2.31.0-py3-none-any.whl", false},
		{"/pypi/simple/requests/", true},
	}
	for _, tc := range tests {
		r, _ := http.NewRequest(http.MethodGet, tc.path, nil)
		if got := p.IsMetadataRequest(r); got != tc.want {
			t.Errorf("IsMetadataRequest(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestPyPIMetadataUpstreamURL(t *testing.T) {
	p := NewPyPI()
	tests := []struct {
		path string
		want string
	}{
		{"/pypi/simple/requests/", "https://pypi.org/simple/requests/"},
		{"/pypi/simple/numpy/", "https://pypi.org/simple/numpy/"},
	}
	for _, tc := range tests {
		r, _ := http.NewRequest(http.MethodGet, tc.path, nil)
		if got := p.MetadataUpstreamURL(r); got != tc.want {
			t.Errorf("MetadataUpstreamURL(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestPyPIMetadataUpstreamURLWhlMetadata(t *testing.T) {
	p := NewPyPI()
	tests := []struct {
		path string
		want string
	}{
		{
			path: "/pypi/packages/ab/cd/requests/requests-2.31.0-py3-none-any.whl.metadata",
			want: "https://files.pythonhosted.org/packages/ab/cd/requests/requests-2.31.0-py3-none-any.whl.metadata",
		},
		{
			path: "/pypi/packages/ab/cd/numpy/numpy-1.24.0-cp311-cp311-manylinux_2_17_x86_64.whl.metadata",
			want: "https://files.pythonhosted.org/packages/ab/cd/numpy/numpy-1.24.0-cp311-cp311-manylinux_2_17_x86_64.whl.metadata",
		},
	}
	for _, tc := range tests {
		r, _ := http.NewRequest(http.MethodGet, tc.path, nil)
		if got := p.MetadataUpstreamURL(r); got != tc.want {
			t.Errorf("MetadataUpstreamURL(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestPyPIRewriteMetadata(t *testing.T) {
	p := NewPyPI()
	input := `<a href="https://files.pythonhosted.org/packages/ab/cd/requests/requests-2.31.0-py3-none-any.whl#sha256=abc">requests-2.31.0</a>`
	want := `<a href="http://localhost:8080/pypi/packages/ab/cd/requests/requests-2.31.0-py3-none-any.whl#sha256=abc">requests-2.31.0</a>`

	got, err := p.RewriteMetadata([]byte(input), "http://localhost:8080")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != want {
		t.Errorf("RewriteMetadata mismatch:\ngot:  %s\nwant: %s", got, want)
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
