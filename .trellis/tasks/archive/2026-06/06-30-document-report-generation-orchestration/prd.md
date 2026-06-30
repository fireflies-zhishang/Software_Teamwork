# C-005 Document Report Generation Orchestration

## Goal

Implement the Document service orchestration for report outline and section content generation so at least one fixed report type can complete a full outline-to-content loop through the AI Gateway and persist generated results.

## What I Already Know

* Issue: https://github.com/Sakayori-Iroha-168/Software_Teamwork/issues/101
* Title: `[C-005] Document 大纲与正文生成编排`
* Module: `document`
* Suggested branch: `PrimeTeam/feat/report-generation-orchestration`
* Depends on issues: #98, #99, #100, #84, #120.
* Authoritative docs:
  * `docs/services/document/README.md`
  * `docs/services/document/docs/requirements.md`
  * `docs/services/ai-gateway/README.md`
  * `docs/services/knowledge/README.md`
* Document service must use AI Gateway chat profiles for model calls.
* Document service must not store provider base URLs, API keys, or call providers directly.
* Material retrieval must go through the Knowledge service, not direct Qdrant access.

## Requirements

* Generate a report outline from report type, template structure, topic, context, and materials.
* Generate section body/table content section by section.
* Persist successful generation results to `ReportOutline`, `ReportSection`, and `ReportSectionVersion`.
* Preserve already generated sections when later generation fails.
* Set job status to `partial_succeeded` when partial output exists, or `failed` when no usable output exists.
* Emit generation events and update job progress during outline and section generation.
* Sanitize logs, events, and errors so they do not expose full prompts, provider raw errors, internal URLs, object keys, or API keys.

## Acceptance Criteria

* [ ] At least one fixed report type can complete outline generation and section content generation.
* [ ] On section-level failure, generated sections remain persisted and the job status reflects `partial_succeeded` or `failed`.
* [ ] Generation process writes progress and events.
* [ ] Logs/events/errors do not include full prompt, provider raw error, internal URL, object key, or API key.
* [ ] Document service calls AI Gateway instead of calling providers directly.
* [ ] Document service calls Knowledge service for retrieval instead of Qdrant directly.
* [ ] Generated results are written to `ReportOutline`, `ReportSection`, and `ReportSectionVersion`.
* [ ] Service-local tests pass with `go test ./...` under `services/document`.

## Definition of Done

* Tests are added or updated before production behavior changes.
* Service-local Go tests pass.
* Backend spec and service boundary rules are followed.
* Trellis task is archived and journaled after implementation.
* PR description lists completed scope, verification commands, risks, and `Closes #101`.

## Out of Scope

* Frontend pages.
* MCP tool implementation.
* Direct provider configuration or provider credential management.
* Direct Qdrant access from Document service.

## Technical Notes

* Current working branch is based on latest `upstream/develop` as of 2026-06-30.
* This task is cross-layer inside `services/document`: service orchestration, cross-service clients, database persistence, worker/job progress, events, and tests.
* Must inspect existing Document service implementation before finalizing the approach.
* Current worker behavior: `report_file_creation` already calls `ReportFileExecutor`; `outline_generation`, `outline_regeneration`, `content_generation`, `content_regeneration`, and `section_regeneration` are placeholders that only mark jobs succeeded.
* Selected implementation approach:
  * Add a Document-owned report generation executor under `internal/service`.
  * Inject that executor into `internal/worker` for non-file generation job types while preserving the existing file creation executor path.
  * Keep model calls behind a service-owned AI Gateway chat client using `/internal/v1/chat/completions`; do not import another service's `internal` packages or call providers directly.
  * Use existing repository methods for reports, templates, outlines, sections, section versions, jobs, attempts, events, and operation logs; add narrowly scoped repository methods only when progress/final state cannot be expressed safely with existing methods.
  * Do not hold PostgreSQL transactions while calling AI Gateway or Knowledge.
  * Treat Knowledge retrieval as an optional dependency for material context. If this slice cannot safely bind material references to Knowledge queries from existing persisted fields, keep the generation pipeline ready for retrieved material context but do not bypass Knowledge or Qdrant boundaries.
* Minimal fixed report type for the first closed loop: `summer_peak_inspection`.
* Prompt, provider raw error bodies, internal URLs, object keys, file refs, and tokens must never be written to logs, events, operation logs, or returned worker errors.

## TDD Plan

1. Worker red test: non-file generation jobs call an injected generation executor with request id, job id, attempt id, user id, and job type instead of completing the placeholder path.
2. Worker red test: executor partial result marks job/attempt partial-succeeded without overwriting successful output.
3. AI Gateway chat client red tests: sends `POST /internal/v1/chat/completions`, propagates `X-Caller-Service: document`, service token, request id, and user id; normalizes non-2xx failures without exposing raw response bodies.
4. Generation service red test: outline generation uses report + template structure + AI JSON output and persists a current `ReportOutline`.
5. Generation service red test: content generation loops sections, persists each successful `ReportSectionVersion`, updates the current section, and returns partial success when a later section fails.
6. Safety red test: sanitized events/errors do not include prompt text, provider raw errors, internal URLs, object keys, or API keys.

## Implementation Notes From Code Inspection

* `services/document/internal/worker/worker.go` is the main routing point.
* `services/document/internal/service/models.go` already defines `JobStatusPartialSucceeded`, `ReportOutline`, `ReportSection`, and `ReportSectionVersion`.
* `services/document/internal/repository/reports.go` already provides `CreateReportOutline`, `ListReportOutlines`, `CreateReportSection`, `UpdateReportSection`, `CreateReportSectionVersion`, and `ListReportSectionVersions`.
* `services/document/internal/repository/postgres.go` already provides job status helpers and `CreateReportEvent`, but the public `ReportEvent` service model currently stores only id/report/job/type/message/createdAt, so event payload additions are out of scope unless required by tests.
* `services/document/internal/platform/aigateway/profile_client.go` is the local pattern for internal AI Gateway headers and sanitized dependency errors.
