package httpapi

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// gatewayOpenAPIPath locates docs/services/gateway/api/openapi.yaml by walking
// up from the test source file. Returns "" and calls t.Skip if not found.
func gatewayOpenAPIPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("runtime.Caller failed; skipping OpenAPI contract test")
		return ""
	}
	dir := filepath.Dir(file)
	for i := 0; i < 12; i++ {
		candidate := filepath.Join(dir, "docs", "services", "gateway", "api", "openapi.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		dir = filepath.Dir(dir)
	}
	t.Skip("docs/services/gateway/api/openapi.yaml not found; skipping OpenAPI contract test")
	return ""
}

// openAPIOperationIDs extracts all operationId values from the YAML by scanning
// for lines of the form "operationId: <id>". No YAML parser dependency.
func openAPIOperationIDs(t *testing.T, yamlPath string) map[string]bool {
	t.Helper()
	f, err := os.Open(yamlPath)
	if err != nil {
		t.Fatalf("open %s: %v", yamlPath, err)
	}
	defer f.Close()

	ids := map[string]bool{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		after, ok := strings.CutPrefix(line, "operationId:")
		if !ok {
			continue
		}
		id := strings.TrimSpace(after)
		if id != "" {
			ids[id] = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan %s: %v", yamlPath, err)
	}
	return ids
}

// schemaTestServer returns a gateway Server usable for content-type and
// envelope shape assertions. No Redis or downstream services required.
func schemaTestServer() http.Handler {
	return NewServer(Config{
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		ServiceVersion:     "test",
		Environment:        "test",
		RequestTimeout:     time.Second,
		MaxBodyBytes:       1024,
		CORSAllowedOrigins: []string{"*"},
	})
}

// decodeJSONContract decodes JSON from r into dst and returns any error.
func decodeJSONContract(r io.Reader, dst any) error {
	return json.NewDecoder(r).Decode(dst)
}

// ── schema-level route contract ───────────────────────────────────────────────

// TestRouteOperationIDsExistInOpenAPISpec verifies that every operationId in
// activeProxyRoutes has a matching entry in docs/services/gateway/api/openapi.yaml.
// This catches typos in routes.go and detects when the OpenAPI is updated
// without a corresponding routes.go change (or vice versa).
func TestRouteOperationIDsExistInOpenAPISpec(t *testing.T) {
	yamlPath := gatewayOpenAPIPath(t)
	specIDs := openAPIOperationIDs(t, yamlPath)
	if len(specIDs) == 0 {
		t.Fatalf("no operationId values found in %s; check YAML format", yamlPath)
	}

	for _, route := range activeProxyRoutes {
		if route.OperationID == "" {
			t.Errorf("route %s %s has empty operationId", route.Method, route.Pattern)
			continue
		}
		if !specIDs[route.OperationID] {
			t.Errorf("route %s %s operationId=%q not found in OpenAPI spec",
				route.Method, route.Pattern, route.OperationID)
		}
	}
}

// TestRouteOperationIDsAreUnique detects copy-paste errors where two routes
// share the same operationId, which would violate the OpenAPI contract.
func TestRouteOperationIDsAreUnique(t *testing.T) {
	seen := map[string]string{} // operationId → "METHOD /pattern"
	for _, route := range activeProxyRoutes {
		key := route.Method + " " + route.Pattern
		if prev, dup := seen[route.OperationID]; dup {
			t.Errorf("duplicate operationId %q: %s and %s", route.OperationID, prev, key)
		}
		seen[route.OperationID] = key
	}
}

// ── SSE / binary content-type contract ───────────────────────────────────────

// knownSSEOperationIDs is the exhaustive set of routes that return
// text/event-stream (Server-Sent Events). Per the gateway OpenAPI contract,
// only POST /api/v1/qa-sessions/{sessionId}/messages streams SSE.
var knownSSEOperationIDs = map[string]bool{
	"createQAMessage": true,
}

// TestStreamResponseFlagMatchesSSEContract verifies that StreamResponse is set
// exactly for routes expected to return text/event-stream per the OpenAPI.
// A non-SSE route tagged as StreamResponse would bypass the request timeout;
// an SSE route without the flag would be incorrectly cut off mid-stream.
func TestStreamResponseFlagMatchesSSEContract(t *testing.T) {
	for _, route := range activeProxyRoutes {
		wantsSSE := knownSSEOperationIDs[route.OperationID]
		if route.StreamResponse && !wantsSSE {
			t.Errorf("route %s %s (operationId=%q): StreamResponse=true but not an SSE route per OpenAPI",
				route.Method, route.Pattern, route.OperationID)
		}
		if !route.StreamResponse && wantsSSE {
			t.Errorf("route %s %s (operationId=%q): is SSE per OpenAPI but StreamResponse=false",
				route.Method, route.Pattern, route.OperationID)
		}
	}
}

// knownBinaryOperationIDs is the exhaustive set of routes whose success
// response is binary content (application/octet-stream or equivalent).
// These must NOT be wrapped in a JSON envelope by the gateway.
var knownBinaryOperationIDs = map[string]bool{
	"getReportFileContent": true,
	"getDocumentContent":   true,
}

// TestBinaryRoutesCoveredByRouteMatrix verifies that every binary-content
// operationId from the OpenAPI is present in activeProxyRoutes. Catches
// accidental removal of binary-content routes from the route table.
func TestBinaryRoutesCoveredByRouteMatrix(t *testing.T) {
	covered := map[string]bool{}
	for _, route := range activeProxyRoutes {
		if knownBinaryOperationIDs[route.OperationID] {
			covered[route.OperationID] = true
		}
	}
	for id := range knownBinaryOperationIDs {
		if !covered[id] {
			t.Errorf("binary-content route operationId=%q missing from activeProxyRoutes", id)
		}
	}
}

// ── gateway-owned response envelope shapes ────────────────────────────────────

// TestGatewayOwnedResponsesReturnJSONContentType asserts that responses
// generated by the gateway itself (health, readiness, 404, 405) always
// set Content-Type: application/json. Downstream binary and SSE pass-through
// is asserted by TestProxyStreamsBinaryContentWithoutJSONEnvelope and
// TestProxyStreamsSSEWithoutFixedTimeout.
func TestGatewayOwnedResponsesReturnJSONContentType(t *testing.T) {
	server := schemaTestServer()

	cases := []struct {
		name     string
		method   string
		path     string
		wantCode int
	}{
		{"healthz", http.MethodGet, "/healthz", http.StatusOK},
		{"readyz", http.MethodGet, "/readyz", http.StatusOK},
		{"not_found", http.MethodGet, "/unknown-path", http.StatusNotFound},
		// Gateway routes unknown methods as not_found (no 405 distinction).
		{"unregistered_method", http.MethodDelete, "/healthz", http.StatusNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			res := httptest.NewRecorder()
			server.ServeHTTP(res, req)

			if res.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d; body = %s", res.Code, tc.wantCode, res.Body.String())
			}
			ct := res.Header().Get("Content-Type")
			if !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
		})
	}
}

