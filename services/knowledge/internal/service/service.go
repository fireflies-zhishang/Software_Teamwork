package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultPage     = 1
	defaultPageSize = 20
	maxPageSize     = 200
	defaultDocType  = "GENERAL"
	maxTags         = 32
	maxTagLength    = 64

	defaultIngestionRunningLease = 30 * time.Minute

	DefaultRetrievalTopK    = 10
	DefaultScoreThreshold   = 0.35
	DefaultVectorCollection = "knowledge_chunks"
)

var (
	defaultChunkStrategy     = json.RawMessage(`{"type":"SEMANTIC_TEXT","size":1600,"overlap":200}`)
	defaultRetrievalStrategy = json.RawMessage(`{"mode":"VECTOR","topK":10,"scoreThreshold":0.35}`)
)

type Clock func() time.Time

type IDGenerator func(prefix string) string

type Service struct {
	repo             Repository
	files            FileClient
	queue            IngestionQueue
	source           SourceReader
	parser           Parser
	chunker          Chunker
	embedder         Embedder
	vectorIndex      VectorIndex
	reranker         Reranker
	vectorCollection string
	retrievalTopK    int
	scoreThreshold   float64
	now              Clock
	newID            IDGenerator
	runningLease     time.Duration
}

type Option func(*Service)

func New(repo Repository) *Service {
	return &Service{
		repo:             repo,
		vectorCollection: DefaultVectorCollection,
		retrievalTopK:    DefaultRetrievalTopK,
		scoreThreshold:   DefaultScoreThreshold,
		now:              func() time.Time { return time.Now().UTC() },
		newID:            newID,
		runningLease:     defaultIngestionRunningLease,
	}
}

