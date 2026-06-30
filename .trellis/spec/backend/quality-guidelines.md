# Quality Guidelines

> Code quality standards for Go backend services.

---

## Overview

Every backend service must remain independently testable, buildable, and
deployable. Quality checks run from each service directory because every service
owns a separate `go.mod`.

Minimum Go service-local checks:

```bash
go test ./...
go build ./cmd/server
```

`services/parser/` is a Python runtime boundary rather than a Go service. When
changing Parser, run these checks from `services/parser`:

```bash
uv run ruff check .
uv run pytest
uv run python -m compileall src tests
```

### Parser Real PaddleOCR Smoke

#### 1. Scope / Trigger

- Trigger: changing Parser PaddleOCR runtime code, model loading behavior,
  deployment resource docs, or local OCR smoke commands.
- Applies to `services/parser` tests/docs and Parser runbook entries.

#### 2. Signatures

```bash
PARSER_PADDLEOCR_SMOKE=1 \
PARSER_PADDLEOCR_ALLOW_DOWNLOAD=1 \
uv run pytest -m paddleocr_smoke -s
```

Offline or deployment-like runs should use:

```bash
PARSER_PADDLEOCR_SMOKE=1 \
PARSER_PADDLEOCR_CONFIG_PATH=/absolute/path/to/paddlex.yaml \
uv run pytest -m paddleocr_smoke -s
```

#### 3. Contracts

- Without `PARSER_PADDLEOCR_SMOKE=1`, the real model smoke must skip and ordinary
  Parser CI must rely on fake OCR tests.
- With `PARSER_PADDLEOCR_SMOKE=1`, missing PaddleOCR/PaddlePaddle runtime or
  missing model policy must produce an actionable local test failure.
- The smoke must call the Parser PaddleOCR backend path and assert non-empty OCR
  output from a small fixture.

#### 4. Validation & Error Matrix

| Condition | Required handling |
| --- | --- |
| Smoke env unset | `pytest.skip`, ordinary CI passes without PaddleOCR. |
| Runtime modules missing | Fail with install command such as `uv sync --group dev --extra paddleocr`. |
| No local model config and downloads not allowed | Fail with `PARSER_PADDLEOCR_CONFIG_PATH` or `PARSER_PADDLEOCR_ALLOW_DOWNLOAD` guidance. |
| OCR returns empty content | Fail with fixture, language/device, and model-completeness guidance. |

#### 5. Good/Base/Bad Cases

- Good: default `uv run pytest` skips real model smoke, while an explicit local
  env run validates model loading and fixture OCR.
- Base: PR records that only fake OCR checks ran because the local machine lacks
  PaddleOCR models.
- Bad: ordinary CI downloads PaddleOCR models, or a smoke failure emits raw
  provider/debug bodies instead of a short actionable diagnostic.

#### 6. Tests Required

- Parser fake OCR suite: `uv run pytest`.
- Parser lint: `uv run ruff check .`.
- Parser compile: `uv run python -m compileall src tests`.
- For PaddleOCR runtime/resource changes, run the env-gated smoke when a real
  model environment is available; otherwise record why it was skipped.

#### 7. Wrong vs Correct

Wrong:

```bash
uv run pytest  # implicitly downloads real OCR models in CI
```

Correct:

```bash
uv run pytest  # fake OCR only; real model smoke skipped
PARSER_PADDLEOCR_SMOKE=1 PARSER_PADDLEOCR_ALLOW_DOWNLOAD=1 \
  uv run pytest -m paddleocr_smoke -s
```

When lint tooling is introduced, CI should run the selected linter for each
changed service.

---

## Go Service CI Baseline

### 1. Scope / Trigger

- Trigger: adding or changing repository CI for landed Go services under `services/*`.

### 2. Signatures

- Workflow: `.github/workflows/go-services.yml`.
- Events: `pull_request` and `push` to `develop` with path filters for `services/**` and the workflow file.
- Matrix key: `service`, with one entry for each landed Go service that owns a `go.mod`.

