package security

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// testOSVResponse mirrors the OSV API response shape used by Scanner.Scan.
type testOSVResponse struct {
	Vulns []testOSVVuln `json:"vulns"`
}

type testOSVVuln struct {
	ID         string         `json:"id"`
	Summary    string         `json:"summary"`
	DBSpecific map[string]any `json:"database_specific"`
	Severity   []osvSeverity  `json:"severity"`
}

// redirectTransport rewrites all outbound requests to the given target server,
// preserving method and body so the scanner hits the test server regardless of
// the hardcoded osvURL constant.
type redirectTransport struct {
	target *url.URL
}

func (t *redirectTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.URL.Scheme = t.target.Scheme
	r2.URL.Host = t.target.Host
	return http.DefaultTransport.RoundTrip(r2)
}

func newScannerWithServer(t *testing.T, body any) *Scanner {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)

	target, _ := url.Parse(srv.URL)
	s := NewScanner()
	s.client.Transport = &redirectTransport{target: target}
	return s
}

func TestScan_SkipsGOEntryWithNoSeverity_KeepsGHSAAlias(t *testing.T) {
	resp := testOSVResponse{
		Vulns: []testOSVVuln{
			{
				ID:         "GO-2021-0052",
				Summary:    "HTTP header injection in golang.org/x/net",
				DBSpecific: map[string]any{"url": "https://pkg.go.dev/vuln/GO-2021-0052", "review_status": "REVIEWED"},
			},
			{
				ID:         "GHSA-h395-qcrw-5vmq",
				Summary:    "HTTP header injection in golang.org/x/net",
				DBSpecific: map[string]any{"severity": "HIGH"},
			},
		},
	}

	s := newScannerWithServer(t, resp)
	records, err := s.Scan(context.Background(), "go", "golang.org/x/net", "0.0.0-20210520")
	if err != nil {
		t.Fatalf("Scan: unexpected error: %v", err)
	}

	for _, r := range records {
		if r.ID == "GO-2021-0052" {
			t.Errorf("GO-* entry with no severity must be skipped, but %q appears in records", r.ID)
		}
	}

	if len(records) != 1 || records[0].ID != "GHSA-h395-qcrw-5vmq" {
		t.Errorf("expected exactly [GHSA-h395-qcrw-5vmq], got %v", records)
	}

	if records[0].Severity != SeverityHigh {
		t.Errorf("GHSA alias severity: got %v, want HIGH", records[0].Severity)
	}
}

func TestScan_SkipsNoneSeverity(t *testing.T) {
	resp := testOSVResponse{
		Vulns: []testOSVVuln{
			{
				ID:         "GHSA-none-severity",
				Summary:    "should be skipped",
				DBSpecific: map[string]any{"severity": "NONE"},
			},
			{
				ID:         "GHSA-real-severity",
				Summary:    "should be kept",
				DBSpecific: map[string]any{"severity": "HIGH"},
			},
		},
	}

	s := newScannerWithServer(t, resp)
	records, err := s.Scan(context.Background(), "npm", "lodash", "4.17.15")
	if err != nil {
		t.Fatalf("Scan: unexpected error: %v", err)
	}

	for _, r := range records {
		if r.ID == "GHSA-none-severity" {
			t.Errorf("vuln with severity NONE must be skipped, but %q appears in records", r.ID)
		}
	}

	if len(records) != 1 || records[0].ID != "GHSA-real-severity" {
		t.Errorf("expected exactly [GHSA-real-severity], got %v", records)
	}
}
