package service

import (
	"encoding/json"
	"io"
	"time"
)

type ReportStatus string

const (
	ReportStatusDraft             ReportStatus = "draft"
	ReportStatusOutlineGenerating ReportStatus = "outline_generating"
	ReportStatusOutlineGenerated  ReportStatus = "outline_generated"
	ReportStatusContentGenerating ReportStatus = "content_generating"
	ReportStatusGenerated         ReportStatus = "generated"
	ReportStatusExporting         ReportStatus = "exporting"
	ReportStatusExported          ReportStatus = "exported"
	ReportStatusFailed            ReportStatus = "failed"
	ReportStatusDeleted           ReportStatus = "deleted"
)

type JobType string

const (
	JobTypeOutlineGeneration   JobType = "outline_generation"
	JobTypeOutlineRegeneration JobType = "outline_regeneration"
	JobTypeContentGeneration   JobType = "content_generation"
	JobTypeContentRegeneration JobType = "content_regeneration"
	JobTypeSectionRegeneration JobType = "section_regeneration"
	JobTypeReportFileCreation  JobType = "report_file_creation"
)

// JobStatus is also used as ReportSection.GenerationStatus, mirroring the
// gateway OpenAPI ReportJobStatus enum.
type JobStatus string

const (
	JobStatusPending          JobStatus = "pending"
	JobStatusRunning          JobStatus = "running"
	JobStatusSucceeded        JobStatus = "succeeded"
	JobStatusPartialSucceeded JobStatus = "partial_succeeded"
	JobStatusFailed           JobStatus = "failed"
	JobStatusCanceled         JobStatus = "canceled"
)

// SectionType mirrors the gateway OpenAPI ReportSection.sectionType enum.
type SectionType string

const (
	SectionTypeText  SectionType = "text"
	SectionTypeTable SectionType = "table"
	SectionTypeImage SectionType = "image"
	SectionTypeMixed SectionType = "mixed"
)

// ContentSource mirrors the gateway OpenAPI ReportSection.contentSource enum.
type ContentSource string

const (
	ContentSourceAI     ContentSource = "ai"
	ContentSourceManual ContentSource = "manual"
	ContentSourceMixed  ContentSource = "mixed"
)

// OutlineSource mirrors the gateway OpenAPI CreateReportOutlineRequest.source enum.
type OutlineSource string

const (
	OutlineSourceManual OutlineSource = "manual"
	OutlineSourceAI     OutlineSource = "ai"
)

