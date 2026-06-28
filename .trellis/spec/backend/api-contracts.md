# API Contracts

> Contract-first rules for gateway-facing and cross-service HTTP APIs.

---

## Scenario: Gateway Contract-First API

### 1. Scope / Trigger

- Trigger: any new or changed frontend-facing gateway endpoint, gateway
  response envelope, frontend API client DTO, or cross-service route ownership.
- Applies to `services/gateway/`, browser API clients under `apps/frontend/`,
  and the domain service that owns the endpoint's business state.

### 2. Signatures

Gateway public endpoints are documented in:

```text
docs/api/gateway.openapi.yaml
```

Public routes use these prefixes:

```text
GET /healthz
GET /readyz
/api/v1/**
```

Every OpenAPI operation must include:

- `operationId`
- `tags`
- `summary`
- at least one success response
- at least one `4XX` response for user-callable operations
- `x-owner-service` for routes backed by a service boundary

### 3. Contracts

Gateway success envelope:

```json
{
  "data": {},
  "requestId": "req_123"
}
```

Gateway paginated envelope:

```json
{
  "data": [],
  "page": {
    "page": 1,
    "pageSize": 20,
    "total": 100
  },
  "requestId": "req_123"
}
```

Gateway error envelope:

```json
{
  "error": {
    "code": "validation_error",
    "message": "request validation failed",
    "requestId": "req_123",
    "fields": {
      "name": "is required"
    }
  }
}
```

Public IDs are strings. Public timestamps use OpenAPI `date-time`.

Gateway must pass request context to downstream services with these headers
when values are available:

| Header | Purpose |
| --- | --- |
| `X-Request-Id` | Correlate frontend request, gateway logs, and downstream logs. |
| `X-User-Id` | Authenticated user identity. |
| `X-User-Roles` | Comma-separated authenticated roles. |
| `X-User-Permissions` | Comma-separated authenticated permissions. |
| `X-Forwarded-For` | Original client address chain. |
| `X-Forwarded-Proto` | Original request protocol. |

### 4. Validation & Error Matrix

| Condition | Public response |
| --- | --- |
| Invalid request shape or field value | `400 validation_error` |
| Missing or invalid authentication | `401 unauthorized` |
| Authenticated caller lacks permission | `403 forbidden` |
| Resource does not exist or is hidden | `404 not_found` |
| State conflict | `409 conflict` |
| Rate or quota exceeded | `429 rate_limited` |
| Downstream service or infrastructure failed | `502 dependency_error` |
| Unexpected gateway failure | `500 internal_error` |

Do not forward raw downstream error bodies, SQL details, object keys, tokens,
prompts, vector payloads, or internal URLs to the frontend.

### 5. Good/Base/Bad Cases

- Good: add a gateway route to `docs/api/gateway.openapi.yaml`, mark
  `x-owner-service`, use the standard envelope, and update
  `docs/service-boundaries.md` if ownership is new.
- Base: proxy a domain-service route through gateway without changing the
  domain response shape, but still normalize errors to the gateway envelope.
- Bad: add a frontend call directly to `services/knowledge` or embed Qdrant,
  MinIO, SQL, prompt, or report-generation logic in gateway.

### 6. Tests Required

When implementation exists:

- Gateway handler tests assert status code, response envelope, and request id.
- Error tests cover validation, auth failure, forbidden, not found, and
  dependency failure where applicable.
- Cross-service client tests use mocked HTTP servers and assert propagated
  context headers.
- Frontend API client tests assert request path, response normalization, and
  error-code mapping.

For documentation-only contract changes:

- Run an OpenAPI linter against `docs/api/gateway.openapi.yaml`.
- Parse the YAML and verify `$ref` targets resolve.
- Check route prefix consistency: health routes stay unversioned, public API
  routes use `/api/v1/**`.

### 7. Wrong vs Correct

#### Wrong

```text
frontend -> services/knowledge/search
gateway handler -> Qdrant query -> raw vector payload response
```

#### Correct

```text
frontend -> gateway /api/v1/search
gateway -> knowledge service
knowledge service -> retrieval infrastructure
gateway -> normalized SearchResponse or ErrorResponse
```

