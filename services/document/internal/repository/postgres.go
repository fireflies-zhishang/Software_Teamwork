package repository

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/document/internal/repository/sqlc"
	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/document/internal/service"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool    *pgxpool.Pool
	db      sqlc.DBTX
	queries *sqlc.Queries
}

func NewPostgres(ctx context.Context, databaseURL string) (*PostgresRepository, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, errors.New("DOCUMENT_DATABASE_URL is required")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, errors.New("DOCUMENT_DATABASE_URL is invalid")
	}
	config.MaxConns = 10
	config.MinConns = 1
	config.MaxConnLifetime = 30 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	return NewPostgresRepository(pool), nil
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool, db: pool, queries: sqlc.New(pool)}
}

func (r *PostgresRepository) Close() {
	r.pool.Close()
}

func (r *PostgresRepository) CheckReady(ctx context.Context) error {
	return r.pool.Ping(ctx)
}

func (r *PostgresRepository) WithinTx(ctx context.Context, fn func(service.ReportRepository) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin document transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txRepo := &PostgresRepository{
		pool:    r.pool,
		db:      tx,
		queries: r.queries.WithTx(tx),
	}
	if err := fn(txRepo); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit document transaction: %w", err)
	}
	return nil
}

func (r *PostgresRepository) WithinJobTx(ctx context.Context, fn func(service.JobRepository) error) error {
	if _, inTx := r.db.(pgx.Tx); inTx {
		return fn(r)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin report job transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txRepo := &PostgresRepository{
		pool:    r.pool,
		db:      tx,
		queries: r.queries.WithTx(tx),
	}
	if err := fn(txRepo); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit report job transaction: %w", err)
	}
	return nil
}

func (r *PostgresRepository) WithinGenerationTx(ctx context.Context, fn func(service.ReportGenerationRepository) error) error {
	if _, inTx := r.db.(pgx.Tx); inTx {
		return fn(r)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin report generation transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txRepo := &PostgresRepository{
		pool:    r.pool,
		db:      tx,
		queries: r.queries.WithTx(tx),
	}
	if err := fn(txRepo); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit report generation transaction: %w", err)
	}
	return nil
}

func (r *PostgresRepository) UpsertReportType(ctx context.Context, value service.ReportType) (service.ReportType, error) {
	if value.CreatedAt.IsZero() {
		value.CreatedAt = time.Now().UTC()
	}
	if value.UpdatedAt.IsZero() {
		value.UpdatedAt = value.CreatedAt
	}
	defaultTemplateID, err := parseOptionalUUIDField(value.DefaultTemplateID, "defaultTemplateId")
	if err != nil {
		return service.ReportType{}, err
	}
	row := r.db.QueryRow(ctx, `
		INSERT INTO report_types (
			code, name, description, enabled, default_template_id, created_at, updated_at
		)
		VALUES ($1, $2, NULLIF($3, ''), $4, NULLIF($5, '')::uuid, $6, $7)
		ON CONFLICT (code) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			enabled = EXCLUDED.enabled,
			default_template_id = EXCLUDED.default_template_id,
			updated_at = EXCLUDED.updated_at
		RETURNING code, name, COALESCE(description, ''), enabled, COALESCE(default_template_id::text, ''), created_at, updated_at`,
		value.Code,
		value.Name,
		value.Description,
		value.Enabled,
		defaultTemplateID,
		value.CreatedAt,
		value.UpdatedAt,
	)
	return scanReportType(row)
}

func (r *PostgresRepository) ListReportTypes(ctx context.Context) ([]service.ReportType, error) {
	rows, err := r.db.Query(ctx, `
		SELECT code, name, COALESCE(description, ''), enabled, COALESCE(default_template_id::text, ''), created_at, updated_at
		FROM report_types
		WHERE enabled = true
		ORDER BY code`)
	if err != nil {
		return nil, fmt.Errorf("list report types: %w", err)
	}
	defer rows.Close()

	values := []service.ReportType{}
	for rows.Next() {
		value, err := scanReportType(rows)
		if err != nil {
			return nil, fmt.Errorf("scan report type: %w", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate report types: %w", err)
	}
	return values, nil
}

func (r *PostgresRepository) ReportTypeExists(ctx context.Context, code string) (bool, error) {
	var exists bool
	if err := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM report_types WHERE code = $1 AND enabled = true
		)`, code).Scan(&exists); err != nil {
		return false, fmt.Errorf("check report type exists: %w", err)
	}
	return exists, nil
}

func (r *PostgresRepository) ListReportTemplates(ctx context.Context, filter service.ReportTemplateListFilter) (service.ReportTemplateListResult, error) {
	where := []string{"deleted_at IS NULL"}
	args := []any{}
	if strings.TrimSpace(filter.ReportType) != "" {
		args = append(args, strings.TrimSpace(filter.ReportType))
		where = append(where, fmt.Sprintf("report_type = $%d", len(args)))
	}
	if filter.Enabled != nil {
		args = append(args, *filter.Enabled)
		where = append(where, fmt.Sprintf("enabled = $%d", len(args)))
	}
	whereSQL := strings.Join(where, " AND ")

	var total int64
	countQuery := "SELECT count(*) FROM report_templates WHERE " + whereSQL
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return service.ReportTemplateListResult{}, fmt.Errorf("count report templates: %w", err)
	}

	offset := (filter.Page - 1) * filter.PageSize
	queryArgs := append(append([]any{}, args...), filter.PageSize, offset)
	query := fmt.Sprintf(`
		SELECT
			id::text, template_name, report_type, version, COALESCE(file_ref, ''),
			filename, file_size, COALESCE(description, ''), enabled, COALESCE(created_by, ''),
			created_at, updated_at, deleted_at
		FROM report_templates
		WHERE %s
		ORDER BY created_at DESC, id DESC
		LIMIT $%d OFFSET $%d`, whereSQL, len(queryArgs)-1, len(queryArgs))
	rows, err := r.db.Query(ctx, query, queryArgs...)
	if err != nil {
		return service.ReportTemplateListResult{}, fmt.Errorf("list report templates: %w", err)
	}
	defer rows.Close()

	items := []service.ReportTemplate{}
	for rows.Next() {
		item, err := scanReportTemplate(rows)
		if err != nil {
			return service.ReportTemplateListResult{}, fmt.Errorf("scan report template: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return service.ReportTemplateListResult{}, fmt.Errorf("iterate report templates: %w", err)
	}
	return service.ReportTemplateListResult{
		Items: items,
		Page:  service.PageMeta{Page: filter.Page, PageSize: filter.PageSize, Total: int(total)},
	}, nil
}

func (r *PostgresRepository) CreateReportTemplate(ctx context.Context, value service.ReportTemplate, structure service.ReportTemplateStructure) (service.ReportTemplate, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO report_templates (
			id, template_name, report_type, version, file_ref, filename, file_size,
			structure_json, style_config_json, description, enabled, created_by, created_at, updated_at
		)
		VALUES (
			$1, $2, $3, $4, NULLIF($5, ''), $6, $7,
			$8::jsonb, $9::jsonb, NULLIF($10, ''), $11, NULLIF($12, ''), $13, $14
		)
		RETURNING
			id::text, template_name, report_type, version, COALESCE(file_ref, ''),
			filename, file_size, COALESCE(description, ''), enabled, COALESCE(created_by, ''),
			created_at, updated_at, deleted_at`,
		value.ID,
		value.TemplateName,
		value.ReportType,
		value.Version,
		value.FileRef,
		value.Filename,
		value.FileSize,
		jsonRawStringOrDefault(structure.OutlineSchema, "[]"),
		jsonRawStringOrDefault(structure.StyleConfig, "{}"),
		value.Description,
		value.Enabled,
		value.CreatedBy,
		value.CreatedAt,
		value.UpdatedAt,
	)
	created, err := scanReportTemplate(row)
	if err != nil {
		if isUniqueViolation(err) {
			return service.ReportTemplate{}, service.NewError(service.CodeConflict, "report template already exists", err)
		}
		return service.ReportTemplate{}, fmt.Errorf("insert report template: %w", err)
	}
	return created, nil
}

