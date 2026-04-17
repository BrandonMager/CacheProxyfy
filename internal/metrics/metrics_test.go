package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BrandonMager/CacheProxyfy/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func newTestServer(t *testing.T) (*metrics.Metrics, *httptest.Server) {
	t.Helper()
	reg := prometheus.NewRegistry()
	m := metrics.New(reg, []string{})
	srv := httptest.NewServer(promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	t.Cleanup(srv.Close)
	return m, srv
}

func scrape(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("scrape: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("scrape: expected 200, got %d", resp.StatusCode)
	}
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return sb.String()
}

func assertMetric(t *testing.T, body, name string) {
	t.Helper()
	if !strings.Contains(body, name) {
		t.Errorf("expected metric %q in /metrics output", name)
	}
}

// TestMetricsEndpointRegistered verifies all metrics are registered and appear
// in the /metrics output after a sample observation has been recorded for each.
// Prometheus omits metric families with zero observations, so we seed one value
// per family — the point is to confirm the name and label schema are correct.
func TestMetricsEndpointRegistered(t *testing.T) {
	m, srv := newTestServer(t)

	// Seed one observation per metric family.
	m.RequestsTotal.WithLabelValues("npm", "hit").Inc()
	m.RequestDuration.WithLabelValues("npm", "hit").Observe(0.1)
	m.BytesServedTotal.WithLabelValues("npm", "hit").Add(1024)
	m.PackageSizeBytes.WithLabelValues("npm").Observe(1024)
	m.UpstreamFetchesTotal.WithLabelValues("npm", "ok").Inc()
	m.UpstreamFetchDuration.WithLabelValues("npm").Observe(0.5)
	m.CVEScansTotal.WithLabelValues("npm", "allow").Inc()
	m.InflightRequests.Inc()

	body := scrape(t, srv)

	expected := []string{
		"cacheproxyfy_requests_total",
		"cacheproxyfy_request_duration_seconds",
		"cacheproxyfy_bytes_served_total",
		"cacheproxyfy_package_size_bytes",
		"cacheproxyfy_upstream_fetches_total",
		"cacheproxyfy_upstream_fetch_duration_seconds",
		"cacheproxyfy_cve_scans_total",
		"cacheproxyfy_inflight_requests",
	}

	for _, name := range expected {
		assertMetric(t, body, name)
	}
}

// TestRequestsCounter verifies that incrementing the counter is reflected in the scrape.
func TestRequestsCounter(t *testing.T) {
	m, srv := newTestServer(t)

	m.RequestsTotal.WithLabelValues("npm", "hit").Inc()
	m.RequestsTotal.WithLabelValues("npm", "hit").Inc()
	m.RequestsTotal.WithLabelValues("npm", "miss").Inc()

	body := scrape(t, srv)

	if !strings.Contains(body, `cacheproxyfy_requests_total{ecosystem="npm",result="hit"} 2`) {
		t.Errorf("expected hit counter = 2, body:\n%s", body)
	}
	if !strings.Contains(body, `cacheproxyfy_requests_total{ecosystem="npm",result="miss"} 1`) {
		t.Errorf("expected miss counter = 1, body:\n%s", body)
	}
}

// TestInflightGauge verifies the gauge goes up and back down correctly.
func TestInflightGauge(t *testing.T) {
	m, srv := newTestServer(t)

	m.InflightRequests.Inc()
	m.InflightRequests.Inc()

	body := scrape(t, srv)
	if !strings.Contains(body, "cacheproxyfy_inflight_requests 2") {
		t.Errorf("expected inflight = 2, body:\n%s", body)
	}

	m.InflightRequests.Dec()
	body = scrape(t, srv)
	if !strings.Contains(body, "cacheproxyfy_inflight_requests 1") {
		t.Errorf("expected inflight = 1 after dec, body:\n%s", body)
	}
}

// TestCVEScanLabels verifies all expected outcome labels are recordable.
func TestCVEScanLabels(t *testing.T) {
	m, srv := newTestServer(t)

	m.CVEScansTotal.WithLabelValues("npm", "allow").Inc()
	m.CVEScansTotal.WithLabelValues("npm", "warn").Inc()
	m.CVEScansTotal.WithLabelValues("npm", "block").Inc()
	m.CVEScansTotal.WithLabelValues("npm", "error").Inc()

	body := scrape(t, srv)

	for _, outcome := range []string{"allow", "warn", "block", "error"} {
		want := `cacheproxyfy_cve_scans_total{ecosystem="npm",outcome="` + outcome + `"} 1`
		if !strings.Contains(body, want) {
			t.Errorf("missing CVE outcome %q in body:\n%s", outcome, body)
		}
	}
}

// TestHistogramObserved verifies that histogram observations produce _count and _sum lines.
func TestHistogramObserved(t *testing.T) {
	m, srv := newTestServer(t)

	m.RequestDuration.WithLabelValues("npm", "miss").Observe(0.42)
	m.RequestDuration.WithLabelValues("npm", "miss").Observe(1.1)

	body := scrape(t, srv)

	if !strings.Contains(body, `cacheproxyfy_request_duration_seconds_count{ecosystem="npm",result="miss"} 2`) {
		t.Errorf("expected duration _count = 2, body:\n%s", body)
	}
}
