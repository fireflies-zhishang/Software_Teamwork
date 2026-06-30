# Document Service

`document` owns report types, templates, materials, reports, outlines, sections,
report jobs, job attempts, events, generated file metadata, statistics, and
operation logs.

The current implementation provides the service/data baseline, implemented
report type/template/material/report/outline/section APIs, the report
job/attempt/event state machine, report file creation, report settings, report
statistics, and operation logs. DOCX export currently uses the in-process Go
`SimpleDOCXGenerator`. Basic AI outline and section-content orchestration is
implemented for the fixed `summer_peak_inspection` report type through AI
Gateway chat calls, with optional Knowledge retrieval context when configured
and requested. MCP tools and the Pandoc/LibreOffice rich DOCX conversion
toolchain remain future work.

## Local Configuration

Required environment variables:

| Variable | Example | Purpose |
| --- | --- | --- |
| `DOCUMENT_DATABASE_URL` | `postgres://document_app:document_app_dev@localhost:5435/document_system?sslmode=disable` | PostgreSQL connection string. |
| `DOCUMENT_REDIS_ADDR` | `localhost:6380` | Redis/asynq queue endpoint. Redis is not the durable job state authority. |
| `DOCUMENT_FILE_SERVICE_URL` | `http://localhost:8082` | Internal file service base URL for later template/material/report-file bytes. |
| `DOCUMENT_FILE_SERVICE_TOKEN` | empty | Service token sent to File Service when request context has no `X-Service-Token`. Falls back to `INTERNAL_SERVICE_TOKEN` when empty. |
| `DOCUMENT_AI_GATEWAY_URL` | `http://localhost:8086` | Internal AI Gateway base URL for report settings validation and report generation chat calls. |
| `DOCUMENT_AI_GATEWAY_PROFILE_ID` | `default-chat` | AI Gateway chat profile reference used by report settings/default generation. |

Optional variables:

| Variable | Default | Purpose |
| --- | --- | --- |
| `DOCUMENT_HTTP_ADDR` | `:8085` | HTTP listen address. |
| `DOCUMENT_AI_GATEWAY_SERVICE_TOKEN` | empty | Service token sent to AI Gateway profile validation APIs. Falls back to `INTERNAL_SERVICE_TOKEN` when empty. |
| `DOCUMENT_KNOWLEDGE_SERVICE_URL` | empty | Optional internal Knowledge service base URL. When empty, report generation skips Knowledge retrieval and uses only report/template/request context. |
| `DOCUMENT_KNOWLEDGE_SERVICE_TOKEN` | empty | Optional service token sent to Knowledge. Falls back to `INTERNAL_SERVICE_TOKEN` when empty. Required when `DOCUMENT_KNOWLEDGE_SERVICE_URL` is set. |
| `INTERNAL_SERVICE_TOKEN` | empty | Shared internal service token fallback for local/dev deployments. |
| `DOCUMENT_PANDOC_PATH` | `pandoc` | Reserved path for a future Pandoc-backed rich DOCX worker. The current Dockerfile does not install this CLI. |
| `DOCUMENT_LIBREOFFICE_PATH` | `soffice` | Reserved path for a future LibreOffice-backed conversion worker. The current Dockerfile does not install this CLI. |
| `DOCUMENT_SHUTDOWN_TIMEOUT` | `10s` | Graceful shutdown timeout. |

## Run

Docker Compose starts PostgreSQL, Redis, applies goose migrations, and then
starts the document service. Default values are embedded in `docker-compose.yml`;
copy `.env.example` to `.env` only when local ports or downstream service URLs
need to be changed. Compose uses `DOCUMENT_COMPOSE_FILE_SERVICE_URL` and
`DOCUMENT_COMPOSE_AI_GATEWAY_URL` for container-network downstream overrides, so
host-run `localhost` examples do not leak into the container by accident:

```powershell
# Optional: Copy-Item .env.example .env
docker compose up --build
```

For a host process pointed at the same Compose dependencies:

```powershell
$env:DOCUMENT_DATABASE_URL = "postgres://document_app:document_app_dev@localhost:5435/document_system?sslmode=disable"
$env:DOCUMENT_REDIS_ADDR = "localhost:6380"
$env:DOCUMENT_FILE_SERVICE_URL = "http://localhost:8082"
$env:DOCUMENT_AI_GATEWAY_URL = "http://localhost:8086"
$env:DOCUMENT_AI_GATEWAY_PROFILE_ID = "default-chat"
go run ./cmd/server
```

Operational routes:

```text
GET /healthz
GET /readyz
```

Both JSON responses use the project envelope: `{ "data": ..., "requestId": "..." }`.
The service-local operational contract is documented in [`api/openapi.yaml`](api/openapi.yaml).

## Active Report Route Coverage

Gateway exposes these document-owned report routes under `/api/v1`. The service
local paths below omit that prefix. Implemented routes call the document service
layer. Job routes persist state and drive the worker state machine; file export
jobs currently produce basic DOCX packages through the in-process Go generator.
Generation jobs for `summer_peak_inspection` call AI Gateway for outline and
section content and persist the generated outline, sections, section versions,
progress, and events. The richer Pandoc/LibreOffice toolchain remains a future
worker dependency and is not installed in the current service image.

