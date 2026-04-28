package security

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const osvURL = "https://api.osv.dev/v1/query"

// ecosystemMap maps proxy ecosystem names to OSV ecosystem names.
var ecosystemMap = map[string]string{
	"npm":   "npm",
	"pypi":  "PyPI",
	"maven": "Maven",
}

type Scanner struct {
	client *http.Client
}

func NewScanner() *Scanner {
	return &Scanner{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

type osvRequest struct {
	Package osvPackage `json:"package"`
	Version string     `json:"version"`
}

type osvPackage struct {
	Name      string `json:"name"`
	Ecosystem string `json:"ecosystem"`
}

type osvResponse struct {
	Vulns []osvVuln `json:"vulns"`
}

type osvVuln struct {
	ID         string         `json:"id"`
	Summary    string         `json:"summary"`
	DBSpecific map[string]any `json:"database_specific"`
}

// canonicalVersion strips wheel/artifact tags from a version string so OSV
// receives a plain PEP 440 version. For a PyPI wheel like "2.33.1-py3-none-any"
// the part after the first "-" is the python/abi/platform tag, not the version.
// PEP 440 versions never contain "-", so the first segment is always correct.
func canonicalVersion(ecosystem, version string) string {
	if strings.ToLower(ecosystem) == "pypi" {
		if idx := strings.Index(version, "-"); idx != -1 {
			return version[:idx]
		}
	}
	return version
}

// Scan queries the OSV API and returns CVE records for the given package.
func (s *Scanner) Scan(ctx context.Context, ecosystem, name, version string) ([]CVERecord, error) {
	osvEco, ok := ecosystemMap[strings.ToLower(ecosystem)]
	if !ok {
		osvEco = ecosystem
	}

	body, err := json.Marshal(osvRequest{
		Package: osvPackage{Name: name, Ecosystem: osvEco},
		Version: canonicalVersion(ecosystem, version),
	})
	if err != nil {
		return nil, fmt.Errorf("security: marshal osv request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, osvURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("security: build osv request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("security: osv request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("security: osv returned %d", resp.StatusCode)
	}

	var result osvResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("security: decode osv response: %w", err)
	}

	records := make([]CVERecord, 0, len(result.Vulns))
	for _, v := range result.Vulns {
		sev := SeverityUnknown
		if s, ok := v.DBSpecific["severity"].(string); ok {
			sev = ParseSeverity(s)
		}
		records = append(records, CVERecord{
			ID:       v.ID,
			Summary:  v.Summary,
			Severity: sev,
		})
	}

	return records, nil
}
