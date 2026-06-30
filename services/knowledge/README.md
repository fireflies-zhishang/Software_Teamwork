# Knowledge Service

Knowledge owns knowledge-base metadata, knowledge document metadata/status,
processing trace state, and future chunk/vector lifecycle coordination.

This implementation includes the A-09 foundation slice, the A-10 document
upload handoff, the A-11 ingestion worker path, A-12 knowledge-query
retrieval, and the A-14 active-operation contract surface. Knowledge accepts
the document upload, stores raw bytes through File Service, creates durable
document/job state, enqueues ingestion work, then consumes the A10 task payload
to read source bytes, parse, chunk, embed, index chunks, expose chunk/content
reads, and run retrieval over hydrated chunks.

## Runtime

- Go module: `go 1.25.0`
- HTTP: standard `net/http` `ServeMux`
- Logging: `log/slog`
- PostgreSQL access: `pgx` + generated `sqlc` query package
- Migrations: `goose`

All landed Go services use the repository Go 1.25 baseline. Knowledge keeps the
standard `net/http` / `http.ServeMux` service shape while leaving room for later
RAG MCP server work.

## Configuration

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `DATABASE_URL` | yes | - | PostgreSQL connection string. |
| `FILE_SERVICE_BASE_URL` | yes | - | Internal File Service base URL for `/internal/v1/files`. |
| `KNOWLEDGE_REDIS_ADDR` | yes | - | Redis/asynq endpoint for ingestion task handoff. |
| `KNOWLEDGE_HTTP_ADDR` | no | `:8083` | HTTP listen address. |
| `KNOWLEDGE_SERVICE_VERSION` | no | `dev` | Version returned by readiness checks. |
| `KNOWLEDGE_ENV` | no | `local` | Runtime environment label. |
| `KNOWLEDGE_MAX_UPLOAD_BYTES` | no | `33554432` | Multipart upload limit in bytes. |
| `KNOWLEDGE_SERVICE_TOKEN` | yes | - | Internal service token forwarded to File Service. |
| `KNOWLEDGE_SHUTDOWN_TIMEOUT` | no | `10s` | Graceful shutdown timeout. |
| `PARSER_SERVICE_BASE_URL` | yes | - | Internal Parser service base URL for document parsing. |
| `PARSER_SERVICE_TOKEN` | no | - | Optional Parser service token. |
| `PARSER_SERVICE_TIMEOUT` | no | `30s` | Parser request timeout. |
| `EMBEDDING_PROVIDER` | no | `local_hashing` | Embedding provider; `ai_gateway` uses AI Gateway. |
| `EMBEDDING_MODEL` | no | `local_hashing` | Embedding model/profile label. |
| `EMBEDDING_DIMENSION` | no | `384` | Embedding vector dimension. |
| `AI_GATEWAY_BASE_URL` | no | - | AI Gateway base URL when `EMBEDDING_PROVIDER=ai_gateway`. |
| `AI_GATEWAY_SERVICE_TOKEN` | no | - | Optional AI Gateway service token. |
| `AI_GATEWAY_EMBEDDING_PROFILE_ID` | no | - | Optional AI Gateway embedding profile ID. |
| `QDRANT_URL` | no | - | Optional Qdrant REST base URL; unset uses in-memory index. |
| `QDRANT_BASE_URL` | no | - | Alias for `QDRANT_URL`. |
| `QDRANT_API_KEY` | no | - | Optional Qdrant API key. |
| `QDRANT_COLLECTION` | no | `knowledge_chunks` | Qdrant collection name. |
| `RERANK_MODEL` | rerank | - | Optional AI Gateway rerank model. When unset, rerank requests use the local no-op fallback. |
| `RERANK_PROFILE_ID` | no | - | Optional AI Gateway rerank profile id. |

When `EMBEDDING_PROVIDER=ai_gateway`, `EMBEDDING_MODEL` must match the resolved AI Gateway embedding profile `model`. If `AI_GATEWAY_EMBEDDING_PROFILE_ID` or `EMBEDDING_PROFILE_ID` is unset, AI Gateway uses its default enabled embedding profile and still validates the `model` value before calling the provider. `RERANK_MODEL` is optional; when unset, query rerank keeps the vector order as a local no-op fallback.

## Implemented Routes

Operational routes:

- `GET /healthz`
- `GET /readyz`

Internal service routes:

