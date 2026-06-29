# Fix Issues 79 And 80

## Goal

Resolve GitHub issues #79 and #80 for `services/file` by aligning the implementation with the documented internal base file-object contract, storage behavior, content streaming, and deletion cleanup model.

## Requirements

- Implement the base internal file resource routes:
  - `POST /internal/v1/files`
  - `GET /internal/v1/files/{fileId}`
  - `DELETE /internal/v1/files/{fileId}`
  - `GET /internal/v1/files/{fileId}/content`
- Keep `file` scoped to base file metadata only; do not store or return knowledge/report/template/material business fields.
- Preserve existing knowledge-document compatibility routes only as compatibility surface, not as the target contract.
- Store and return only safe metadata: `id`, `filename`, `contentType`, `sizeBytes`, `checksumSha256`, `createdAt`, `deletedAt`.
- Do not return bucket, object key, internal storage URL, MinIO credentials, or storage internals.
- Enforce upload size limits at the HTTP/multipart path.
- Reject malformed multipart, missing file, empty file, oversized file, and invalid or mismatched checksum.
- Compute SHA-256 when caller does not provide it.
- Stream file content through the content endpoint with safe `Content-Type`, `Content-Length`, `Content-Disposition`, and `X-Request-Id` headers.
- Escape filenames used in response headers.
- Implement delete semantics so deleted files can no longer be read through metadata or content endpoints.
- Implement or preserve object cleanup behavior with retryable/failure-summary state where feasible in current service architecture.
- Map storage dependency failures to `dependency_error`.

## Acceptance Criteria

- [x] `go test ./...` passes inside `services/file`.
- [x] `httptest` coverage includes multipart errors, missing file, empty file, oversized file, checksum validation, content stream, and reading after delete.
- [x] Public/internal base responses do not include bucket, object key, MinIO URL, or credentials.
- [x] Content response uses safe `Content-Disposition` formatting.
- [x] Deleted files return `not_found` for metadata and content reads.
- [x] Logs/tests do not expose object key, internal URL, access key, or file contents.

## Definition Of Done

- Tests added or updated for touched behavior.
- File service checks pass.
- Docs/OpenAPI remain consistent with implementation, or are updated if implementation changes contract.
- Work is reviewed against `docs/services/file/**` and `docs/architecture/technology-decisions.md`.

## Technical Approach

- Use the existing `services/file` Go module and its current layering.
- Prefer extending current service, repository, and storage ports instead of introducing a new framework.
- Keep production-facing dependencies aligned with project docs: standard `net/http`/`ServeMux`, `slog`, MinIO adapter boundary, and no ORM.
- Add database/sqlc/goose scaffolding only if missing and low-risk for this task; prioritize working internal contract and tests in the current service architecture.

## Out Of Scope

- Gateway/public frontend `/api/v1/**` contract changes.
- Knowledge/document owner service business state or permission rules.
- Range requests, presigned URLs, resumable upload, deduplication, antivirus scanning, and tenant quota enforcement.
- Full asynchronous `asynq` worker if the current service has no queue infrastructure yet; keep cleanup state compatible with future async purge.

## Technical Notes

- Issue #79: https://github.com/Sakayori-Iroha-168/Software_Teamwork/issues/79
- Issue #80: https://github.com/Sakayori-Iroha-168/Software_Teamwork/issues/80
- Authority:
  - `docs/services/file/README.md`
  - `docs/services/file/docs/data-models.md`
  - `docs/services/file/docs/implementation.md`
  - `docs/architecture/technology-decisions.md`
  - `services/file/api/openapi.yaml`
- Branch at task start: `EIRTeam/feat/file-service-module`.
- Working tree at task start: clean, with local branch already rebased to `upstream/develop`.

## Implementation Notes

- Added `/internal/v1/files/**` handlers, DTOs, service methods, and tests while keeping legacy knowledge-document compatibility routes.
- Added SHA-256 checksum validation/computation, safe content streaming headers, upload size/multipart validation, and delete-then-hide semantics.
- Added local filesystem object-store adapter for non-MinIO local durability behind the existing `ObjectStore` port.
- Added `sqlc.yaml`, `internal/repository/queries/file_objects.sql`, `migrations/0001_create_file_objects.sql`, and a PostgreSQL repository adapter scaffold for `file_objects`.
- Verification run from `services/file` with repository-local Go cache:
  - `go test ./...`
  - `go build ./cmd/server`
- Repository-level verification:
  - `git diff --check`