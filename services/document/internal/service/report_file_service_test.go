package service

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type fakeReportFileRepository struct {
	reports   map[string]Report
	sections  []ReportSection
	files     map[string]ReportFile
	jobs      map[string]ReportJob
	attempts  map[string]ReportJobAttempt
	taskIDErr error
}

func newFakeReportFileRepository() *fakeReportFileRepository {
	return &fakeReportFileRepository{
		reports:  map[string]Report{},
		files:    map[string]ReportFile{},
		jobs:     map[string]ReportJob{},
		attempts: map[string]ReportJobAttempt{},
	}
}

func (f *fakeReportFileRepository) GetReportByID(_ context.Context, id string) (Report, error) {
	report, ok := f.reports[id]
	if !ok {
		return Report{}, NewError(CodeNotFound, "report not found", nil)
	}
	return report, nil
}

func (f *fakeReportFileRepository) ListReportSections(context.Context, string) ([]ReportSection, error) {
	return f.sections, nil
}

func (f *fakeReportFileRepository) FindReportJobByID(_ context.Context, id string) (ReportJob, error) {
	job, ok := f.jobs[id]
	if !ok {
		return ReportJob{}, NewError(CodeNotFound, "report job not found", nil)
	}
	return job, nil
}

func (f *fakeReportFileRepository) CreateReportJob(_ context.Context, value ReportJob) (ReportJob, error) {
	f.jobs[value.ID] = value
	return value, nil
}

func (f *fakeReportFileRepository) UpdateReportJobStatus(_ context.Context, id string, status JobStatus, errorCode, errorMessage string, startedAt, finishedAt *time.Time) (ReportJob, error) {
	job := f.jobs[id]
	job.Status = status
	job.ErrorCode = errorCode
	job.ErrorMessage = errorMessage
	job.StartedAt = startedAt
	job.FinishedAt = finishedAt
	f.jobs[id] = job
	return job, nil
}

func (f *fakeReportFileRepository) UpdateJobAsynqTaskID(_ context.Context, id, taskID string) error {
	if f.taskIDErr != nil {
		return f.taskIDErr
	}
	job := f.jobs[id]
	job.AsynqTaskID = taskID
	f.jobs[id] = job
	return nil
}

func (f *fakeReportFileRepository) CreateReportJobAttempt(_ context.Context, value ReportJobAttempt) (ReportJobAttempt, error) {
	f.attempts[value.ID] = value
	return value, nil
}

func (f *fakeReportFileRepository) UpdateAttemptAsynqTaskID(_ context.Context, attemptID, taskID string) error {
	attempt := f.attempts[attemptID]
	attempt.AsynqTaskID = taskID
	f.attempts[attemptID] = attempt
	return nil
}

func (f *fakeReportFileRepository) SetAttemptFailed(_ context.Context, attemptID, errCode, errMsg string) error {
	attempt := f.attempts[attemptID]
	attempt.Status = JobStatusFailed
	attempt.ErrorCode = errCode
	attempt.ErrorMessage = errMsg
	f.attempts[attemptID] = attempt
	return nil
}

func (f *fakeReportFileRepository) CreateReportFile(_ context.Context, value ReportFile) (ReportFile, error) {
	f.files[value.ID] = value
	return value, nil
}

func (f *fakeReportFileRepository) ListReportFiles(_ context.Context, filter ReportFileListFilter) ([]ReportFile, int, error) {
	var result []ReportFile
	for _, file := range f.files {
		if filter.ReportID != "" && file.ReportID != filter.ReportID {
			continue
		}
		if filter.CreatorID != "" && file.CreatedBy != filter.CreatorID {
			continue
		}
		result = append(result, file)
	}
	return result, len(result), nil
}

func (f *fakeReportFileRepository) GetReportFileByID(_ context.Context, id string) (ReportFile, error) {
	file, ok := f.files[id]
	if !ok {
		return ReportFile{}, NewError(CodeNotFound, "report file not found", nil)
	}
	return file, nil
}

func (f *fakeReportFileRepository) GetReportFileByJobID(_ context.Context, jobID string) (ReportFile, error) {
	for _, file := range f.files {
		if file.JobID == jobID {
			return file, nil
		}
	}
	return ReportFile{}, NewError(CodeNotFound, "report file not found", nil)
}

func (f *fakeReportFileRepository) UpdateReportFile(_ context.Context, value ReportFile) (ReportFile, error) {
	f.files[value.ID] = value
	return value, nil
}

type fakeReportFileQueue struct {
	called bool
}

func (f *fakeReportFileQueue) EnqueueReportJob(context.Context, JobType, string, string, string, string) (string, error) {
	f.called = true
	return "task-1", nil
}

type fakeReportFileContentClient struct {
	content FileContent
	created UploadedFile
}

func (f *fakeReportFileContentClient) CreateFile(_ context.Context, _ RequestContext, file UploadedFile) (FileObject, error) {
	f.created = file
	return FileObject{ID: "file-internal-1", Filename: file.Filename, ContentType: file.ContentType, SizeBytes: file.SizeBytes}, nil
}

func (f *fakeReportFileContentClient) ReadFileContent(context.Context, RequestContext, string) (FileContent, error) {
	return f.content, nil
}

