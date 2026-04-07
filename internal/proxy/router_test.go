package proxy

import (
	"testing"
)

func TestRouterMatchEnabled(t *testing.T) {
	r := NewRouter([]string{"npm"})

	prefix, h, ok := r.Match("/npm/lodash/-/lodash-4.17.21.tgz")
	if !ok {
		t.Fatal("expected match for /npm/...")
	}
	if prefix != "npm" {
		t.Errorf("expected prefix=npm, got %q", prefix)
	}
	if h == nil {
		t.Error("expected non-nil handler")
	}
}

func TestRouterMatchDisabledEcosystem(t *testing.T) {
	r := NewRouter([]string{})

	_, _, ok := r.Match("/npm/lodash/-/lodash-4.17.21.tgz")
	if ok {
		t.Error("expected no match for disabled ecosystem")
	}
}

func TestRouterMatchUnknownEcosystem(t *testing.T) {
	r := NewRouter([]string{"npm"})

	_, _, ok := r.Match("/pypi/requests/-/requests-2.28.0.tar.gz")
	if ok {
		t.Error("expected no match for unknown ecosystem")
	}
}

func TestRouterMatchNoSlash(t *testing.T) {
	r := NewRouter([]string{"npm"})

	_, _, ok := r.Match("/npm")
	if ok {
		t.Error("expected no match for path with no second slash")
	}
}

func TestRouterMatchCaseInsensitive(t *testing.T) {
	r := NewRouter([]string{"NPM"})

	_, _, ok := r.Match("/npm/lodash/-/lodash-4.17.21.tgz")
	if !ok {
		t.Error("expected match when ecosystem registered as uppercase NPM")
	}
}

func TestRouterEcosystems(t *testing.T) {
	r := NewRouter([]string{"npm"})
	names := r.Ecosystems()
	if len(names) != 1 || names[0] != "npm" {
		t.Errorf("expected [npm], got %v", names)
	}
}
