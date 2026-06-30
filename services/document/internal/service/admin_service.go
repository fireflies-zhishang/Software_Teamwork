package service

import (
	"context"
	"strings"
	"time"
)

type AdminRepository interface {
	GetReportSettings(ctx context.Context) (ReportSettings, error)
	SaveReportSettings(ctx context.Context, settings ReportSettings) (ReportSettings, error)
	ReportTypeExists(ctx context.Context, code string) (bool, error)
	FindReportTemplateByID(ctx context.Context, id string) (ReportTemplate, error)
	GetReportStatisticsOverview(ctx context.Context, recentDays int) (ReportStatisticsOverview, error)
	ListReportDailyStatistics(ctx context.Context, days int) ([]ReportDailyStatistic, error)
	ListOperationLogs(ctx context.Context, filter OperationLogListFilter) (OperationLogListResult, error)
	CreateOperationLog(ctx context.Context, log OperationLog) (OperationLog, error)
}

type ModelProfileValidator interface {
	GetModelProfile(ctx context.Context, reqCtx RequestContext, id string) (ModelProfileReference, error)
}

type OperationLogRecorder interface {
	CreateOperationLog(ctx context.Context, log OperationLog) (OperationLog, error)
}

type AdminService struct {
	repo      AdminRepository
	profiles  ModelProfileValidator
	clock     func() time.Time
	newLogID  func() string
	recentDay int
}

func NewAdminService(repo AdminRepository, profiles ModelProfileValidator) *AdminService {
	return &AdminService{
		repo:      repo,
		profiles:  profiles,
		clock:     func() time.Time { return time.Now().UTC() },
		newLogID:  newID,
		recentDay: DefaultReportStatisticsDays,
	}
}

func (s *AdminService) GetReportSettings(ctx context.Context, reqCtx RequestContext) (ReportSettings, error) {
	if err := requireAdminContext(reqCtx); err != nil {
		return ReportSettings{}, err
	}
	settings, err := s.repo.GetReportSettings(ctx)
	if err != nil {
		return ReportSettings{}, dependencyError("get report settings", err)
	}
	return normalizeReportSettings(settings), nil
}

func (s *AdminService) UpdateReportSettings(ctx context.Context, reqCtx RequestContext, input UpdateReportSettingsInput) (ReportSettings, error) {
	if err := requireAdminContext(reqCtx); err != nil {
		return ReportSettings{}, err
	}
	current, err := s.repo.GetReportSettings(ctx)
	if err != nil {
		return ReportSettings{}, dependencyError("get report settings", err)
	}
	settings := normalizeReportSettings(current)
	if input.LLM != nil {
		updated, err := s.mergeLLMConfig(ctx, reqCtx, settings.LLM, *input.LLM)
		if err != nil {
			return ReportSettings{}, err
		}
		settings.LLM = updated
	}
	if input.DefaultTemplates != nil {
		templates, err := s.validateDefaultTemplates(ctx, *input.DefaultTemplates)
		if err != nil {
			return ReportSettings{}, err
		}
		settings.DefaultTemplates = templates
	}
	if input.File != nil {
		file, err := mergeFileDefaults(settings.File, *input.File)
		if err != nil {
			return ReportSettings{}, err
		}
		settings.File = file
	}
	settings.UpdatedAt = s.clock()

	saved, err := s.repo.SaveReportSettings(ctx, settings)
	if err != nil {
		return ReportSettings{}, dependencyError("save report settings", err)
	}
	s.record(ctx, OperationLog{
		OperatorID:       reqCtx.UserID,
		OperatorName:     reqCtx.UserID,
		OperationType:    OperationUpdateReportSettings,
		TargetType:       "report_settings",
		TargetID:         "default",
		RequestID:        reqCtx.RequestID,
		RequestSource:    "api",
		OperationResult:  OperationResultSucceeded,
		ParameterSummary: map[string]any{"updatedFields": updatedSettingsFields(input)},
		CreatedAt:        settings.UpdatedAt,
	})
	return normalizeReportSettings(saved), nil
}

