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
	if repo.report.LatestJobID != job.ID || repo.report.Status != ReportStatusContentGenerating {
		t.Fatalf("report generation metadata = %+v, want latest job and content_generating", repo.report)
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

func TestJobServiceCreateJobRejectsDeletedReportForAllJobTypes(t *testing.T) {
	ctx := context.Background()
	deletedAt := time.Now().UTC()
	reportCases := []struct {
		name   string
		report Report
	}{
		{
			name: "deleted status",
			report: Report{
				ID:        "report-1",
				CreatorID: "user-1",
				Status:    ReportStatusDeleted,
			},
		},
		{
			name: "deleted at set",
			report: Report{
				ID:        "report-1",
				CreatorID: "user-1",
				Status:    ReportStatusGenerated,
				DeletedAt: &deletedAt,
			},
		},
	}
	jobTypes := []JobType{
		JobTypeOutlineGeneration,
		JobTypeOutlineRegeneration,
		JobTypeContentGeneration,
		JobTypeContentRegeneration,
		JobTypeSectionRegeneration,
		JobTypeReportFileCreation,
	}

	for _, reportCase := range reportCases {
		for _, jobType := range jobTypes {
			t.Run(reportCase.name+" "+string(jobType), func(t *testing.T) {
				repo := &fakeJobRepository{
					report:   reportCase.report,
					sections: map[string]ReportSection{"section-1": {ID: "section-1", ReportID: "report-1"}},
				}
				enqueuer := &fakeTaskEnqueuer{}
				svc := NewJobService(repo, enqueuer)
				input := CreateJobInput{
					RequestID: "req-deleted",
					UserID:    "user-1",
					ReportID:  "report-1",
					JobType:   jobType,
				}
				if jobType == JobTypeSectionRegeneration {
					input.SectionID = "section-1"
				}

				_, err := svc.CreateJob(ctx, RequestContext{UserID: "user-1"}, input)
				if err == nil {
					t.Fatal("CreateJob() error = nil, want conflict")
				}
				appErr, ok := Classify(err)
				if !ok || appErr.Code != CodeConflict {
					t.Fatalf("CreateJob() error = %v, want conflict", err)
				}
				if repo.createdJob.ID != "" || repo.createdAttempt.ID != "" || enqueuer.jobType != "" || repo.reportFile.ID != "" {
					t.Fatalf("deleted report should not create job/attempt/file or enqueue, job=%+v attempt=%+v file=%+v enqueued=%q", repo.createdJob, repo.createdAttempt, repo.reportFile, enqueuer.jobType)
				}
			})
		}
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

func TestJobServiceCreateSectionRegenerationTargetsSectionAndPersistsGenerationPayload(t *testing.T) {
	ctx := context.Background()
	repo := &fakeJobRepository{
		report:   Report{ID: "report-1", CreatorID: "user-1"},
		sections: map[string]ReportSection{"section-1": {ID: "section-1", ReportID: "report-1"}},
	}
	svc := NewJobService(repo, &fakeTaskEnqueuer{})

	job, err := svc.CreateJob(ctx, RequestContext{UserID: "user-1", RequestID: "req-section"}, CreateJobInput{
		RequestID:    "req-section",
		UserID:       "user-1",
		ReportID:     "report-1",
		JobType:      JobTypeSectionRegeneration,
		SectionID:    "section-1",
		Requirements: "focus on overload risks",
		MaterialIDs:  []string{"material-1", "material-2"},
		Options: map[string]any{
			"knowledgeBaseIds": []any{"kb-1"},
			"topK":             float64(3),
		},
	})
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	if job.TargetType != "section" || job.TargetID != "section-1" {
		t.Fatalf("job target = %s/%s, want section/section-1", job.TargetType, job.TargetID)
	}
	if repo.createdJob.TargetType != "section" || repo.createdJob.TargetID != "section-1" {
		t.Fatalf("persisted job target = %+v", repo.createdJob)
	}
	if repo.createdJob.RequestPayload["requirements"] != "focus on overload risks" {
		t.Fatalf("request payload requirements = %#v", repo.createdJob.RequestPayload)
	}
	target, ok := repo.createdJob.RequestPayload["target"].(map[string]any)
	if !ok || target["scope"] != "section" || target["sectionId"] != "section-1" {
		t.Fatalf("request payload target = %#v", repo.createdJob.RequestPayload["target"])
	}
	materials, ok := repo.createdJob.RequestPayload["materialIds"].([]string)
	if !ok || len(materials) != 2 || materials[0] != "material-1" || materials[1] != "material-2" {
		t.Fatalf("request payload materialIds = %#v", repo.createdJob.RequestPayload["materialIds"])
	}
	options, ok := repo.createdJob.RequestPayload["options"].(map[string]any)
	if !ok || options["topK"] != float64(3) {
		t.Fatalf("request payload options = %#v", repo.createdJob.RequestPayload["options"])
	}
}

func TestJobServiceCreateSectionRegenerationRejectsSectionFromAnotherReport(t *testing.T) {
	ctx := context.Background()
	repo := &fakeJobRepository{
		report:   Report{ID: "report-1", CreatorID: "user-1"},
		sections: map[string]ReportSection{"section-2": {ID: "section-2", ReportID: "other-report"}},
	}
	enqueuer := &fakeTaskEnqueuer{}
	svc := NewJobService(repo, enqueuer)

	_, err := svc.CreateJob(ctx, RequestContext{UserID: "user-1"}, CreateJobInput{
		RequestID: "req-section",
		UserID:    "user-1",
		ReportID:  "report-1",
		JobType:   JobTypeSectionRegeneration,
		SectionID: "section-2",
	})
	if err == nil {
		t.Fatal("CreateJob() error = nil, want not_found")
	}
	appErr, ok := Classify(err)
	if !ok || appErr.Code != CodeNotFound {
		t.Fatalf("CreateJob() error = %v, want not_found", err)
	}
	if repo.createdJob.ID != "" || enqueuer.jobType != "" {
		t.Fatalf("job should not be created or enqueued, job=%+v enqueued=%q", repo.createdJob, enqueuer.jobType)
	}
}

func TestJobServiceCreateJobRejectsSectionTargetForNonSectionRegeneration(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name        string
		jobType     JobType
		targetScope string
		sectionID   string
	}{
		{name: "outline generation explicit section scope", jobType: JobTypeOutlineGeneration, targetScope: "section", sectionID: "section-1"},
		{name: "outline regeneration explicit section scope", jobType: JobTypeOutlineRegeneration, targetScope: "section", sectionID: "section-1"},
		{name: "content generation explicit section scope", jobType: JobTypeContentGeneration, targetScope: "section", sectionID: "section-1"},
		{name: "content regeneration section id without scope", jobType: JobTypeContentRegeneration, sectionID: "section-1"},
		{name: "report file creation explicit section scope", jobType: JobTypeReportFileCreation, targetScope: "section", sectionID: "section-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeJobRepository{
				report:   Report{ID: "report-1", CreatorID: "user-1"},
				sections: map[string]ReportSection{"section-1": {ID: "section-1", ReportID: "report-1"}},
			}
			enqueuer := &fakeTaskEnqueuer{}
			svc := NewJobService(repo, enqueuer)

			_, err := svc.CreateJob(ctx, RequestContext{UserID: "user-1"}, CreateJobInput{
				RequestID:   "req-section-target",
				UserID:      "user-1",
				ReportID:    "report-1",
				JobType:     tt.jobType,
				TargetScope: tt.targetScope,
				SectionID:   tt.sectionID,
			})
			if err == nil {
				t.Fatal("CreateJob() error = nil, want validation error")
			}
			appErr, ok := Classify(err)
			if !ok || appErr.Code != CodeValidation {
				t.Fatalf("CreateJob() error = %v, want validation_error", err)
			}
			if repo.createdJob.ID != "" || repo.createdAttempt.ID != "" || repo.reportFile.ID != "" || enqueuer.jobType != "" {
				t.Fatalf("section target should not create job/attempt/file or enqueue, job=%+v attempt=%+v file=%+v enqueued=%q", repo.createdJob, repo.createdAttempt, repo.reportFile, enqueuer.jobType)
			}
		})
	}
}