func (r *PostgresRepository) FindReportTemplateByID(ctx context.Context, id string) (service.ReportTemplate, error) {
	templateID, err := parseUUID(id)
	if err != nil {
		return service.ReportTemplate{}, service.ValidationError(map[string]string{"reportTemplateId": "must be a valid UUID"})
	}
	row := r.db.QueryRow(ctx, `
		SELECT
			id::text, template_name, report_type, version, COALESCE(file_ref, ''),
			filename, file_size, COALESCE(description, ''), enabled, COALESCE(created_by, ''),
			created_at, updated_at, deleted_at
		FROM report_templates
		WHERE id = $1 AND deleted_at IS NULL`, templateID)
	template, err := scanReportTemplate(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.ReportTemplate{}, service.NewError(service.CodeNotFound, "report template not found", err)
		}
		return service.ReportTemplate{}, fmt.Errorf("find report template: %w", err)
	}
	return template, nil
}

func (r *PostgresRepository) UpdateReportTemplate(ctx context.Context, input service.UpdateReportTemplateInput) (service.ReportTemplate, error) {
	templateID, err := parseUUID(input.ID)
	if err != nil {
		return service.ReportTemplate{}, service.ValidationError(map[string]string{"reportTemplateId": "must be a valid UUID"})
	}
	templateName := ""
	if input.TemplateName != nil {
		templateName = strings.TrimSpace(*input.TemplateName)
	}
	description := ""
	if input.Description != nil {
		description = strings.TrimSpace(*input.Description)
	}
	enabled := false
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	row := r.db.QueryRow(ctx, `
		UPDATE report_templates
		SET
			template_name = CASE WHEN $2 THEN $3 ELSE template_name END,
			description = CASE WHEN $4 THEN NULLIF($5, '') ELSE description END,
			enabled = CASE WHEN $6 THEN $7 ELSE enabled END,
			updated_at = $8
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING
			id::text, template_name, report_type, version, COALESCE(file_ref, ''),
			filename, file_size, COALESCE(description, ''), enabled, COALESCE(created_by, ''),
			created_at, updated_at, deleted_at`,
		templateID,
		input.TemplateName != nil,
		templateName,
		input.Description != nil,
		description,
		input.Enabled != nil,
		enabled,
		time.Now().UTC(),
	)
	template, err := scanReportTemplate(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.ReportTemplate{}, service.NewError(service.CodeNotFound, "report template not found", err)
		}
		return service.ReportTemplate{}, fmt.Errorf("update report template: %w", err)
	}
	return template, nil
}

