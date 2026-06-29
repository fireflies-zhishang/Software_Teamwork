package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestActiveReportRoutesHaveStableScaffoldCoverage(t *testing.T) {
	server := NewServer(Config{})

	routes := []struct {
		method       string
		path         string
		expectedCode string
	}{
		{http.MethodGet, "/report-types", "dependency_error"},
		{http.MethodGet, "/report-templates", "dependency_error"},
		{http.MethodPost, "/report-templates", "dependency_error"},
		{http.MethodGet, "/report-templates/rtpl_123", "dependency_error"},
		{http.MethodPatch, "/report-templates/rtpl_123", "dependency_error"},
		{http.MethodDelete, "/report-templates/rtpl_123", "dependency_error"},
		{http.MethodGet, "/report-templates/rtpl_123/structure", "dependency_error"},
		{http.MethodPatch, "/report-templates/rtpl_123/structure", "dependency_error"},
		{http.MethodGet, "/report-materials", "dependency_error"},
		{http.MethodPost, "/report-materials", "dependency_error"},
		{http.MethodGet, "/report-materials/mat_123", "dependency_error"},
		{http.MethodDelete, "/report-materials/mat_123", "dependency_error"},
		// reports / outlines / sections are implemented for real (C-03); with
		// no ReportService configured here they fail the same way the
		// template/material routes do above: a dependency_error, not a 404
		// or a scaffold not_implemented.
		{http.MethodGet, "/reports", "dependency_error"},
		{http.MethodPost, "/reports", "dependency_error"},
		{http.MethodGet, "/reports/rpt_123", "dependency_error"},
		{http.MethodPatch, "/reports/rpt_123", "dependency_error"},
		{http.MethodDelete, "/reports/rpt_123", "dependency_error"},
		{http.MethodGet, "/reports/rpt_123/outlines", "dependency_error"},
		{http.MethodPost, "/reports/rpt_123/outlines", "dependency_error"},
		{http.MethodGet, "/reports/rpt_123/outlines/out_123", "dependency_error"},
		{http.MethodPatch, "/reports/rpt_123/outlines/out_123", "dependency_error"},
		{http.MethodDelete, "/reports/rpt_123/outlines/out_123/sections/sec_123", "dependency_error"},
		{http.MethodGet, "/reports/rpt_123/sections", "dependency_error"},
		{http.MethodPost, "/reports/rpt_123/sections", "dependency_error"},
		{http.MethodGet, "/reports/rpt_123/sections/sec_123", "dependency_error"},
		{http.MethodPatch, "/reports/rpt_123/sections/sec_123", "dependency_error"},
		{http.MethodGet, "/reports/rpt_123/sections/sec_123/versions", "dependency_error"},
		{http.MethodPost, "/reports/rpt_123/sections/sec_123/versions", "dependency_error"},
		{http.MethodGet, "/reports/rpt_123/jobs", "dependency_error"},
		{http.MethodPost, "/reports/rpt_123/jobs", "dependency_error"},
		{http.MethodGet, "/report-jobs/job_123", "dependency_error"},
		{http.MethodGet, "/report-jobs/job_123/attempts", "dependency_error"},
		{http.MethodPost, "/report-jobs/job_123/attempts", "dependency_error"},
		{http.MethodGet, "/reports/rpt_123/events", "dependency_error"},
		{http.MethodGet, "/report-files", "not_implemented"},
		{http.MethodPost, "/report-files", "not_implemented"},
		{http.MethodGet, "/report-files/rfile_123", "not_implemented"},
		{http.MethodGet, "/report-files/rfile_123/content", "not_implemented"},
		{http.MethodGet, "/report-statistics/overview", "not_implemented"},
		{http.MethodGet, "/report-statistics/daily", "not_implemented"},
		{http.MethodGet, "/report-operation-logs", "not_implemented"},
		{http.MethodGet, "/report-settings", "not_implemented"},
		{http.MethodPatch, "/report-settings", "not_implemented"},
	}

	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-User-Id", "usr_test")
			req.Header.Set("X-Request-Id", "req_routes")
			rec := httptest.NewRecorder()

			server.ServeHTTP(rec, req)

			if rec.Code == http.StatusNotFound {
				t.Fatalf("route fell through to 404: %s", rec.Body.String())
			}

			var body struct {
				Error struct {
					Code      string `json:"code"`
					RequestID string `json:"requestId"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode error response: %v; body=%s", err, rec.Body.String())
			}
			if body.Error.Code != route.expectedCode {
				t.Fatalf("error code = %q, want %q; body=%s", body.Error.Code, route.expectedCode, rec.Body.String())
			}
			if body.Error.RequestID != "req_routes" {
				t.Fatalf("requestId = %q, want req_routes", body.Error.RequestID)
			}
		})
	}
}
