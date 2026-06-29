package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/file/internal/service"
)

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, doc service.Document) (service.Document, error) {
	return service.Document{}, service.ErrConflict
}

func (r *PostgresRepository) FindByID(ctx context.Context, id string) (service.Document, error) {
	return service.Document{}, service.ErrNotFound
}

func (r *PostgresRepository) ReplaceTags(ctx context.Context, id string, tags []string) (service.Document, error) {
	return service.Document{}, service.ErrNotFound
}

func (r *PostgresRepository) MarkDeleted(ctx context.Context, id string, deletedAt time.Time) (service.Document, error) {
	return service.Document{}, service.ErrNotFound
}

func (r *PostgresRepository) CreateFile(ctx context.Context, file service.FileObject) (service.FileObject, error) {
	const query = `
		INSERT INTO file_objects (
			id,
			filename,
			content_type,
			size_bytes,
			checksum_sha256,
			storage_backend,
			storage_bucket,
			storage_object_key,
			storage_version_id,
			storage_etag,
			status,
			created_by_service,
			request_id,
			created_at,
			updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING
			id,
			filename,
			content_type,
			size_bytes,
			checksum_sha256,
			storage_backend,
			storage_bucket,
			storage_object_key,
			storage_version_id,
			storage_etag,
			status,
			created_by_service,
			request_id,
			created_at,
			updated_at,
			deleted_at,
			delete_requested_at,
			purged_at,
			last_error_code,
			last_error_message
	`
	created, err := scanFileObject(r.db.QueryRowContext(ctx, query,
		file.ID,
		file.Filename,
		file.ContentType,
		file.SizeBytes,
		nullableString(file.ChecksumSHA256),
		file.StorageBackend,
		nullableString(file.StorageBucket),
		file.StorageObjectKey,
		nullableString(file.StorageVersionID),
		nullableString(file.StorageETag),
		string(file.Status),
		nullableString(file.CreatedByService),
		nullableString(file.RequestID),
		file.CreatedAt,
		file.UpdatedAt,
	))
	if err != nil {
		if isUniqueViolation(err) {
			return service.FileObject{}, service.ErrConflict
		}
		return service.FileObject{}, fmt.Errorf("insert file object: %w", err)
	}
	return created, nil
}

func (r *PostgresRepository) FindFileByID(ctx context.Context, id string) (service.FileObject, error) {
	const query = `
		SELECT
			id,
			filename,
			content_type,
			size_bytes,
			checksum_sha256,
			storage_backend,
			storage_bucket,
			storage_object_key,
			storage_version_id,
			storage_etag,
			status,
			created_by_service,
			request_id,
			created_at,
			updated_at,
			deleted_at,
			delete_requested_at,
			purged_at,
			last_error_code,
			last_error_message
		FROM file_objects
		WHERE id = $1
		  AND deleted_at IS NULL
		  AND status = 'available'
	`
	file, err := scanFileObject(r.db.QueryRowContext(ctx, query, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return service.FileObject{}, service.ErrNotFound
		}
		return service.FileObject{}, fmt.Errorf("find file object: %w", err)
	}
	return file, nil
}

func (r *PostgresRepository) MarkFileDeleteRequested(ctx context.Context, id string, deletedAt time.Time) (service.FileObject, error) {
	const query = `
		UPDATE file_objects
		SET status = 'delete_requested',
			deleted_at = $2,
			delete_requested_at = $2,
			updated_at = $2
		WHERE id = $1
		  AND deleted_at IS NULL
		RETURNING
			id,
			filename,
			content_type,
			size_bytes,
			checksum_sha256,
			storage_backend,
			storage_bucket,
			storage_object_key,
			storage_version_id,
			storage_etag,
			status,
			created_by_service,
			request_id,
			created_at,
			updated_at,
			deleted_at,
			delete_requested_at,
			purged_at,
			last_error_code,
			last_error_message
	`
	file, err := scanFileObject(r.db.QueryRowContext(ctx, query, id, deletedAt.UTC()))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return service.FileObject{}, service.ErrNotFound
		}
		return service.FileObject{}, fmt.Errorf("mark file delete requested: %w", err)
	}
	return file, nil
}