### 3. Contracts

- Toolchain: `actions/setup-go@v5` with `go-version: '1.25.x'`.
- Working directory: `services/${{ matrix.service }}`.
- Required commands for every matrix service: `go test ./...` and `go build ./cmd/server`.
- QA contract: run `go build ./cmd/agent` when `services/qa/cmd/agent` exists.
- Cache dependency input must exist for every matrix service; use `services/${{ matrix.service }}/go.mod` unless all services have `go.sum`.

### 4. Validation & Error Matrix

| Condition | Required response |
|-----------|-------------------|
| Service directory has `go.mod` but no matrix entry | Add it before merging CI changes. |
| Matrix entry has no `services/<name>/go.mod` | Remove or fix the entry; setup/run will fail. |
| Dockerfile Go image diverges from module baseline | Update module and Go build image together. |
| `services/qa/cmd/agent` exists but CI does not build it | Add or restore the QA agent build step. |

### 5. Good/Base/Bad Cases

- Good: `services/qa` runs tests, server build, and agent build under Go `1.25.x`.
- Base: a service with no `go.sum` still caches against its existing `go.mod`.
- Bad: a root-level Go workflow runs from the repository root and assumes a root `go.mod`.

### 6. Tests Required

- For each changed Go service, run `go test ./...` from the service directory.
- For each changed Go service, run `go build ./cmd/server` from the service directory.
- For QA, also run `go build ./cmd/agent` from `services/qa`.
- Run `git diff --check` before commit.

### 7. Wrong vs Correct

Wrong:

```yaml
with:
  go-version: '1.25.x'
  cache-dependency-path: services/${{ matrix.service }}/go.sum
```

Correct when not every service has `go.sum`:

```yaml
with:
  go-version: '1.25.x'
  cache-dependency-path: services/${{ matrix.service }}/go.mod
```

---

## Go Migration CI Baseline

### 1. Scope / Trigger

- Trigger: adding or changing repository CI for service-owned PostgreSQL migrations under `services/*/migrations`.

### 2. Signatures

- Workflow: `.github/workflows/go-migrations.yml`.
- Events: `pull_request` and `push` to `develop` with path filters for service migrations, service README files, the workflow file, and technology decisions.
- Matrix key: `service`, with one entry for each landed Go service that owns SQL migration files.

### 3. Contracts

- PostgreSQL CI image: `postgres:16-alpine`.
- Goose command: `go run github.com/pressly/goose/v3/cmd/goose@v3.27.1 -dir migrations postgres "$DATABASE_URL" up`.
- Working directory: `services/${{ matrix.service }}`.
- Migration filenames must match ordered snake_case names such as `0001_create_users.sql`.
- SQL migrations must include `-- +goose Up`; `-- +goose Down` is optional only for forward-only slices.

### 4. Validation & Error Matrix

| Condition | Required response |
|-----------|-------------------|
| Service has `migrations/*.sql` but no matrix entry | Add the service to migration CI before merging. |
| SQL migration has no `-- +goose Up` annotation | Add the annotation so goose can parse it. |
| Migration filename lacks an ordered prefix | Rename to `0001_<snake_case_summary>.sql` or the next ordered prefix. |
| README goose command version differs from CI | Update both to `v3.27.1`. |

### 5. Good/Base/Bad Cases

- Good: `services/auth` migration applies against an empty PostgreSQL database with `goose@v3.27.1`.
- Base: a forward-only migration has `-- +goose Up` and no down section.
- Bad: a service relies only on PostgreSQL Docker init scripts, or README says `goose` without a pinned version.

### 6. Tests Required

- Run migration apply validation for every matrix service or rely on the PR workflow when local PostgreSQL is unavailable.
- Run `git diff --check` before commit.
- Run service-local Go tests when migration files or repository code changed.

### 7. Wrong vs Correct

Wrong:

```bash
goose -dir migrations postgres "$DATABASE_URL" up
```

