package service

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type JobRepository interface {
	GetReportByID(ctx context.Context, id string) (Report, error)
	FindReportJobByID(ctx context.Context, id string) (ReportJob, error)
	ListReportJobsByReportID(ctx context.Context, reportID string) ([]ReportJob, error)
	CreateReportJob(ctx context.Context, value ReportJob) (ReportJob, error)
	UpdateReportJobStatus(ctx context.Context, id string, status JobStatus, errorCode, errorMessage string, startedAt, finishedAt *time.Time) (ReportJob, error)
	UpdateJobAsynqTaskID(ctx context.Context, id, taskID string) error
	CreateReportJobAttempt(ctx context.Context, value ReportJobAttempt) (ReportJobAttempt, error)
	UpdateAttemptAsynqTaskID(ctx context.Context, attemptID, taskID string) error
	SetAttemptFailed(ctx context.Context, attemptID, errCode, errMsg string) error
	CreateReportFile(ctx context.Context, value ReportFile) (ReportFile, error)
	UpdateReportFile(ctx context.Context, value ReportFile) (ReportFile, error)
	// ClaimRetry atomically validates status/retry_count, increments retry_count,
	// and inserts the attempt — preventing double-retry races.
	ClaimRetry(ctx context.Context, jobID, attemptID, triggerSource, reason string) (ReportJobAttempt, error)
	ListReportJobAttemptsByJobID(ctx context.Context, jobID string) ([]ReportJobAttempt, error)
	ListReportEventsByReportID(ctx context.Context, reportID string) ([]ReportEvent, error)
}

// TaskEnqueuer submits async tasks to the queue.
type TaskEnqueuer interface {
	EnqueueReportJob(ctx context.Context, jobType JobType, jobID, attemptID, requestID, userID string) (string, error)
}

type JobService struct {
	repo     JobRepository
	enqueuer TaskEnqueuer
}

func NewJobService(repo JobRepository, enqueuer TaskEnqueuer) *JobService {
	return &JobService{repo: repo, enqueuer: enqueuer}
}

func (s *JobService) requireReportAccess(ctx context.Context, rctx RequestContext, reportID string) (Report, error) {
	report, err := s.repo.GetReportByID(ctx, reportID)
	if err != nil {
		return Report{}, mapRepositoryReadError(err, "report not found")
	}
	if !rctx.CanAccessReport(report) {
		return Report{}, NewError(CodeForbidden, "you do not have access to this report", nil)
	}
	return report, nil
}

func (s *JobService) GetJob(ctx context.Context, rctx RequestContext, id string) (ReportJob, error) {
	job, err := s.repo.FindReportJobByID(ctx, id)
	if err != nil {
		return ReportJob{}, err
	}
	if _, err := s.requireReportAccess(ctx, rctx, job.ReportID); err != nil {
		return ReportJob{}, err
	}
	return job, nil
}

func (s *JobService) ListJobs(ctx context.Context, rctx RequestContext, reportID string) ([]ReportJob, error) {
	if _, err := s.requireReportAccess(ctx, rctx, reportID); err != nil {
		return nil, err
	}
	return s.repo.ListReportJobsByReportID(ctx, reportID)
}

type CreateJobInput struct {
	RequestID string
	UserID    string
	ReportID  string
	JobType   JobType
}

