# Docker 构建环境与镜像源

日期：2026-06-30

本文记录本仓库 Docker 构建的本机环境配置。优先级固定为：

```text
能跑 > 构建快 > 镜像小 > 内存少 > 存储少
```

默认构建不绑定某一个国内镜像站。镜像源、Go proxy、apt/apk/PyPI 源都可以按本机网络显式覆盖；覆盖后仍应保留 Go module checksum verification。

## 推荐快速配置

先启用 BuildKit：

```bash
export DOCKER_BUILDKIT=1
```

`DOCKER_BUILDKIT` 必须是执行 `docker build` / `docker compose build` 的 shell 环境变量；它不是 Compose build arg，写入 `deploy/.env` 不一定能控制 Docker builder。

国内网络下，Go module 建议使用：

```bash
export GO_DOCKER_GOPROXY=https://goproxy.cn,direct
export GO_DOCKER_GOSUMDB=sum.golang.google.cn
```

不要把 `GOSUMDB=off` 当作普通修复。`goproxy.cn` 会声明支持代理 `sum.golang.org` checksum database；当该代理路径上的 lookup/tile 返回异常 404 时，`go install github.com/pressly/goose/v3/cmd/goose@v3.27.1` 会在校验阶段失败，进而让 `migrate-file` 失败并取消依赖它的 Parser/服务构建。使用 `sum.golang.google.cn` 可以绕开第三方 sumdb 代理路径，同时继续做 checksum verification。

可选包管理器镜像：

```bash
export ALPINE_MIRROR=https://mirrors.tuna.tsinghua.edu.cn/alpine
export DEBIAN_APT_MIRROR=https://mirrors.tuna.tsinghua.edu.cn/debian
export DEBIAN_SECURITY_APT_MIRROR=https://mirrors.tuna.tsinghua.edu.cn/debian-security
export PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple
export UV_DEFAULT_INDEX=https://pypi.tuna.tsinghua.edu.cn/simple
```

然后构建：

```bash
cd deploy
docker compose --env-file .env.example build
docker compose --env-file .env.example --profile ai build
```

如需长期生效，把 `GO_DOCKER_GOPROXY`、`GO_DOCKER_GOSUMDB`、`ALPINE_MIRROR`、`DEBIAN_APT_MIRROR`、`DEBIAN_SECURITY_APT_MIRROR`、`PIP_INDEX_URL`、`UV_DEFAULT_INDEX` 等 Compose build args 写入从 `deploy/.env.example` 复制出来的 `deploy/.env`。`DOCKER_BUILDKIT=1` 仍然在 shell profile 或 CI job env 中设置。

Compose 基础设施镜像也可以按本机/企业 registry 覆盖，但默认必须继续是明确 tag：

```bash
export POSTGRES_IMAGE=postgres:16-alpine
export REDIS_IMAGE=redis:7-alpine
export QDRANT_IMAGE=qdrant/qdrant:v1.18.2
export MINIO_IMAGE=minio/minio:RELEASE.2025-09-07T16-13-09Z
export MINIO_MC_IMAGE=minio/mc:RELEASE.2025-08-13T08-35-41Z
```

如果企业 registry 使用重写后的完整镜像名，把这些变量设成完整目标镜像，而不是使用 `DOCKER_IMAGE_REGISTRY_PREFIX`。Compose 的 `image:` 字段和 Dockerfile 的 `FROM` 字段是两条不同路径。

## Docker Hub 镜像源

Docker daemon 的 registry mirror 是本机配置，不应写死到仓库 Dockerfile。先看当前配置：

```bash
docker info --format '{{json .RegistryConfig.Mirrors}}'
```

如果 base image 拉取报错，先验证镜像源能否正确返回 manifest：

```bash
docker manifest inspect alpine:3.22 >/tmp/alpine-manifest.json
docker manifest inspect golang:1.25-alpine >/tmp/golang-manifest.json
docker manifest inspect python:3.12-slim >/tmp/python-manifest.json
docker manifest inspect postgres:16-alpine >/tmp/postgres-manifest.json
docker manifest inspect redis:7-alpine >/tmp/redis-manifest.json
docker manifest inspect qdrant/qdrant:v1.18.2 >/tmp/qdrant-manifest.json
docker manifest inspect minio/minio:RELEASE.2025-09-07T16-13-09Z >/tmp/minio-manifest.json
docker manifest inspect minio/mc:RELEASE.2025-08-13T08-35-41Z >/tmp/minio-mc-manifest.json
```

本次排查中，`https://docker.m.daocloud.io/` 作为 daemon mirror 时对 `alpine:3.22` 和 `postgres:16-alpine` manifest 返回 `401 Unauthorized`，并且会让 BuildKit 在解析 `FROM alpine:3.22`、`FROM postgres:16-alpine` 或外部 Dockerfile frontend 时失败。遇到这种情况，先移除或替换 daemon mirror，再重试构建。

`DOCKER_IMAGE_REGISTRY_PREFIX` 只用于企业/团队提供的显式 registry rewrite，例如：

```bash
export DOCKER_IMAGE_REGISTRY_PREFIX=registry.example.com/dockerhub/
```

