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
}

// proxyDurationBuckets covers the expected range for a caching proxy:
// fast cache hits (ms) through slow upstream package downloads (minutes).
var proxyDurationBuckets = []float64{
	0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300,
}

// sizeBuckets covers package sizes from 1 KB to ~256 MB in 4× steps.
var sizeBuckets = prometheus.ExponentialBuckets(1024, 4, 10)

// New registers all metrics against reg and returns the populated Metrics struct.
// Use prometheus.NewRegistry() for an isolated registry (recommended in production
// to avoid polluting the default global registry).
func New(reg prometheus.Registerer) *Metrics {
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
	)

	return m
}
