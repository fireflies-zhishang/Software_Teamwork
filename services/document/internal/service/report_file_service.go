package service

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"
)

const docxContentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

type ReportFileRepository interface {
	GetReportByID(ctx context.Context, id string) (Report, error)
	ListReportSections(ctx context.Context, reportID string) ([]ReportSection, error)
	FindReportJobByID(ctx context.Context, id string) (ReportJob, error)
	CreateReportJob(ctx context.Context, value ReportJob) (ReportJob, error)
	UpdateReportJobStatus(ctx context.Context, id string, status JobStatus, errorCode, errorMessage string, startedAt, finishedAt *time.Time) (ReportJob, error)
	UpdateJobAsynqTaskID(ctx context.Context, id, taskID string) error
	CreateReportJobAttempt(ctx context.Context, value ReportJobAttempt) (ReportJobAttempt, error)
	UpdateAttemptAsynqTaskID(ctx context.Context, attemptID, taskID string) error
	SetAttemptFailed(ctx context.Context, attemptID, errCode, errMsg string) error

	CreateReportFile(ctx context.Context, value ReportFile) (ReportFile, error)
	ListReportFiles(ctx context.Context, filter ReportFileListFilter) ([]ReportFile, int, error)
	GetReportFileByID(ctx context.Context, id string) (ReportFile, error)
	GetReportFileByJobID(ctx context.Context, jobID string) (ReportFile, error)
	UpdateReportFile(ctx context.Context, value ReportFile) (ReportFile, error)
}

type ReportFileTaskEnqueuer interface {
	EnqueueReportJob(ctx context.Context, jobType JobType, jobID, attemptID, requestID, userID string) (string, error)
}

type ReportFileContentClient interface {
	CreateFile(ctx context.Context, reqCtx RequestContext, file UploadedFile) (FileObject, error)
	ReadFileContent(ctx context.Context, reqCtx RequestContext, fileID string) (FileContent, error)
}

type ReportFileGenerator interface {
	GenerateDOCX(ctx context.Context, report Report, sections []ReportSection) ([]byte, error)
}

type ReportFileService struct {
	repo      ReportFileRepository
	files     ReportFileContentClient
	enqueuer  ReportFileTaskEnqueuer
	generator ReportFileGenerator
	now       func() time.Time
}

func NewReportFileService(repo ReportFileRepository, files ReportFileContentClient, enqueuer ReportFileTaskEnqueuer, generator ReportFileGenerator) *ReportFileService {
	return &ReportFileService{
		repo:      repo,
		files:     files,
		enqueuer:  enqueuer,
		generator: generator,
		now:       func() time.Time { return time.Now().UTC() },
	}
}

func (s *ReportFileService) ListReportFiles(ctx context.Context, reqCtx RequestContext, filter ReportFileListFilter) (ReportFileListResult, error) {
	if err := requireGatewayContext(reqCtx); err != nil {
		return ReportFileListResult{}, err
	}
	if strings.TrimSpace(filter.ReportID) != "" {
		if _, err := s.requireReportAccess(ctx, reqCtx, filter.ReportID); err != nil {
			return ReportFileListResult{}, err
		}
	}
	if !reqCtx.IsAdmin() {
		filter.CreatorID = reqCtx.UserID
	}
	filter.Page, filter.PageSize = normalizePage(filter.Page, filter.PageSize)
	items, total, err := s.repo.ListReportFiles(ctx, filter)
	if err != nil {
		return ReportFileListResult{}, dependencyError("list report files", err)
	}
	return ReportFileListResult{
		Items: items,
		Page:  PageMeta{Page: filter.Page, PageSize: filter.PageSize, Total: total},
	}, nil
}

