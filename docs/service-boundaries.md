# 服务边界矩阵

本文档用于约束 `gateway`、`auth`、`file`、`knowledge`、`qa`、`document` 的职责归属，避免早期并行开发时把业务规则堆进 gateway。

## 总览

| Service | Owns | Exposes to gateway | Must not own |
| --- | --- | --- | --- |
| `gateway` | Public API, routing, Redis-backed session cache, auth context propagation, response/error envelope, request id, lightweight aggregation. | `/api/v1/**`, `/healthz`, `/readyz`. | Durable user/role/permission persistence, document parsing, vector search, LLM workflows, report generation business logic. |
| `auth` | Users, credentials, roles, permissions, sessions or tokens, session identity issuing and revocation. | Login, logout, current user, permission checks, session identity for gateway caching. | File metadata, knowledge indexing, QA messages, report records. |
| `file` | Uploads, original files, object storage coordination, file metadata lifecycle. | Upload, download, file metadata, file deletion. | Knowledge chunking, vector index, RAG, report generation. |
| `knowledge` | Knowledge bases, document ingestion state, chunks, embeddings, retrieval policies, search. | Missing/TBD: frontend-backend contract not finalized. | User identity, raw object storage, LLM answer generation, DOCX export. |
| `qa` | Chat sessions, messages, intent routing for QA, RAG answer generation, citations. | Missing/TBD: frontend-backend contract not finalized. | Knowledge base CRUD, file upload, report record management. |
| `document` | Report templates, report records, outlines, section content, DOCX export. | Missing/TBD: frontend-backend contract not finalized. | QA chat, knowledge indexing, auth persistence. |

## Workflow Ownership

| Workflow | Gateway role | Owner service | Notes |
| --- | --- | --- | --- |
| Register / login | Public entrypoint, response normalization, Redis session cache write. | `auth` | Password validation and session/token issuing stay in auth; auth returns identity/session payload for gateway caching. |
| Current user | Read Redis session cache and normalize response. | `auth` | Auth owns user/session source data; gateway owns runtime cache lookup and downstream context injection. |
| Knowledge base CRUD | Missing public contract. | `knowledge` | Placeholder only. Do not expose as stable frontend API until the contract is finalized. |
| Upload document to knowledge base | Public file upload entrypoint. | `file`; knowledge handoff TBD. | File service owns raw upload; knowledge ingestion/indexing handoff is intentionally missing until finalized. Gateway must not implement parsing or indexing. |
| Document processing retry | Missing public contract. | `knowledge` | Placeholder only. Retry semantics must be defined by the future knowledge contract. |
| Download original document | Route and enforce auth context. | `file` | File service owns object lookup and download authorization details. |
| Frontend knowledge search | Missing public contract. | `knowledge` | Placeholder only. Search filters, ranking, and result shape are not stable. |
| Chat answer generation | Missing public contract. | `qa` | Placeholder only. Streaming/non-streaming message and citation formats are not stable. |
| Citation source lookup | Missing public contract. | `qa` or `knowledge`, depending on final citation model. | Placeholder only. The service storing citation references will own lookup. |
| Report outline generation | Missing public contract. | `document` | Placeholder only. |
| Report section generation | Missing public contract. | `document` | Placeholder only. |
| Report DOCX export | Missing public contract. | `document` | Placeholder only. Generated files may later use file service behind document service. |
| Admin overview | Missing public contract. | `gateway` aggregates; each service owns its metric. | Placeholder only. Metrics and aggregation shape are not stable. |

## Missing Contract Register

The following downstream frontend/backend interfaces are intentionally blank in
`docs/api/gateway.openapi.yaml` until the teams finalize their request and
response shapes:

| Area | Placeholder paths | Owner |
| --- | --- | --- |
| Knowledge base and retrieval | `GET/POST /api/v1/knowledge-bases`, `GET/PATCH/DELETE /api/v1/knowledge-bases/{knowledgeBaseId}`, `GET /api/v1/knowledge-bases/{knowledgeBaseId}/documents`, `GET /api/v1/documents/{documentId}`, `GET /api/v1/documents/{documentId}/chunks`, `POST /api/v1/search` | `knowledge` |
| QA chat and RAG | `/api/v1/chat/**` | `qa` |
| Report generation | `/api/v1/reports/**` | `document` |
| Administration aggregation | `/api/v1/admin/**` | `gateway` plus domain services |

Do not generate frontend API clients or backend handlers for these placeholder
paths until the corresponding OpenAPI operations are added.

## Data Ownership Rules

- A service that owns a database table also owns the API that mutates that data.
- Gateway may expose a frontend-friendly path for that mutation, but must delegate business validation to the owner service.
- Cross-service IDs should be strings in public API contracts. Each service can decide internal ID representation.
- Timestamps in public contracts use RFC 3339 / OpenAPI `date-time`.
- Delete operations must be owned by the service that owns the resource lifecycle.

## Boundary Checks For New Endpoints

Before adding a gateway endpoint, answer these questions in the endpoint doc or OpenAPI description:

1. Which service owns the resource state?
2. Does the endpoint only route, or does it aggregate multiple services?
3. If it aggregates, what frontend screen needs this shape?
4. Which service validates domain rules?
5. Which error codes can the frontend rely on?
6. Does the endpoint expose raw object keys, credentials, prompts, vector payloads, or internal URLs? It should not.

## Anti-Patterns

- Adding SQL, MinIO, Qdrant, or LLM calls directly in gateway handlers.
- Duplicating permission logic in frontend, gateway, and domain service without a single owner.
- Letting gateway translate one frontend action into a long business workflow when one domain service should own the workflow.
- Returning downstream service internals directly to the frontend.
- Creating shared Go packages before at least three services need the same stable abstraction.
