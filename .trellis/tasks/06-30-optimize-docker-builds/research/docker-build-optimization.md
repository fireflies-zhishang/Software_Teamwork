# Docker Build Optimization Research

## Repo Findings

- Go service Dockerfiles already use multi-stage builds and small Alpine runtime images.
- Go service builds repeat the same pattern seven times and currently use `go mod download` without BuildKit cache mounts, so cache reuse depends only on Docker layer cache and is fragile when source/module files change.
- `services/document/Dockerfile` and `services/ai-gateway/Dockerfile` do not use the same `CGO_ENABLED=0 GOOS=linux -trimpath -ldflags="-s -w"` build flags as most other Go services.
- Only Parser, QA, and Document have `.dockerignore`; Auth/File/Gateway/Knowledge/AI Gateway do not.
- Parser is the only Python runtime image. It uses `python:3.12-slim`, apt packages (`libglib2.0-0`, `libgl1`, `libgomp1`), uv, and PaddleOCR extras. This is intentionally heavier than Go services.
- Root Compose already pins infrastructure images. Upstream PR #340 aligned Redis and Alpine tags and documented the MinIO server/client exception.
- Compose infrastructure images are also part of the Docker source surface: PostgreSQL, Redis, Qdrant, MinIO server, and MinIO client should keep pinned defaults but allow full-image override variables.
- `services/qa/Dockerfile.host` is a Docker surface even though it builds from a precompiled host binary; it should support the same image registry prefix pattern for its `postgres:16-alpine` base.

## External Build Conventions

- Docker BuildKit cache mounts are the standard Dockerfile mechanism for persistent package manager caches without baking cache contents into final layers.
- Docker daemon registry mirrors accelerate image pulls at the daemon level and should be documented as local machine configuration, not hardcoded into `FROM` lines.
- Dockerfile `ARG` before `FROM` can parameterize a registry prefix while keeping default public images.
- uv supports Python package index configuration through environment/build arguments; this is the right level for PyPI/Tsinghua/Aliyun style mirror selection.
- Go checksum verification is part of build correctness. A Go module proxy may also proxy checksum database (`/sumdb/...`) requests when it advertises support. If that mirror has incomplete/broken sumdb paths, `go install` can fail even when module downloads appear to work.

## Go Proxy / SumDB Findings

- User-observed failure: `goproxy.cn` sumdb mirror returned bad/404 responses during `go install github.com/pressly/goose/v3/cmd/goose@v3.27.1`, causing migration image build failure.
- Local probes on 2026-06-30:
  - `https://goproxy.cn/sumdb/sum.golang.org/supported` returned `200`, so Go may use goproxy.cn as a checksum database proxy for `sum.golang.org`.
  - Specific lookup probes for `github.com/pressly/goose/v3@v3.27.1`, `github.com/tursodatabase/libsql-client-go@...`, and `modernc.org/libc@v1.72.1` returned `200` during this session, but that does not prove all required tiles/lookups are reliable.
  - `https://goproxy.cn/sumdb/sum.golang.google.cn/supported` returned `404`; this can make Go access `sum.golang.google.cn` directly instead of through goproxy.cn when `GOSUMDB=sum.golang.google.cn` is configured.
  - `https://proxy.golang.org` timed out from the current environment, so the official proxy is not a reliable sole default for this network.
- Do not set `GOSUMDB=off` as the normal fix; that trades build speed for weaker supply-chain verification. Use it only as an explicit last-resort local workaround.

## Docker Daemon Mirror Findings

- Local Docker daemon mirrors on 2026-06-30: `["https://docker.m.daocloud.io/"]`.
- `DOCKER_BUILDKIT=1 docker build --file deploy/Dockerfile.migrate ... deploy` parsed the Dockerfile and resolved `golang:1.25-alpine`, then failed resolving `alpine:3.22` with:
  `unexpected status from HEAD request to https://docker.m.daocloud.io/v2/library/alpine/manifests/3.22?ns=docker.io: 401 Unauthorized`.
- `docker buildx build --check --file services/qa/Dockerfile.host services/qa` failed before Dockerfile logic ran because the same daemon mirror returned `401 Unauthorized` for `postgres:16-alpine`.
- Earlier attempts with `# syntax=docker/dockerfile:*` failed before repository Dockerfile logic ran because the same daemon mirror blocked the external Dockerfile frontend image. Removing syntax headers avoids that extra pull while keeping BuildKit cache mounts working on current Docker.
- This is an environment issue, not a service Dockerfile issue. The correct fix is to remove or replace the broken Docker daemon registry mirror, then rerun image metadata/build checks.