func (s *ReportFileService) CreateReportFile(ctx context.Context, reqCtx RequestContext, input CreateReportFileInput) (ReportFile, error) {
	if err := requireGatewayContext(reqCtx); err != nil {
		return ReportFile{}, err
	}
	reportID := strings.TrimSpace(input.ReportID)
	if reportID == "" {
		return ReportFile{}, ValidationError(map[string]string{"reportId": "is required"})
	}
	format := strings.ToLower(strings.TrimSpace(input.Format))
	if format == "" {
		return ReportFile{}, ValidationError(map[string]string{"format": "is required"})
	}
	if format != ReportFileFormatDOCX {
		return ReportFile{}, ValidationError(map[string]string{"format": "must be docx"})
	}
	report, err := s.requireReportAccess(ctx, reqCtx, reportID)
	if err != nil {
		return ReportFile{}, err
	}
	if report.Status == ReportStatusDeleted || report.DeletedAt != nil {
		return ReportFile{}, NewError(CodeConflict, "report has been deleted", nil)
	}

	now := s.now()
	job := ReportJob{
		ID:          newID(),
		RequestID:   reqCtx.RequestID,
		Source:      "api",
		JobType:     JobTypeReportFileCreation,
		TargetType:  "report_file",
		TargetID:    report.ID,
		QueueName:   "document",
		ReportID:    report.ID,
		TemplateID:  strings.TrimSpace(input.TemplateID),
		Status:      JobStatusPending,
		MaxAttempts: 3,
		CreatedAt:   now,
	}
	createdJob, err := s.repo.CreateReportJob(ctx, job)
	if err != nil {
		return ReportFile{}, dependencyError("create report file job", err)
	}
	attempt := ReportJobAttempt{
		ID:            newID(),
		JobID:         createdJob.ID,
		AttemptNumber: 1,
		TriggerSource: "api",
		Status:        JobStatusPending,
		CreatedAt:     now,
	}
	createdAttempt, err := s.repo.CreateReportJobAttempt(ctx, attempt)
	if err != nil {
		return ReportFile{}, dependencyError("create report file job attempt", err)
	}
	reportFile, err := s.repo.CreateReportFile(ctx, ReportFile{
		ID:        newID(),
		ReportID:  report.ID,
		JobID:     createdJob.ID,
		Filename:  docxFilename(report),
		Format:    ReportFileFormatDOCX,
		Status:    ReportFileStatusPending,
		CreatedBy: reqCtx.UserID,
		CreatedAt: now,
	})
	if err != nil {
		return ReportFile{}, dependencyError("create report file", err)
	}
	if s.enqueuer == nil {
		return reportFile, nil
	}
	taskID, err := s.enqueuer.EnqueueReportJob(ctx, JobTypeReportFileCreation, createdJob.ID, createdAttempt.ID, reqCtx.RequestID, reqCtx.UserID)
	if err != nil {
		finishedAt := s.now()
		_, _ = s.repo.UpdateReportJobStatus(ctx, createdJob.ID, JobStatusFailed, "enqueue_failed", "failed to enqueue task", nil, &finishedAt)
		_ = s.repo.SetAttemptFailed(ctx, createdAttempt.ID, "enqueue_failed", "failed to enqueue task")
		reportFile.Status = ReportFileStatusFailed
		_, _ = s.repo.UpdateReportFile(ctx, reportFile)
		return ReportFile{}, dependencyError("enqueue report file job", err)
	}
	if err := s.repo.UpdateJobAsynqTaskID(ctx, createdJob.ID, taskID); err != nil {
		_ = s.repo.UpdateAttemptAsynqTaskID(ctx, createdAttempt.ID, taskID)
		return reportFile, nil
	}
	_ = s.repo.UpdateAttemptAsynqTaskID(ctx, createdAttempt.ID, taskID)
	return reportFile, nil
}

func (s *ReportFileService) GetReportFile(ctx context.Context, reqCtx RequestContext, id string) (ReportFile, error) {
	if err := requireGatewayContext(reqCtx); err != nil {
		return ReportFile{}, err
	}
	reportFile, err := s.repo.GetReportFileByID(ctx, id)
	if err != nil {
		return ReportFile{}, mapRepositoryReadError(err, "report file not found")
	}
	if _, err := s.requireReportAccess(ctx, reqCtx, reportFile.ReportID); err != nil {
		return ReportFile{}, err
	}
	return reportFile, nil
}

