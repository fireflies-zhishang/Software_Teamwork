# 测试策略

日期：2026-06-30

本文把当前仓库已经可执行的检查、CI 覆盖和仍缺的测试能力放在一起，作为 PR 前验证基线。具体服务实现状态仍以各服务 `docs/implementation.md` 为准。

本文中的检查分为三类：

- 当前 CI 覆盖：已经由 GitHub Actions 执行，可作为 required checks 候选。
- PR 前建议：本地应按改动范围尽量执行，并在 PR body 写明结果。
- 缺口：当前缺少稳定环境或脚本，不能写成 required，落地后再升级。

## 总体原则

- 改什么跑什么，但跨契约、跨服务或共享文档变更要扩大检查范围。
- OpenAPI 是协作源；改 Gateway active API 时必须跑契约校验和前端类型同步检查。
- 数据库 migration 必须能从空库 apply。
- env-gated integration tests 默认可能跳过；如果本次改动触碰 repository、SQL 或 migration，应尽量提供本地数据库执行记录。
- 当前有前端 Playwright 基础 smoke，但没有后端跨服务完整 E2E smoke；不要用单服务测试或前端 mock E2E 替代跨服务验收。
- Parser runtime、Dockerfile 和 Parser Service CI 已落地；当前 CI 使用 fake OCR backend 覆盖 lint/test/compile，并在 PaddleOCR 依赖、锁文件或 Dockerfile 变化时校验 extra lock，不等同于真实 PaddleOCR 模型 smoke。
- open PR、未合入 issue 和草案不能写成当前 `develop` 已实现；测试记录也不能把未稳定依赖的检查写成 required。

## 当前 CI 覆盖

| Workflow | 覆盖 | 当前说明 |
| --- | --- | --- |
| `frontend.yml` | `apps/web/**`、根前端依赖文件和 workflow | 执行 `bun install --frozen-lockfile`、`bun run --cwd apps/web check`、`build`、`test:unit` 和 Playwright E2E smoke。 |
| `go-services.yml` | `services/{ai-gateway,auth,document,file,gateway,knowledge,qa}` | 根据变更路径只选择受影响服务，执行 `go test ./...`、`go build ./cmd/server`；QA 额外 build `./cmd/agent`；Knowledge 或 workflow 变更时额外用 PostgreSQL 16 和 `KNOWLEDGE_TEST_DATABASE_URL` 执行 repository lifecycle integration test。 |
| `go-migrations.yml` | 有 SQL migration 的后端服务 | 校验 migration 文件名并用 `goose@v3.27.1` 对 PostgreSQL 16 apply。 |
| `parser-service.yml` | `services/parser/**` | 执行 `uv sync --frozen --group dev`、`uv run ruff check .`、`uv run pytest` 和 `uv run python -m compileall src tests`；当 `services/parser/pyproject.toml`、`uv.lock` 或 `Dockerfile` 变化时额外执行 PaddleOCR extra lock dry-run；测试使用 fake OCR backend，不下载真实 PaddleOCR 模型。 |
| `docker-deploy-checks.yml` | 服务镜像输入、已有服务 Dockerfile、服务 Compose、`deploy/**` | 对受影响服务的可构建 Dockerfile 执行 `docker build`，对变更的 Compose 文件执行 `docker compose ... config --quiet`；不 push 镜像、不部署。 |
| `gateway-contract.yml` | Gateway OpenAPI active API | 执行 verifier unit tests 和 `python3 scripts/verify_gateway_active_api.py`。 |
| `check-api-types.yml` | 前端 Gateway 类型漂移 | 在 `apps/web` 执行 `bun run api:generate`，本地等价命令为 `bun run --cwd apps/web api:generate`，并要求 generated diff 干净。 |
| `commitlint.yml` / `pr-guard.yml` | 协作规则 | 检查提交格式、PR body、issue 关联和 base 更新要求。 |

当前可作为 required checks 的优先候选是 Frontend、Go service tests、goose migration apply、Parser Service、Docker/Compose config、Gateway contract/API drift 和 API type drift。Parser 真实 PaddleOCR 模型 smoke、完整 DB integration jobs 和后端跨服务 E2E smoke 仍未落地；在 CI 提供稳定依赖前只能作为 PR 前建议或缺口登记。

