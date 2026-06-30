# CI/CD Guidelines

> GitHub Actions and Docker Compose delivery rules for this monorepo.

---

## Overview

This repository uses GitHub Actions for pull request checks and deployment
automation. Existing collaboration workflows protect contribution rules. Product
CI/CD should be added around the confirmed monorepo layout:

```text
apps/web/
services/gateway/
services/auth/
services/file/
services/qa/
services/knowledge/
services/document/
services/ai-gateway/
deploy/docker-compose.yml
```

Deployment target: single-machine Docker Compose.

---

## Existing Guard Workflows

These workflows already exist and must remain separate from product build jobs:

| Workflow | File | Purpose |
|----------|------|---------|
| Auto Label | `.github/workflows/auto-label.yml` | Applies team/path labels and syncs PR `blocked` label from linked issues |
| PR Guard | `.github/workflows/pr-guard.yml` | Enforces fork + PR collaboration rules and allowed base branches |
| Commitlint | `.github/workflows/commitlint.yml` | Enforces Conventional Commits on PR commits |

Do not weaken collaboration checks when adding product CI.

## Current CI Status

Use `docs/testing/strategy.md` as the authority for current CI coverage and
required-check candidates. As of the current docs baseline:

- Current CI covers collaboration guardrails, Go service tests/builds, goose
  migration apply, frontend check/build/unit/E2E smoke, Docker/Compose config
  checks, Gateway contract drift, and frontend Gateway API type drift.
- The best required-check candidates are frontend check/build, Go service tests,
  goose migration apply, Docker/Compose config, Gateway contract/API drift, and
  API type drift.
- Full DB integration test jobs and backend cross-service E2E smoke are gaps
  until stable workflows and dependencies land.
- Open PRs, draft issues, and unmerged capabilities must be documented as
  pending/follow-up, not as current `develop` behavior.

When editing workflow specs, keep the current-vs-target distinction explicit:
current CI coverage, PR-before local recommendations, and future gaps are not
interchangeable.

---

## Auto Label Service Path Contract

### 1. Scope / Trigger

Update this contract when changing `.github/labeler.json` service labels,
service directory layout, or service documentation layout.

### 2. Signatures

- Workflow file: `.github/workflows/auto-label.yml`
- Config file: `.github/labeler.json`
- Rule section: `pathLabels[]`
- Rule shape: `{ "paths": string[], "labels": string[] }`

### 3. Contracts

Each service label must cover both implementation and documentation paths:

| Label | Required paths |
|-------|----------------|
| `service:gateway` | `services/gateway/**`, `docs/services/gateway/**` |
| `service:auth` | `services/auth/**`, `docs/services/auth/**` |
| `service:file` | `services/file/**`, `docs/services/file/**` |
| `service:qa` | `services/qa/**`, `docs/services/qa/**` |
| `service:knowledge` | `services/knowledge/**`, `docs/services/knowledge/**` |
| `service:document` | `services/document/**`, `docs/services/document/**` |
| `service:ai-gateway` | `services/ai-gateway/**`, `docs/services/ai-gateway/**` |

All labels referenced by `.github/labeler.json` must exist in the GitHub
repository. The workflow skips missing labels rather than failing the PR, so
local changes must verify remote label existence when adding a new label name.

### 4. Validation & Error Matrix

| Condition | Required handling |
|-----------|-------------------|
| `.github/labeler.json` is invalid JSON | Fix before commit; Auto Label would fail at runtime. |
| Referenced label does not exist remotely | Create the label or remove the rule before PR. |
| Service implementation path changes | Update the matching docs path rule in the same PR. |
| Service documentation path changes | Update the matching implementation path rule in the same PR. |

### 5. Good/Base/Bad Cases

- Good: `docs/services/knowledge/README.md` matches `documentation` and
  `service:knowledge`.
- Base: `services/knowledge/internal/service/service.go` matches `backend` and
  `service:knowledge`.
- Bad: `docs/services/knowledge/README.md` matches only `documentation`.

### 6. Tests Required

- Parse `.github/labeler.json` as JSON.
- Run a local matcher using the same glob conversion as `auto-label.yml` for at
  least one implementation path and one docs path per service label.
- Check all configured labels exist with `gh label list` before adding a new
  label reference.

### 7. Wrong vs Correct

#### Wrong

```json
{
  "paths": ["services/knowledge/**"],
  "labels": ["service:knowledge"]
}
```

#### Correct

```json
{
  "paths": ["services/knowledge/**", "docs/services/knowledge/**"],
  "labels": ["service:knowledge"]
}
```

---

