# Migrate Auth to pgx v5

## Goal

Migrate `services/auth` PostgreSQL access from `github.com/jackc/pgx/v4` to `github.com/jackc/pgx/v5` in line with the repository technology baseline, while preserving existing Auth behavior for users, roles, permissions, sessions, token hashes, and migrations.

## Source

- GitHub issue: https://github.com/Sakayori-Iroha-168/Software_Teamwork/issues/244
- Issue title: `[A-20260630-03] Migrate Auth to pgx v5`
- Status checked via GitHub API on 2026-06-30: open
- Assignee: `Fisherman6a`
- Labels: `enhancement`, `L1nggTeam`, `backend`, `service:auth`
- Comment context: claimed by `Fisherman6a`; no extra technical constraints.

## What I Already Know

- `docs/architecture/technology-decisions.md` sets the PostgreSQL client target baseline to `pgx/v5@v5.7.6`.
- At task start, `services/auth` depended on `github.com/jackc/pgx/v4@v4.18.3`.
- The migration should touch Auth repository, connection pool, transaction, tests, and generated/sqlc integration only where needed.
- Auth public and internal API semantics must not change.
- HTTP handlers must not directly import pgx or sqlc generated packages; database access should remain inside the repository adapter layer.
- No ORM should be introduced.

## Requirements

- Update `services/auth/go.mod` so Auth uses `github.com/jackc/pgx/v5@v5.7.6` or the exact version required by the current technology baseline.
- Remove Auth's dependency on `github.com/jackc/pgx/v4`.
- Migrate Auth repository, transaction, connection pool, migration smoke, and tests to pgx v5 import paths and APIs.
- Confirm `services/auth/sqlc.yaml`, SQL queries, migrations, and repository adapter boundaries remain compatible.
- Update Auth implementation documentation so it no longer presents pgx v4 as an outstanding or reusable baseline.
- Keep changes scoped to Auth pgx migration and directly related docs.

## Acceptance Criteria

- [x] `services/auth` no longer depends on `github.com/jackc/pgx/v4`.
- [x] `services/auth` uses `github.com/jackc/pgx/v5` at the repository technology baseline version.
- [x] `cd services/auth && go test ./...` passes.
- [x] Auth migration smoke or equivalent PostgreSQL integration validation is recorded; if it cannot run locally, the PR notes why and what risk remains.
- [x] HTTP handlers do not directly import pgx or sqlc generated packages.
- [x] Documentation no longer describes Auth pgx/v4 usage as a reusable baseline.

## Out of Scope

- Introducing an ORM or changing Auth's persistence model.
- Changing Auth public or internal HTTP API semantics.
- Refactoring unrelated services or shared infrastructure.
- Migrating non-Auth services unless a shared root lockfile or workspace metadata requires a mechanical dependency update.

## Technical Notes

- Relevant docs:
  - `docs/architecture/technology-decisions.md`
  - `docs/services/auth/docs/implementation.md`
  - `.trellis/spec/backend/index.md`
  - `.trellis/spec/backend/database-guidelines.md`
  - `.trellis/spec/backend/directory-structure.md`
  - `.trellis/spec/backend/quality-guidelines.md`
  - `.trellis/spec/backend/error-handling.md`
- Expected verification:
  - `cd services/auth && go test ./...`
  - dependency scan proving no Auth pgx v4 import/module remains
  - repository boundary scan for handler imports

## Verification Notes

- `cd services/auth && go test ./...` passed in a `golang:1.25` container on 2026-06-30 after the pgx v5 migration.
- Auth migration smoke passed on 2026-06-30 against a temporary Docker `postgres:16-alpine` database:
  - command shape: `go run github.com/pressly/goose/v3/cmd/goose@v3.27.1 -dir migrations postgres "$DATABASE_URL" up`
  - applied `0001_create_auth_core_tables.sql`
  - applied `0002_seed_auth_roles_permissions.sql`
  - final goose version: `2`
