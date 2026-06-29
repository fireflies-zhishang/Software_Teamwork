package service_test

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/file/internal/platform/storage"
	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/file/internal/repository"
	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/file/internal/service"
)

func TestUploadDocumentStoresMetadataAndContent(t *testing.T) {
	repo := repository.NewMemoryRepository()
	store := storage.NewMemoryStore()
	now := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)
	documents := service.New(repo, store,
		service.WithClock(func() time.Time { return now }),
		service.WithIDGenerator(func(prefix string) (string, error) { return prefix + "_test", nil }),
	)

	doc, err := documents.UploadDocument(context.Background(), actorContext(), service.UploadDocumentInput{
		KnowledgeBaseID: "kb_123",
		FileName:        "policy.pdf",
		ContentType:     "application/pdf",
		SizeBytes:       int64(len("content")),
		Tags:            []string{" policy ", "inspection", "policy"},
		Content:         strings.NewReader("content"),
	})
	if err != nil {
		t.Fatalf("UploadDocument() error = %v", err)
	}
	if doc.ID != "doc_test" {
		t.Fatalf("document id = %q", doc.ID)
	}
	if doc.Name != "policy.pdf" {
		t.Fatalf("document name = %q", doc.Name)
	}
	if doc.Status != service.DocumentStatusUploaded {
		t.Fatalf("status = %q", doc.Status)
	}
	if got, want := strings.Join(doc.Tags, ","), "policy,inspection"; got != want {
		t.Fatalf("tags = %q, want %q", got, want)
	}
	if !doc.CreatedAt.Equal(now) {
		t.Fatalf("createdAt = %s", doc.CreatedAt)
	}

	content, err := documents.GetDocumentContent(context.Background(), actorContext(), doc.ID)
	if err != nil {
		t.Fatalf("GetDocumentContent() error = %v", err)
	}
	defer content.Body.Close()
	body, err := io.ReadAll(content.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(body) != "content" {
		t.Fatalf("content = %q", string(body))
	}
}