func TestCreateReportFileCreatesPendingJobAndSafeMetadata(t *testing.T) {
	repo := newFakeReportFileRepository()
	repo.reports["report-1"] = Report{ID: "report-1", Name: "June / Report", CreatorID: "user-1", Status: ReportStatusGenerated}
	queue := &fakeReportFileQueue{}
	svc := NewReportFileService(repo, &fakeReportFileContentClient{}, queue, NewSimpleDOCXGenerator())

	file, err := svc.CreateReportFile(context.Background(), RequestContext{UserID: "user-1", RequestID: "req-1"}, CreateReportFileInput{
		ReportID: "report-1",
		Format:   "docx",
	})
	if err != nil {
		t.Fatalf("CreateReportFile() error = %v", err)
	}
	if file.Status != ReportFileStatusPending || file.FileRef != "" {
		t.Fatalf("unexpected report file metadata: %+v", file)
	}
	if file.Filename != "June _ Report.docx" {
		t.Fatalf("filename = %q", file.Filename)
	}
	if !queue.called {
		t.Fatal("expected queue handoff")
	}
}

func TestCreateReportFileReturnsMetadataWhenTaskIDPersistenceFailsAfterEnqueue(t *testing.T) {
	repo := newFakeReportFileRepository()
	repo.taskIDErr = errors.New("postgres unavailable")
	repo.reports["report-1"] = Report{ID: "report-1", Name: "Traceable", CreatorID: "user-1", Status: ReportStatusGenerated}
	queue := &fakeReportFileQueue{}
	svc := NewReportFileService(repo, &fakeReportFileContentClient{}, queue, NewSimpleDOCXGenerator())

	file, err := svc.CreateReportFile(context.Background(), RequestContext{UserID: "user-1", RequestID: "req-1"}, CreateReportFileInput{
		ReportID: "report-1",
		Format:   "docx",
	})
	if err != nil {
		t.Fatalf("CreateReportFile() error = %v", err)
	}
	if file.ID == "" || file.JobID == "" {
		t.Fatalf("expected traceable report file metadata, got %+v", file)
	}
	if !queue.called {
		t.Fatal("expected queue handoff")
	}
	foundAttemptTaskID := false
	for _, attempt := range repo.attempts {
		if attempt.JobID == file.JobID && attempt.AsynqTaskID == "task-1" {
			foundAttemptTaskID = true
		}
	}
	if !foundAttemptTaskID {
		t.Fatalf("expected attempt task id to remain traceable, attempts=%+v", repo.attempts)
	}
}

func TestCreateReportFileRequiresFormat(t *testing.T) {
	repo := newFakeReportFileRepository()
	repo.reports["report-1"] = Report{ID: "report-1", CreatorID: "user-1"}
	svc := NewReportFileService(repo, &fakeReportFileContentClient{}, nil, nil)

	_, err := svc.CreateReportFile(context.Background(), RequestContext{UserID: "user-1"}, CreateReportFileInput{ReportID: "report-1"})
	appErr, ok := Classify(err)
	if !ok || appErr.Code != CodeValidation || appErr.Fields["format"] == "" {
		t.Fatalf("expected format validation error, got %v", err)
	}
}

func TestReadReportFileContentRejectsPendingFile(t *testing.T) {
	repo := newFakeReportFileRepository()
	repo.reports["report-1"] = Report{ID: "report-1", CreatorID: "user-1"}
	repo.files["rf-1"] = ReportFile{ID: "rf-1", ReportID: "report-1", Status: ReportFileStatusPending}
	svc := NewReportFileService(repo, &fakeReportFileContentClient{}, nil, nil)

	_, err := svc.ReadReportFileContent(context.Background(), RequestContext{UserID: "user-1"}, "rf-1")
	appErr, ok := Classify(err)
	if !ok || appErr.Code != CodeConflict {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestExecuteReportFileCreationUsesSavedSectionsAndStoresFileRef(t *testing.T) {
	repo := newFakeReportFileRepository()
	repo.reports["report-1"] = Report{ID: "report-1", Name: "Saved report", CreatorID: "user-1"}
	repo.sections = []ReportSection{{Title: "Edited section", Content: "final edited content"}}
	repo.files["rf-1"] = ReportFile{ID: "rf-1", ReportID: "report-1", JobID: "job-1", Filename: "Saved report.docx", Format: ReportFileFormatDOCX, Status: ReportFileStatusPending}
	files := &fakeReportFileContentClient{}
	svc := NewReportFileService(repo, files, nil, NewSimpleDOCXGenerator())

	if err := svc.ExecuteReportFileCreation(context.Background(), ReportFileExecutionPayload{JobID: "job-1", UserID: "user-1"}); err != nil {
		t.Fatalf("ExecuteReportFileCreation() error = %v", err)
	}
	updated := repo.files["rf-1"]
	if updated.Status != ReportFileStatusSucceeded || updated.FileRef != "file-internal-1" || updated.FileSize == 0 {
		t.Fatalf("unexpected updated file: %+v", updated)
	}
	data, err := io.ReadAll(files.created.Content)
	if err != nil {
		t.Fatalf("read generated content: %v", err)
	}
	if !docxContains(t, data, "final edited content") {
		t.Fatal("generated DOCX did not include saved section content")
	}
}

func TestSimpleDOCXGeneratorCreatesWordPackage(t *testing.T) {
	data, err := NewSimpleDOCXGenerator().GenerateDOCX(context.Background(), Report{Name: "Inspection"}, []ReportSection{{Title: "Summary", Content: "all clear"}})
	if err != nil {
		t.Fatalf("GenerateDOCX() error = %v", err)
	}
	if !docxContains(t, data, "all clear") {
		t.Fatal("DOCX package did not include section content")
	}
}

func docxContains(t *testing.T, data []byte, value string) bool {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open docx zip: %v", err)
	}
	for _, file := range reader.File {
		if file.Name != "word/document.xml" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open document.xml: %v", err)
		}
		defer rc.Close()
		content, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read document.xml: %v", err)
		}
		return strings.Contains(string(content), value)
	}
	return false
}
