package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/document/internal/service"
)

func TestPostgresRepositoryAdminSettingsLogsAndStats(t *testing.T) {
	databaseURL := os.Getenv("DOCUMENT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DOCUMENT_TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool := newTestPool(t, ctx, databaseURL)
	defer pool.Close()
	applyMigration(t, ctx, pool)

	repo := NewPostgresRepository(pool)
	now := time.Date(2026, 6, 30, 8, 0, 0, 0, time.UTC)
	reportType, err := repo.UpsertReportType(ctx, service.ReportType{
		Code:      "admin_report",
		Name:      "Admin Report",
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("UpsertReportType() error = %v", err)
	}
	template, err := repo.CreateReportTemplate(ctx, service.ReportTemplate{
		ID:           "00000000-0000-0000-0000-000000002101",
		TemplateName: "admin template",
		ReportType:   reportType.Code,
		Version:      1,
		Filename:     "template.docx",
		FileSize:     10,
		Enabled:      true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, service.ReportTemplateStructure{})
	if err != nil {
		t.Fatalf("CreateReportTemplate() error = %v", err)
	}
	if _, err := repo.CreateReportMaterial(ctx, service.ReportMaterial{
		ID:           "00000000-0000-0000-0000-000000002102",
		MaterialName: "material",
		MaterialType: "doc",
		Filename:     "material.docx",
		FileSize:     20,
		Enabled:      true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("CreateReportMaterial() error = %v", err)
	}
	report, err := repo.CreateReport(ctx, service.Report{
		ID:         "00000000-0000-0000-0000-000000002103",
		Name:       "admin report",
		ReportType: reportType.Code,
		TemplateID: template.ID,
		Topic:      "admin",
		Status:     service.ReportStatusGenerated,
		Source:     "backend",
		GeneratedAt: func() *time.Time {
			value := now
			return &value
		}(),
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateReport() error = %v", err)
	}
	if _, err := repo.CreateReportJob(ctx, service.ReportJob{
		ID:          "00000000-0000-0000-0000-000000002104",
		RequestID:   "req-admin",
		Source:      "api",
		JobType:     service.JobTypeContentGeneration,
		TargetType:  "report",
		TargetID:    report.ID,
		QueueName:   "document",
		ReportID:    report.ID,
		Status:      service.JobStatusSucceeded,
		MaxAttempts: 3,
		FinishedAt:  &now,
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("CreateReportJob() error = %v", err)
	}

	settings, err := repo.SaveReportSettings(ctx, service.ReportSettings{
		LLM:              service.ReportSettingsModelConfig{Provider: "ai-gateway", ProfileID: "mp-chat", Model: "gpt-test", TimeoutSeconds: 60},
		DefaultTemplates: map[string]string{reportType.Code: template.ID},
		File:             service.ReportSettingsFileDefaults{DefaultFormat: "docx", DefaultNumberingMode: "global"},
		UpdatedAt:        now,
	})
	if err != nil {
		t.Fatalf("SaveReportSettings() error = %v", err)
	}
	if settings.DefaultTemplates[reportType.Code] != template.ID {
		t.Fatalf("settings = %+v", settings)
	}
	reloaded, err := repo.GetReportSettings(ctx)
	if err != nil {
		t.Fatalf("GetReportSettings() error = %v", err)
	}
	if reloaded.LLM.ProfileID != "mp-chat" {
		t.Fatalf("reloaded settings = %+v", reloaded)
	}

	if _, err := repo.CreateOperationLog(ctx, service.OperationLog{
		ID:              "00000000-0000-0000-0000-000000002105",
		OperatorID:      "admin-1",
		OperationType:   service.OperationCreateReport,
		TargetType:      "report",
		TargetID:        report.ID,
		RequestID:       "req-admin",
		RequestSource:   "mcp",
		ToolName:        "generate_report_outline",
		OperationResult: service.OperationResultSucceeded,
		ParameterSummary: map[string]any{
			"reportType": reportType.Code,
		},
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("CreateOperationLog() error = %v", err)
	}
	logs, err := repo.ListOperationLogs(ctx, service.OperationLogListFilter{
		Page:          1,
		PageSize:      10,
		TargetType:    "report",
		RequestSource: "mcp",
	})
	if err != nil {
		t.Fatalf("ListOperationLogs() error = %v", err)
	}
	if logs.Page.Total != 1 || len(logs.Items) != 1 || logs.Items[0].ToolName != "generate_report_outline" {
		t.Fatalf("logs = %+v", logs)
	}

	overview, err := repo.GetReportStatisticsOverview(ctx, 30)
	if err != nil {
		t.Fatalf("GetReportStatisticsOverview() error = %v", err)
	}
	if overview.ReportCount != 1 || overview.TemplateCount != 3 || overview.MaterialCount != 1 || overview.JobStatusCounts[string(service.JobStatusSucceeded)] != 1 {
		t.Fatalf("overview = %+v", overview)
	}
	daily, err := repo.ListReportDailyStatistics(ctx, 30)
	if err != nil {
		t.Fatalf("ListReportDailyStatistics() error = %v", err)
	}
	if len(daily) == 0 {
		t.Fatalf("daily stats should not be empty")
	}
}

func TestInitialReportDefaultSeedIsIdempotentAndPreservesUserChanges(t *testing.T) {
	databaseURL := os.Getenv("DOCUMENT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DOCUMENT_TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool := newTestPool(t, ctx, databaseURL)
	defer pool.Close()
	applyMigration(t, ctx, pool)

	repo := NewPostgresRepository(pool)
	reportTypes, err := repo.ListReportTypes(ctx)
	if err != nil {
		t.Fatalf("ListReportTypes() error = %v", err)
	}
	typesByCode := map[string]service.ReportType{}
	for _, reportType := range reportTypes {
		typesByCode[reportType.Code] = reportType
	}
	for code, name := range map[string]string{
		"summer_peak_inspection": "迎峰度夏检查报告",
		"coal_inventory_audit":   "煤库存审计报告",
	} {
		if got := typesByCode[code]; got.Code != code || got.Name != name || !got.Enabled {
			t.Fatalf("seeded report type %q = %+v, want enabled %q", code, got, name)
		}
	}

	settings, err := repo.GetReportSettings(ctx)
	if err != nil {
		t.Fatalf("GetReportSettings() error = %v", err)
	}
	const summerTemplateID = "11111111-1111-4111-8111-111111111101"
	const coalTemplateID = "11111111-1111-4111-8111-111111111102"
	if settings.DefaultTemplates["summer_peak_inspection"] != summerTemplateID ||
		settings.DefaultTemplates["coal_inventory_audit"] != coalTemplateID {
		t.Fatalf("default template settings = %+v", settings.DefaultTemplates)
	}
	if settings.File.DefaultFormat != "docx" ||
		settings.File.DefaultNumberingMode != "global" ||
		settings.File.DefaultStyleProfileID != "first-slice-default-docx" {
		t.Fatalf("file defaults = %+v", settings.File)
	}

	var placeholderCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM report_templates
		WHERE id::text IN ($1, $2)
		  AND file_ref IS NULL
		  AND structure_json ->> 'templateStatus' = 'needs_decision'`,
		summerTemplateID,
		coalTemplateID,
	).Scan(&placeholderCount); err != nil {
		t.Fatalf("count placeholder templates: %v", err)
	}
	if placeholderCount != 2 {
		t.Fatalf("placeholder template count = %d, want 2", placeholderCount)
	}

	const customSummerTemplateID = "22222222-2222-4222-8222-222222222201"
	if _, err := pool.Exec(ctx, `
		UPDATE report_settings
		SET
			default_templates_json = default_templates_json || $1::jsonb,
			file_json = file_json || $2::jsonb
		WHERE id = 'default'`,
		`{"summer_peak_inspection":"`+customSummerTemplateID+`"}`,
		`{"defaultNumberingMode":"by_chapter","defaultStyleProfileId":"custom-style"}`,
	); err != nil {
		t.Fatalf("customize report settings: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE report_types
		SET name = 'User Summer Report', enabled = false
		WHERE code = 'summer_peak_inspection'`); err != nil {
		t.Fatalf("customize report type: %v", err)
	}

	applyMigrationFile(t, ctx, pool, "../../migrations/0003_seed_initial_report_defaults.sql")

	var reportTypeCount, reportTemplateCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM report_types
		WHERE code IN ('summer_peak_inspection', 'coal_inventory_audit')`).Scan(&reportTypeCount); err != nil {
		t.Fatalf("count report types: %v", err)
	}
	if reportTypeCount != 2 {
		t.Fatalf("report type count after rerun = %d, want 2", reportTypeCount)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM report_templates
		WHERE id::text IN ($1, $2)`,
		summerTemplateID,
		coalTemplateID,
	).Scan(&reportTemplateCount); err != nil {
		t.Fatalf("count report templates: %v", err)
	}
	if reportTemplateCount != 2 {
		t.Fatalf("report template count after rerun = %d, want 2", reportTemplateCount)
	}

	settings, err = repo.GetReportSettings(ctx)
	if err != nil {
		t.Fatalf("GetReportSettings() after rerun error = %v", err)
	}
	if settings.DefaultTemplates["summer_peak_inspection"] != customSummerTemplateID ||
		settings.DefaultTemplates["coal_inventory_audit"] != coalTemplateID {
		t.Fatalf("default templates after rerun = %+v", settings.DefaultTemplates)
	}
	if settings.File.DefaultNumberingMode != "by_chapter" || settings.File.DefaultStyleProfileID != "custom-style" {
		t.Fatalf("file defaults after rerun = %+v", settings.File)
	}

	var summerName string
	var summerEnabled bool
	if err := pool.QueryRow(ctx, `
		SELECT name, enabled
		FROM report_types
		WHERE code = 'summer_peak_inspection'`).Scan(&summerName, &summerEnabled); err != nil {
		t.Fatalf("load customized summer report type: %v", err)
	}
	if summerName != "User Summer Report" || summerEnabled {
		t.Fatalf("summer report type after rerun = %q/%v, want user value/false", summerName, summerEnabled)
	}
}
