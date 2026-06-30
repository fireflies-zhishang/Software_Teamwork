# A-014 Knowledge contract alignment

## Goal

Align Knowledge active-operation implementation, contract tests, error envelopes, request-id behavior, and service documentation with the public docs and gateway OpenAPI for issue #86. This is an alignment and verification task, not a new product capability expansion.

## What I Already Know

- Issue #86 asks for A-014: Knowledge 契约测试、实现文档和现有代码对齐.
- Suggested branch: `L1nggTeam/test/knowledge-contract-alignment`.
- Authoritative sources are:
  - `docs/services/knowledge/README.md`
  - `docs/services/knowledge/docs/api-contract.md`
  - `docs/services/knowledge/docs/implementation.md`
  - `docs/services/gateway/api/openapi.yaml`
  - `docs/architecture/service-boundaries.md`
- `docs/services/knowledge/docs/api-contract.md` section 2.6 explicitly allows A-14 tests to use seeded repositories and fake file/queue/vector/AI adapters; real A-11 worker runtime is not required.
- Current `develop` already includes A-12 retrieval-related code and `POST /internal/v1/knowledge-queries`.
- `services/gateway/internal/http/routes.go` currently proxies `POST /api/v1/knowledge-queries` to Knowledge but still marks `PATCH/DELETE /api/v1/documents/{documentId}`, `GET /api/v1/documents/{documentId}/chunks`, and `GET /api/v1/documents/{documentId}/content` as `NotImplemented`.
- `services/knowledge/internal/repository/postgres.go` and service code already include chunk-list and retrieval plumbing, so the expected work is likely contract coverage plus route/handler/doc alignment rather than broad new behavior.
- GitHub issue claim via `gh` failed because GitHub CLI is not authenticated in this environment. The user needs to claim/assign in the web UI or configure `gh auth`.

## Assumptions

- Docs are authoritative. If code conflicts with docs, fix code or explicitly record implementation status; do not downgrade docs to match incomplete code.
- Keep the PR focused on Knowledge, Gateway routing/proxy tests only where needed, and documentation/runbook updates.
- Use fakes/seeded data for contract tests unless a real dependency smoke is already supported by local tooling.
- Do not introduce new product endpoints outside the active paths already described in gateway OpenAPI and Knowledge docs.

## Requirements

- Add or update contract tests for active Knowledge operations, especially:
  - `GET /api/v1/documents/{documentId}/chunks`
  - `GET /api/v1/documents/{documentId}/content`
  - `POST /api/v1/knowledge-queries`
  - relevant document metadata state paths if implemented.
- Cover stable error envelopes for validation, unauthorized/forbidden, not found, conflict when applicable, dependency error, and internal error where the current code path supports them.
- Cover `X-Request-Id` propagation and JSON `requestId` behavior for Knowledge-facing handlers and Gateway proxy paths.
- Check implementation against Knowledge docs and Gateway OpenAPI; fix code when docs and code conflict.
- Remove stale references to old `docs/api` paths or action-style search paths from Knowledge documentation and runtime docs.
- Update README, `.env.example`, local Docker Compose notes, and run instructions as needed for File, AI Gateway, Qdrant, Redis, and Parser/Knowledge integration.
- Record a runnable local validation path or explicitly document what requires external services and what is covered by fakes.

## Acceptance Criteria

- [ ] `cd services/knowledge && go test ./...` passes.
- [ ] Gateway/Knowledge contract tests cover main active Knowledge routes, error envelope shape, and request-id behavior.
- [ ] Documentation no longer refers to old `docs/api` paths or action-style Knowledge search routes.
- [ ] Implementation and docs agree on owner-service boundaries for File, Parser, AI Gateway, Qdrant, Redis, and Gateway.
- [ ] Any unrun smoke or integration check is documented with reason and remaining risk.

## Definition Of Done

- Tests added or updated for contract and error behavior.
- Relevant service-local checks run.
- Documentation updated for current implementation and local validation.
- No unrelated refactors or product-scope expansion.
- Commit messages follow Conventional Commits.

## Technical Approach

Use docs-first reconciliation:

1. Inventory active Knowledge operations in `docs/services/gateway/api/openapi.yaml`, `services/gateway/internal/http/routes.go`, and `services/knowledge/internal/http/server.go`.
2. Add focused handler/proxy tests with seeded memory repositories and fake dependencies for success, error envelope, and request-id propagation.
3. Implement small route/handler gaps where the docs already define active paths and service/repository support exists.
4. Update Knowledge implementation docs and run instructions to reflect exactly what is implemented, what is fake-backed, and what still requires external smoke.

## Out Of Scope

- Full upload -> worker -> Parser -> Qdrant -> retrieval end-to-end smoke if local dependencies are not already wired.
- New product APIs outside existing Gateway OpenAPI active paths.
- New organization/electric-plant/profession-level authorization model.
- Replacing fake contract tests with real File/Redis/Qdrant/AI Gateway environments.

## Technical Notes

- `services/knowledge/internal/http/server.go` is the likely Knowledge handler focus.
- `services/gateway/internal/http/routes.go` and proxy tests may need updates if active paths should no longer return `not_implemented`.
- `services/knowledge/internal/service/retrieval.go`, `types.go`, `repository/memory.go`, and `repository/postgres.go` contain retrieval/chunk contracts.
- Backend guidelines to apply:
  - `.trellis/spec/backend/index.md`
  - `.trellis/spec/backend/api-contracts.md`
  - `.trellis/spec/backend/error-handling.md`
  - `.trellis/spec/backend/database-guidelines.md`
  - `.trellis/spec/backend/quality-guidelines.md`
  - `.trellis/spec/backend/logging-guidelines.md`