## Related Documents

- `docs/gateway.md`
- `docs/api/gateway.openapi.yaml`
- `docs/service-boundaries.md`
- `docs/frontend-backend-contract.md`

## Scenario: Missing Downstream API Contracts

### 1. Scope / Trigger

- Trigger: a downstream service such as `knowledge`, `qa`, `document`, or an
  aggregation surface has not finalized its frontend/backend contract yet.
- Applies to `docs/api/gateway.openapi.yaml`, `docs/gateway.md`,
  `docs/service-boundaries.md`, and `docs/frontend-backend-contract.md`.

### 2. Signatures

Unfinalized endpoints must not be added as active `paths` operations in:

```text
docs/api/gateway.openapi.yaml
```

Instead, list them under the OpenAPI root extension:

```yaml
x-missing-contracts:
  - service: knowledge
    status: missing
    reason: Frontend/backend contract is not finalized yet.
    placeholderOperations:
      - GET /api/v1/knowledge-bases
```

### 3. Contracts

Active OpenAPI `paths` represent stable frontend-facing contracts. Missing
placeholder operations are TODO markers only:

- frontend clients must not generate callable methods from placeholders,
- backend services must not treat placeholders as implementation commitments,
- docs may describe expected ownership, but not stable request/response fields.

### 4. Validation & Error Matrix

| Condition | Required handling |
| --- | --- |
| Endpoint request/response shape is finalized | Add an active OpenAPI operation with owner, schemas, and error responses. |
| Endpoint owner is known but shape is not finalized | Keep it in `x-missing-contracts` only. |
| Placeholder overlaps with an active operation | Use method-level placeholders, not broad path globs that hide stable operations. |
| Frontend needs a missing endpoint | First finalize and review the OpenAPI operation, then generate or implement clients. |

### 5. Good/Base/Bad Cases

- Good: keep `POST /api/v1/knowledge-bases/{knowledgeBaseId}/documents` active
  for file upload, while marking `GET /api/v1/knowledge-bases/{knowledgeBaseId}/documents`
  missing for the future knowledge-owned list contract.
- Base: mention future `qa` chat routes in prose and mark `/api/v1/chat/**`
  missing until message and SSE event shapes are agreed.
- Bad: add full `knowledge`, `qa`, or `document` schemas to OpenAPI as if they
  were stable just to reserve routes.

### 6. Tests Required

For documentation-only contract changes:

- Parse `docs/api/gateway.openapi.yaml`.
- Verify every active `/api/v1/**` operation has an allowed finalized owner.
- Verify missing downstream areas are present in `x-missing-contracts`.
- Check broad placeholders do not contradict active operations.
- Check Markdown links resolve.

### 7. Wrong vs Correct

#### Wrong

```text
OpenAPI paths include POST /api/v1/chat/sessions/{sessionId}/stream
even though QA message and SSE event contracts are not agreed.
```

#### Correct

```text
x-missing-contracts lists /api/v1/chat/** as missing until the QA contract is finalized.
```

## Scenario: Domain Service Interface Documents

### 1. Scope / Trigger

- Trigger: adding or changing a service-level interface document such as
  `docs/auth.md` or `docs/file.md`.
- Applies when gateway-facing routes depend on an internal domain service
  contract, even if the service code has not been implemented yet.

### 2. Signatures

Service interface documents must list every related gateway route with:

- HTTP method
- gateway path
- authentication requirement
- owner service
- short behavior summary

If an internal service route is proposed, mark it as an internal draft and keep
it separate from the public gateway contract.

### 3. Contracts

Document request and response fields using the same public IDs, timestamps,
envelopes, and error shapes defined in `docs/api/gateway.openapi.yaml`.
Binary success responses, such as file downloads, may omit the JSON envelope,
but error responses must still use the standard error shape.

### 4. Validation & Error Matrix

For each documented endpoint, separate:

- status codes already declared in OpenAPI,
- future status codes that require an OpenAPI update before frontend reliance.

### 5. Good/Base/Bad Cases

- Good: `docs/file.md` documents file-owned routes, notes knowledge-owned
  related routes, and calls out that object keys must not reach the frontend.
