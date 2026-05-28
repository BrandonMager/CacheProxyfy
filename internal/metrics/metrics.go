package metrics

import "github.com/prometheus/client_golang/prometheus"

// Metrics holds all Prometheus instruments for CacheProxyfy.
// Every field is a labeled instrument so Grafana/PromQL can slice by ecosystem.
type Metrics struct {
	// RequestsTotal counts every completed proxy request.
	// Labels: ecosystem, result ("hit" | "miss" | "error")
	RequestsTotal *prometheus.CounterVec

	// RequestDuration is the end-to-end latency histogram for every request,
	// including cache hits, upstream fetches, and errors.
	// Labels: ecosystem, result
	RequestDuration *prometheus.HistogramVec

	// BytesServedTotal is the cumulative bytes written to clients.
	// Only incremented on successful responses (hit or miss).
	// Labels: ecosystem, result ("hit" | "miss")
	BytesServedTotal *prometheus.CounterVec

	// PackageSizeBytes records the distribution of package sizes returned to clients.
	// Useful for capacity planning and P95/P99 size analysis.
	// Labels: ecosystem
	PackageSizeBytes *prometheus.HistogramVec

	// UpstreamFetchesTotal counts outbound fetches to upstream registries.
	// Only incremented when the singleflight leader performs the actual fetch.
	// Labels: ecosystem, status ("ok" | "error")
	UpstreamFetchesTotal *prometheus.CounterVec

	// UpstreamFetchDuration is the latency histogram for upstream fetches.
	// Only observed on successful fetches by the singleflight leader.
	// Labels: ecosystem
	UpstreamFetchDuration *prometheus.HistogramVec

	// CVEScansTotal counts OSV security scan decisions.
	// Labels: ecosystem, outcome ("allow" | "warn" | "block" | "error")
	CVEScansTotal *prometheus.CounterVec

	// InflightRequests is the instantaneous number of in-flight proxy requests.
	// Use this as a saturation signal.
	InflightRequests prometheus.Gauge

	// RateLimitWaitDuration is how long upstream fetches spend waiting for a rate-limit
	// token. Near-zero when the limiter is disabled or burst is available; higher values
	// indicate upstream back-pressure. Labels: ecosystem
	RateLimitWaitDuration *prometheus.HistogramVec

	// UpstreamRetriesTotal counts retry attempts triggered by transient upstream
	// failures (network errors, 5xx, 429). Only incremented when a retry actually
	// fires — the initial attempt is not counted. Alert on sustained non-zero rates.
	// Labels: ecosystem
	UpstreamRetriesTotal *prometheus.CounterVec
}

// proxyDurationBuckets covers the expected range for a caching proxy:
// fast cache hits (ms) through slow upstream package downloads (minutes).
var proxyDurationBuckets = []float64{
	0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300,
}

// sizeBuckets covers package sizes from 1 KB to ~256 MB in 4× steps.
var sizeBuckets = prometheus.ExponentialBuckets(1024, 4, 10)

// rateLimitWaitBuckets covers the expected range for token-bucket wait times:
// from negligible (sub-millisecond when burst is available) to several seconds
// (low rps, high concurrency).
var rateLimitWaitBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// New registers all metrics against reg and returns the populated Metrics struct.
// ecosystems is used to pre-initialise every labeled series at zero so all
// metric families appear in /metrics immediately, before any traffic arrives.
// Use prometheus.NewRegistry() for an isolated registry (recommended in production
// to avoid polluting the default global registry).
func New(reg prometheus.Registerer, ecosystems []string) *Metrics {
	m := &Metrics{
		RequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cacheproxyfy_requests_total",
			Help: "Total proxy requests partitioned by ecosystem and result.",
		}, []string{"ecosystem", "result"}),

		RequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "cacheproxyfy_request_duration_seconds",
			Help:    "End-to-end request latency in seconds.",
			Buckets: proxyDurationBuckets,
		}, []string{"ecosystem", "result"}),

		BytesServedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cacheproxyfy_bytes_served_total",
			Help: "Total bytes written to clients on successful responses.",
		}, []string{"ecosystem", "result"}),

		PackageSizeBytes: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "cacheproxyfy_package_size_bytes",
			Help:    "Distribution of package payload sizes in bytes.",
			Buckets: sizeBuckets,
		}, []string{"ecosystem"}),

		UpstreamFetchesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cacheproxyfy_upstream_fetches_total",
			Help: "Total upstream registry fetches partitioned by ecosystem and status.",
		}, []string{"ecosystem", "status"}),

		UpstreamFetchDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "cacheproxyfy_upstream_fetch_duration_seconds",
			Help:    "Latency of upstream registry fetches in seconds.",
			Buckets: proxyDurationBuckets,
		}, []string{"ecosystem"}),

		CVEScansTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cacheproxyfy_cve_scans_total",
			Help: "OSV security scan decisions partitioned by ecosystem and outcome.",
		}, []string{"ecosystem", "outcome"}),

		InflightRequests: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "cacheproxyfy_inflight_requests",
			Help: "Current number of proxy requests in flight (saturation signal).",
		}),

		RateLimitWaitDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "cacheproxyfy_rate_limit_wait_duration_seconds",
			Help:    "Time spent waiting for a rate-limit token before an upstream fetch, per ecosystem.",
			Buckets: rateLimitWaitBuckets,
		}, []string{"ecosystem"}),

		UpstreamRetriesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cacheproxyfy_upstream_retries_total",
			Help: "Total retry attempts against upstream registries due to transient errors, per ecosystem.",
		}, []string{"ecosystem"}),
	}

	reg.MustRegister(
		m.RequestsTotal,
		m.RequestDuration,
		m.BytesServedTotal,
		m.PackageSizeBytes,
		m.UpstreamFetchesTotal,
		m.UpstreamFetchDuration,
		m.CVEScansTotal,
		m.InflightRequests,
		m.RateLimitWaitDuration,
		m.UpstreamRetriesTotal,
	)

	for _, eco := range ecosystems {
		for _, result := range []string{"hit", "miss", "error"} {
			m.RequestsTotal.With(prometheus.Labels{"ecosystem": eco, "result": result})
			m.RequestDuration.With(prometheus.Labels{"ecosystem": eco, "result": result})
		}
		for _, result := range []string{"hit", "miss"} {
			m.BytesServedTotal.With(prometheus.Labels{"ecosystem": eco, "result": result})
		}
		m.PackageSizeBytes.With(prometheus.Labels{"ecosystem": eco})
		for _, status := range []string{"ok", "error"} {
			m.UpstreamFetchesTotal.With(prometheus.Labels{"ecosystem": eco, "status": status})
		}
		m.UpstreamFetchDuration.With(prometheus.Labels{"ecosystem": eco})
		for _, outcome := range []string{"allow", "warn", "block", "error"} {
			m.CVEScansTotal.With(prometheus.Labels{"ecosystem": eco, "outcome": outcome})
		}
		m.RateLimitWaitDuration.With(prometheus.Labels{"ecosystem": eco})
		m.UpstreamRetriesTotal.With(prometheus.Labels{"ecosystem": eco})
	}

	return m
}
