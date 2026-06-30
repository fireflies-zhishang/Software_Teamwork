# 本地联调运行手册

日期：2026-06-29

本文记录当前仓库可以怎样在本地启动、验证和排查服务。它不是生产部署说明；根级 `deploy/docker-compose.yml` 只作为本地/演示联调基线，不等同于已经具备完整一键 E2E smoke。

## 当前结论

| 范围 | 当前状态 | 说明 |
| --- | --- | --- |
| 根级本地/演示 Compose | partial | `deploy/docker-compose.yml` 已提供共享 PostgreSQL、Redis、Qdrant、MinIO、服务 migration、`seed-local` / `seed-local-ai` 和服务串联基线；仍缺统一跨服务 smoke 和一键验收脚本，现有 seed data 只覆盖本地登录、基础报告类型、示例知识库和 AI profile placeholder。 |
| QA 服务 Compose | partial | `services/qa/docker-compose.yml` 会启动 QA PostgreSQL、Auth PostgreSQL、Redis、Auth、QA 和 Gateway；不包含 Knowledge、Document、File、AI Gateway。 |
| Document 服务 Compose | partial | `services/document/docker-compose.yml` 会启动 Document PostgreSQL、Redis、migration 和 Document；不包含 File、AI Gateway。 |
| AI Gateway 本地运行 | root profile / host-run | 根级 `docker compose --profile ai` 会启动 AI Gateway、migration 和 placeholder profile seed；单独调试时也可 host-run，真实 provider smoke 仍需配置有效 provider key。 |
| File / Knowledge 独立运行 | host-run | 需要手动准备各自依赖；File MinIO adapter 已落地但缺真实对象存储 smoke，Knowledge Qdrant adapter 尚未落地。 |
| Parser Runtime | contract-only | 当前只有内部 OpenAPI、README 和目录 scaffold；Python packaging、PaddleOCR runtime、Docker image 和 HTTP smoke 尚未落地。 |
| 前端联调入口 | host-run | 前端只调用 public Gateway `/api/v1/**`；不要直连内部服务。 |

因此当前本地联调应按“根级依赖基线 + 服务级 smoke + 手动拼接关键链路”的方式执行。除非 #125 等跨服务 smoke 任务落地，不要在 PR 或文档中声称已有完整一键本地 E2E 验收环境。

## 前置依赖

| 工具 | 当前基线 | 用途 |
| --- | --- | --- |
| Go | `1.25` | 后端服务 build/test/run。 |
| Bun | `1.3.x`，根 `packageManager` 为 `bun@1.3.12` | 前端 install/check/build。 |
| Docker Compose | 支持 Compose v2 | 启动服务级 PostgreSQL、Redis 和服务容器。 |
| PostgreSQL | `postgres:16-alpine` | 服务数据库和 migration smoke。 |
| Redis | `redis:7-alpine` 或当前服务 Compose 镜像 | Gateway session cache、QA/Document 队列。 |

需要访问 GitHub、Go module proxy、npm registry 或 provider 时，按本机 `proxy` 约定给单条命令加代理环境变量：

```bash
env all_proxy=socks5://127.0.0.1:10808 http_proxy=http://127.0.0.1:10808 https_proxy=http://127.0.0.1:10808 <command>
```

## 服务级启动

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
| 前端 Gateway 类型 | `bun run --cwd apps/web api:generate` 后检查 diff | 生成类型应与 Gateway OpenAPI 保持同步。 |

## 已知缺口

| 缺口 | 影响 | 跟踪 |
| --- | --- | --- |
| 根级跨服务 smoke 缺失 | 即使使用 `deploy/docker-compose.yml` 启动本地/演示基线，也不能自动证明 Auth/Gateway/File/Knowledge/QA/Document/AI Gateway 链路可用。 | #125 |
| 跨服务契约测试和 E2E smoke 缺失 | 不能自动证明前端 -> Gateway -> 多服务链路可用。 | #125 |
| Parser runtime 和 smoke 缺失 | 只能检查 `services/parser/api/openapi.yaml` 与文档一致性，不能验证真实 PaddleOCR 解析或 Knowledge -> Parser 调用链路。 | 待拆分 |
| Qdrant adapter 未落地，MinIO 跨服务 smoke 缺失 | Knowledge 检索闭环仍缺 Qdrant adapter；File 已有 MinIO adapter，但真实对象存储和跨服务内容读取 smoke 仍缺。 | #152、#154 |
| Document 真实 AI 生成和富 DOCX 工具链未落地 | 报告 job 状态机和基础 DOCX 导出可用；真实大纲/正文生成、Pandoc/LibreOffice 富 DOCX 转换和跨服务内容读取 smoke 仍需补齐。 | #160、#223 |
| Document 跨服务 smoke 仍缺失 | settings/statistics/logs 已在服务端落地，但管理端、Gateway、File Service、Document worker 串联 smoke 仍未一键化。 | #159、#221 |
| QA Agent Run MVP 和权限一致性仍在推进 | QA 会话/消息基础可用，完整 Agent 编排和 403 一致性仍需收口。 | #157、#217 |
| 前端跨后端真实 E2E smoke 缺失 | 前端 CI 已覆盖 check/build、Vitest unit/component tests 和 Playwright E2E smoke；但浏览器测试仍以 mock/前端关键流为主，不能替代真实 Gateway/多服务联调。 | #125 |

## PR 前判断

- 只改文档：至少执行 `git diff --check`，并检查新增链接、相对路径和实现事实。
- 改后端服务：执行对应服务 `go test ./...` 和 `go build ./cmd/server`；QA 还要 `go build ./cmd/agent`。
- 改 migration：执行 goose apply；如果服务有 env-gated repository integration tests，尽量使用本地 PostgreSQL 跑一遍。
- 改 Parser 契约或运行时规划：检查 `services/parser/api/openapi.yaml`、Parser README 和 Knowledge ingestion 文档是否一致；runtime 落地前不要记录 `pytest`、PaddleOCR smoke 或 Parser build 结果。
- 改 Gateway OpenAPI：执行 `python3 scripts/verify_gateway_active_api.py`，前端类型相关改动还要执行 `bun run --cwd apps/web api:generate` 并检查生成 diff。
- 改前端：执行 `bun install --frozen-lockfile`、`bun run --cwd apps/web check`、`bun run --cwd apps/web build`、`bun run --cwd apps/web test:unit`；涉及关键浏览器流程时执行 `bun run --cwd apps/web playwright install --with-deps chromium` 和 `bun run --cwd apps/web test:e2e`。
