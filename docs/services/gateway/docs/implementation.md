# Gateway 实现说明

版本：v0.2
日期：2026-06-30
范围：`services/gateway/` 当前实现、契约对齐、缺口和后续实现约束

## 1. 文档定位

本文档描述 `gateway` 当前实现状态和后续实现约束。它只补充服务 README、OpenAPI、架构和技术选型文档，不覆盖这些上游契约。

权威来源：

| 类型 | 权威来源 | 本文档关系 |
| --- | --- | --- |
| 服务公开说明 | `docs/services/gateway/README.md` | 只能补充，不能覆盖 |
| 服务 OpenAPI | `docs/services/gateway/api/openapi.yaml` | 前端公开契约权威来源 |
| Active owner map | `docs/services/gateway/docs/active-api-owner-map.md` | 路由审计清单 |
| 服务边界 | `docs/architecture/service-boundaries.md` | 必须遵守 |
| 技术基线 | `docs/architecture/technology-decisions.md` | 必须跟随 |
| 代码实现 | `services/gateway/` | 本文档记录当前状态和差距 |

凡是本文档与上表文件冲突，以上游文件为准；发现冲突时，在“文档与实现出入”中记录并生成回写或实现任务。

## 2. 当前结论

| 项目 | 状态 | 说明 |
| --- | --- | --- |
| 文档状态 | active | README、OpenAPI、active owner map 和数据模型文档存在。 |
| 代码状态 | partial | Go gateway、auth public routes、Redis session cache、proxy route matrix、中间件和错误归一化已实现。 |
| 契约对齐 | guarded / partial | route matrix 覆盖 97 个 active operations，并校验 method/path/owner/operationId 与 OpenAPI 一致；admin parser-configs 已转为 Knowledge proxy，剩余若干 Knowledge document/retrieval routes 仍被标为 `NotImplemented`。 |
| 数据持久化 | redis / none | Gateway 不持久化业务数据库；使用 Redis 保存 session cache。 |
| 测试状态 | partial | 单元测试覆盖 route matrix、auth proxy、headers、binary/SSE proxy、中间件；缺真实 Redis/downstream 集成测试。 |
| 建议动作 | 补实现 / 回写文档 | 处理 active 但 501 的 routes，并补端到端联调验证。 |

## 3. 已实现

| 能力 | 代码位置 | 契约来源 | 验证方式 | 备注 |
| --- | --- | --- | --- | --- |
| 健康检查 | `services/gateway/internal/http/server.go` | `docs/services/gateway/api/openapi.yaml` | `cd services/gateway && go test ./...` | `GET /healthz`、`GET /readyz`。 |
| 用户/会话公开入口 | `services/gateway/internal/http/auth.go` | Gateway OpenAPI auth paths | `TestCreateSessionCachesSessionWithoutRawToken` | 转发 auth，成功后写 Redis session cache。 |
| Redis session cache | `services/gateway/internal/platform/redis/session_store.go` | `docs/services/gateway/README.md` | config/auth proxy tests | 使用 token hash key，不缓存原始 token。 |
| 认证上下文注入 | `services/gateway/internal/http/proxy.go` | `frontend-backend-contract.md` | `TestProxyInjectsAuthenticatedContextHeaders` | 注入 `X-User-*`、`X-Request-Id`、`X-Service-Token`。 |
| active route matrix | `services/gateway/internal/http/routes.go` | Gateway OpenAPI / owner map | `TestActiveRouteMatrixCoversGatewayOwnerMap` | 覆盖 gateway/auth 直接路由和 owner proxy routes；校验 97 个 active operations 的 method/path/owner/operationId。 |
| binary content proxy | `services/gateway/internal/http/proxy.go` | Gateway OpenAPI file content paths | `TestProxyStreamsBinaryContentWithoutJSONEnvelope` | 文件流成功响应不包 JSON。 |
| SSE proxy | `services/gateway/internal/http/proxy.go` | QA SSE contract | `TestProxyStreamsSSEWithoutFixedTimeout` | `Accept: text/event-stream` 使用 streaming client。 |
| CORS / body limit / timeout / recover / request id | `services/gateway/internal/middleware/` | 前后端集成契约 | middleware/server tests | 覆盖基础 edge policy。 |
| 服务边界导入守卫 | `services/gateway/internal/http/routes_internal_test.go` | 服务边界 / 技术基线 | `TestGatewayDoesNotImportBusinessInfrastructureClients` | 防止 Gateway 生产代码引入 SQL、MinIO、Qdrant 或 provider SDK。 |
| Admin parser config proxy | `services/gateway/internal/http/routes.go`、`parser_config_test.go` | Gateway OpenAPI admin runtime config | `cd services/gateway && go test ./...` | 转发 `/api/v1/admin/parser-configs` 到 Knowledge `/internal/v1/parser-configs`，支持管理员权限、request id、validation/conflict/error 归一化。 |

## 4. 未实现

