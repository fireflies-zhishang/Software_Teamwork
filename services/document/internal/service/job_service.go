package service

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type JobRepository interface {
	WithinJobTx(ctx context.Context, fn func(JobRepository) error) error
	GetReportByID(ctx context.Context, id string) (Report, error)
	FindReportJobByID(ctx context.Context, id string) (ReportJob, error)
	ListReportJobsByReportID(ctx context.Context, reportID string) ([]ReportJob, error)
	CreateReportJob(ctx context.Context, value ReportJob) (ReportJob, error)
	UpdateReportJobStatus(ctx context.Context, id string, status JobStatus, errorCode, errorMessage string, startedAt, finishedAt *time.Time) (ReportJob, error)
	UpdateReportGenerationStatus(ctx context.Context, reportID, jobID string, jobType JobType, status JobStatus, updatedAt time.Time) error
	UpdateJobAsynqTaskID(ctx context.Context, id, taskID string) error
	CreateReportJobAttempt(ctx context.Context, value ReportJobAttempt) (ReportJobAttempt, error)
	UpdateAttemptAsynqTaskID(ctx context.Context, attemptID, taskID string) error
	SetAttemptFailed(ctx context.Context, attemptID, errCode, errMsg string) error
	CreateReportFile(ctx context.Context, value ReportFile) (ReportFile, error)
	UpdateReportFile(ctx context.Context, value ReportFile) (ReportFile, error)
	GetReportSectionByID(ctx context.Context, id string) (ReportSection, error)
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
	RequestID    string
	UserID       string
	ReportID     string
	JobType      JobType
	TargetScope  string
	SectionID    string
	Requirements string
	MaterialIDs  []string
	Options      map[string]any
	Retrieval    map[string]any
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
	if report.Status == ReportStatusDeleted || report.DeletedAt != nil {
		return ReportJob{}, NewError(CodeConflict, "report has been deleted", nil)
	}
	targetType, targetID, err := resolveCreateJobTarget(input)
	if err != nil {
		return ReportJob{}, err
	}
	if err := s.validateCreateJobTarget(ctx, input.ReportID, targetType, targetID); err != nil {
		return ReportJob{}, err
	}
	now := time.Now().UTC()
	job := ReportJob{
		ID:             newID(),
		RequestID:      input.RequestID,
		Source:         "api",
		JobType:        input.JobType,
		TargetType:     targetType,
		TargetID:       targetID,
		QueueName:      "document",
		ReportID:       input.ReportID,
		RequestPayload: createJobRequestPayload(input, targetType, targetID),
		Status:         JobStatusPending,
		MaxAttempts:    3,
		CreatedAt:      now,
	}
	var created ReportJob
	attempt := ReportJobAttempt{
		ID:            newID(),
		JobID:         job.ID,
		AttemptNumber: 1,
		TriggerSource: "api",
		Status:        JobStatusPending,
		CreatedAt:     now,
	}
	var reportFile ReportFile
	if err := s.repo.WithinJobTx(ctx, func(txRepo JobRepository) error {
		var err error
		created, err = txRepo.CreateReportJob(ctx, job)
		if err != nil {
			return fmt.Errorf("create report job: %w", err)
		}
		if isReportGenerationJobType(created.JobType) {
			if err := txRepo.UpdateReportGenerationStatus(ctx, created.ReportID, created.ID, created.JobType, JobStatusPending, now); err != nil {
				if _, ok := Classify(err); ok {
					return err
				}
				return dependencyError("update report generation status", err)
			}
		}
		// Create attempt #1 so the attempts list reflects every execution, including the first.
		attempt.JobID = created.ID
		attempt, err = txRepo.CreateReportJobAttempt(ctx, attempt)
		if err != nil {
			return fmt.Errorf("create initial attempt: %w", err)
		}
		if input.JobType == JobTypeReportFileCreation {
			reportFile, err = txRepo.CreateReportFile(ctx, ReportFile{
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
				return fmt.Errorf("create report file: %w", err)
			}
		}
		return nil
	}); err != nil {
		return ReportJob{}, err
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

func (s *JobService) validateCreateJobTarget(ctx context.Context, reportID, targetType, targetID string) error {
	if targetType != "section" {
		return nil
	}
	section, err := s.repo.GetReportSectionByID(ctx, targetID)
	if err != nil {
		return mapRepositoryReadError(err, "report section not found")
	}
	if section.ReportID != reportID {
		return NewError(CodeNotFound, "report section not found", nil)
	}
	return nil
}

func resolveCreateJobTarget(input CreateJobInput) (string, string, error) {
	scope := strings.TrimSpace(input.TargetScope)
	sectionID := strings.TrimSpace(input.SectionID)
	if scope == "" {
		scope = "report"
	}
	if input.JobType == JobTypeSectionRegeneration {
		if sectionID == "" {
			return "", "", ValidationError(map[string]string{"target.sectionId": "is required for section_regeneration"})
		}
		scope = "section"
	} else if scope == "section" || sectionID != "" {
		return "", "", ValidationError(map[string]string{"target.scope": "section scope is only supported for section_regeneration"})
	}
	switch scope {
	case "report":
		return "report", input.ReportID, nil
	case "section":
		if sectionID == "" {
			return "", "", ValidationError(map[string]string{"target.sectionId": "is required"})
		}
		return "section", sectionID, nil
	default:
		return "", "", ValidationError(map[string]string{"target.scope": "unsupported target scope"})
	}
}

func isReportGenerationJobType(jobType JobType) bool {
	switch jobType {
	case JobTypeOutlineGeneration, JobTypeOutlineRegeneration, JobTypeContentGeneration, JobTypeContentRegeneration, JobTypeSectionRegeneration:
		return true
	default:
		return false
	}
}

func createJobRequestPayload(input CreateJobInput, targetType, targetID string) map[string]any {
	payload := map[string]any{}
	if requirements := strings.TrimSpace(input.Requirements); requirements != "" {
		payload["requirements"] = requirements
	}
	if len(input.MaterialIDs) > 0 {
		payload["materialIds"] = append([]string(nil), input.MaterialIDs...)
	}
	if len(input.Options) > 0 {
		payload["options"] = cloneJSONLikeMap(input.Options)
	}
	if len(input.Retrieval) > 0 {
		payload["retrieval"] = cloneJSONLikeMap(input.Retrieval)
	}
	if input.TargetScope != "" || input.SectionID != "" || targetType != "report" {
		target := map[string]any{
			"scope": targetType,
		}
		if targetType == "section" {
			target["sectionId"] = targetID
		}
		payload["target"] = target
	}
	return payload
}

func cloneJSONLikeMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	clone := make(map[string]any, len(input))
	for key, value := range input {
		clone[key] = value
	}
	return clone
}

func (s *JobService) RetryJob(ctx context.Context, rctx RequestContext, id, reason string) (ReportJobAttempt, error) {
	job, err := s.repo.FindReportJobByID(ctx, id)
	if err != nil {
		return ReportJobAttempt{}, err
	}
	report, err := s.requireReportAccess(ctx, rctx, job.ReportID)
	if err != nil {
		return ReportJobAttempt{}, err
	}
	// Guard: do not allow retrying export jobs for reports that have been
	// soft-deleted after the original job was submitted.
	if report.Status == ReportStatusDeleted || report.DeletedAt != nil {
		return ReportJobAttempt{}, NewError(CodeConflict, "cannot retry job for a deleted report", nil)
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
