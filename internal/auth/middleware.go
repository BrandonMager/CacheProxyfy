package auth

import (
	"crypto/subtle"
	"net/http"

	"golang.org/x/crypto/bcrypt"
)

// Config holds the authentication settings passed in from the top-level config.
type Config struct {
	Enabled bool
	Users   []User
}

// User represents a single set of credentials. PasswordHash must be a bcrypt hash.
type User struct {
	Username     string
	PasswordHash string
}

// Middleware enforces HTTP Basic Auth on all routes except /healthz, which is
// always exempt so Docker and Kubernetes health probes work without credentials.
// When Enabled is false every request passes through unchanged.
type Middleware struct {
	enabled   bool
	users     []User
	dummyHash []byte
}

// New builds a Middleware from cfg. If auth is enabled the dummy hash is
// generated once here so validate() can burn bcrypt time for unknown users
// and prevent timing-based username enumeration.
func New(cfg Config) *Middleware {
	m := &Middleware{
		enabled: cfg.Enabled,
		users:   cfg.Users,
	}
	if cfg.Enabled {
		hash, err := bcrypt.GenerateFromPassword([]byte("cacheproxyfy-dummy"), bcrypt.MinCost)
		if err != nil {
			panic("auth: generate dummy hash: " + err.Error())
		}
		m.dummyHash = hash
	}
	return m
}

// Wrap returns an http.Handler that enforces Basic Auth before delegating to
// next. If auth is disabled, next is returned unchanged.
func (m *Middleware) Wrap(next http.Handler) http.Handler {
	if !m.enabled {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}

		username, password, ok := r.BasicAuth()
		if !ok || !m.validate(username, password) {
			w.Header().Set("WWW-Authenticate", `Basic realm="CacheProxyfy"`)
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// validate checks username + password against the configured user list.
// It always runs a bcrypt comparison to prevent a timing oracle on whether a
// given username exists.
func (m *Middleware) validate(username, password string) bool {
	var target *User
	for i := range m.users {
		if subtle.ConstantTimeCompare([]byte(m.users[i].Username), []byte(username)) == 1 {
			target = &m.users[i]
			break
		}
	}

	if target == nil {
		bcrypt.CompareHashAndPassword(m.dummyHash, []byte(password)) //nolint:errcheck
		return false
	}

	return bcrypt.CompareHashAndPassword([]byte(target.PasswordHash), []byte(password)) == nil
}
