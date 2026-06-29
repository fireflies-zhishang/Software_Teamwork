# File Service

`services/file` is the first runnable Go module for base file-object upload, metadata, deletion, and original content lookup. It is an internal foundation service for owner services to call; it does not own knowledge ingestion, knowledge document state, chunks, indexing, QA, report templates, report materials, report files, or report workflows.

Public frontend routes remain owned by gateway and are documented in `docs/services/gateway/api/openapi.yaml`. Frontend callers must not call this service directly. Stable file capability must be reached through gateway `/api/v1/**` resources owned by `knowledge` or `document`, while those owner services reuse this service's internal base file APIs.

The implemented internal contract is generic file-object shaped (`/internal/v1/files/**`). The knowledge-document routes remain available only for compatibility and should not be extended for report templates, report materials, generated report files, or new knowledge business metadata.

## Current Scope

Implemented now:

- `GET /healthz`
- `GET /readyz`
- `POST /internal/v1/files`
- `GET /internal/v1/files/{fileId}`
- `DELETE /internal/v1/files/{fileId}`
- `GET /internal/v1/files/{fileId}/content`
- `POST /internal/v1/knowledge-bases/{knowledgeBaseId}/documents`
- `GET /internal/v1/documents/{documentId}`
- `PATCH /internal/v1/documents/{documentId}`
- `DELETE /internal/v1/documents/{documentId}`
- `GET /internal/v1/documents/{documentId}/content`


Out of scope for this MVP:

- Local MinIO setup
- Production MinIO adapter
- Production PostgreSQL repository adapter; `sqlc.yaml`, first query file, and a `goose` migration are present as the contract scaffold
- Knowledge ingestion handoff and knowledge document state
- Report template, report material, and generated report file business state
- Public knowledge-owned document list/detail/chunks/content contracts

## Local Run

```powershell
go test ./...
go build ./cmd/server
$env:FILE_HTTP_ADDR=':8082'; go run ./cmd/server
```

Business endpoints require gateway context headers for local testing:

```text
X-Request-Id: req_local
X-User-Id: usr_local
X-User-Roles: admin
X-User-Permissions: document:upload,document:read,document:update,document:delete
```

The service enforces the permission header for business routes. Missing user
context returns `401 unauthorized`; missing operation permission returns
`403 forbidden`.

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `FILE_HTTP_ADDR` | `:8082` | HTTP listen address. |
| `FILE_MAX_UPLOAD_BYTES` | `33554432` | Multipart upload limit in bytes. |
| `FILE_STORAGE_BACKEND` | `memory` | Supported values: `memory`, `local`. |
| `FILE_LOCAL_STORAGE_DIR` | `.file-storage` | Local object-store root when `FILE_STORAGE_BACKEND=local`. |
| `FILE_SHUTDOWN_TIMEOUT` | `10s` | Graceful shutdown timeout. |

## Storage Port

Object storage is behind `service.ObjectStore`. The current `memory` adapter exists only for tests and early local integration. The `local` adapter stores objects under `FILE_LOCAL_STORAGE_DIR` for local durable smoke tests. Neither adapter exposes object keys or storage paths through API responses.

A future MinIO adapter should be added under `internal/platform/storage/minio` and wired through `internal/config` without changing `internal/http` handlers or service use cases.

## Metadata Port

File metadata is behind the service repository port. The current memory repository supports handler tests and local smoke testing. A future PostgreSQL implementation should live under `internal/repository` and add real migrations under `migrations/`. It must store only base file metadata such as file id, display filename, content type, size, checksum, storage reference, created timestamp, and deleted timestamp. Knowledge-base IDs, report IDs, template IDs, material IDs, business tags, processing status, and ACLs belong to their owner services.

## Multipart Upload Shape

Upload uses `multipart/form-data`:

- `file`: required binary part
- `checksumSha256`: optional SHA-256 checksum for `/internal/v1/files`; when omitted, the service computes it
- `tags`: optional repeated fields for compatibility document uploads, for example `tags=policy` and `tags=inspection`

## Response Shape

JSON success responses use:

```json
{
  "data": {},
  "requestId": "req_123"
}
```

JSON errors use:

```json
{
  "error": {
    "code": "validation_error",
    "message": "request validation failed",
    "requestId": "req_123"
  }
}
```

Internal metadata responses include base file fields such as `contentType`, `sizeBytes`, and checksum for owner-service integration. They never expose bucket names, object keys, internal storage URLs, or storage credentials.

Content reads return raw bytes on success and the same JSON error envelope on failure.