func TestCreateFileComputesChecksumAndStoresContent(t *testing.T) {
	files := newTestService(t)
	created, err := files.CreateFile(context.Background(), internalContext(), service.CreateFileInput{
		FileName:    "policy.pdf",
		ContentType: "application/pdf",
		SizeBytes:   int64(len("content")),
		Content:     strings.NewReader("content"),
	})
	if err != nil {
		t.Fatalf("CreateFile() error = %v", err)
	}
	if created.ID != "file_1" {
		t.Fatalf("file id = %q", created.ID)
	}
	if created.Filename != "policy.pdf" || created.ContentType != "application/pdf" || created.SizeBytes != int64(len("content")) {
		t.Fatalf("file metadata = %+v", created)
	}
	if created.ChecksumSHA256 != "ed7002b439e9ac845f22357d822bac1444730fbdb6016d3ec9432297b9ec9f73" {
		t.Fatalf("checksum = %q", created.ChecksumSHA256)
	}
	if created.StorageObjectKey == "" {
		t.Fatal("storage object key is empty")
	}

	content, err := files.GetFileContent(context.Background(), internalContext(), created.ID)
	if err != nil {
		t.Fatalf("GetFileContent() error = %v", err)
	}
	defer content.Body.Close()
	body, err := io.ReadAll(content.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(body) != "content" {
		t.Fatalf("content = %q", string(body))
	}
}

func TestCreateFileRejectsChecksumMismatch(t *testing.T) {
	files := newTestService(t)
	_, err := files.CreateFile(context.Background(), internalContext(), service.CreateFileInput{
		FileName:       "policy.pdf",
		ContentType:    "application/pdf",
		SizeBytes:      int64(len("content")),
		ChecksumSHA256: strings.Repeat("0", 64),
		Content:        strings.NewReader("content"),
	})
	if !hasCode(err, service.CodeValidation) {
		t.Fatalf("CreateFile() error = %v, want validation_error", err)
	}
}

func TestDeleteFileHidesMetadataAndContent(t *testing.T) {
	files := newTestService(t)
	created, err := files.CreateFile(context.Background(), internalContext(), service.CreateFileInput{
		FileName:    "policy.pdf",
		ContentType: "application/pdf",
		SizeBytes:   int64(len("content")),
		Content:     strings.NewReader("content"),
	})
	if err != nil {
		t.Fatalf("CreateFile() error = %v", err)
	}
	if err := files.DeleteFile(context.Background(), internalContext(), created.ID); err != nil {
		t.Fatalf("DeleteFile() error = %v", err)
	}
	if _, err := files.GetFile(context.Background(), internalContext(), created.ID); !hasCode(err, service.CodeNotFound) {
		t.Fatalf("GetFile() error = %v, want not_found", err)
	}
	if _, err := files.GetFileContent(context.Background(), internalContext(), created.ID); !hasCode(err, service.CodeNotFound) {
		t.Fatalf("GetFileContent() error = %v, want not_found", err)
	}
}

func TestCreateFileRequiresInternalCaller(t *testing.T) {
	files := newTestService(t)
	_, err := files.CreateFile(context.Background(), service.RequestContext{}, service.CreateFileInput{
		FileName:    "policy.pdf",
		ContentType: "application/pdf",
		SizeBytes:   int64(len("content")),
		Content:     strings.NewReader("content"),
	})
	if !hasCode(err, service.CodeUnauthorized) {
		t.Fatalf("CreateFile() error = %v, want unauthorized", err)
	}
}
func TestUpdateDocumentReplacesTags(t *testing.T) {
	documents := newTestService(t)
	doc := uploadTestDocument(t, documents)

	updated, err := documents.UpdateDocument(context.Background(), actorContext(), service.UpdateDocumentInput{
		DocumentID: doc.ID,
		Tags:       []string{"new", "new", "reviewed"},
	})
	if err != nil {
		t.Fatalf("UpdateDocument() error = %v", err)
	}
	if got, want := strings.Join(updated.Tags, ","), "new,reviewed"; got != want {
		t.Fatalf("tags = %q, want %q", got, want)
	}
}

func TestDeleteDocumentHidesMetadataAndContent(t *testing.T) {
	documents := newTestService(t)
	doc := uploadTestDocument(t, documents)

	if err := documents.DeleteDocument(context.Background(), actorContext(), doc.ID); err != nil {
		t.Fatalf("DeleteDocument() error = %v", err)
	}
	if _, err := documents.GetDocument(context.Background(), actorContext(), doc.ID); !hasCode(err, service.CodeNotFound) {
		t.Fatalf("GetDocument() error = %v, want not_found", err)
	}
	if _, err := documents.GetDocumentContent(context.Background(), actorContext(), doc.ID); !hasCode(err, service.CodeNotFound) {
		t.Fatalf("GetDocumentContent() error = %v, want not_found", err)
	}
}

func TestUploadDocumentRequiresActor(t *testing.T) {
	documents := newTestService(t)
	_, err := documents.UploadDocument(context.Background(), service.RequestContext{}, service.UploadDocumentInput{
		KnowledgeBaseID: "kb_123",
		FileName:        "policy.pdf",
		ContentType:     "application/pdf",
		SizeBytes:       1,
		Content:         strings.NewReader("x"),
	})
	if !hasCode(err, service.CodeUnauthorized) {
		t.Fatalf("UploadDocument() error = %v, want unauthorized", err)
	}
}

func TestUploadDocumentRequiresPermission(t *testing.T) {
	documents := newTestService(t)
	_, err := documents.UploadDocument(context.Background(), service.RequestContext{UserID: "usr_123"}, service.UploadDocumentInput{
		KnowledgeBaseID: "kb_123",
		FileName:        "policy.pdf",
		ContentType:     "application/pdf",
		SizeBytes:       1,
		Content:         strings.NewReader("x"),
	})
	if !hasCode(err, service.CodeForbidden) {
		t.Fatalf("UploadDocument() error = %v, want forbidden", err)
	}
}

func TestNormalizeTagsRejectsControlCharacters(t *testing.T) {
	_, err := service.NormalizeTags([]string{"ok", "bad\n"})
	if err == nil {
		t.Fatal("NormalizeTags() error = nil, want error")
	}
}

func newTestService(t *testing.T) *service.Service {
	t.Helper()
	repo := repository.NewMemoryRepository()
	store := storage.NewMemoryStore()
	counter := 0
	return service.New(repo, store, service.WithIDGenerator(func(prefix string) (string, error) {
		counter++
		return prefix + "_" + strconv.Itoa(counter), nil
	}))
}

func uploadTestDocument(t *testing.T, documents *service.Service) service.Document {
	t.Helper()
	doc, err := documents.UploadDocument(context.Background(), actorContext(), service.UploadDocumentInput{
		KnowledgeBaseID: "kb_123",
		FileName:        "policy.pdf",
		ContentType:     "application/pdf",
		SizeBytes:       int64(len("content")),
		Tags:            []string{"initial"},
		Content:         strings.NewReader("content"),
	})
	if err != nil {
		t.Fatalf("UploadDocument() error = %v", err)
	}
	return doc
}

func internalContext() service.RequestContext {
	return service.RequestContext{
		RequestID:     "req_test",
		CallerService: "knowledge",
		ServiceToken:  "test-token",
	}
}

func actorContext() service.RequestContext {
	return service.RequestContext{
		RequestID:   "req_test",
		UserID:      "usr_123",
		Permissions: []string{"document:upload", "document:read", "document:update", "document:delete"},
	}
}

func hasCode(err error, code service.Code) bool {
	var appErr *service.AppError
	return errors.As(err, &appErr) && appErr.Code == code
}