func TestJobServiceCreateGenerationJobMarksReportFailedWhenEnqueueFails(t *testing.T) {
	ctx := context.Background()
	repo := &fakeJobRepository{
		report: Report{ID: "report-1", CreatorID: "user-1"},
	}
	svc := NewJobService(repo, &fakeTaskEnqueuer{err: errors.New("redis unavailable")})

	_, err := svc.CreateJob(ctx, RequestContext{UserID: "user-1"}, CreateJobInput{
		RequestID: "req-1",
		UserID:    "user-1",
		ReportID:  "report-1",
		JobType:   JobTypeOutlineGeneration,
	})
	if err == nil {
		t.Fatal("CreateJob() error = nil, want enqueue error")
	}
	if repo.report.LatestJobID != repo.createdJob.ID || repo.report.Status != ReportStatusFailed {
		t.Fatalf("report generation metadata = %+v, want failed latest job", repo.report)
	}
}

func TestJobServiceCreateGenerationJobRollsBackWhenInitialStateFails(t *testing.T) {
	ctx := context.Background()
	repo := &fakeJobRepository{
		report:              Report{ID: "report-1", CreatorID: "user-1"},
		generationStatusErr: NewError(CodeConflict, "report has been deleted", nil),
	}
	enqueuer := &fakeTaskEnqueuer{}
	svc := NewJobService(repo, enqueuer)

	_, err := svc.CreateJob(ctx, RequestContext{UserID: "user-1"}, CreateJobInput{
		RequestID: "req-race",
		UserID:    "user-1",
		ReportID:  "report-1",
		JobType:   JobTypeOutlineGeneration,
	})
	if err == nil {
		t.Fatal("CreateJob() error = nil, want conflict")
	}
	appErr, ok := Classify(err)
	if !ok || appErr.Code != CodeConflict {
		t.Fatalf("CreateJob() error = %v, want conflict", err)
	}
	if repo.createdJob.ID != "" || repo.createdAttempt.ID != "" || repo.reportFile.ID != "" || enqueuer.jobType != "" {
		t.Fatalf("failed initial state should not leave job/attempt/file or enqueue, job=%+v attempt=%+v file=%+v enqueued=%q", repo.createdJob, repo.createdAttempt, repo.reportFile, enqueuer.jobType)
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

func TestRetryJobRejectsDeletedReport(t *testing.T) {
	ctx := context.Background()
	deletedAt := time.Now().UTC()
	repo := &fakeJobRepository{
		report: Report{ID: "report-1", CreatorID: "user-1", Status: ReportStatusDeleted, DeletedAt: &deletedAt},
		job: ReportJob{
			ID:        "job-1",
			ReportID:  "report-1",
			JobType:   JobTypeReportFileCreation,
			RequestID: "req-retry-deleted",
		},
	}
	svc := NewJobService(repo, &fakeTaskEnqueuer{})

	_, err := svc.RetryJob(ctx, RequestContext{UserID: "user-1"}, "job-1", "retry after delete")
	if err == nil {
		t.Fatal("expected error retrying job for deleted report, got nil")
	}
	appErr, ok := Classify(err)
	if !ok || appErr.Code != CodeConflict {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestRetryJobRejectsReportWithDeletedAtSet(t *testing.T) {
	ctx := context.Background()
	deletedAt := time.Now().UTC()
	repo := &fakeJobRepository{
		// Status may lag behind DeletedAt during soft-delete; both must be guarded.
		report: Report{ID: "report-1", CreatorID: "user-1", Status: ReportStatusExporting, DeletedAt: &deletedAt},
		job: ReportJob{
			ID:       "job-1",
			ReportID: "report-1",
			JobType:  JobTypeReportFileCreation,
		},
	}
	svc := NewJobService(repo, &fakeTaskEnqueuer{})

	_, err := svc.RetryJob(ctx, RequestContext{UserID: "user-1"}, "job-1", "")
	if err == nil {
		t.Fatal("expected error when report has DeletedAt set, got nil")
	}
	appErr, ok := Classify(err)
	if !ok || appErr.Code != CodeConflict {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

type fakeJobRepository struct {
	report              Report
	job                 ReportJob
	createdJob          ReportJob
	createdAttempt      ReportJobAttempt
	reportFile          ReportFile
	sections            map[string]ReportSection
	operationLogs       []OperationLog
	taskIDErr           error
	generationStatusErr error
}

func (f *fakeJobRepository) WithinJobTx(ctx context.Context, fn func(JobRepository) error) error {
	snapshot := *f
	if f.sections != nil {
		snapshot.sections = make(map[string]ReportSection, len(f.sections))
		for id, section := range f.sections {
			snapshot.sections[id] = section
		}
	}
	snapshot.operationLogs = append([]OperationLog(nil), f.operationLogs...)

	if err := fn(f); err != nil {
		*f = snapshot
		return err
	}
	return nil
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
	f.createdJob = value
	return value, nil
}

func (f *fakeJobRepository) UpdateReportJobStatus(_ context.Context, id string, status JobStatus, errorCode, errorMessage string, startedAt, finishedAt *time.Time) (ReportJob, error) {
	job := f.createdJob
	if job.ID != id {
		job = f.job
	}
	if job.ID == "" {
		job = ReportJob{ID: id, ReportID: f.report.ID}
	}
	job.Status = status
	job.ErrorCode = errorCode
	job.ErrorMessage = errorMessage
	if startedAt != nil {
		job.StartedAt = startedAt
	}
	if finishedAt != nil {
		job.FinishedAt = finishedAt
	}
	if isReportGenerationJobType(job.JobType) {
		updatedAt := time.Now().UTC()
		if finishedAt != nil {
			updatedAt = *finishedAt
		} else if startedAt != nil {
			updatedAt = *startedAt
		}
		if err := f.UpdateReportGenerationStatus(context.Background(), job.ReportID, job.ID, job.JobType, status, updatedAt); err != nil {
			return ReportJob{}, err
		}
	}
	if f.createdJob.ID == id {
		f.createdJob = job
	} else {
		f.job = job
	}
	return job, nil
}

func (f *fakeJobRepository) UpdateReportGenerationStatus(_ context.Context, reportID, jobID string, jobType JobType, status JobStatus, updatedAt time.Time) error {
	if f.generationStatusErr != nil {
		return f.generationStatusErr
	}
	if f.report.ID != reportID {
		return NewError(CodeNotFound, "report not found", nil)
	}
	switch status {
	case JobStatusPending, JobStatusRunning:
		if jobType == JobTypeOutlineGeneration || jobType == JobTypeOutlineRegeneration {
			f.report.Status = ReportStatusOutlineGenerating
		} else {
			f.report.Status = ReportStatusContentGenerating
		}
	case JobStatusSucceeded:
		if jobType == JobTypeOutlineGeneration || jobType == JobTypeOutlineRegeneration {
			f.report.Status = ReportStatusOutlineGenerated
		} else {
			f.report.Status = ReportStatusGenerated
			generatedAt := updatedAt
			f.report.GeneratedAt = &generatedAt
		}
	case JobStatusPartialSucceeded:
		if jobType == JobTypeOutlineGeneration || jobType == JobTypeOutlineRegeneration {
			f.report.Status = ReportStatusOutlineGenerated
		} else {
			f.report.Status = ReportStatusGenerated
			generatedAt := updatedAt
			f.report.GeneratedAt = &generatedAt
		}
	case JobStatusFailed, JobStatusCanceled:
		f.report.Status = ReportStatusFailed
	}
	f.report.LatestJobID = jobID
	f.report.UpdatedAt = updatedAt
	return nil
}

func (f *fakeJobRepository) UpdateJobAsynqTaskID(context.Context, string, string) error {
	if f.taskIDErr != nil {
		return f.taskIDErr
	}
	return nil
}

func (f *fakeJobRepository) CreateReportJobAttempt(_ context.Context, value ReportJobAttempt) (ReportJobAttempt, error) {
	f.createdAttempt = value
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

func (f *fakeJobRepository) GetReportSectionByID(_ context.Context, id string) (ReportSection, error) {
	if f.sections == nil {
		return ReportSection{}, NewError(CodeNotFound, "report section not found", nil)
	}
	section, ok := f.sections[id]
	if !ok {
		return ReportSection{}, NewError(CodeNotFound, "report section not found", nil)
	}
	return section, nil
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
