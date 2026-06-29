# Gateway Active API Contract Verifier PRD

## Background

Issue #149 (`[S-20260629-03] Gateway active API 契约校验器`) requires a repeatable verifier and CI gate for the public gateway OpenAPI contract. The gateway OpenAPI is the source used by frontend type generation, owner assignment, and backend route implementation. Missing operation metadata or drift between OpenAPI and the owner map can block parallel frontend/backend work.

## Contract Sources

- `docs/services/gateway/api/openapi.yaml`
- `docs/services/gateway/docs/active-api-owner-map.md`
- `docs/architecture/frontend-backend-contract.md`
- `docs/architecture/service-boundaries.md`
- `.trellis/spec/backend/api-contracts.md`
- `.trellis/spec/frontend/type-safety.md`
- `.trellis/spec/cicd.md`

## Goals

- Add a locally repeatable contract verifier for active gateway API paths.
- Add a CI gate that runs the verifier on relevant pull requests.
- Check that active `/api/v1/**` operations include required OpenAPI metadata and response classes.
- Check that `x-missing-contracts` remains placeholder-only and does not overlap with active paths.
- Check that stable active public paths remain RESTful and avoid action-style path segments.
- Check that `docs/services/gateway/docs/active-api-owner-map.md` stays consistent with OpenAPI.

## Non-Goals

- Do not redesign gateway API paths outside the violations required by the verifier.
- Do not generate frontend clients in this task.
- Do not implement gateway handlers or downstream services.
- Do not introduce heavyweight runtime dependencies.

## Functional Requirements

1. The verifier loads `docs/services/gateway/api/openapi.yaml`.
2. For each active `/api/v1/**` operation, the verifier fails when any of the following are missing:
   - `operationId`
   - non-empty `tags`
   - `x-owner-service`
   - effective `security` defined either on the operation or inherited from the OpenAPI root
   - at least one `2XX` response
   - at least one `4XX` response
3. `/healthz` and `/readyz` are operational exceptions:
   - they must still have `operationId`, non-empty `tags`, and success responses;
   - they may use `security: []`;
   - they are treated as owner `gateway` in the owner map.
4. The verifier fails if a stable active public path contains action-style segments such as `login`, `logout`, `register`, `download`, `search`, `generate`, `export`, `retry`, or `revoke`.
5. The verifier fails if any `x-missing-contracts.placeholderOperations` entry has the same method and path as an active OpenAPI operation.
6. The verifier fails if `apps/web/package.json` generates frontend API types from anything other than `../../docs/services/gateway/api/openapi.yaml`.
7. The verifier compares the OpenAPI-derived active operation table with `docs/services/gateway/docs/active-api-owner-map.md`:
   - active operation count must match;
   - owner summary counts must match;
   - active operations table rows must match method, path, owner service, first tag, operation id, and auth;
   - missing contract placeholder rows must match OpenAPI `x-missing-contracts`.
8. The verifier exits non-zero with readable error messages when validation fails.

## Test Requirements

- Add unit tests for the verifier before implementation.
- Cover failures for missing operation metadata, missing `4XX`, action-style path segments, missing-contract overlap, frontend generation source drift, and owner map drift.
- Run the verifier against the real repository contract as part of local validation.

## CI Requirements

- Add a GitHub Actions workflow that runs the verifier on pull requests to `develop`.
- The workflow should run only for relevant gateway contract, owner map, frontend API generation, verifier, or workflow changes.
- Keep collaboration guard workflows unchanged.

## Acceptance Criteria

- Modifying gateway OpenAPI without updating the owner map makes the verifier fail.
- Removing owner, operationId, effective auth, or a `4XX` response from an active `/api/v1/**` operation makes the verifier fail.
- Adding an active path that overlaps with `x-missing-contracts` makes the verifier fail.
- Changing frontend type generation away from the gateway OpenAPI makes the verifier fail.
- CI can run the verifier with project-provided code and configuration.