| Method | Local path | Operation ID | Status |
| --- | --- | --- | --- |
| `GET` | `/report-types` | `listReportTypes` | Implemented |
| `GET` | `/report-templates` | `listReportTemplates` | Implemented |
| `POST` | `/report-templates` | `createReportTemplate` | Implemented |
| `GET` | `/report-templates/{reportTemplateId}` | `getReportTemplate` | Implemented |
| `PATCH` | `/report-templates/{reportTemplateId}` | `updateReportTemplate` | Implemented |
| `DELETE` | `/report-templates/{reportTemplateId}` | `deleteReportTemplate` | Implemented |
| `GET` | `/report-templates/{reportTemplateId}/structure` | `getReportTemplateStructure` | Implemented |
| `PATCH` | `/report-templates/{reportTemplateId}/structure` | `updateReportTemplateStructure` | Implemented |
| `GET` | `/report-materials` | `listReportMaterials` | Implemented |
| `POST` | `/report-materials` | `createReportMaterial` | Implemented |
| `GET` | `/report-materials/{materialId}` | `getReportMaterial` | Implemented |
| `DELETE` | `/report-materials/{materialId}` | `deleteReportMaterial` | Implemented |
| `GET` | `/reports` | `listReports` | Implemented |
| `POST` | `/reports` | `createReport` | Implemented |
| `GET` | `/reports/{reportId}` | `getReport` | Implemented |
| `PATCH` | `/reports/{reportId}` | `updateReport` | Implemented |
| `DELETE` | `/reports/{reportId}` | `deleteReport` | Implemented |
| `GET` | `/reports/{reportId}/outlines` | `listReportOutlines` | Implemented |
| `POST` | `/reports/{reportId}/outlines` | `createReportOutline` | Implemented |
| `GET` | `/reports/{reportId}/outlines/{outlineId}` | `getReportOutline` | Implemented |
| `PATCH` | `/reports/{reportId}/outlines/{outlineId}` | `updateReportOutline` | Implemented |
| `DELETE` | `/reports/{reportId}/outlines/{outlineId}/sections/{sectionId}` | `deleteReportOutlineSection` | Implemented |
| `GET` | `/reports/{reportId}/sections` | `listReportSections` | Implemented |
| `POST` | `/reports/{reportId}/sections` | `createReportSection` | Implemented; single create or batch save |
| `GET` | `/reports/{reportId}/sections/{sectionId}` | `getReportSection` | Implemented |
| `PATCH` | `/reports/{reportId}/sections/{sectionId}` | `updateReportSection` | Implemented |
| `GET` | `/reports/{reportId}/sections/{sectionId}/versions` | `listReportSectionVersions` | Implemented |
| `POST` | `/reports/{reportId}/sections/{sectionId}/versions` | `createReportSectionVersion` | Implemented |
| `GET` | `/reports/{reportId}/jobs` | `listReportJobs` | Implemented |
| `POST` | `/reports/{reportId}/jobs` | `createReportJob` | Implemented; enqueues AI outline/content or file worker task |
| `GET` | `/report-jobs/{jobId}` | `getReportJob` | Implemented |
| `GET` | `/report-jobs/{jobId}/attempts` | `listReportJobAttempts` | Implemented |
| `POST` | `/report-jobs/{jobId}/attempts` | `createReportJobAttempt` | Implemented; retry claim/enqueue |
| `GET` | `/reports/{reportId}/events` | `listReportEvents` | Implemented |
| `GET` | `/report-files` | `listReportFiles` | Implemented |
| `POST` | `/report-files` | `createReportFile` | Implemented; enqueues basic DOCX export |
| `GET` | `/report-files/{reportFileId}` | `getReportFile` | Implemented |
| `GET` | `/report-files/{reportFileId}/content` | `getReportFileContent` | Implemented; requires succeeded file content |
| `GET` | `/report-statistics/overview` | `getReportStatisticsOverview` | Implemented |
| `GET` | `/report-statistics/daily` | `listDailyReportStatistics` | Implemented |
| `GET` | `/report-operation-logs` | `listReportOperationLogs` | Implemented |
| `GET` | `/report-settings` | `getReportSettings` | Implemented |
| `PATCH` | `/report-settings` | `updateReportSettings` | Implemented; admin/super_admin only |

## Migrations

Migration files live in `migrations/` and are applied with the project-pinned `goose@v3.27.1` command.

```powershell
go run github.com/pressly/goose/v3/cmd/goose@v3.27.1 -dir migrations postgres "$env:DOCUMENT_DATABASE_URL" up
```

The first migration creates the report generation tables and seeds the initial
report types:

- `summer_peak_inspection`
- `coal_inventory_audit`

`report_jobs`, `report_job_attempts`, and `report_events` are PostgreSQL
business-state tables. Redis/asynq should only carry queue payloads and task
execution coordination.

The second migration creates the singleton report settings row and adds indexes
used by active report, statistics, job status, and operation-log queries.

The third migration seeds the first-slice report defaults. It inserts the two
report types if they are missing, adds enabled placeholder template metadata for
local development, and fills missing `report_settings` keys for:

- `defaultTemplates.summer_peak_inspection`
- `defaultTemplates.coal_inventory_audit`
- `file.defaultFormat=docx`
- `file.defaultNumberingMode=global`
- `file.defaultStyleProfileId=first-slice-default-docx`

The placeholder templates intentionally have no stored file reference. Their
description and structure/style JSON mark the formal DOCX template dependency as
`needs_decision` and point back to
`services/document/migrations/0003_seed_initial_report_defaults.sql` as the
runnable import path. Re-running the seed keeps existing report type rows,
template rows, and user-provided settings values. The default settings do not
store provider API keys, provider URLs, object storage details, or internal
file references.

## SQLC

SQL queries live under `internal/repository/queries/`, and generated code lives
under `internal/repository/sqlc/`.

```bash
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate
```

## Tests

```powershell
go test ./...
go build ./cmd/server
```

Repository integration tests are skipped unless `DOCUMENT_TEST_DATABASE_URL` is
set:

```powershell
$env:DOCUMENT_TEST_DATABASE_URL = "postgres://document:document@localhost:5432/document_test?sslmode=disable"
go test ./internal/repository
```