func (s *AdminService) GetStatisticsOverview(ctx context.Context, reqCtx RequestContext, recentDays int) (ReportStatisticsOverview, error) {
	if err := requireAdminContext(reqCtx); err != nil {
		return ReportStatisticsOverview{}, err
	}
	days, err := normalizeStatisticsDays(recentDays)
	if err != nil {
		return ReportStatisticsOverview{}, err
	}
	overview, err := s.repo.GetReportStatisticsOverview(ctx, days)
	if err != nil {
		return ReportStatisticsOverview{}, dependencyError("get report statistics overview", err)
	}
	if overview.JobStatusCounts == nil {
		overview.JobStatusCounts = map[string]int{}
	}
	if overview.RecentDays == 0 {
		overview.RecentDays = days
	}
	return overview, nil
}

func (s *AdminService) ListDailyStatistics(ctx context.Context, reqCtx RequestContext, days int) ([]ReportDailyStatistic, error) {
	if err := requireAdminContext(reqCtx); err != nil {
		return nil, err
	}
	normalizedDays, err := normalizeStatisticsDays(days)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListReportDailyStatistics(ctx, normalizedDays)
	if err != nil {
		return nil, dependencyError("list report daily statistics", err)
	}
	if items == nil {
		items = []ReportDailyStatistic{}
	}
	return items, nil
}

func (s *AdminService) ListOperationLogs(ctx context.Context, reqCtx RequestContext, filter OperationLogListFilter) (OperationLogListResult, error) {
	if err := requireAdminContext(reqCtx); err != nil {
		return OperationLogListResult{}, err
	}
	filter.Page, filter.PageSize = normalizePage(filter.Page, filter.PageSize)
	result, err := s.repo.ListOperationLogs(ctx, filter)
	if err != nil {
		return OperationLogListResult{}, dependencyError("list report operation logs", err)
	}
	if result.Items == nil {
		result.Items = []OperationLog{}
	}
	for i := range result.Items {
		result.Items[i] = sanitizeOperationLog(result.Items[i])
	}
	if result.Page.Page == 0 {
		result.Page.Page = filter.Page
	}
	if result.Page.PageSize == 0 {
		result.Page.PageSize = filter.PageSize
	}
	return result, nil
}

func (s *AdminService) mergeLLMConfig(ctx context.Context, reqCtx RequestContext, current, patch ReportSettingsModelConfig) (ReportSettingsModelConfig, error) {
	current.Provider = DefaultReportSettingsProvider
	if strings.TrimSpace(patch.Provider) != "" && strings.TrimSpace(patch.Provider) != DefaultReportSettingsProvider {
		return ReportSettingsModelConfig{}, ValidationError(map[string]string{"llm.provider": "must be ai-gateway"})
	}
	profileID := strings.TrimSpace(patch.ProfileID)
	profileIDProvided := patch.ProfileIDSet || profileID != ""
	if !profileIDProvided {
		if patch.TimeoutSeconds > 0 {
			current.TimeoutSeconds = patch.TimeoutSeconds
		}
		return current, validateLLMConfig(current)
	}
	if profileID == "" {
		current.ProfileID = ""
		current.Model = ""
		if patch.TimeoutSeconds > 0 {
			current.TimeoutSeconds = patch.TimeoutSeconds
		}
		return current, validateLLMConfig(current)
	}
	if s.profiles == nil {
		return ReportSettingsModelConfig{}, dependencyError("ai gateway profile validator is not configured", nil)
	}
	profile, err := s.profiles.GetModelProfile(ctx, reqCtx, profileID)
	if err != nil {
		if appErr, ok := Classify(err); ok && appErr.Code == CodeNotFound {
			return ReportSettingsModelConfig{}, ValidationError(map[string]string{"llm.profileId": "does not exist"})
		}
		return ReportSettingsModelConfig{}, dependencyError("validate ai gateway profile", err)
	}
	if !profile.Enabled {
		return ReportSettingsModelConfig{}, ValidationError(map[string]string{"llm.profileId": "must reference an enabled profile"})
	}
	if strings.TrimSpace(profile.Purpose) != "" && strings.TrimSpace(profile.Purpose) != "chat" {
		return ReportSettingsModelConfig{}, ValidationError(map[string]string{"llm.profileId": "must reference a chat profile"})
	}
	current.Provider = DefaultReportSettingsProvider
	current.ProfileID = profile.ID
	current.Model = profile.Model
	current.TimeoutSeconds = profile.TimeoutSeconds
	if current.TimeoutSeconds <= 0 {
		current.TimeoutSeconds = patch.TimeoutSeconds
	}
	return current, validateLLMConfig(current)
}

