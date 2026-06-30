# PR #334 Deleted Report And Target Scope Review

## Context

The latest Codex PR review for PR #334 reported two contract and lifecycle
issues in Document report job creation.

## Requirements

1. Creating any job for a deleted report must fail before a `report_jobs` row is
   inserted. This applies to report generation jobs, section regeneration, and
   report file creation. The service must reject both `ReportStatusDeleted` and
   non-nil `DeletedAt`.
2. Public OpenAPI contracts must not advertise `target.scope` values that the
   service rejects. The implemented submit-time scopes are `report` and
   `section`; `outline` and `file` should not be enum values until supported.

## Acceptance Criteria

- `JobService.CreateJob` rejects deleted reports with `conflict` before job
  creation, attempt creation, and enqueue.
- Regression tests cover deleted-report rejection for all supported job types.
- Service-local, Document docs, and Gateway public OpenAPI `target.scope`
  enums match the implemented `report | section` behavior.
- `cd services/document && go test ./... -count=1` passes.
- `cd services/document && go build ./cmd/server` passes.
- `git diff --check` passes.
