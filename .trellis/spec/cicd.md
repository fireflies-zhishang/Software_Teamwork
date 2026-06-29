# CI/CD Guidelines

> GitHub Actions and Docker Compose delivery rules for this monorepo.

---

## Overview

This repository uses GitHub Actions for pull request checks and deployment
automation. Existing collaboration workflows protect contribution rules. Product
CI/CD should be added around the confirmed monorepo layout:

```text
apps/web/
services/gateway/
services/auth/
services/file/
services/qa/
services/knowledge/
services/document/
services/ai-gateway/
deploy/docker-compose.yml
```

Deployment target: single-machine Docker Compose.

---

## Existing Guard Workflows

These workflows already exist and must remain separate from product build jobs:

| Workflow | File | Purpose |
|----------|------|---------|
| Auto Label | `.github/workflows/auto-label.yml` | Applies team/path labels when configured labels exist |
| PR Guard | `.github/workflows/pr-guard.yml` | Enforces fork + PR collaboration rules and allowed base branches |
| Commitlint | `.github/workflows/commitlint.yml` | Enforces Conventional Commits on PR commits |

Do not weaken collaboration checks when adding product CI.

---

## Auto Label Service Path Contract

### 1. Scope / Trigger

Update this contract when changing `.github/labeler.json` service labels,
service directory layout, or service documentation layout.

### 2. Signatures

- Workflow file: `.github/workflows/auto-label.yml`
- Config file: `.github/labeler.json`
- Rule section: `pathLabels[]`
- Rule shape: `{ "paths": string[], "labels": string[] }`

### 3. Contracts

Each service label must cover both implementation and documentation paths:

| Label | Required paths |
|-------|----------------|
| `service:gateway` | `services/gateway/**`, `docs/services/gateway/**` |
| `service:auth` | `services/auth/**`, `docs/services/auth/**` |
| `service:file` | `services/file/**`, `docs/services/file/**` |
| `service:qa` | `services/qa/**`, `docs/services/qa/**` |
| `service:knowledge` | `services/knowledge/**`, `docs/services/knowledge/**` |
| `service:document` | `services/document/**`, `docs/services/document/**` |
| `service:ai-gateway` | `services/ai-gateway/**`, `docs/services/ai-gateway/**` |

All labels referenced by `.github/labeler.json` must exist in the GitHub
repository. The workflow skips missing labels rather than failing the PR, so
local changes must verify remote label existence when adding a new label name.

### 4. Validation & Error Matrix

| Condition | Required handling |
|-----------|-------------------|
| `.github/labeler.json` is invalid JSON | Fix before commit; Auto Label would fail at runtime. |
| Referenced label does not exist remotely | Create the label or remove the rule before PR. |
| Service implementation path changes | Update the matching docs path rule in the same PR. |
| Service documentation path changes | Update the matching implementation path rule in the same PR. |

### 5. Good/Base/Bad Cases

- Good: `docs/services/knowledge/README.md` matches `documentation` and
  `service:knowledge`.
- Base: `services/knowledge/internal/service/service.go` matches `backend` and
  `service:knowledge`.
- Bad: `docs/services/knowledge/README.md` matches only `documentation`.

### 6. Tests Required

- Parse `.github/labeler.json` as JSON.
- Run a local matcher using the same glob conversion as `auto-label.yml` for at
  least one implementation path and one docs path per service label.
- Check all configured labels exist with `gh label list` before adding a new
  label reference.

### 7. Wrong vs Correct

#### Wrong

```json
{
  "paths": ["services/knowledge/**"],
  "labels": ["service:knowledge"]
}
```

#### Correct

```json
{
  "paths": ["services/knowledge/**", "docs/services/knowledge/**"],
  "labels": ["service:knowledge"]
}
```

---

## Required Product Workflows

Recommended workflow files:

