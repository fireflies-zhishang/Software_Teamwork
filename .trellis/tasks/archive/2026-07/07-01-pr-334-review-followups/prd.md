# PR #334 Review Follow-ups

## Context

PR #334 adds Document Service report generation orchestration. The latest Codex
review identified three medium-severity edge cases in the report generation
lifecycle that should be fixed before the PR is merged.

## Requirements

1. Content or section generation that ends in `partial_succeeded` must not make
   the whole report look fully failed when usable generated content exists.
   Public report status has no partial enum, so job and attempt status remain
   the detailed source while report status should reflect the closest usable
   generation stage.
2. AI content generation must preserve manually edited sections by default.
   `ManualEdited=true` sections should be skipped unless
   `requestPayload.options.preserveManualEdits` is explicitly `false`.
3. The current AI generation scope is limited to `summer_peak_inspection`.
   Content generation, content regeneration, and section regeneration must
   enforce the same report type capability check as outline generation.

## Acceptance Criteria

- Repository lifecycle mapping keeps partial content/section generation reports
  in a usable generated state and sets `generated_at`; failed and canceled jobs
  still map to `failed`.
- Service tests prove manually edited sections are skipped by default, can be
  overwritten only with an explicit opt-out, and unsupported report types are
  rejected before AI Gateway calls.
- Existing progress semantics are preserved: skipped manual sections count
  toward job progress without creating a new section version.
- `cd services/document && go test ./... -count=1` passes.
- `cd services/document && go build ./cmd/server` passes.
- `git diff --check` passes.
