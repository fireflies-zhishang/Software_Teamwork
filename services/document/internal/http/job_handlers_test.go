package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/document/internal/service"
)

// mockJobSvc implements JobSvc for testing.
type mockJobSvc struct {
	createJobFn    func(ctx context.Context, rctx service.RequestContext, input service.CreateJobInput) (service.ReportJob, error)
	listJobsFn     func(ctx context.Context, rctx service.RequestContext, reportID string) ([]service.ReportJob, error)
	getJobFn       func(ctx context.Context, rctx service.RequestContext, id string) (service.ReportJob, error)
	retryJobFn     func(ctx context.Context, rctx service.RequestContext, id, reason string) (service.ReportJobAttempt, error)
	listAttemptsFn func(ctx context.Context, rctx service.RequestContext, jobID string) ([]service.ReportJobAttempt, error)
	listEventsFn   func(ctx context.Context, rctx service.RequestContext, reportID string) ([]service.ReportEvent, error)
}

func (m *mockJobSvc) CreateJob(ctx context.Context, rctx service.RequestContext, input service.CreateJobInput) (service.ReportJob, error) {
	return m.createJobFn(ctx, rctx, input)
}

func (m *mockJobSvc) ListJobs(ctx context.Context, rctx service.RequestContext, reportID string) ([]service.ReportJob, error) {
	return m.listJobsFn(ctx, rctx, reportID)
}

func (m *mockJobSvc) GetJob(ctx context.Context, rctx service.RequestContext, id string) (service.ReportJob, error) {
	return m.getJobFn(ctx, rctx, id)
}

func (m *mockJobSvc) RetryJob(ctx context.Context, rctx service.RequestContext, id, reason string) (service.ReportJobAttempt, error) {
	return m.retryJobFn(ctx, rctx, id, reason)
}

func (m *mockJobSvc) ListAttempts(ctx context.Context, rctx service.RequestContext, jobID string) ([]service.ReportJobAttempt, error) {
	return m.listAttemptsFn(ctx, rctx, jobID)
}

func (m *mockJobSvc) ListEvents(ctx context.Context, rctx service.RequestContext, reportID string) ([]service.ReportEvent, error) {
	return m.listEventsFn(ctx, rctx, reportID)
}

func newTestServerWithJobSvc(svc JobSvc) *Server {
	return NewServer(Config{JobSvc: svc})
}

func TestCreateJobAcceptsGenerationPayload(t *testing.T) {
	var captured service.CreateJobInput
	mock := &mockJobSvc{
		createJobFn: func(ctx context.Context, rctx service.RequestContext, input service.CreateJobInput) (service.ReportJob, error) {
			captured = input
			return service.ReportJob{
				ID:          "job-1",
				ReportID:    input.ReportID,
				JobType:     input.JobType,
				TargetType:  input.TargetScope,
				TargetID:    input.SectionID,
				Status:      service.JobStatusPending,
				MaxAttempts: 3,
				CreatedAt:   time.Now().UTC(),
			}, nil
		},
	}
	server := newTestServerWithJobSvc(mock)
	body := `{
		"jobType": "section_regeneration",
		"target": {"scope": "section", "sectionId": "section-1"},
		"requirements": "focus on overload risks",
		"materialIds": ["material-1"],
		"options": {"knowledgeBaseIds": ["kb-1"], "topK": 3}
	}`

	req := httptest.NewRequest(http.MethodPost, "/reports/report-1/jobs", strings.NewReader(body))
	req.Header.Set("X-User-Id", "usr_owner")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if captured.JobType != service.JobTypeSectionRegeneration || captured.TargetScope != "section" || captured.SectionID != "section-1" {
		t.Fatalf("captured input = %+v", captured)
	}
	if captured.Requirements != "focus on overload risks" {
		t.Fatalf("captured requirements = %q", captured.Requirements)
	}
	if len(captured.MaterialIDs) != 1 || captured.MaterialIDs[0] != "material-1" {
		t.Fatalf("captured material IDs = %#v", captured.MaterialIDs)
	}
	if captured.Options["topK"] != float64(3) {
		t.Fatalf("captured options = %#v", captured.Options)
	}
}