func (s *AdminService) validateDefaultTemplates(ctx context.Context, values map[string]string) (map[string]string, error) {
	cleaned := map[string]string{}
	fields := map[string]string{}
	for reportType, templateID := range values {
		reportType = strings.TrimSpace(reportType)
		templateID = strings.TrimSpace(templateID)
		if reportType == "" || templateID == "" {
			fields["defaultTemplates"] = "report type and template id must not be empty"
			continue
		}
		exists, err := s.repo.ReportTypeExists(ctx, reportType)
		if err != nil {
			return nil, dependencyError("check report type", err)
		}
		if !exists {
			fields["defaultTemplates."+reportType] = "report type does not exist or is disabled"
			continue
		}
		template, err := s.repo.FindReportTemplateByID(ctx, templateID)
		if err != nil {
			fields["defaultTemplates."+reportType] = "template does not exist"
			continue
		}
		if !template.Enabled || template.DeletedAt != nil {
			fields["defaultTemplates."+reportType] = "template must be enabled"
			continue
		}
		if template.ReportType != reportType {
			fields["defaultTemplates."+reportType] = "template report type does not match"
			continue
		}
		cleaned[reportType] = templateID
	}
	if len(fields) > 0 {
		return nil, ValidationError(fields)
	}
	return cleaned, nil
}

func (s *AdminService) record(ctx context.Context, log OperationLog) {
	recordOperationLog(ctx, s.repo, log)
}

func requireAdminContext(reqCtx RequestContext) error {
	if err := requireGatewayContext(reqCtx); err != nil {
		return err
	}
	if !reqCtx.IsAdmin() {
		return NewError(CodeForbidden, "admin role is required", nil)
	}
	return nil
}

func normalizeReportSettings(settings ReportSettings) ReportSettings {
	settings.LLM.Provider = DefaultReportSettingsProvider
	if settings.DefaultTemplates == nil {
		settings.DefaultTemplates = map[string]string{}
	}
	if settings.File.DefaultFormat == "" {
		settings.File.DefaultFormat = DefaultReportSettingsFormat
	}
	if settings.File.DefaultNumberingMode == "" {
		settings.File.DefaultNumberingMode = DefaultReportNumberingMode
	}
	if settings.File.Extra == nil {
		settings.File.Extra = map[string]any{}
	}
	return settings
}

func validateLLMConfig(value ReportSettingsModelConfig) error {
	fields := map[string]string{}
	if strings.TrimSpace(value.Provider) != "" && strings.TrimSpace(value.Provider) != DefaultReportSettingsProvider {
		fields["llm.provider"] = "must be ai-gateway"
	}
	if value.TimeoutSeconds < 0 {
		fields["llm.timeoutSeconds"] = "must be greater than or equal to 0"
	}
	if len(fields) > 0 {
		return ValidationError(fields)
	}
	return nil
}

func mergeFileDefaults(current, patch ReportSettingsFileDefaults) (ReportSettingsFileDefaults, error) {
	if strings.TrimSpace(patch.DefaultFormat) != "" {
		current.DefaultFormat = strings.TrimSpace(patch.DefaultFormat)
	}
	if strings.TrimSpace(patch.DefaultNumberingMode) != "" {
		current.DefaultNumberingMode = strings.TrimSpace(patch.DefaultNumberingMode)
	}
	if patch.DefaultStyleProfileIDSet {
		current.DefaultStyleProfileID = strings.TrimSpace(patch.DefaultStyleProfileID)
	}
	if patch.Extra != nil {
		current.Extra = cloneMap(patch.Extra)
	}
	current = normalizeReportSettings(ReportSettings{File: current}).File
	fields := map[string]string{}
	if current.DefaultFormat != DefaultReportSettingsFormat {
		fields["file.defaultFormat"] = "must be docx"
	}
	if current.DefaultNumberingMode != DefaultReportNumberingMode && current.DefaultNumberingMode != ReportNumberingModeByChapter {
		fields["file.defaultNumberingMode"] = "must be global or by_chapter"
	}
	if len(fields) > 0 {
		return ReportSettingsFileDefaults{}, ValidationError(fields)
	}
	return current, nil
}