| 缺口 | 文档来源 | 影响范围 | 建议任务 |
| --- | --- | --- | --- |
| 多个 active Knowledge document/retrieval routes 返回 501 | Gateway OpenAPI active paths | API / frontend / knowledge | 待确认：补下游实现或调整 active 状态。 |
| 管理概览/跨服务指标聚合契约缺失 | `x-missing-contracts`、owner map | API / frontend | 待确认：先定契约再实现。 |
| 真实依赖 ready/smoke 未验证 | README / deploy expectation | deploy / integration | 待确认：补 Redis + auth + owner services smoke。 |

## 5. 文档与实现出入

| 出入点 | 文档要求 | 当前实现 | 风险 | 建议处理 |
| --- | --- | --- | --- | --- |
| Active OpenAPI vs 501 | OpenAPI 和 owner map 将 97 operations 列为 active | `routes.go` 中 5 个 Knowledge document/retrieval operations 标为 `NotImplemented`；admin parser-configs 已取消 501 并代理到 Knowledge | 前端可生成方法但调用剩余未落地路径会得到 501 | 对应 owner service 补实现，或在契约/owner map 标注阶段性不可调用。 |
| readyz 依赖 | Gateway README 要求统一入口可用 | `gatewayReadyCheck` 要求 Redis、auth、knowledge、qa、document、ai-gateway base URL 全配置 | 本地只启动 gateway 时 `/readyz` 易失败 | README/implementation 保留该行为，补本地 smoke 配置。 |
| 下游错误归一化 | 前后端契约要求统一 error envelope | proxy 会丢弃非公开错误细节并归一化 | 有利于安全，但可能隐藏调试信息 | 在日志/trace 中补 request id 和 dependency 信息。 |
| Gateway 不写业务逻辑 | 服务边界要求 Gateway 不访问 SQL/MinIO/Qdrant/LLM | 当前代码符合 | 无 | 持续通过 review/测试防回归。 |

## 6. MVP / mock / memory backend / 占位

| 项目 | 当前用途 | 退出条件 | 关联任务 |
| --- | --- | --- | --- |
| `NotImplemented` route flag | 为已进入 OpenAPI 但下游未完成的 paths 返回稳定 501 | 下游实现完成或 OpenAPI 降级 | 仅剩 document PATCH/DELETE、chunks、content、knowledge-queries 等 Knowledge routes |
| test memory session store | Gateway auth/proxy 单元测试 | 保留测试用 | 无 |

## 7. 运行与配置

| 项目 | 当前状态 | 缺口 |
| --- | --- | --- |
| 启动命令 | `cd services/gateway && go run ./cmd/server` | 需要 Redis、auth 和 owner base URLs 才能 ready。 |
| 环境变量 | `GATEWAY_HTTP_ADDR`、Redis、token hash secret、auth/knowledge/qa/document/ai-gateway base URLs、CORS、timeouts | 缺根级 Compose 串联验证。 |
| PostgreSQL / migration | 不拥有 PostgreSQL | 无。 |
| Redis / queue | Redis session cache | 缺真实 Redis 集成测试。 |
| Object storage / vector store / AI provider | 不直接访问 | 必须继续由 owner services / ai-gateway 处理。 |

## 8. 测试与验证

| 验证项 | 命令或步骤 | 当前结果 | 缺口 |
| --- | --- | --- | --- |
| 单元测试 | `cd services/gateway && go test ./...` | pass（本次执行） | 不覆盖真实 Redis/downstreams。 |
| 集成测试 | Gateway + Redis + auth + owner services smoke | missing | 需要本地 Compose 或脚本。 |
| 契约测试 | `TestActiveRouteMatrixCoversGatewayOwnerMap`、`TestNotImplementedRoutesReturnStableGatewayError`、`TestGatewayDoesNotImportBusinessInfrastructureClients` | pass（本次执行） | 仍不覆盖真实 Redis/downstreams。 |
| 手工 smoke | 登录、访问 knowledge/report/qa route | not run | 需要完整依赖环境。 |

## 9. 建议任务

| 任务 | 类型 | 优先级 | 依据 | 说明 |
| --- | --- | --- | --- | --- |
| 清零 Gateway active 501 routes | 修改既有任务 | P0 | Active contract 可调用性 | 按 owner service 实现或回写契约状态。 |
| 增加 Gateway integration smoke | 新任务 | P1 | readyz 和 proxy 依赖真实服务 | 覆盖 Redis/auth/owner base URL。 |
| 扩展 OpenAPI schema-level route test | 后续增强 | P2 | 当前已校验 method/path/owner/operationId | 可继续补充参数、响应 content type 和 security 字段漂移检查。 |

## 10. 最近检查记录

| 日期 | 检查人/工具 | 代码基准 | 结论 |
| --- | --- | --- | --- |
| 2026-06-29 | Codex goal | `eddf917` + working tree | Gateway 架构边界清晰，route matrix 覆盖 97 active operations；主要风险是 active contract 中仍有多条 501 占位。 |
| 2026-06-30 | Codex | `8f294ec` + 本分支改动 | route matrix 已显式覆盖 gateway/auth 直接路由和 owner proxy routes，并与 OpenAPI 的 method/path/owner/operationId 对齐；501 占位和 Gateway 不直连业务基础设施均有回归测试。 |
| 2026-06-30 | Codex | A-13 PR #249 | admin parser-configs 已完成 Gateway proxy 和权限/error 测试，不再属于 501 占位范围。 |
