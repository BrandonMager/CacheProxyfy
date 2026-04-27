package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/BrandonMager/CacheProxyfy/internal/config"
	"github.com/BrandonMager/CacheProxyfy/internal/db"
)

// DBClient is the subset of db.DB methods the API layer needs.
// *db.DB satisfies this automatically — no glue code required.
type DBClient interface {
	GetStats(ctx context.Context, since time.Time) (db.Stats, error)
	GetPackage(ctx context.Context, ecosystem, name, version string) (db.Package, error)
	ListVersions(ctx context.Context, ecosystem, name string) ([]db.Package, error)
	ListPackages(ctx context.Context, ecosystem string) ([]db.Package, error)
	ListCVEAlerts(ctx context.Context, since time.Time, ecosystem string) ([]db.CVEAlert, error)
	ListPackageCVEAlerts(ctx context.Context, ecosystem, name, version string) ([]db.CVEAlert, error)
}

type Handler struct {
	db  DBClient
	cfg *config.Config
}

func NewHandler(db DBClient, cfg *config.Config) *Handler {
	return &Handler{db: db, cfg: cfg}
}

// RegisterRoutes mounts all API endpoints onto the provided mux.
// Call this on the internal (9090) mux so these endpoints are never public.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/stats", h.handleStats)
	mux.HandleFunc("/api/packages", h.handlePackages)
	mux.HandleFunc("/api/packages/list", h.handlePackageList)
	mux.HandleFunc("/api/packages/cve-alerts", h.handlePackageCVEAlerts)
	mux.HandleFunc("/api/cve-alerts", h.handleCVEAlerts)
	mux.HandleFunc("/api/config", h.handleConfig)
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

// handlePackageList handles GET /api/packages/list[?ecosystem=<eco>]
//
// Returns all cached packages, optionally filtered by ecosystem.
// No required params — omitting ecosystem returns packages across all ecosystems.
func (h *Handler) handlePackageList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ecosystem := r.URL.Query().Get("ecosystem")

	pkgs, err := h.db.ListPackages(r.Context(), ecosystem)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if pkgs == nil {
		pkgs = []db.Package{}
	}
	writeJSON(w, pkgs)
}

// handlePackageCVEAlerts handles GET /api/packages/cve-alerts?ecosystem=&name=&version=
//
// Returns all CVE alerts ever recorded for the given package version.
// Required query params: ecosystem, name, version
func (h *Handler) handlePackageCVEAlerts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	ecosystem := q.Get("ecosystem")
	name := q.Get("name")
	version := q.Get("version")

	if ecosystem == "" || name == "" || version == "" {
		http.Error(w, "ecosystem, name and version are required", http.StatusBadRequest)
		return
	}

	alerts, err := h.db.ListPackageCVEAlerts(r.Context(), ecosystem, name, version)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if alerts == nil {
		alerts = []db.CVEAlert{}
	}
	writeJSON(w, alerts)
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

// handleConfig handles GET /api/config
// Returns the active configuration with all secrets omitted.
func (h *Handler) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.cfg == nil {
		http.Error(w, "config unavailable", http.StatusInternalServerError)
		return
	}
	writeJSON(w, configResponse{
		Proxy: proxyConfigResponse{
			Port:       h.cfg.Proxy.Port,
			Ecosystems: h.cfg.Proxy.Ecosystems,
		},
		Cache: cacheConfigResponse{
			Backend:  h.cfg.Cache.Backend,
			LocalDir: h.cfg.Cache.LocalDir,
			TTLHours: h.cfg.Cache.TTLHours,
		},
		S3: s3ConfigResponse{
			Bucket:    h.cfg.S3.Bucket,
			Region:    h.cfg.S3.Region,
			Endpoint:  h.cfg.S3.Endpoint,
			KeyPrefix: h.cfg.S3.KeyPrefix,
		},
		Redis: redisConfigResponse{
			Addr: h.cfg.Redis.Addr,
			DB:   h.cfg.Redis.DB,
		},
		Database: databaseConfigResponse{
			Host:    h.cfg.Database.Host,
			Port:    h.cfg.Database.Port,
			User:    h.cfg.Database.User,
			DBName:  h.cfg.Database.DBName,
			SSLMode: h.cfg.Database.SSLMode,
		},
		Security: securityConfigResponse{
			CVEScanning:   h.cfg.Security.CVEScanning,
			BlockSeverity: h.cfg.Security.BlockSeverity,
			WarnSeverity:  h.cfg.Security.WarnSeverity,
		},
		Log: logConfigResponse{
			Level:  h.cfg.Log.Level,
			Format: h.cfg.Log.Format,
		},
	})
}

type configResponse struct {
	Proxy    proxyConfigResponse    `json:"proxy"`
	Cache    cacheConfigResponse    `json:"cache"`
	S3       s3ConfigResponse       `json:"s3"`
	Redis    redisConfigResponse    `json:"redis"`
	Database databaseConfigResponse `json:"database"`
	Security securityConfigResponse `json:"security"`
	Log      logConfigResponse      `json:"log"`
}

type proxyConfigResponse struct {
	Port       int      `json:"port"`
	Ecosystems []string `json:"ecosystems"`
}

type cacheConfigResponse struct {
	Backend  string `json:"backend"`
	LocalDir string `json:"local_dir"`
	TTLHours int    `json:"ttl_hours"`
}

type s3ConfigResponse struct {
	Bucket    string `json:"bucket"`
	Region    string `json:"region"`
	Endpoint  string `json:"endpoint"`
	KeyPrefix string `json:"key_prefix"`
}

type redisConfigResponse struct {
	Addr string `json:"addr"`
	DB   int    `json:"db"`
}

type databaseConfigResponse struct {
	Host    string `json:"host"`
	Port    int    `json:"port"`
	User    string `json:"user"`
	DBName  string `json:"dbname"`
	SSLMode string `json:"sslmode"`
}

type securityConfigResponse struct {
	CVEScanning   bool   `json:"cve_scanning"`
	BlockSeverity string `json:"block_severity"`
	WarnSeverity  string `json:"warn_severity"`
}

type logConfigResponse struct {
	Level  string `json:"level"`
	Format string `json:"format"`
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}