## Auto Label Blocked PR Contract

### 1. Scope / Trigger

Update this contract when changing PR issue-link requirements, task issue
blocked semantics, or `.github/workflows/auto-label.yml` blocked-label logic.

### 2. Signatures

- Workflow file: `.github/workflows/auto-label.yml`
- PR events: `pull_request_target` opened, edited, synchronize, reopened,
  ready_for_review, labeled, unlabeled
- Issue events: `issues` edited, labeled, unlabeled, closed, reopened
- Primary PR link source: GitHub `closingIssuesReferences`
- Fallback PR link syntax: GitHub closing keywords in the `关联 Issue` section,
  for example `Closes #118`, `Fixes #119`, or `Resolves #120`
- Synced label: `blocked`

### 3. Contracts

- The workflow only treats GitHub closing issue references as linked issues.
- A PR receives `blocked` only when it has at least one linked issue and every
  linked issue is blocked.
- A managed task issue with body fields is blocked only when it is open and has
  task body field `状态：Blocked` or `Risk：Blocked`.
- A non-task linked issue without those body fields may use issue label
  `blocked` as the blocked signal.
- Closed issues, pull request pseudo-issues, unreadable issues, and issues
  without blocked state count as not blocked.
- On issue changes, the workflow finds open pull requests that reference that
  issue through timeline cross-references and PR search, then recomputes the PR
  `blocked` label.

### 4. Validation & Error Matrix

| Condition | Required handling |
|-----------|-------------------|
| PR has no linked issues | Remove `blocked` from the PR if present. |
| PR has mixed blocked and not-blocked linked issues | Remove `blocked` from the PR. |
| All linked issues are blocked | Add `blocked` to the PR when the label exists. |
| Linked issue changes from blocked to not blocked | Recompute open linked PRs and remove `blocked` where needed. |
| A linked issue cannot be read | Treat it as not blocked and log a warning. |
| `blocked` label does not exist remotely | Skip adding it and log a warning rather than failing unrelated PR labeling. |

### 5. Good/Base/Bad Cases

- Good: PR body contains `Closes #118` and `Fixes #119`; both issues are open
  with `Risk：Blocked`; PR gets `blocked`.
- Base: PR body contains `Closes #118`; issue #118 is open with
  `状态：In Progress`; PR does not get `blocked`.
- Bad: PR body says `关联 Issue: #118` without a closing keyword and expects
  blocked sync.

### 6. Tests Required

- Parse `.github/workflows/auto-label.yml` as YAML.
- Run `actionlint`.
- Extract the embedded `github-script` body and run `node --check` inside an
  async wrapper.
- Before relying on a new synced label name, verify it exists with
  `gh label list`.

### 7. Wrong vs Correct

#### Wrong

```markdown
## 关联 Issue

- #118
```

#### Correct

```markdown
## 关联 Issue

- Closes #118
```

---

## PR Guard Body Contract

### 1. Scope / Trigger

Update this contract when changing `.github/workflows/pr-guard.yml`,
`.github/pull_request_template.md`, `docs/collaboration/repository-settings.md`,
or any agent-facing PR creation workflow.

### 2. Signatures

- Workflow file: `.github/workflows/pr-guard.yml`
- PR template: `.github/pull_request_template.md`
- Repository rules: `docs/collaboration/repository-settings.md`
- PR command shape:

```bash
gh pr create \
  --repo Sakayori-Iroha-168/Software_Teamwork \
  --base develop \
  --head <owner>:<branch> \
  --title "<conventional-commit-title>" \
  --body-file <filled-chinese-template-file>
```

### 3. Contracts

- PR title must follow Conventional Commit style and must not contain Chinese
  characters.
- PR body must contain Chinese content.
- PR body must preserve the template intent with these filled sections:
  `修改内容`, `关联 Issue`, `验证`, `已知风险`, and `检查项`.
- PR body must not leave template placeholder text in `修改内容`, `关联 Issue`,
  `验证`, or `已知风险`.
- `关联 Issue` must contain a GitHub closing keyword such as `Closes #118`;
  if there is no issue, it must explain the reason instead of writing only
  `无`.
- Agents must read both `CONTRIBUTING.md` and
  `docs/collaboration/repository-settings.md` before opening or editing a PR;
  `CONTRIBUTING.md` covers branch/base/head/commit policy, while
  `repository-settings.md` contains the PR Guard title/body language contract.

### 4. Validation & Error Matrix