func (r *PostgresRepository) MarkFilePurged(ctx context.Context, id string, purgedAt time.Time) (service.FileObject, error) {
	const query = `
		UPDATE file_objects
		SET status = 'purged',
			purged_at = $2,
			last_error_code = NULL,
			last_error_message = NULL,
			updated_at = $2
		WHERE id = $1
		RETURNING
			id,
			filename,
			content_type,
			size_bytes,
			checksum_sha256,
			storage_backend,
			storage_bucket,
			storage_object_key,
			storage_version_id,
			storage_etag,
			status,
			created_by_service,
			request_id,
			created_at,
			updated_at,
			deleted_at,
			delete_requested_at,
			purged_at,
			last_error_code,
			last_error_message
	`
	file, err := scanFileObject(r.db.QueryRowContext(ctx, query, id, purgedAt.UTC()))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return service.FileObject{}, service.ErrNotFound
		}
		return service.FileObject{}, fmt.Errorf("mark file purged: %w", err)
	}
	return file, nil
}

func (r *PostgresRepository) MarkFilePurgeFailed(ctx context.Context, id string, code string, message string, failedAt time.Time) (service.FileObject, error) {
	const query = `
		UPDATE file_objects
		SET status = 'failed',
			last_error_code = $2,
			last_error_message = $3,
			updated_at = $4
		WHERE id = $1
		RETURNING
			id,
			filename,
			content_type,
			size_bytes,
			checksum_sha256,
			storage_backend,
			storage_bucket,
			storage_object_key,
			storage_version_id,
			storage_etag,
			status,
			created_by_service,
			request_id,
			created_at,
			updated_at,
			deleted_at,
			delete_requested_at,
			purged_at,
			last_error_code,
			last_error_message
	`
	file, err := scanFileObject(r.db.QueryRowContext(ctx, query, id, nullableString(code), nullableString(message), failedAt.UTC()))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return service.FileObject{}, service.ErrNotFound
		}
		return service.FileObject{}, fmt.Errorf("mark file purge failed: %w", err)
	}
	return file, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanFileObject(row scanner) (service.FileObject, error) {
	var file service.FileObject
	var checksum sql.NullString
	var bucket sql.NullString
	var versionID sql.NullString
	var etag sql.NullString
	var status string
	var createdByService sql.NullString
	var requestID sql.NullString
	var deletedAt sql.NullTime
	var deleteRequestedAt sql.NullTime
	var purgedAt sql.NullTime
	var lastErrorCode sql.NullString
	var lastErrorMessage sql.NullString
	if err := row.Scan(
		&file.ID,
		&file.Filename,
		&file.ContentType,
		&file.SizeBytes,
		&checksum,
		&file.StorageBackend,
		&bucket,
		&file.StorageObjectKey,
		&versionID,
		&etag,
		&status,
		&createdByService,
		&requestID,
		&file.CreatedAt,
		&file.UpdatedAt,
		&deletedAt,
		&deleteRequestedAt,
		&purgedAt,
		&lastErrorCode,
		&lastErrorMessage,
	); err != nil {
		return service.FileObject{}, err
	}
	file.ChecksumSHA256 = checksum.String
	file.StorageBucket = bucket.String
	file.StorageVersionID = versionID.String
	file.StorageETag = etag.String
	file.Status = service.FileStatus(status)
	file.CreatedByService = createdByService.String
	file.RequestID = requestID.String
	if deletedAt.Valid {
		value := deletedAt.Time
		file.DeletedAt = &value
	}
	if deleteRequestedAt.Valid {
		value := deleteRequestedAt.Time
		file.DeleteRequestedAt = &value
	}
	if purgedAt.Valid {
		value := purgedAt.Time
		file.PurgedAt = &value
	}
	file.LastErrorCode = lastErrorCode.String
	file.LastErrorMessage = lastErrorMessage.String
	return file, nil
}

func nullableString(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func isUniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "duplicate key value") || strings.Contains(err.Error(), "SQLSTATE 23505")
}