Correct:

```bash
go run github.com/pressly/goose/v3/cmd/goose@v3.27.1 -dir migrations postgres "$DATABASE_URL" up
```

---

## Scenario: Local Integration Compose Baseline

### 1. Scope / Trigger

- Trigger: adding or changing `deploy/docker-compose.yml`, local demo seed
  data, service Dockerfiles, service port mappings, health checks, readiness
  wiring, service tokens, or `.env.example` files for the backend integration
  environment.
- Applies to `deploy/**`, service-local Dockerfiles, migration wiring, and
  documentation that tells frontend or new contributors how to start services.

### 2. Signatures

- Compose entrypoint:
  - `docker compose -f deploy/docker-compose.yml --env-file deploy/.env.example config --quiet`
  - `docker compose -f deploy/docker-compose.yml --env-file deploy/.env.example --profile ai config --quiet`
- Runtime entrypoint:
  - `cd deploy && docker compose up -d --build`
  - `cd deploy && docker compose --profile ai up -d --build`
- Public browser/API entrypoint:
  - `http://localhost:8080` through gateway.
- Operational routes:
  - `GET /healthz`
  - `GET /readyz`

### 3. Contracts

- Frontend and browser-facing documentation must route traffic through gateway;
  internal service ports may be exposed only for local debugging.
- `.env.example` values must be local placeholders and must not contain real
  provider keys, tokens, passwords, or production credentials.
- Compose must include health checks for infrastructure and service containers
  where the image has a practical probe command.
- PostgreSQL health checks used by migration jobs must probe TCP readiness, e.g.
  `pg_isready -h localhost -U postgres -d postgres`; socket-only checks can pass
  during the official image's temporary init server and start migrations before
  port `5432` accepts connections.
- Qdrant health checks must use commands available inside `qdrant/qdrant`; do
  not assume `curl` or `wget` exists. A bash TCP probe such as
  `bash -ec '</dev/tcp/127.0.0.1/6333'` is acceptable when HTTP tooling is absent.
- Service containers must use service-local Dockerfiles and keep each Go service
  independently buildable.
- Docker build priority is: runnable builds first, build speed second, small
  image size third, low memory use fourth, and low storage use fifth.
- Go service Dockerfiles must keep checksum verification enabled by default.
  The default build args should use `GOPROXY=https://proxy.golang.org,direct`
  and `GOSUMDB=sum.golang.org`; mainland China acceleration may be an explicit
  build-arg override such as `GOPROXY=https://goproxy.cn,direct` paired with
  `GOSUMDB=sum.golang.google.cn`.
- Do not use `GOSUMDB=off` as a normal Docker build fix. It is only an explicit
  last-resort local workaround because it disables module checksum verification.
- Compose infrastructure images must keep explicit pinned defaults. If a local
  or enterprise registry is required, expose it through image variables such as
  `POSTGRES_IMAGE`, `REDIS_IMAGE`, `QDRANT_IMAGE`, `MINIO_IMAGE`, and
  `MINIO_MC_IMAGE`; do not replace pinned defaults with `latest`.
- Local Docker image tags must stay pinned and version-aligned across Compose,
  Dockerfiles, README/runbooks, and `docs/architecture/technology-decisions.md`.
  The current backend baseline is `postgres:16-alpine`, `redis:7-alpine`,
  `qdrant/qdrant:v1.18.2`, `golang:1.25-alpine`, and `alpine:3.22` for Go
  service runtime stages. MinIO is the explicit exception: root Compose uses one
  `minio/minio` server image and one `minio/mc` bucket-initializer image, so
  different release tags are allowed only when the higher server tag has no
  matching `mc` manifest and the reason is documented.
- PostgreSQL seed scripts may create local/demo data only after service-owned
  migrations have applied; production seed or secret material does not belong in
  `deploy/seeds`.
