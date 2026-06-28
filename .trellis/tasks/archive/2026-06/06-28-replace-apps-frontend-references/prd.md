# Replace remaining apps/frontend references

## Goal

Remove stale `apps/frontend` references and any remaining old frontend branch wording so the repository documentation and UI copy consistently use `apps/web` and the current `CONTRIBUTING.md`-based workflow.

## What I already know

- `rg` found stale `apps/frontend` references in `README.md` and `docs/repository-settings.md`.
- `rg` also found old `frontend-dev` wording in `apps/web/src/layouts/app-layout.tsx` and `apps/web/src/pages/dashboard/page.tsx`.
- The repository's frontend source root is `apps/web/`.
- Branch and PR policy should follow `CONTRIBUTING.md`, not `frontend-dev`-specific wording.

## Requirements

- Replace `apps/frontend` references with `apps/web` where they describe the current frontend app.
- Update remaining `frontend-dev` copy in UI text to match the current workflow or make it generic.
- Leave historical references alone only when they are clearly describing past state or other repositories.
- Keep changes scoped to documentation and UI copy; do not change behavior.

## Acceptance Criteria

- [ ] `rg -n \"apps/frontend\" .` returns no active documentation/UI references for this repository's current frontend app.
- [ ] Old `frontend-dev` wording in the frontend shell/dashboard copy is updated to the current workflow language.
- [ ] `git diff --check` passes.

## Out of Scope

- No application logic changes.
- No dependency or build changes.
- No branch or remote changes.

## Technical Notes

- Files flagged by search: `README.md`, `docs/repository-settings.md`, `apps/web/src/layouts/app-layout.tsx`, `apps/web/src/pages/dashboard/page.tsx`.
