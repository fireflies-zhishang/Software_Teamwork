# F-015 Knowledge Capability Gating

## Goal

Complete GitHub issue #281 by making the Knowledge frontend treat Gateway capability errors as explicit product states instead of successful empty data. Ready paths for knowledge base CRUD, document list, and document upload must keep using the real Gateway API, while currently unready paths are gated with clear UI feedback.

## Requirements

- Keep Knowledge base CRUD, document list, document detail, and document upload on Gateway `/api/v1/**` APIs.
- For document PATCH/DELETE, document chunks, document content download, `knowledge-queries`, and admin parser configs, show explicit not-ready or dependency-failed states when Gateway returns `501`, `not_implemented`, or `dependency_error`.
- Do not show mock retrieval results. Search must either show real `knowledge-queries` results or a clear unavailable/error state.
- Include `requestId` in user-facing error details whenever Gateway provides it; when missing, state that no requestId was returned.
- Distinguish permission failures (`403` / `forbidden`) from implementation readiness and dependency failures.
- Do not call Knowledge, File, Parser, Qdrant, or AI Gateway internal service URLs from the browser.
- Keep task documents and final progress notes synchronized with implementation and verification.

## Acceptance Criteria

- [x] Ready Knowledge CRUD/upload flows still call Gateway real APIs.
- [x] Chunks/content/knowledge-queries/parser-configs return disabled, skipped, not-ready, or dependency-failed UI on capability errors.
- [x] `501` / `not_implemented` is never treated as an empty list or successful no-result search.
- [x] Error notices include `requestId` when available and an explicit missing-requestId note otherwise.
- [x] Browser code does not introduce direct calls to internal Knowledge/File/Parser/Qdrant/AI Gateway addresses.
- [x] Frontend checks are run and reported: `bun run --cwd apps/web check`, `bun run --cwd apps/web build`, and `git diff --check`.

## Definition of Done

- Code implemented under `apps/web/src/` using existing frontend patterns.
- Unit tests added or updated for new capability/error classification logic.
- Task PRD/research/progress files reflect final scope and validation status.
- Work is committed with Conventional Commit format and pushed to the fork branch.
- After push, remote updates are checked again against `upstream/develop`.
- Completed Trellis task is archived; unrelated active tasks are left untouched.

## Technical Approach

Add a small frontend helper for Gateway capability errors that classifies `ApiError` into `not_ready`, `dependency_failed`, `forbidden`, or generic error. Reuse that helper in Knowledge documents, chunks, search, and parser-config pages so state copy and requestId display are consistent. Keep the API transport wrapper responsible for preserving status, code, and requestId from Gateway envelopes.

## Decision (ADR-lite)

**Context**: Gateway OpenAPI exposes Knowledge active paths, but current implementation docs state several active paths may return `501 not_implemented` or `502 dependency_error` while backend tasks are still in flight.

**Decision**: Gate by runtime Gateway error classification in the frontend rather than hard-disabling all unready paths up front. This lets ready deployments use the API immediately while still preventing 501/502 from being mistaken for empty success.

**Consequences**: The UI remains contract-driven and does not need a separate capability endpoint. Runtime capability state depends on Gateway returning standard error envelopes, so missing request IDs must be surfaced explicitly.

## Out of Scope

- Implementing Knowledge retrieval, Parser runtime, Qdrant, embedding, or rerank.
- Replacing backend tasks #84, #236, or #125.
- Adding direct browser calls to internal services.
- Changing Gateway OpenAPI or generated types unless implementation requires it.

## Technical Notes

- Issue: https://github.com/Sakayori-Iroha-168/Software_Teamwork/issues/281
- Current branch: `Frontend/feat/knowledge-capability-gating`, created from `upstream/develop` on 2026-06-30.
- Relevant docs read: `CONTRIBUTING.md`, `docs/collaboration/frontend-workflow.md`, `docs/architecture/frontend-backend-contract.md`, `docs/architecture/current-capability-matrix.md`, `docs/services/gateway/docs/active-api-owner-map.md`, `docs/services/knowledge/docs/implementation.md`, `.trellis/spec/frontend/*`, `.trellis/spec/backend/error-handling.md`, `.trellis/spec/backend/api-contracts.md`.
- `docs/collaboration/frontend-readiness-task-plan.md` is referenced by the issue but is not present in current `upstream/develop`; repo search found no such file.

## Progress Report

- 2026-06-30: Synced all remotes and read issue #281 body/comments via GitHub API. EIR has already claimed and is assigned.
- 2026-06-30: Created this Trellis task and aligned branch/base/scope metadata.
- 2026-06-30: Read current docs and frontend implementation; identified Knowledge documents, chunks, search, parser-config pages and `apps/web/src/api/client.ts` as the implementation surface.
- 2026-06-30: Implemented `getGatewayCapabilityIssue` / `formatGatewayCapabilityError`, added classifier tests, and wired Knowledge base management, documents, chunks, search, and parser-config pages to requestId-aware capability states.
- 2026-06-30: Removed the unauthenticated direct download anchor fallback for document content; downloads now use the typed Gateway client only.
- 2026-06-30: Updated `.trellis/spec/frontend/type-safety.md` with the Gateway capability error presentation convention.
- 2026-06-30: Addressed PR review finding by routing the document list `useDocuments` error state through `getGatewayCapabilityIssue(error, '文档列表')`, so `501` / `not_implemented`, `502` / `dependency_error`, and `403` / `forbidden` show the same classified title, description, requestId detail, and variant as other Knowledge pages.
- 2026-06-30: Rebasing and push state checked after the review fix: branch `Frontend/feat/knowledge-capability-gating` is based on `upstream/develop` `2864395`, and PR #291 is updated from the EIR fork branch.
- 2026-06-30: Synchronized archived task metadata and progress records with the PR review follow-up while keeping volatile PR head/CI state for the final handoff report.
- 2026-06-30: Addressed follow-up PR review finding for the no-knowledge-base document page by routing the auxiliary `useKnowledgeBases(1, 100)` selector query through `getGatewayCapabilityIssue(error, '知识库列表')`, including classified copy, retry, and requestId details instead of an empty selector.

## Verification

- `bun run --cwd apps/web test:unit -- src/features/knowledge/capability.test.ts src/api/client.test.ts` — passed.
- `bun run --cwd apps/web check` — passed.
- `bun run --cwd apps/web build` — passed; Vite reported the existing large chunk warning.
- `git diff --check` — passed.
- Post-review rebase verification repeated on 2026-06-30: targeted unit tests, `check`, `build`, and `git diff --check` all passed; Vite large chunk warning remains.
- Follow-up review verification repeated on 2026-06-30: targeted unit tests, `check`, `build`, and `git diff --check` passed; Vite large chunk warning remains.
- GitHub Actions are checked after each push and reported in the final handoff; local verification above is the task quality gate.
