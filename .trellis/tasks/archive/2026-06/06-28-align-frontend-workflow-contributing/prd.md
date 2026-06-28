# Align frontend workflow with contributing rules

## Goal

Make repository collaboration guidance treat `CONTRIBUTING.md` as the authoritative branch/PR policy, while preserving frontend-specific details about `apps/web`, Bun commands, checks, and agent skill routing.

## What I already know

- `CONTRIBUTING.md` says routine work should use fork + PR into the main repository `develop` branch.
- The recently merged frontend workflow documents still describe `upstream/frontend-dev` as the default frontend integration branch.
- The user wants the branch strategy to follow the contributor's `CONTRIBUTING.md`, and either delete the frontend-specific branch policy or merge it into that policy.
- The user also wants an explanation of how the frontend specs from `develop` and L1ngg's previous frontend spec work were adapted during the merge.

## Requirements

- Keep `CONTRIBUTING.md` as the single source of truth for branch and PR target policy.
- Update frontend workflow docs/skills so they point to `CONTRIBUTING.md` for branch and PR rules.
- Preserve frontend-specific operational guidance:
  - `apps/web/src/` source location.
  - Bun commands from repository root.
  - frontend lint/typecheck/build expectations.
  - frontend implementation spec references.
- Remove or reword `upstream/frontend-dev` as the default PR target.
- Add a concise explanation of the recent frontend spec merge strategy.

## Acceptance Criteria

- [ ] `docs/frontend-workflow.md` no longer conflicts with `CONTRIBUTING.md` about the default branch/PR target.
- [ ] `AGENTS.md` no longer says the frontend integration branch is `upstream/frontend-dev`.
- [ ] project frontend workflow skill copies no longer instruct default PRs to `frontend-dev`.
- [ ] A documented explanation exists for how the frontend specs were adapted during the merge.
- [ ] Markdown/config checks pass where relevant.

## Out of Scope

- Do not change application code.
- Do not alter `CONTRIBUTING.md` unless strictly necessary.
- Do not change remote branches or push until edits are verified.