func NewKnowledgeService(repo Repository, opts ...Option) *Service {
	s := New(repo)
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func NewWithOptions(repo Repository, now Clock, idGenerator IDGenerator) *Service {
	return NewWithDependencies(repo, nil, nil, now, idGenerator)
}

func NewWithDependencies(repo Repository, files FileClient, queue IngestionQueue, now Clock, idGenerator IDGenerator, opts ...Option) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	if idGenerator == nil {
		idGenerator = newID
	}
	s := &Service{
		repo:             repo,
		files:            files,
		queue:            queue,
		vectorCollection: DefaultVectorCollection,
		retrievalTopK:    DefaultRetrievalTopK,
		scoreThreshold:   DefaultScoreThreshold,
		now:              now,
		newID:            idGenerator,
		runningLease:     defaultIngestionRunningLease,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
}

func WithProcessingPipeline(source SourceReader, parser Parser, chunker Chunker) Option {
	return func(s *Service) {
		s.source = source
		s.parser = parser
		s.chunker = chunker
	}
}

func WithVectorIndex(embedder Embedder, vectorIndex VectorIndex, collection ...string) Option {
	return func(s *Service) {
		s.embedder = embedder
		s.vectorIndex = vectorIndex
		if len(collection) > 0 && strings.TrimSpace(collection[0]) != "" {
			s.vectorCollection = strings.TrimSpace(collection[0])
		}
	}
}

func WithReranker(reranker Reranker) Option {
	return func(s *Service) {
		s.reranker = reranker
	}
}

func WithIngestionRunningLease(duration time.Duration) Option {
	return func(s *Service) {
		if duration > 0 {
			s.runningLease = duration
		}
	}
}

func (s *Service) CreateKnowledgeBase(ctx context.Context, reqCtx RequestContext, input CreateKnowledgeBaseInput) (KnowledgeBase, error) {
	scope, err := mutationScope(reqCtx)
	if err != nil {
		return KnowledgeBase{}, err
	}

	fields := map[string]string{}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		fields["name"] = "is required"
	}
	if len(name) > 120 {
		fields["name"] = "must be at most 120 characters"
	}
	description := ""
	if input.Description != nil {
		description = strings.TrimSpace(*input.Description)
	}
	if len(description) > 2000 {
		fields["description"] = "must be at most 2000 characters"
	}
	docType := defaultDocType
	if input.DocType != nil && strings.TrimSpace(*input.DocType) != "" {
		docType = strings.TrimSpace(*input.DocType)
	}
	if len(docType) > 64 {
		fields["docType"] = "must be at most 64 characters"
	}
	chunkStrategy := defaultChunkStrategy
	if len(input.ChunkStrategy) > 0 {
		chunkStrategy = cloneRaw(input.ChunkStrategy)
		if !json.Valid(chunkStrategy) {
			fields["chunkStrategy"] = "must be a valid JSON object"
		}
	}
	retrievalStrategy := defaultRetrievalStrategy
	if len(input.RetrievalStrategy) > 0 {
		retrievalStrategy = cloneRaw(input.RetrievalStrategy)
		if !json.Valid(retrievalStrategy) {
			fields["retrievalStrategy"] = "must be a valid JSON object"
		}
	}
	if len(fields) > 0 {
		return KnowledgeBase{}, ValidationError("request validation failed", fields)
	}

	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = s.newID("kb")
	}
	now := s.now()
	kb, err := s.repo.CreateKnowledgeBase(ctx, CreateKnowledgeBaseRecord{
		ID:                id,
		Name:              name,
		Description:       description,
		DocType:           docType,
		ChunkStrategy:     chunkStrategy,
		RetrievalStrategy: retrievalStrategy,
		CreatedBy:         scope.UserID,
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil {
		return KnowledgeBase{}, repositoryError(err)
	}
	return kb, nil
}

func (s *Service) ListKnowledgeBases(ctx context.Context, reqCtx RequestContext, input ListKnowledgeBasesInput) (KnowledgeBaseList, error) {
	scope, err := readScope(reqCtx)
	if err != nil {
		return KnowledgeBaseList{}, err
	}
	page, err := normalizePage(input.Page)
	if err != nil {
		return KnowledgeBaseList{}, err
	}
	list, err := s.repo.ListKnowledgeBases(ctx, scope, page)
	if err != nil {
		return KnowledgeBaseList{}, repositoryError(err)
	}
	return list, nil
}

func (s *Service) GetKnowledgeBase(ctx context.Context, reqCtx RequestContext, id string) (KnowledgeBase, error) {
	scope, err := readScope(reqCtx)
	if err != nil {
		return KnowledgeBase{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return KnowledgeBase{}, ValidationError("request validation failed", map[string]string{"knowledgeBaseId": "is required"})
	}
	kb, err := s.repo.GetKnowledgeBase(ctx, id, scope)
	if err != nil {
		return KnowledgeBase{}, repositoryError(err)
	}
	return kb, nil
}

func (s *Service) UpdateKnowledgeBase(ctx context.Context, reqCtx RequestContext, input UpdateKnowledgeBaseInput) (KnowledgeBase, error) {
	scope, err := mutationScope(reqCtx)
	if err != nil {
		return KnowledgeBase{}, err
	}
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return KnowledgeBase{}, ValidationError("request validation failed", map[string]string{"knowledgeBaseId": "is required"})
	}

	fields := map[string]string{}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			fields["name"] = "cannot be blank"
		}
		if len(name) > 120 {
			fields["name"] = "must be at most 120 characters"
		}
		input.Name = &name
	}
	if input.Description != nil {
		description := strings.TrimSpace(*input.Description)
		if len(description) > 2000 {
			fields["description"] = "must be at most 2000 characters"
		}
		input.Description = &description
	}
	if input.DocType != nil {
		docType := strings.TrimSpace(*input.DocType)
		if docType == "" {
			fields["docType"] = "cannot be blank"
		}
		if len(docType) > 64 {
			fields["docType"] = "must be at most 64 characters"
		}
		input.DocType = &docType
	}
	if input.ChunkStrategy != nil && !json.Valid(*input.ChunkStrategy) {
		fields["chunkStrategy"] = "must be a valid JSON object"
	}
	if input.RetrievalStrategy != nil && !json.Valid(*input.RetrievalStrategy) {
		fields["retrievalStrategy"] = "must be a valid JSON object"
	}
	if input.Name == nil && input.Description == nil && input.DocType == nil && input.ChunkStrategy == nil && input.RetrievalStrategy == nil {
		fields["body"] = "must include at least one supported field"
	}
	if len(fields) > 0 {
		return KnowledgeBase{}, ValidationError("request validation failed", fields)
	}

	kb, err := s.repo.UpdateKnowledgeBase(ctx, UpdateKnowledgeBaseRecord{
		ID:                id,
		Name:              input.Name,
		Description:       input.Description,
		DocType:           input.DocType,
		ChunkStrategy:     input.ChunkStrategy,
		RetrievalStrategy: input.RetrievalStrategy,
		UpdatedAt:         s.now(),
	}, scope)
	if err != nil {
		return KnowledgeBase{}, repositoryError(err)
	}
	return kb, nil
}