- File Service runs with PostgreSQL metadata in the root local Compose baseline,
  so it must receive `FILE_INTERNAL_SERVICE_TOKEN` or `INTERNAL_SERVICE_TOKEN`.
  Callers that may reach File Service without passing through gateway, such as
  Knowledge and Document background workers, must also receive a matching file
  service token and send it as `X-Service-Token`.
- Optional services such as `ai-gateway` may use a Compose profile, but gateway
  base URL configuration should remain explicit so route failures are
  diagnosable.

### 4. Validation & Error Matrix

| Condition | Required handling |
| --- | --- |
| Compose YAML or env interpolation is invalid | `docker compose ... config --quiet` must fail before merge. |
| Required Docker image is unavailable locally | Document `docker pull` commands and report Docker runtime validation as skipped. |
| Same component appears with multiple Docker tags | Use the documented baseline or, absent a special reason, the higher explicit version; if code cannot be changed in the same task, record the implementation/documentation mismatch in the service implementation document. |
| Docker build times out on `proxy.golang.org` | Rebuild with explicit build args `GOPROXY=https://goproxy.cn,direct` and `GOSUMDB=sum.golang.google.cn`; keep checksum verification enabled. |
| Go migration image fails while verifying goose dependencies through a proxy sumdb path | Avoid the broken third-party sumdb proxy path by setting `GOSUMDB=sum.golang.google.cn`; do not default to `GOSUMDB=off`. |
| Compose infrastructure image pull is slow or blocked | Fix the Docker daemon registry mirror first; if using an enterprise registry rewrite, set the matching `*_IMAGE` variable to a complete pinned image name. |
| Migration jobs fail with `connect: connection refused` immediately after PostgreSQL init | Ensure Postgres healthcheck uses `pg_isready -h localhost`, then recreate containers without deleting volumes unless seed state requires it. |
| Qdrant stays `health: starting` while `http://localhost:6333/readyz` works | Inspect Docker health output for missing probe tools and switch to an in-image TCP probe. |
| File calls return `401 unauthorized` while `file /readyz` is healthy | Verify `FILE_INTERNAL_SERVICE_TOKEN` on file and matching `KNOWLEDGE_SERVICE_TOKEN`, `DOCUMENT_FILE_SERVICE_TOKEN`, or propagated `X-Service-Token` on callers. |
| Gateway readiness fails | Check Redis and auth first, then search logs by `X-Request-Id`. |
| Auth/document/ai-gateway readiness fails | Inspect PostgreSQL container, migration job, and service logs. |
| Seed data insert fails | Keep scripts idempotent with `ON CONFLICT` and verify migrations ran first. |
| Optional AI Gateway is not running | Core startup may proceed, but QA/model routes should document dependency failure risk. |

### 5. Good/Base/Bad Cases

- Good: `deploy/README.md` documents ports, env keys, image pulls, seed data,
  request-id troubleshooting, and common dependency failures; Compose config
  parses for both default and optional profiles.
- Base: Docker runtime smoke tests are skipped when images are missing, but the
  exact image pull commands and skipped validation are reported.
- Bad: frontend documentation points to `http://localhost:8083` for Knowledge,
  `.env.example` contains a real provider API key, a seed script writes data
  before the owning service migration job completes, or File Service runs in DB
  mode without matching service tokens for direct internal callers.

### 6. Tests Required

- Run Compose config parsing for default and optional profiles.
- Search Docker and docs for duplicate image tags such as `redis:7` vs
  `redis:7-alpine`, `alpine:3.21` vs `alpine:3.22`, and MinIO server/client
  tags before declaring version cleanup complete.
- Run `git diff --check`.
- Run `go test ./...` and `go build ./cmd/server` for changed Go services or
  every service referenced by the integration baseline when feasible.
- For QA, also run `go build ./cmd/agent` when `services/qa/cmd/agent` exists.
- Run Docker image build/start smoke tests when the required local images are
  available; otherwise document the missing image installation commands.
- Runtime smoke tests must include `docker compose ps` and at least one host
  `/readyz` call for gateway, each core service, Qdrant, and optional
  `ai-gateway` when the `ai` profile is enabled.

