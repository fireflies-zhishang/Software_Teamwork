package service

import (
	"context"
	"strings"
	"time"
)

// ReportRepository is the persistence contract the report service depends
// on. It is satisfied by repository.PostgresRepository; tests can supply a
// fake implementation instead of standing up PostgreSQL.
type ReportRepository interface {
	WithinTx(ctx context.Context, fn func(ReportRepository) error) error

	CreateReport(ctx context.Context, value Report) (Report, error)
	GetReportByID(ctx context.Context, id string) (Report, error)
	ListReports(ctx context.Context, filter ReportListFilter) ([]Report, int, error)
	UpdateReport(ctx context.Context, value Report) (Report, error)
	SoftDeleteReport(ctx context.Context, id string, deletedAt time.Time) (Report, error)

	CreateReportOutline(ctx context.Context, value ReportOutline) (ReportOutline, error)
	ListReportOutlines(ctx context.Context, reportID string) ([]ReportOutline, error)
	GetReportOutlineByID(ctx context.Context, id string) (ReportOutline, error)
	UpdateReportOutline(ctx context.Context, value ReportOutline) (ReportOutline, error)

	CreateReportSection(ctx context.Context, value ReportSection) (ReportSection, error)
	ListReportSections(ctx context.Context, reportID string) ([]ReportSection, error)
	GetReportSectionByID(ctx context.Context, id string) (ReportSection, error)
	UpdateReportSection(ctx context.Context, value ReportSection) (ReportSection, error)

	CreateReportSectionVersion(ctx context.Context, value ReportSectionVersion) (ReportSectionVersion, error)
	ListReportSectionVersions(ctx context.Context, sectionID string) ([]ReportSectionVersion, error)
}

type ReportListFilter struct {
	Page       int
	PageSize   int
	ReportType string
	Status     string
	Keyword    string
	// CreatorID restricts the result set to one creator. The service forces
	// this to the calling user's ID unless that user is an admin.
	CreatorID string
}

type ReportListResult struct {
	Items []Report
	Page  PageMeta
}

type CreateReportInput struct {
	Name           string
	ReportType     string
	TemplateID     string
	Topic          string
	Specialty      string
	BusinessObject string
	Year           int
	Source         string
}

type UpdateReportInput struct {
	Name           *string
	TemplateID     *string
	Topic          *string
	Specialty      *string
	BusinessObject *string
	Year           *int
}

type CreateOutlineInput struct {
	Source   OutlineSource
	Sections []ReportOutlineNode
}

type UpdateOutlineInput struct {
	Sections     []ReportOutlineNode
	ManualEdited *bool
}

type CreateSectionInput struct {
	OutlineNodeID string
	ParentID      string
	Title         string
	Level         int
	SortOrder     *int
	Numbering     string
	Content       string
	Tables        []map[string]any
}

type UpdateSectionInput struct {
	Title        *string
	Content      *string
	Tables       *[]map[string]any
	ManualEdited *bool
}

type SaveSectionsInput struct {
	Sections []SaveSectionInput
}

type SaveSectionInput struct {
	ID            string
	OutlineNodeID *string
	ParentID      *string
	Title         *string
	Level         *int
	SortOrder     *int
	Numbering     *string
	Content       *string
	Tables        *[]map[string]any
	ManualEdited  *bool
}

type CreateSectionVersionInput struct {
	Source       ContentSource
	Requirements string
	Content      *string
	Tables       *[]map[string]any
}

type ReportService struct {
	repo  ReportRepository
	clock func() time.Time
}

func NewReportService(repo ReportRepository) *ReportService {
	return &ReportService{repo: repo, clock: func() time.Time { return time.Now().UTC() }}
}

func (s *ReportService) now() time.Time {
	return s.clock()
}

// --- Reports ---

func (s *ReportService) ListReports(ctx context.Context, reqCtx RequestContext, filter ReportListFilter) (ReportListResult, error) {
	if err := requireGatewayContext(reqCtx); err != nil {
		return ReportListResult{}, err
	}
	if !reqCtx.IsAdmin() {
		filter.CreatorID = reqCtx.UserID
	}
	filter.Page, filter.PageSize = normalizePage(filter.Page, filter.PageSize)
	reports, total, err := s.repo.ListReports(ctx, filter)
	if err != nil {
		return ReportListResult{}, dependencyError("list reports", err)
	}
	return ReportListResult{
		Items: reports,
		Page:  PageMeta{Page: filter.Page, PageSize: filter.PageSize, Total: total},
	}, nil
}

