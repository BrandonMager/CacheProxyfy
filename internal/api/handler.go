package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/BrandonMager/CacheProxyfy/internal/db"
)

// DBClient is the subset of db.DB methods the API layer needs.
// *db.DB satisfies this automatically — no glue code required.
type DBClient interface {
	GetStats(ctx context.Context, since time.Time) (db.Stats, error)
	GetPackage(ctx context.Context, ecosystem, name, version string) (db.Package, error)
	ListVersions(ctx context.Context, ecosystem, name string) ([]db.Package, error)
	ListCVEAlerts(ctx context.Context, since time.Time, ecosystem string) ([]db.CVEAlert, error)
}

type Handler struct {
	db DBClient
}

func NewHandler(db DBClient) *Handler {
	return &Handler{db: db}
}

// RegisterRoutes mounts all API endpoints onto the provided mux.
// Call this on the internal (9090) mux so these endpoints are never public.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/stats", h.handleStats)
	mux.HandleFunc("/api/packages", h.handlePackages)
	mux.HandleFunc("/api/cve-alerts", h.handleCVEAlerts)
}

// handleStats handles GET /api/stats?since=<duration>
// Returns aggregate cache statistics for the requested time window.
// The `since` param accepts any Go duration string (e.g. "24h", "7d", "1h").
// Defaults to 24h when omitted or invalid.
func (h *Handler) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	since := parseSince(r.URL.Query().Get("since"))

	stats, err := h.db.GetStats(r.Context(), since)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, stats)
}

// handlePackages handles GET /api/packages?ecosystem=&name=[&version=]
//
// With ecosystem+name only  → list all cached versions of that package.
// With ecosystem+name+version → return the single matching package record.
//
// Required query params: ecosystem, name
// Optional query param:  version
func (h *Handler) handlePackages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	ecosystem := q.Get("ecosystem")
	name := q.Get("name")

	if ecosystem == "" || name == "" {
		http.Error(w, "ecosystem and name are required", http.StatusBadRequest)
		return
	}

	version := q.Get("version")

	if version != "" {
		pkg, err := h.db.GetPackage(r.Context(), ecosystem, name, version)
		if errors.Is(err, db.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, pkg)
		return
	}

	pkgs, err := h.db.ListVersions(r.Context(), ecosystem, name)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if pkgs == nil {
		pkgs = []db.Package{}
	}
	writeJSON(w, pkgs)
}

// handleCVEAlerts handles GET /api/cve-alerts?since=<duration>[&ecosystem=<eco>]
//
// Returns CVE alerts recorded within the given time window.
// Optionally filtered by ecosystem. Defaults to the last 24h when since is omitted.
func (h *Handler) handleCVEAlerts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	since := parseSince(q.Get("since"))
	ecosystem := q.Get("ecosystem")

	alerts, err := h.db.ListCVEAlerts(r.Context(), since, ecosystem)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if alerts == nil {
		alerts = []db.CVEAlert{}
	}
	writeJSON(w, alerts)
}

// parseSince converts a duration string like "24h" or "168h" into a past time.Time.
// Falls back to 24 hours ago when the value is empty or unparseable.
func parseSince(s string) time.Time {
	if s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			return time.Now().Add(-d)
		}
	}
	return time.Now().Add(-24 * time.Hour)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}
