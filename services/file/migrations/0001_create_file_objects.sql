CREATE TABLE IF NOT EXISTS file_objects (
    id TEXT PRIMARY KEY,
    filename TEXT NOT NULL,
    content_type TEXT NOT NULL DEFAULT 'application/octet-stream',
    size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
    checksum_sha256 TEXT CHECK (checksum_sha256 IS NULL OR checksum_sha256 ~ '^[0-9a-f]{64}$'),
    storage_backend TEXT NOT NULL,
    storage_bucket TEXT,
    storage_object_key TEXT NOT NULL,
    storage_version_id TEXT,
    storage_etag TEXT,
    status TEXT NOT NULL CHECK (status IN ('available', 'delete_requested', 'purging', 'purged', 'failed')),
    created_by_service TEXT,
    request_id TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ,
    delete_requested_at TIMESTAMPTZ,
    purged_at TIMESTAMPTZ,
    last_error_code TEXT,
    last_error_message TEXT
);

CREATE INDEX IF NOT EXISTS idx_file_objects_created_at
    ON file_objects (created_at DESC);

CREATE INDEX IF NOT EXISTS idx_file_objects_status
    ON file_objects (status);

CREATE INDEX IF NOT EXISTS idx_file_objects_checksum_sha256
    ON file_objects (checksum_sha256)
    WHERE checksum_sha256 IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_file_objects_deleted_at
    ON file_objects (deleted_at);

CREATE INDEX IF NOT EXISTS idx_file_objects_created_by_service
    ON file_objects (created_by_service);