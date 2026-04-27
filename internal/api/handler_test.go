package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BrandonMager/CacheProxyfy/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubDB is a test double for DBClient.
type stubDB struct {
	stats         db.Stats
	pkg           db.Package
	pkgs          []db.Package
	summaries     []db.PackageSummary
	alerts        []db.CVEAlert
	total         int
	err           error
	capturedSince time.Time
}

func (s *stubDB) GetStats(ctx context.Context, since time.Time) (db.Stats, error) {
	s.capturedSince = since
	return s.stats, s.err
}

func (s *stubDB) GetPackage(_ context.Context, _, _, _ string) (db.Package, error) {
	return s.pkg, s.err
}

func (s *stubDB) ListVersions(_ context.Context, _, _ string, _, _ int) ([]db.Package, error) {
	return s.pkgs, s.err
}

func (s *stubDB) ListPackages(_ context.Context, _ string) ([]db.Package, error) {
	return s.pkgs, s.err
}

func (s *stubDB) ListPackageSummaries(_ context.Context, _ string, _, _ int) ([]db.PackageSummary, error) {
	return s.summaries, s.err
}

func (s *stubDB) CountPackageSummaries(_ context.Context, _ string) (int, error) {
	return s.total, s.err
}

func (s *stubDB) CountVersions(_ context.Context, _, _ string) (int, error) {
	return s.total, s.err
}

func (s *stubDB) ListCVEAlerts(_ context.Context, since time.Time, _ string) ([]db.CVEAlert, error) {
	s.capturedSince = since
	return s.alerts, s.err
}

func (s *stubDB) ListPackageCVEAlerts(_ context.Context, _, _, _ string) ([]db.CVEAlert, error) {
	return s.alerts, s.err
}

// helpers

func newMux(stub *stubDB) *http.ServeMux {
	mux := http.NewServeMux()
	NewHandler(stub, nil).RegisterRoutes(mux)
	return mux
}

// ── /api/stats ────────────────────────────────────────────────────────────────

func TestHandleStats_DefaultWindow(t *testing.T) {
	stub := &stubDB{
		stats: db.Stats{
			TotalPackages: 10,
			TotalHits:     80,
			TotalMisses:   20,
			BytesSaved:    1024,
			HitRate:       0.8,
		},
	}

	before := time.Now()
	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	w := httptest.NewRecorder()
	newMux(stub).ServeHTTP(w, req)
	after := time.Now()

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var got db.Stats
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, stub.stats, got)

	expectedMin := before.Add(-24*time.Hour - time.Second)
	expectedMax := after.Add(-24*time.Hour + time.Second)
	assert.WithinRange(t, stub.capturedSince, expectedMin, expectedMax)
}

func TestHandleStats_CustomWindow(t *testing.T) {
	stub := &stubDB{stats: db.Stats{TotalHits: 5}}

	before := time.Now()
	req := httptest.NewRequest(http.MethodGet, "/api/stats?since=1h", nil)
	w := httptest.NewRecorder()
	newMux(stub).ServeHTTP(w, req)
	after := time.Now()

	require.Equal(t, http.StatusOK, w.Code)

	expectedMin := before.Add(-time.Hour - time.Second)
	expectedMax := after.Add(-time.Hour + time.Second)
	assert.WithinRange(t, stub.capturedSince, expectedMin, expectedMax)
}

func TestHandleStats_InvalidSinceFallsBackTo24h(t *testing.T) {
	stub := &stubDB{stats: db.Stats{}}

	before := time.Now()
	req := httptest.NewRequest(http.MethodGet, "/api/stats?since=badvalue", nil)
	w := httptest.NewRecorder()
	newMux(stub).ServeHTTP(w, req)
	after := time.Now()

	require.Equal(t, http.StatusOK, w.Code)

	expectedMin := before.Add(-24*time.Hour - time.Second)
	expectedMax := after.Add(-24*time.Hour + time.Second)
	assert.WithinRange(t, stub.capturedSince, expectedMin, expectedMax)
}