func normalizeStatisticsDays(days int) (int, error) {
	if days == 0 {
		return DefaultReportStatisticsDays, nil
	}
	if days < 1 || days > MaximumReportStatisticsDays {
		return 0, ValidationError(map[string]string{"days": "must be between 1 and 366"})
	}
	return days, nil
}

func recordOperationLog(ctx context.Context, recorder OperationLogRecorder, log OperationLog) {
	if recorder == nil {
		return
	}
	if strings.TrimSpace(log.ID) == "" {
		log.ID = newID()
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now().UTC()
	}
	log = sanitizeOperationLog(log)
	_, _ = recorder.CreateOperationLog(ctx, log)
}

// RecordOperationLog writes an operation log through the shared sanitizer.
func RecordOperationLog(ctx context.Context, recorder OperationLogRecorder, log OperationLog) {
	recordOperationLog(ctx, recorder, log)
}

func recordOperationIfSupported(ctx context.Context, candidate any, log OperationLog) {
	recorder, ok := candidate.(OperationLogRecorder)
	if !ok {
		return
	}
	recordOperationLog(ctx, recorder, log)
}

func requestSource(reqCtx RequestContext, fallback string) string {
	if strings.TrimSpace(reqCtx.CallerService) != "" {
		return strings.TrimSpace(reqCtx.CallerService)
	}
	return fallback
}

func sanitizeOperationLog(log OperationLog) OperationLog {
	log.ParameterSummary = sanitizeMap(log.ParameterSummary)
	log.Metadata = sanitizeMap(log.Metadata)
	return log
}

func sanitizeMap(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	output := map[string]any{}
	for key, value := range input {
		cleanKey := strings.TrimSpace(key)
		if cleanKey == "" || isSensitiveLogKey(cleanKey) {
			continue
		}
		output[cleanKey] = sanitizeValue(value)
	}
	return output
}

func sanitizeValue(value any) any {
	switch typed := value.(type) {
	case string:
		return sanitizeStringValue(typed)
	case map[string]any:
		return sanitizeMap(typed)
	case map[string]string:
		output := map[string]any{}
		for key, item := range typed {
			if isSensitiveLogKey(key) {
				continue
			}
			output[key] = sanitizeValue(item)
		}
		return output
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, sanitizeValue(item))
		}
		return out
	default:
		return value
	}
}

func sanitizeStringValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	if len(trimmed) > 256 || containsAny(lower, sensitiveLogValueMarkers) {
		return "[redacted]"
	}
	return trimmed
}

// SanitizeLogValue returns a low-risk value for logs and operation summaries.
func SanitizeLogValue(value string) string {
	return sanitizeStringValue(value)
}

func containsAny(value string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

var sensitiveLogValueMarkers = []string{
	"api_key",
	"apikey",
	"authorization:",
	"bearer ",
	"database_url",
	"file_ref",
	"fileref",
	"http://",
	"https://",
	"internalurl",
	"minio",
	"objectkey",
	"password",
	"prompt",
	"s3://",
	"secret",
	"signedurl",
	"storageurl",
	"token",
	"x-amz-",
}

func isSensitiveLogKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
	for _, needle := range []string{
		"apikey", "authorization", "bearer", "bucket", "content", "databaseurl",
		"downloadurl", "fileref", "internalurl", "minio", "objectkey",
		"password", "prompt", "secret", "signedurl", "storageurl", "token",
		"url",
	} {
		if strings.Contains(normalized, needle) {
			return true
		}
	}
	return false
}

func updatedSettingsFields(input UpdateReportSettingsInput) []string {
	fields := []string{}
	if input.LLM != nil {
		fields = append(fields, "llm")
	}
	if input.DefaultTemplates != nil {
		fields = append(fields, "defaultTemplates")
	}
	if input.File != nil {
		fields = append(fields, "file")
	}
	return fields
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