func (s *Service) DeleteKnowledgeBase(ctx context.Context, reqCtx RequestContext, id string) error {
	scope, err := mutationScope(reqCtx)
	if err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return ValidationError("request validation failed", map[string]string{"knowledgeBaseId": "is required"})
	}
	if err := s.repo.SoftDeleteKnowledgeBase(ctx, id, s.now(), scope); err != nil {
		return repositoryError(err)
	}
	return nil
}

func (s *Service) UploadDocument(ctx context.Context, reqCtx RequestContext, input UploadDocumentInput) (KnowledgeDocument, error) {
	scope, err := mutationScope(reqCtx)
	if err != nil {
		return KnowledgeDocument{}, err
	}
	if s.files == nil {
		return KnowledgeDocument{}, DependencyError("file service client is not configured", nil)
	}
	if s.queue == nil {
		return KnowledgeDocument{}, DependencyError("ingestion queue is not configured", nil)
	}

	knowledgeBaseID := strings.TrimSpace(input.KnowledgeBaseID)
	fields := map[string]string{}
	if knowledgeBaseID == "" {
		fields["knowledgeBaseId"] = "is required"
	}
	file := input.File
	filename := normalizeDocumentName(file.Filename)
	if filename == "" {
		fields["file"] = "filename is required"
	}
	if file.Content == nil {
		fields["file"] = "is required"
	} else if file.SizeBytes == 0 {
		fields["file"] = "must not be empty"
	}
	tags, tagFields := normalizeTags(input.Tags)
	for key, value := range tagFields {
		fields[key] = value
	}
	if len(fields) > 0 {
		return KnowledgeDocument{}, ValidationError("request validation failed", fields)
	}
	if _, err := s.repo.GetKnowledgeBase(ctx, knowledgeBaseID, scope); err != nil {
		return KnowledgeDocument{}, repositoryError(err)
	}

	file.Filename = filename
	createdFile, err := s.files.CreateFile(ctx, reqCtx, file)
	if err != nil {
		return KnowledgeDocument{}, normalizeFileClientError(err)
	}
	fileID := strings.TrimSpace(createdFile.ID)
	if fileID == "" {
		return KnowledgeDocument{}, DependencyError("file service returned invalid response", nil)
	}

	now := s.now()
	documentID := s.newID("doc")
	jobID := s.newID("job")
	contentType := strings.TrimSpace(createdFile.ContentType)
	if contentType == "" {
		contentType = strings.TrimSpace(file.ContentType)
	}
	sizeBytes := createdFile.SizeBytes
	if sizeBytes == 0 {
		sizeBytes = file.SizeBytes
	}
	parserSnapshot, err := s.ResolveParserConfig(ctx, contentType)
	if err != nil {
		_ = s.files.DeleteFile(ctx, reqCtx, fileID)
		return KnowledgeDocument{}, err
	}
	parserSnapshotJSON, err := marshalParserConfigSnapshot(parserSnapshot)
	if err != nil {
		_ = s.files.DeleteFile(ctx, reqCtx, fileID)
		return KnowledgeDocument{}, DependencyError("effective parser config is invalid", err)
	}
	doc, job, err := s.repo.CreateDocumentWithJob(ctx, CreateDocumentWithJobRecord{
		DocumentID:           documentID,
		KnowledgeBaseID:      knowledgeBaseID,
		FileRef:              fileID,
		Name:                 displayName(createdFile.Filename, filename),
		ContentType:          contentType,
		SizeBytes:            sizeBytes,
		Status:               DocumentStatusUploaded,
		Tags:                 tags,
		CurrentJobID:         jobID,
		CreatedBy:            scope.UserID,
		JobID:                jobID,
		JobType:              JobTypeDocumentIngestion,
		JobStatus:            JobStatusQueued,
		JobStage:             "uploaded",
		JobMessage:           "document uploaded and queued for ingestion",
		MaxAttempts:          DefaultIngestionMaxAttempts,
		ParserConfigID:       parserSnapshot.ParserConfigID,
		ParserConfigSnapshot: parserSnapshotJSON,
		CreatedAt:            now,
		UpdatedAt:            now,
	}, scope)
	if err != nil {
		_ = s.files.DeleteFile(ctx, reqCtx, fileID)
		return KnowledgeDocument{}, repositoryError(err)
	}

	if err := s.queue.EnqueueDocumentIngestion(ctx, DocumentIngestionTask{
		RequestID:       strings.TrimSpace(reqCtx.RequestID),
		JobID:           job.ID,
		DocumentID:      doc.ID,
		KnowledgeBaseID: doc.KnowledgeBaseID,
		UserID:          scope.UserID,
	}); err != nil {
		_ = s.repo.MarkDocumentJobFailed(ctx, doc.ID, job.ID, nil, string(CodeDependency), "ingestion queue handoff failed", s.now())
		return KnowledgeDocument{}, DependencyError("ingestion queue handoff failed", err)
	}

	return doc, nil
}