| Workflow | Suggested File | Trigger |
|----------|----------------|---------|
| Frontend CI | `.github/workflows/frontend-ci.yml` | `apps/web/**` |
| Go Services CI | `.github/workflows/go-services-ci.yml` | `services/**` |
| Docker Build | `.github/workflows/docker-build.yml` | service Dockerfiles, service code, `deploy/**` |
| Deploy | `.github/workflows/deploy.yml` | protected branch or manual dispatch |

Use path filters so unrelated documentation or service changes do not run every
job. A workflow may still run a cheap detection job to decide which service jobs
are needed.

## Scenario: Gateway Active API Contract Workflow

### 1. Scope / Trigger

- Trigger: changing the public gateway OpenAPI, gateway active owner map,
  frontend OpenAPI generation command, or the gateway contract verifier.
- Applies to `docs/services/gateway/api/openapi.yaml`,
  `docs/services/gateway/docs/active-api-owner-map.md`, `apps/web/package.json`,
  `package.json`, `scripts/verify_gateway_active_api.py`, `scripts/tests/**`,
  and `.github/workflows/gateway-contract.yml`.

### 2. Signatures

Local commands:

```bash
python scripts/verify_gateway_active_api.py
bun run check:gateway-contract
python -m unittest scripts.tests.test_verify_gateway_active_api
```

Workflow file:

```text
.github/workflows/gateway-contract.yml
```

### 3. Contracts

The verifier is the CI gate for these executable contracts:

- Active `/api/v1/**` operations must include `operationId`, non-empty `tags`,
  `x-owner-service`, effective `security`, at least one `2XX` response, and at
  least one `4XX` response.
- `/healthz` and `/readyz` are operational exceptions owned by `gateway` and may
  use `security: []`.
- Stable active public paths must not use action-style segments such as
  `login`, `logout`, `register`, `download`, `search`, `generate`, `export`,
  `retry`, or `revoke`.
- `x-missing-contracts.placeholderOperations` must not overlap active OpenAPI
  paths.
- `apps/web` API type generation must use
  `../../docs/services/gateway/api/openapi.yaml`.
- `docs/services/gateway/docs/active-api-owner-map.md` must match the active
  operations, owner summary, and missing contract placeholders derived from
  OpenAPI.

### 4. Validation & Error Matrix

| Condition | Required handling |
| --- | --- |
| OpenAPI metadata is missing on an active `/api/v1/**` operation | Verifier exits non-zero and names the method/path and missing field. |
| Owner map table or summary drifts from OpenAPI | Verifier exits non-zero and reports owner-map drift. |
| Missing-contract placeholder overlaps an active operation | Verifier exits non-zero and names the overlapping placeholder. |
| Frontend generation source changes away from gateway OpenAPI | Verifier exits non-zero and prints the expected source path. |
| PyYAML is unavailable in CI | Workflow installs `pyyaml` before running verifier commands. |

### 5. Good/Base/Bad Cases

- Good: update OpenAPI and owner map together, then run
  `bun run check:gateway-contract`.
- Base: update only verifier tests or workflow wiring; CI still runs the
  verifier unit tests and real-contract check.
- Bad: add `GET /api/v1/search` or an active operation without a `4XX` response
  and rely on manual review to catch it.

### 6. Tests Required

- Unit tests must cover missing required metadata, missing `4XX`, action-style
  path segments, missing-contract overlap, frontend generation source drift,
  and owner-map drift.
- Local verification before PR must run:

```bash
python -m unittest scripts.tests.test_verify_gateway_active_api
python scripts/verify_gateway_active_api.py
```

### 7. Wrong vs Correct

#### Wrong

```text
Change docs/services/gateway/api/openapi.yaml
Skip docs/services/gateway/docs/active-api-owner-map.md
Open PR without running the verifier
```

#### Correct

```text
Change docs/services/gateway/api/openapi.yaml
Update docs/services/gateway/docs/active-api-owner-map.md
Run bun run check:gateway-contract
Let .github/workflows/gateway-contract.yml enforce the same gate in PR
```

