-- name: CreateFileObject :one
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
VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
    $11, $12, $13, $14, $15
)
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
    last_error_message;

-- name: GetFileObject :one
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
  AND status = 'available';

-- name: MarkFileDeleteRequested :one
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
    last_error_message;

-- name: MarkFilePurged :one
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
    last_error_message;

-- name: MarkFilePurgeFailed :one
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
    last_error_message;