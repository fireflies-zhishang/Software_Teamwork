package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type ReportGenerationRepository interface {
	WithinGenerationTx(ctx context.Context, fn func(ReportGenerationRepository) error) error
	GetReportByID(ctx context.Context, id string) (Report, error)
	FindReportJobByID(ctx context.Context, id string) (ReportJob, error)
	GetReportTemplateStructure(ctx context.Context, id string) (ReportTemplateStructure, error)
	GetReportSettings(ctx context.Context) (ReportSettings, error)
	CreateReportOutline(ctx context.Context, value ReportOutline) (ReportOutline, error)
	ListReportOutlines(ctx context.Context, reportID string) ([]ReportOutline, error)
	CreateReportSection(ctx context.Context, value ReportSection) (ReportSection, error)
	ListReportSections(ctx context.Context, reportID string) ([]ReportSection, error)
	UpdateReportSection(ctx context.Context, value ReportSection) (ReportSection, error)
	CreateReportSectionVersion(ctx context.Context, value ReportSectionVersion) (ReportSectionVersion, error)
	ListReportSectionVersions(ctx context.Context, sectionID string) ([]ReportSectionVersion, error)
	CreateReportEvent(ctx context.Context, value ReportEvent) (ReportEvent, error)
	UpdateReportJobProgress(ctx context.Context, jobID string, completed, total int) error
}

type ReportGenerationChatClient interface {
	CreateChatCompletion(ctx context.Context, reqCtx RequestContext, input ChatCompletionRequest) (ChatCompletionResponse, error)
}

type ReportGenerationKnowledgeRetriever interface {
	RetrieveReportContext(ctx context.Context, reqCtx RequestContext, input ReportKnowledgeRetrievalInput) ([]ReportKnowledgeSnippet, error)
}

type ReportGenerationService struct {
	repo      ReportGenerationRepository
	chat      ReportGenerationChatClient
	retriever ReportGenerationKnowledgeRetriever
	clock     func() time.Time
}

func NewReportGenerationService(repo ReportGenerationRepository, chat ReportGenerationChatClient, retrievers ...ReportGenerationKnowledgeRetriever) *ReportGenerationService {
	var retriever ReportGenerationKnowledgeRetriever
	if len(retrievers) > 0 {
		retriever = retrievers[0]
	}
	return &ReportGenerationService{
		repo:      repo,
		chat:      chat,
		retriever: retriever,
		clock:     func() time.Time { return time.Now().UTC() },
	}
}

func (s *ReportGenerationService) ExecuteReportGeneration(ctx context.Context, payload ReportGenerationExecutionPayload) (ReportGenerationExecutionResult, error) {
	if s.chat == nil {
		return ReportGenerationExecutionResult{}, NewError(CodeDependency, "ai gateway chat client is not configured", nil)
	}
	job, err := s.repo.FindReportJobByID(ctx, payload.JobID)
	if err != nil {
		return ReportGenerationExecutionResult{}, mapRepositoryReadError(err, "report job not found")
	}
	jobType := payload.JobType
	if jobType == "" {
		jobType = job.JobType
	}
	report, err := s.repo.GetReportByID(ctx, job.ReportID)
	if err != nil {
		return ReportGenerationExecutionResult{}, mapRepositoryReadError(err, "report not found")
	}
	if report.Status == ReportStatusDeleted || report.DeletedAt != nil {
		return ReportGenerationExecutionResult{}, NewError(CodeConflict, "report has been deleted", nil)
	}
	reqCtx := RequestContext{RequestID: payload.RequestID, UserID: payload.UserID, CallerService: "worker"}

	switch jobType {
	case JobTypeOutlineGeneration, JobTypeOutlineRegeneration:
		return s.executeOutlineGeneration(ctx, reqCtx, payload, job, report)
	case JobTypeContentGeneration, JobTypeContentRegeneration, JobTypeSectionRegeneration:
		return s.executeContentGeneration(ctx, reqCtx, payload, job, report)
	default:
		return ReportGenerationExecutionResult{}, ValidationError(map[string]string{"jobType": "unsupported report generation job type"})
	}
}

