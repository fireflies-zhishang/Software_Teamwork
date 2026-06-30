package service

import (
	"context"
	"encoding/json"
	"io"
	"time"
)

const (
	PermissionKnowledgeRead     = "knowledge:read"
	PermissionKnowledgeWrite    = "knowledge:write"
	PermissionKnowledgeAdmin    = "knowledge:admin"
	PermissionSystemAdmin       = "system:admin"
	PermissionAdminParserConfig = "admin:parser-config:write"
)

type ParserBackend string

const (
	ParserBackendBuiltin          ParserBackend = "builtin"
	ParserBackendTika             ParserBackend = "tika"
	ParserBackendUnstructured     ParserBackend = "unstructured"
	ParserBackendLocalOCR         ParserBackend = "local_ocr"
	ParserBackendRemoteCompatible ParserBackend = "remote_compatible"
)

type ParserConfig struct {
	ID                    string
	Name                  string
	Backend               ParserBackend
	Enabled               bool
	IsDefault             bool
	Concurrency           int
	SupportedContentTypes []string
	EndpointURL           *string
	DefaultParameters     json.RawMessage
	CreatedAt             time.Time
	UpdatedAt             time.Time
	DeletedAt             *time.Time
}

type ParserConfigList struct {
	Items []ParserConfig
}

type ParserConfigSnapshot struct {
	ParserConfigID        string          `json:"parserConfigId"`
	Backend               ParserBackend   `json:"backend"`
	Concurrency           int             `json:"concurrency"`
	SupportedContentTypes []string        `json:"supportedContentTypes,omitempty"`
	EndpointURL           *string         `json:"endpointUrl,omitempty"`
	DefaultParameters     json.RawMessage `json:"defaultParameters,omitempty"`
}

type ParserConfigAudit struct {
	ID             string
	ParserConfigID string
	ActorUserID    string
	Action         string
	Summary        json.RawMessage
	CreatedAt      time.Time
}

type CreateParserConfigInput struct {
	Name                  string
	Backend               ParserBackend
	Enabled               *bool
	IsDefault             *bool
	Concurrency           int
	SupportedContentTypes []string
	EndpointURL           *string
	DefaultParameters     json.RawMessage
}

type UpdateParserConfigInput struct {
	ID                    string
	Name                  *string
	Backend               *ParserBackend
	Enabled               *bool
	IsDefault             *bool
	Concurrency           *int
	SupportedContentTypes *[]string
	EndpointURL           **string
	DefaultParameters     *json.RawMessage
}

type DocumentStatus string

const (
	DocumentStatusUploaded  DocumentStatus = "uploaded"
	DocumentStatusParsing   DocumentStatus = "parsing"
	DocumentStatusChunking  DocumentStatus = "chunking"
	DocumentStatusEmbedding DocumentStatus = "embedding"
	DocumentStatusReady     DocumentStatus = "ready"
	DocumentStatusFailed    DocumentStatus = "failed"
)

const (
	JobTypeIngest            = "ingest"
	JobTypeDocumentIngestion = JobTypeIngest
	LegacyJobTypeIngestion   = "document_ingestion"

	DefaultIngestionMaxAttempts int32 = 3

	JobStatusQueued    = "queued"
	JobStatusRunning   = "running"
	JobStatusSucceeded = "succeeded"
	JobStatusFailed    = "failed"
	JobStatusCancelled = "cancelled"
)

type RequestContext struct {
	RequestID      string
	UserID         string
	CallerService  string
	ServiceToken   string
	Roles          []string
	Permissions    []string
	ForwardedFor   string
	ForwardedProto string
}

type AccessScope struct {
	UserID     string
	CanReadAll bool
	CanWrite   bool
}

type Page struct {
	Page     int
	PageSize int
	Total    int64
}

type PageInput struct {
	Page     int
	PageSize int
}

type KnowledgeBase struct {
	ID                string
	Name              string
	Description       string
	DocType           string
	ChunkStrategy     json.RawMessage
	RetrievalStrategy json.RawMessage
	DocumentCount     int64
	ChunkCount        int64
	CreatedBy         string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         *time.Time
}

type KnowledgeDocument struct {
	ID              string
	KnowledgeBaseID string
	FileRef         *string
	Name            string
	ContentType     *string
	SizeBytes       *int64
	Status          DocumentStatus
	ErrorCode       *string
	ErrorMessage    *string
	ChunkCount      int64
	Tags            []string
	ParserBackend   *string
	CurrentJobID    *string
	CreatedBy       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
}

