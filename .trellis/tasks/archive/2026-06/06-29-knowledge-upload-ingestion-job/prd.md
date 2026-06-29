# Knowledge document upload, file integration, and ingestion job creation

## Goal

Implement issue #82 (`[A-10] Knowledge 文档上传、file 集成与入库任务创建`) as a backend-owned Knowledge upload slice. The Knowledge service must accept the gateway-forwarded multipart document upload, store raw bytes through File Service's base `/internal/v1/files` contract, persist Knowledge-owned document and processing job state in PostgreSQL, enqueue an ingestion task, and return a public `DocumentSummary` without leaking internal file details.

## What I Already Know

- Issue #82 is assigned to `L1nggTeam`, labels `backend` and `service:knowledge`, priority `P0`, batch `Batch 1`.
- The recommended branch is `L1nggTeam/feat/knowledge-upload-ingestion-job`; the current branch already matches.
- Dependencies are #79 (`[A-07] File 服务收敛到 /internal/v1/files 基础对象契约`) and #81 (`[A-09] Knowledge 知识库与文档状态基础 API`).
- Docs are authoritative when code and older private planning conflict.
- Browser-facing callers use gateway `/api/v1/**`; the frontend must not call `knowledge` or `file` directly.
- `knowledge` owns knowledge-base document resources, document status, chunks, vectors, retrieval, and processing jobs.
- `file` owns only base file-object metadata, object storage coordination, deletion, and original content reads.
- New integrations must call `file` service `/internal/v1/files/**`, not the legacy file-service knowledge-document compatibility routes.
- Existing Knowledge code supports knowledge-base CRUD plus document list/detail, but there is no upload handler yet.
- Existing File code already implements `POST /internal/v1/files`, metadata read, delete, and content read.

## Requirements

- Add a Knowledge-owned multipart upload endpoint for `POST /internal/v1/knowledge-bases/{knowledgeBaseId}/documents`, corresponding to gateway public `POST /api/v1/knowledge-bases/{knowledgeBaseId}/documents`.
- Require authenticated gateway context and Knowledge write permission consistently with existing mutation routes.
- Accept multipart field `file` as required and `tags` as optional repeated fields or JSON string, following `docs/services/knowledge/docs/api-contract.md`.
- Validate upload failures as field-level or form-level `validation_error`, including missing file, invalid multipart, empty file, invalid tags, invalid `knowledgeBaseId`, and unsupported query/body shape.
- Call File Service `POST /internal/v1/files` to persist raw file bytes. The request must send only base file-object fields (`file`, optional checksum if supported later), not `knowledgeBaseId`, tags, document status, ACLs, or business metadata.
- Persist the returned file ID only as internal `knowledge_documents.file_ref`. Do not expose it in `DocumentSummary`, logs, or public errors.
- Create a Knowledge document record with status `uploaded` initially, content type, size, sanitized/display filename, tags, creator, timestamps, and `file_ref`.
- Create a processing job record in PostgreSQL for the uploaded document. The job should be `queued`, reference the knowledge base and document, use a clear ingestion job type, default `max_attempts` to 3, and be linked from `knowledge_documents.current_job_id`.
- Enqueue an asynq ingestion task after durable job creation. The queue payload must contain traceable IDs only: `requestId`, `jobId`, `documentId`, `knowledgeBaseId`, and `userId`.
- Return `201 Created` with the standard envelope and `DocumentSummary`; visible status should be `uploaded` or `parsing`, with `jobId` present.
- Keep PostgreSQL as the source of truth for document and job state. Redis/asynq must not be the authoritative state store.
- Update service-local OpenAPI if the implemented internal route is missing from `services/knowledge/api/openapi.yaml`.
- Add focused tests for HTTP handler behavior, service orchestration, repository persistence, and failure compensation.

## Acceptance Criteria

- [ ] `POST /internal/v1/knowledge-bases/{knowledgeBaseId}/documents` accepts multipart upload and returns `201` with `data.id`, `data.knowledgeBaseId`, `data.name`, `data.status`, `data.jobId`, `data.createdAt`, and `requestId`.
- [ ] The response never includes `fileRef`, `fileId`, object key, MinIO URL, internal file URL, or raw vector/payload fields.
- [ ] Upload validation failures return `400` with `error.code = "validation_error"` and useful `error.fields`.
- [ ] File Service write failure prevents Knowledge document/job creation and maps to `validation_error` for upstream validation failures or `dependency_error` for upstream dependency/internal failures.
- [ ] If Knowledge document/job creation fails after file creation, the implementation attempts best-effort File Service deletion compensation and returns an appropriate error without leaking the file ID publicly.
- [ ] The created Knowledge document stores `file_ref` internally and can be retrieved/listed through existing document APIs without exposing the internal file reference.
- [ ] A processing job row is created in PostgreSQL and linked through `knowledge_documents.current_job_id`.
- [ ] Queue payload includes `requestId`, `jobId`, `documentId`, `knowledgeBaseId`, and `userId`, and excludes file object keys, raw content, prompts, internal URLs, or tokens.
- [ ] Logs and error responses do not contain object keys, internal URLs, document full text, raw multipart content, SQL details, MinIO details, prompts, tokens, or stack traces.
- [ ] Relevant `go test ./...` checks for `services/knowledge` pass.