## 本地命令矩阵

| 改动范围 | 必跑命令 |
| --- | --- |
| 文档 | `git diff --check`；检查新增链接和实现事实。 |
| Gateway OpenAPI / owner map | `python3 -m unittest scripts.tests.test_verify_gateway_active_api`；`python3 scripts/verify_gateway_active_api.py`。 |
| 前端 | `bun install --frozen-lockfile`；`bun run --cwd apps/web check`；`bun run --cwd apps/web build`；`bun run --cwd apps/web test:unit`；关键页面改动再跑 `bun run --cwd apps/web test:e2e`。 |
| 前端 API 类型 | `bun run --cwd apps/web api:generate`；确认 generated diff 符合预期。 |
| 单个 Go 服务 | `cd services/<service> && go test ./...`；`go build ./cmd/server`。 |
| QA 服务 | `cd services/qa && go test ./...`；`go build ./cmd/server`；`go build ./cmd/agent`。 |
| Dockerfile | 对变更的可构建 Dockerfile 执行 `docker build --file <Dockerfile> <context>`；不 push 镜像。 |
| Compose | `docker compose -f <compose-file> config --quiet`。 |
| Knowledge repository / SQL | `cd services/knowledge && KNOWLEDGE_TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable' go test ./internal/repository -count=1`。 |
| migration | `go run github.com/pressly/goose/v3/cmd/goose@v3.27.1 -dir migrations postgres "$DATABASE_URL" up`。 |
| Parser 契约 / 文档 / runtime | 检查 `services/parser/api/openapi.yaml` 与 `docs/services/parser/README.md`、Knowledge ingestion 文档一致；如改 runtime，执行 `cd services/parser && uv run ruff check . && uv run pytest && uv run python -m compileall src tests`，并说明是否仅覆盖 fake OCR backend。 |
| AI Gateway provider adapter | `cd services/ai-gateway && go test ./...`；尽量加 fake provider case 和真实 provider smoke 记录。 |
| Document worker/job | `cd services/document && go test ./...`；如改 repository，设置 `DOCUMENT_TEST_DATABASE_URL` 跑 repository integration tests。 |

## 后端测试层级

| 层级 | 当前做法 | 适用场景 |
| --- | --- | --- |
| Unit tests | Go `testing`、fake repository、fake provider、httptest。 | service rules、handler validation、脱敏、错误归一化。 |
| Repository tests | 部分服务有 SQL/repository tests；Knowledge/QA/Document 有 env-gated PostgreSQL integration tests；Knowledge repository lifecycle 已接入 CI PostgreSQL job。 | repository、SQL、transaction、migration 相关改动。 |
| Migration apply | CI 使用 PostgreSQL 16 和 goose apply。 | 新增或修改 migration。 |
| Contract tests | Gateway active API verifier、route coverage tests。 | OpenAPI、owner map、active path 和 RESTful path 规则。 |
| Parser runtime tests | OpenAPI schema review、文档一致性检查、FastAPI handler/service tests 和 fake OCR backend。 | Parser API/runtime 变更；真实 PaddleOCR 模型、OCR 质量和部署资源仍需单独 smoke。 |
| Cross-service smoke | 当前缺失统一脚本。 | Auth -> Gateway -> Domain、Document -> File/AI Gateway、QA -> Knowledge/AI Gateway 等链路。 |

env-gated repository tests：

```bash
cd services/qa
QA_TEST_DATABASE_URL='postgres://qa_app:qa_app_dev@localhost:5433/qa_system?sslmode=disable' go test ./internal/repository

cd services/document
DOCUMENT_TEST_DATABASE_URL='postgres://document_app:document_app_dev@localhost:5435/document_system?sslmode=disable' go test ./internal/repository

cd services/knowledge
KNOWLEDGE_TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable' go test ./internal/repository -count=1
```

## 前端测试层级

