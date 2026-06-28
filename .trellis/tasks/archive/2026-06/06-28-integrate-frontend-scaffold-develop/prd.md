# Integrate frontend scaffold onto develop

## Goal

Base the work on the current `upstream/develop` line, bring back L1ngg's previously authored frontend scaffold and frontend workflow/spec guidance, then commit and push the result to the fork's `develop` branch (`origin/develop`).

## What I already know

- `upstream/develop` is the current team development baseline.
- `upstream/frontend-dev` is stale and only contains early repository bootstrap commits.
- Local `frontend-dev` contains 8 commits ahead of `upstream/frontend-dev`, including frontend docs/spec work, app scaffold, and frontend workflow skills.
- A direct merge from local `frontend-dev` into `develop` conflicts in frontend spec files and would also delete many files that exist on `develop`.

## Requirements

- Start from the latest `upstream/develop` content.
- Preserve `develop`-only collaboration, CI, README, service placeholder, and Trellis updates.
- Bring in frontend-related work from local `frontend-dev`:
  - `apps/web/` scaffold.
  - root frontend tooling files such as `package.json`, `bun.lock`, `.prettierrc`, `.prettierignore` when relevant.
  - `docs/frontend-workflow.md`.
  - project frontend workflow skill copies under `.agents/skills/`, `.claude/skills/`, and `.cursor/skills/`.
  - frontend spec updates under `.trellis/spec/frontend/`, merged carefully against `develop`.
  - `AGENTS.md` frontend notes if missing from `develop`.
- Do not intentionally carry over unrelated deletions from local `frontend-dev`.
- Commit the integrated result locally and push it to `origin develop`.

## Acceptance Criteria

- [ ] Current working branch is based on `upstream/develop`.
- [ ] Frontend scaffold exists under `apps/web/`.
- [ ] Frontend workflow docs/skills/spec guidance are present and consistent with `develop`.
- [ ] Git working tree has one integration commit for the selected changes.
- [ ] `origin/develop` is updated with the integration commit.
- [ ] Relevant frontend checks are run, or skipped checks are reported with reasons.

## Out of Scope

- No new frontend features beyond the existing scaffold.
- No direct push to `upstream`.
- No broad rewrite of backend, CI, or collaboration docs beyond resolving integration conflicts.

## Technical Notes

- Direct merge conflict files observed earlier:
  - `.gitignore`
  - `.trellis/spec/frontend/component-guidelines.md`
  - `.trellis/spec/frontend/directory-structure.md`
  - `.trellis/spec/frontend/hook-guidelines.md`
  - `.trellis/spec/frontend/index.md`
  - `.trellis/spec/frontend/quality-guidelines.md`
  - `.trellis/spec/frontend/state-management.md`
  - `.trellis/spec/frontend/type-safety.md`
- Candidate source commits from local `frontend-dev`:
  - `304f327 docs: document frontend technology stack guidelines`
  - `0bae5cf docs: define frontend collaboration workflow`
  - `bde8998 docs: translate frontend workflow to Chinese`
  - `531108a feat(frontend): scaffold web app shell`
  - `43303db docs: add frontend workflow skill`
