# Auth Service API Docs

## Goal

Write an interface document for the auth service so frontend/backend collaborators can understand available authentication endpoints, request/response contracts, error behavior, and integration requirements.

## What I Already Know

* User requested: "编写auth service的接口文档".
* This is documentation work, not a behavior change unless code inspection reveals a missing source of truth that must be generated.
* Current repository is in architecture and engineering-guideline phase; `README.md` describes the target `services/auth/` path, but service implementation files are not present yet.
* Existing source-of-truth public auth contract is in `docs/api/gateway.openapi.yaml` under `/api/v1/auth/**`.
* `docs/gateway.md`, `docs/frontend-backend-contract.md`, and `docs/service-boundaries.md` define that frontend calls gateway, while `auth` owns users, credentials, roles, permissions, sessions/tokens, login, logout, current user, and permission checks.
* User clarified the desired session model: auth service returns identity/session information; gateway stores all session information in Redis; later gateway requests read session information from Redis and propagate identity to downstream services.

## Assumptions (Temporary)

* Because there is no auth service implementation yet, the document should be derived from existing architecture and gateway OpenAPI contracts.
* The document should be committed under `docs/` alongside `docs/gateway.md`.
* The primary audience is project developers integrating with the auth service.

## Open Questions

* None blocking. Proceed with `docs/auth.md` as the auth-service planning and interface document.

## Requirements (Evolving)

* Create `docs/auth.md` covering auth service responsibilities, gateway-facing interface, request/response contracts, error behavior, context propagation, security notes, storage ownership, and future implementation guidance.
* Update gateway documentation to describe Redis-backed session cache behavior and downstream identity propagation.
* Update auth service documentation to describe the auth-to-gateway interface payloads needed for gateway session caching.
* Update public OpenAPI auth schemas so login/register responses expose session identity fields consistently with the clarified model.
* Document public gateway paths that map to auth-owned capabilities: register, login, logout, and current user.
* Clearly distinguish public gateway contract (`/api/v1/auth/**`) from internal auth service ownership and future service-local routes.
* Follow existing documentation style and placement where possible.

## Acceptance Criteria (Evolving)

* [x] Auth service endpoints are listed with method, path, purpose, authentication requirement, request schema, success response, and error responses.
* [x] Document includes integration notes for tokens/cookies/session behavior if applicable.
* [x] Document location and format match existing repository conventions.
* [x] Documentation is derived from code or existing specs rather than assumptions.
* [x] The document does not claim implementation details that are not yet present in code.
* [x] Gateway documentation explains Redis session cache keys, cached fields, TTL, hit/miss behavior, and downstream context headers.
* [x] Auth service documentation explains the session identity response returned to gateway.
* [x] Gateway OpenAPI auth response schemas include session identity fields needed by frontend and gateway.

## Definition of Done

* Documentation file added or updated.
* Relevant docs/spec conventions checked.
* Formatting/readability reviewed.
* No unrelated code changes.

## Out of Scope (Explicit)

* Changing auth service runtime behavior.
* Adding new auth endpoints.
* Implementing frontend integration.

## Technical Notes

* Task created: 2026-06-28 10:15 CST.
* Inspected `README.md`, `docs/gateway.md`, `docs/frontend-backend-contract.md`, `docs/service-boundaries.md`, `docs/api/gateway.openapi.yaml`, and `.trellis/spec/backend/api-contracts.md`.
* No `services/auth/` code exists in the current tree.