| 层级 | 当前状态 | 说明 |
| --- | --- | --- |
| Type check | 已落地 | `bun run --cwd apps/web typecheck`。 |
| Lint | 已落地 | `bun run --cwd apps/web lint`。 |
| Format check | 已落地 | `bun run --cwd apps/web format:check`。 |
| Build | 已落地 | `bun run --cwd apps/web build`。 |
| API type generation | 已落地 | `bun run --cwd apps/web api:generate`，以 Gateway OpenAPI 为源。 |
| Component/unit tests | 已落地 | `bun run --cwd apps/web test:unit`，使用 Vitest + React Testing Library。 |
| Browser/E2E tests | 已落地 / smoke | `bun run --cwd apps/web test:e2e`，使用 Playwright 覆盖基础应用 smoke；完整业务 E2E 仍需随页面能力扩展。 |

前端不得直接调用服务内部地址。涉及 QA SSE、上传、报告任务进度或 admin model/parser configuration 的改动，应同时检查 `docs/architecture/frontend-backend-contract.md` 和 Gateway OpenAPI。

## 契约和文档检查

| 检查 | 触发条件 |
| --- | --- |
| Gateway active API verifier | 改 `docs/services/gateway/api/openapi.yaml`、owner map、Gateway route 或前端 API 生成规则。 |
| 服务 implementation 文档 | 改服务能力、stub/501 状态、runtime dependency、migration、worker 或 provider adapter。 |
| 技术选型基线 | 引入新运行时依赖、镜像、CLI、SDK、队列、数据库或工具链。 |
| 本地联调手册 | 新增 Compose、env template、seed data、跨服务 smoke 或端口约定。 |
| Parser 契约一致性 | 改 `services/parser/api/openapi.yaml`、Parser README、Knowledge ingestion 对 Parser 的调用约定或 parser runtime configuration。 |
| 测试策略 | 新增 CI workflow、测试框架、E2E smoke、路径过滤规则或 required check。 |

文档同步检查：

| 改动类型 | 必须同步考虑 |
| --- | --- |
| 服务能力、stub/501 状态、worker、provider adapter 或 migration 变化 | 对应服务 `docs/implementation.md`。 |
| OpenAPI / Gateway active path / 数据模型变化 | OpenAPI、owner map、README、service boundaries 或相关契约文档；契约语义变化需先交管理组决策。 |
| runtime dependency / Compose / CI 变化 | `technology-decisions.md`、runbook 或本文。 |
| Parser runtime、PaddleOCR、Python packaging、Parser Docker 或 HTTP tests 变化 | Parser README、`technology-decisions.md`、runbook 和本文。 |
| open PR 或未合入能力 | 只能写 pending、待合入或 follow-up，不得写成已实现。 |

## 跨服务 smoke 目标

当前还没有统一 E2E 脚本。#125 完成后至少应覆盖：

1. Auth 创建会话，Gateway 写入 Redis session cache。
2. Gateway 使用认证上下文代理一个 Knowledge/QA/Document active path。
3. File 保存并读取一个基础 file object，业务服务响应不泄露 object key。
4. Knowledge 调用 Parser `/internal/v1/parsed-documents`，解析结果只返回规范化 text/page/block 和脱敏 metadata，不泄露 object key、内部 URL 或 provider debug body。
5. AI Gateway 创建 chat、embedding、rerank profile，并通过 fake provider 完成三类调用。
6. QA 创建 session/message，非流式和 SSE 路径都能保存 response run 和事件摘要。
7. Document 创建 report/job，worker 推进 attempt/event；真实生成落地后再验证 AI Gateway 和 File Service。
8. 前端 typed client 能在 Gateway OpenAPI 更新后重新生成并通过 check/build。

## PR 记录要求

PR body 的检查部分要写具体命令和结果。示例：

```text
已运行：
- git diff --check
- cd services/ai-gateway && go test ./...
- python3 scripts/verify_gateway_active_api.py

未运行：
- DOCUMENT_TEST_DATABASE_URL integration tests；原因：本次未改 document SQL/repository，且本地未启动 document PostgreSQL。
```

如果只写“已测试”而没有命令，reviewer 无法判断覆盖范围。对于因为环境缺失而未运行的检查，应写明缺什么环境和残余风险。