## Definition of Done

- Tests added or updated for success, validation errors, file-service failure, job/repository failure compensation, and no public file ID leakage.
- Service-local OpenAPI and docs are updated only where implementation changes the local contract.
- Lint/type/build checks expected by this repo pass or skipped checks are reported with reasons.
- PR targets upstream `develop`, uses Conventional Commits, and references issue #82.

## Technical Approach

Implement this as a Knowledge vertical slice:

1. Add ports in `services/knowledge/internal/service` for a base file client and ingestion queue client.
2. Add a service method such as `UploadDocument(ctx, reqCtx, input)` that validates context/input, calls file service, writes Knowledge document and processing job state, attempts compensation on partial failure, and enqueues the ingestion task.
3. Extend the repository interface with a transactional method that creates the document, creates the job, and binds `current_job_id` atomically.
4. Implement the transactional method in memory and PostgreSQL repositories. For PostgreSQL, add sqlc queries for document/job creation and `current_job_id` binding.
5. Add an HTTP multipart handler under `services/knowledge/internal/http/server.go`, mirroring the response/error conventions already used by list/get document handlers.
6. Add a lightweight File Service HTTP client under Knowledge-owned code; it should parse the standard envelope and normalize upstream errors into Knowledge service errors.
7. Add an asynq queue adapter when Redis configuration is present. Keep an injectable no-op or test queue implementation for local unit tests only if production startup still requires a real queue client before upload can report success.
8. Update `services/knowledge/api/openapi.yaml` to include the internal upload route if still absent.

## Decision (ADR-lite)

**Context**: The docs allow two broad upload orders: create Knowledge state before file write, or write File first and then create Knowledge document/job with compensation. Issue #82 explicitly requires File Service integration, file write failure handling, job creation failure compensation, and that File not store Knowledge business metadata.

**Decision**: Use File-first orchestration for this MVP: validate upload, write the raw file to `/internal/v1/files`, then create Knowledge document and processing job in one PostgreSQL transaction, then enqueue an ingestion task. If Knowledge persistence fails after file creation, attempt best-effort `DELETE /internal/v1/files/{fileId}` compensation. PostgreSQL remains the durable state authority.

**Consequences**: This avoids orphan Knowledge documents when raw file persistence fails and keeps File Service generic. It introduces a possible orphan file if compensation deletion fails; that risk is acceptable for the MVP if the failure is logged with safe identifiers and can later be handled by cleanup jobs.

## Out of Scope

- Full parser, chunking, embedding, Qdrant indexing, retrieval implementation, or worker execution logic beyond enqueueing the ingestion task.
- Gateway proxy implementation or frontend upload UI.
- Public processing-job APIs beyond the `jobId` surfaced in `DocumentSummary`.
- Switching File Service legacy compatibility document routes.
- Advanced idempotency, virus scanning, quota enforcement, OCR, parser runtime configuration, or manual retry UI.
- Returning original file content through `GET /api/v1/documents/{documentId}/content`; this task only creates the upload/resource/job path.

## Technical Notes

- Research artifact: [`research/upload-flow.md`](research/upload-flow.md).
- Primary docs:
  - `docs/services/knowledge/README.md`
  - `docs/services/knowledge/docs/api-contract.md`
  - `docs/services/knowledge/docs/data-models.md`
  - `docs/services/knowledge/docs/implementation.md`
  - `docs/services/file/README.md`
  - `docs/services/file/docs/implementation.md`
  - `docs/architecture/frontend-backend-contract.md`
  - `docs/architecture/service-boundaries.md`
  - `docs/requirements-analysis/decision-sync-checklist.md`
- Primary code paths:
  - `services/knowledge/internal/http/server.go`
  - `services/knowledge/internal/service/types.go`
  - `services/knowledge/internal/service/service.go`
  - `services/knowledge/internal/repository/memory.go`
  - `services/knowledge/internal/repository/postgres.go`
  - `services/knowledge/internal/repository/queries/knowledge.sql`
  - `services/knowledge/migrations/0001_create_knowledge_core_tables.sql`
  - `services/file/api/openapi.yaml`
  - `services/file/internal/http/server.go`
  - `services/file/internal/service/service.go`
- Existing Knowledge permissions use `knowledge:read` and `knowledge:write`; public docs mention upload/delete by role-level RBAC, so this task should use `knowledge:write` unless a repo-local auth spec mandates a more specific permission.
- The current service-local Knowledge OpenAPI only has document list/get routes; upload route likely needs to be added.
- The current Knowledge config only has HTTP/database/shutdown fields; file-service URL and Redis/asynq queue settings likely need new typed config.
