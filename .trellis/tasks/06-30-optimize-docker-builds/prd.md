# Optimize Docker Build Speed And Image Footprint

## Goal

Make the repository's Docker builds runnable first, then faster, smaller, and lighter for local and CI use. The work should address slow upstream pulls/downloads through configurable mirrors, reduce repeated dependency downloads with BuildKit caches, keep Go service images small, and make the heavier Parser image more deliberate without forcing every Dockerfile into one shared base.

## What I Already Know

- The user suspects slow Docker builds are caused by Docker/image/package sources and wants mirror support such as Aliyun/Tsinghua where appropriate.
- The user prioritizes Docker work in this order: runnable builds first, build speed second, small image size third, low memory use fourth, and low storage use fifth.
- The user does not require every Dockerfile to become one identical template, but wants the current Alpine/Debian split to be intentional and documented.
- The user observed `goproxy.cn` checksum database mirror failures during `go install github.com/pressly/goose/v3/cmd/goose@v3.27.1`, causing `migrate-file` to fail and the Parser build to be cancelled by Compose dependency failure.
- The user clarified that Docker build environment issues should be documented for other contributors, including the fastest safe setup and diagnostics for bad mirrors.
- The user clarified that the scope is all Docker-related surfaces, not only Go image/build mirrors. Compose infrastructure images, Parser Python/Debian sources, QA host Dockerfile, Docker daemon mirrors, and docs must be checked together.
- Upstream was updated before implementation. `upstream/develop` fast-forwarded to `a31b628`; PR #340 (`fix(docker): align local image versions`) is merged and includes commit `8455530`.
- PR #340 aligned local image versions but did not add BuildKit cache mounts, configurable image registry prefixes, Python/uv mirror controls, Parser multi-stage slimming, or per-service `.dockerignore` coverage.
- At task start, Go service Dockerfiles used `golang:1.25-alpine` build stages, `alpine:3.22` runtime stages, and `GOPROXY=https://goproxy.cn,direct`. The implementation now targets safer defaults with explicit domestic overrides.
- Current Parser Dockerfile uses `python:3.12-slim` and installs PaddleOCR extras in the same runtime stage.
- Existing docs already define pinned image tags: `postgres:16-alpine`, `redis:7-alpine`, `qdrant/qdrant:v1.18.2`, `golang:1.25-alpine`, `alpine:3.22`, MinIO server/client tags, and Parser's Debian/Python runtime exception.

## Requirements

- Keep Go services independently buildable with service-local Dockerfiles.
- Preserve the current Go baseline of `golang:1.25-alpine` build stage and `alpine:3.22` runtime stage unless there is a measured reason to change it.
- Add configurable image registry prefix support for Dockerfile `FROM` lines so domestic mirrors can be used without hardcoding one vendor into repository defaults.
- Keep Compose infrastructure images pinned by default while allowing full-image overrides for local/enterprise registries.
- Default build settings must favor correctness/runnability over speed. A fast mirror may be documented as an opt-in override only if module checksum verification remains enabled and the failure mode is documented.
- Keep dependency mirrors configurable by build arguments/environment:
  - Go: default should not rely on a single third-party mirror with known sumdb issues; domestic speedups must be explicit build args.
  - Alpine apk: optional repository mirror override, disabled by default.
  - Debian apt and Python/uv for Parser: optional mirror/index overrides, disabled by default.
- Add BuildKit cache mounts for Go module cache/build cache and Parser package manager caches where practical.
- Add or update `.dockerignore` files for source-backed Docker build contexts so generated files, local caches, binaries, logs, and VCS metadata are excluded.
- Optimize Parser image shape while keeping Debian slim if required by PaddleOCR/native runtime dependencies.
- Update deploy/runbook/testing/architecture docs so the Docker baseline explains:
  - why Go services use Alpine while Parser uses Debian slim,
  - how to use mirrors,
  - how to use BuildKit/build cache,
  - how to diagnose broken Docker daemon registry mirrors and Go sumdb mirror paths,
  - which checks prove Docker changes are valid.
- Update CI Docker build invocation when needed to keep BuildKit-only syntax working.
- Do not introduce production secrets, unpinned `latest` tags, or runtime-only assumptions into build layers.

## Acceptance Criteria

