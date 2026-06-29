package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
)

const (
	maxTags      = 32
	maxTagLength = 64

	permissionUpload = "document:upload"
	permissionRead   = "document:read"
	permissionUpdate = "document:update"
	permissionDelete = "document:delete"

	checksumSHA256HexLength = 64
	storageBackendMemory    = "memory"
)

type DocumentRepository interface {
	Create(ctx context.Context, doc Document) (Document, error)
	FindByID(ctx context.Context, id string) (Document, error)
	ReplaceTags(ctx context.Context, id string, tags []string) (Document, error)
	MarkDeleted(ctx context.Context, id string, deletedAt time.Time) (Document, error)
	CreateFile(ctx context.Context, file FileObject) (FileObject, error)
	FindFileByID(ctx context.Context, id string) (FileObject, error)
	MarkFileDeleteRequested(ctx context.Context, id string, deletedAt time.Time) (FileObject, error)
	MarkFilePurged(ctx context.Context, id string, purgedAt time.Time) (FileObject, error)
	MarkFilePurgeFailed(ctx context.Context, id string, code string, message string, failedAt time.Time) (FileObject, error)
}

type ObjectStore interface {
	Put(ctx context.Context, key string, body io.Reader, contentType string, sizeBytes int64) error
	Get(ctx context.Context, key string) (StoredObject, error)
	Delete(ctx context.Context, key string) error
}

type Service struct {
	repo           DocumentRepository
	store          ObjectStore
	storageBackend string
	now            func() time.Time
	newID          func(prefix string) (string, error)
}

type Option func(*Service)