### 7. Wrong vs Correct

#### Wrong

```text
frontend -> http://localhost:8083/internal/v1/knowledge-bases
deploy/.env.example -> real provider API key
document worker -> file /internal/v1/files without X-Service-Token
seed SQL -> inserts model_profiles before ai-gateway migrations
```

#### Correct

```text
frontend -> gateway http://localhost:8080/api/v1/knowledge-bases
deploy/.env.example -> local placeholder secrets only
document worker -> file /internal/v1/files with DOCUMENT_FILE_SERVICE_TOKEN
seed SQL -> idempotent local/demo data after service migrations
```

---

## Scenario: Environment-Gated Cross-Service Smoke

### 1. Scope / Trigger

- Trigger: adding a smoke test that calls another service or an optional external
  dependency such as AI Gateway, a model provider, object storage, or a parser
  runtime.
- Applies when ordinary unit CI must remain deterministic but operators still
  need an executable proof of the real service client and runtime configuration.

### 2. Signatures

- Name the opt-in gate `<CALLER>_<DEPENDENCY>_SMOKE`; the enabled value is `1`.
- Keep the test beside the real service client and expose one explicit command,
  for example:

```bash
QA_AI_GATEWAY_SMOKE=1 go test ./internal/platform/modelclient \
  -run '^TestAIGatewaySmoke$' -count=1 -v
```

- Reuse the caller's normal endpoint, credential-header, token, profile/model,
  and timeout environment keys instead of inventing parallel smoke-only keys.

### 3. Contracts

- With the gate unset, the test must call `t.Skip` before reading credentials or
  making network requests.
- With the gate enabled, missing required endpoint/profile/token configuration
  must fail with an actionable message that names keys, never values.
- The positive path must use the production service client and assert a minimal
  normalized response.
- Negative probes should stop at the dependency boundary when possible, such as
  invalid service-token or missing-profile checks, to avoid provider cost and
  nondeterministic side effects.
- Missing-resource probes must use a request-scoped unique identifier such as
  `requestID + "-missing-profile"`; do not append a predictable suffix to a
  configured valid resource ID because that name can already exist and reach a
  real provider.
- Generate a request ID for cross-service log correlation. Do not log tokens,
  provider keys, raw downstream bodies, prompts, document text, or vectors.
- Controlled fake providers are preferred. Real providers run only when an
  operator explicitly supplies the gate and credentials.

### 4. Validation & Error Matrix

| Condition | Required behavior |
| --- | --- |
| Smoke gate unset | `SKIP`; ordinary `go test ./...` remains green and offline. |
| Gate set but token/profile/endpoint missing | Fail before network I/O with actionable key names. |
| Dependency authentication rejected | Assert the caller's normalized unauthorized/dependency classification; discard raw body. |
| Selected profile/resource missing | Use a request-scoped missing ID, assert the caller's normalized not-found/dependency classification, and verify a controlled fixture receives no provider call. |
| Provider or dependency unavailable | Fail with request ID and safe configuration hints, not endpoint secrets or provider body. |
| Positive response malformed | Fail on stable normalized fields such as role, finish reason, ID, or result count. |

### 5. Good/Base/Bad Cases

- Good: an opt-in test uses the QA model client, a controlled AI Gateway profile,
  request-ID correlation, positive response assertions, and token/profile
  negative probes whose missing profile ID is scoped to that request.
- Base: a real-provider smoke is documented but skipped until an operator sets
  all required variables.
- Bad: normal CI always calls an external model, a missing-profile probe derives
  `validProfileID + "-missing"`, a test imports another service's `internal`
  packages, or a failure prints raw provider output or a credential.

### 6. Tests Required

- Run the targeted test with the gate unset and assert it reports `SKIP`.
- Exercise the enabled positive and negative paths with a controlled fixture
  before relying on a real provider; assert the missing-resource probe does not
  reach the controlled provider.
