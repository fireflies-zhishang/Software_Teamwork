package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestJobServiceCreateJobAcceptsDocumentJobTypes(t *testing.T) {
	ctx := context.Background()
	repo := &fakeJobRepository{
		report: Report{
			ID:        "report-1",
			CreatorID: "user-1",
		},
	}
	enqueuer := &fakeTaskEnqueuer{}
	svc := NewJobService(repo, enqueuer)

	job, err := svc.CreateJob(ctx, RequestContext{UserID: "user-1"}, CreateJobInput{
		RequestID: "req-1",
		UserID:    "user-1",
		ReportID:  "report-1",
		JobType:   JobTypeContentGeneration,
	})
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	if job.JobType != JobTypeContentGeneration {
		t.Fatalf("JobType = %q, want %q", job.JobType, JobTypeContentGeneration)
	}
	if enqueuer.jobType != JobTypeContentGeneration {
		t.Fatalf("enqueued job type = %q, want %q", enqueuer.jobType, JobTypeContentGeneration)
	}
}

func TestJobServiceCreateReportFileJobCreatesPendingReportFile(t *testing.T) {
	ctx := context.Background()
	repo := &fakeJobRepository{
		report: Report{
			ID:        "report-1",
			Name:      "Export Source",
			CreatorID: "user-1",
			Status:    ReportStatusGenerated,
		},
	}
	enqueuer := &fakeTaskEnqueuer{}
	svc := NewJobService(repo, enqueuer)

	job, err := svc.CreateJob(ctx, RequestContext{UserID: "user-1"}, CreateJobInput{
		RequestID: "req-1",
		UserID:    "user-1",
		ReportID:  "report-1",
		JobType:   JobTypeReportFileCreation,
	})
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	if job.JobType != JobTypeReportFileCreation {
		t.Fatalf("JobType = %q, want %q", job.JobType, JobTypeReportFileCreation)
	}
	if repo.reportFile.JobID != job.ID {
		t.Fatalf("ReportFile.JobID = %q, want %q", repo.reportFile.JobID, job.ID)
	}
	if repo.reportFile.Status != ReportFileStatusPending || repo.reportFile.Format != ReportFileFormatDOCX {
		t.Fatalf("unexpected report file: %+v", repo.reportFile)
	}
	if repo.reportFile.Filename != "Export Source.docx" {
		t.Fatalf("ReportFile.Filename = %q", repo.reportFile.Filename)
	}
	if enqueuer.jobType != JobTypeReportFileCreation {
		t.Fatalf("enqueued job type = %q, want %q", enqueuer.jobType, JobTypeReportFileCreation)
	}
}

func TestJobServiceCreateJobRejectsUnknownJobType(t *testing.T) {
	ctx := context.Background()
	svc := NewJobService(&fakeJobRepository{
		report: Report{ID: "report-1", CreatorID: "user-1"},
	}, &fakeTaskEnqueuer{})

	_, err := svc.CreateJob(ctx, RequestContext{UserID: "user-1"}, CreateJobInput{
		RequestID: "req-1",
		UserID:    "user-1",
		ReportID:  "report-1",
		JobType:   JobType("unknown"),
	})
	if err == nil {
		t.Fatal("CreateJob() error = nil, want validation error")
	}
	appErr, ok := Classify(err)
	if !ok || appErr.Code != CodeValidation {
		t.Fatalf("CreateJob() error = %v, want validation_error", err)
	}
}

func TestJobServiceCreateJobRecordsOperationLog(t *testing.T) {
	ctx := context.Background()
	repo := &fakeJobRepository{
		report: Report{ID: "report-1", CreatorID: "user-1"},
	}
	svc := NewJobService(repo, &fakeTaskEnqueuer{})

	job, err := svc.CreateJob(ctx, RequestContext{UserID: "user-1", RequestID: "req-job-audit"}, CreateJobInput{
		RequestID: "req-job-audit",
		UserID:    "user-1",
		ReportID:  "report-1",
		JobType:   JobTypeReportFileCreation,
	})
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	if len(repo.operationLogs) != 1 {
		t.Fatalf("operation log count = %d, want 1", len(repo.operationLogs))
	}
	if got := repo.operationLogs[0]; got.OperationType != OperationReportFileCreation || got.TargetID != job.ID || got.Metadata["reportId"] != "report-1" {
		t.Fatalf("unexpected job operation log: %+v", got)
	}
}

func TestJobServiceCreateJobRecordsFailedOperationLogWhenEnqueueFails(t *testing.T) {
	ctx := context.Background()
	repo := &fakeJobRepository{
		report: Report{ID: "report-1", CreatorID: "user-1"},
	}
	svc := NewJobService(repo, &fakeTaskEnqueuer{err: errors.New("redis unavailable")})

	_, err := svc.CreateJob(ctx, RequestContext{UserID: "user-1", RequestID: "req-job-failed"}, CreateJobInput{
		RequestID: "req-job-failed",
		UserID:    "user-1",
		ReportID:  "report-1",
		JobType:   JobTypeContentGeneration,
	})
	if err == nil {
		t.Fatal("CreateJob() error = nil, want enqueue error")
	}
	if len(repo.operationLogs) != 1 {
		t.Fatalf("operation log count = %d, want 1", len(repo.operationLogs))
	}
	if got := repo.operationLogs[0]; got.OperationType != OperationReportJobFailed || got.OperationResult != OperationResultFailed || got.TargetType != "job" || got.RequestID != "req-job-failed" {
		t.Fatalf("unexpected failed job operation log: %+v", got)
	}
}