func TestHandleStats_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/stats", nil)
	w := httptest.NewRecorder()
	newMux(&stubDB{}).ServeHTTP(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// ── /api/packages ─────────────────────────────────────────────────────────────

func TestHandlePackages_ListVersions(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	stub := &stubDB{
		total: 2,
		pkgs: []db.Package{
			{ID: 1, Ecosystem: "npm", Name: "react", Version: "18.0.0", SizeBytes: 512, CachedAt: now},
			{ID: 2, Ecosystem: "npm", Name: "react", Version: "17.0.2", SizeBytes: 480, CachedAt: now},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/packages?ecosystem=npm&name=react", nil)
	w := httptest.NewRecorder()
	newMux(stub).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var got paginatedResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, 2, got.Total)
	assert.Equal(t, 1, got.Page)
	items, ok := got.Items.([]interface{})
	require.True(t, ok)
	assert.Len(t, items, 2)
}

func TestHandlePackages_ListVersions_AllMetadataFields(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	lastHit := now.Add(-time.Hour)
	stub := &stubDB{
		total: 2,
		pkgs: []db.Package{
			{
				ID:        10,
				Ecosystem: "pypi",
				Name:      "requests",
				Version:   "2.31.0",
				Checksum:  "sha256:abc123",
				SizeBytes: 131072,
				CachedAt:  now,
				LastHitAt: &lastHit,
			},
			{
				ID:        11,
				Ecosystem: "pypi",
				Name:      "requests",
				Version:   "2.28.0",
				Checksum:  "sha256:def456",
				SizeBytes: 128000,
				CachedAt:  now.Add(-24 * time.Hour),
				LastHitAt: nil,
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/packages?ecosystem=pypi&name=requests", nil)
	w := httptest.NewRecorder()
	newMux(stub).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var got paginatedResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, 2, got.Total)
	items, ok := got.Items.([]interface{})
	require.True(t, ok)
	require.Len(t, items, 2)
}

func TestHandlePackages_ListVersions_Empty(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/packages?ecosystem=npm&name=unknown", nil)
	w := httptest.NewRecorder()
	newMux(&stubDB{}).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var got paginatedResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, 0, got.Total)
	items, ok := got.Items.([]interface{})
	require.True(t, ok)
	assert.Empty(t, items)
}

func TestHandlePackages_GetPackage(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	stub := &stubDB{
		pkg: db.Package{ID: 1, Ecosystem: "npm", Name: "react", Version: "18.0.0", SizeBytes: 512, CachedAt: now},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/packages?ecosystem=npm&name=react&version=18.0.0", nil)
	w := httptest.NewRecorder()
	newMux(stub).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var got db.Package
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, "18.0.0", got.Version)
}

func TestHandlePackages_GetPackage_NotFound(t *testing.T) {
	stub := &stubDB{err: db.ErrNotFound}

	req := httptest.NewRequest(http.MethodGet, "/api/packages?ecosystem=npm&name=react&version=99.0.0", nil)
	w := httptest.NewRecorder()
	newMux(stub).ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandlePackages_MissingParams(t *testing.T) {
	cases := []string{
		"/api/packages",
		"/api/packages?ecosystem=npm",
		"/api/packages?name=react",
	}
	for _, path := range cases {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		newMux(&stubDB{}).ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code, "path: %s", path)
	}
}

func TestHandlePackages_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/packages?ecosystem=npm&name=react", nil)
	w := httptest.NewRecorder()
	newMux(&stubDB{}).ServeHTTP(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHandlePackages_DBError(t *testing.T) {
	stub := &stubDB{err: errors.New("connection reset")}

	req := httptest.NewRequest(http.MethodGet, "/api/packages?ecosystem=npm&name=react", nil)
	w := httptest.NewRecorder()
	newMux(stub).ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ── /api/packages/list ────────────────────────────────────────────────────────

func TestHandlePackageList_AllEcosystems(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	stub := &stubDB{
		pkgs: []db.Package{
			{ID: 1, Ecosystem: "npm", Name: "lodash", Version: "4.17.21", SizeBytes: 256, CachedAt: now},
			{ID: 2, Ecosystem: "pypi", Name: "requests", Version: "2.31.0", SizeBytes: 128, CachedAt: now},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/packages/list", nil)
	w := httptest.NewRecorder()
	newMux(stub).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var got []db.Package
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Len(t, got, 2)
}

func TestHandlePackageList_FilteredByEcosystem(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	stub := &stubDB{
		pkgs: []db.Package{
			{ID: 1, Ecosystem: "npm", Name: "lodash", Version: "4.17.21", SizeBytes: 256, CachedAt: now},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/packages/list?ecosystem=npm", nil)
	w := httptest.NewRecorder()
	newMux(stub).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var got []db.Package
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Len(t, got, 1)
	assert.Equal(t, "npm", got[0].Ecosystem)
}

func TestHandlePackageList_Empty(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/packages/list", nil)
	w := httptest.NewRecorder()
	newMux(&stubDB{}).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var got []db.Package
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Empty(t, got)
}

func TestHandlePackageList_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/packages/list", nil)
	w := httptest.NewRecorder()
	newMux(&stubDB{}).ServeHTTP(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHandlePackageList_DBError(t *testing.T) {
	stub := &stubDB{err: errors.New("connection reset")}

	req := httptest.NewRequest(http.MethodGet, "/api/packages/list", nil)
	w := httptest.NewRecorder()
	newMux(stub).ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ── /api/cve-alerts ───────────────────────────────────────────────────────────

func TestHandleCVEAlerts_ReturnsAlerts(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	stub := &stubDB{
		alerts: []db.CVEAlert{
			{ID: 1, Ecosystem: "npm", Name: "lodash", Version: "4.17.15", CVEID: "CVE-2021-23337", Severity: "high", Outcome: "blocked", RecordedAt: now},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/cve-alerts?since=24h", nil)
	w := httptest.NewRecorder()
	newMux(stub).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var got []db.CVEAlert
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Len(t, got, 1)
	assert.Equal(t, "CVE-2021-23337", got[0].CVEID)
}

func TestHandleCVEAlerts_Empty(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/cve-alerts", nil)
	w := httptest.NewRecorder()
	newMux(&stubDB{}).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var got []db.CVEAlert
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Empty(t, got)
}

func TestHandleCVEAlerts_DefaultWindow(t *testing.T) {
	stub := &stubDB{}

	before := time.Now()
	req := httptest.NewRequest(http.MethodGet, "/api/cve-alerts", nil)
	w := httptest.NewRecorder()
	newMux(stub).ServeHTTP(w, req)
	after := time.Now()

	require.Equal(t, http.StatusOK, w.Code)

	expectedMin := before.Add(-24*time.Hour - time.Second)
	expectedMax := after.Add(-24*time.Hour + time.Second)
	assert.WithinRange(t, stub.capturedSince, expectedMin, expectedMax)
}

func TestHandleCVEAlerts_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodDelete, "/api/cve-alerts", nil)
	w := httptest.NewRecorder()
	newMux(&stubDB{}).ServeHTTP(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHandleCVEAlerts_DBError(t *testing.T) {
	stub := &stubDB{err: errors.New("timeout")}

	req := httptest.NewRequest(http.MethodGet, "/api/cve-alerts", nil)
	w := httptest.NewRecorder()
	newMux(stub).ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ── /api/packages/summaries ───────────────────────────────────────────────────

func TestHandlePackageSummaries_ReturnsSummaries(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	lastHit := now.Add(-time.Hour)
	stub := &stubDB{
		total: 2,
		summaries: []db.PackageSummary{
			{Ecosystem: "pypi", Name: "requests", LatestVersion: "2.31.0", VersionCount: 3, TotalSizeBytes: 393216, LastCachedAt: now, LastHitAt: &lastHit},
			{Ecosystem: "npm", Name: "lodash", LatestVersion: "4.17.21", VersionCount: 1, TotalSizeBytes: 262144, LastCachedAt: now, LastHitAt: nil},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/packages/summaries", nil)
	w := httptest.NewRecorder()
	newMux(stub).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var got paginatedResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, 2, got.Total)
	assert.Equal(t, 1, got.Page)
	items, ok := got.Items.([]interface{})
	require.True(t, ok)
	assert.Len(t, items, 2)
}

func TestHandlePackageSummaries_Empty(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/packages/summaries", nil)
	w := httptest.NewRecorder()
	newMux(&stubDB{}).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var got paginatedResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, 0, got.Total)
	items, ok := got.Items.([]interface{})
	require.True(t, ok)
	assert.Empty(t, items)
}

func TestHandlePackageSummaries_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/packages/summaries", nil)
	w := httptest.NewRecorder()
	newMux(&stubDB{}).ServeHTTP(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHandlePackageSummaries_DBError(t *testing.T) {
	stub := &stubDB{err: errors.New("connection reset")}

	req := httptest.NewRequest(http.MethodGet, "/api/packages/summaries", nil)
	w := httptest.NewRecorder()
	newMux(stub).ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ── /api/packages/cve-alerts ──────────────────────────────────────────────────

func TestHandlePackageCVEAlerts_ReturnsAlerts(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	stub := &stubDB{
		alerts: []db.CVEAlert{
			{ID: 1, Ecosystem: "pypi", Name: "requests", Version: "2.28.0", CVEID: "CVE-2023-32681", Severity: "MEDIUM", Outcome: "warn", RecordedAt: now},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/packages/cve-alerts?ecosystem=pypi&name=requests&version=2.28.0", nil)
	w := httptest.NewRecorder()
	newMux(stub).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var got []db.CVEAlert
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	require.Len(t, got, 1)
	assert.Equal(t, "CVE-2023-32681", got[0].CVEID)
	assert.Equal(t, "MEDIUM", got[0].Severity)
}

func TestHandlePackageCVEAlerts_Empty(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/packages/cve-alerts?ecosystem=pypi&name=requests&version=2.31.0", nil)
	w := httptest.NewRecorder()
	newMux(&stubDB{}).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var got []db.CVEAlert
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Empty(t, got)
}

func TestHandlePackageCVEAlerts_MissingParams(t *testing.T) {
	cases := []string{
		"/api/packages/cve-alerts",
		"/api/packages/cve-alerts?ecosystem=pypi",
		"/api/packages/cve-alerts?ecosystem=pypi&name=requests",
	}
	for _, path := range cases {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		newMux(&stubDB{}).ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code, "path: %s", path)
	}
}

func TestHandlePackageCVEAlerts_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/packages/cve-alerts?ecosystem=pypi&name=requests&version=2.28.0", nil)
	w := httptest.NewRecorder()
	newMux(&stubDB{}).ServeHTTP(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHandlePackageCVEAlerts_DBError(t *testing.T) {
	stub := &stubDB{err: errors.New("connection reset")}

	req := httptest.NewRequest(http.MethodGet, "/api/packages/cve-alerts?ecosystem=pypi&name=requests&version=2.28.0", nil)
	w := httptest.NewRecorder()
	newMux(stub).ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