// TestGatewayErrorEnvelopeShape verifies that every gateway-owned error
// response carries the canonical { "error": { "code", "message", "requestId" } }
// shape required by the frontend-backend contract and docs/services/gateway/api/openapi.yaml.
func TestGatewayErrorEnvelopeShape(t *testing.T) {
	server := NewServer(Config{
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		ServiceVersion:     "test",
		Environment:        "test",
		RequestTimeout:     time.Second,
		MaxBodyBytes:       4,
		CORSAllowedOrigins: []string{"*"},
	})

	cases := []struct {
		name      string
		method    string
		path      string
		body      string
		wantCode  int
		wantError string
	}{
		{
			name:      "not_found",
			method:    http.MethodGet,
			path:      "/no-such-route",
			wantCode:  http.StatusNotFound,
			wantError: "not_found",
		},
		{
			name:      "body_too_large",
			method:    http.MethodPost,
			path:      "/healthz",
			body:      "12345",
			wantCode:  http.StatusRequestEntityTooLarge,
			wantError: "validation_error",
		},
		{
			// Gateway routes unknown methods as not_found (no 405 distinction).
			name:      "unregistered_method",
			method:    http.MethodDelete,
			path:      "/healthz",
			wantCode:  http.StatusNotFound,
			wantError: "not_found",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var bodyReader io.Reader
			if tc.body != "" {
				bodyReader = strings.NewReader(tc.body)
			}
			req := httptest.NewRequest(tc.method, tc.path, bodyReader)
			req.Header.Set("X-Request-Id", "req_schema_"+tc.name)
			res := httptest.NewRecorder()
			server.ServeHTTP(res, req)

			if res.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d; body = %s", res.Code, tc.wantCode, res.Body.String())
			}

			var envelope struct {
				Error struct {
					Code      string `json:"code"`
					Message   string `json:"message"`
					RequestID string `json:"requestId"`
				} `json:"error"`
			}
			if err := decodeJSONContract(res.Body, &envelope); err != nil {
				t.Fatalf("decode error envelope: %v", err)
			}
			if envelope.Error.Code != tc.wantError {
				t.Errorf("error.code = %q, want %q", envelope.Error.Code, tc.wantError)
			}
			if envelope.Error.Message == "" {
				t.Error("error.message must not be empty")
			}
			if envelope.Error.RequestID == "" {
				t.Error("error.requestId must not be empty")
			}
		})
	}
}

// TestHealthEnvelopeShape verifies that /healthz and /readyz return the
// canonical { "data": { "status", "service" }, "requestId" } shape defined
// in the gateway-local openapi.yaml HealthResponse schema.
func TestHealthEnvelopeShape(t *testing.T) {
	server := schemaTestServer()

	cases := []struct {
		path       string
		wantStatus string
	}{
		{"/healthz", "ok"},
		{"/readyz", "ready"},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("X-Request-Id", "req_schema_health")
			res := httptest.NewRecorder()
			server.ServeHTTP(res, req)

			if res.Code != http.StatusOK {
				t.Fatalf("status = %d; body = %s", res.Code, res.Body.String())
			}

			var envelope struct {
				Data struct {
					Status  string `json:"status"`
					Service string `json:"service"`
				} `json:"data"`
				RequestID string `json:"requestId"`
			}
			body := res.Body.Bytes()
			if err := decodeJSONContract(bytes.NewReader(body), &envelope); err != nil {
				t.Fatalf("decode health envelope: %v; body = %s", err, body)
			}
			if envelope.Data.Status != tc.wantStatus {
				t.Errorf("data.status = %q, want %q", envelope.Data.Status, tc.wantStatus)
			}
			if envelope.Data.Service == "" {
				t.Error("data.service must not be empty")
			}
			if envelope.RequestID == "" {
				t.Error("requestId must not be empty")
			}
		})
	}
}