- Run the caller service's full unit tests and required builds with the gate
  unset.
- Run `git diff --check` and verify the documented command and link targets.

### 7. Wrong vs Correct

#### Wrong

```go
func TestProviderSmoke(t *testing.T) {
    token := os.Getenv("PROVIDER_API_KEY")
    t.Logf("calling provider with token %s", token)
    missingProfileID := validProfileID + "-missing"
    callProviderDirectly(token)
}
```

#### Correct

```go
func TestAIGatewaySmoke(t *testing.T) {
    if os.Getenv("QA_AI_GATEWAY_SMOKE") != "1" {
        t.Skip("set QA_AI_GATEWAY_SMOKE=1 to run the smoke")
    }
    requestID := newRequestID()
    missingProfileID := requestID + "-missing-profile"
    client := newRuntimeServiceClientFromEnvironment(t)
    result := callWithRequestID(t, client)
    assertNormalizedResult(t, result)
    assertMissingProfileDependencyError(t, client, missingProfileID)
}
```

---

## Forbidden Patterns

- Root-level Go module used to build all microservices together.
- Cross-service imports from `services/<other-service>/internal/...`.
- HTTP handlers that contain business rules, SQL, Qdrant queries, or MinIO object logic.
- Unbounded goroutines without cancellation.
- HTTP clients without timeouts.
- SQL built by concatenating user input.
- Panics for expected business errors.
- Global mutable state for request-scoped data.
- Logging secrets, tokens, raw credentials, or full sensitive payloads.

---

## Required Patterns

- Pass `context.Context` through request, service, repository, and infrastructure calls.
- Use graceful shutdown for HTTP servers.
- Validate environment configuration at startup.
- Keep service dependencies explicit in constructors.
- Keep business workflows in `internal/service/`.
- Keep persistence in `internal/repository/`.
- Use stable API response shapes: project-owned JSON APIs use
  `{ data, requestId }` / `{ error }`; AI Gateway model invocation success
  responses use OpenAI-compatible shapes as documented in
  `docs/services/ai-gateway/api/internal.openapi.yaml`.
- Add or update tests for changed business logic.

---

## Testing Requirements

Use a risk-based test strategy:

| Change Type | Required Coverage |
|-------------|-------------------|
| Pure functions or validators | Unit tests |
| Service business workflows | Unit tests with mocked repositories/clients |
| Repository SQL changes | Integration tests when database test tooling exists |
| HTTP handlers | Handler tests for status code and response shape |
| Cross-service client changes | Contract-style tests or mocked HTTP server tests |
| Migration changes | Migration validation once tooling exists |

Test naming:

- Use `Test<FunctionOrBehavior>`.
- Prefer table-driven tests for validators, mappers, and policy decisions.
- Test expected errors explicitly with `errors.Is` or `errors.As`.

---

## Configuration Quality

- Read configuration from environment variables in `internal/config` using an
  `envconfig`-style structured loader.
- Validate all required variables at startup.
- Keep defaults safe for local development only.
- Do not read environment variables throughout business logic.
- Document required variables in service README or deployment docs.

---

## Code Review Checklist

Reviewers should check:

- [ ] Does the change stay within the correct service boundary?
- [ ] Are HTTP request and response contracts stable?
- [ ] Are errors classified and returned through the standard error shape?
- [ ] Is sensitive data excluded from logs and responses?
- [ ] Are database changes represented by service-owned migrations?
- [ ] Are Redis/Qdrant/MinIO responsibilities owned by the correct service?
- [ ] Are timeouts and context cancellation handled for external calls?
- [ ] Do tests cover the changed behavior?
- [ ] Can the service still run `go test ./...` and `go build ./cmd/server` independently?

---

## Common Mistakes

- Adding shared code before three services actually need the same behavior.
- Testing only handlers while business rules remain untested.
- Treating Docker Compose startup as a substitute for service-level tests.
- Allowing the gateway to accumulate all business logic.
