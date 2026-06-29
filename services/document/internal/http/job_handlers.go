package httpapi

import (
	"net/http"
	"time"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/document/internal/service"
)

type jobResponse struct {
	ID           string  `json:"id"`
	JobType      string  `json:"jobType"`
	Status       string  `json:"status"`
	ReportID     string  `json:"reportId"`
	AttemptCount int     `json:"attemptCount"`
	MaxAttempts  int     `json:"maxAttempts"`
	ErrorCode    string  `json:"errorCode,omitempty"`
	ErrorMessage string  `json:"errorMessage,omitempty"`
	StartedAt    *string `json:"startedAt,omitempty"`
	FinishedAt   *string `json:"finishedAt,omitempty"`
	CreatedAt    string  `json:"createdAt"`
}

type attemptResponse struct {
	ID            string  `json:"id"`
	JobID         string  `json:"jobId"`
	AttemptNumber int     `json:"attemptNumber"`
	TriggerSource string  `json:"triggerSource"`
	Status        string  `json:"status"`
	ErrorCode     string  `json:"errorCode,omitempty"`
	ErrorMessage  string  `json:"errorMessage,omitempty"`
	StartedAt     *string `json:"startedAt,omitempty"`
	FinishedAt    *string `json:"finishedAt,omitempty"`
	CreatedAt     string  `json:"createdAt"`
}

type eventResponse struct {
	ID        string `json:"id"`
	ReportID  string `json:"reportId"`
	JobID     string `json:"jobId,omitempty"`
	EventType string `json:"eventType"`
	Message   string `json:"message,omitempty"`
	CreatedAt string `json:"createdAt"`
}

func toJobResponse(j service.ReportJob) jobResponse {
	r := jobResponse{
		ID:           j.ID,
		JobType:      string(j.JobType),
		Status:       string(j.Status),
		ReportID:     j.ReportID,
		AttemptCount: j.RetryCount,
		MaxAttempts:  j.MaxAttempts,
		ErrorCode:    j.ErrorCode,
		ErrorMessage: j.ErrorMessage,
		CreatedAt:    j.CreatedAt.UTC().Format(time.RFC3339),
	}
	if j.StartedAt != nil {
		s := j.StartedAt.UTC().Format(time.RFC3339)
		r.StartedAt = &s
	}
	if j.FinishedAt != nil {
		f := j.FinishedAt.UTC().Format(time.RFC3339)
		r.FinishedAt = &f
	}
	return r
}

func toAttemptResponse(a service.ReportJobAttempt) attemptResponse {
	r := attemptResponse{
		ID:            a.ID,
		JobID:         a.JobID,
		AttemptNumber: a.AttemptNumber,
		TriggerSource: a.TriggerSource,
		Status:        string(a.Status),
		ErrorCode:     a.ErrorCode,
		ErrorMessage:  a.ErrorMessage,
		CreatedAt:     a.CreatedAt.UTC().Format(time.RFC3339),
	}
	if a.StartedAt != nil {
		s := a.StartedAt.UTC().Format(time.RFC3339)
		r.StartedAt = &s
	}
	if a.FinishedAt != nil {
		f := a.FinishedAt.UTC().Format(time.RFC3339)
		r.FinishedAt = &f
	}
	return r
}

func toEventResponse(e service.ReportEvent) eventResponse {
	return eventResponse{
		ID:        e.ID,
		ReportID:  e.ReportID,
		JobID:     e.JobID,
		EventType: e.EventType,
		Message:   e.Message,
		CreatedAt: e.CreatedAt.UTC().Format(time.RFC3339),
	}
}

type createJobRequest struct {
	JobType string `json:"jobType"`
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	if s.jobSvc == nil {
		writeError(w, r, service.NewError(service.CodeDependency, "job service not configured", nil))
		return
	}
	rctx := s.requestContext(r)
	reportID := r.PathValue("reportId")
	var req createJobRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.JobType == "" {
		writeError(w, r, service.ValidationError(map[string]string{"jobType": "required"}))
		return
	}
	input := service.CreateJobInput{
		RequestID: requestIDFromContext(r.Context()),
		UserID:    rctx.UserID,
		ReportID:  reportID,
		JobType:   service.JobType(req.JobType),
	}
	job, err := s.jobSvc.CreateJob(r.Context(), rctx, input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, r, http.StatusAccepted, toJobResponse(job))
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	if s.jobSvc == nil {
		writeError(w, r, service.NewError(service.CodeDependency, "job service not configured", nil))
		return
	}
	rctx := s.requestContext(r)
	reportID := r.PathValue("reportId")
	jobs, err := s.jobSvc.ListJobs(r.Context(), rctx, reportID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	resp := make([]jobResponse, len(jobs))
	for i, j := range jobs {
		resp[i] = toJobResponse(j)
	}
	writeData(w, r, http.StatusOK, resp)
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	if s.jobSvc == nil {
		writeError(w, r, service.NewError(service.CodeDependency, "job service not configured", nil))
		return
	}
	rctx := s.requestContext(r)
	jobID := r.PathValue("jobId")
	job, err := s.jobSvc.GetJob(r.Context(), rctx, jobID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, r, http.StatusOK, toJobResponse(job))
}

type retryJobRequest struct {
	Reason string `json:"reason"`
}

func (s *Server) handleRetryJob(w http.ResponseWriter, r *http.Request) {
	if s.jobSvc == nil {
		writeError(w, r, service.NewError(service.CodeDependency, "job service not configured", nil))
		return
	}
	rctx := s.requestContext(r)
	jobID := r.PathValue("jobId")
	var req retryJobRequest
	if r.ContentLength != 0 {
		if !decodeJSON(w, r, &req) {
			return
		}
	}
	attempt, err := s.jobSvc.RetryJob(r.Context(), rctx, jobID, req.Reason)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, r, http.StatusAccepted, toAttemptResponse(attempt))
}

func (s *Server) handleListAttempts(w http.ResponseWriter, r *http.Request) {
	if s.jobSvc == nil {
		writeError(w, r, service.NewError(service.CodeDependency, "job service not configured", nil))
		return
	}
	rctx := s.requestContext(r)
	jobID := r.PathValue("jobId")
	attempts, err := s.jobSvc.ListAttempts(r.Context(), rctx, jobID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	resp := make([]attemptResponse, len(attempts))
	for i, a := range attempts {
		resp[i] = toAttemptResponse(a)
	}
	writeData(w, r, http.StatusOK, resp)
}

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	if s.jobSvc == nil {
		writeError(w, r, service.NewError(service.CodeDependency, "job service not configured", nil))
		return
	}
	rctx := s.requestContext(r)
	reportID := r.PathValue("reportId")
	events, err := s.jobSvc.ListEvents(r.Context(), rctx, reportID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	resp := make([]eventResponse, len(events))
	for i, e := range events {
		resp[i] = toEventResponse(e)
	}
	writeData(w, r, http.StatusOK, resp)
}
