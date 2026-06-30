# 本地联调运行手册

日期：2026-06-29

本文记录当前仓库可以怎样在本地启动、验证和排查服务。它不是生产部署说明；根级 `deploy/docker-compose.yml` 只作为本地/演示联调基线，不等同于已经具备完整一键 E2E smoke。

## 当前结论

| 范围 | 当前状态 | 说明 |
| --- | --- | --- |
| 根级本地/演示 Compose | partial | `deploy/docker-compose.yml` 已提供共享 PostgreSQL、Redis、Parser、服务 migration、`seed-local` / `seed-local-ai` 和基础服务串联；Compose 也会启动 Qdrant/MinIO 容器，并通过 `minio-init` 创建 `software-teamwork-local` bucket。默认 Knowledge 使用 in-memory vector index、File 容器使用 local storage；File 的 PostgreSQL + MinIO 路径通过显式 env-gated smoke 验证。现有 seed data 只覆盖本地登录、基础报告类型、示例知识库和 AI profile placeholder。 |
| QA 服务 Compose | partial | `services/qa/docker-compose.yml` 会启动 QA PostgreSQL、Auth PostgreSQL、Redis、Auth、QA 和 Gateway；不包含 Knowledge、Document、File、AI Gateway。 |
| Document 服务 Compose | partial | `services/document/docker-compose.yml` 会启动 Document PostgreSQL、Redis、migration 和 Document；不包含 File、AI Gateway。 |
| AI Gateway 本地运行 | root profile / host-run | 根级 `docker compose --profile ai` 会启动 AI Gateway、migration 和 placeholder profile seed；单独调试时也可 host-run，真实 provider smoke 仍需配置有效 provider key。 |
| File / Knowledge 独立运行 | host-run | 需要手动准备各自依赖；File 已有 env-gated PostgreSQL + MinIO 联合 smoke，Knowledge 已有 ingestion worker、Parser Service client、embedding 和 Qdrant/in-memory vector index 写入，仍缺 content、knowledge-queries、retrieval/rerank 闭环和真实依赖端到端 smoke。 |
| Parser Runtime | partial | `services/parser/` 已提供 Python/FastAPI runtime、内部 HTTP API、Dockerfile、service-token auth、可选 PaddleOCR extra 和 env-gated 真实 PaddleOCR 模型 smoke；CI 仍只用 fake OCR backend 覆盖 lint/test/compile，不要求普通开发者安装模型。 |
| 前端联调入口 | host-run | 前端只调用 public Gateway `/api/v1/**`；不要直连内部服务。 |

因此当前本地联调应按“根级依赖基线 + 服务级 smoke + 手动拼接关键链路”的方式执行。除非 #125 等跨服务 smoke 任务落地，不要在 PR 或文档中声称已有完整一键本地 E2E 验收环境。

## 前置依赖

| 工具 | 当前基线 | 用途 |
| --- | --- | --- |
| Go | `1.25` | 后端服务 build/test/run。 |
| Bun | `bun@1.3.12` | 前端 install/check/build；以根 `packageManager` 为准。 |
| Docker Compose | 支持 Compose v2 | 启动服务级 PostgreSQL、Redis 和服务容器。 |
| PostgreSQL | `postgres:16-alpine` | 服务数据库和 migration smoke。 |
| Redis | `redis:7-alpine` | Gateway session cache、QA/Document 队列。 |
| Alpine runtime | `alpine:3.22` | Go 服务 runtime 和 migration 镜像的运行阶段。 |

Docker 构建优先级按“能跑 > 构建快 > 镜像小 > 内存少 > 存储少”处理。默认
Go Docker build 使用 `GO_DOCKER_GOPROXY=https://proxy.golang.org,direct` 和
`GO_DOCKER_GOSUMDB=sum.golang.org`，保持 checksum verification 开启；国内网络
需要加速时显式设置：

```bash
export GO_DOCKER_GOPROXY=https://goproxy.cn,direct
export GO_DOCKER_GOSUMDB=sum.golang.google.cn
```