func (s *ReportService) CreateReport(ctx context.Context, reqCtx RequestContext, input CreateReportInput) (Report, error) {
	if err := requireGatewayContext(reqCtx); err != nil {
		return Report{}, err
	}
	fields := map[string]string{}
	if strings.TrimSpace(input.Name) == "" {
		fields["name"] = "name is required"
	}
	if strings.TrimSpace(input.ReportType) == "" {
		fields["reportType"] = "reportType is required"
	}
	if strings.TrimSpace(input.TemplateID) == "" {
		fields["templateId"] = "templateId is required"
	}
	if strings.TrimSpace(input.Topic) == "" {
		fields["topic"] = "topic is required"
	}
	if len(fields) > 0 {
		return Report{}, ValidationError(fields)
	}

	source := strings.TrimSpace(input.Source)
	if source == "" {
		source = "backend"
	}
	now := s.now()
	report := Report{
		ID:             newID(),
		Name:           input.Name,
		ReportType:     input.ReportType,
		TemplateID:     input.TemplateID,
		Topic:          input.Topic,
		Specialty:      input.Specialty,
		BusinessObject: input.BusinessObject,
		Year:           input.Year,
		Status:         ReportStatusDraft,
		CreatorID:      reqCtx.UserID,
		CreatorName:    reqCtx.UserID,
		Source:         source,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	created, err := s.repo.CreateReport(ctx, report)
	if err != nil {
		return Report{}, mapRepositoryReadError(err, "create report failed")
	}
	recordOperationIfSupported(ctx, s.repo, OperationLog{
		OperatorID:      reqCtx.UserID,
		OperatorName:    reqCtx.UserID,
		OperationType:   OperationCreateReport,
		TargetType:      "report",
		TargetID:        created.ID,
		RequestID:       reqCtx.RequestID,
		RequestSource:   requestSource(reqCtx, source),
		OperationResult: OperationResultSucceeded,
		ParameterSummary: map[string]any{
			"reportType": created.ReportType,
			"templateId": created.TemplateID,
			"source":     created.Source,
		},
		CreatedAt: now,
	})
	return created, nil
}

func (s *ReportService) GetReport(ctx context.Context, reqCtx RequestContext, reportID string) (Report, error) {
	if err := requireGatewayContext(reqCtx); err != nil {
		return Report{}, err
	}
	report, err := s.repo.GetReportByID(ctx, reportID)
	if err != nil {
		return Report{}, mapRepositoryReadError(err, "report not found")
	}
	if !reqCtx.CanAccessReport(report) {
		return Report{}, NewError(CodeForbidden, "you do not have access to this report", nil)
	}
	return report, nil
}

func (s *ReportService) UpdateReport(ctx context.Context, reqCtx RequestContext, reportID string, input UpdateReportInput) (Report, error) {
	report, err := s.GetReport(ctx, reqCtx, reportID)
	if err != nil {
		return Report{}, err
	}
	if report.Status == ReportStatusDeleted || report.DeletedAt != nil {
		return Report{}, NewError(CodeConflict, "report has been deleted", nil)
	}
	if input.Name != nil {
		report.Name = *input.Name
	}
	if input.TemplateID != nil {
		report.TemplateID = *input.TemplateID
	}
	if input.Topic != nil {
		report.Topic = *input.Topic
	}
	if input.Specialty != nil {
		report.Specialty = *input.Specialty
	}
	if input.BusinessObject != nil {
		report.BusinessObject = *input.BusinessObject
	}
	if input.Year != nil {
		report.Year = *input.Year
	}
	report.UpdatedAt = s.now()
	updated, err := s.repo.UpdateReport(ctx, report)
	if err != nil {
		return Report{}, mapRepositoryReadError(err, "report not found")
	}
	recordOperationIfSupported(ctx, s.repo, OperationLog{
		OperatorID:      reqCtx.UserID,
		OperatorName:    reqCtx.UserID,
		OperationType:   OperationUpdateReport,
		TargetType:      "report",
		TargetID:        updated.ID,
		RequestID:       reqCtx.RequestID,
		RequestSource:   requestSource(reqCtx, "api"),
		OperationResult: OperationResultSucceeded,
		ParameterSummary: map[string]any{
			"nameChanged":           input.Name != nil,
			"templateChanged":       input.TemplateID != nil,
			"topicChanged":          input.Topic != nil,
			"specialtyChanged":      input.Specialty != nil,
			"businessObjectChanged": input.BusinessObject != nil,
			"yearChanged":           input.Year != nil,
		},
		CreatedAt: s.now(),
	})
	return updated, nil
}

func (s *ReportService) SoftDeleteReport(ctx context.Context, reqCtx RequestContext, reportID string) error {
	report, err := s.GetReport(ctx, reqCtx, reportID)
	if err != nil {
		return err
	}
	if report.Status == ReportStatusDeleted || report.DeletedAt != nil {
		return NewError(CodeConflict, "report has already been deleted", nil)
	}
	if _, err := s.repo.SoftDeleteReport(ctx, reportID, s.now()); err != nil {
		return mapRepositoryReadError(err, "report not found")
	}
	recordOperationIfSupported(ctx, s.repo, OperationLog{
		OperatorID:      reqCtx.UserID,
		OperatorName:    reqCtx.UserID,
		OperationType:   OperationDeleteReport,
		TargetType:      "report",
		TargetID:        reportID,
		RequestID:       reqCtx.RequestID,
		RequestSource:   requestSource(reqCtx, "api"),
		OperationResult: OperationResultSucceeded,
		CreatedAt:       s.now(),
	})
	return nil
}

// --- Outlines ---

func (s *ReportService) ListOutlines(ctx context.Context, reqCtx RequestContext, reportID string) ([]ReportOutline, error) {
	if _, err := s.GetReport(ctx, reqCtx, reportID); err != nil {
		return nil, err
	}
	outlines, err := s.repo.ListReportOutlines(ctx, reportID)
	if err != nil {
		return nil, dependencyError("list report outlines", err)
	}
	return outlines, nil
}

func (s *ReportService) CreateOutline(ctx context.Context, reqCtx RequestContext, reportID string, input CreateOutlineInput) (ReportOutline, error) {
	report, err := s.GetReport(ctx, reqCtx, reportID)
	if err != nil {
		return ReportOutline{}, err
	}
	if report.Status == ReportStatusDeleted || report.DeletedAt != nil {
		return ReportOutline{}, NewError(CodeConflict, "report has been deleted", nil)
	}
	if len(input.Sections) == 0 {
		return ReportOutline{}, ValidationError(map[string]string{"sections": "sections must not be empty"})
	}
	source := input.Source
	if source == "" {
		source = OutlineSourceManual
	}

	existing, err := s.repo.ListReportOutlines(ctx, reportID)
	if err != nil {
		return ReportOutline{}, dependencyError("list report outlines", err)
	}
	nextVersion := 1
	for _, outline := range existing {
		if outline.Version >= nextVersion {
			nextVersion = outline.Version + 1
		}
	}

	now := s.now()
	outline := ReportOutline{
		ID:           newID(),
		ReportID:     reportID,
		Sections:     RenumberOutline(assignOutlineNodeIDs(input.Sections)),
		Version:      nextVersion,
		Source:       source,
		ManualEdited: source == OutlineSourceManual,
		IsCurrent:    true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	created, err := s.repo.CreateReportOutline(ctx, outline)
	if err != nil {
		return ReportOutline{}, mapRepositoryReadError(err, "create report outline failed")
	}
	recordOperationIfSupported(ctx, s.repo, OperationLog{
		OperatorID:      reqCtx.UserID,
		OperatorName:    reqCtx.UserID,
		OperationType:   OperationSaveOutline,
		TargetType:      "outline",
		TargetID:        created.ID,
		RequestID:       reqCtx.RequestID,
		RequestSource:   requestSource(reqCtx, "api"),
		OperationResult: OperationResultSucceeded,
		ParameterSummary: map[string]any{
			"reportId":     reportID,
			"version":      created.Version,
			"source":       created.Source,
			"sectionCount": len(created.Sections),
		},
		CreatedAt: now,
	})
	return created, nil
}

func (s *ReportService) GetOutline(ctx context.Context, reqCtx RequestContext, reportID, outlineID string) (ReportOutline, error) {
	if _, err := s.GetReport(ctx, reqCtx, reportID); err != nil {
		return ReportOutline{}, err
	}
	outline, err := s.repo.GetReportOutlineByID(ctx, outlineID)
	if err != nil {
		return ReportOutline{}, mapRepositoryReadError(err, "report outline not found")
	}
	if outline.ReportID != reportID {
		return ReportOutline{}, NewError(CodeNotFound, "report outline not found", nil)
	}
	return outline, nil
}

func (s *ReportService) UpdateOutline(ctx context.Context, reqCtx RequestContext, reportID, outlineID string, input UpdateOutlineInput) (ReportOutline, error) {
	report, err := s.GetReport(ctx, reqCtx, reportID)
	if err != nil {
		return ReportOutline{}, err
	}
	if report.Status == ReportStatusDeleted || report.DeletedAt != nil {
		return ReportOutline{}, NewError(CodeConflict, "report has been deleted", nil)
	}
	outline, err := s.GetOutline(ctx, reqCtx, reportID, outlineID)
	if err != nil {
		return ReportOutline{}, err
	}
	if len(input.Sections) == 0 {
		return ReportOutline{}, ValidationError(map[string]string{"sections": "sections must not be empty"})
	}
	outline.Sections = RenumberOutline(assignOutlineNodeIDs(input.Sections))
	if input.ManualEdited != nil {
		outline.ManualEdited = *input.ManualEdited
	} else {
		outline.ManualEdited = true
	}
	outline.UpdatedAt = s.now()
	updated, err := s.repo.UpdateReportOutline(ctx, outline)
	if err != nil {
		return ReportOutline{}, mapRepositoryReadError(err, "report outline not found")
	}
	recordOperationIfSupported(ctx, s.repo, OperationLog{
		OperatorID:      reqCtx.UserID,
		OperatorName:    reqCtx.UserID,
		OperationType:   OperationSaveOutline,
		TargetType:      "outline",
		TargetID:        updated.ID,
		RequestID:       reqCtx.RequestID,
		RequestSource:   requestSource(reqCtx, "api"),
		OperationResult: OperationResultSucceeded,
		ParameterSummary: map[string]any{
			"reportId":     reportID,
			"version":      updated.Version,
			"sectionCount": len(updated.Sections),
		},
		CreatedAt: s.now(),
	})
	return updated, nil
}

func (s *ReportService) DeleteOutlineSection(ctx context.Context, reqCtx RequestContext, reportID, outlineID, sectionID string) (ReportOutline, error) {
	report, err := s.GetReport(ctx, reqCtx, reportID)
	if err != nil {
		return ReportOutline{}, err
	}
	if report.Status == ReportStatusDeleted || report.DeletedAt != nil {
		return ReportOutline{}, NewError(CodeConflict, "report has been deleted", nil)
	}
	outline, err := s.GetOutline(ctx, reqCtx, reportID, outlineID)
	if err != nil {
		return ReportOutline{}, err
	}
	remaining, removed := RemoveOutlineNode(outline.Sections, sectionID)
	if !removed {
		return ReportOutline{}, NewError(CodeNotFound, "outline section not found", nil)
	}
	outline.Sections = RenumberOutline(remaining)
	outline.ManualEdited = true
	outline.UpdatedAt = s.now()
	updated, err := s.repo.UpdateReportOutline(ctx, outline)
	if err != nil {
		return ReportOutline{}, mapRepositoryReadError(err, "report outline not found")
	}
	recordOperationIfSupported(ctx, s.repo, OperationLog{
		OperatorID:      reqCtx.UserID,
		OperatorName:    reqCtx.UserID,
		OperationType:   OperationSaveOutline,
		TargetType:      "outline",
		TargetID:        updated.ID,
		RequestID:       reqCtx.RequestID,
		RequestSource:   requestSource(reqCtx, "api"),
		OperationResult: OperationResultSucceeded,
		ParameterSummary: map[string]any{
			"reportId":         reportID,
			"deletedSectionId": sectionID,
			"sectionCount":     len(updated.Sections),
		},
		CreatedAt: s.now(),
	})
	return updated, nil
}

func assignOutlineNodeIDs(nodes []ReportOutlineNode) []ReportOutlineNode {
	result := make([]ReportOutlineNode, len(nodes))
	for i, node := range nodes {
		if strings.TrimSpace(node.ID) == "" {
			node.ID = newID()
		}
		if len(node.Children) > 0 {
			node.Children = assignOutlineNodeIDs(node.Children)
		}
		result[i] = node
	}
	return result
}

// --- Sections ---

func (s *ReportService) ListSections(ctx context.Context, reqCtx RequestContext, reportID string) ([]ReportSection, error) {
	if _, err := s.GetReport(ctx, reqCtx, reportID); err != nil {
		return nil, err
	}
	sections, err := s.repo.ListReportSections(ctx, reportID)
	if err != nil {
		return nil, dependencyError("list report sections", err)
	}
	return sections, nil
}

func (s *ReportService) CreateSection(ctx context.Context, reqCtx RequestContext, reportID string, input CreateSectionInput) (ReportSection, error) {
	report, err := s.GetReport(ctx, reqCtx, reportID)
	if err != nil {
		return ReportSection{}, err
	}
	if report.Status == ReportStatusDeleted || report.DeletedAt != nil {
		return ReportSection{}, NewError(CodeConflict, "report has been deleted", nil)
	}
	if strings.TrimSpace(input.Title) == "" {
		return ReportSection{}, ValidationError(map[string]string{"title": "title is required"})
	}
	if err := validateSectionSortOrder(input.SortOrder); err != nil {
		return ReportSection{}, err
	}

	siblings, err := s.repo.ListReportSections(ctx, reportID)
	if err != nil {
		return ReportSection{}, dependencyError("list report sections", err)
	}
	sortOrder := nextSectionSortOrder(siblings, input.ParentID)
	if input.SortOrder != nil {
		sortOrder = *input.SortOrder
	}
	section := buildNewSection(reportID, input, sortOrder, s.now())
	if err := validateSectionParent(reportID, section, sectionsByID(siblings)); err != nil {
		return ReportSection{}, err
	}
	created, err := s.repo.CreateReportSection(ctx, section)
	if err != nil {
		return ReportSection{}, mapRepositoryReadError(err, "create report section failed")
	}
	recordOperationIfSupported(ctx, s.repo, OperationLog{
		OperatorID:      reqCtx.UserID,
		OperatorName:    reqCtx.UserID,
		OperationType:   OperationUpdateSection,
		TargetType:      "section",
		TargetID:        created.ID,
		RequestID:       reqCtx.RequestID,
		RequestSource:   requestSource(reqCtx, "api"),
		OperationResult: OperationResultSucceeded,
		ParameterSummary: map[string]any{
			"reportId":   reportID,
			"title":      created.Title,
			"createdNew": true,
		},
		CreatedAt: s.now(),
	})
	return created, nil
}

func (s *ReportService) GetSection(ctx context.Context, reqCtx RequestContext, reportID, sectionID string) (ReportSection, error) {
	if _, err := s.GetReport(ctx, reqCtx, reportID); err != nil {
		return ReportSection{}, err
	}
	section, err := s.repo.GetReportSectionByID(ctx, sectionID)
	if err != nil {
		return ReportSection{}, mapRepositoryReadError(err, "report section not found")
	}
	if section.ReportID != reportID {
		return ReportSection{}, NewError(CodeNotFound, "report section not found", nil)
	}
	return section, nil
}

func (s *ReportService) UpdateSection(ctx context.Context, reqCtx RequestContext, reportID, sectionID string, input UpdateSectionInput) (ReportSection, error) {
	report, err := s.GetReport(ctx, reqCtx, reportID)
	if err != nil {
		return ReportSection{}, err
	}
	if report.Status == ReportStatusDeleted || report.DeletedAt != nil {
		return ReportSection{}, NewError(CodeConflict, "report has been deleted", nil)
	}
	section, err := s.GetSection(ctx, reqCtx, reportID, sectionID)
	if err != nil {
		return ReportSection{}, err
	}
	if section.GenerationStatus == JobStatusRunning {
		return ReportSection{}, NewError(CodeConflict, "section content generation is in progress", nil)
	}

	section = applySectionUpdate(section, input, s.now())
	updated, err := s.repo.UpdateReportSection(ctx, section)
	if err != nil {
		return ReportSection{}, mapRepositoryReadError(err, "report section not found")
	}
	recordOperationIfSupported(ctx, s.repo, OperationLog{
		OperatorID:      reqCtx.UserID,
		OperatorName:    reqCtx.UserID,
		OperationType:   OperationUpdateSection,
		TargetType:      "section",
		TargetID:        updated.ID,
		RequestID:       reqCtx.RequestID,
		RequestSource:   requestSource(reqCtx, "api"),
		OperationResult: OperationResultSucceeded,
		ParameterSummary: map[string]any{
			"reportId":       reportID,
			"titleChanged":   input.Title != nil,
			"contentChanged": input.Content != nil,
			"tablesChanged":  input.Tables != nil,
		},
		CreatedAt: s.now(),
	})
	return updated, nil
}

func (s *ReportService) SaveSections(ctx context.Context, reqCtx RequestContext, reportID string, input SaveSectionsInput) ([]ReportSection, error) {
	report, err := s.GetReport(ctx, reqCtx, reportID)
	if err != nil {
		return nil, err
	}
	if report.Status == ReportStatusDeleted || report.DeletedAt != nil {
		return nil, NewError(CodeConflict, "report has been deleted", nil)
	}
	if len(input.Sections) == 0 {
		return nil, ValidationError(map[string]string{"sections": "sections must not be empty"})
	}

	saved := make([]ReportSection, 0, len(input.Sections))
	err = s.repo.WithinTx(ctx, func(txRepo ReportRepository) error {
		existing, err := txRepo.ListReportSections(ctx, reportID)
		if err != nil {
			return dependencyError("list report sections", err)
		}
		byID := sectionsByID(existing)

		for _, item := range input.Sections {
			if err := validateSectionSortOrder(item.SortOrder); err != nil {
				return err
			}
			sectionID := strings.TrimSpace(item.ID)
			if sectionID == "" {
				createInput, err := createInputFromSaveSection(item)
				if err != nil {
					return err
				}
				section := buildNewSection(reportID, createInput, nextSectionSortOrder(existing, createInput.ParentID), s.now())
				if item.SortOrder != nil {
					section.SortOrder = *item.SortOrder
				}
				if err := validateSectionParent(reportID, section, byID); err != nil {
					return err
				}
				created, err := txRepo.CreateReportSection(ctx, section)
				if err != nil {
					return mapRepositoryReadError(err, "create report section failed")
				}
				existing = append(existing, created)
				byID[created.ID] = created
				saved = append(saved, created)
				continue
			}

			section, ok := byID[sectionID]
			if !ok || section.ReportID != reportID {
				return NewError(CodeNotFound, "report section not found", nil)
			}
			if section.GenerationStatus == JobStatusRunning {
				return NewError(CodeConflict, "section content generation is in progress", nil)
			}
			section = applySectionSave(section, item, s.now())
			if err := validateSectionParent(reportID, section, byID); err != nil {
				return err
			}
			updated, err := txRepo.UpdateReportSection(ctx, section)
			if err != nil {
				return mapRepositoryReadError(err, "report section not found")
			}
			byID[updated.ID] = updated
			for i, existingSection := range existing {
				if existingSection.ID == updated.ID {
					existing[i] = updated
					break
				}
			}
			saved = append(saved, updated)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	recordOperationIfSupported(ctx, s.repo, OperationLog{
		OperatorID:      reqCtx.UserID,
		OperatorName:    reqCtx.UserID,
		OperationType:   OperationUpdateSection,
		TargetType:      "report",
		TargetID:        reportID,
		RequestID:       reqCtx.RequestID,
		RequestSource:   requestSource(reqCtx, "api"),
		OperationResult: OperationResultSucceeded,
		ParameterSummary: map[string]any{
			"sectionCount": len(saved),
		},
		CreatedAt: s.now(),
	})
	return saved, nil
}

func applySectionUpdate(section ReportSection, input UpdateSectionInput, updatedAt time.Time) ReportSection {
	contentChanged := false
	if input.Title != nil {
		section.Title = *input.Title
	}
	if input.Content != nil {
		section.Content = *input.Content
		contentChanged = true
	}
	if input.Tables != nil {
		section.Tables = *input.Tables
		contentChanged = true
	}
	if contentChanged {
		section.Version++
		// A content/table edit is always a manual edit, and must not be
		// reversible by also passing manualEdited:false in the same
		// request: that would defeat the "generation must not silently
		// overwrite manual edits" guarantee.
		section.ManualEdited = true
		if section.ContentSource == ContentSourceAI {
			section.ContentSource = ContentSourceMixed
		} else {
			section.ContentSource = ContentSourceManual
		}
	} else if input.ManualEdited != nil {
		section.ManualEdited = *input.ManualEdited
	}
	section.UpdatedAt = updatedAt
	return section
}

func applySectionSave(section ReportSection, input SaveSectionInput, updatedAt time.Time) ReportSection {
	if input.OutlineNodeID != nil {
		section.OutlineNodeID = *input.OutlineNodeID
	}
	if input.ParentID != nil {
		section.ParentID = strings.TrimSpace(*input.ParentID)
	}
	if input.Level != nil {
		section.Level = *input.Level
		if section.Level <= 0 {
			section.Level = 1
		}
	}
	if input.SortOrder != nil {
		section.SortOrder = *input.SortOrder
	}
	if input.Numbering != nil {
		section.Numbering = *input.Numbering
	}
	return applySectionUpdate(section, UpdateSectionInput{
		Title:        input.Title,
		Content:      input.Content,
		Tables:       input.Tables,
		ManualEdited: input.ManualEdited,
	}, updatedAt)
}

func createInputFromSaveSection(input SaveSectionInput) (CreateSectionInput, error) {
	if input.Title == nil || strings.TrimSpace(*input.Title) == "" {
		return CreateSectionInput{}, ValidationError(map[string]string{"sections": "new sections require title"})
	}
	level := 0
	if input.Level != nil {
		level = *input.Level
	}
	tables := []map[string]any(nil)
	if input.Tables != nil {
		tables = *input.Tables
	}
	return CreateSectionInput{
		OutlineNodeID: stringPtrValue(input.OutlineNodeID),
		ParentID:      stringPtrValue(input.ParentID),
		Title:         *input.Title,
		Level:         level,
		SortOrder:     input.SortOrder,
		Numbering:     stringPtrValue(input.Numbering),
		Content:       stringPtrValue(input.Content),
		Tables:        tables,
	}, nil
}

func buildNewSection(reportID string, input CreateSectionInput, sortOrder int, now time.Time) ReportSection {
	level := input.Level
	if level <= 0 {
		level = 1
	}

	// content_source is NOT NULL in the database, and this endpoint only
	// ever creates sections manually (AI generation is out of scope for
	// C-03), so it is always "manual" regardless of whether content is
	// provided yet.
	manualEdited := strings.TrimSpace(input.Content) != "" || len(input.Tables) > 0
	id := newID()
	return ReportSection{
		ID:               id,
		ReportID:         reportID,
		ParentID:         strings.TrimSpace(input.ParentID),
		OutlineNodeID:    input.OutlineNodeID,
		SectionPath:      id,
		Title:            input.Title,
		Level:            level,
		SortOrder:        sortOrder,
		Numbering:        input.Numbering,
		SectionType:      SectionTypeText,
		Content:          input.Content,
		Tables:           input.Tables,
		GenerationStatus: JobStatusPending,
		ContentSource:    ContentSourceManual,
		ManualEdited:     manualEdited,
		Version:          1,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

func nextSectionSortOrder(sections []ReportSection, parentID string) int {
	sortOrder := 0
	for _, section := range sections {
		if section.ParentID == parentID && section.SortOrder >= sortOrder {
			sortOrder = section.SortOrder + 1
		}
	}
	return sortOrder
}

func sectionsByID(sections []ReportSection) map[string]ReportSection {
	byID := map[string]ReportSection{}
	for _, section := range sections {
		byID[section.ID] = section
	}
	return byID
}

func validateSectionSortOrder(sortOrder *int) error {
	if sortOrder != nil && *sortOrder < 0 {
		return ValidationError(map[string]string{"sortOrder": "must be greater than or equal to 0"})
	}
	return nil
}

func validateSectionParent(reportID string, section ReportSection, sections map[string]ReportSection) error {
	parentID := strings.TrimSpace(section.ParentID)
	if parentID == "" {
		return nil
	}
	if parentID == section.ID {
		return ValidationError(map[string]string{"parentId": "must not reference the same section"})
	}
	parent, ok := sections[parentID]
	if !ok || parent.ReportID != reportID {
		return ValidationError(map[string]string{"parentId": "must reference a section in the same report"})
	}

	seen := map[string]struct{}{section.ID: {}}
	current := parent
	for {
		if _, ok := seen[current.ID]; ok {
			return ValidationError(map[string]string{"parentId": "must not create a section cycle"})
		}
		seen[current.ID] = struct{}{}

		nextID := strings.TrimSpace(current.ParentID)
		if nextID == "" {
			return nil
		}
		if nextID == section.ID {
			return ValidationError(map[string]string{"parentId": "must not create a section cycle"})
		}
		next, ok := sections[nextID]
		if !ok || next.ReportID != reportID {
			return ValidationError(map[string]string{"parentId": "must reference a section in the same report"})
		}
		current = next
	}
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// --- Section versions ---

func (s *ReportService) ListSectionVersions(ctx context.Context, reqCtx RequestContext, reportID, sectionID string) ([]ReportSectionVersion, error) {
	if _, err := s.GetSection(ctx, reqCtx, reportID, sectionID); err != nil {
		return nil, err
	}
	versions, err := s.repo.ListReportSectionVersions(ctx, sectionID)
	if err != nil {
		return nil, dependencyError("list report section versions", err)
	}
	return versions, nil
}

func (s *ReportService) CreateSectionVersion(ctx context.Context, reqCtx RequestContext, reportID, sectionID string, input CreateSectionVersionInput) (ReportSectionVersion, error) {
	section, err := s.GetSection(ctx, reqCtx, reportID, sectionID)
	if err != nil {
		return ReportSectionVersion{}, err
	}
	if input.Source != ContentSourceManual && input.Source != ContentSourceAI {
		return ReportSectionVersion{}, ValidationError(map[string]string{"source": "source must be manual or ai"})
	}

	content := section.Content
	if input.Content != nil {
		content = *input.Content
	}
	tables := section.Tables
	if input.Tables != nil {
		tables = *input.Tables
	}

	existing, err := s.repo.ListReportSectionVersions(ctx, sectionID)
	if err != nil {
		return ReportSectionVersion{}, dependencyError("list report section versions", err)
	}
	nextVersion := 1
	for _, version := range existing {
		if version.Version >= nextVersion {
			nextVersion = version.Version + 1
		}
	}

	version := ReportSectionVersion{
		ID:           newID(),
		ReportID:     reportID,
		SectionID:    sectionID,
		Version:      nextVersion,
		Source:       input.Source,
		Content:      content,
		Tables:       tables,
		Requirements: input.Requirements,
		CreatedBy:    reqCtx.UserID,
		CreatedAt:    s.now(),
	}
	created, err := s.repo.CreateReportSectionVersion(ctx, version)
	if err != nil {
		return ReportSectionVersion{}, mapRepositoryReadError(err, "create report section version failed")
	}
	recordOperationIfSupported(ctx, s.repo, OperationLog{
		OperatorID:      reqCtx.UserID,
		OperatorName:    reqCtx.UserID,
		OperationType:   OperationCreateSectionVersion,
		TargetType:      "section",
		TargetID:        sectionID,
		RequestID:       reqCtx.RequestID,
		RequestSource:   requestSource(reqCtx, "api"),
		OperationResult: OperationResultSucceeded,
		ParameterSummary: map[string]any{
			"reportId": reportID,
			"version":  created.Version,
			"source":   created.Source,
		},
		CreatedAt: s.now(),
	})
	return created, nil
}