| Condition | Required handling |
| --- | --- |
| PR title contains Chinese characters | Rewrite the title to an English Conventional Commit title. |
| PR body is handwritten in English only | Replace it with the filled Chinese PR template before reporting completion. |
| PR body omits `Closes #<issue>` for an issue-backed task | Add the closing keyword in `关联 Issue`. |
| PR body keeps template placeholders | Replace placeholders with concrete Chinese content. |
| Agent read only `CONTRIBUTING.md` before creating PR | Stop and read `docs/collaboration/repository-settings.md` plus `.github/pull_request_template.md`; then re-check the PR body. |

### 5. Good/Base/Bad Cases

- Good: title is `test(frontend): add critical flow coverage`; body uses the
  Chinese template sections, lists concrete verification commands, includes
  `Closes #117`, and records known risks in Chinese.
- Base: title is English Conventional Commit style; body is mostly Chinese and
  includes all required template sections and issue linkage.
- Bad: body is manually written in English because the agent focused only on
  base/head/commitlint/`Closes #117` and skipped the PR Guard language rules.

### 6. Tests Required

Before reporting a PR as ready:

```bash
gh pr view <PR_NUMBER> --repo Sakayori-Iroha-168/Software_Teamwork \
  --json title,body,baseRefName,headRefName,headRepositoryOwner
gh pr checks <PR_NUMBER> --repo Sakayori-Iroha-168/Software_Teamwork
```

Review the returned JSON manually for:

- `baseRefName == "develop"`.
- `headRepositoryOwner.login` is the developer fork owner.
- Title has no Chinese characters.
- Body contains Chinese text and the required template sections.
- Body contains the correct closing keyword or a concrete no-issue reason.

### 7. Wrong vs Correct

#### Wrong

```markdown
## Summary

- Add tests.

## Verification

- bun run --cwd apps/web check

Closes #117
```

This is wrong because the body is English-only and does not use the required
Chinese PR template sections.

#### Correct

```markdown
## 修改内容

- 新增前端关键流程测试。

## 关联 Issue

- Closes #117

## 验证

- `bun run --cwd apps/web check`：通过。

## 已知风险

- 无。
```

---

## Target Product Workflows

Product workflow files:

| Workflow | Suggested File | Trigger |
|----------|----------------|---------|
| Frontend CI | `.github/workflows/frontend.yml` | `apps/web/**` |
| Go Services CI | `.github/workflows/go-services.yml` | `services/**` |
| Docker / Deploy Checks | `.github/workflows/docker-deploy-checks.yml` | service image inputs, service Compose files, `deploy/**` |
| Deploy | `.github/workflows/deploy.yml` | protected branch or manual dispatch |

Use path filters so unrelated documentation or service changes do not run every
job. A workflow may still run a cheap detection job to decide which service jobs
are needed.

## Scenario: Path-Derived Matrix Inputs

### 1. Scope / Trigger

- Trigger: a GitHub Actions workflow derives a job matrix from changed file
  paths, pull request metadata, issue text, or other contributor-controlled
  input.
- Applies to `actions/github-script` detection jobs and downstream shell steps
  that consume matrix values.

### 2. Signatures

- Detection output: JSON arrays written with `core.setOutput`, for example
  `dockerfiles` or `compose-files`.
- Matrix consumption: `${{ fromJSON(needs.detect.outputs.<name>) }}`.
- Shell consumption: matrix values passed through step `env`, then read as
  shell variables.

### 3. Contracts

- Detection jobs must whitelist path-derived matrix entries against a known set
  or a repo-owned manifest before writing outputs.
- PR changed-file detection must consider both `filename` and
  `previous_filename` from `pulls.listFiles` so renamed files exercise checks
  for the old and new affected paths.
- Pattern checks alone are insufficient for contributor-controlled file names.
- Shell steps must not interpolate `${{ matrix.* }}` directly inside `run`
  scripts. Pass the value through `env` and quote the shell variable.

### 4. Validation & Error Matrix

| Condition | Required handling |
| --- | --- |
| Changed path matches a broad glob but is not in the known set | Exclude it from the matrix output. |
| PR file entry has `previous_filename` | Evaluate both old and new paths through the same whitelist/routing rules. |
| Workflow file changes | Expand to the repo-owned known set, not arbitrary matching paths. |
| Matrix value is consumed by a shell step | Use `env:` and quote the shell variable in `run`. |
| A path contains quotes, command separators, spaces, or shell metacharacters | It must not reach shell execution unless it is an explicit known path. |

### 5. Good/Base/Bad Cases

- Good: `services/qa/Dockerfile.host` is in the known Dockerfile set and builds
  through a quoted `DOCKERFILE` env variable.
