# QA 服务实现说明

版本：v0.1
日期：2026-06-29
范围：`services/qa/` 当前实现、契约对齐、缺口和后续实现约束

## 1. 文档定位

本文档描述 `qa` 当前实现状态和后续实现约束。它只补充服务 README、OpenAPI、架构和技术选型文档，不覆盖这些上游契约。

权威来源：

| 类型 | 权威来源 | 本文档关系 |
| --- | --- | --- |
| 服务公开说明 | `docs/services/qa/README.md` | 只能补充，不能覆盖 |
| 服务 OpenAPI | `services/qa/api/openapi.yaml`、`docs/services/qa/api/openapi.yaml` | 只能跟随，不能另起契约 |
| Gateway 公开契约 | `docs/services/gateway/api/openapi.yaml` | 前端稳定契约以 gateway 为准 |
| 服务边界 | `docs/architecture/service-boundaries.md` | 必须遵守 |
| 技术基线 | `docs/architecture/technology-decisions.md` | 必须跟随 |
| 代码实现 | `services/qa/` | 本文档记录当前状态和差距 |

凡是本文档与上表文件冲突，以上游文件为准；发现冲突时，在“文档与实现出入”中记录并生成回写或实现任务。

## 2. 当前结论

| 项目 | 状态 | 说明 |
| --- | --- | --- |
| 文档状态 | active | README、数据模型、公开设计 OpenAPI 和服务内部 OpenAPI 存在。 |
| 代码状态 | partial / B-03 branch covered | Go service、PostgreSQL repository、QA sessions/messages/SSE、资源查询、settings、MCP/model tooling 已实现；本分支补齐 B-03 ResponseRun 与非流式 Agent Loop MVP。 |
| 契约对齐 | partial | Gateway 25 个 QA active operations 均有 proxy route；QA 内部 routes 也注册，模型调用通过 AI Gateway chat completions；端到端 RAG 仍依赖 Knowledge retrieval。 |
| 数据持久化 | postgres | runtime 使用 PostgreSQL；配置 secret 使用本地加密 key。 |
| 测试状态 | covered / partial | 单元测试覆盖 service、repository mapping、HTTP、MCP/model/local tools；缺完整依赖端到端。 |
| 建议动作 | 补联调 / 回写文档 | 补 Knowledge retrieval 可用前的降级说明，并验证 QA -> AI Gateway -> provider 的跨服务 smoke。 |

## 3. 已实现

| 能力 | 代码位置 | 契约来源 | 验证方式 | 备注 |
| --- | --- | --- | --- | --- |
| 健康/就绪检查 | `services/qa/internal/http/server.go` | `services/qa/api/openapi.yaml` | `cd services/qa && go test ./...` | `/readyz` 使用 repo ping。 |
| QA session CRUD | `services/qa/internal/http/server.go`、`internal/service/qa.go` | Gateway OpenAPI QA paths | HTTP/service tests | 创建、列表、详情、更新、删除。 |
| 消息创建与 SSE | `services/qa/internal/http/server.go`、`internal/service/qa.go` | Gateway OpenAPI | `TestStreamUsesContractEventNames` | 支持 `Accept: text/event-stream`。 |
| response runs / tool calls / citations | `services/qa/internal/http/resource_handlers.go`、`internal/service/resources.go` | Gateway OpenAPI | service/repository tests | 返回脱敏资源摘要。 |
| QA/LLM config versions | `services/qa/internal/http/resource_handlers.go`、`internal/service/settings.go` | Gateway OpenAPI | config/settings tests | 配置版本持久化并加密敏感字段。 |
| retrieval test / metrics | `services/qa/internal/http/resource_handlers.go` | Gateway OpenAPI | resource tests | 依赖 Knowledge retrieval client。 |
| B-03 非流式 Agent Run MVP | `services/qa/internal/service/qa.go`、`internal/service/agent`、`internal/repository` | #89 / QA README / QA 数据模型 | service、repository、modelclient tests | 创建用户消息、助手占位、response run、初始事件和模型调用摘要；落库 `completed`、`model_error`、`timeout`、`cancelled`、`max_iterations` 等终止原因。 |
| AI Gateway chat client | `services/qa/internal/platform/modelclient/openai.go` | QA README / AI Gateway OpenAPI | modelclient tests | 发送 OpenAI-compatible chat request，透传 `X-Caller-Service: qa` 和 request id，支持 `profile_id`。 |
| MCP client/tooling | `services/qa/internal/platform/mcpclient`、`localtools` | QA README | platform tests | 支持 stdio、streamable HTTP、内置工具。 |
| PostgreSQL schema/repository | `services/qa/migrations/*.sql`、`internal/repository` | QA 数据模型 | repository tests | 有 integration tests，但依赖 `QA_TEST_DATABASE_URL`。 |

