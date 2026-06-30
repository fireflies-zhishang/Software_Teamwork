package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry holds Prometheus metrics for the gateway service.
// Each instance owns an isolated prometheus.Registry so multiple instances
// can coexist in tests without duplicate-registration panics.
type Registry struct {
	reg                    *prometheus.Registry
	httpRequestsTotal      *prometheus.CounterVec
	httpRequestDurationSec *prometheus.HistogramVec
}

// New creates a Registry with gateway HTTP request metrics pre-registered.
func New() *Registry {
	reg := prometheus.NewRegistry()
	factory := promauto.With(reg)

	return &Registry{
		reg: reg,
		httpRequestsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "gateway",
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests handled by the gateway.",
		}, []string{"method", "path", "status"}),
		httpRequestDurationSec: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "gateway",
			Name:      "http_request_duration_seconds",
			Help:      "Duration in seconds of HTTP requests handled by the gateway.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"method", "path", "status"}),
	}
}

// Handler returns an http.Handler that serves the Prometheus text format.
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.reg, promhttp.HandlerOpts{})
}

// Gatherer exposes the underlying prometheus.Gatherer for diagnostics and tests.
func (r *Registry) Gatherer() prometheus.Gatherer {
	return r.reg
}

// ObserveHTTP records one HTTP request.
// Labels must be low-cardinality: method is the HTTP verb, path is the matched
// route pattern (never a raw URL containing user-supplied IDs), status is the
// numeric HTTP response code as a string.
func (r *Registry) ObserveHTTP(method, path, status string, durationSec float64) {
	r.httpRequestsTotal.WithLabelValues(method, path, status).Inc()
	r.httpRequestDurationSec.WithLabelValues(method, path, status).Observe(durationSec)
}