- Good: renaming a file from `services/auth/**` to `services/qa/**` selects both
  affected services, because old and new paths are evaluated.
- Base: a service `.dockerignore` change maps to that service's known
  Dockerfiles.
- Bad: `services/qa/Dockerfile";echo pwned #` matches a broad workflow trigger
  and is interpolated directly into a `run` script.

### 6. Tests Required

- Parse changed-file detection scripts with `node --check`.
- Add a local detection regression for valid known paths, workflow-file changes,
  and malicious path strings containing shell metacharacters.
- Run `actionlint` and `git diff --check`.

### 7. Wrong vs Correct

#### Wrong

```yaml
run: |
  dockerfile="${{ matrix.dockerfile }}"
  docker build --file "$dockerfile" "$(dirname "$dockerfile")"
```

#### Correct

```yaml
env:
  DOCKERFILE: ${{ matrix.dockerfile }}
run: |
  dockerfile="$DOCKERFILE"
  docker build --file "$dockerfile" "$(dirname "$dockerfile")"
```

## Scenario: Gateway Active API Contract Workflow

### 1. Scope / Trigger

- Trigger: changing the public gateway OpenAPI, gateway active owner map,
  frontend OpenAPI generation command, or the gateway contract verifier.
- Applies to `docs/services/gateway/api/openapi.yaml`,
  `docs/services/gateway/docs/active-api-owner-map.md`, `apps/web/package.json`,
  `package.json`, `scripts/verify_gateway_active_api.py`, `scripts/tests/**`,
  and `.github/workflows/gateway-contract.yml`.

### 2. Signatures

Local commands:

```bash
python scripts/verify_gateway_active_api.py
bun run check:gateway-contract
python -m unittest scripts.tests.test_verify_gateway_active_api
```

Workflow file:

```text
.github/workflows/gateway-contract.yml
```

### 3. Contracts

The verifier is the CI gate for these executable contracts:

- Active `/api/v1/**` operations must include `operationId`, non-empty `tags`,
  `x-owner-service`, effective `security`, at least one `2XX` response, and at
  least one `4XX` response.
- `/healthz` and `/readyz` are operational exceptions owned by `gateway` and may
  use `security: []`.
- Stable active public paths must not use action-style segments such as
  `login`, `logout`, `register`, `download`, `search`, `generate`, `export`,
  `retry`, or `revoke`.
- `x-missing-contracts.placeholderOperations` must not overlap active OpenAPI
  paths.
- `apps/web` API type generation must use
  `../../docs/services/gateway/api/openapi.yaml`.
- `docs/services/gateway/docs/active-api-owner-map.md` must match the active
  operations, owner summary, and missing contract placeholders derived from
  OpenAPI.

### 4. Validation & Error Matrix

| Condition | Required handling |
| --- | --- |
| OpenAPI metadata is missing on an active `/api/v1/**` operation | Verifier exits non-zero and names the method/path and missing field. |
| Owner map table or summary drifts from OpenAPI | Verifier exits non-zero and reports owner-map drift. |
| Missing-contract placeholder overlaps an active operation | Verifier exits non-zero and names the overlapping placeholder. |
| Frontend generation source changes away from gateway OpenAPI | Verifier exits non-zero and prints the expected source path. |
| PyYAML is unavailable in CI | Workflow installs `pyyaml` before running verifier commands. |

### 5. Good/Base/Bad Cases

- Good: update OpenAPI and owner map together, then run
  `bun run check:gateway-contract`.
- Base: update only verifier tests or workflow wiring; CI still runs the
  verifier unit tests and real-contract check.
- Bad: add `GET /api/v1/search` or an active operation without a `4XX` response
  and rely on manual review to catch it.

### 6. Tests Required

- Unit tests must cover missing required metadata, missing `4XX`, action-style
  path segments, missing-contract overlap, frontend generation source drift,
  and owner-map drift.
- Local verification before PR must run:

```bash
python -m unittest scripts.tests.test_verify_gateway_active_api
python scripts/verify_gateway_active_api.py
```

### 7. Wrong vs Correct

#### Wrong

```text
Change docs/services/gateway/api/openapi.yaml
Skip docs/services/gateway/docs/active-api-owner-map.md
Open PR without running the verifier
```

#### Correct

```text
Change docs/services/gateway/api/openapi.yaml
Update docs/services/gateway/docs/active-api-owner-map.md
Run bun run check:gateway-contract
Let .github/workflows/gateway-contract.yml enforce the same gate in PR
```

---

## Frontend CI Target

Frontend CI is a landed workflow. It runs only when frontend files, root
frontend dependency files, or the frontend workflow file change.