该值会直接改写 Dockerfile `FROM` 镜像名，必须包含末尾 `/`，并且目标 registry 必须提供 `golang:1.25-alpine`、`alpine:3.22`、`python:3.12-slim`、`postgres:16-alpine` 等同名路径。普通 Docker Hub daemon mirror 优先在 Docker daemon 配置里处理，不建议用 `DOCKER_IMAGE_REGISTRY_PREFIX` 硬凑。

## Go 构建

默认值：

```text
GO_DOCKER_GOPROXY=https://proxy.golang.org,direct
GO_DOCKER_GOSUMDB=sum.golang.org
```

国内推荐显式覆盖：

```bash
export GO_DOCKER_GOPROXY=https://goproxy.cn,direct
export GO_DOCKER_GOSUMDB=sum.golang.google.cn
```

验证 goose 依赖下载和校验：

```bash
docker run --rm golang:1.25-alpine sh -c \
  'GOMODCACHE=/tmp/modcache GOCACHE=/tmp/gocache GOPROXY=https://goproxy.cn,direct GOSUMDB=sum.golang.google.cn go install github.com/pressly/goose/v3/cmd/goose@v3.27.1'
```

如果这条命令失败，先修正 Go proxy/sumdb 或本机网络，再构建 migration 镜像。

## Alpine 与 Debian

Go 服务继续使用：

```text
builder: golang:1.25-alpine
runtime: alpine:3.22
```

原因是 Go 服务可以产出静态小二进制，Alpine runtime 镜像小，适合当前服务边界。

Parser 继续使用：

```text
python:3.12-slim
```

原因是 Parser 依赖 PaddleOCR/PaddlePaddle/native Python wheels 和系统库。为了省体积强行切到 Alpine/musl 会优先破坏“能跑”。Parser 的优化策略是 Debian slim、多阶段构建、BuildKit cache 和避免把包管理器缓存带进 runtime。

## Parser 构建

Parser 可选镜像源示例：

```bash
DOCKER_BUILDKIT=1 docker build \
  --build-arg DEBIAN_APT_MIRROR=https://mirrors.tuna.tsinghua.edu.cn/debian \
  --build-arg DEBIAN_SECURITY_APT_MIRROR=https://mirrors.tuna.tsinghua.edu.cn/debian-security \
  --build-arg PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple \
  --build-arg UV_DEFAULT_INDEX=https://pypi.tuna.tsinghua.edu.cn/simple \
  -t software-teamwork-parser:local \
  services/parser
```

Parser 是当前最大的镜像和最高内存服务。默认本地 Compose 保持：

```text
PARSER_LOAD_BACKEND_ON_STARTUP=false
PARSER_MAX_CONCURRENCY=1
```

这会减少启动时常驻模型内存，优先保证 16 GB 级别开发机可运行。真实 OCR 第一次调用仍可能下载/加载模型，构建和运行时间都明显高于 Go 服务。

## Compose 基础设施镜像

本地联调依赖这些 pinned 镜像：

| 组件 | 默认镜像 | 覆盖变量 |
| --- | --- | --- |
| PostgreSQL | `postgres:16-alpine` | `POSTGRES_IMAGE` |
| Redis | `redis:7-alpine` | `REDIS_IMAGE` |
| Qdrant | `qdrant/qdrant:v1.18.2` | `QDRANT_IMAGE` |
| MinIO server | `minio/minio:RELEASE.2025-09-07T16-13-09Z` | `MINIO_IMAGE` |
| MinIO client | `minio/mc:RELEASE.2025-08-13T08-35-41Z` | `MINIO_MC_IMAGE` |

不要把这些变量改成 `latest`。如果某个镜像在当前网络下拉取慢或失败，优先配置 Docker daemon mirror；企业 registry 需要完整改写时，再设置对应 `*_IMAGE` 变量。

## 存储与缓存

Dockerfiles 使用 BuildKit cache mount 复用：

- Go module cache: `/go/pkg/mod`
- Go build cache: `/root/.cache/go-build`
- apk cache: `/var/cache/apk`
- apt cache: `/var/cache/apt`、`/var/lib/apt/lists`
- pip/uv cache: `/root/.cache/pip`、`/root/.cache/uv`

这些 cache 不会进入最终 runtime layer，但会占用本机 builder 存储。查看和清理：

```bash
docker system df
docker builder prune
```

清理会让下一次构建重新下载依赖；只在磁盘压力明显时执行。

## 验证清单

配置层验证：

```bash
docker compose -f deploy/docker-compose.yml --env-file deploy/.env.example config --quiet
docker compose -f deploy/docker-compose.yml --env-file deploy/.env.example --profile ai config --quiet
docker compose -f services/qa/docker-compose.yml config --quiet
docker compose -f services/qa/docker-compose.db.yml config --quiet
docker compose -f services/document/docker-compose.yml config --quiet
```

构建层验证：

```bash
DOCKER_BUILDKIT=1 docker build -f deploy/Dockerfile.migrate deploy
DOCKER_BUILDKIT=1 docker build services/auth
DOCKER_BUILDKIT=1 docker build services/parser
DOCKER_BUILDKIT=1 docker build -f services/qa/Dockerfile.host services/qa
```

如果构建在 `FROM alpine:3.22`、`FROM golang:1.25-alpine`、`FROM python:3.12-slim`、`FROM postgres:16-alpine` 或 Compose `image:` 的 metadata/pull 阶段失败，优先排查 Docker daemon registry mirror；这类失败发生在仓库 Dockerfile 逻辑执行之前。
