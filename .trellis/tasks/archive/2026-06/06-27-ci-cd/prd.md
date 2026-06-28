# PRD: Tech Stack Specs & CI/CD Guidelines

## Goal

Produce comprehensive developer documentation for the monorepo knowledge management system (电力行业知识管理系统).

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go for all backend microservices |
| Frontend | React + TypeScript; build tool intentionally unspecified for now |
| Primary DB | PostgreSQL |
| Cache / Queue | Redis |
| Vector DB | Qdrant (self-hosted Docker) |
| Object Storage | MinIO |
| CI/CD | GitHub Actions |
| Deploy | Docker Compose (single machine) |
| Repo | Monorepo |

## Deliverables

1. `README.md` — project overview, tech stack, quick-start, repo structure
2. `.trellis/spec/backend/` — fill all stub spec files (directory-structure, database-guidelines, error-handling, quality-guidelines, logging-guidelines)
3. `.trellis/spec/frontend/` — fill all stub spec files (directory-structure, component-guidelines, hook-guidelines, state-management, type-safety, quality-guidelines)
4. `.trellis/spec/cicd.md` — GitHub Actions workflows spec (lint/test/build for Go + TS/React, Docker Compose deploy)

## MVP Scope

- README: project purpose, architecture overview, quick-start with Docker Compose, monorepo layout
- Backend: Go standard project layout, PostgreSQL via sqlx/pgx, Qdrant Go client, structured error handling
- Frontend: React + TypeScript component patterns, API layer, state management (React Query)
- CI/CD: path-filtered jobs, lint + test + build per service, single-machine Docker Compose deploy

## Status Check

Status: documentation deliverables completed; quality verification and commit steps still pending.

Initial findings from 2026-06-28 repository inspection before implementation:

- `README.md` contained placeholder setup instructions and only a minimal Trellis/collaboration overview.
- Backend and frontend spec files under `.trellis/spec/backend/` and `.trellis/spec/frontend/` contained placeholder sections.
- Existing GitHub Actions covered collaboration guardrails only: auto label, PR guard, and commitlint.
- Product CI/CD workflows for Go, React/TypeScript, Docker image build, and Docker Compose deploy were not present.
- `.trellis/spec/cicd.md` was not present.

Implementation progress on 2026-06-28:

- `README.md` was rewritten in Chinese with the confirmed architecture, service map, target layout, local development flow, and CI/CD summary.
- `.trellis/spec/backend/` was filled with active Go backend engineering guidelines.
- `.trellis/spec/frontend/` was filled with active React + TypeScript frontend engineering guidelines.
- `.trellis/spec/cicd.md` was added with GitHub Actions and Docker Compose delivery rules.
- Product CI workflow files are documented but not created yet; this task's original deliverable is a CI/CD specification, not runtime CI implementation.

Quality verification on 2026-06-28:

- Placeholder/template scan passed for README, Trellis specs, and this PRD.
- `git diff --check` passed.
- No Go, frontend, or Docker runtime checks were run because this task changed documentation/spec files only and the target service directories do not exist yet.

## README-First Plan

1. Confirm architecture assumptions that affect README content:
   - technology stack details,
   - monorepo directory layout,
   - microservice boundaries,
   - local Docker Compose topology,
   - CI/CD stages and deployment target.
2. Update `README.md` first with a clear project overview, architecture, service map, quick start, repository layout, and workflow summary.
3. Fill backend/frontend spec files so they match the confirmed README architecture.
4. Add `.trellis/spec/cicd.md` documenting GitHub Actions and Docker Compose deployment rules.
5. Add or update GitHub Actions workflows only after documentation decisions are confirmed.
6. Run documentation and workflow validation checks, then complete Trellis quality/spec/commit steps.

## Open Questions

- None for the documentation deliverables covered by this task.

## Confirmed Architecture Decisions

### Microservice Split

The README should describe the system as a gateway-centered microservice architecture:

- `frontend` calls the backend through `gateway service`.
- `gateway service` routes requests to backend domain services.
- Backend services:
  - `auth service` for identity, authentication, authorization, and session/token concerns.
  - `file service` for file upload, storage orchestration, and file metadata.
  - `智能问答` for AI Q&A over indexed knowledge.
  - `知识库` for knowledge ingestion, indexing, metadata, and retrieval coordination.
  - `文档生成` for document/report generation workflows.
- Infrastructure services:
  - `postgres` for relational application data.
  - `redis` for cache, sessions, queues, or short-lived coordination.
  - `qdrant` for vector search.
  - `minio` for object storage.

Initial data-flow assumptions from the architecture diagram:

- `智能问答` depends on `知识问答检索` and the `知识库`.
- `知识库` stores vector data in `qdrant`.
- `file service` stores object payloads in `minio` and feeds knowledge/document workflows.
- `gateway service` coordinates request routing and may access `postgres`/`redis` for cross-cutting API concerns.

### Backend Language

All backend microservices use Go, including AI-adjacent services such as `智能问答`, `知识库`, and `文档生成`.

### Frontend Stack Boundary

The README should document the frontend as React + TypeScript without committing to a specific build tool such as Vite or Next.js yet. Quick-start and CI examples should use placeholder package scripts such as `npm run lint`, `npm run test`, and `npm run build` until the frontend build tool is selected.

### Monorepo Directory Layout

Use a service-grouped monorepo layout in the README and specs:

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

This layout keeps the frontend and backend services visually separate, makes microservice ownership clear, and gives CI/CD workflows straightforward path filters.

### Go Module Strategy

Each Go microservice owns an independent `go.mod` file under its service directory:

```text
services/gateway/go.mod
services/auth/go.mod
services/file/go.mod
services/qa/go.mod
services/knowledge/go.mod
services/document/go.mod
```

README and CI/CD examples should treat each service as an independently linted, tested, built, and containerized unit. Shared code should not be assumed until a dedicated shared-library decision is made.

### Service Communication

Use HTTP/REST for service-to-service communication in the initial architecture. The `gateway service` exposes the public backend API to `frontend` and routes to internal Go services over HTTP. README and CI/CD examples should avoid gRPC/protobuf assumptions for now.

### Documentation Language

Use Chinese for `README.md` so the project overview, architecture, and quick-start are easy for the team to read. Keep `.trellis/spec/**` in English to match the existing spec templates and keep AI-facing engineering conventions consistent.