func (r *PostgresRepository) DeleteReportTemplate(ctx context.Context, id string, deletedAt time.Time) error {
	templateID, err := parseUUID(id)
	if err != nil {
		return service.ValidationError(map[string]string{"reportTemplateId": "must be a valid UUID"})
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE report_templates
		SET deleted_at = $2, enabled = false, updated_at = $2
		WHERE id = $1 AND deleted_at IS NULL`, templateID, deletedAt)
	if err != nil {
		return fmt.Errorf("delete report template: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return service.NewError(service.CodeNotFound, "report template not found", pgx.ErrNoRows)
	}
	return nil
}

func (r *PostgresRepository) GetReportTemplateStructure(ctx context.Context, id string) (service.ReportTemplateStructure, error) {
	templateID, err := parseUUID(id)
	if err != nil {
		return service.ReportTemplateStructure{}, service.ValidationError(map[string]string{"reportTemplateId": "must be a valid UUID"})
	}
	var structure, style []byte
	if err := r.db.QueryRow(ctx, `
		SELECT structure_json, style_config_json
		FROM report_templates
		WHERE id = $1 AND deleted_at IS NULL`, templateID).Scan(&structure, &style); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.ReportTemplateStructure{}, service.NewError(service.CodeNotFound, "report template not found", err)
		}
		return service.ReportTemplateStructure{}, fmt.Errorf("get report template structure: %w", err)
	}
	return service.ReportTemplateStructure{OutlineSchema: structure, StyleConfig: style}, nil
}

func (r *PostgresRepository) UpdateReportTemplateStructure(ctx context.Context, id string, structure service.ReportTemplateStructure, updatedAt time.Time) (service.ReportTemplateStructure, error) {
	templateID, err := parseUUID(id)
	if err != nil {
		return service.ReportTemplateStructure{}, service.ValidationError(map[string]string{"reportTemplateId": "must be a valid UUID"})
	}
	var outline, style []byte
	if err := r.db.QueryRow(ctx, `
		UPDATE report_templates
		SET structure_json = $2::jsonb, style_config_json = $3::jsonb, updated_at = $4
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING structure_json, style_config_json`,
		templateID,
		jsonRawStringOrDefault(structure.OutlineSchema, "[]"),
		jsonRawStringOrDefault(structure.StyleConfig, "{}"),
		updatedAt,
	).Scan(&outline, &style); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.ReportTemplateStructure{}, service.NewError(service.CodeNotFound, "report template not found", err)
		}
		return service.ReportTemplateStructure{}, fmt.Errorf("update report template structure: %w", err)
	}
	return service.ReportTemplateStructure{OutlineSchema: outline, StyleConfig: style}, nil
}

func (r *PostgresRepository) ListReportMaterials(ctx context.Context, filter service.ReportMaterialListFilter) (service.ReportMaterialListResult, error) {
	where := []string{"deleted_at IS NULL"}
	args := []any{}
	if strings.TrimSpace(filter.Category) != "" {
		args = append(args, strings.TrimSpace(filter.Category))
		where = append(where, fmt.Sprintf("category = $%d", len(args)))
	}
	if filter.Enabled != nil {
		args = append(args, *filter.Enabled)
		where = append(where, fmt.Sprintf("enabled = $%d", len(args)))
	}
	whereSQL := strings.Join(where, " AND ")

	var total int64
	countQuery := "SELECT count(*) FROM report_materials WHERE " + whereSQL
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return service.ReportMaterialListResult{}, fmt.Errorf("count report materials: %w", err)
	}

	offset := (filter.Page - 1) * filter.PageSize
	queryArgs := append(append([]any{}, args...), filter.PageSize, offset)
	query := fmt.Sprintf(`
		SELECT
			id::text, material_name, material_type, COALESCE(category, ''), COALESCE(file_ref, ''),
			filename, file_size, COALESCE(description, ''), tags_json, enabled, COALESCE(created_by, ''),
			created_at, updated_at, deleted_at
		FROM report_materials
		WHERE %s
		ORDER BY created_at DESC, id DESC
		LIMIT $%d OFFSET $%d`, whereSQL, len(queryArgs)-1, len(queryArgs))
	rows, err := r.db.Query(ctx, query, queryArgs...)
	if err != nil {
		return service.ReportMaterialListResult{}, fmt.Errorf("list report materials: %w", err)
	}
	defer rows.Close()

	items := []service.ReportMaterial{}
	for rows.Next() {
		item, err := scanReportMaterial(rows)
		if err != nil {
			return service.ReportMaterialListResult{}, fmt.Errorf("scan report material: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return service.ReportMaterialListResult{}, fmt.Errorf("iterate report materials: %w", err)
	}
	return service.ReportMaterialListResult{
		Items: items,
		Page:  service.PageMeta{Page: filter.Page, PageSize: filter.PageSize, Total: int(total)},
	}, nil
}

func (r *PostgresRepository) CreateReportMaterial(ctx context.Context, value service.ReportMaterial) (service.ReportMaterial, error) {
	tags, err := json.Marshal(value.Tags)
	if err != nil {
		return service.ReportMaterial{}, fmt.Errorf("encode material tags: %w", err)
	}
	row := r.db.QueryRow(ctx, `
		INSERT INTO report_materials (
			id, material_name, material_type, category, file_ref, filename, file_size,
			description, tags_json, enabled, created_by, created_at, updated_at
		)
		VALUES (
			$1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6, $7,
			NULLIF($8, ''), $9::jsonb, $10, NULLIF($11, ''), $12, $13
		)
		RETURNING
			id::text, material_name, material_type, COALESCE(category, ''), COALESCE(file_ref, ''),
			filename, file_size, COALESCE(description, ''), tags_json, enabled, COALESCE(created_by, ''),
			created_at, updated_at, deleted_at`,
		value.ID,
		value.MaterialName,
		value.MaterialType,
		value.Category,
		value.FileRef,
		value.Filename,
		value.FileSize,
		value.Description,
		string(tags),
		value.Enabled,
		value.CreatedBy,
		value.CreatedAt,
		value.UpdatedAt,
	)
	created, err := scanReportMaterial(row)
	if err != nil {
		if isUniqueViolation(err) {
			return service.ReportMaterial{}, service.NewError(service.CodeConflict, "report material already exists", err)
		}
		return service.ReportMaterial{}, fmt.Errorf("insert report material: %w", err)
	}
	return created, nil
}

func (r *PostgresRepository) FindReportMaterialByID(ctx context.Context, id string) (service.ReportMaterial, error) {
	materialID, err := parseUUID(id)
	if err != nil {
		return service.ReportMaterial{}, service.ValidationError(map[string]string{"materialId": "must be a valid UUID"})
	}
	row := r.db.QueryRow(ctx, `
		SELECT
			id::text, material_name, material_type, COALESCE(category, ''), COALESCE(file_ref, ''),
			filename, file_size, COALESCE(description, ''), tags_json, enabled, COALESCE(created_by, ''),
			created_at, updated_at, deleted_at
		FROM report_materials
		WHERE id = $1 AND deleted_at IS NULL`, materialID)
	material, err := scanReportMaterial(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.ReportMaterial{}, service.NewError(service.CodeNotFound, "report material not found", err)
		}
		return service.ReportMaterial{}, fmt.Errorf("find report material: %w", err)
	}
	return material, nil
}

func (r *PostgresRepository) DeleteReportMaterial(ctx context.Context, id string, deletedAt time.Time) error {
	materialID, err := parseUUID(id)
	if err != nil {
		return service.ValidationError(map[string]string{"materialId": "must be a valid UUID"})
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE report_materials
		SET deleted_at = $2, enabled = false, updated_at = $2
		WHERE id = $1 AND deleted_at IS NULL`, materialID, deletedAt)
	if err != nil {
		return fmt.Errorf("delete report material: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return service.NewError(service.CodeNotFound, "report material not found", pgx.ErrNoRows)
	}
	return nil
}

func (r *PostgresRepository) CreateReport(ctx context.Context, value service.Report) (service.Report, error) {
	if value.CreatedAt.IsZero() {
		value.CreatedAt = time.Now().UTC()
	}
	if value.UpdatedAt.IsZero() {
		value.UpdatedAt = value.CreatedAt
	}
	templateID, err := parseOptionalUUIDField(value.TemplateID, "templateId")
	if err != nil {
		return service.Report{}, err
	}
	latestJobID, err := parseOptionalUUIDField(value.LatestJobID, "latestJobId")
	if err != nil {
		return service.Report{}, err
	}
	latestReportFileID, err := parseOptionalUUIDField(value.LatestReportFileID, "latestReportFileId")
	if err != nil {
		return service.Report{}, err
	}
	row := r.db.QueryRow(ctx, `
		INSERT INTO reports (
			id, report_name, report_type, template_id, topic, specialty,
			plant_or_business_object, report_year, status, creator_id, creator_name,
			source, latest_job_id, latest_report_file_id, generated_at, exported_at,
			created_at, updated_at
		)
		VALUES (
			$1, $2, $3, NULLIF($4, '')::uuid, $5, NULLIF($6, ''),
			NULLIF($7, ''), NULLIF($8, 0), $9, NULLIF($10, ''), NULLIF($11, ''),
			$12, NULLIF($13, '')::uuid, NULLIF($14, '')::uuid, $15, $16, $17, $18
		)
		RETURNING
			id::text, report_name, report_type, COALESCE(template_id::text, ''), topic,
			COALESCE(specialty, ''), COALESCE(plant_or_business_object, ''),
			COALESCE(report_year, 0), status, COALESCE(creator_id, ''),
			COALESCE(creator_name, ''), source, COALESCE(latest_job_id::text, ''),
			COALESCE(latest_report_file_id::text, ''), generated_at, exported_at,
			created_at, updated_at, deleted_at`,
		value.ID,
		value.Name,
		value.ReportType,
		templateID,
		value.Topic,
		value.Specialty,
		value.BusinessObject,
		value.Year,
		string(value.Status),
		value.CreatorID,
		value.CreatorName,
		value.Source,
		latestJobID,
		latestReportFileID,
		value.GeneratedAt,
		value.ExportedAt,
		value.CreatedAt,
		value.UpdatedAt,
	)
	report, err := scanReport(row)
	if err != nil {
		if isUniqueViolation(err) {
			return service.Report{}, service.NewError(service.CodeConflict, "report already exists", err)
		}
		return service.Report{}, fmt.Errorf("insert report: %w", err)
	}
	return report, nil
}

func (r *PostgresRepository) CreateReportJob(ctx context.Context, value service.ReportJob) (service.ReportJob, error) {
	if value.CreatedAt.IsZero() {
		value.CreatedAt = time.Now().UTC()
	}
	if value.MaxAttempts == 0 {
		value.MaxAttempts = 3
	}
	templateID, err := parseOptionalUUIDField(value.TemplateID, "templateId")
	if err != nil {
		return service.ReportJob{}, err
	}
	requestPayload, err := json.Marshal(jsonObjectOrEmpty(value.RequestPayload))
	if err != nil {
		return service.ReportJob{}, fmt.Errorf("encode report job request payload: %w", err)
	}
	row := r.db.QueryRow(ctx, `
		INSERT INTO report_jobs (
			id, request_id, source, job_type, target_type, target_id, asynq_task_id,
			queue_name, report_id, template_id, request_payload_json, status, error_code, error_message,
			retry_count, max_attempts, started_at, finished_at, created_at
		)
		VALUES (
			$1, NULLIF($2, ''), $3, $4, $5, $6, NULLIF($7, ''),
			$8, $9, NULLIF($10, '')::uuid, $11::jsonb, $12, NULLIF($13, ''), NULLIF($14, ''),
			$15, $16, $17, $18, $19
		)
		RETURNING
			id::text, COALESCE(request_id, ''), source, job_type, target_type,
			target_id, COALESCE(asynq_task_id, ''), queue_name, report_id::text,
			COALESCE(template_id::text, ''), request_payload_json, status, progress_json,
			COALESCE(error_code, ''), COALESCE(error_message, ''), retry_count,
			max_attempts, started_at, finished_at, created_at`,
		value.ID,
		value.RequestID,
		value.Source,
		string(value.JobType),
		value.TargetType,
		value.TargetID,
		value.AsynqTaskID,
		value.QueueName,
		value.ReportID,
		templateID,
		requestPayload,
		string(value.Status),
		value.ErrorCode,
		value.ErrorMessage,
		value.RetryCount,
		value.MaxAttempts,
		value.StartedAt,
		value.FinishedAt,
		value.CreatedAt,
	)
	job, err := scanReportJob(row)
	if err != nil {
		if isUniqueViolation(err) {
			return service.ReportJob{}, service.NewError(service.CodeConflict, "report job already exists", err)
		}
		return service.ReportJob{}, fmt.Errorf("insert report job: %w", err)
	}
	return job, nil
}

func (r *PostgresRepository) FindReportJobByID(ctx context.Context, id string) (service.ReportJob, error) {
	jobID, err := parseUUID(id)
	if err != nil {
		return service.ReportJob{}, service.NewError(service.CodeValidation, "invalid report job id", err)
	}
	row := r.db.QueryRow(ctx, `
		SELECT
			id::text, COALESCE(request_id, ''), source, job_type, target_type,
			target_id, COALESCE(asynq_task_id, ''), queue_name, report_id::text,
			COALESCE(template_id::text, ''), request_payload_json, status, progress_json,
			COALESCE(error_code, ''), COALESCE(error_message, ''), retry_count,
			max_attempts, started_at, finished_at, created_at
		FROM report_jobs
		WHERE id = $1`, jobID)
	job, err := scanReportJob(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.ReportJob{}, service.NewError(service.CodeNotFound, "report job not found", err)
		}
		return service.ReportJob{}, fmt.Errorf("find report job: %w", err)
	}
	return job, nil
}

