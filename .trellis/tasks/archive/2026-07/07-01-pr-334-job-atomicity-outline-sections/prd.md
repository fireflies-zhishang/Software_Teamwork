# PR #334 Job Atomicity And Outline Section Review

## Context

Codex PR review rounds for PR #334 reported remaining Document report
generation risks. The earlier head `7c7422138b83` covered job initial-state
atomicity and current-outline content generation. The latest head
`f717f53ebc9c` adds target-scope consistency and outline/skeleton atomicity.

## Requirements

1. `JobService.CreateJob` must not leave orphan pending jobs when any step after
   `CreateReportJob` fails before enqueue. This includes concurrent report
   deletion between the initial report access check and generation status update.
   The durable state must either be created atomically for an accepted job, or
   reliably compensated so callers and operators do not see an untracked pending
   job with no attempt and no queue task.
2. Content generation and regeneration must generate only the sections that
   belong to the current outline structure. Historical sections left behind by
   earlier outline generations or regenerations must not be included in the
   content-generation target set or progress total.
3. Section-scoped job targets must be accepted only for
   `section_regeneration`. `content_generation`, `content_regeneration`,
   outline jobs, and file creation jobs must reject `target.scope=section` or a
   submitted `sectionId` instead of persisting a section target that execution
   later ignores.
4. AI outline creation and the section skeletons derived from that outline must
   be written atomically. If any skeleton insert fails, the new current outline
   and any partial skeleton rows must roll back so future content generation
   cannot continue from an incomplete current outline.

## Acceptance Criteria

- A failure in report generation status update after job insertion does not
  leave a pending job without an attempt or queue task.
- Regression coverage exercises the post-job-insert failure path and verifies
  the persisted job is not orphaned.
- Content generation target selection filters report sections to the current
  outline's section IDs.
- Regression coverage exercises old and current outline sections and verifies
  only current outline sections are generated and counted.
- Regression coverage rejects section targets for non-`section_regeneration`
  job types before job, attempt, report file, or queue side effects.
- Regression coverage simulates a skeleton creation failure and verifies the
  new current outline and partial skeletons are rolled back.
- `cd services/document && go test ./... -count=1` passes.
- `cd services/document && go build ./cmd/server` passes.
- `git diff --check` passes.