func TestJobServiceCreateJobReturnsTraceableJobWhenTaskIDPersistenceFailsAfterEnqueue(t *testing.T) {
	ctx := context.Background()
	repo := &fakeJobRepository{
		report:    Report{ID: "report-1", CreatorID: "user-1"},
		taskIDErr: errors.New("postgres unavailable"),
	}
	svc := NewJobService(repo, &fakeTaskEnqueuer{})

	job, err := svc.CreateJob(ctx, RequestContext{UserID: "user-1", RequestID: "req-job-trace"}, CreateJobInput{
		RequestID: "req-job-trace",
		UserID:    "user-1",
		ReportID:  "report-1",
		JobType:   JobTypeContentGeneration,
	})
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	if job.ID == "" || job.ReportID != "report-1" {
		t.Fatalf("expected traceable job metadata, got %+v", job)
	}
}

func TestJobServiceRetryJobDoesNotPersistRawReason(t *testing.T) {
	ctx := context.Background()
	repo := &fakeJobRepository{
		report: Report{ID: "report-1", CreatorID: "user-1"},
		job: ReportJob{
			ID:        "job-1",
			ReportID:  "report-1",
			JobType:   JobTypeContentGeneration,
			RequestID: "req-retry",
		},
	}
	svc := NewJobService(repo, &fakeTaskEnqueuer{})

	rawReason := "retry with prompt=secret https://minio.local/bucket/object?X-Amz-Signature=abc"
	_, err := svc.RetryJob(ctx, RequestContext{UserID: "user-1"}, "job-1", rawReason)
	if err != nil {
		t.Fatalf("RetryJob() error = %v", err)
	}
	if len(repo.operationLogs) != 1 {
		t.Fatalf("operation log count = %d, want 1", len(repo.operationLogs))
	}
	summary := repo.operationLogs[0].ParameterSummary
	if got := summary["reason"]; got == rawReason || strings.Contains(jobTestString(got), "prompt=") || strings.Contains(jobTestString(got), "X-Amz-Signature") {
		t.Fatalf("retry operation log leaked raw reason: %+v", summary)
	}
	if summary["reasonProvided"] != true {
		t.Fatalf("reasonProvided = %v, want true", summary["reasonProvided"])
	}
}

type fakeJobRepository struct {
	report        Report
	job           ReportJob
	reportFile    ReportFile
	operationLogs []OperationLog
	taskIDErr     error
}

func (f *fakeJobRepository) GetReportByID(context.Context, string) (Report, error) {
	return f.report, nil
}

func (f *fakeJobRepository) FindReportJobByID(context.Context, string) (ReportJob, error) {
	return f.job, nil
}

func (f *fakeJobRepository) ListReportJobsByReportID(context.Context, string) ([]ReportJob, error) {
	return nil, nil
}

func (f *fakeJobRepository) CreateReportJob(_ context.Context, value ReportJob) (ReportJob, error) {
	if value.CreatedAt.IsZero() {
		value.CreatedAt = time.Now().UTC()
	}
	return value, nil
}

func (f *fakeJobRepository) UpdateReportJobStatus(context.Context, string, JobStatus, string, string, *time.Time, *time.Time) (ReportJob, error) {
	return ReportJob{}, nil
}

func (f *fakeJobRepository) UpdateJobAsynqTaskID(context.Context, string, string) error {
	if f.taskIDErr != nil {
		return f.taskIDErr
	}
	return nil
}

func (f *fakeJobRepository) CreateReportJobAttempt(_ context.Context, value ReportJobAttempt) (ReportJobAttempt, error) {
	return value, nil
}

func (f *fakeJobRepository) UpdateAttemptAsynqTaskID(context.Context, string, string) error {
	return nil
}

func (f *fakeJobRepository) SetAttemptFailed(context.Context, string, string, string) error {
	return nil
}

func (f *fakeJobRepository) CreateReportFile(_ context.Context, value ReportFile) (ReportFile, error) {
	f.reportFile = value
	return value, nil
}

func (f *fakeJobRepository) UpdateReportFile(_ context.Context, value ReportFile) (ReportFile, error) {
	f.reportFile = value
	return value, nil
}

func (f *fakeJobRepository) ClaimRetry(context.Context, string, string, string, string) (ReportJobAttempt, error) {
	return ReportJobAttempt{ID: "attempt-1", JobID: "job-1"}, nil
}

func (f *fakeJobRepository) ListReportJobAttemptsByJobID(context.Context, string) ([]ReportJobAttempt, error) {
	return nil, nil
}

func (f *fakeJobRepository) ListReportEventsByReportID(context.Context, string) ([]ReportEvent, error) {
	return nil, nil
}

func (f *fakeJobRepository) CreateOperationLog(_ context.Context, log OperationLog) (OperationLog, error) {
	f.operationLogs = append(f.operationLogs, log)
	return log, nil
}

type fakeTaskEnqueuer struct {
	jobType JobType
	err     error
}

func (f *fakeTaskEnqueuer) EnqueueReportJob(_ context.Context, jobType JobType, _, _, _, _ string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.jobType = jobType
	return "task-1", nil
}

func jobTestString(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}