不要把 `GOSUMDB=off` 当作普通修复。`goproxy.cn` 的 `sum.golang.org` 代理路径
曾导致 goose migration 镜像构建校验失败；使用 `sum.golang.google.cn` 可以绕开该
第三方 sumdb 代理路径，同时保留模块校验。

Docker daemon mirror、Alpine/Debian/Python 镜像源、BuildKit cache 和磁盘清理见
[`Docker 构建环境与镜像源`](./docker-build-environment.md)。

需要访问 GitHub、Go module proxy、npm registry 或 provider 时，按本机 `proxy` 约定给单条命令加代理环境变量：

```bash
env all_proxy=socks5://127.0.0.1:10808 http_proxy=http://127.0.0.1:10808 https_proxy=http://127.0.0.1:10808 <command>
```

## 根级本地栈

```bash
cd deploy
cp .env.example .env
docker compose up -d --build
```

可选 AI Gateway profile：

```bash
cd deploy
docker compose --profile ai up -d --build
```

根级 Compose 详情见 [`deploy/README.md`](../../deploy/README.md)。

## 服务级启动

### File PostgreSQL + MinIO 联合 smoke

仅验证 File 服务自己的 metadata + object storage 组合，不代表完整
Gateway/Knowledge/QA/Document 跨服务 E2E。

先启动 PostgreSQL、MinIO、bucket 初始化和 File migration：

```bash
cd deploy
docker compose --env-file .env.example up -d postgres minio minio-init migrate-file
```

`minio-init` 使用 MinIO `mc` 客户端镜像创建 bucket `software-teamwork-local`；它不是第二个 MinIO server。默认本地配置：

```text
endpoint: localhost:9000
access key: minio_local_demo
secret key: minio-local-demo-password
region: empty
```

然后运行显式启用的 smoke：

```bash
cd ../services/file
FILE_MINIO_POSTGRES_SMOKE=1 \
FILE_TEST_DATABASE_URL='postgres://file_app:file_app_dev@localhost:5432/file_system?sslmode=disable' \
FILE_MINIO_ENDPOINT='localhost:9000' \
FILE_MINIO_ACCESS_KEY='minio_local_demo' \
FILE_MINIO_SECRET_KEY='minio-local-demo-password' \
FILE_MINIO_BUCKET='software-teamwork-local' \
go test ./internal/integration -run TestFileMinIOPostgresSmoke -count=1 -v
```

该测试默认跳过普通 CI。显式启用后会创建独立 PostgreSQL schema，通过
`/internal/v1/files/**` 上传、读取、删除测试对象，并验证删除后 metadata/content
不可读。失败时先看：

```bash
cd ../../deploy
docker compose logs postgres migrate-file minio minio-init
```

### QA + Auth + Gateway 局部环境

```bash
cd services/qa
docker compose up --build
```

该 Compose 适合验证 Auth、QA、Gateway 的基础 ready 状态和 QA 非 provider 依赖路径：

```bash
curl -fsS http://localhost:8081/readyz
curl -fsS http://localhost:8084/readyz
curl -fsS http://localhost:8080/readyz
```

注意：默认 `AI_GATEWAY_URL` 指向 Compose 网络内的 `http://ai-gateway:8086/internal/v1/chat/completions`，但该 Compose 没有 `ai-gateway` 服务。触发真实 LLM 调用、LLM connection test 或 Agent Run 时，需要额外启动 AI Gateway 并改写 `QA_AI_GATEWAY_URL`。

### Document 局部环境

```bash
cd services/document
docker compose up --build
```

该 Compose 适合验证 Document PostgreSQL、Redis、migration、job enqueue 和 worker 状态机：

```bash
curl -fsS http://localhost:8085/readyz
```

