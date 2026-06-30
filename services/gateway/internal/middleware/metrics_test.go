package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/gateway/internal/metrics"
	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/gateway/internal/middleware"
)

// ── HTTP passthrough behaviour ────────────────────────────────────────────────

func TestMetricsMiddlewarePassesThrough(t *testing.T) {
	reg := metrics.New()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware.Chain(mux, middleware.Metrics(reg, mux))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ping", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
}

func TestMetricsMiddlewareCapturesNonOKStatus(t *testing.T) {
	reg := metrics.New()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /fail", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})

	handler := middleware.Chain(mux, middleware.Metrics(reg, mux))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/fail", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestMetricsMiddlewareNilRegistryIsNoOp(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware.Chain(mux, middleware.Metrics(nil, mux))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ok", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
}

func TestMetricsMiddlewareFlusherPassThrough(t *testing.T) {
	reg := metrics.New()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /stream", func(w http.ResponseWriter, r *http.Request) {
		f, ok := w.(http.Flusher)
		if !ok {
			t.Error("responseWriter does not implement http.Flusher")
			http.Error(w, "no flusher", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("data: hello\n\n"))
		f.Flush()
	})

	handler := middleware.Chain(mux, middleware.Metrics(reg, mux))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/stream", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
}

func TestMetricsMiddlewareImplicit200OnWrite(t *testing.T) {
	reg := metrics.New()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /body", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	handler := middleware.Chain(mux, middleware.Metrics(reg, mux))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/body", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
}

// ── Prometheus metric contract ────────────────────────────────────────────────

// TestMetricsCounterLabels200 verifies gateway_http_requests_total is
// incremented with correct method/path/status labels for a 200 response.
func TestMetricsCounterLabels200(t *testing.T) {
	reg := metrics.New()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	middleware.Chain(mux, middleware.Metrics(reg, mux)).
		ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ping", nil))

	if err := testutil.GatherAndCompare(reg.Gatherer(), strings.NewReader(`
# HELP gateway_http_requests_total Total number of HTTP requests handled by the gateway.
# TYPE gateway_http_requests_total counter
gateway_http_requests_total{method="GET",path="GET /ping",status="200"} 1
`), "gateway_http_requests_total"); err != nil {
		t.Errorf("counter label mismatch: %v", err)
	}
}

// TestMetricsCounterLabels400 verifies that a 400 response is recorded with
// status="400", not the default "200".
func TestMetricsCounterLabels400(t *testing.T) {
	reg := metrics.New()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /fail", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	middleware.Chain(mux, middleware.Metrics(reg, mux)).
		ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/fail", nil))

	if err := testutil.GatherAndCompare(reg.Gatherer(), strings.NewReader(`
# HELP gateway_http_requests_total Total number of HTTP requests handled by the gateway.
# TYPE gateway_http_requests_total counter
gateway_http_requests_total{method="GET",path="GET /fail",status="400"} 1
`), "gateway_http_requests_total"); err != nil {
		t.Errorf("status label should be 400: %v", err)
	}
}

// TestMetricsPathLabelUsesRoutePattern ensures that a request to /items/123 is
// recorded as path="GET /items/{id}", never the raw URL, keeping label
// cardinality bounded.
func TestMetricsPathLabelUsesRoutePattern(t *testing.T) {
	reg := metrics.New()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /items/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	middleware.Chain(mux, middleware.Metrics(reg, mux)).
		ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/items/123", nil))

	if err := testutil.GatherAndCompare(reg.Gatherer(), strings.NewReader(`
# HELP gateway_http_requests_total Total number of HTTP requests handled by the gateway.
# TYPE gateway_http_requests_total counter
gateway_http_requests_total{method="GET",path="GET /items/{id}",status="200"} 1
`), "gateway_http_requests_total"); err != nil {
		t.Errorf("path label must be route pattern, not raw URL: %v", err)
	}
}

// TestMetricsUnmatchedRouteUsesCatchAll verifies that requests not matching any
// specific pattern are recorded under "/" rather than the raw URL.
func TestMetricsUnmatchedRouteUsesCatchAll(t *testing.T) {
	reg := metrics.New()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	middleware.Chain(mux, middleware.Metrics(reg, mux)).
		ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/unknown/path/999", nil))

	if err := testutil.GatherAndCompare(reg.Gatherer(), strings.NewReader(`
# HELP gateway_http_requests_total Total number of HTTP requests handled by the gateway.
# TYPE gateway_http_requests_total counter
gateway_http_requests_total{method="GET",path="/",status="404"} 1
`), "gateway_http_requests_total"); err != nil {
		t.Errorf("unmatched route should use catch-all pattern: %v", err)
	}
}
