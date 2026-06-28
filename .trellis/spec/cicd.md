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
- JWT signing secrets,
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
