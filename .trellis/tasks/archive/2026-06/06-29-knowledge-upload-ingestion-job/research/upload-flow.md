# Knowledge Upload Flow Research

## Sources Inspected

- GitHub issue #82: `[A-10] Knowledge 文档上传、file 集成与入库任务创建`
- `docs/services/knowledge/README.md`
- `docs/services/knowledge/docs/api-contract.md`
- `docs/services/knowledge/docs/data-models.md`
- `docs/services/knowledge/docs/implementation.md`
- `docs/services/file/README.md`
- `docs/services/file/docs/implementation.md`
- `docs/architecture/frontend-backend-contract.md`
- `docs/architecture/service-boundaries.md`
- `docs/requirements-analysis/decision-sync-checklist.md`
- `services/knowledge/api/openapi.yaml`
- `docs/services/gateway/api/openapi.yaml`
- `services/file/api/openapi.yaml`
- `services/knowledge/internal/http/server.go`
- `services/knowledge/internal/service/types.go`
- `services/knowledge/migrations/0001_create_knowledge_core_tables.sql`
- `services/knowledge/internal/repository/queries/knowledge.sql`
- `services/file/internal/http/server.go`
- `services/file/internal/service/service.go`

## Findings

- The browser-facing upload route is `POST /api/v1/knowledge-bases/{knowledgeBaseId}/documents`, owned by `knowledge` through gateway. Service-local implementation should expose the matching internal knowledge route under `/internal/v1/knowledge-bases/{knowledgeBaseId}/documents`.
- `file` already exposes the intended base file-object contract at `POST /internal/v1/files`. Its legacy knowledge-document-shaped compatibility routes must not be extended for this task.
- `knowledge_documents.file_ref` is the internal file reference. It must be persisted in Knowledge but omitted from `DocumentSummary` responses.
- `processing_jobs` already exists in the initial Knowledge migration and is the durable source of truth for ingestion job state. Redis/asynq may carry queue payloads, but job status must remain in PostgreSQL.
- The existing Knowledge service has list/get document support but no upload handler, no document creation repository method, no processing job creation method, no file-service client, and no queue client.
- The current service-local `services/knowledge/api/openapi.yaml` lists document list/get routes but does not yet declare the internal upload route, so it should be updated alongside code.

## Recommended Flow

Use the docs-recommended order:

1. Validate gateway user context and `knowledge:write` permission.
2. Parse `multipart/form-data`, requiring a non-empty `file` part and optional `tags`.
3. Verify the target knowledge base exists and is visible/writable to the caller.
4. Call `file` service `POST /internal/v1/files` with the raw multipart file.
5. In one Knowledge repository transaction, create `knowledge_documents`, create a queued `processing_jobs` record, and set `knowledge_documents.current_job_id`.
6. Enqueue an asynq ingestion task whose payload contains only traceable identifiers such as `requestId`, `jobId`, `documentId`, `knowledgeBaseId`, and `userId`.
7. Return `201 Created` with `DocumentSummary`; `status` should be `uploaded` unless enqueue succeeds and the implementation intentionally moves to `parsing` immediately.

## Compensation And Error Mapping

- File write failure: do not create a Knowledge document or job; map upstream validation errors to `validation_error` where appropriate, and dependency failures to `dependency_error`.
- Knowledge document/job creation failure after file creation: attempt best-effort `DELETE /internal/v1/files/{fileId}` and return `dependency_error` or `conflict` according to the repository error. Never include internal file ID in the public response.
- Queue enqueue failure after durable job creation: keep PostgreSQL as truth and mark the job/document failure state or return dependency failure only if the implementation can keep the response consistent. The PRD uses the stricter MVP acceptance: do not report success unless the queue handoff is created or the job is durably marked for retry/recovery.
- Logs and errors must not include object keys, internal URLs, raw file content, document full text, SQL details, MinIO error details, prompts, tokens, or stack traces.
