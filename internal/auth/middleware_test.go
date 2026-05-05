package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func makeHash(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt.GenerateFromPassword: %v", err)
	}
	return string(hash)
}

var okHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func TestMiddleware_Disabled_AllowsAll(t *testing.T) {
	mw := New(Config{Enabled: false})
	req := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
	w := httptest.NewRecorder()
	mw.Wrap(okHandler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestMiddleware_NoCredentials_Returns401(t *testing.T) {
	mw := New(Config{
		Enabled: true,
		Users:   []User{{Username: "alice", PasswordHash: makeHash(t, "secret")}},
	})
	req := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
	w := httptest.NewRecorder()
	mw.Wrap(okHandler).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	if got := w.Header().Get("WWW-Authenticate"); got == "" {
		t.Error("expected WWW-Authenticate header on 401 response")
	}
}

func TestMiddleware_WrongPassword_Returns401(t *testing.T) {
	mw := New(Config{
		Enabled: true,
		Users:   []User{{Username: "alice", PasswordHash: makeHash(t, "secret")}},
	})
	req := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
	req.SetBasicAuth("alice", "wrong")
	w := httptest.NewRecorder()
	mw.Wrap(okHandler).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestMiddleware_UnknownUser_Returns401(t *testing.T) {
	mw := New(Config{
		Enabled: true,
		Users:   []User{{Username: "alice", PasswordHash: makeHash(t, "secret")}},
	})
	req := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
	req.SetBasicAuth("bob", "secret")
	w := httptest.NewRecorder()
	mw.Wrap(okHandler).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestMiddleware_ValidCredentials_AllowsThrough(t *testing.T) {
	mw := New(Config{
		Enabled: true,
		Users:   []User{{Username: "alice", PasswordHash: makeHash(t, "secret")}},
	})
	req := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
	req.SetBasicAuth("alice", "secret")
	w := httptest.NewRecorder()
	mw.Wrap(okHandler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestMiddleware_MultipleUsers(t *testing.T) {
	mw := New(Config{
		Enabled: true,
		Users: []User{
			{Username: "alice", PasswordHash: makeHash(t, "alice-secret")},
			{Username: "bob", PasswordHash: makeHash(t, "bob-secret")},
		},
	})
	handler := mw.Wrap(okHandler)

	cases := []struct {
		user, pass string
		want       int
	}{
		{"alice", "alice-secret", http.StatusOK},
		{"bob", "bob-secret", http.StatusOK},
		{"alice", "bob-secret", http.StatusUnauthorized},
		{"bob", "alice-secret", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/npm/lodash/-/lodash-4.17.21.tgz", nil)
		req.SetBasicAuth(tc.user, tc.pass)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Errorf("user=%q pass=%q: expected %d, got %d", tc.user, tc.pass, tc.want, w.Code)
		}
	}
}

func TestMiddleware_HealthzExempt(t *testing.T) {
	mw := New(Config{
		Enabled: true,
		Users:   []User{{Username: "alice", PasswordHash: makeHash(t, "secret")}},
	})
	// No credentials — /healthz must still return 200.
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	mw.Wrap(okHandler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("/healthz: expected 200, got %d", w.Code)
	}
}
