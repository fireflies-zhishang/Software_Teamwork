---
name: frontend-workflow
description: "Use when working on this project's frontend app, apps/web, fork/upstream PR flow, Bun frontend commands, frontend lint/build checks, or frontend collaboration rules."
---

# Frontend Workflow

Use this project-local skill before frontend implementation, review, branch/PR guidance, or CI/Lint setup for `apps/web`.

## Read First

1. `CONTRIBUTING.md` — authoritative repository branch, PR, commit, and merge policy.
2. `docs/frontend-workflow.md` — frontend-specific workflow supplement.
3. `.trellis/spec/frontend/index.md` — agent-facing frontend implementation standards.
4. `.trellis/spec/frontend/quality-guidelines.md` — required checks, Bun command form, and review checklist.
5. `.trellis/spec/frontend/directory-structure.md` — `apps/web/src/` layout and module boundaries.

## Fixed Project Rules

- Frontend source lives under `apps/web/src/`; do not create repository-root `src/` for frontend code.
- Branch, PR, commit, and merge policy comes from `CONTRIBUTING.md`; do not override it in frontend-specific docs or commands.
- Current frontend work follows the repository fork + PR flow into `upstream/develop`; do not target `frontend-dev` unless `CONTRIBUTING.md` is updated to enable it.
- Use Bun for frontend dependencies and scripts.
- From the repository root, run frontend scripts as `bun run --cwd apps/web <script>`.

## Required Checks

For non-trivial frontend changes, run and report:

```bash
bun run --cwd apps/web check
bun run --cwd apps/web build
git diff --check
```

If a check is not run, report it with the reason and remaining risk.

## Trellis Fit

This skill is for on-demand frontend workflow context. Do not replace the Trellis workflow-state hooks with this content. Hooks should stay focused on active-task and phase context; frontend rules belong here and in `.trellis/spec/frontend/`.
