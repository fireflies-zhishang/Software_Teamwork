# Gateway Service

`gateway` is the public backend entrypoint for frontend, admin, backend module,
and tool callers. It provides operational health checks, request edge
middleware, auth session cache handling, and active OpenAPI route proxy coverage.
Gateway does not own a business database or implement downstream domain
workflows.

Authoritative contracts and service boundaries:

- `docs/services/gateway/README.md`
- `docs/services/gateway/api/openapi.yaml`
- `docs/services/gateway/docs/data-models.md`
- `docs/architecture/service-boundaries.md`
- `docs/architecture/technology-decisions.md`

## Local Run

```bash
cd services/gateway
go run ./cmd/server
```

Default address:

```text
:8080
```

Health checks:

```bash
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/readyz
```

Both endpoints return the project success envelope and include `X-Request-Id`:

```json
{
  "data": {
    "status": "ok",
    "service": "gateway",
    "version": "0.1.0",
    "environment": "local"
  },
  "requestId": "req_123"
}
```

## Environment Variables

| Variable | Default | Description |
| --- | --- | --- |
| `GATEWAY_HTTP_ADDR` | `:8080` | HTTP listen address. |
| `GATEWAY_METRICS_ADDR` | `:9091` | Internal Prometheus metrics listen address. Not exposed via the public gateway port. |
| `GATEWAY_SERVICE_VERSION` | `0.1.0` | Version reported by health checks and startup logs. |
| `GATEWAY_ENV` | `local` | Runtime environment label. |
| `GATEWAY_MAX_BODY_BYTES` | `10485760` | Maximum request body size enforced at the gateway edge. |
| `GATEWAY_REQUEST_TIMEOUT` | `30s` | Per-request context timeout. |
| `GATEWAY_SHUTDOWN_TIMEOUT` | `10s` | Graceful shutdown timeout. |
| `GATEWAY_DOWNSTREAM_TIMEOUT` | `10s` | Timeout for auth and owner-service HTTP calls. |
| `GATEWAY_CORS_ALLOWED_ORIGINS` | `*` | Comma-separated allowed CORS origins. |
| `GATEWAY_CORS_ALLOWED_METHODS` | `GET,POST,PATCH,DELETE,OPTIONS` | Comma-separated allowed CORS methods. |
| `GATEWAY_CORS_ALLOWED_HEADERS` | `Authorization,Content-Type,X-Request-Id` | Comma-separated allowed CORS headers. |
| `GATEWAY_CORS_ALLOW_CREDENTIALS` | `false` | Whether CORS credentialed requests are allowed. |
| `GATEWAY_REDIS_ADDR` | `localhost:6379` | Redis endpoint for `gateway:session:<accessTokenHash>` cache entries. |
| `GATEWAY_REDIS_PASSWORD` | unset | Redis password. Never log this value. |
| `GATEWAY_REDIS_DB` | `0` | Redis DB index. |
| `GATEWAY_TOKEN_HASH_SECRET` | local dev default | HMAC secret used to derive opaque-token cache keys. Override outside local development. |
| `GATEWAY_TOKEN_HASH_KEY_VERSION` | `v1` | Version segment in `hmac-sha256:<version>:<hex>`. |
| `GATEWAY_INTERNAL_SERVICE_TOKEN` | unset | Internal service credential forwarded as `X-Service-Token` when configured. |
| `GATEWAY_AUTH_BASE_URL` | `http://localhost:8001` | Auth service base URL for user/session public routes. |
| `GATEWAY_KNOWLEDGE_BASE_URL` | unset | Knowledge service base URL for knowledge-owned active routes. |
| `GATEWAY_QA_BASE_URL` | unset | QA service base URL for QA-owned active routes. |
| `GATEWAY_DOCUMENT_BASE_URL` | unset | Document service base URL for document-owned active routes. |
| `GATEWAY_AI_GATEWAY_BASE_URL` | unset | AI Gateway base URL for admin model-profile routes. |

## Metrics

The gateway exposes Prometheus metrics on a separate internal port (default `:9091`)
so that `/metrics` is never reachable through the public gateway port.

Metrics exposed:

| Metric | Type | Labels | Description |
| --- | --- | --- | --- |
| `gateway_http_requests_total` | Counter | `method`, `path`, `status` | Total HTTP requests handled by the gateway. `path` is the matched route pattern (e.g. `GET /api/v1/sessions`), never the raw URL. |
| `gateway_http_request_duration_seconds` | Histogram | `method`, `path`, `status` | Request duration in seconds. |

Verify locally after starting the server:

```bash
curl -s http://localhost:9091/metrics | grep gateway_http
```

## Tests

Run service-local checks from this directory:

```bash
go test ./...
go build ./cmd/server
```

## Runtime Behavior

- `POST /api/v1/users` and `POST /api/v1/sessions` call auth internal
  `/internal/v1/**` resources, then cache the returned session identity in
  Redis.
- Protected routes require `Authorization: Bearer <accessToken>` and resolve
  identity from Redis before proxying.
- Gateway injects `X-Request-Id`, `X-User-Id`, `X-User-Roles`,
  `X-User-Permissions`, `X-Forwarded-For`, and `X-Forwarded-Proto` into
  downstream requests.
- Active routes from `docs/services/gateway/docs/active-api-owner-map.md` are
  registered. Missing `admin-overview` and `admin-metrics` placeholders are not
  implemented.
- Binary content and `text/event-stream` responses are streamed from downstream
  without wrapping them in a JSON envelope.

## Scope Guardrails

- Gateway initializes Redis only for short-lived session cache entries.
- Gateway does not persist auth, file, knowledge, QA, document, or AI Gateway
  business state.
- Public JSON responses use `{ "data": ..., "requestId": ... }` for success and
  `{ "error": { "code": ..., "message": ..., "requestId": ... } }` for errors.
- Gateway must not call SQL, MinIO, Qdrant, or model providers directly.