func (r *PostgresRepository) CreateReportJobAttempt(ctx context.Context, value service.ReportJobAttempt) (service.ReportJobAttempt, error) {
	if value.CreatedAt.IsZero() {
		value.CreatedAt = time.Now().UTC()
	}
	row := r.db.QueryRow(ctx, `
		INSERT INTO report_job_attempts (
			id, job_id, attempt_number, asynq_task_id, trigger_source, reason,
			status, error_code, error_message, started_at, finished_at, created_at
		)
		VALUES (
			$1, $2, $3, NULLIF($4, ''), $5, NULLIF($6, ''),
			$7, NULLIF($8, ''), NULLIF($9, ''), $10, $11, $12
		)
		RETURNING
			id::text, job_id::text, attempt_number, COALESCE(asynq_task_id, ''),
			trigger_source, COALESCE(reason, ''), status, COALESCE(error_code, ''),
			COALESCE(error_message, ''), started_at, finished_at, created_at`,
		value.ID,
		value.JobID,
		value.AttemptNumber,
		value.AsynqTaskID,
		value.TriggerSource,
		value.Reason,
		string(value.Status),
		value.ErrorCode,
		value.ErrorMessage,
		value.StartedAt,
		value.FinishedAt,
		value.CreatedAt,
	)
	attempt, err := scanReportJobAttempt(row)
	if err != nil {
		if isUniqueViolation(err) {
			return service.ReportJobAttempt{}, service.NewError(service.CodeConflict, "report job attempt already exists", err)
		}
		return service.ReportJobAttempt{}, fmt.Errorf("insert report job attempt: %w", err)
	}
	return attempt, nil
}

func (r *PostgresRepository) CreateReportEvent(ctx context.Context, value service.ReportEvent) (service.ReportEvent, error) {
	if value.CreatedAt.IsZero() {
		value.CreatedAt = time.Now().UTC()
	}
	jobID, err := parseOptionalUUIDField(value.JobID, "jobId")
	if err != nil {
		return service.ReportEvent{}, err
	}
	row := r.db.QueryRow(ctx, `
		INSERT INTO report_events (
			id, report_id, job_id, event_type, message, created_at
		)
		VALUES ($1, $2, NULLIF($3, '')::uuid, $4, NULLIF($5, ''), $6)
		RETURNING id::text, report_id::text, COALESCE(job_id::text, ''), event_type, COALESCE(message, ''), created_at`,
		value.ID,
		value.ReportID,
		jobID,
		value.EventType,
		value.Message,
		value.CreatedAt,
	)
	event, err := scanReportEvent(row)
	if err != nil {
		if isUniqueViolation(err) {
			return service.ReportEvent{}, service.NewError(service.CodeConflict, "report event already exists", err)
		}
		return service.ReportEvent{}, fmt.Errorf("insert report event: %w", err)
	}
	return event, nil
}