func New(repo DocumentRepository, store ObjectStore, opts ...Option) *Service {
	s := &Service{
		repo:           repo,
		store:          store,
		storageBackend: storageBackendMemory,
		now:            func() time.Time { return time.Now().UTC() },
		newID:          newPublicID,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func WithStorageBackend(backend string) Option {
	return func(s *Service) {
		trimmed := strings.TrimSpace(backend)
		if trimmed != "" {
			s.storageBackend = trimmed
		}
	}
}
func WithIDGenerator(newID func(prefix string) (string, error)) Option {
	return func(s *Service) {
		if newID != nil {
			s.newID = newID
		}
	}
}

func (s *Service) CreateFile(ctx context.Context, reqCtx RequestContext, input CreateFileInput) (FileObject, error) {
	if err := validateInternalCaller(reqCtx); err != nil {
		return FileObject{}, err
	}

	fields := map[string]string{}
	name, err := normalizeFileName(input.FileName)
	if err != nil {
		fields["file"] = err.Error()
	}
	if input.Content == nil {
		fields["file"] = "is required"
	} else if input.SizeBytes == 0 {
		fields["file"] = "must not be empty"
	}
	checksum, err := normalizeSHA256(input.ChecksumSHA256)
	if err != nil {
		fields["checksumSha256"] = err.Error()
	}
	if len(fields) > 0 {
		return FileObject{}, ValidationError("request validation failed", fields)
	}

	data, err := io.ReadAll(input.Content)
	if err != nil {
		return FileObject{}, DependencyError("file content read failed", err)
	}
	if len(data) == 0 {
		return FileObject{}, ValidationError("request validation failed", map[string]string{"file": "must not be empty"})
	}
	if input.SizeBytes > 0 && int64(len(data)) != input.SizeBytes {
		return FileObject{}, ValidationError("request validation failed", map[string]string{"file": "size does not match multipart metadata"})
	}

	computed := sha256.Sum256(data)
	computedChecksum := hex.EncodeToString(computed[:])
	if checksum != "" && checksum != computedChecksum {
		return FileObject{}, ValidationError("request validation failed", map[string]string{"checksumSha256": "does not match file content"})
	}
	if checksum == "" {
		checksum = computedChecksum
	}

	fileID, err := s.newID("file")
	if err != nil {
		return FileObject{}, DependencyError("file id generation failed", err)
	}

	contentType := strings.TrimSpace(input.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	objectKey := "files/" + fileID
	if err := s.store.Put(ctx, objectKey, bytes.NewReader(data), contentType, int64(len(data))); err != nil {
		return FileObject{}, DependencyError("object storage write failed", err)
	}

	now := s.now()
	file := FileObject{
		ID:               fileID,
		Filename:         name,
		ContentType:      contentType,
		SizeBytes:        int64(len(data)),
		ChecksumSHA256:   checksum,
		StorageBackend:   s.storageBackend,
		StorageObjectKey: objectKey,
		Status:           FileStatusAvailable,
		CreatedByService: callerService(reqCtx),
		RequestID:        strings.TrimSpace(reqCtx.RequestID),
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	created, err := s.repo.CreateFile(ctx, file)
	if err != nil {
		_ = s.store.Delete(ctx, objectKey)
		if errors.Is(err, ErrConflict) {
			return FileObject{}, ConflictError("file already exists", err)
		}
		return FileObject{}, DependencyError("file metadata write failed", err)
	}
	return created, nil
}

func (s *Service) GetFile(ctx context.Context, reqCtx RequestContext, fileID string) (FileObject, error) {
	if err := validateInternalCaller(reqCtx); err != nil {
		return FileObject{}, err
	}
	id := strings.TrimSpace(fileID)
	if id == "" {
		return FileObject{}, ValidationError("request validation failed", map[string]string{"fileId": "is required"})
	}
	file, err := s.repo.FindFileByID(ctx, id)
	if err != nil {
		return FileObject{}, mapFileRepositoryError(err, "file not found")
	}
	return file, nil
}

func (s *Service) DeleteFile(ctx context.Context, reqCtx RequestContext, fileID string) error {
	if err := validateInternalCaller(reqCtx); err != nil {
		return err
	}
	id := strings.TrimSpace(fileID)
	if id == "" {
		return ValidationError("request validation failed", map[string]string{"fileId": "is required"})
	}

	file, err := s.repo.MarkFileDeleteRequested(ctx, id, s.now())
	if err != nil {
		return mapFileRepositoryError(err, "file not found")
	}
	if strings.TrimSpace(file.StorageObjectKey) == "" {
		_, _ = s.repo.MarkFilePurgeFailed(ctx, id, string(CodeDependency), "object storage reference is missing", s.now())
		return DependencyError("object storage reference is missing", errors.New("missing storage object key"))
	}
	if err := s.store.Delete(ctx, file.StorageObjectKey); err != nil {
		if errors.Is(err, ErrNotFound) {
			_, _ = s.repo.MarkFilePurged(ctx, id, s.now())
			return nil
		}
		_, _ = s.repo.MarkFilePurgeFailed(ctx, id, string(CodeDependency), "object storage delete failed", s.now())
		return DependencyError("object storage delete failed", err)
	}
	if _, err := s.repo.MarkFilePurged(ctx, id, s.now()); err != nil {
		return DependencyError("file cleanup metadata update failed", err)
	}
	return nil
}

func (s *Service) GetFileContent(ctx context.Context, reqCtx RequestContext, fileID string) (FileContent, error) {
	if err := validateInternalCaller(reqCtx); err != nil {
		return FileContent{}, err
	}
	id := strings.TrimSpace(fileID)
	if id == "" {
		return FileContent{}, ValidationError("request validation failed", map[string]string{"fileId": "is required"})
	}

	file, err := s.repo.FindFileByID(ctx, id)
	if err != nil {
		return FileContent{}, mapFileRepositoryError(err, "file not found")
	}
	object, err := s.store.Get(ctx, file.StorageObjectKey)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return FileContent{}, NotFoundError("file content not found")
		}
		return FileContent{}, DependencyError("object storage read failed", err)
	}
	contentType := object.ContentType
	if contentType == "" {
		contentType = file.ContentType
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	sizeBytes := object.SizeBytes
	if sizeBytes < 0 {
		sizeBytes = file.SizeBytes
	}
	return FileContent{File: file, Body: object.Body, ContentType: contentType, SizeBytes: sizeBytes}, nil
}

func (s *Service) UploadDocument(ctx context.Context, reqCtx RequestContext, input UploadDocumentInput) (Document, error) {
	if err := validateActor(reqCtx); err != nil {
		return Document{}, err
	}

	if err := requirePermission(reqCtx, permissionUpload); err != nil {
		return Document{}, err
	}
	fields := map[string]string{}
	knowledgeBaseID := strings.TrimSpace(input.KnowledgeBaseID)
	if knowledgeBaseID == "" {
		fields["knowledgeBaseId"] = "is required"
	}

	name, err := normalizeFileName(input.FileName)
	if err != nil {
		fields["file"] = err.Error()
	}
	if input.SizeBytes <= 0 {
		fields["file"] = "must not be empty"
	}
	if input.Content == nil {
		fields["file"] = "is required"
	}

	tags, err := NormalizeTags(input.Tags)
	if err != nil {
		fields["tags"] = err.Error()
	}
	if len(fields) > 0 {
		return Document{}, ValidationError("request validation failed", fields)
	}

	docID, err := s.newID("doc")
	if err != nil {
		return Document{}, DependencyError("document id generation failed", err)
	}

	contentType := strings.TrimSpace(input.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	objectKey := "documents/" + docID
	if err := s.store.Put(ctx, objectKey, input.Content, contentType, input.SizeBytes); err != nil {
		return Document{}, DependencyError("object storage write failed", err)
	}

	doc := Document{
		ID:              docID,
		KnowledgeBaseID: knowledgeBaseID,
		Name:            name,
		Status:          DocumentStatusUploaded,
		Tags:            tags,
		CreatedAt:       s.now(),
		ContentType:     contentType,
		SizeBytes:       input.SizeBytes,
		ObjectKey:       objectKey,
		OwnerUserID:     strings.TrimSpace(reqCtx.UserID),
	}

	created, err := s.repo.Create(ctx, doc)
	if err != nil {
		_ = s.store.Delete(ctx, objectKey)
		if errors.Is(err, ErrConflict) {
			return Document{}, ConflictError("document already exists", err)
		}
		return Document{}, DependencyError("document metadata write failed", err)
	}

	return created, nil
}

func (s *Service) GetDocument(ctx context.Context, reqCtx RequestContext, documentID string) (Document, error) {
	if err := validateActor(reqCtx); err != nil {
		return Document{}, err
	}
	if err := requirePermission(reqCtx, permissionRead); err != nil {
		return Document{}, err
	}
	id := strings.TrimSpace(documentID)
	if id == "" {
		return Document{}, ValidationError("request validation failed", map[string]string{"documentId": "is required"})
	}

	doc, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return Document{}, mapRepositoryError(err, "document not found")
	}
	return doc, nil
}

func (s *Service) UpdateDocument(ctx context.Context, reqCtx RequestContext, input UpdateDocumentInput) (Document, error) {
	if err := validateActor(reqCtx); err != nil {
		return Document{}, err
	}
	if err := requirePermission(reqCtx, permissionUpdate); err != nil {
		return Document{}, err
	}
	id := strings.TrimSpace(input.DocumentID)
	if id == "" {
		return Document{}, ValidationError("request validation failed", map[string]string{"documentId": "is required"})
	}
	tags, err := NormalizeTags(input.Tags)
	if err != nil {
		return Document{}, ValidationError("request validation failed", map[string]string{"tags": err.Error()})
	}

	doc, err := s.repo.ReplaceTags(ctx, id, tags)
	if err != nil {
		return Document{}, mapRepositoryError(err, "document not found")
	}
	return doc, nil
}

func (s *Service) DeleteDocument(ctx context.Context, reqCtx RequestContext, documentID string) error {
	if err := validateActor(reqCtx); err != nil {
		return err
	}
	if err := requirePermission(reqCtx, permissionDelete); err != nil {
		return err
	}
	id := strings.TrimSpace(documentID)
	if id == "" {
		return ValidationError("request validation failed", map[string]string{"documentId": "is required"})
	}

	doc, err := s.repo.MarkDeleted(ctx, id, s.now())
	if err != nil {
		return mapRepositoryError(err, "document not found")
	}
	if err := s.store.Delete(ctx, doc.ObjectKey); err != nil && !errors.Is(err, ErrNotFound) {
		return DependencyError("object storage delete failed", err)
	}
	return nil
}

func (s *Service) GetDocumentContent(ctx context.Context, reqCtx RequestContext, documentID string) (DocumentContent, error) {
	if err := validateActor(reqCtx); err != nil {
		return DocumentContent{}, err
	}
	if err := requirePermission(reqCtx, permissionRead); err != nil {
		return DocumentContent{}, err
	}

	id := strings.TrimSpace(documentID)
	if id == "" {
		return DocumentContent{}, ValidationError("request validation failed", map[string]string{"documentId": "is required"})
	}

	doc, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return DocumentContent{}, mapRepositoryError(err, "document not found")
	}

	object, err := s.store.Get(ctx, doc.ObjectKey)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return DocumentContent{}, NotFoundError("document content not found")
		}
		return DocumentContent{}, DependencyError("object storage read failed", err)
	}

	contentType := object.ContentType
	if contentType == "" {
		contentType = doc.ContentType
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	sizeBytes := object.SizeBytes
	if sizeBytes < 0 {
		sizeBytes = doc.SizeBytes
	}

	return DocumentContent{
		Document:    doc,
		Body:        object.Body,
		ContentType: contentType,
		SizeBytes:   sizeBytes,
	}, nil
}

func NormalizeTags(tags []string) ([]string, error) {
	if len(tags) > maxTags {
		return nil, fmt.Errorf("must contain at most %d tags", maxTags)
	}

	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(tags))
	for _, raw := range tags {
		if strings.ContainsAny(raw, "\x00\r\n") {
			return nil, fmt.Errorf("must not contain control characters")
		}
		tag := strings.TrimSpace(raw)
		if tag == "" {
			continue
		}
		if len(tag) > maxTagLength {
			return nil, fmt.Errorf("each tag must be at most %d characters", maxTagLength)
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		normalized = append(normalized, tag)
	}
	return normalized, nil
}

func validateActor(reqCtx RequestContext) error {
	if strings.TrimSpace(reqCtx.UserID) == "" {
		return UnauthorizedError()
	}
	return nil
}

func validateInternalCaller(reqCtx RequestContext) error {
	if strings.TrimSpace(reqCtx.CallerService) == "" && strings.TrimSpace(reqCtx.UserID) == "" {
		return UnauthorizedError()
	}
	return nil
}

func requirePermission(reqCtx RequestContext, permission string) error {
	for _, candidate := range reqCtx.Permissions {
		if strings.TrimSpace(candidate) == permission {
			return nil
		}
	}
	return ForbiddenError("permission is required")
}

func normalizeFileName(name string) (string, error) {
	trimmed := strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	trimmed = path.Base(trimmed)
	if trimmed == "." || trimmed == "/" || trimmed == "" {
		return "", fmt.Errorf("filename is required")
	}
	if strings.ContainsAny(trimmed, "\x00\r\n") {
		return "", fmt.Errorf("filename is invalid")
	}
	return trimmed, nil
}

func normalizeSHA256(value string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return "", nil
	}
	if len(trimmed) != checksumSHA256HexLength {
		return "", fmt.Errorf("must be a 64-character hexadecimal SHA-256 value")
	}
	if _, err := hex.DecodeString(trimmed); err != nil {
		return "", fmt.Errorf("must be a 64-character hexadecimal SHA-256 value")
	}
	return trimmed, nil
}

func callerService(reqCtx RequestContext) string {
	caller := strings.TrimSpace(reqCtx.CallerService)
	if caller != "" {
		return caller
	}
	return "gateway"
}

func mapRepositoryError(err error, notFoundMessage string) error {
	if errors.Is(err, ErrNotFound) {
		return NotFoundError(notFoundMessage)
	}
	if errors.Is(err, ErrConflict) {
		return ConflictError("document state conflict", err)
	}
	return DependencyError("document metadata access failed", err)
}

func mapFileRepositoryError(err error, notFoundMessage string) error {
	if errors.Is(err, ErrNotFound) {
		return NotFoundError(notFoundMessage)
	}
	if errors.Is(err, ErrConflict) {
		return ConflictError("file state conflict", err)
	}
	return DependencyError("file metadata access failed", err)
}

func newPublicID(prefix string) (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(bytes), nil
}