注意：模板、材料和报告文件 bytes 需要 File Service；真实大纲/正文生成需要 AI Gateway。当前基础 DOCX 导出使用 Document 内置 `SimpleDOCXGenerator`，不需要 Pandoc/LibreOffice；Pandoc/LibreOffice 仅是后续富 DOCX worker 工具链。当前 Compose 只给 File/AI Gateway 下游设置 URL，不启动这些下游服务，所以 Document-only 环境不能完整读取生成文件内容。Document worker 会执行 `report_file_creation` 的基础 DOCX 导出；其他大纲/正文生成类 job 仍只完成 job/attempt 状态流转。

### AI Gateway root profile / host-run

根级 `deploy/docker-compose.yml` 的 `--profile ai` 会启动 AI Gateway、执行 migration，并通过
`seed-local-ai` 写入本地 placeholder profile。下面的 host-run 示例用于单独调试服务进程。
AI Gateway 服务 token 运行时只接受 hash：

```bash
TOKEN=dev-internal-service-token-change-me
printf '%s' "$TOKEN" | shasum -a 256 | awk '{print "sha256:" $1}'
```

最小 host-run 环境示例：

```bash
export AI_GATEWAY_HTTP_ADDR=:8086
export AI_GATEWAY_DATABASE_URL='postgres://ai_gateway:ai_gateway@localhost:5436/ai_gateway?sslmode=disable'
export AI_GATEWAY_SERVICE_TOKEN_HASHES='sha256:<token-sha256-hex>'
export AI_GATEWAY_CREDENTIAL_ENCRYPTION_KEY_REF=local-dev-key-v1
export AI_GATEWAY_CREDENTIAL_ENCRYPTION_KEY='<long-local-secret>'

cd services/ai-gateway
go run github.com/pressly/goose/v3/cmd/goose@v3.27.1 -dir migrations postgres "$AI_GATEWAY_DATABASE_URL" up
go run ./cmd/server
```

创建可调用 profile 后，`/internal/v1/chat/completions`、`/internal/v1/embeddings` 和 `/internal/v1/rerankings` 都会按 profile 解析 provider、model、base URL 和 API key。当前 fake-provider 单元测试覆盖了协议形态；真实 provider smoke 仍需单独执行并记录。

## 手动联调顺序

完整链路尚未一键化时，建议按下面顺序缩小问题范围：

1. 单服务测试和 build 先通过：`go test ./...`、`go build ./cmd/server`。
2. 对有 migration 的服务执行 goose apply smoke。
3. 启动 Auth、Gateway、目标领域服务和该领域服务的数据库。
4. 需要模型调用时再启动 AI Gateway，并创建对应 `purpose=chat|embedding|rerank` 的 enabled/default profile。
5. 需要文件 bytes 时再启动 File Service；不要让领域服务直接暴露 object key、bucket 或内部 URL。
6. 通过 Gateway public `/api/v1/**` 验证前端可见能力；只在服务间 smoke 中直连 `/internal/v1/**`。

## 冒烟检查清单

| 场景 | 检查 | 当前预期 |
| --- | --- | --- |
| Auth/Gateway/QA 局部环境 | `GET /readyz` | 三个服务 ready；真实 AI 调用可能因未启动 AI Gateway 失败。 |
| Document 局部环境 | 创建 report job 后查询 job/attempt/events | 非文件生成类任务会入队并由 worker 推进为 succeeded；不会生成真实 AI 大纲/正文。若额外提供 File Service，`report_file_creation` 可生成基础 DOCX 并通过 content endpoint 读取成功文件。 |
| AI Gateway profile | 创建 chat/embedding/rerank profile，调用对应内部 endpoint | fake provider 和兼容 provider 应返回 OpenAI-style body；真实 provider 需手工验证。 |
| Gateway contract | `python3 scripts/verify_gateway_active_api.py` | active path、owner、security 和 owner map 不漂移。 |
| Parser PaddleOCR model | `PARSER_PADDLEOCR_SMOKE=1 PARSER_PADDLEOCR_ALLOW_DOWNLOAD=1 uv run pytest -m paddleocr_smoke -s` | 只在本机具备 PaddleOCR extra 和可用模型下载/缓存时运行；验证真实模型加载和最小 fixture OCR 非空。 |
| File PostgreSQL + MinIO | `FILE_MINIO_POSTGRES_SMOKE=1 ... go test ./internal/integration -run TestFileMinIOPostgresSmoke -count=1 -v` | 只在真实 PostgreSQL/MinIO 可用时运行；验证 upload、metadata、content read、delete 和清理状态。 |
| 前端 Gateway 类型 | `bun run --cwd apps/web api:generate` 后检查 diff | 生成类型应与 Gateway OpenAPI 保持同步。 |