func (r *PostgresRepository) ListReportJobsByReportID(ctx context.Context, reportID string) ([]service.ReportJob, error) {
	id, err := parseUUID(reportID)
	if err != nil {
		return nil, service.NewError(service.CodeValidation, "invalid report id", err)
	}
	rows, err := r.db.Query(ctx, `
		SELECT
			id::text, COALESCE(request_id, ''), source, job_type, target_type,
			target_id, COALESCE(asynq_task_id, ''), queue_name, report_id::text,
			COALESCE(template_id::text, ''), request_payload_json, status, progress_json,
			COALESCE(error_code, ''), COALESCE(error_message, ''), retry_count,
			max_attempts, started_at, finished_at, created_at
		FROM report_jobs
		WHERE report_id = $1
		ORDER BY created_at DESC`, id)
	if err != nil {
		return nil, fmt.Errorf("list report jobs: %w", err)
	}
	defer rows.Close()
	var jobs []service.ReportJob
	for rows.Next() {
		job, err := scanReportJob(rows)
		if err != nil {
			return nil, fmt.Errorf("scan report job: %w", err)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list report jobs rows: %w", err)
	}
	return jobs, nil
}

func (r *PostgresRepository) UpdateReportJobStatus(ctx context.Context, id string, status service.JobStatus, errorCode, errorMessage string, startedAt, finishedAt *time.Time) (service.ReportJob, error) {
	return r.updateJobStatusAndReport(ctx, id, status, errorCode, errorMessage, startedAt, finishedAt, reportStatusUpdatedAt(startedAt, finishedAt))
}

func updateReportJobStatusRow(ctx context.Context, db sqlc.DBTX, id string, status service.JobStatus, errorCode, errorMessage string, startedAt, finishedAt *time.Time) (service.ReportJob, error) {
	row := db.QueryRow(ctx, `
		UPDATE report_jobs SET
			status = $2,
			progress_json = CASE
				WHEN $2 = 'running' THEN jsonb_build_object('completed', 0, 'total', 1)
				WHEN $2 IN ('succeeded', 'partial_succeeded') THEN
					CASE
						WHEN progress_json ? 'completed'
							AND progress_json ? 'total'
							AND (
								COALESCE((progress_json ->> 'completed')::int, 0) > 0
								OR COALESCE((progress_json ->> 'total')::int, 0) > 1
							) THEN progress_json
						ELSE jsonb_build_object('completed', 1, 'total', 1)
					END
				WHEN $2 IN ('failed', 'canceled') THEN
					CASE
						WHEN progress_json ? 'completed' AND progress_json ? 'total' THEN progress_json
						ELSE jsonb_build_object('completed', 0, 'total', 1)
					END
				ELSE progress_json
			END,
			error_code = NULLIF($3, ''),
			error_message = NULLIF($4, ''),
			started_at = CASE WHEN $5::timestamptz IS NOT NULL THEN $5::timestamptz ELSE started_at END,
			finished_at = $6
		WHERE id = $1::uuid
		RETURNING
			id::text, COALESCE(request_id, ''), source, job_type, target_type,
			target_id, COALESCE(asynq_task_id, ''), queue_name, report_id::text,
			COALESCE(template_id::text, ''), request_payload_json, status, progress_json,
			COALESCE(error_code, ''), COALESCE(error_message, ''), retry_count,
			max_attempts, started_at, finished_at, created_at`,
		id,
		string(status),
		errorCode,
		errorMessage,
		startedAt,
		finishedAt,
	)
	job, err := scanReportJob(row)
	if err != nil {
		return service.ReportJob{}, fmt.Errorf("update report job status: %w", err)
	}
	return job, nil
}

func (r *PostgresRepository) UpdateReportGenerationStatus(ctx context.Context, reportID, jobID string, jobType service.JobType, status service.JobStatus, updatedAt time.Time) error {
	job := service.ReportJob{
		ID:       jobID,
		ReportID: reportID,
		JobType:  jobType,
	}
	return updateReportGenerationStatusForJob(ctx, r.db, job, status, updatedAt)
}

func (r *PostgresRepository) UpdateReportJobProgress(ctx context.Context, id string, completed, total int) error {
	if completed < 0 {
		completed = 0
	}
	if total < completed {
		total = completed
	}
	jobID, err := parseUUID(id)
	if err != nil {
		return service.NewError(service.CodeValidation, "invalid job id", err)
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE report_jobs
		SET progress_json = jsonb_build_object('completed', $2::int, 'total', $3::int)
		WHERE id = $1`, jobID, completed, total)
	if err != nil {
		return fmt.Errorf("update report job progress: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return service.NewError(service.CodeNotFound, "report job not found", nil)
	}
	return nil
}

func (r *PostgresRepository) UpdateJobAsynqTaskID(ctx context.Context, id, taskID string) error {
	jobID, err := parseUUID(id)
	if err != nil {
		return service.NewError(service.CodeValidation, "invalid job id", err)
	}
	_, err = r.db.Exec(ctx, `UPDATE report_jobs SET asynq_task_id = $2 WHERE id = $1`, jobID, taskID)
	if err != nil {
		return fmt.Errorf("update job asynq task id: %w", err)
	}
	return nil
}

func (r *PostgresRepository) SetJobRunning(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := r.updateJobStatusAndReport(ctx, id, service.JobStatusRunning, "", "", &now, nil, now)
	if err == nil {
		_ = r.recordJobEvent(ctx, id, "job.running", "report job started")
	}
	return err
}

func (r *PostgresRepository) SetJobSucceeded(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := r.updateJobStatusAndReport(ctx, id, service.JobStatusSucceeded, "", "", nil, &now, now)
	if err == nil {
		_ = r.recordJobEvent(ctx, id, "job.succeeded", "report job succeeded")
	}
	return err
}

func (r *PostgresRepository) SetJobPartialSucceeded(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := r.updateJobStatusAndReport(ctx, id, service.JobStatusPartialSucceeded, "", "", nil, &now, now)
	if err == nil {
		_ = r.recordJobEvent(ctx, id, "job.partial_succeeded", "report job partially succeeded")
	}
	return err
}

func (r *PostgresRepository) SetJobFailed(ctx context.Context, id, errCode, errMsg string) error {
	now := time.Now().UTC()
	_, err := r.updateJobStatusAndReport(ctx, id, service.JobStatusFailed, errCode, errMsg, nil, &now, now)
	if err == nil {
		_ = r.recordJobEvent(ctx, id, "job.failed", "report job failed")
	}
	return err
}

func (r *PostgresRepository) updateJobStatusAndReport(ctx context.Context, id string, status service.JobStatus, errorCode, errorMessage string, startedAt, finishedAt *time.Time, updatedAt time.Time) (service.ReportJob, error) {
	if tx, ok := r.db.(pgx.Tx); ok {
		return updateJobStatusAndReportWithDB(ctx, tx, id, status, errorCode, errorMessage, startedAt, finishedAt, updatedAt)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return service.ReportJob{}, fmt.Errorf("begin report job status transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	job, err := updateJobStatusAndReportWithDB(ctx, tx, id, status, errorCode, errorMessage, startedAt, finishedAt, updatedAt)
	if err != nil {
		return service.ReportJob{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return service.ReportJob{}, fmt.Errorf("commit report job status transaction: %w", err)
	}
	return job, nil
}

func updateJobStatusAndReportWithDB(ctx context.Context, db sqlc.DBTX, id string, status service.JobStatus, errorCode, errorMessage string, startedAt, finishedAt *time.Time, updatedAt time.Time) (service.ReportJob, error) {
	job, err := updateReportJobStatusRow(ctx, db, id, status, errorCode, errorMessage, startedAt, finishedAt)
	if err != nil {
		return service.ReportJob{}, err
	}
	if err := updateReportGenerationStatusForJob(ctx, db, job, status, updatedAt); err != nil {
		return service.ReportJob{}, err
	}
	return job, nil
}

func reportStatusUpdatedAt(startedAt, finishedAt *time.Time) time.Time {
	if finishedAt != nil {
		return finishedAt.UTC()
	}
	if startedAt != nil {
		return startedAt.UTC()
	}
	return time.Now().UTC()
}

func updateReportGenerationStatusForJob(ctx context.Context, db sqlc.DBTX, job service.ReportJob, jobStatus service.JobStatus, updatedAt time.Time) error {
	reportStatus, ok := reportStatusForGenerationJob(job.JobType, jobStatus)
	if !ok {
		return nil
	}
	reportID, err := parseUUID(job.ReportID)
	if err != nil {
		return service.NewError(service.CodeValidation, "invalid report id", err)
	}
	jobID, err := parseUUID(job.ID)
	if err != nil {
		return service.NewError(service.CodeValidation, "invalid job id", err)
	}
	setGeneratedAt := shouldSetReportGeneratedAt(job.JobType, jobStatus)
	tag, err := db.Exec(ctx, `
		UPDATE reports
		SET
			latest_job_id = $2,
			status = $3,
			generated_at = CASE WHEN $4::boolean THEN $5::timestamptz ELSE generated_at END,
			updated_at = $5
		WHERE id = $1
		  AND deleted_at IS NULL
		  AND status <> 'deleted'`,
		reportID,
		jobID,
		string(reportStatus),
		setGeneratedAt,
		updatedAt,
	)
	if err != nil {
		return fmt.Errorf("update report generation status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return service.NewError(service.CodeConflict, "report has been deleted", nil)
	}
	return nil
}

func reportStatusForGenerationJob(jobType service.JobType, jobStatus service.JobStatus) (service.ReportStatus, bool) {
	isOutline := false
	switch jobType {
	case service.JobTypeOutlineGeneration, service.JobTypeOutlineRegeneration:
		isOutline = true
	case service.JobTypeContentGeneration, service.JobTypeContentRegeneration, service.JobTypeSectionRegeneration:
	default:
		return "", false
	}
	switch jobStatus {
	case service.JobStatusPending, service.JobStatusRunning:
		if isOutline {
			return service.ReportStatusOutlineGenerating, true
		}
		return service.ReportStatusContentGenerating, true
	case service.JobStatusSucceeded:
		if isOutline {
			return service.ReportStatusOutlineGenerated, true
		}
		return service.ReportStatusGenerated, true
	case service.JobStatusPartialSucceeded:
		if isOutline {
			return service.ReportStatusOutlineGenerated, true
		}
		return service.ReportStatusGenerated, true
	case service.JobStatusFailed, service.JobStatusCanceled:
		return service.ReportStatusFailed, true
	default:
		return "", false
	}
}

func shouldSetReportGeneratedAt(jobType service.JobType, jobStatus service.JobStatus) bool {
	if jobStatus != service.JobStatusSucceeded && jobStatus != service.JobStatusPartialSucceeded {
		return false
	}
	switch jobType {
	case service.JobTypeContentGeneration, service.JobTypeContentRegeneration, service.JobTypeSectionRegeneration:
		return true
	default:
		return false
	}
}

func (r *PostgresRepository) recordJobEvent(ctx context.Context, jobID, eventType, message string) error {
	id, err := parseUUID(jobID)
	if err != nil {
		return service.NewError(service.CodeValidation, "invalid job id", err)
	}
	eventID := newUUIDString()
	tag, err := r.db.Exec(ctx, `
		INSERT INTO report_events (id, report_id, job_id, event_type, message, created_at)
		SELECT $1, report_id, id, $2, $3, $4
		FROM report_jobs
		WHERE id = $5`,
		eventID, eventType, message, time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("record report job event: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return service.NewError(service.CodeNotFound, "report job not found", nil)
	}
	return nil
}

func (r *PostgresRepository) IncrementJobRetryCount(ctx context.Context, id string) (service.ReportJob, error) {
	jobID, err := parseUUID(id)
	if err != nil {
		return service.ReportJob{}, service.NewError(service.CodeValidation, "invalid job id", err)
	}
	row := r.db.QueryRow(ctx, `
		UPDATE report_jobs SET
			retry_count = retry_count + 1,
			status = 'pending',
			progress_json = '{}'::jsonb,
			error_code = NULL,
			error_message = NULL
		WHERE id = $1
		RETURNING
			id::text, COALESCE(request_id, ''), source, job_type, target_type,
			target_id, COALESCE(asynq_task_id, ''), queue_name, report_id::text,
			COALESCE(template_id::text, ''), request_payload_json, status, progress_json,
			COALESCE(error_code, ''), COALESCE(error_message, ''), retry_count,
			max_attempts, started_at, finished_at, created_at`, jobID)
	job, err := scanReportJob(row)
	if err != nil {
		return service.ReportJob{}, fmt.Errorf("increment job retry count: %w", err)
	}
	return job, nil
}

func (r *PostgresRepository) ClaimRetry(ctx context.Context, jobID, attemptID, triggerSource, reason string) (service.ReportJobAttempt, error) {
	id, err := parseUUID(jobID)
	if err != nil {
		return service.ReportJobAttempt{}, service.NewError(service.CodeValidation, "invalid job id", err)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return service.ReportJobAttempt{}, fmt.Errorf("begin retry transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status, reportID, jobType string
	var retryCount, maxAttempts int
	if err := tx.QueryRow(ctx,
		`SELECT status, retry_count, max_attempts, report_id::text, job_type FROM report_jobs WHERE id = $1 FOR UPDATE`, id,
	).Scan(&status, &retryCount, &maxAttempts, &reportID, &jobType); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.ReportJobAttempt{}, service.NewError(service.CodeNotFound, "report job not found", err)
		}
		return service.ReportJobAttempt{}, fmt.Errorf("lock report job: %w", err)
	}
	s := service.JobStatus(status)
	if s != service.JobStatusFailed && s != service.JobStatusCanceled {
		return service.ReportJobAttempt{}, service.NewError(service.CodeValidation, "only failed or canceled jobs can be retried", nil)
	}
	if retryCount >= maxAttempts {
		return service.ReportJobAttempt{}, service.NewError(service.CodeValidation, "max retry attempts reached", nil)
	}
	var attemptNumber int
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(MAX(attempt_number), 0) + 1 FROM report_job_attempts WHERE job_id = $1`, id,
	).Scan(&attemptNumber); err != nil {
		return service.ReportJobAttempt{}, fmt.Errorf("load next retry attempt number: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE report_jobs SET retry_count = retry_count + 1, status = 'pending', progress_json = '{}'::jsonb, error_code = NULL, error_message = NULL WHERE id = $1`, id,
	); err != nil {
		return service.ReportJobAttempt{}, fmt.Errorf("increment retry count: %w", err)
	}

	now := time.Now().UTC()
	if err := updateReportGenerationStatusForJob(ctx, tx, service.ReportJob{
		ID:       jobID,
		ReportID: reportID,
		JobType:  service.JobType(jobType),
	}, service.JobStatusPending, now); err != nil {
		return service.ReportJobAttempt{}, err
	}
	row := tx.QueryRow(ctx, `
		INSERT INTO report_job_attempts (
			id, job_id, attempt_number, trigger_source, reason, status, created_at
		)
		VALUES ($1, $2, $3, $4, $5, 'pending', $6)
		RETURNING
			id::text, job_id::text, attempt_number, COALESCE(asynq_task_id, ''),
			trigger_source, COALESCE(reason, ''), status, COALESCE(error_code, ''),
			COALESCE(error_message, ''), started_at, finished_at, created_at`,
		attemptID, id, attemptNumber, triggerSource, reason, now,
	)
	attempt, err := scanReportJobAttempt(row)
	if err != nil {
		if isUniqueViolation(err) {
			return service.ReportJobAttempt{}, service.NewError(service.CodeConflict, "retry attempt already exists", err)
		}
		return service.ReportJobAttempt{}, fmt.Errorf("insert retry attempt: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return service.ReportJobAttempt{}, fmt.Errorf("commit retry transaction: %w", err)
	}
	return attempt, nil
}

func (r *PostgresRepository) UpdateAttemptAsynqTaskID(ctx context.Context, attemptID, taskID string) error {
	id, err := parseUUID(attemptID)
	if err != nil {
		return service.NewError(service.CodeValidation, "invalid attempt id", err)
	}
	_, err = r.db.Exec(ctx, `UPDATE report_job_attempts SET asynq_task_id = $2 WHERE id = $1`, id, taskID)
	if err != nil {
		return fmt.Errorf("update attempt asynq task id: %w", err)
	}
	return nil
}

func (r *PostgresRepository) SetAttemptRunning(ctx context.Context, attemptID string) error {
	id, err := parseUUID(attemptID)
	if err != nil {
		return service.NewError(service.CodeValidation, "invalid attempt id", err)
	}
	_, err = r.db.Exec(ctx,
		`UPDATE report_job_attempts SET status = 'running', started_at = $2 WHERE id = $1`,
		id, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("set attempt running: %w", err)
	}
	return nil
}

func (r *PostgresRepository) SetAttemptSucceeded(ctx context.Context, attemptID string) error {
	id, err := parseUUID(attemptID)
	if err != nil {
		return service.NewError(service.CodeValidation, "invalid attempt id", err)
	}
	_, err = r.db.Exec(ctx,
		`UPDATE report_job_attempts SET status = 'succeeded', finished_at = $2 WHERE id = $1`,
		id, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("set attempt succeeded: %w", err)
	}
	return nil
}

func (r *PostgresRepository) SetAttemptPartialSucceeded(ctx context.Context, attemptID string) error {
	id, err := parseUUID(attemptID)
	if err != nil {
		return service.NewError(service.CodeValidation, "invalid attempt id", err)
	}
	_, err = r.db.Exec(ctx,
		`UPDATE report_job_attempts SET status = 'partial_succeeded', finished_at = $2 WHERE id = $1`,
		id, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("set attempt partial succeeded: %w", err)
	}
	return nil
}

func (r *PostgresRepository) SetAttemptFailed(ctx context.Context, attemptID, errCode, errMsg string) error {
	id, err := parseUUID(attemptID)
	if err != nil {
		return service.NewError(service.CodeValidation, "invalid attempt id", err)
	}
	_, err = r.db.Exec(ctx,
		`UPDATE report_job_attempts SET status = 'failed', error_code = NULLIF($2,''), error_message = NULLIF($3,''), finished_at = $4 WHERE id = $1`,
		id, errCode, errMsg, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("set attempt failed: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ListReportJobAttemptsByJobID(ctx context.Context, jobID string) ([]service.ReportJobAttempt, error) {
	id, err := parseUUID(jobID)
	if err != nil {
		return nil, service.NewError(service.CodeValidation, "invalid job id", err)
	}
	rows, err := r.db.Query(ctx, `
		SELECT
			id::text, job_id::text, attempt_number, COALESCE(asynq_task_id, ''),
			trigger_source, COALESCE(reason, ''), status, COALESCE(error_code, ''),
			COALESCE(error_message, ''), started_at, finished_at, created_at
		FROM report_job_attempts
		WHERE job_id = $1
		ORDER BY attempt_number ASC`, id)
	if err != nil {
		return nil, fmt.Errorf("list report job attempts: %w", err)
	}
	defer rows.Close()
	var attempts []service.ReportJobAttempt
	for rows.Next() {
		attempt, err := scanReportJobAttempt(rows)
		if err != nil {
			return nil, fmt.Errorf("scan report job attempt: %w", err)
		}
		attempts = append(attempts, attempt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list report job attempts rows: %w", err)
	}
	return attempts, nil
}

func (r *PostgresRepository) ListReportEventsByReportID(ctx context.Context, reportID string) ([]service.ReportEvent, error) {
	id, err := parseUUID(reportID)
	if err != nil {
		return nil, service.NewError(service.CodeValidation, "invalid report id", err)
	}
	rows, err := r.db.Query(ctx, `
		SELECT
			id::text, report_id::text, COALESCE(job_id::text, ''), event_type,
			COALESCE(message, ''), created_at
		FROM report_events
		WHERE report_id = $1
		ORDER BY created_at DESC`, id)
	if err != nil {
		return nil, fmt.Errorf("list report events: %w", err)
	}
	defer rows.Close()
	var events []service.ReportEvent
	for rows.Next() {
		event, err := scanReportEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan report event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list report events rows: %w", err)
	}
	return events, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanReportType(row scanner) (service.ReportType, error) {
	var value service.ReportType
	if err := row.Scan(
		&value.Code,
		&value.Name,
		&value.Description,
		&value.Enabled,
		&value.DefaultTemplateID,
		&value.CreatedAt,
		&value.UpdatedAt,
	); err != nil {
		return service.ReportType{}, err
	}
	return value, nil
}

func scanReport(row scanner) (service.Report, error) {
	var value service.Report
	var status string
	if err := row.Scan(
		&value.ID,
		&value.Name,
		&value.ReportType,
		&value.TemplateID,
		&value.Topic,
		&value.Specialty,
		&value.BusinessObject,
		&value.Year,
		&status,
		&value.CreatorID,
		&value.CreatorName,
		&value.Source,
		&value.LatestJobID,
		&value.LatestReportFileID,
		&value.GeneratedAt,
		&value.ExportedAt,
		&value.CreatedAt,
		&value.UpdatedAt,
		&value.DeletedAt,
	); err != nil {
		return service.Report{}, err
	}
	value.Status = service.ReportStatus(status)
	return value, nil
}

func scanReportTemplate(row scanner) (service.ReportTemplate, error) {
	var value service.ReportTemplate
	if err := row.Scan(
		&value.ID,
		&value.TemplateName,
		&value.ReportType,
		&value.Version,
		&value.FileRef,
		&value.Filename,
		&value.FileSize,
		&value.Description,
		&value.Enabled,
		&value.CreatedBy,
		&value.CreatedAt,
		&value.UpdatedAt,
		&value.DeletedAt,
	); err != nil {
		return service.ReportTemplate{}, err
	}
	return value, nil
}

func scanReportMaterial(row scanner) (service.ReportMaterial, error) {
	var value service.ReportMaterial
	var tagsRaw []byte
	if err := row.Scan(
		&value.ID,
		&value.MaterialName,
		&value.MaterialType,
		&value.Category,
		&value.FileRef,
		&value.Filename,
		&value.FileSize,
		&value.Description,
		&tagsRaw,
		&value.Enabled,
		&value.CreatedBy,
		&value.CreatedAt,
		&value.UpdatedAt,
		&value.DeletedAt,
	); err != nil {
		return service.ReportMaterial{}, err
	}
	if len(tagsRaw) > 0 {
		if err := json.Unmarshal(tagsRaw, &value.Tags); err != nil {
			return service.ReportMaterial{}, err
		}
	}
	if value.Tags == nil {
		value.Tags = []string{}
	}
	return value, nil
}

func scanReportJob(row scanner) (service.ReportJob, error) {
	var value service.ReportJob
	var jobType, status string
	var requestPayloadRaw, progressRaw []byte
	if err := row.Scan(
		&value.ID,
		&value.RequestID,
		&value.Source,
		&jobType,
		&value.TargetType,
		&value.TargetID,
		&value.AsynqTaskID,
		&value.QueueName,
		&value.ReportID,
		&value.TemplateID,
		&requestPayloadRaw,
		&status,
		&progressRaw,
		&value.ErrorCode,
		&value.ErrorMessage,
		&value.RetryCount,
		&value.MaxAttempts,
		&value.StartedAt,
		&value.FinishedAt,
		&value.CreatedAt,
	); err != nil {
		return service.ReportJob{}, err
	}
	value.JobType = service.JobType(jobType)
	value.Status = service.JobStatus(status)
	if len(requestPayloadRaw) > 0 {
		if err := json.Unmarshal(requestPayloadRaw, &value.RequestPayload); err != nil {
			return service.ReportJob{}, err
		}
	}
	if value.RequestPayload == nil {
		value.RequestPayload = map[string]any{}
	}
	if len(progressRaw) > 0 {
		if err := json.Unmarshal(progressRaw, &value.Progress); err != nil {
			return service.ReportJob{}, err
		}
	}
	if value.Progress == nil {
		value.Progress = map[string]any{}
	}
	return value, nil
}

func jsonObjectOrEmpty(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func jsonRawStringOrDefault(value json.RawMessage, fallback string) string {
	if len(value) == 0 || strings.TrimSpace(string(value)) == "" {
		return fallback
	}
	return string(value)
}

func scanReportJobAttempt(row scanner) (service.ReportJobAttempt, error) {
	var value service.ReportJobAttempt
	var status string
	if err := row.Scan(
		&value.ID,
		&value.JobID,
		&value.AttemptNumber,
		&value.AsynqTaskID,
		&value.TriggerSource,
		&value.Reason,
		&status,
		&value.ErrorCode,
		&value.ErrorMessage,
		&value.StartedAt,
		&value.FinishedAt,
		&value.CreatedAt,
	); err != nil {
		return service.ReportJobAttempt{}, err
	}
	value.Status = service.JobStatus(status)
	return value, nil
}

func scanReportEvent(row scanner) (service.ReportEvent, error) {
	var value service.ReportEvent
	if err := row.Scan(
		&value.ID,
		&value.ReportID,
		&value.JobID,
		&value.EventType,
		&value.Message,
		&value.CreatedAt,
	); err != nil {
		return service.ReportEvent{}, err
	}
	return value, nil
}

func reportJobFromSQLC(row sqlc.GetReportJobByIDRow) service.ReportJob {
	return service.ReportJob{
		ID:             uuidToString(row.ID),
		RequestID:      textToString(row.RequestID),
		Source:         row.Source,
		JobType:        service.JobType(row.JobType),
		TargetType:     row.TargetType,
		TargetID:       row.TargetID,
		AsynqTaskID:    textToString(row.AsynqTaskID),
		QueueName:      row.QueueName,
		ReportID:       uuidToString(row.ReportID),
		TemplateID:     uuidToString(row.TemplateID),
		RequestPayload: map[string]any{},
		Status:         service.JobStatus(row.Status),
		Progress:       map[string]any{},
		ErrorCode:      textToString(row.ErrorCode),
		ErrorMessage:   textToString(row.ErrorMessage),
		RetryCount:     int(row.RetryCount),
		MaxAttempts:    int(row.MaxAttempts),
		StartedAt:      timestamptzToTimePtr(row.StartedAt),
		FinishedAt:     timestamptzToTimePtr(row.FinishedAt),
		CreatedAt:      timestamptzToTime(row.CreatedAt),
	}
}

func newUUIDString() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("00000000-0000-4000-8000-%012x", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func parseUUID(value string) (pgtype.UUID, error) {
	var uuid pgtype.UUID
	if err := uuid.Scan(value); err != nil {
		return pgtype.UUID{}, err
	}
	return uuid, nil
}

func parseOptionalUUIDField(value, field string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	if _, err := parseUUID(trimmed); err != nil {
		return "", service.ValidationError(map[string]string{field: "must be a valid UUID"})
	}
	return trimmed, nil
}

func uuidToString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	b := value.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func textToString(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func timestamptzToTime(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

func timestamptzToTimePtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func isUniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "duplicate key value") || strings.Contains(err.Error(), "SQLSTATE 23505")
}