type ReportType struct {
	Code              string
	Name              string
	Description       string
	Enabled           bool
	DefaultTemplateID string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type PageMeta struct {
	Page     int
	PageSize int
	Total    int
}

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

type FileContent struct {
	Filename    string
	ContentType string
	SizeBytes   int64
	Content     io.ReadCloser
}

type ReportTemplate struct {
	ID           string
	TemplateName string
	ReportType   string
	Version      int
	FileRef      string
	Filename     string
	FileSize     int64
	Description  string
	Enabled      bool
	CreatedBy    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *time.Time
}

type ReportTemplateStructure struct {
	OutlineSchema json.RawMessage
	StyleConfig   json.RawMessage
}

type ReportTemplateListFilter struct {
	Page       int
	PageSize   int
	ReportType string
	Enabled    *bool
}

type ReportTemplateListResult struct {
	Items []ReportTemplate
	Page  PageMeta
}

type CreateReportTemplateInput struct {
	TemplateName string
	ReportType   string
	Description  string
	File         UploadedFile
}

type UpdateReportTemplateInput struct {
	ID           string
	TemplateName *string
	Description  *string
	Enabled      *bool
}

type UpdateReportTemplateStructureInput struct {
	ID        string
	Structure ReportTemplateStructure
}

type ReportMaterial struct {
	ID           string
	MaterialName string
	MaterialType string
	Category     string
	FileRef      string
	Filename     string
	FileSize     int64
	Description  string
	Tags         []string
	Enabled      bool
	CreatedBy    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *time.Time
}

type ReportMaterialListFilter struct {
	Page     int
	PageSize int
	Category string
	Enabled  *bool
}

type ReportMaterialListResult struct {
	Items []ReportMaterial
	Page  PageMeta
}

type CreateReportMaterialInput struct {
	MaterialName string
	MaterialType string
	Category     string
	Description  string
	Tags         []string
	File         UploadedFile
}

type Report struct {
	ID                 string
	Name               string
	ReportType         string
	TemplateID         string
	Topic              string
	Specialty          string
	BusinessObject     string
	Year               int
	Status             ReportStatus
	CreatorID          string
	CreatorName        string
	Source             string
	LatestJobID        string
	LatestReportFileID string
	GeneratedAt        *time.Time
	ExportedAt         *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          *time.Time
}

type ReportJob struct {
	ID             string
	RequestID      string
	Source         string
	JobType        JobType
	TargetType     string
	TargetID       string
	AsynqTaskID    string
	QueueName      string
	ReportID       string
	TemplateID     string
	RequestPayload map[string]any
	Status         JobStatus
	Progress       map[string]any
	ErrorCode      string
	ErrorMessage   string
	RetryCount     int
	MaxAttempts    int
	StartedAt      *time.Time
	FinishedAt     *time.Time
	CreatedAt      time.Time
}

type ReportGenerationExecutionPayload struct {
	RequestID string
	JobType   JobType
	JobID     string
	AttemptID string
	UserID    string
}

type ReportGenerationExecutionResult struct {
	Status JobStatus
}

type ReportFileStatus string

const (
	ReportFileStatusPending   ReportFileStatus = "pending"
	ReportFileStatusRunning   ReportFileStatus = "running"
	ReportFileStatusSucceeded ReportFileStatus = "succeeded"
	ReportFileStatusFailed    ReportFileStatus = "failed"
)

const ReportFileFormatDOCX = "docx"

type ReportFile struct {
	ID        string
	ReportID  string
	JobID     string
	Filename  string
	Format    string
	FileRef   string
	FileSize  int64
	Status    ReportFileStatus
	CreatedBy string
	CreatedAt time.Time
}

type ReportFileListFilter struct {
	Page      int
	PageSize  int
	ReportID  string
	CreatorID string
}

type ReportFileListResult struct {
	Items []ReportFile
	Page  PageMeta
}

type CreateReportFileInput struct {
	ReportID     string
	Format       string
	TemplateID   string
	StyleOptions json.RawMessage
}

type ReportJobAttempt struct {
	ID            string
	JobID         string
	AttemptNumber int
	AsynqTaskID   string
	TriggerSource string
	Reason        string
	Status        JobStatus
	ErrorCode     string
	ErrorMessage  string
	StartedAt     *time.Time
	FinishedAt    *time.Time
	CreatedAt     time.Time
}

type ReportEvent struct {
	ID        string
	ReportID  string
	JobID     string
	EventType string
	Message   string
	CreatedAt time.Time
}

// ReportOutlineNode is one node of the multi-level outline tree stored as
// ReportOutline.OutlineJSON. ClientSectionID lets callers correlate a node
// across requests before a server-assigned ID exists.
type ReportOutlineNode struct {
	ID              string              `json:"id"`
	ClientSectionID string              `json:"clientSectionId,omitempty"`
	Title           string              `json:"title"`
	Level           int                 `json:"level"`
	Numbering       string              `json:"numbering,omitempty"`
	Children        []ReportOutlineNode `json:"children,omitempty"`
}

type ReportOutline struct {
	ID           string
	ReportID     string
	Sections     []ReportOutlineNode
	Version      int
	Source       OutlineSource
	SourceJobID  string
	IsCurrent    bool
	ManualEdited bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type ReportSection struct {
	ID               string
	ReportID         string
	OutlineID        string
	ParentID         string
	OutlineNodeID    string
	SectionPath      string
	Title            string
	Level            int
	SortOrder        int
	Numbering        string
	SectionType      SectionType
	Content          string
	Tables           []map[string]any
	GenerationStatus JobStatus
	ContentSource    ContentSource
	ManualEdited     bool
	Version          int
	LastJobID        string
	GeneratedAt      *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type ReportSectionVersion struct {
	ID           string
	ReportID     string
	SectionID    string
	Version      int
	Source       ContentSource
	Content      string
	Tables       []map[string]any
	JobID        string
	Requirements string
	CreatedBy    string
	CreatedAt    time.Time
}

type ReportSettingsModelConfig struct {
	Provider  string
	ProfileID string
	// ProfileIDSet is used by PATCH inputs to distinguish omitted profileId from an explicit clear.
	ProfileIDSet   bool
	Model          string
	TimeoutSeconds int
}

type ReportSettingsFileDefaults struct {
	DefaultFormat         string
	DefaultNumberingMode  string
	DefaultStyleProfileID string
	// DefaultStyleProfileIDSet distinguishes omitted style profile from an explicit clear.
	DefaultStyleProfileIDSet bool
	Extra                    map[string]any
}

type ReportSettings struct {
	LLM              ReportSettingsModelConfig
	DefaultTemplates map[string]string
	File             ReportSettingsFileDefaults
	UpdatedAt        time.Time
}

type UpdateReportSettingsInput struct {
	LLM              *ReportSettingsModelConfig
	DefaultTemplates *map[string]string
	File             *ReportSettingsFileDefaults
}

type ModelProfileReference struct {
	ID             string
	Purpose        string
	Provider       string
	Model          string
	Enabled        bool
	TimeoutSeconds int
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatTokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type ChatCompletionRequest struct {
	Model       string
	ProfileID   string
	Messages    []ChatMessage
	Temperature *float64
	TopP        *float64
	MaxTokens   int
}

type ChatCompletionResponse struct {
	Content      string
	FinishReason string
	Usage        ChatTokenUsage
}

type ReportKnowledgeRetrievalInput struct {
	Query            string
	KnowledgeBaseIDs []string
	TopK             int
	ScoreThreshold   *float64
	Rerank           bool
	RerankTopN       *int
}

type ReportKnowledgeSnippet struct {
	Score           float64
	KnowledgeBaseID string
	DocumentID      string
	ChunkID         string
	DocumentName    string
	SectionPath     string
	ContentPreview  string
}

type ReportStatisticsOverview struct {
	ReportCount     int
	TemplateCount   int
	MaterialCount   int
	JobStatusCounts map[string]int
	RecentDays      int
}

type ReportDailyStatistic struct {
	Date           string
	ReportType     string
	CreatedCount   int
	GeneratedCount int
	FailedCount    int
	ExportedCount  int
}

type OperationLog struct {
	ID               string
	OperatorID       string
	OperatorName     string
	OperationType    string
	TargetType       string
	TargetID         string
	RequestID        string
	RequestSource    string
	ToolName         string
	ParameterSummary map[string]any
	OperationResult  string
	ErrorMessage     string
	Metadata         map[string]any
	CreatedAt        time.Time
}

type OperationLogListFilter struct {
	Page          int
	PageSize      int
	TargetType    string
	TargetID      string
	OperationType string
	RequestID     string
	RequestSource string
	ToolName      string
}

type OperationLogListResult struct {
	Items []OperationLog
	Page  PageMeta
}

const (
	OperationCreateReport              = "create_report"
	OperationUpdateReport              = "update_report"
	OperationDeleteReport              = "delete_report"
	OperationOutlineGeneration         = "outline_generation"
	OperationOutlineRegeneration       = "outline_regeneration"
	OperationSaveOutline               = "save_outline"
	OperationContentGeneration         = "content_generation"
	OperationContentRegeneration       = "content_regeneration"
	OperationSectionRegeneration       = "section_regeneration"
	OperationUpdateSection             = "update_section"
	OperationCreateSectionVersion      = OperationSectionRegeneration
	OperationUploadTemplate            = "upload_template"
	OperationUpdateTemplate            = "update_template"
	OperationDeleteTemplate            = "delete_template"
	OperationUploadMaterial            = "upload_material"
	OperationDeleteMaterial            = "delete_material"
	OperationCreateReportJob           = "create_report_job"
	OperationRetryReportJob            = "retry_report_job"
	OperationUpdateReportSettings      = "update_report_settings"
	OperationReportJobRunning          = "report_job_running"
	OperationReportJobSucceeded        = "report_job_succeeded"
	OperationReportJobPartialSucceeded = "report_job_partial_succeeded"
	OperationReportJobFailed           = "report_job_failed"
	OperationReportFileCreation        = "report_file_creation"
	OperationResultSucceeded           = "succeeded"
	OperationResultPartialSucceeded    = "partial_succeeded"
	OperationResultFailed              = "failed"
	DefaultReportSettingsProvider      = "ai-gateway"
	DefaultReportSettingsFormat        = "docx"
	DefaultReportNumberingMode         = "global"
	ReportNumberingModeByChapter       = "by_chapter"
	DefaultReportStatisticsDays        = 30
	MaximumReportStatisticsDays        = 366
)
