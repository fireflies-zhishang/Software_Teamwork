# Mark Missing Downstream API Contracts

## Goal

Adjust the gateway/API documentation so only the currently agreed auth,
gateway, and file service interfaces are presented as concrete frontend-facing
contracts. Interfaces for downstream services whose frontend/backend contracts
are not finalized yet should be left as explicit missing/TBD placeholders.

## What I Already Know

* User said downstream service frontend/backend interfaces are not fully
  determined.
* User wants every service except auth, gateway, and file to stay empty for now
  and be marked as missing.
* Existing PR #26 currently includes concrete OpenAPI paths for knowledge, QA,
  document/report, and admin overview.
* Existing docs already contain detailed auth and file service interface docs.

## Assumptions

* "认证网关文件三大服务" means auth, gateway, and file are the only concrete
  service contracts to keep in this PR.
* Gateway health/readiness and generic envelope/error/security schemas should
  remain because they are gateway-level contract surface.
* File upload/download-related paths may remain concrete even when they mention
  knowledge handoff, but knowledge-owned query/search/chunk APIs should be
  marked missing until finalized.

## Requirements

* Keep concrete OpenAPI operations for:
  * gateway health/readiness,
  * auth register/login/logout/me,
  * file-owned upload/update/delete/download routes.
* Remove or demote concrete knowledge, QA/chat, document/report, and admin
  overview frontend-facing operations from the active OpenAPI contract.
* Add an explicit placeholder section that marks missing downstream contracts
  for knowledge, QA, document/report, and admin overview.
* Update gateway, frontend-backend, service-boundary, and file docs so they do
  not imply unfinished downstream service interfaces are stable.
* Preserve valid OpenAPI YAML with resolvable `$ref` targets.

## Acceptance Criteria

* [x] `docs/api/gateway.openapi.yaml` contains no active concrete paths owned by
  `knowledge`, `qa`, or `document` except file-upload handoff text where needed.
* [x] Missing downstream service interfaces are called out explicitly.
* [x] Auth, gateway health/readiness, and file-owned public routes remain
  documented.
* [x] Markdown docs point readers to the missing-contract status instead of
  implying stable downstream APIs.
* [x] OpenAPI YAML parses and all `$ref` targets resolve.

## Definition of Done

* Documentation updated.
* Relevant Trellis API/docs guidelines checked.
* OpenAPI and local Markdown link checks pass.
* Changes committed and pushed to the existing PR branch.

## Out of Scope

* Designing final knowledge, QA, document/report, or admin overview contracts.
* Implementing backend services or frontend API clients.
* Changing auth or file runtime behavior.

## Technical Notes

* Initial inspection found concrete downstream paths in
  `docs/api/gateway.openapi.yaml` for knowledge bases, search, chat, reports,
  and admin overview.
* Related docs: `docs/gateway.md`, `docs/frontend-backend-contract.md`,
  `docs/service-boundaries.md`, `docs/file.md`.