func (s *JobService) CreateJob(ctx context.Context, rctx RequestContext, input CreateJobInput) (ReportJob, error) {
	if !isSupportedReportJobType(input.JobType) {
		return ReportJob{}, ValidationError(map[string]string{
			"jobType": "unsupported report job type",
		})
	}
	report, err := s.requireReportAccess(ctx, rctx, input.ReportID)
	if err != nil {
		return ReportJob{}, err
	}
	if input.JobType == JobTypeReportFileCreation && (report.Status == ReportStatusDeleted || report.DeletedAt != nil) {
		return ReportJob{}, NewError(CodeConflict, "report has been deleted", nil)
	}
	now := time.Now().UTC()
	job := ReportJob{
		ID:          newID(),
		RequestID:   input.RequestID,
		Source:      "api",
		JobType:     input.JobType,
		TargetType:  "report",
		TargetID:    input.ReportID,
		QueueName:   "document",
		ReportID:    input.ReportID,
		Status:      JobStatusPending,
		MaxAttempts: 3,
		CreatedAt:   now,
	}
	created, err := s.repo.CreateReportJob(ctx, job)
	if err != nil {
		return ReportJob{}, fmt.Errorf("create report job: %w", err)
	}
	// Create attempt #1 so the attempts list reflects every execution, including the first.
	attempt := ReportJobAttempt{
		ID:            newID(),
		JobID:         created.ID,
		AttemptNumber: 1,
		TriggerSource: "api",
		Status:        JobStatusPending,
		CreatedAt:     now,
	}
	attempt, err = s.repo.CreateReportJobAttempt(ctx, attempt)
	if err != nil {
		return ReportJob{}, fmt.Errorf("create initial attempt: %w", err)
	}
	var reportFile ReportFile
	if input.JobType == JobTypeReportFileCreation {
		reportFile, err = s.repo.CreateReportFile(ctx, ReportFile{
			ID:        newID(),
			ReportID:  report.ID,
			JobID:     created.ID,
			Filename:  docxFilename(report),
			Format:    ReportFileFormatDOCX,
			Status:    ReportFileStatusPending,
			CreatedBy: rctx.UserID,
			CreatedAt: now,
		})
		if err != nil {
			return ReportJob{}, fmt.Errorf("create report file: %w", err)
		}
	}
	taskID, err := s.enqueuer.EnqueueReportJob(ctx, input.JobType, created.ID, attempt.ID, input.RequestID, input.UserID)
	if err != nil {
		finishedAt := time.Now().UTC()
		_, _ = s.repo.UpdateReportJobStatus(ctx, created.ID, JobStatusFailed, "enqueue_failed", "failed to enqueue task", nil, &finishedAt)
		_ = s.repo.SetAttemptFailed(ctx, attempt.ID, "enqueue_failed", "failed to enqueue task")
		if input.JobType == JobTypeReportFileCreation && reportFile.ID != "" {
			reportFile.Status = ReportFileStatusFailed
			_, _ = s.repo.UpdateReportFile(ctx, reportFile)
		}
		recordJobFailureIfSupported(ctx, s.repo, rctx, created, input.RequestID, "failed to enqueue task", map[string]any{
			"reportId":  created.ReportID,
			"attemptId": attempt.ID,
		})
		return ReportJob{}, fmt.Errorf("enqueue job task: %w", err)
	}
	if err := s.repo.UpdateJobAsynqTaskID(ctx, created.ID, taskID); err != nil {
		_ = s.repo.UpdateAttemptAsynqTaskID(ctx, attempt.ID, taskID)
		return created, nil
	}
	_ = s.repo.UpdateAttemptAsynqTaskID(ctx, attempt.ID, taskID)
	created.AsynqTaskID = taskID
	recordOperationIfSupported(ctx, s.repo, OperationLog{
		OperatorID:      rctx.UserID,
		OperatorName:    rctx.UserID,
		OperationType:   operationForJobType(created.JobType),
		TargetType:      "job",
		TargetID:        created.ID,
		RequestID:       input.RequestID,
		RequestSource:   requestSource(rctx, created.Source),
		OperationResult: OperationResultSucceeded,
		ParameterSummary: map[string]any{
			"jobType":    created.JobType,
			"targetType": created.TargetType,
		},
		Metadata: map[string]any{
			"reportId": created.ReportID,
			"taskId":   created.AsynqTaskID,
		},
		CreatedAt: now,
	})
	return created, nil
}