type ProcessingJob struct {
	ID                   string
	KnowledgeBaseID      string
	DocumentID           *string
	JobType              string
	Status               string
	CurrentStage         *string
	ProgressPercent      int32
	Message              *string
	ErrorCode            *string
	ErrorMessage         *string
	Attempts             int32
	MaxAttempts          int32
	ParserConfigID       *string
	ParserConfigSnapshot json.RawMessage
	StartedAt            *time.Time
	FinishedAt           *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type KnowledgeBaseList struct {
	Items []KnowledgeBase
	Page  Page
}

type DocumentList struct {
	Items []KnowledgeDocument
	Page  Page
}

type CreateKnowledgeBaseInput struct {
	ID                string
	Name              string
	Description       *string
	DocType           *string
	ChunkStrategy     json.RawMessage
	RetrievalStrategy json.RawMessage
}

type UpdateKnowledgeBaseInput struct {
	ID                string
	Name              *string
	Description       *string
	DocType           *string
	ChunkStrategy     *json.RawMessage
	RetrievalStrategy *json.RawMessage
}

type ListKnowledgeBasesInput struct {
	Page PageInput
}

type UploadedFile struct {
	Filename       string
	ContentType    string
	SizeBytes      int64
	ChecksumSHA256 string
	Content        io.Reader
}

type FileObject struct {
	ID             string
	Filename       string
	ContentType    string
	SizeBytes      int64
	ChecksumSHA256 string
	CreatedAt      time.Time
}

type FileClient interface {
	CreateFile(ctx context.Context, reqCtx RequestContext, file UploadedFile) (FileObject, error)
	DeleteFile(ctx context.Context, reqCtx RequestContext, fileID string) error
}

type SourceDocument struct {
	Body        io.ReadCloser
	ContentType string
	SizeBytes   int64
}

type SourceReader interface {
	ReadSource(ctx context.Context, reqCtx RequestContext, fileID string) (SourceDocument, error)
}

type ParseInput struct {
	Name        string
	ContentType string
	Body        io.Reader
	SizeBytes   int64
	RequestID   string
	UserID      string
}

type ParsedDocument struct {
	Content string
	Title   string
	Backend string
	Pages   []ParsedPage
}

type ParsedPage struct {
	PageNumber      int
	Content         string
	ParseStrategy   string
	TextLayerStatus string
	OCRConfidence   *float64
	DPI             *int
	Warnings        []string
}

type Parser interface {
	Parse(ctx context.Context, input ParseInput) (ParsedDocument, error)
}

type ChunkInput struct {
	Content  string
	Strategy json.RawMessage
}

type ChunkSpec struct {
	SectionPath *string
	Content     string
	TokenCount  int
	ChunkType   *string
	Metadata    map[string]any
}

type Chunker interface {
	Chunk(ctx context.Context, input ChunkInput) ([]ChunkSpec, error)
}

type EmbeddingRequest struct {
	Texts     []string
	RequestID string
	UserID    string
}

type EmbeddingResult struct {
	Vectors   [][]float32
	Provider  string
	Model     string
	Dimension int
}

type Embedder interface {
	Embed(ctx context.Context, request EmbeddingRequest) (EmbeddingResult, error)
}

type VectorPoint struct {
	ID      string
	Vector  []float32
	Payload map[string]any
}

const (
	VectorPayloadDocumentID       = "document_id"
	VectorPayloadIngestionAttempt = "ingestion_attempt"
)

type VectorIndex interface {
	Upsert(ctx context.Context, points []VectorPoint) error
	DeleteByDocumentIngestionAttempt(ctx context.Context, documentID string, ingestionAttempt string) error
	DeleteStaleDocumentPoints(ctx context.Context, documentID string, activeIngestionAttempt string) error
	Search(ctx context.Context, request VectorSearchRequest) ([]VectorSearchHit, error)
}

type DocumentIngestionTask struct {
	RequestID       string `json:"requestId"`
	JobID           string `json:"jobId"`
	DocumentID      string `json:"documentId"`
	KnowledgeBaseID string `json:"knowledgeBaseId"`
	UserID          string `json:"userId"`
}

type IngestionQueue interface {
	EnqueueDocumentIngestion(ctx context.Context, task DocumentIngestionTask) error
}

type UploadDocumentInput struct {
	KnowledgeBaseID string
	File            UploadedFile
	Tags            []string
}

type ListDocumentsInput struct {
	KnowledgeBaseID string
	Status          *DocumentStatus
	Page            PageInput
}

type ListChunksInput struct {
	DocumentID string
	Page       PageInput
}

type ChunkList struct {
	Items []DocumentChunk
	Page  Page
}

type Repository interface {
	CreateKnowledgeBase(ctx context.Context, input CreateKnowledgeBaseRecord) (KnowledgeBase, error)
	ListKnowledgeBases(ctx context.Context, scope AccessScope, page PageInput) (KnowledgeBaseList, error)
	GetKnowledgeBase(ctx context.Context, id string, scope AccessScope) (KnowledgeBase, error)
	UpdateKnowledgeBase(ctx context.Context, input UpdateKnowledgeBaseRecord, scope AccessScope) (KnowledgeBase, error)
	SoftDeleteKnowledgeBase(ctx context.Context, id string, deletedAt time.Time, scope AccessScope) error
	CreateDocumentWithJob(ctx context.Context, input CreateDocumentWithJobRecord, scope AccessScope) (KnowledgeDocument, ProcessingJob, error)
	MarkDocumentJobFailed(ctx context.Context, documentID string, jobID string, expectedAttempts *int32, code string, message string, failedAt time.Time) error
	ListDocumentsByKnowledgeBase(ctx context.Context, knowledgeBaseID string, status *DocumentStatus, scope AccessScope, page PageInput) (DocumentList, error)
	GetDocument(ctx context.Context, id string, scope AccessScope) (KnowledgeDocument, error)
	FindChunksByIDs(ctx context.Context, ids []string) ([]DocumentChunk, error)
	ListParserConfigs(ctx context.Context, enabled *bool) ([]ParserConfig, error)
	GetParserConfig(ctx context.Context, id string) (ParserConfig, error)
	CreateParserConfig(ctx context.Context, config ParserConfig, audit ParserConfigAudit) (ParserConfig, error)
	UpdateParserConfig(ctx context.Context, config ParserConfig, audit ParserConfigAudit) (ParserConfig, error)
	SoftDeleteParserConfig(ctx context.Context, id string, deletedAt time.Time, audit ParserConfigAudit) error
	GetEffectiveParserConfig(ctx context.Context, contentType string) (ParserConfig, error)
	GetProcessingJob(ctx context.Context, id string) (ProcessingJob, error)
	ClaimProcessingJob(ctx context.Context, id string, update JobStateUpdate) (ProcessingJob, error)
	UpdateJobState(ctx context.Context, id string, update JobStateUpdate) (ProcessingJob, error)
	UpdateDocumentProcessingState(ctx context.Context, id string, update DocumentStateUpdate) (KnowledgeDocument, error)
	CompleteIngestion(ctx context.Context, input CompleteIngestionRecord) (ProcessingJob, error)
	ListChunks(ctx context.Context, documentID string, scope AccessScope, page PageInput) (ChunkList, error)
}

type CreateKnowledgeBaseRecord struct {
	ID                string
	Name              string
	Description       string
	DocType           string
	ChunkStrategy     json.RawMessage
	RetrievalStrategy json.RawMessage
	CreatedBy         string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type UpdateKnowledgeBaseRecord struct {
	ID                string
	Name              *string
	Description       *string
	DocType           *string
	ChunkStrategy     *json.RawMessage
	RetrievalStrategy *json.RawMessage
	UpdatedAt         time.Time
}

type CreateDocumentWithJobRecord struct {
	DocumentID           string
	KnowledgeBaseID      string
	FileRef              string
	Name                 string
	ContentType          string
	SizeBytes            int64
	Status               DocumentStatus
	Tags                 []string
	CurrentJobID         string
	CreatedBy            string
	JobID                string
	JobType              string
	JobStatus            string
	JobStage             string
	JobMessage           string
	MaxAttempts          int32
	ParserConfigID       string
	ParserConfigSnapshot json.RawMessage
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type DocumentStateUpdate struct {
	Status       DocumentStatus
	ErrorCode    *string
	ErrorMessage *string
	UpdatedAt    time.Time
}

type JobStateUpdate struct {
	Status             string
	CurrentStage       *string
	ProgressPercent    int32
	Message            *string
	ErrorCode          *string
	ErrorMessage       *string
	Attempts           *int32
	StartedAt          *time.Time
	FinishedAt         *time.Time
	UpdatedAt          time.Time
	StaleRunningBefore *time.Time
	ExpectedAttempts   *int32
}

type DocumentChunk struct {
	ID                 string
	KnowledgeBaseID    string
	DocumentID         string
	ChunkIndex         int32
	SectionPath        *string
	Content            string
	TokenCount         *int32
	ChunkType          *string
	QdrantPointID      *string
	EmbeddingProvider  *string
	EmbeddingModel     *string
	EmbeddingDimension *int32
	Metadata           map[string]any
	CreatedAt          time.Time
}

type CompleteIngestionRecord struct {
	DocumentID       string
	JobID            string
	ExpectedAttempts *int32
	ParserBackend    *string
	Chunks           []DocumentChunk
	UpdatedAt        time.Time
	FinishedAt       time.Time
}