---

## Frontend CI

Frontend CI should run only when frontend files or frontend-related workflow
files change.

Required steps once `apps/web/package.json` exists:

```bash
cd apps/web
bun install --frozen-lockfile
bun run lint
bun run test
bun run build
```

Rules:

- Keep CI commands behind package scripts.
- Do not encode a specific build tool in workflow logic unless the frontend tool is selected and documented.
- Cache package-manager dependencies using lockfile-based keys.
- Fail if the lockfile and package manifest are inconsistent.

---

## Go Services CI

Each Go service owns an independent `go.mod`. CI must test and build changed
services independently.

Service paths:

```text
services/gateway/
services/auth/
services/file/
services/qa/
services/knowledge/
services/document/
services/ai-gateway/
```

Required service-local checks:

```bash
go test ./...
go build ./cmd/server
```

Rules:

- Run checks from the changed service directory.
- Do not rely on a root `go.mod`.
- Cache Go modules per service or with keys that include service `go.sum`.
- If shared code is introduced later, update path filters so dependent services run.
- Use a matrix job when multiple services changed.

Example matrix dimensions:

```yaml
service:
  - gateway
  - auth
  - file
  - qa
  - knowledge
  - document
  - ai-gateway
```

---

## Docker Build

Every runtime service should have its own Dockerfile:

```text
apps/web/Dockerfile
services/gateway/Dockerfile
services/auth/Dockerfile
services/file/Dockerfile
services/qa/Dockerfile
services/knowledge/Dockerfile
services/document/Dockerfile
services/ai-gateway/Dockerfile
```

Rules:

- Use multi-stage builds for Go services.
- Produce small runtime images.
- Build images for changed services on PRs.
- Push images only from trusted branches or manual release workflows.
- Tag images with commit SHA and, when applicable, branch or release tags.
- Never build images with secrets baked into layers.

---

## Docker Compose Deployment

Deployment uses `deploy/docker-compose.yml` on a single machine.

Compose must include:

- frontend,
- gateway,
- auth,
- file,
- qa,
- knowledge,
- document,
- ai-gateway,
- postgres,
- redis,
- qdrant,
- minio.

Deployment rules:

- Store runtime secrets outside the repository.
- Use `.env.example` for required variable names only.
- Use named volumes for PostgreSQL, Qdrant, MinIO, and Redis when persistence is required.
- Expose only frontend and gateway publicly by default.
- Keep internal services on the Compose network.
- Add health checks for infrastructure and services before relying on automated deployment.

---

## Secrets and Environments

GitHub Actions secrets should be scoped by environment:

- `staging` for test deployment,
- `production` for release deployment if production is later enabled.

Never commit:

- database passwords,
- session, service-token, or signing secrets,
- MinIO access keys or secret keys,
- API keys,
- SSH private keys,
- cloud credentials.

Deployment workflows should use GitHub Environments and required reviewers for
production-like targets.

---

## Rollback

Every deployment workflow must have a documented rollback path before production
use.

Minimum rollback strategy:

1. Keep previous image tags available.
2. Keep the previous Compose file or release revision identifiable.
3. Re-deploy the last known-good image tags.
4. Do not run irreversible migrations automatically without an explicit release decision.

---

## Required Checks Before Merge

For PRs:

- PR Guard passes.
- Commitlint passes.
- Frontend CI passes when `apps/web/**` changes.
- Go service CI passes for each changed service.
- Docker build passes when Dockerfiles or deploy definitions change.
- Documentation changes update README/specs when architecture or commands change.

---

## Common Mistakes

- Running all service builds for every small frontend change.
- Assuming a root Go module exists.
- Pushing Docker images from untrusted pull request contexts.
- Committing production `.env` files.
- Exposing internal services directly to the public network.
- Adding deployment automation before rollback and secret handling are defined.