func (s *ReportGenerationService) executeOutlineGeneration(ctx context.Context, reqCtx RequestContext, payload ReportGenerationExecutionPayload, job ReportJob, report Report) (ReportGenerationExecutionResult, error) {
	if err := validateSupportedAIReportType(report.ReportType, "outline"); err != nil {
		return ReportGenerationExecutionResult{}, err
	}
	settings, err := s.safeSettings(ctx)
	if err != nil {
		return ReportGenerationExecutionResult{}, err
	}
	structure := ReportTemplateStructure{}
	if strings.TrimSpace(report.TemplateID) != "" {
		structure, err = s.repo.GetReportTemplateStructure(ctx, report.TemplateID)
		if err != nil {
			return ReportGenerationExecutionResult{}, mapRepositoryReadError(err, "report template structure not found")
		}
	}
	_ = s.recordEvent(ctx, report.ID, payload.JobID, "outline.started", "outline generation started")
	generationContext, err := s.loadGenerationContext(ctx, reqCtx, report, ReportSection{}, job)
	if err != nil {
		_ = s.recordEvent(ctx, report.ID, payload.JobID, "outline.failed", "outline generation failed")
		return ReportGenerationExecutionResult{}, err
	}
	resp, err := s.chat.CreateChatCompletion(ctx, reqCtx, ChatCompletionRequest{
		Model:     settings.LLM.Model,
		ProfileID: settings.LLM.ProfileID,
		Messages: []ChatMessage{
			{Role: "system", Content: "Return strict JSON for a power-industry report outline. Do not include markdown."},
			{Role: "user", Content: buildOutlinePrompt(report, structure, generationContext)},
		},
	})
	if err != nil {
		_ = s.recordEvent(ctx, report.ID, payload.JobID, "outline.failed", "outline generation failed")
		return ReportGenerationExecutionResult{}, dependencyError("generate report outline", err)
	}
	nodes, err := parseGeneratedOutline(resp.Content)
	if err != nil {
		_ = s.recordEvent(ctx, report.ID, payload.JobID, "outline.failed", "outline generation failed")
		return ReportGenerationExecutionResult{}, err
	}
	existing, err := s.repo.ListReportOutlines(ctx, report.ID)
	if err != nil {
		return ReportGenerationExecutionResult{}, dependencyError("list report outlines", err)
	}
	nextVersion := nextOutlineVersion(existing)
	now := s.clock()
	outline := ReportOutline{
		ID:           newID(),
		ReportID:     report.ID,
		Sections:     nodes,
		Version:      nextVersion,
		Source:       OutlineSourceAI,
		SourceJobID:  payload.JobID,
		IsCurrent:    true,
		ManualEdited: false,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	var created ReportOutline
	if err := s.repo.WithinGenerationTx(ctx, func(txRepo ReportGenerationRepository) error {
		var err error
		created, err = txRepo.CreateReportOutline(ctx, outline)
		if err != nil {
			return dependencyError("create report outline", err)
		}
		if err := createSectionSkeletons(ctx, txRepo, report.ID, created, payload.JobID, now); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return ReportGenerationExecutionResult{}, err
	}
	_ = s.repo.UpdateReportJobProgress(ctx, payload.JobID, CountOutlineNodes(created.Sections), CountOutlineNodes(created.Sections))
	_ = s.recordEvent(ctx, report.ID, payload.JobID, "outline.succeeded", "outline generation succeeded")
	return ReportGenerationExecutionResult{Status: JobStatusSucceeded}, nil
}

func (s *ReportGenerationService) executeContentGeneration(ctx context.Context, reqCtx RequestContext, payload ReportGenerationExecutionPayload, job ReportJob, report Report) (ReportGenerationExecutionResult, error) {
	if err := validateSupportedAIReportType(report.ReportType, "content"); err != nil {
		return ReportGenerationExecutionResult{}, err
	}
	settings, err := s.safeSettings(ctx)
	if err != nil {
		return ReportGenerationExecutionResult{}, err
	}
	sections, err := s.repo.ListReportSections(ctx, report.ID)
	if err != nil {
		return ReportGenerationExecutionResult{}, dependencyError("list report sections", err)
	}
	if job.JobType != JobTypeSectionRegeneration {
		sections, err = s.currentOutlineSections(ctx, report.ID, sections)
		if err != nil {
			return ReportGenerationExecutionResult{}, err
		}
	}
	sections = targetGenerationSections(sections, job)
	if len(sections) == 0 {
		return ReportGenerationExecutionResult{}, ValidationError(map[string]string{"sections": "no report sections available for content generation"})
	}
	sortSections(sections)
	_ = s.recordEvent(ctx, report.ID, payload.JobID, "content.started", "content generation started")
	completed := 0
	total := len(sections)
	preserveManual := preserveManualEdits(job)
	for _, section := range sections {
		if preserveManual && section.ManualEdited {
			completed++
			_ = s.repo.UpdateReportJobProgress(ctx, payload.JobID, completed, total)
			_ = s.recordEvent(ctx, report.ID, payload.JobID, "section.skipped", "section generation skipped because manual edits are preserved")
			continue
		}
		section.GenerationStatus = JobStatusRunning
		section.LastJobID = payload.JobID
		section.UpdatedAt = s.clock()
		_, _ = s.repo.UpdateReportSection(ctx, section)
		generationContext, err := s.loadGenerationContext(ctx, reqCtx, report, section, job)
		if err != nil {
			section.GenerationStatus = JobStatusFailed
			section.UpdatedAt = s.clock()
			_, _ = s.repo.UpdateReportSection(ctx, section)
			_ = s.recordEvent(ctx, report.ID, payload.JobID, "section.failed", "section generation failed")
			_ = s.repo.UpdateReportJobProgress(ctx, payload.JobID, completed, total)
			if completed > 0 {
				_ = s.recordEvent(ctx, report.ID, payload.JobID, "content.partial_succeeded", "content generation partially succeeded")
				return ReportGenerationExecutionResult{Status: JobStatusPartialSucceeded}, nil
			}
			return ReportGenerationExecutionResult{}, err
		}
		resp, err := s.chat.CreateChatCompletion(ctx, reqCtx, ChatCompletionRequest{
			Model:     settings.LLM.Model,
			ProfileID: settings.LLM.ProfileID,
			Messages: []ChatMessage{
				{Role: "system", Content: "Return strict JSON for one report section. Do not include markdown."},
				{Role: "user", Content: buildSectionPrompt(report, section, generationContext)},
			},
		})
		if err != nil {
			section.GenerationStatus = JobStatusFailed
			section.UpdatedAt = s.clock()
			_, _ = s.repo.UpdateReportSection(ctx, section)
			_ = s.recordEvent(ctx, report.ID, payload.JobID, "section.failed", "section generation failed")
			_ = s.repo.UpdateReportJobProgress(ctx, payload.JobID, completed, total)
			if completed > 0 {
				_ = s.recordEvent(ctx, report.ID, payload.JobID, "content.partial_succeeded", "content generation partially succeeded")
				return ReportGenerationExecutionResult{Status: JobStatusPartialSucceeded}, nil
			}
			return ReportGenerationExecutionResult{}, dependencyError("generate report section", err)
		}
		generated, err := parseGeneratedSection(resp.Content)
		if err != nil {
			section.GenerationStatus = JobStatusFailed
			section.UpdatedAt = s.clock()
			_, _ = s.repo.UpdateReportSection(ctx, section)
			_ = s.recordEvent(ctx, report.ID, payload.JobID, "section.failed", "section generation failed")
			_ = s.repo.UpdateReportJobProgress(ctx, payload.JobID, completed, total)
			if completed > 0 {
				_ = s.recordEvent(ctx, report.ID, payload.JobID, "content.partial_succeeded", "content generation partially succeeded")
				return ReportGenerationExecutionResult{Status: JobStatusPartialSucceeded}, nil
			}
			return ReportGenerationExecutionResult{}, err
		}
		nextVersion, err := s.nextSectionVersion(ctx, section)
		if err != nil {
			return ReportGenerationExecutionResult{}, err
		}
		section.Content = generated.Content
		section.Tables = generated.Tables
		section.GenerationStatus = JobStatusSucceeded
		section.ContentSource = ContentSourceAI
		section.ManualEdited = false
		section.Version = nextVersion
		section.LastJobID = payload.JobID
		now := s.clock()
		section.GeneratedAt = &now
		section.UpdatedAt = now
		updated, err := s.repo.UpdateReportSection(ctx, section)
		if err != nil {
			return ReportGenerationExecutionResult{}, dependencyError("update generated report section", err)
		}
		if _, err := s.repo.CreateReportSectionVersion(ctx, ReportSectionVersion{
			ID:        newID(),
			ReportID:  report.ID,
			SectionID: updated.ID,
			Version:   nextVersion,
			Source:    ContentSourceAI,
			Content:   updated.Content,
			Tables:    updated.Tables,
			JobID:     payload.JobID,
			CreatedBy: payload.UserID,
			CreatedAt: now,
		}); err != nil {
			return ReportGenerationExecutionResult{}, dependencyError("create report section version", err)
		}
		completed++
		_ = s.repo.UpdateReportJobProgress(ctx, payload.JobID, completed, total)
		_ = s.recordEvent(ctx, report.ID, payload.JobID, "section.succeeded", "section generation succeeded")
	}
	_ = s.recordEvent(ctx, report.ID, payload.JobID, "content.succeeded", "content generation succeeded")
	return ReportGenerationExecutionResult{Status: JobStatusSucceeded}, nil
}

func validateSupportedAIReportType(reportType, generationKind string) error {
	if reportType != "summer_peak_inspection" {
		return ValidationError(map[string]string{"reportType": fmt.Sprintf("unsupported report type for AI %s generation", generationKind)})
	}
	return nil
}

func preserveManualEdits(job ReportJob) bool {
	payload := jsonObject(job.RequestPayload)
	options, ok := payload["options"].(map[string]any)
	if !ok {
		return true
	}
	value, ok := options["preserveManualEdits"].(bool)
	if !ok {
		return true
	}
	return value
}

func (s *ReportGenerationService) safeSettings(ctx context.Context) (ReportSettings, error) {
	settings, err := s.repo.GetReportSettings(ctx)
	if err != nil {
		return ReportSettings{}, dependencyError("get report settings", err)
	}
	return normalizeReportSettings(settings), nil
}

type reportGenerationContext struct {
	Requirements string
	MaterialIDs  []string
	Snippets     []ReportKnowledgeSnippet
}

func (s *ReportGenerationService) loadGenerationContext(ctx context.Context, reqCtx RequestContext, report Report, section ReportSection, job ReportJob) (reportGenerationContext, error) {
	payload := jsonObject(job.RequestPayload)
	result := reportGenerationContext{
		Requirements: stringValue(payload["requirements"]),
		MaterialIDs:  stringSliceValue(payload["materialIds"]),
	}
	retrieval := mergedRetrievalOptions(payload)
	knowledgeBaseIDs := stringSliceValue(retrieval["knowledgeBaseIds"])
	if len(knowledgeBaseIDs) == 0 || s.retriever == nil {
		return result, nil
	}
	query := strings.TrimSpace(report.Topic)
	if strings.TrimSpace(section.Title) != "" {
		query = strings.TrimSpace(query + " " + section.Title)
	}
	if query == "" {
		query = strings.TrimSpace(report.ReportType)
	}
	snippets, err := s.retriever.RetrieveReportContext(ctx, reqCtx, ReportKnowledgeRetrievalInput{
		Query:            query,
		KnowledgeBaseIDs: knowledgeBaseIDs,
		TopK:             intValue(retrieval["topK"]),
		ScoreThreshold:   floatPtrValue(retrieval["scoreThreshold"]),
		Rerank:           boolValue(retrieval["rerank"]),
		RerankTopN:       intPtrValue(retrieval["rerankTopN"]),
	})
	if err != nil {
		return reportGenerationContext{}, dependencyError("retrieve report knowledge context", err)
	}
	result.Snippets = snippets
	return result, nil
}

func createSectionSkeletons(ctx context.Context, repo ReportGenerationRepository, reportID string, outline ReportOutline, jobID string, now time.Time) error {
	var createNodes func(nodes []ReportOutlineNode, parentID string) error
	sortOrder := 0
	createNodes = func(nodes []ReportOutlineNode, parentID string) error {
		for _, node := range nodes {
			id := newID()
			section := ReportSection{
				ID:               id,
				ReportID:         reportID,
				OutlineID:        outline.ID,
				ParentID:         parentID,
				OutlineNodeID:    node.ID,
				SectionPath:      id,
				Title:            node.Title,
				Level:            node.Level,
				SortOrder:        sortOrder,
				Numbering:        node.Numbering,
				SectionType:      SectionTypeText,
				GenerationStatus: JobStatusPending,
				ContentSource:    ContentSourceAI,
				ManualEdited:     false,
				Version:          1,
				LastJobID:        jobID,
				CreatedAt:        now,
				UpdatedAt:        now,
			}
			sortOrder++
			created, err := repo.CreateReportSection(ctx, section)
			if err != nil {
				return dependencyError("create report section skeleton", err)
			}
			if len(node.Children) > 0 {
				if err := createNodes(node.Children, created.ID); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return createNodes(outline.Sections, "")
}

func (s *ReportGenerationService) nextSectionVersion(ctx context.Context, section ReportSection) (int, error) {
	existing, err := s.repo.ListReportSectionVersions(ctx, section.ID)
	if err != nil {
		return 0, dependencyError("list report section versions", err)
	}
	next := section.Version + 1
	if next <= 1 {
		next = 1
	}
	for _, version := range existing {
		if version.Version >= next {
			next = version.Version + 1
		}
	}
	return next, nil
}

func (s *ReportGenerationService) recordEvent(ctx context.Context, reportID, jobID, eventType, message string) error {
	_, err := s.repo.CreateReportEvent(ctx, ReportEvent{
		ID:        newID(),
		ReportID:  reportID,
		JobID:     jobID,
		EventType: eventType,
		Message:   sanitizeStringValue(message),
		CreatedAt: s.clock(),
	})
	return err
}

func buildOutlinePrompt(report Report, structure ReportTemplateStructure, generationContext reportGenerationContext) string {
	return fmt.Sprintf("reportType=%s topic=%s requirements=%s materialRefs=%s retrievedContext=%s templateOutlineSchema=%s output={\"sections\":[{\"title\":\"...\",\"children\":[]}]}",
		report.ReportType,
		report.Topic,
		compactTextForPrompt(generationContext.Requirements, 1024),
		strings.Join(generationContext.MaterialIDs, ","),
		formatKnowledgeSnippets(generationContext.Snippets),
		compactJSONForPrompt(structure.OutlineSchema),
	)
}

func buildSectionPrompt(report Report, section ReportSection, generationContext reportGenerationContext) string {
	return fmt.Sprintf("reportType=%s topic=%s sectionNumber=%s sectionTitle=%s requirements=%s materialRefs=%s retrievedContext=%s output={\"content\":\"...\",\"tables\":[]}",
		report.ReportType,
		report.Topic,
		section.Numbering,
		section.Title,
		compactTextForPrompt(generationContext.Requirements, 1024),
		strings.Join(generationContext.MaterialIDs, ","),
		formatKnowledgeSnippets(generationContext.Snippets),
	)
}

func formatKnowledgeSnippets(snippets []ReportKnowledgeSnippet) string {
	if len(snippets) == 0 {
		return ""
	}
	parts := make([]string, 0, len(snippets))
	for _, snippet := range snippets {
		preview := compactTextForPrompt(snippet.ContentPreview, 512)
		if preview == "" {
			continue
		}
		parts = append(parts, preview)
	}
	return strings.Join(parts, "\n")
}

func compactTextForPrompt(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[:limit]
}

func compactJSONForPrompt(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "{}"
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	text := string(data)
	if len(text) > 2048 {
		return text[:2048]
	}
	return text
}

type generatedOutlinePayload struct {
	Sections []generatedOutlineNode `json:"sections"`
}

type generatedOutlineNode struct {
	Title    string                 `json:"title"`
	Children []generatedOutlineNode `json:"children"`
}

func parseGeneratedOutline(content string) ([]ReportOutlineNode, error) {
	var payload generatedOutlinePayload
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return nil, NewError(CodeDependency, "AI outline response was not valid JSON", nil)
	}
	nodes, err := generatedNodesToOutline(payload.Sections)
	if err != nil {
		return nil, err
	}
	return RenumberOutline(nodes), nil
}

func generatedNodesToOutline(nodes []generatedOutlineNode) ([]ReportOutlineNode, error) {
	if len(nodes) == 0 {
		return nil, NewError(CodeDependency, "AI outline response did not include sections", nil)
	}
	result := make([]ReportOutlineNode, 0, len(nodes))
	for _, node := range nodes {
		title := strings.TrimSpace(node.Title)
		if title == "" {
			return nil, NewError(CodeDependency, "AI outline response included an empty section title", nil)
		}
		children, err := generatedNodesToOutlineOptional(node.Children)
		if err != nil {
			return nil, err
		}
		result = append(result, ReportOutlineNode{
			ID:       newID(),
			Title:    title,
			Children: children,
		})
	}
	return result, nil
}

func generatedNodesToOutlineOptional(nodes []generatedOutlineNode) ([]ReportOutlineNode, error) {
	if len(nodes) == 0 {
		return nil, nil
	}
	return generatedNodesToOutline(nodes)
}

type generatedSectionPayload struct {
	Content string           `json:"content"`
	Tables  []map[string]any `json:"tables"`
}

func parseGeneratedSection(content string) (generatedSectionPayload, error) {
	var payload generatedSectionPayload
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return generatedSectionPayload{}, NewError(CodeDependency, "AI section response was not valid JSON", nil)
	}
	payload.Content = strings.TrimSpace(payload.Content)
	if payload.Content == "" && len(payload.Tables) == 0 {
		return generatedSectionPayload{}, NewError(CodeDependency, "AI section response was empty", nil)
	}
	if payload.Tables == nil {
		payload.Tables = []map[string]any{}
	}
	return payload, nil
}

func nextOutlineVersion(existing []ReportOutline) int {
	next := 1
	for _, outline := range existing {
		if outline.Version >= next {
			next = outline.Version + 1
		}
	}
	return next
}

func (s *ReportGenerationService) currentOutlineSections(ctx context.Context, reportID string, sections []ReportSection) ([]ReportSection, error) {
	outlines, err := s.repo.ListReportOutlines(ctx, reportID)
	if err != nil {
		return nil, dependencyError("list report outlines", err)
	}
	currentOutlineID := currentReportOutlineID(outlines)
	if currentOutlineID == "" {
		return sections, nil
	}
	return sectionsForOutline(sections, currentOutlineID), nil
}

func currentReportOutlineID(outlines []ReportOutline) string {
	var current ReportOutline
	for _, outline := range outlines {
		if !outline.IsCurrent || strings.TrimSpace(outline.ID) == "" {
			continue
		}
		if current.ID == "" || outline.Version > current.Version {
			current = outline
		}
	}
	return current.ID
}

func sectionsForOutline(sections []ReportSection, outlineID string) []ReportSection {
	outlineID = strings.TrimSpace(outlineID)
	if outlineID == "" {
		return sections
	}
	filtered := make([]ReportSection, 0, len(sections))
	for _, section := range sections {
		if strings.TrimSpace(section.OutlineID) == outlineID {
			filtered = append(filtered, section)
		}
	}
	return filtered
}

func targetGenerationSections(sections []ReportSection, job ReportJob) []ReportSection {
	if job.JobType != JobTypeSectionRegeneration || strings.TrimSpace(job.TargetID) == "" || job.TargetType != "section" {
		return sections
	}
	targetID := strings.TrimSpace(job.TargetID)
	for _, section := range sections {
		if section.ID == targetID {
			return []ReportSection{section}
		}
	}
	return nil
}

func sortSections(sections []ReportSection) {
	sort.SliceStable(sections, func(i, j int) bool {
		if sections[i].SortOrder == sections[j].SortOrder {
			return sections[i].ID < sections[j].ID
		}
		return sections[i].SortOrder < sections[j].SortOrder
	})
}

func jsonObject(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func mergedRetrievalOptions(payload map[string]any) map[string]any {
	result := map[string]any{}
	for _, key := range []string{"options", "retrieval"} {
		if nested, ok := payload[key].(map[string]any); ok {
			for nestedKey, nestedValue := range nested {
				result[nestedKey] = nestedValue
			}
		}
	}
	return result
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func stringSliceValue(value any) []string {
	switch typed := value.(type) {
	case []string:
		return trimStringSlice(typed)
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := stringValue(item); text != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func trimStringSlice(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			result = append(result, text)
		}
	}
	return result
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		if typed > 0 {
			return typed
		}
	case int32:
		if typed > 0 {
			return int(typed)
		}
	case int64:
		if typed > 0 {
			return int(typed)
		}
	case float64:
		if typed > 0 {
			return int(typed)
		}
	}
	return 0
}

func intPtrValue(value any) *int {
	if parsed := intValue(value); parsed > 0 {
		return &parsed
	}
	return nil
}

func floatPtrValue(value any) *float64 {
	switch typed := value.(type) {
	case float64:
		if typed > 0 {
			return &typed
		}
	case float32:
		parsed := float64(typed)
		if parsed > 0 {
			return &parsed
		}
	}
	return nil
}

func boolValue(value any) bool {
	parsed, ok := value.(bool)
	return ok && parsed
}