func (s *JobService) RetryJob(ctx context.Context, rctx RequestContext, id, reason string) (ReportJobAttempt, error) {
	job, err := s.repo.FindReportJobByID(ctx, id)
	if err != nil {
		return ReportJobAttempt{}, err
	}
	if _, err := s.requireReportAccess(ctx, rctx, job.ReportID); err != nil {
		return ReportJobAttempt{}, err
	}
	// ClaimRetry atomically validates state and increments retry_count in one transaction.
	attempt, err := s.repo.ClaimRetry(ctx, job.ID, newID(), "user", reason)
	if err != nil {
		return ReportJobAttempt{}, err
	}
	taskID, err := s.enqueuer.EnqueueReportJob(ctx, job.JobType, job.ID, attempt.ID, job.RequestID, rctx.UserID)
	if err != nil {
		// Compensate: ClaimRetry already committed (job=pending, attempt=pending).
		// Mark both as failed so the job is retryable again.
		finishedAt := time.Now().UTC()
		_, _ = s.repo.UpdateReportJobStatus(ctx, job.ID, JobStatusFailed, "enqueue_failed", "failed to enqueue retry task", nil, &finishedAt)
		_ = s.repo.SetAttemptFailed(ctx, attempt.ID, "enqueue_failed", "failed to enqueue retry task")
		recordJobFailureIfSupported(ctx, s.repo, rctx, job, job.RequestID, "failed to enqueue retry task", map[string]any{
			"reportId":  job.ReportID,
			"attemptId": attempt.ID,
		})
		return ReportJobAttempt{}, fmt.Errorf("enqueue retry task: %w", err)
	}
	_ = s.repo.UpdateAttemptAsynqTaskID(ctx, attempt.ID, taskID)
	recordOperationIfSupported(ctx, s.repo, OperationLog{
		OperatorID:      rctx.UserID,
		OperatorName:    rctx.UserID,
		OperationType:   OperationRetryReportJob,
		TargetType:      "job",
		TargetID:        job.ID,
		RequestID:       job.RequestID,
		RequestSource:   requestSource(rctx, "api"),
		OperationResult: OperationResultSucceeded,
		ParameterSummary: map[string]any{
			"jobType":        job.JobType,
			"reasonProvided": strings.TrimSpace(reason) != "",
		},
		Metadata: map[string]any{
			"attemptId": attempt.ID,
			"taskId":    taskID,
		},
		CreatedAt: time.Now().UTC(),
	})
	return attempt, nil
}

func (s *JobService) ListAttempts(ctx context.Context, rctx RequestContext, jobID string) ([]ReportJobAttempt, error) {
	job, err := s.repo.FindReportJobByID(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if _, err := s.requireReportAccess(ctx, rctx, job.ReportID); err != nil {
		return nil, err
	}
	return s.repo.ListReportJobAttemptsByJobID(ctx, jobID)
}

func (s *JobService) ListEvents(ctx context.Context, rctx RequestContext, reportID string) ([]ReportEvent, error) {
	if _, err := s.requireReportAccess(ctx, rctx, reportID); err != nil {
		return nil, err
	}
	return s.repo.ListReportEventsByReportID(ctx, reportID)
}

func isSupportedReportJobType(jobType JobType) bool {
	switch jobType {
	case JobTypeOutlineGeneration,
		JobTypeOutlineRegeneration,
		JobTypeContentGeneration,
		JobTypeContentRegeneration,
		JobTypeSectionRegeneration,
		JobTypeReportFileCreation:
		return true
	default:
		return false
	}
}

func operationForJobType(jobType JobType) string {
	switch jobType {
	case JobTypeOutlineGeneration:
		return OperationOutlineGeneration
	case JobTypeOutlineRegeneration:
		return OperationOutlineRegeneration
	case JobTypeContentGeneration:
		return OperationContentGeneration
	case JobTypeContentRegeneration:
		return OperationContentRegeneration
	case JobTypeSectionRegeneration:
		return OperationSectionRegeneration
	case JobTypeReportFileCreation:
		return OperationReportFileCreation
	default:
		return OperationCreateReportJob
	}
}

func recordJobFailureIfSupported(ctx context.Context, recorder any, rctx RequestContext, job ReportJob, requestID, message string, metadata map[string]any) {
	recordOperationIfSupported(ctx, recorder, OperationLog{
		OperatorID:      rctx.UserID,
		OperatorName:    rctx.UserID,
		OperationType:   OperationReportJobFailed,
		TargetType:      "job",
		TargetID:        job.ID,
		RequestID:       requestID,
		RequestSource:   requestSource(rctx, job.Source),
		OperationResult: OperationResultFailed,
		ErrorMessage:    message,
		ParameterSummary: map[string]any{
			"jobType":    job.JobType,
			"targetType": job.TargetType,
		},
		Metadata:  metadata,
		CreatedAt: time.Now().UTC(),
	})
}