func (s *Service) ListDocuments(ctx context.Context, reqCtx RequestContext, input ListDocumentsInput) (DocumentList, error) {
	scope, err := readScope(reqCtx)
	if err != nil {
		return DocumentList{}, err
	}
	knowledgeBaseID := strings.TrimSpace(input.KnowledgeBaseID)
	if knowledgeBaseID == "" {
		return DocumentList{}, ValidationError("request validation failed", map[string]string{"knowledgeBaseId": "is required"})
	}
	if input.Status != nil && !validDocumentStatus(*input.Status) {
		return DocumentList{}, ValidationError("request validation failed", map[string]string{"status": "is not supported"})
	}
	page, err := normalizePage(input.Page)
	if err != nil {
		return DocumentList{}, err
	}
	list, err := s.repo.ListDocumentsByKnowledgeBase(ctx, knowledgeBaseID, input.Status, scope, page)
	if err != nil {
		return DocumentList{}, repositoryError(err)
	}
	return list, nil
}

func (s *Service) GetDocument(ctx context.Context, reqCtx RequestContext, id string) (KnowledgeDocument, error) {
	scope, err := readScope(reqCtx)
	if err != nil {
		return KnowledgeDocument{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return KnowledgeDocument{}, ValidationError("request validation failed", map[string]string{"documentId": "is required"})
	}
	doc, err := s.repo.GetDocument(ctx, id, scope)
	if err != nil {
		return KnowledgeDocument{}, repositoryError(err)
	}
	return doc, nil
}

func (s *Service) GetDocumentContent(ctx context.Context, reqCtx RequestContext, id string) (SourceDocument, error) {
	scope, err := readScope(reqCtx)
	if err != nil {
		return SourceDocument{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return SourceDocument{}, ValidationError("request validation failed", map[string]string{"documentId": "is required"})
	}
	doc, err := s.repo.GetDocument(ctx, id, scope)
	if err != nil {
		return SourceDocument{}, repositoryError(err)
	}
	if s.source == nil {
		return SourceDocument{}, DependencyError("document content reader is not configured", nil)
	}
	if doc.FileRef == nil || strings.TrimSpace(*doc.FileRef) == "" {
		return SourceDocument{}, DependencyError("document source is not configured", nil)
	}
	source, err := s.source.ReadSource(ctx, reqCtx, strings.TrimSpace(*doc.FileRef))
	if err != nil {
		return SourceDocument{}, DependencyError("document content read failed", err)
	}
	if source.Body == nil {
		return SourceDocument{}, DependencyError("document content read failed", nil)
	}
	source.ContentType = strings.TrimSpace(source.ContentType)
	if source.ContentType == "" && doc.ContentType != nil {
		source.ContentType = strings.TrimSpace(*doc.ContentType)
	}
	if source.ContentType == "" {
		source.ContentType = "application/octet-stream"
	}
	if source.SizeBytes <= 0 && doc.SizeBytes != nil && *doc.SizeBytes > 0 {
		source.SizeBytes = *doc.SizeBytes
	}
	return source, nil
}

func normalizeDocumentName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	name = strings.ReplaceAll(name, "\\", "/")
	base := filepath.Base(name)
	base = strings.TrimSpace(base)
	if base == "." || base == "/" {
		return ""
	}
	if len(base) > 255 {
		base = base[:255]
	}
	return base
}

func displayName(primary string, fallback string) string {
	if normalized := normalizeDocumentName(primary); normalized != "" {
		return normalized
	}
	return fallback
}

func normalizeTags(input []string) ([]string, map[string]string) {
	fields := map[string]string{}
	seen := map[string]struct{}{}
	tags := make([]string, 0, len(input))
	for _, raw := range input {
		tag := strings.TrimSpace(raw)
		if tag == "" {
			continue
		}
		if len(tag) > maxTagLength {
			fields["tags"] = "each tag must be at most 64 characters"
			continue
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	if len(tags) > maxTags {
		fields["tags"] = "must contain at most 32 tags"
		tags = tags[:maxTags]
	}
	if len(fields) == 0 {
		return tags, nil
	}
	return tags, fields
}

func normalizeFileClientError(err error) error {
	if err == nil {
		return nil
	}
	if appErr, ok := Classify(err); ok {
		switch appErr.Code {
		case CodeValidation:
			return ValidationError("request validation failed", map[string]string{"file": "is invalid"})
		case CodeUnauthorized, CodeForbidden:
			return DependencyError("file service rejected knowledge request", err)
		case CodeNotFound:
			return DependencyError("file service resource not found", err)
		case CodeDependency, CodeInternal, CodeConflict, CodeRateLimited:
			return err
		default:
			return DependencyError("file service failed", err)
		}
	}
	return DependencyError("file service failed", err)
}

func readScope(reqCtx RequestContext) (AccessScope, error) {
	if strings.TrimSpace(reqCtx.UserID) == "" {
		return AccessScope{}, UnauthorizedError()
	}
	return AccessScope{
		UserID:     strings.TrimSpace(reqCtx.UserID),
		CanReadAll: hasAdminRole(reqCtx.Roles) || hasPermission(reqCtx.Permissions, PermissionKnowledgeRead) || hasPermission(reqCtx.Permissions, PermissionKnowledgeWrite),
		CanWrite:   hasAdminRole(reqCtx.Roles) || hasPermission(reqCtx.Permissions, PermissionKnowledgeWrite),
	}, nil
}

func mutationScope(reqCtx RequestContext) (AccessScope, error) {
	scope, err := readScope(reqCtx)
	if err != nil {
		return AccessScope{}, err
	}
	if !scope.CanWrite {
		return AccessScope{}, ForbiddenError("knowledge write permission is required")
	}
	return scope, nil
}

func normalizePage(page PageInput) (PageInput, error) {
	if page.Page == 0 {
		page.Page = defaultPage
	}
	if page.PageSize == 0 {
		page.PageSize = defaultPageSize
	}
	fields := map[string]string{}
	if page.Page < 1 {
		fields["page"] = "must be greater than or equal to 1"
	}
	if page.PageSize < 1 || page.PageSize > maxPageSize {
		fields["pageSize"] = "must be between 1 and 200"
	}
	if len(fields) > 0 {
		return PageInput{}, ValidationError("request validation failed", fields)
	}
	return page, nil
}

func validDocumentStatus(status DocumentStatus) bool {
	switch status {
	case DocumentStatusUploaded, DocumentStatusParsing, DocumentStatusChunking, DocumentStatusEmbedding, DocumentStatusReady, DocumentStatusFailed:
		return true
	default:
		return false
	}
}

func hasAdminRole(roles []string) bool {
	for _, role := range roles {
		switch strings.ToLower(strings.TrimSpace(role)) {
		case "admin", "super_admin", "superadmin":
			return true
		}
	}
	return false
}

func hasPermission(permissions []string, target string) bool {
	for _, permission := range permissions {
		if strings.TrimSpace(permission) == target {
			return true
		}
	}
	return false
}

func repositoryError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) {
		return NotFoundError("resource not found")
	}
	if errors.Is(err, ErrConflict) {
		return ConflictError("resource already exists", err)
	}
	if _, ok := Classify(err); ok {
		return err
	}
	return DependencyError("repository operation failed", err)
}

func cloneRaw(value json.RawMessage) json.RawMessage {
	if value == nil {
		return nil
	}
	return append(json.RawMessage(nil), value...)
}

func newID(prefix string) string {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return prefix + "_" + hex.EncodeToString([]byte(time.Now().UTC().Format("20060102150405.000000000")))
	}
	return prefix + "_" + hex.EncodeToString(buf[:])
}