- Base: a service document summarizes the gateway OpenAPI without adding
  implementation-only behavior.
- Bad: a service document declares a new frontend-facing status code or field
  as stable without updating `docs/api/gateway.openapi.yaml`.

### 6. Tests Required

For documentation-only changes:

- Parse `docs/api/gateway.openapi.yaml`.
- Verify documented public paths exist in the OpenAPI file.
- Check Markdown links resolve.

When implementation exists, add handler or client tests for the documented
status codes, envelopes, request id propagation, and context headers.

### 7. Wrong vs Correct

#### Wrong

```text
docs/file.md declares GET /api/v1/files/{id}/download as stable
gateway.openapi.yaml has no matching public path
```

#### Correct

```text
docs/file.md references /api/v1/documents/{documentId}/download
gateway.openapi.yaml owns the same public path and owner-service marker
```

## Scenario: Gateway Redis Session Cache

### 1. Scope / Trigger

- Trigger: adding or changing auth login/register/logout/current-user
  behavior, gateway auth middleware, or session identity fields.
- Applies to `services/gateway/`, `services/auth/`, `docs/auth.md`,
  `docs/gateway.md`, `docs/frontend-backend-contract.md`, and
  `docs/api/gateway.openapi.yaml`.

### 2. Signatures

Public auth routes stay under:

```text
POST /api/v1/auth/register
POST /api/v1/auth/login
POST /api/v1/auth/logout
GET  /api/v1/auth/me
```

Auth success responses must include `data.user` and `data.session`.

### 3. Contracts

`data.user` must include:

- `id`
- `username`
- `roles`
- `permissions`

`data.session` must include:

- `sessionId`
- `accessToken`
- `tokenType`
- `expiresAt`

Gateway must store the runtime session in Redis using:

```text
gateway:session:<accessTokenHash>
```

The cached value must include enough fields to inject `X-User-Id`,
`X-User-Roles`, and `X-User-Permissions` without calling auth on every
business request. The Redis TTL must not outlive `data.session.expiresAt`.
Redis is not the durable source of user, role, permission, or session truth.

### 4. Validation & Error Matrix

| Condition | Public response |
| --- | --- |
| Missing bearer credential | `401 unauthorized` |
| Redis session miss, expired session, or malformed cache value | `401 unauthorized` |
| Auth rejects login credentials | `401 unauthorized` |
| Gateway cannot access Redis for an authenticated business request | `502 dependency_error` |
| Auth service or durable auth store is unavailable during login/logout | `502 dependency_error` |

Do not expose raw tokens, token hashes, Redis keys, session secrets, or auth
internal URLs to frontend responses or logs.

### 5. Good/Base/Bad Cases

- Good: login response returns `user` plus `session`; gateway hashes the access
  token for the Redis key, sets TTL from `expiresAt`, and injects downstream
  identity headers from the cache.
- Base: `/auth/me` reads the Redis session cache and returns `UserResponse`
  without calling auth for every request.
- Bad: gateway stores original access tokens in logs or treats Redis as the
  durable source of permissions.

### 6. Tests Required

When implementation exists:

- Auth handler/client tests assert `AuthResponse` includes `user.permissions`
  and `session`.
- Gateway auth middleware tests cover Redis hit, miss, expired session,
  malformed session, and Redis dependency failure.
- Gateway downstream client tests assert `X-User-Id`, `X-User-Roles`,
  `X-User-Permissions`, and `X-Request-Id` are propagated.
- Logout tests assert auth revocation is called and Redis cache is deleted.

For documentation-only changes:

- Parse `docs/api/gateway.openapi.yaml`.
- Verify `AuthResponse` requires `user` and `session`.
- Verify docs mention `gateway:session:<accessTokenHash>` and Redis TTL.

### 7. Wrong vs Correct

#### Wrong

```text
gateway receives Authorization: Bearer token
gateway calls auth service on every business request
gateway logs the raw token on failures
```

#### Correct

```text
gateway receives Authorization: Bearer token
gateway hashes token and reads gateway:session:<accessTokenHash>
gateway injects cached user, roles, and permissions into downstream headers
```