## 已知缺口

| 缺口 | 影响 | 跟踪 |
| --- | --- | --- |
| 根级跨服务 smoke 缺失 | 即使使用 `deploy/docker-compose.yml` 启动本地/演示基线，也不能自动证明 Auth/Gateway/File/Knowledge/QA/Document/AI Gateway 链路可用。 | #125 |
| 跨服务契约测试和 E2E smoke 缺失 | 不能自动证明前端 -> Gateway -> 多服务链路可用。 | #125 |
| Parser 真实 OCR smoke 不在普通 CI 中运行 | Parser 已有 env-gated 真实 PaddleOCR 模型 smoke，但 CI 仍使用 fake OCR backend；真实模型、OCR 质量和部署资源需要在具备模型的本地或部署环境手动记录。 | #125 |
| Knowledge retrieval/rerank 与对象存储跨服务 smoke 缺失 | File 自身 PostgreSQL + MinIO smoke 已有；Knowledge 已有 Qdrant/in-memory vector index 写入和 File handoff，但 content、knowledge-queries、rerank、真实对象存储跨服务内容读取 smoke 仍缺。 | #152、#154、#289 |
| 生产部署基线缺失 | 当前 `deploy/docker-compose.yml` 是本地/演示基线，不能直接当生产部署。 | #150 |
| Document 真实 AI 生成和富 DOCX 工具链未落地 | 报告 job 状态机和基础 DOCX 导出可用；真实大纲/正文生成、Pandoc/LibreOffice 富 DOCX 转换和跨服务内容读取 smoke 仍需补齐。 | #160、#223 |
| Document 跨服务 smoke 仍缺失 | settings/statistics/logs 已在服务端落地，但管理端、Gateway、File Service、Document worker 串联 smoke 仍未一键化。 | #159、#221 |
| QA Agent Run MVP 和权限一致性仍在推进 | QA 会话/消息基础可用，完整 Agent 编排和 403 一致性仍需收口。 | #157、#217 |
| 前端业务 E2E 覆盖不足 | 已有 Playwright 基础 smoke；Knowledge、QA、Document 等完整业务流程仍需随页面能力扩展。 | #117、#163 |

## PR 前判断

- 只改文档：至少执行 `git diff --check`，并检查新增链接、相对路径和实现事实。
- 改后端服务：执行对应服务 `go test ./...` 和 `go build ./cmd/server`；QA 还要 `go build ./cmd/agent`。
- 改 migration：执行 goose apply；如果服务有 env-gated repository integration tests，尽量使用本地 PostgreSQL 跑一遍。
- 改 Parser 契约或运行时：检查 `docs/services/parser/api/internal.openapi.yaml`、`services/parser/api/openapi.yaml`（实现本地副本）、Parser README、Knowledge ingestion 文档和 `parser-service.yml` 是否一致；运行 `cd services/parser && uv run ruff check . && uv run pytest && uv run python -m compileall src tests`。如触碰 PaddleOCR runtime 或部署资源，尽量追加 `PARSER_PADDLEOCR_SMOKE=1` 的真实模型 smoke，并在 PR 记录中区分 fake OCR 与真实模型结果。
- 改 Gateway OpenAPI：执行 `python3 scripts/verify_gateway_active_api.py`，前端类型相关改动还要执行 `bun run --cwd apps/web api:generate` 并检查生成 diff。
- 改前端：执行 `bun install --frozen-lockfile`、`bun run --cwd apps/web check`、`bun run --cwd apps/web build`、`bun run --cwd apps/web test:unit`；关键页面改动再跑 `bun run --cwd apps/web test:e2e`。