## Recommended Approach

1. Add BuildKit cache mounts without requiring an external Dockerfile frontend image:
   - Go: cache `/go/pkg/mod` and `/root/.cache/go-build`.
   - apk: cache `/var/cache/apk` only when installing packages.
   - Parser/uv: cache uv/Python package downloads in builder stage.
2. Add build args:
   - `IMAGE_REGISTRY_PREFIX` for `FROM` image mirror/registry overrides.
   - `GOPROXY`, with safe defaults and explicit domestic override examples.
   - `GOSUMDB`, defaulting to the Go toolchain default; domestic override examples can use `sum.golang.google.cn` to avoid a broken third-party sumdb mirror path while keeping verification enabled.
   - `ALPINE_MIRROR`, empty by default.
   - Parser-specific `APT_MIRROR`, `UV_DEFAULT_INDEX`, and optionally `UV_INDEX`.
3. Add Compose image override variables:
   - `POSTGRES_IMAGE`, `REDIS_IMAGE`, `QDRANT_IMAGE`, `MINIO_IMAGE`, `MINIO_MC_IMAGE`.
   - Defaults must remain pinned and must not become `latest`.
4. Add `.dockerignore` files for all Go service build contexts.
5. Keep Go and Parser runtime families separate:
   - Go: Alpine is small and compatible with static binaries.
   - Parser: Debian slim avoids Alpine/musl problems with Paddle/PaddleOCR native wheels.
6. Update CI to force BuildKit for Dockerfile builds.
7. Update docs with mirror examples and expectations.

## Risks And Mitigations

- BuildKit-only syntax can fail on old Docker engines. Mitigation: document Compose v2/BuildKit baseline and set `DOCKER_BUILDKIT=1` in CI build jobs.
- External Dockerfile frontend pulls can fail before build logic runs when a daemon registry mirror is broken. Mitigation: rely on the Docker engine's bundled frontend for current cache-mount syntax instead of adding `# syntax=docker/dockerfile:*` headers.
- Domestic mirror URLs can become unavailable or serve incomplete module/sumdb data. Mitigation: make mirror args optional and overridable, keep checksum verification enabled by default, and document tested combinations plus failure symptoms.
- Parser image may remain large because PaddleOCR dependencies dominate. Mitigation: keep cache out of runtime layers and document that size is capability-driven.
- Registry prefix syntax must include a trailing slash when used. Mitigation: document examples like `--build-arg IMAGE_REGISTRY_PREFIX=registry.cn-hangzhou.aliyuncs.com/dockerhub-mirror/` or a team-approved mirror path.
- Compose `image:` overrides can accidentally drift to unpinned tags. Mitigation: keep pinned defaults in Compose and document `*_IMAGE` variables as full pinned image replacements only.

## Validation Notes

- Compose config validation passed for:
  - `deploy/docker-compose.yml` default profile
  - `deploy/docker-compose.yml --profile ai`
  - `services/qa/docker-compose.yml`
  - `services/qa/docker-compose.db.yml`
  - `services/document/docker-compose.yml`
- `git diff --check` passed.
- `docker buildx build --check --target build` passed for every changed Go service Dockerfile and migration Dockerfile.
- `docker buildx build --check` passed for `services/parser/Dockerfile`.
- `services/qa/Dockerfile.host` supports `IMAGE_REGISTRY_PREFIX` and `POSTGRES_VERSION`; validate it with Dockerfile static checks and, when a working Docker daemon mirror is available, a real build.
- `services/qa/Dockerfile.host` validation is blocked in this environment because the daemon mirror returns `401 Unauthorized` for `postgres:16-alpine` metadata.
- Representative BuildKit build-stage checks passed:
  - `services/auth/Dockerfile --target build` with `GOPROXY=https://goproxy.cn,direct`, `GOSUMDB=sum.golang.google.cn`, and Tsinghua Alpine mirror.
  - `deploy/Dockerfile.migrate --target build` with the same Go/sumdb settings; this validates the goose dependency path that previously failed.
- Full Alpine runtime builds are blocked in the current local environment by Docker daemon mirror `https://docker.m.daocloud.io/` returning `401 Unauthorized` for `alpine:3.22` metadata. Fix daemon mirror configuration before using this machine for full runtime image validation.