## 4. 未实现

| 缺口 | 文档来源 | 影响范围 | 建议任务 |
| --- | --- | --- | --- |
| 依赖的 Knowledge `/internal/v1/knowledge-queries` 未在 Knowledge 实现 | `docs/services/gateway/api/openapi.yaml`、QA RAG 流程 | QA / Knowledge / frontend | 拆 Knowledge retrieval / Qdrant / embedding-rerank 闭环任务；QA 保持降级和依赖说明。 |
| AI Gateway chat 已实现但未做跨服务 smoke | `docs/services/ai-gateway/api/openapi.yaml` | QA / AI Gateway | 补 QA -> AI Gateway -> fake/real provider smoke。 |
| 真实 MCP/Knowledge/Model 端到端测试未证明 | QA README | integration | 补 Compose 或 smoke；在根级联调环境完成前不写成 required。 |
| AI Gateway service-token 配置需联调 | QA config / AI Gateway middleware | QA / AI Gateway / deploy | 验证 `AI_GATEWAY_TOKEN` 缺省复用 `INTERNAL_SERVICE_TOKEN` 与 AI Gateway token hashes 一致，并补 profile seed 说明。 |

## 5. 文档与实现出入

| 出入点 | 文档要求 | 当前实现 | 风险 | 建议处理 |
| --- | --- | --- | --- | --- |
| 模型调用边界 | 文档要求业务服务通过 AI Gateway 调模型 | `services/qa/internal/config/config.go` 默认 `AI_GATEWAY_URL=http://localhost:8086/internal/v1/chat/completions`，token header 默认 `X-Service-Token`，不再要求 `DEEPSEEK_API_KEY` fallback | 与架构方向一致；仍需部署联调 token hash 和 caller header | 补 QA -> AI Gateway smoke。 |
| Knowledge retrieval dependency | QA 文档将检索作为 RAG 主路径 | Knowledge 当前未实现 `knowledge-queries` | QA 问答闭环无法真实检索 | 补 Knowledge retrieval 或 QA mock/fallback 状态说明。 |
| Gateway active QA paths | Gateway 25 个 QA operations active | QA 内部 routes 全注册 | route 层对齐，但业务结果依赖外部服务 | 增加跨服务 contract smoke。 |
| MCP 原始信息不得暴露 | 文档要求只返回脱敏摘要 | 代码有 tool-call summary 和 local tool safety tests | 当前方向一致 | 持续补审计和字段级契约测试。 |
| B-03 Agent Run 状态 | README 描述 Agent Run、termination 和 maxIterations；#229 要求未合入能力不得写成 develop 事实 | 本分支将 B-03 ResponseRun、终止原因、模型调用摘要和基础测试一起提交；合入后可视为当前实现，未合入前 PR 描述需说明仍是待合入能力 | 如果只合代码不合文档会造成状态漂移 | 本文档和能力矩阵随本 PR 更新。 |

## 6. MVP / mock / memory backend / 占位

| 项目 | 当前用途 | 退出条件 | 关联任务 |
| --- | --- | --- | --- |
| built-in/local tools | 无外部 MCP server 时支持开发调试 | 生产工具白名单和 MCP server 稳定后限制启用 | 后续工具白名单 / MCP 运维任务 |
| AI Gateway default endpoint | 未显式配置时使用本地 AI Gateway chat completions | 环境差异需要部署文档明确覆盖 | QA -> AI Gateway smoke / profile seed 任务 |
| repository integration tests gated by env | 避免无 DB 环境失败 | CI 提供 `QA_TEST_DATABASE_URL` | testing required checks 分阶段升级任务 |