- `GET /internal/v1/knowledge-bases`
- `POST /internal/v1/knowledge-bases`
- `GET /internal/v1/knowledge-bases/{knowledgeBaseId}`
- `PATCH /internal/v1/knowledge-bases/{knowledgeBaseId}`
- `DELETE /internal/v1/knowledge-bases/{knowledgeBaseId}`
- `GET /internal/v1/knowledge-bases/{knowledgeBaseId}/documents`
- `POST /internal/v1/knowledge-bases/{knowledgeBaseId}/documents`
- `GET /internal/v1/documents/{documentId}`
- `GET /internal/v1/documents/{documentId}/chunks`
- `GET /internal/v1/documents/{documentId}/content`
- `POST /internal/v1/knowledge-queries`

Public gateway equivalents are documented in
`docs/services/gateway/api/openapi.yaml`.

## Access Context

Business routes require gateway-injected `X-User-Id`.

Supported permission strings follow the current auth docs:

- `knowledge:read`
- `knowledge:write`

Rules:

- Callers can read resources they created.
- `knowledge:read`, `knowledge:write`, `admin`, or `super_admin` can read
  broader resources.
- Create, update, and delete require `knowledge:write`, `admin`, or
  `super_admin`.
- Hidden or deleted resources return `404 not_found`.
- Authenticated callers without mutation rights receive `403 forbidden`.

## Data Model

The first migration creates:

- `knowledge_bases`
- `knowledge_documents`
- `processing_jobs`
- `document_chunks`

Document upload stores the File Service object ID only in
`knowledge_documents.file_ref`. Public document responses expose `jobId` and
document status, but never `fileRef`, File Service internal IDs, object keys, or
internal URLs.

`document_chunks` is now written by the ingestion worker. Qdrant payloads are
limited to `knowledge_base_id`, `document_id`, `chunk_id`, `chunk_index`,
`chunk_type`, `section_path`, `tags`, `metadata`, `job_id`,
`job_attempt`, and ingestion attempt markers. Knowledge query retrieval uses the
configured vector index and PostgreSQL chunk hydration; tests can still inject
fake embedder/vector adapters without real AI Gateway or Qdrant.

Knowledge base deletion is soft-delete-first:

- mark `knowledge_bases.deleted_at`;
- mark owned `knowledge_documents.deleted_at` in the same transaction for the
  PostgreSQL runtime repository;
- leave chunk/index cleanup for a future lifecycle job instead of hard-deleting
  chunks or vectors in this metadata route.

## Local Integration Notes

The default local service path uses PostgreSQL, File Service, Parser Service,
Redis/asynq, local hashing embeddings, and an in-memory vector index. This is
enough for upload handoff, worker processing, chunk listing, original content
reads, and seeded/fake-backed contract tests.

Real Qdrant and AI Gateway integration is optional:

- Leave `QDRANT_URL` empty to use the in-memory vector index.
- Set `QDRANT_URL=http://qdrant:6333` and `QDRANT_COLLECTION=knowledge_chunks`
  only after the collection exists and Qdrant is healthy.
- Leave `EMBEDDING_PROVIDER=local_hashing` for deterministic local runs.
- Set `EMBEDDING_PROVIDER=ai_gateway`, `AI_GATEWAY_BASE_URL`, and
  `AI_GATEWAY_SERVICE_TOKEN` only when the optional AI Gateway profile is
  running with a real provider credential.
- Set `RERANK_MODEL` only when AI Gateway rerank should be called. Otherwise
  `rerank=true` requests keep vector order as a local no-op fallback.

`GET /internal/v1/documents/{documentId}/content` validates the Knowledge-owned
document first, then reads the raw bytes from File Service internally. It never
exposes `file_ref`, object keys, File Service IDs, or storage URLs in JSON
responses.

## Migrations

Apply the service-owned migration with the project-pinned `goose@v3.27.1` command:

```bash
go run github.com/pressly/goose/v3/cmd/goose@v3.27.1 -dir migrations postgres "$DATABASE_URL" up
```
## Development

```bash
go test ./...
go build ./cmd/server
```

Contract tests under `internal/http` use seeded repositories and fake file,
vector, and embedding adapters, matching the decoupling rule in
`docs/services/knowledge/docs/api-contract.md` section 2.6. Full upload ->
worker -> Parser -> Qdrant -> query smoke still requires the local integration
stack and real dependency configuration.

Regenerate the query package from `sqlc.yaml` after changing SQL files:

```bash
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0 generate
```