func (s *ReportFileService) ReadReportFileContent(ctx context.Context, reqCtx RequestContext, id string) (FileContent, error) {
	if s.files == nil {
		return FileContent{}, NewError(CodeDependency, "file service is not configured", nil)
	}
	reportFile, err := s.GetReportFile(ctx, reqCtx, id)
	if err != nil {
		return FileContent{}, err
	}
	if reportFile.Status != ReportFileStatusSucceeded {
		return FileContent{}, NewError(CodeConflict, "report file is not ready", nil)
	}
	if strings.TrimSpace(reportFile.FileRef) == "" {
		return FileContent{}, NewError(CodeConflict, "report file content is not ready", nil)
	}
	content, err := s.files.ReadFileContent(ctx, reqCtx, reportFile.FileRef)
	if err != nil {
		return FileContent{}, mapFileError(err)
	}
	if strings.TrimSpace(content.Filename) == "" {
		content.Filename = reportFile.Filename
	}
	if strings.TrimSpace(content.ContentType) == "" {
		content.ContentType = docxContentType
	}
	if content.SizeBytes <= 0 {
		content.SizeBytes = reportFile.FileSize
	}
	return content, nil
}

func (s *ReportFileService) ExecuteReportFileCreation(ctx context.Context, payload ReportFileExecutionPayload) error {
	if s.generator == nil {
		return NewError(CodeDependency, "report file generator is not configured", nil)
	}
	if s.files == nil {
		return NewError(CodeDependency, "file service is not configured", nil)
	}
	reportFile, err := s.repo.GetReportFileByJobID(ctx, payload.JobID)
	if err != nil {
		return mapRepositoryReadError(err, "report file not found")
	}
	reportFile.Status = ReportFileStatusRunning
	if _, err := s.repo.UpdateReportFile(ctx, reportFile); err != nil {
		return dependencyError("mark report file running", err)
	}
	report, err := s.repo.GetReportByID(ctx, reportFile.ReportID)
	if err != nil {
		_ = s.failReportFile(ctx, reportFile, "report_not_found")
		return mapRepositoryReadError(err, "report not found")
	}
	sections, err := s.repo.ListReportSections(ctx, report.ID)
	if err != nil {
		_ = s.failReportFile(ctx, reportFile, "section_load_failed")
		return dependencyError("list report sections", err)
	}
	data, err := s.generator.GenerateDOCX(ctx, report, sections)
	if err != nil {
		_ = s.failReportFile(ctx, reportFile, "docx_generation_failed")
		return dependencyError("generate docx", err)
	}
	file, err := s.files.CreateFile(ctx, RequestContext{
		RequestID: payload.RequestID,
		UserID:    payload.UserID,
	}, UploadedFile{
		Filename:    reportFile.Filename,
		ContentType: docxContentType,
		SizeBytes:   int64(len(data)),
		Content:     bytes.NewReader(data),
	})
	if err != nil {
		_ = s.failReportFile(ctx, reportFile, "file_upload_failed")
		return mapFileError(err)
	}
	reportFile.FileRef = file.ID
	reportFile.FileSize = file.SizeBytes
	reportFile.Status = ReportFileStatusSucceeded
	reportFile.Filename = file.Filename
	if _, err := s.repo.UpdateReportFile(ctx, reportFile); err != nil {
		return dependencyError("mark report file succeeded", err)
	}
	return nil
}

type ReportFileExecutionPayload struct {
	RequestID string
	JobID     string
	UserID    string
}

func (s *ReportFileService) failReportFile(ctx context.Context, reportFile ReportFile, code string) error {
	reportFile.Status = ReportFileStatusFailed
	if _, err := s.repo.UpdateReportFile(ctx, reportFile); err != nil {
		return fmt.Errorf("mark report file failed after %s: %w", code, err)
	}
	return nil
}

func (s *ReportFileService) requireReportAccess(ctx context.Context, reqCtx RequestContext, reportID string) (Report, error) {
	report, err := s.repo.GetReportByID(ctx, reportID)
	if err != nil {
		return Report{}, mapRepositoryReadError(err, "report not found")
	}
	if !reqCtx.CanAccessReport(report) {
		return Report{}, NewError(CodeForbidden, "you do not have access to this report", nil)
	}
	return report, nil
}

func docxFilename(report Report) string {
	name := strings.TrimSpace(report.Name)
	if name == "" {
		name = "report"
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_", "\r", "_", "\n", "_")
	name = strings.TrimSpace(replacer.Replace(name))
	if name == "" {
		name = "report"
	}
	if !strings.HasSuffix(strings.ToLower(name), ".docx") {
		name += ".docx"
	}
	return name
}