## 7. 运行与配置

| 项目 | 当前状态 | 缺口 |
| --- | --- | --- |
| 启动命令 | `cd services/qa && go run ./cmd/server` | 需要 PostgreSQL、模型 endpoint、Knowledge URL。 |
| 环境变量 | `QA_DATABASE_URL`、`QA_HTTP_ADDR`、`KNOWLEDGE_SERVICE_URL`、`INTERNAL_SERVICE_TOKEN`、`AI_GATEWAY_URL`/token、MCP、tool limits、settings flags | 需统一命名和 secret 注入说明。 |
| PostgreSQL / migration | `migrations/0001` 到 `0004`，`sqlc.yaml`，runtime repository | 需要 CI migration apply 证据。 |
| Redis / queue | 当前交互式主路径不使用队列 | 后续离线任务再接 asynq。 |
| Object storage / vector store / AI provider | 通过 Knowledge/AI Gateway/MCP 间接访问 | 需补 QA -> AI Gateway/provider smoke。 |

## 8. 测试与验证

| 验证项 | 命令或步骤 | 当前结果 | 缺口 |
| --- | --- | --- | --- |
| 单元测试 | `cd services/qa && go test ./internal/repository ./internal/service ./internal/service/agent ./internal/platform/modelclient` | pass（本次执行） | 真实 DB tests 可能被 env gate 跳过。 |
| 服务构建 | `cd services/qa && go build -buildvcs=false ./cmd/server && go build -buildvcs=false ./cmd/agent` | pass（本次执行） | `-buildvcs=false` 用于规避本地 worktree VCS stamping。 |
| 集成测试 | `QA_TEST_DATABASE_URL=... go test ./internal/repository` | not run | 需要 PostgreSQL。 |
| 契约测试 | Gateway route matrix + QA HTTP tests | partial | 未从 OpenAPI 自动校验全部 schema。 |
| 手工 smoke | Gateway -> QA session -> message stream | not run | 需要 Auth/Gateway/Redis/Knowledge/Model。 |

## 9. 建议任务

| 任务 | 类型 | 优先级 | 依据 | 说明 |
| --- | --- | --- | --- | --- |
| 补 QA -> AI Gateway 模型调用 smoke | 新任务 | P0 | AI Gateway-only 架构规则 | 覆盖 token header、caller service、profile id、provider failure mapping。 |
| 补 QA + Knowledge retrieval 联调 | 新任务 | P0 | RAG 主链路 | 覆盖 no result、dependency_error、citation snapshot。 |
| #89 合入后确认 Agent Run 状态 | 回写文档 | P0 | 文档/代码出入评审结论 | 确认本文和能力矩阵在 `develop` 基线上保留 B-03 实现状态，不把真实 RAG 闭环误写成已完成。 |
| 补 QA OpenAPI schema contract test | 新任务 | P1 | active paths 已多 | 防字段漂移。 |

## 10. 最近检查记录

| 日期 | 检查人/工具 | 代码基准 | 结论 |
| --- | --- | --- | --- |
| 2026-06-29 | Codex #89 branch | `31711d9` + working tree | B-03 非流式 Agent Run MVP 覆盖成功、模型失败、超时、取消和 max-iterations；response_run、assistant message、初始事件和模型调用摘要保持一致。剩余风险为 Knowledge retrieval、跨服务 smoke 和 env-gated DB integration。 |
| 2026-06-29 | Codex after proxy rebase | `0e402ca` + working tree | QA route 层基本对齐，config 默认走 AI Gateway chat；主要剩余风险在 Knowledge retrieval 未完成和跨服务 smoke 未跑。 |
| 2026-06-29 | Codex after rebase | `808c589` + working tree | QA route 层基本对齐，AI Gateway chat 下游已落地；当时主要剩余风险在 Knowledge retrieval 未完成、跨服务 smoke 未跑和 direct provider fallback 边界，后续 `develop` 已移除 DeepSeek fallback。 |
| 2026-06-29 | Codex goal | `eddf917` + working tree | QA 代码量已较完整，route 层基本对齐；当时主要风险在 Knowledge/AI Gateway 下游未完成和 direct provider fallback 边界。 |