Target steps:

```bash
bun install --frozen-lockfile
bun run --cwd apps/web check
bun run --cwd apps/web build
bun run --cwd apps/web test:unit
bun run --cwd apps/web test:e2e
```

Rules:

- Keep CI commands behind package scripts.
- Do not encode a specific build tool in workflow logic unless the frontend tool is selected and documented.
- Cache package-manager dependencies using lockfile-based keys.
- Fail if the lockfile and package manifest are inconsistent.
- Add Vitest, React Testing Library, and Playwright only after their scripts and
  dependencies exist in `apps/web/package.json`; do not mark them required from
  this spec alone.

---

## Go Services CI

Each Go service owns an independent `go.mod`. CI must test and build changed
services independently.

Service paths:

```text
services/gateway/
services/auth/
services/file/
services/qa/
services/knowledge/
services/document/
services/ai-gateway/
```

Required service-local checks:

```bash
go test ./...
go build ./cmd/server
```

Rules:

- Run checks from the changed service directory.
- Do not rely on a root `go.mod`.
- Cache Go modules per service or with keys that include service `go.sum`.
- If shared code is introduced later, update path filters so dependent services run.
- Use a matrix job when multiple services changed.

Example matrix dimensions:

```yaml
service:
  - gateway
  - auth
  - file
  - qa
  - knowledge
  - document
  - ai-gateway
```

---

## Docker Build

Every runtime service should have its own Dockerfile:

```text
apps/web/Dockerfile
services/gateway/Dockerfile
services/auth/Dockerfile
services/file/Dockerfile
services/qa/Dockerfile
services/knowledge/Dockerfile
services/parser/Dockerfile
services/document/Dockerfile
services/ai-gateway/Dockerfile
```

Rules:

- Use multi-stage builds for Go services.
- Produce small runtime images.
- Build images for changed services on PRs.
- Treat service source, module/lock files, Dockerfile, and `.dockerignore`
  changes as image inputs for the service's source-backed Dockerfiles.
- Push images only from trusted branches or manual release workflows.
- Tag images with commit SHA and, when applicable, branch or release tags.
- Never build images with secrets baked into layers.

---

## Docker Compose Deployment

Deployment uses `deploy/docker-compose.yml` on a single machine.

Compose must include:

- frontend,
- gateway,
- auth,
- file,
- qa,
- knowledge,
- document,
- ai-gateway,
- postgres,
- redis,
- qdrant,
- minio.

Deployment rules:

- Store runtime secrets outside the repository.
- Use `.env.example` for required variable names only.
- Use named volumes for PostgreSQL, Qdrant, MinIO, and Redis when persistence is required.
- Expose only frontend and gateway publicly by default.
- Keep internal services on the Compose network.
- Add health checks for infrastructure and services before relying on automated deployment.

---

## Secrets and Environments

GitHub Actions secrets should be scoped by environment:

- `staging` for test deployment,
- `production` for release deployment if production is later enabled.

Never commit:

- database passwords,
- session, service-token, or signing secrets,
- MinIO access keys or secret keys,
- API keys,
- SSH private keys,
- cloud credentials.

Deployment workflows should use GitHub Environments and required reviewers for
production-like targets.

---

## Rollback

Every deployment workflow must have a documented rollback path before production
use.

Minimum rollback strategy:

1. Keep previous image tags available.
2. Keep the previous Compose file or release revision identifiable.
3. Re-deploy the last known-good image tags.
4. Do not run irreversible migrations automatically without an explicit release decision.

---

## Checks Before Merge

Current required checks are defined by repository branch protection and
`docs/testing/strategy.md`; do not infer additional required checks from target
workflow sections above. For PRs:

- PR Guard passes.
- Commitlint passes.
- Current product CI passes for touched areas when the workflow exists.
- Frontend changes are covered by Frontend CI; local `bun run --cwd apps/web check`,
  `bun run --cwd apps/web build`, and targeted tests remain useful PR-before
  evidence.
- Docker/Compose config checks are covered for affected service image inputs,
  existing buildable Dockerfiles, and service Compose files; image push, full DB
  integration jobs, and cross-service smoke remain future gates until stable
  workflows land.
- Documentation changes update README/specs when architecture, commands,
  contracts, or implementation status change.

---

## Common Mistakes

- Running all service builds for every small frontend change.
- Assuming a root Go module exists.
- Pushing Docker images from untrusted pull request contexts.
- Committing production `.env` files.
- Exposing internal services directly to the public network.
- Adding deployment automation before rollback and secret handling are defined.