func TestListJobsEmptyList(t *testing.T) {
	mock := &mockJobSvc{
		listJobsFn: func(ctx context.Context, rctx service.RequestContext, reportID string) ([]service.ReportJob, error) {
			return []service.ReportJob{}, nil
		},
	}
	server := newTestServerWithJobSvc(mock)

	req := httptest.NewRequest(http.MethodGet, "/reports/550e8400-e29b-41d4-a716-446655440000/jobs", nil)
	req.Header.Set("X-User-Id", "usr_owner")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body struct {
		Data []jobResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data) != 0 {
		t.Fatalf("expected empty list, got %d items", len(body.Data))
	}
}

func TestGetJobNotFound(t *testing.T) {
	mock := &mockJobSvc{
		getJobFn: func(ctx context.Context, rctx service.RequestContext, id string) (service.ReportJob, error) {
			return service.ReportJob{}, service.NewError(service.CodeNotFound, "report job not found", nil)
		},
	}
	server := newTestServerWithJobSvc(mock)

	req := httptest.NewRequest(http.MethodGet, "/report-jobs/550e8400-e29b-41d4-a716-446655440001", nil)
	req.Header.Set("X-User-Id", "usr_owner")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetJobForbidden(t *testing.T) {
	mock := &mockJobSvc{
		getJobFn: func(ctx context.Context, rctx service.RequestContext, id string) (service.ReportJob, error) {
			if rctx.UserID != "usr_owner" {
				return service.ReportJob{}, service.NewError(service.CodeForbidden, "you do not have access to this report", nil)
			}
			return service.ReportJob{ID: id}, nil
		},
	}
	server := newTestServerWithJobSvc(mock)

	req := httptest.NewRequest(http.MethodGet, "/report-jobs/550e8400-e29b-41d4-a716-446655440001", nil)
	req.Header.Set("X-User-Id", "usr_other")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 Forbidden", rec.Code)
	}
}

func TestRetryJobMaxAttemptsReached(t *testing.T) {
	mock := &mockJobSvc{
		retryJobFn: func(ctx context.Context, rctx service.RequestContext, id, reason string) (service.ReportJobAttempt, error) {
			return service.ReportJobAttempt{}, service.NewError(service.CodeValidation, "max retry attempts reached", nil)
		},
	}
	server := newTestServerWithJobSvc(mock)

	req := httptest.NewRequest(http.MethodPost, "/report-jobs/550e8400-e29b-41d4-a716-446655440001/attempts", nil)
	req.Header.Set("X-User-Id", "usr_owner")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestListAttempts(t *testing.T) {
	now := time.Now().UTC()
	mock := &mockJobSvc{
		listAttemptsFn: func(ctx context.Context, rctx service.RequestContext, jobID string) ([]service.ReportJobAttempt, error) {
			return []service.ReportJobAttempt{
				{
					ID:            "attempt-1",
					JobID:         jobID,
					AttemptNumber: 1,
					TriggerSource: "system",
					Status:        service.JobStatusSucceeded,
					CreatedAt:     now,
				},
			}, nil
		},
	}
	server := newTestServerWithJobSvc(mock)

	req := httptest.NewRequest(http.MethodGet, "/report-jobs/550e8400-e29b-41d4-a716-446655440001/attempts", nil)
	req.Header.Set("X-User-Id", "usr_owner")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestListEvents(t *testing.T) {
	mock := &mockJobSvc{
		listEventsFn: func(ctx context.Context, rctx service.RequestContext, reportID string) ([]service.ReportEvent, error) {
			return []service.ReportEvent{}, nil
		},
	}
	server := newTestServerWithJobSvc(mock)

	req := httptest.NewRequest(http.MethodGet, "/reports/550e8400-e29b-41d4-a716-446655440000/events", nil)
	req.Header.Set("X-User-Id", "usr_owner")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
