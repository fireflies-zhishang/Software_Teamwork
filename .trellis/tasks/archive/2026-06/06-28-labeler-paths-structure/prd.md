# Refine labeler paths and repository structure

## Goal

Establish the repository's initial monorepo directory skeleton from README and
CONTRIBUTING guidance, then refine `.github/labeler.json` so pull requests are
auto-labeled by the actual target paths and referenced labels exist in GitHub.

## Requirements

- Preserve the existing uncommitted `.github/labeler.json` account-label change
  that adds `ChenBaoZhuangdayun` to `PrimeTeam`.
- Create a minimal, trackable folder skeleton for the README target layout:
  `apps/web`, the six Go service directories under `services/`, and
  `deploy`.
- Avoid pretending services or the frontend are initialized before tool choices
  and runtime code exist.
- Update `.github/labeler.json` `pathLabels` to use `apps/web/**`,
  `services/**`, service-specific paths, `deploy/**`, `.github/workflows/**`,
  docs/spec paths, and Trellis/agent configuration paths.
- Create any GitHub labels referenced by the new path rules if they are missing.
- Keep maintainer-facing repository settings documentation consistent with the
  expanded label set and README frontend path.

## Acceptance Criteria

- [x] README target directories are present in git via placeholder files.
- [x] `.github/labeler.json` is valid JSON and has a trailing newline.
- [x] Existing account label mappings remain intact.
- [x] Path labels distinguish frontend, backend, each backend service, CI,
      deployment, documentation, and Trellis/agent infrastructure.
- [x] `gh label list` shows all labels referenced by `.github/labeler.json`.
- [x] `docs/repository-settings.md` reflects the current Auto Label label set.

## Definition of Done

- Validate JSON syntax for `.github/labeler.json`.
- Inspect git diff to confirm only intended files changed.
- Run the relevant lightweight checks available for this documentation/config
  change.

## Technical Approach

- Use placeholder `.gitkeep` files for currently uninitialized directories.
- Keep README/CONTRIBUTING semantics as the source of truth for path labels.
- Add labels remotely with `gh label create` using short Chinese descriptions.

## Decision (ADR-lite)

**Context**: README defines the target architecture, but the repository has not
initialized frontend tooling, Go services, or deploy manifests yet.

**Decision**: Create trackable directories with `.gitkeep` only, and reserve
`go.mod`, `package.json`, Dockerfiles, and Compose files for future tasks that
actually initialize those components.

**Consequences**: The folder skeleton is visible to contributors and Auto Label
can match the intended paths, without introducing empty runtime/build artifacts.

## Out of Scope

- Initializing React tooling or Go service modules.
- Adding product CI workflows beyond refining label paths.
- Changing branch protection or PR guard behavior.

## Technical Notes

- README target layout lists `apps/web`, `services/{gateway,auth,file,qa,knowledge,document}`,
  `deploy`, `docs`, `.github/workflows`, and `.trellis`.
- CONTRIBUTING states Auto Label skips labels that do not already exist.
- Existing remote labels before this task: default GitHub labels, `L1nggTeam`,
  `PrimeTeam`, `JerryTeam`, `frontend`, `backend`, and `documentation`.
- Spec update review: no code-spec update is needed because this task does not
  add or change runtime APIs, commands, database schemas, env contracts, or
  cross-layer behavior.
