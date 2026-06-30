package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/gateway/internal/metrics"
)

// Metrics returns a middleware that records HTTP request counts and durations.
// The route pattern is resolved via mux.Handler so path labels are
// low-cardinality route templates rather than raw request URLs.
// If reg is nil the middleware is a no-op pass-through.
func Metrics(reg *metrics.Registry, mux *http.ServeMux) Middleware {
	if reg == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, pattern := mux.Handler(r)
			if pattern == "" {
				pattern = "unknown"
			}
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r)
			reg.ObserveHTTP(r.Method, pattern, strconv.Itoa(rw.status), time.Since(start).Seconds())
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture the HTTP status code.
// It records only the first status written (matching net/http semantics),
// captures the implicit 200 on the first Write call, and forwards Flush so
// SSE / streaming proxy paths that assert http.Flusher continue to work.
type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *responseWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.status = code
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.status = http.StatusOK
		w.wroteHeader = true
	}
	return w.ResponseWriter.Write(b)
}

func (w *responseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *responseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
