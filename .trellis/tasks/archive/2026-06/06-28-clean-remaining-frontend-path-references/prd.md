# Clean remaining frontend path references

## Goal

Remove remaining stale frontend path references from active specs and archived task PRDs so repository searches consistently point to the current frontend root, `apps/web`.

## What I already know

- `.trellis/spec/cicd.md` still uses the old frontend app path in active CI/CD guidance.
- Archived task PRDs still include the old path in historical requirement text and technical notes.
- `.github/labeler.json`, `README.md`, and `docs/repository-settings.md` now use `apps/web`.

## Requirements

- Update active CI/CD spec references to `apps/web`.
- Update archived PRD references when they describe repository paths, so future search does not surface the old app path as a current instruction.
- Keep historical intent intact while avoiding stale path literals.
- Confirm no `apps/frontend` references remain.

## Acceptance Criteria

- [ ] `rg -n "apps/frontend" .` returns no matches.
- [ ] CI/CD frontend examples use Bun and `apps/web`.
- [ ] `git diff --check` passes.

## Out of Scope

- No CI workflow implementation.
- No application code changes.
- No remote branch or label changes.
