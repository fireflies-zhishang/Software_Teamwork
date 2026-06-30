package repository

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/document/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresRepositoryPersistsReportJobAttemptAndEvent(t *testing.T) {
	databaseURL := os.Getenv("DOCUMENT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DOCUMENT_TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool := newTestPool(t, ctx, databaseURL)
	defer pool.Close()
	applyMigration(t, ctx, pool)

	repo := NewPostgresRepository(pool)
	now := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)

	reportType, err := repo.UpsertReportType(ctx, service.ReportType{
		Code:      "integration_report",
		Name:      "Integration Report",
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("UpsertReportType() error = %v", err)
	}
	if reportType.Code != "integration_report" {
		t.Fatalf("reportType.Code = %q", reportType.Code)
	}

	report, err := repo.CreateReport(ctx, service.Report{
		ID:         "00000000-0000-0000-0000-000000000101",
		Name:       "June baseline report",
		ReportType: reportType.Code,
		Topic:      "baseline",
		Status:     service.ReportStatusDraft,
		Source:     "backend",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("CreateReport() error = %v", err)
	}

	job, err := repo.CreateReportJob(ctx, service.ReportJob{
		ID:          "00000000-0000-0000-0000-000000000201",
		RequestID:   "req_integration",
		Source:      "api",
		JobType:     service.JobTypeOutlineGeneration,
		TargetType:  "report",
		TargetID:    report.ID,
		QueueName:   "document",
		ReportID:    report.ID,
		Status:      service.JobStatusPending,
		RetryCount:  0,
		MaxAttempts: 3,
		CreatedAt:   now,
	})
	if err != nil {
		t.Fatalf("CreateReportJob() error = %v", err)
	}
	if job.AsynqTaskID != "" {
		t.Fatalf("new job should not require asynq task id, got %q", job.AsynqTaskID)
	}
	if job.MaxAttempts != 3 {
		t.Fatalf("MaxAttempts = %d, want 3", job.MaxAttempts)
	}

	attempt, err := repo.CreateReportJobAttempt(ctx, service.ReportJobAttempt{
		ID:            "00000000-0000-0000-0000-000000000301",
		JobID:         job.ID,
		AttemptNumber: 1,
		TriggerSource: "api",
		Status:        service.JobStatusPending,
		CreatedAt:     now,
	})
	if err != nil {
		t.Fatalf("CreateReportJobAttempt() error = %v", err)
	}
	if attempt.AttemptNumber != 1 {
		t.Fatalf("AttemptNumber = %d, want 1", attempt.AttemptNumber)
	}

	event, err := repo.CreateReportEvent(ctx, service.ReportEvent{
		ID:        "00000000-0000-0000-0000-000000000401",
		ReportID:  report.ID,
		JobID:     job.ID,
		EventType: "job.created",
		Message:   "job created",
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateReportEvent() error = %v", err)
	}
	if event.EventType != "job.created" {
		t.Fatalf("EventType = %q", event.EventType)
	}

	foundJob, err := repo.FindReportJobByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("FindReportJobByID() error = %v", err)
	}
	if foundJob.Status != service.JobStatusPending {
		t.Fatalf("foundJob.Status = %q", foundJob.Status)
	}
	if err := repo.SetJobRunning(ctx, job.ID); err != nil {
		t.Fatalf("SetJobRunning() error = %v", err)
	}
	if err := repo.SetJobSucceeded(ctx, job.ID); err != nil {
		t.Fatalf("SetJobSucceeded() error = %v", err)
	}
	completedJob, err := repo.FindReportJobByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("FindReportJobByID() after completion error = %v", err)
	}
	if completedJob.Progress["completed"] != float64(1) || completedJob.Progress["total"] != float64(1) {
		t.Fatalf("completedJob.Progress = %#v, want completed/total 1/1", completedJob.Progress)
	}
	events, err := repo.ListReportEventsByReportID(ctx, report.ID)
	if err != nil {
		t.Fatalf("ListReportEventsByReportID() error = %v", err)
	}
	if len(events) < 3 {
		t.Fatalf("event count = %d, want at least 3", len(events))
	}
	foundSucceeded := false
	for _, event := range events {
		if event.EventType == "job.succeeded" {
			foundSucceeded = true
			break
		}
	}
	if !foundSucceeded {
		t.Fatalf("events = %+v, want job.succeeded event", events)
	}
}

func TestPostgresRepositoryTerminalStatusPreservesDetailedProgress(t *testing.T) {
	databaseURL := os.Getenv("DOCUMENT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DOCUMENT_TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool := newTestPool(t, ctx, databaseURL)
	defer pool.Close()
	applyMigration(t, ctx, pool)

	repo := NewPostgresRepository(pool)
	now := time.Date(2026, 6, 30, 11, 0, 0, 0, time.UTC)
	reportType, err := repo.UpsertReportType(ctx, service.ReportType{
		Code:      "progress_report",
		Name:      "Progress Report",
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("UpsertReportType() error = %v", err)
	}
	report, err := repo.CreateReport(ctx, service.Report{
		ID:         "00000000-0000-0000-0000-000000000801",
		Name:       "progress report",
		ReportType: reportType.Code,
		Topic:      "progress",
		Status:     service.ReportStatusDraft,
		Source:     "backend",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("CreateReport() error = %v", err)
	}

	tests := []struct {
		name             string
		jobID            string
		markFinal        func(context.Context, string) error
		wantStatus       service.JobStatus
		wantReportStatus service.ReportStatus
		wantGeneratedAt  bool
	}{
		{
			name:             "partial succeeded",
			jobID:            "00000000-0000-0000-0000-000000000802",
			markFinal:        repo.SetJobPartialSucceeded,
			wantStatus:       service.JobStatusPartialSucceeded,
			wantReportStatus: service.ReportStatusGenerated,
			wantGeneratedAt:  true,
		},
		{
			name:             "succeeded",
			jobID:            "00000000-0000-0000-0000-000000000803",
			markFinal:        repo.SetJobSucceeded,
			wantStatus:       service.JobStatusSucceeded,
			wantReportStatus: service.ReportStatusGenerated,
			wantGeneratedAt:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job, err := repo.CreateReportJob(ctx, service.ReportJob{
				ID:          tt.jobID,
				RequestID:   "req_progress",
				Source:      "api",
				JobType:     service.JobTypeContentGeneration,
				TargetType:  "report",
				TargetID:    report.ID,
				QueueName:   "document",
				ReportID:    report.ID,
				Status:      service.JobStatusPending,
				MaxAttempts: 3,
				CreatedAt:   now,
			})
			if err != nil {
				t.Fatalf("CreateReportJob() error = %v", err)
			}
			if err := repo.SetJobRunning(ctx, job.ID); err != nil {
				t.Fatalf("SetJobRunning() error = %v", err)
			}
			if err := repo.UpdateReportJobProgress(ctx, job.ID, 1, 2); err != nil {
				t.Fatalf("UpdateReportJobProgress() error = %v", err)
			}
			if err := tt.markFinal(ctx, job.ID); err != nil {
				t.Fatalf("mark final status error = %v", err)
			}

			completedJob, err := repo.FindReportJobByID(ctx, job.ID)
			if err != nil {
				t.Fatalf("FindReportJobByID() error = %v", err)
			}
			if completedJob.Status != tt.wantStatus {
				t.Fatalf("Status = %q, want %q", completedJob.Status, tt.wantStatus)
			}
			if completedJob.Progress["completed"] != float64(1) || completedJob.Progress["total"] != float64(2) {
				t.Fatalf("Progress = %#v, want completed/total 1/2", completedJob.Progress)
			}
			updatedReport, err := repo.GetReportByID(ctx, report.ID)
			if err != nil {
				t.Fatalf("GetReportByID() error = %v", err)
			}
			if updatedReport.Status != tt.wantReportStatus || updatedReport.LatestJobID != job.ID {
				t.Fatalf("report metadata = %+v, want status %q latest job %q", updatedReport, tt.wantReportStatus, job.ID)
			}
			if (updatedReport.GeneratedAt != nil) != tt.wantGeneratedAt {
				t.Fatalf("report generatedAt = %v, want present=%v", updatedReport.GeneratedAt, tt.wantGeneratedAt)
			}
		})
	}
}

func TestPostgresRepositoryUpdatesReportGenerationLifecycle(t *testing.T) {
	databaseURL := os.Getenv("DOCUMENT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DOCUMENT_TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool := newTestPool(t, ctx, databaseURL)
	defer pool.Close()
	applyMigration(t, ctx, pool)

	repo := NewPostgresRepository(pool)
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	reportType, err := repo.UpsertReportType(ctx, service.ReportType{
		Code:      "generation_lifecycle_report",
		Name:      "Generation Lifecycle Report",
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("UpsertReportType() error = %v", err)
	}
	report, err := repo.CreateReport(ctx, service.Report{
		ID:         "00000000-0000-0000-0000-000000000901",
		Name:       "generation lifecycle",
		ReportType: reportType.Code,
		Topic:      "lifecycle",
		Status:     service.ReportStatusDraft,
		Source:     "backend",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("CreateReport() error = %v", err)
	}

	outlineJob, err := repo.CreateReportJob(ctx, service.ReportJob{
		ID:          "00000000-0000-0000-0000-000000000902",
		RequestID:   "req_generation_lifecycle",
		Source:      "api",
		JobType:     service.JobTypeOutlineGeneration,
		TargetType:  "report",
		TargetID:    report.ID,
		QueueName:   "document",
		ReportID:    report.ID,
		Status:      service.JobStatusPending,
		MaxAttempts: 3,
		CreatedAt:   now,
	})
	if err != nil {
		t.Fatalf("CreateReportJob(outline) error = %v", err)
	}
	if err := repo.UpdateReportGenerationStatus(ctx, report.ID, outlineJob.ID, outlineJob.JobType, service.JobStatusPending, now); err != nil {
		t.Fatalf("UpdateReportGenerationStatus(outline pending) error = %v", err)
	}
	updated, err := repo.GetReportByID(ctx, report.ID)
	if err != nil {
		t.Fatalf("GetReportByID(outline pending) error = %v", err)
	}
	if updated.Status != service.ReportStatusOutlineGenerating || updated.LatestJobID != outlineJob.ID {
		t.Fatalf("outline pending report metadata = %+v", updated)
	}
	if err := repo.SetJobSucceeded(ctx, outlineJob.ID); err != nil {
		t.Fatalf("SetJobSucceeded(outline) error = %v", err)
	}
	updated, err = repo.GetReportByID(ctx, report.ID)
	if err != nil {
		t.Fatalf("GetReportByID(outline succeeded) error = %v", err)
	}
	if updated.Status != service.ReportStatusOutlineGenerated || updated.GeneratedAt != nil {
		t.Fatalf("outline succeeded report metadata = %+v", updated)
	}

	contentJob, err := repo.CreateReportJob(ctx, service.ReportJob{
		ID:          "00000000-0000-0000-0000-000000000903",
		RequestID:   "req_generation_lifecycle",
		Source:      "api",
		JobType:     service.JobTypeContentGeneration,
		TargetType:  "report",
		TargetID:    report.ID,
		QueueName:   "document",
		ReportID:    report.ID,
		Status:      service.JobStatusPending,
		MaxAttempts: 3,
		CreatedAt:   now,
	})
	if err != nil {
		t.Fatalf("CreateReportJob(content) error = %v", err)
	}
	if err := repo.UpdateReportGenerationStatus(ctx, report.ID, contentJob.ID, contentJob.JobType, service.JobStatusPending, now); err != nil {
		t.Fatalf("UpdateReportGenerationStatus(content pending) error = %v", err)
	}
	updated, err = repo.GetReportByID(ctx, report.ID)
	if err != nil {
		t.Fatalf("GetReportByID(content pending) error = %v", err)
	}
	if updated.Status != service.ReportStatusContentGenerating || updated.LatestJobID != contentJob.ID {
		t.Fatalf("content pending report metadata = %+v", updated)
	}
	if err := repo.SetJobSucceeded(ctx, contentJob.ID); err != nil {
		t.Fatalf("SetJobSucceeded(content) error = %v", err)
	}
	updated, err = repo.GetReportByID(ctx, report.ID)
	if err != nil {
		t.Fatalf("GetReportByID(content succeeded) error = %v", err)
	}
	if updated.Status != service.ReportStatusGenerated || updated.LatestJobID != contentJob.ID || updated.GeneratedAt == nil {
		t.Fatalf("content succeeded report metadata = %+v", updated)
	}
}

func TestPostgresRepositoryClaimRetryUsesNextAttemptNumber(t *testing.T) {
	databaseURL := os.Getenv("DOCUMENT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DOCUMENT_TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool := newTestPool(t, ctx, databaseURL)
	defer pool.Close()
	applyMigration(t, ctx, pool)

	repo := NewPostgresRepository(pool)
	now := time.Date(2026, 6, 29, 10, 30, 0, 0, time.UTC)

	reportType, err := repo.UpsertReportType(ctx, service.ReportType{
		Code:      "retry_attempt_report",
		Name:      "Retry Attempt Report",
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("UpsertReportType() error = %v", err)
	}
	report, err := repo.CreateReport(ctx, service.Report{
		ID:         "00000000-0000-0000-0000-000000000701",
		Name:       "retry attempt report",
		ReportType: reportType.Code,
		Topic:      "retry",
		Status:     service.ReportStatusDraft,
		Source:     "backend",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("CreateReport() error = %v", err)
	}
	job, err := repo.CreateReportJob(ctx, service.ReportJob{
		ID:           "00000000-0000-0000-0000-000000000702",
		RequestID:    "req_retry_attempt",
		Source:       "api",
		JobType:      service.JobTypeOutlineGeneration,
		TargetType:   "report",
		TargetID:     report.ID,
		QueueName:    "document",
		ReportID:     report.ID,
		Status:       service.JobStatusFailed,
		ErrorCode:    "worker_failed",
		ErrorMessage: "initial attempt failed",
		RetryCount:   0,
		MaxAttempts:  3,
		CreatedAt:    now,
	})
	if err != nil {
		t.Fatalf("CreateReportJob() error = %v", err)
	}
	if _, err := repo.CreateReportJobAttempt(ctx, service.ReportJobAttempt{
		ID:            "00000000-0000-0000-0000-000000000703",
		JobID:         job.ID,
		AttemptNumber: 1,
		TriggerSource: "api",
		Status:        service.JobStatusFailed,
		ErrorCode:     "worker_failed",
		ErrorMessage:  "initial attempt failed",
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("CreateReportJobAttempt() error = %v", err)
	}

	attempt, err := repo.ClaimRetry(ctx, job.ID, "00000000-0000-0000-0000-000000000704", "user", "try again")
	if err != nil {
		t.Fatalf("ClaimRetry() error = %v", err)
	}
	if attempt.AttemptNumber != 2 {
		t.Fatalf("AttemptNumber = %d, want 2", attempt.AttemptNumber)
	}
	if attempt.TriggerSource != "user" || attempt.Reason != "try again" {
		t.Fatalf("unexpected retry attempt metadata: %+v", attempt)
	}

	updatedJob, err := repo.FindReportJobByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("FindReportJobByID() error = %v", err)
	}
	if updatedJob.RetryCount != 1 || updatedJob.Status != service.JobStatusPending {
		t.Fatalf("updated job retry/status = %d/%q, want 1/%q", updatedJob.RetryCount, updatedJob.Status, service.JobStatusPending)
	}
	updatedReport, err := repo.GetReportByID(ctx, report.ID)
	if err != nil {
		t.Fatalf("GetReportByID() error = %v", err)
	}
	if updatedReport.Status != service.ReportStatusOutlineGenerating || updatedReport.LatestJobID != job.ID {
		t.Fatalf("retry report metadata = %+v, want outline_generating latest job", updatedReport)
	}
}

func TestPostgresRepositoryWithinJobTxRollsBackOnGenerationStatusConflict(t *testing.T) {
	databaseURL := os.Getenv("DOCUMENT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DOCUMENT_TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool := newTestPool(t, ctx, databaseURL)
	defer pool.Close()
	applyMigration(t, ctx, pool)

	repo := NewPostgresRepository(pool)
	now := time.Date(2026, 6, 29, 10, 45, 0, 0, time.UTC)
	reportType, err := repo.UpsertReportType(ctx, service.ReportType{
		Code:      "job_tx_conflict_report",
		Name:      "Job Tx Conflict Report",
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("UpsertReportType() error = %v", err)
	}
	report, err := repo.CreateReport(ctx, service.Report{
		ID:         "00000000-0000-0000-0000-000000000801",
		Name:       "job tx conflict",
		ReportType: reportType.Code,
		Topic:      "job tx conflict",
		Status:     service.ReportStatusDraft,
		Source:     "backend",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("CreateReport() error = %v", err)
	}
	if _, err := repo.SoftDeleteReport(ctx, report.ID, now.Add(time.Minute)); err != nil {
		t.Fatalf("SoftDeleteReport() error = %v", err)
	}

	err = repo.WithinJobTx(ctx, func(txRepo service.JobRepository) error {
		job, err := txRepo.CreateReportJob(ctx, service.ReportJob{
			ID:          "00000000-0000-0000-0000-000000000802",
			RequestID:   "req_job_tx_conflict",
			Source:      "api",
			JobType:     service.JobTypeOutlineGeneration,
			TargetType:  "report",
			TargetID:    report.ID,
			QueueName:   "document",
			ReportID:    report.ID,
			Status:      service.JobStatusPending,
			MaxAttempts: 3,
			CreatedAt:   now,
		})
		if err != nil {
			return err
		}
		return txRepo.UpdateReportGenerationStatus(ctx, report.ID, job.ID, job.JobType, service.JobStatusPending, now)
	})
	appErr, ok := service.Classify(err)
	if !ok || appErr.Code != service.CodeConflict {
		t.Fatalf("WithinJobTx() error = %v, want conflict", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM report_jobs WHERE id = '00000000-0000-0000-0000-000000000802'`).Scan(&count); err != nil {
		t.Fatalf("count rolled back report job: %v", err)
	}
	if count != 0 {
		t.Fatalf("rolled back job count = %d, want 0", count)
	}
}

func TestPostgresRepositoryWithinGenerationTxRollsBackOutlineAndSections(t *testing.T) {
	databaseURL := os.Getenv("DOCUMENT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DOCUMENT_TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool := newTestPool(t, ctx, databaseURL)
	defer pool.Close()
	applyMigration(t, ctx, pool)

	repo := NewPostgresRepository(pool)
	now := time.Date(2026, 6, 29, 10, 50, 0, 0, time.UTC)
	reportType, err := repo.UpsertReportType(ctx, service.ReportType{
		Code:      "generation_tx_report",
		Name:      "Generation Tx Report",
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("UpsertReportType() error = %v", err)
	}
	report, err := repo.CreateReport(ctx, service.Report{
		ID:         "00000000-0000-0000-0000-000000000811",
		Name:       "generation tx rollback",
		ReportType: reportType.Code,
		Topic:      "generation tx rollback",
		Status:     service.ReportStatusDraft,
		Source:     "backend",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("CreateReport() error = %v", err)
	}
	oldOutline, err := repo.CreateReportOutline(ctx, service.ReportOutline{
		ID:        "00000000-0000-0000-0000-000000000812",
		ReportID:  report.ID,
		Sections:  []service.ReportOutlineNode{{ID: "old-node", Title: "Old outline", Level: 1}},
		Version:   1,
		Source:    service.OutlineSourceManual,
		IsCurrent: true,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateReportOutline() old error = %v", err)
	}

	rollbackErr := errors.New("rollback generation transaction")
	err = repo.WithinGenerationTx(ctx, func(txRepo service.ReportGenerationRepository) error {
		newOutline, err := txRepo.CreateReportOutline(ctx, service.ReportOutline{
			ID:        "00000000-0000-0000-0000-000000000813",
			ReportID:  report.ID,
			Sections:  []service.ReportOutlineNode{{ID: "new-node", Title: "New outline", Level: 1}},
			Version:   2,
			Source:    service.OutlineSourceAI,
			IsCurrent: true,
			CreatedAt: now,
			UpdatedAt: now,
		})
		if err != nil {
			return err
		}
		if _, err := txRepo.CreateReportSection(ctx, service.ReportSection{
			ID:               "00000000-0000-0000-0000-000000000814",
			ReportID:         report.ID,
			OutlineID:        newOutline.ID,
			SectionPath:      "00000000-0000-0000-0000-000000000814",
			Title:            "Partial skeleton",
			Level:            1,
			SortOrder:        0,
			SectionType:      service.SectionTypeText,
			GenerationStatus: service.JobStatusPending,
			ContentSource:    service.ContentSourceAI,
			Version:          1,
			CreatedAt:        now,
			UpdatedAt:        now,
		}); err != nil {
			return err
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("WithinGenerationTx() error = %v, want rollbackErr", err)
	}

	outlines, err := repo.ListReportOutlines(ctx, report.ID)
	if err != nil {
		t.Fatalf("ListReportOutlines() error = %v", err)
	}
	if len(outlines) != 1 || outlines[0].ID != oldOutline.ID || !outlines[0].IsCurrent {
		t.Fatalf("outlines after rollback = %+v, want previous current outline only", outlines)
	}
	sections, err := repo.ListReportSections(ctx, report.ID)
	if err != nil {
		t.Fatalf("ListReportSections() error = %v", err)
	}
	if len(sections) != 0 {
		t.Fatalf("section count after rollback = %d, want 0", len(sections))
	}
}

func TestPostgresRepositoryWithinTxRollsBack(t *testing.T) {
	databaseURL := os.Getenv("DOCUMENT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DOCUMENT_TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool := newTestPool(t, ctx, databaseURL)
	defer pool.Close()
	applyMigration(t, ctx, pool)

	repo := NewPostgresRepository(pool)
	now := time.Date(2026, 6, 29, 11, 0, 0, 0, time.UTC)
	reportType, err := repo.UpsertReportType(ctx, service.ReportType{
		Code:      "tx_report",
		Name:      "Transactional Report",
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("UpsertReportType() error = %v", err)
	}
	report, err := repo.CreateReport(ctx, service.Report{
		ID:         "00000000-0000-0000-0000-000000000501",
		Name:       "rollback report",
		ReportType: reportType.Code,
		Topic:      "rollback",
		Status:     service.ReportStatusDraft,
		Source:     "backend",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("CreateReport() error = %v", err)
	}

	rollbackErr := errors.New("rollback requested")
	err = repo.WithinTx(ctx, func(txRepo service.ReportRepository) error {
		pgRepo, ok := txRepo.(*PostgresRepository)
		if !ok {
			t.Fatalf("tx repository type = %T, want *PostgresRepository", txRepo)
		}
		_, err := pgRepo.CreateReportJob(ctx, service.ReportJob{
			ID:          "00000000-0000-0000-0000-000000000601",
			Source:      "api",
			JobType:     service.JobTypeOutlineGeneration,
			TargetType:  "report",
			TargetID:    report.ID,
			QueueName:   "document",
			ReportID:    report.ID,
			Status:      service.JobStatusPending,
			MaxAttempts: 3,
			CreatedAt:   now,
		})
		if err != nil {
			return err
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("WithinTx() error = %v, want rollbackErr", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM report_jobs WHERE id = '00000000-0000-0000-0000-000000000601'`).Scan(&count); err != nil {
		t.Fatalf("count rolled back job: %v", err)
	}
	if count != 0 {
		t.Fatalf("rolled back job count = %d, want 0", count)
	}
}

func newTestPool(t *testing.T, ctx context.Context, databaseURL string) *pgxpool.Pool {
	t.Helper()
	schema := "document_test_" + strings.ReplaceAll(time.Now().Format("20060102150405.000000000"), ".", "_")
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect admin pool: %v", err)
	}
	defer admin.Close()
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
	})

	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse pool config: %v", err)
	}
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect schema pool: %v", err)
	}
	return pool
}

func applyMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	files, err := filepath.Glob("../../migrations/*.sql")
	if err != nil {
		t.Fatalf("find migrations: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no migrations found")
	}
	sort.Strings(files)
	for _, file := range files {
		applyMigrationFile(t, ctx, pool, file)
	}
}

func applyMigrationFile(t *testing.T, ctx context.Context, pool *pgxpool.Pool, file string) {
	t.Helper()
	sqlBytes, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read migration %s: %v", file, err)
	}
	parts := strings.Split(string(sqlBytes), "-- +goose Down")
	up := strings.TrimPrefix(parts[0], "-- +goose Up")
	for _, stmt := range strings.Split(up, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" || strings.HasPrefix(stmt, "--") {
			continue
		}
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("apply migration %s statement %q: %v", file, stmt, err)
		}
	}
}