- [ ] Go service Dockerfiles support configurable registry prefix and build args without changing default public image names.
- [ ] Go service Dockerfiles use BuildKit cache mounts for module/build caches.
- [ ] Parser Dockerfile supports configurable registry prefix plus apt/uv/Python index mirrors and avoids carrying unnecessary build cache into runtime.
- [ ] Compose infrastructure images support explicit pinned defaults plus override variables for PostgreSQL, Redis, Qdrant, MinIO server, and MinIO client.
- [ ] QA host-binary Dockerfile supports configurable registry prefix/postgres version without changing the pinned default.
- [ ] Source-backed Docker contexts have `.dockerignore` coverage for local artifacts.
- [ ] `docker compose` config validation passes for root default profile, root `ai` profile, QA compose, QA DB compose, and Document compose.
- [ ] Changed Dockerfiles build at least far enough to validate Dockerfile syntax and build-arg wiring; if large Parser dependencies make a full build impractical, record the exact skipped runtime validation.
- [ ] `git diff --check` passes.
- [ ] Documentation points to the new mirror/cache/environment workflow and remains aligned with pinned image baseline.

## Definition Of Done

- Dockerfiles, workflow, and docs are updated together.
- Checks are run and recorded in the final answer.
- Remaining image-size or full-runtime validation risks are explicitly called out.
- Trellis task remains traceable with research notes under `research/`.

## Technical Approach

Use a conservative two-family Docker baseline:

- Go services: keep Alpine runtime for small images, static Go binaries, and existing health probes. Add BuildKit cache mounts and optional mirror/build args to reduce repeated downloads, but keep checksum verification enabled.
- Parser: keep Debian slim because PaddleOCR/Paddle/native OCR dependencies are more compatible with Debian wheels and system packages than Alpine musl. Convert to a builder/runtime pattern if feasible, keep cache directories out of final layers, and expose mirror args for apt and uv/Python indexes.

Mirror behavior should be opt-in or overridable. Repository defaults stay portable: public images and official package indexes remain the documented neutral baseline, while local users can set build args or Docker daemon registry mirrors for domestic acceleration.

## Decision (ADR-lite)

**Context**: Builds are slow and Dockerfiles are similar but not identical. Go services and Parser have different runtime dependency profiles. Hardcoding one domestic mirror would speed up one environment but make CI and international contributors more fragile. `goproxy.cn` can also proxy the Go checksum database and has been observed to fail during goose dependency verification, so mirror selection can break "can run" even when it improves download speed.

**Decision**: Standardize build mechanics and override points, not all images. Keep Go on Alpine, Parser on Debian slim, keep Compose infrastructure pinned, add configurable mirror/image arguments, add BuildKit cache mounts, add `.dockerignore` coverage, and document Docker daemon registry mirrors separately from Dockerfile package mirrors and Compose image overrides. Treat mirrors as opt-in acceleration: default builds should preserve checksum verification and avoid known broken sumdb mirror paths. Avoid an external Dockerfile frontend image requirement because a broken daemon mirror can fail before the repository Dockerfile is parsed.

**Consequences**: Default builds remain portable and safer. Domestic builds become faster when users configure a verified mirror/sumdb combination. BuildKit becomes the expected Docker builder. Parser image will still be the largest image because PaddleOCR/Paddle dependencies dominate, but cache and layer shape should improve.

## Out Of Scope

- Replacing PaddleOCR/Paddle dependencies or changing Parser OCR capability.
- Building and publishing shared organization base images.
- Introducing Kubernetes, production registry publishing, or multi-arch release automation.
- Collapsing all service Dockerfiles into one generated template.
- Changing application runtime behavior unrelated to Docker build/deploy.

## Technical Notes

- Relevant upstream PR: #340 `fix(docker): align local image versions`, merged 2026-06-30, now in local `develop`.
- Relevant files inspected:
  - `services/*/Dockerfile`
  - `services/parser/Dockerfile`
  - `deploy/Dockerfile.migrate`
  - `deploy/docker-compose.yml`
  - `services/qa/docker-compose.yml`
  - `services/document/docker-compose.yml`
  - `.github/workflows/docker-deploy-checks.yml`
  - `deploy/README.md`
  - `docs/runbooks/local-integration.md`
  - `docs/architecture/technology-decisions.md`
  - `.trellis/spec/backend/quality-guidelines.md`
  - `.trellis/spec/cicd.md`

## Research References

- [`research/docker-build-optimization.md`](research/docker-build-optimization.md) - Summary of local repo findings and recommended Docker build strategy.
