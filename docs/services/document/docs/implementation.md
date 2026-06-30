# Document 服务实现说明

版本：v0.2
日期：2026-06-30
范围：`services/document/` 当前实现、契约对齐、缺口和后续实现约束

## 1. 文档定位

本文档描述 `document` 当前实现状态和后续实现约束。它只补充服务 README、OpenAPI、架构和技术选型文档，不覆盖这些上游契约。

权威来源：

| 类型 | 权威来源 | 本文档关系 |
| --- | --- | --- |
| 服务公开说明 | `docs/services/document/README.md` | 只能补充，不能覆盖 |
| 服务公开 OpenAPI | `docs/services/document/api/public.openapi.yaml` | Document-owned public 设计面；前端稳定契约仍以 gateway 为准 |
| 服务内部 OpenAPI | `docs/services/document/api/internal.openapi.yaml` | 内部运行和 report job contract；只能跟随，不能另起契约 |
| Gateway 公开契约 | `docs/services/gateway/api/public.openapi.yaml` | 前端稳定契约以 gateway 为准 |
| 服务边界 | `docs/architecture/service-boundaries.md` | 必须遵守 |
| 技术基线 | `docs/architecture/technology-decisions.md` | 必须跟随 |
| 代码实现 | `services/document/` | 本文档记录当前状态和差距 |

凡是本文档与上表文件冲突，以上游文件为准；发现冲突时，在“文档与实现出入”中记录并生成回写或实现任务。

## 2. 当前结论

| 项目 | 状态 | 说明 |
| --- | --- | --- |
| 文档状态 | active | README、需求、数据模型、前端 API 设计和 OpenAPI 存在。 |
| 代码状态 | partial | Go service、PostgreSQL repository、`pgx/v5@v5.9.2`、模板/材料/报告/大纲/章节 API、report jobs/attempts/events、report files/content、基础内置 DOCX 导出、asynq worker 状态机、report settings、statistics、operation logs 和 `summer_peak_inspection` 基础 AI 大纲/正文生成编排已实现；剩余缺口为 Document MCP tools、更多报告类型生成策略和 Pandoc/LibreOffice 富 DOCX 工具链。 |
| 契约对齐 | partial | Gateway active document paths 有 43 个；当前 Document active routes 已由服务处理。report job 请求体已按 gateway 的 `target/requirements/materialIds/options` 形态接入；report file content 只有在文件 `succeeded` 且 File Service 已保存内容后可读取。 |
| 数据持久化 | postgres | runtime 使用 PostgreSQL；模板/材料底层文件通过 File Service client。 |
| 测试状态 | partial | service、HTTP、repository tests 存在；集成测试依赖 `DOCUMENT_TEST_DATABASE_URL`。 |
| 建议动作 | 补实现 / 验证 | 优先补 Document MCP tools、AI Gateway/Knowledge/File Service 跨服务 smoke、更多报告类型生成策略和 Pandoc/LibreOffice 富 DOCX 工具链。 |

## 3. 已实现

| 能力 | 代码位置 | 契约来源 | 验证方式 | 备注 |
| --- | --- | --- | --- | --- |
| 健康/就绪检查 | `services/document/internal/http/server.go` | Document OpenAPI | `cd services/document && go test ./...` | `/readyz` 检查 repository。 |
| 报告类型 | `internal/service/document.go`、`internal/http/types_handlers.go` | Gateway / Document OpenAPI | HTTP tests | `GET /report-types`。 |
| 报告模板 CRUD 和结构 | `internal/http/template_handlers.go`、`internal/service/document.go` | Document README | HTTP/service tests | 创建模板时调用 File Service 保存文件。 |
| 报告材料 CRUD | `internal/http/material_handlers.go`、`internal/service/document.go` | Document README | HTTP/service tests | 创建材料时调用 File Service 保存文件。 |
| 报告记录 CRUD | `internal/http/reports.go`、`internal/service/report_service.go` | Gateway / Document OpenAPI | `TestCreateReportThenGetByOwner` 等 | 含权限和软删除规则。 |
| 大纲和章节 | `internal/service/report_service.go`、`internal/service/outline.go` | Document README | outline/report service tests | 支持大纲版本、章节树、编号、章节版本。 |
| report jobs / attempts / events | `internal/http/job_handlers.go`、`internal/service/job_service.go` | Gateway / Document OpenAPI | job service/http tests | 支持创建任务、查询任务、重试、列出尝试和事件。 |
| asynq client / worker 状态机 | `internal/worker/client.go`、`internal/worker/worker.go`、`cmd/server/main.go` | 技术基线 / Document README | worker/job tests | 创建任务时入队，worker 更新 job/attempt running/succeeded/failed/partial_succeeded；`report_file_creation` 执行基础 DOCX 导出，非文件类生成 job 调用报告生成 executor。 |
| AI 大纲/正文生成编排 | `internal/service/report_generation_service.go`、`internal/platform/aigateway/chat_client.go`、`cmd/server/main.go` | Document README / AI Gateway internal API | generation service / AI Gateway client / worker tests | `summer_peak_inspection` 可通过 AI Gateway chat 生成大纲，创建章节骨架，再逐章节生成正文和章节版本；部分失败保留已成功章节。 |
| Knowledge 检索上下文 client | `internal/platform/knowledgeclient/client.go`、`internal/service/report_generation_service.go` | Knowledge internal API / 服务边界 | knowledge client / generation service tests | 当 job payload 的 `options` 或 `retrieval` 包含 `knowledgeBaseIds` 且配置了 Knowledge URL 时调用 `/internal/v1/knowledge-queries`；prompt 只使用安全 `contentPreview`。 |
| report files / content | `internal/http/report_files.go`、`internal/service/report_file_service.go`、`internal/worker/worker.go` | Gateway / Document OpenAPI | report file service/http/worker tests | `POST /report-files` 创建文件元数据和异步任务；`report_file_creation` worker 使用内置 `SimpleDOCXGenerator` 从已保存章节生成基础 DOCX，上传 File Service 后 content endpoint 可读取。 |
| report settings | `internal/http/admin_handlers.go`、`internal/service/admin_service.go`、`internal/repository/admin.go` | Gateway / Document OpenAPI | HTTP/service/repository tests | 持久化 AI Gateway profile 引用、默认模板和文件默认值；`PATCH` 仅 admin/super_admin。 |
| statistics / operation logs | `internal/http/admin_handlers.go`、`internal/service/admin_service.go`、`internal/repository/admin.go` | Gateway / Document OpenAPI | HTTP/service/repository tests | 支持概览、每日趋势和操作日志过滤；日志写入路径做敏感字段脱敏。 |
| AI Gateway profile client | `internal/platform/aigateway/profile_client.go`、`cmd/server/main.go` | AI Gateway internal API | client/config tests | Document 只校验并引用 profile，不保存 provider key。 |
| PostgreSQL repository | `internal/repository`、`migrations/0001_create_report_generation_tables.sql` | 数据模型 | repository tests | runtime 使用 `pgx/v5`。 |
| File Service client | `internal/platform/fileclient` | File/Document 边界 | fileclient tests | multipart 创建 file，delete cleanup。 |

## 4. 未实现

| 缺口 | 文档来源 | 影响范围 | 建议任务 |
| --- | --- | --- | --- |
| Document MCP tools | Document README / requirements | QA / tool integration | 注册工具、权限校验、参数校验、脱敏输出和调用链路仍未实现。 |
| 更多报告类型生成策略 | Document README / requirements | worker / report content | 当前基础 AI 闭环覆盖 `summer_peak_inspection`；`coal_inventory_audit` 仍需补业务 prompt、模板和验收样例后再开放。 |
| Pandoc / LibreOffice rich DOCX generation | Document README / 技术基线 | rich DOCX | 当前只提供内置 Go 基础 DOCX；落地富 DOCX 前不得承诺 Pandoc/LibreOffice 转换已可用。 |

## 5. 文档与实现出入

| 出入点 | 文档要求 | 当前实现 | 风险 | 建议处理 |
| --- | --- | --- | --- | --- |
| Active document paths | Gateway OpenAPI 将 jobs/files/statistics/logs/settings 设为 active | jobs/attempts/events、settings/statistics/logs、report files/content 和基础 AI 大纲/正文生成已实现；content 在文件未完成或缺少 File Service 内容时返回未就绪错误 | 前端可联调基础 AI 生成和基础文件导出，但不能把文件导出理解为富 DOCX | 补 AI Gateway + Knowledge + File Service + Redis 的跨服务 smoke，保留 content 未就绪错误语义。 |
| Redis/asynq | README 要求使用 asynq over Redis 执行报告任务 | `cmd/server` 已创建 asynq client/worker，任务创建会入队并持久化 task id；文件生成 job 执行基础 DOCX 导出，生成类 job 调用 AI Gateway chat executor | 运行时需要 Redis 和 AI Gateway；Knowledge 检索为可选依赖 | 补跨服务 smoke 和更多报告类型。 |
| AI Gateway/Pandoc/LibreOffice | README 描述生成和导出依赖 | AI Gateway chat 已用于基础大纲/正文生成；report file creation 当前使用内置 Go 生成器，不调用 Pandoc/LibreOffice | 部署方仍不能期待富 DOCX 转换；AI Gateway profile/可用性决定生成任务是否成功 | 在 implementation 中标明 AI 生成已落地、富 DOCX 未落地。 |
| Document MCP tools | README/requirements 描述后续可注册 Document MCP 工具 | 当前没有 Document MCP tool registry、handler 或 QA 调用链路 | 后续排期容易漏掉 MCP tools，或误以为 README 中的工具已可用 | 在本文未实现任务表单列；拆实现任务。 |
| Service path prefix | Gateway public paths 是 `/api/v1/report-*` | Document service 本地 routes 无 `/internal/v1` 前缀，gateway 默认剥离 `/api/v1` | 这与 gateway proxy 逻辑一致但易误解 | README/implementation 明确 document local path 形态。 |
| `go-redis` 传递依赖版本 | 技术基线固定直接 Redis client 为 `go-redis/v9@v9.21.0` | Document 通过 `asynq v0.26.0` 间接带入 `go-redis/v9@v9.14.1`；本次版本修复不改非 Docker 代码依赖 | 文档基线和锁定依赖存在出入，后续队列依赖升级时可能被忽略 | 下次升级 asynq 或调整 worker queue 依赖时优先消除该出入；不能消除时继续在本文记录原因。 |

## 6. MVP / mock / memory backend / 占位

| 项目 | 当前用途 | 退出条件 | 关联任务 |
| --- | --- | --- | --- |
| `handleNotImplemented` helper | 历史占位 helper；当前 active routes 不再挂到该 handler | 删除未使用 helper，或后续新增 pending route 前同步 route coverage 和状态文档 | cleanup follow-up |
| generation fixed-type scope | 首个 AI 生成闭环只覆盖 `summer_peak_inspection` | `coal_inventory_audit` 有业务 prompt、模板和样例验收后再开放 | follow-up |
| fake repositories in tests | service/http 单元测试 | 保留测试用 | 无 |
| env-gated repository integration tests | 无 DB 环境跳过 | CI 提供 `DOCUMENT_TEST_DATABASE_URL` | testing required checks 分阶段升级任务 |

## 7. 运行与配置

| 项目 | 当前状态 | 缺口 |
| --- | --- | --- |
| 启动命令 | `cd services/document && go run ./cmd/server` | 需要 PostgreSQL、Redis、File Service 和多个预留 env。 |
| 环境变量 | `DOCUMENT_DATABASE_URL`、`DOCUMENT_REDIS_ADDR`、`DOCUMENT_FILE_SERVICE_URL`、`DOCUMENT_AI_GATEWAY_URL`、`DOCUMENT_AI_GATEWAY_PROFILE_ID`、可选 `DOCUMENT_AI_GATEWAY_SERVICE_TOKEN` / `DOCUMENT_KNOWLEDGE_SERVICE_URL` / `DOCUMENT_KNOWLEDGE_SERVICE_TOKEN` / `INTERNAL_SERVICE_TOKEN`、Pandoc/LibreOffice paths | Redis 已用于 asynq；AI Gateway profile/chat client 已用于 settings 校验和基础生成；Knowledge 为按请求启用的可选检索依赖；Pandoc/LibreOffice 当前未实际使用。 |
| PostgreSQL / migration | `migrations/0001_create_report_generation_tables.sql`，`sqlc.yaml`，runtime repository | 需要 migration CI/smoke。 |
| Redis / queue | asynq client/worker 已接入 report job enqueue/status lifecycle 和 AI 生成 executor | 需要 Redis + AI Gateway smoke。 |
| Object storage / vector store / AI provider | 模板/材料和基础 report file DOCX 通过 File Service；AI 生成通过 AI Gateway；按请求可调用 Knowledge 检索 | Knowledge/File/AI Gateway 跨服务 smoke 和富 DOCX export 未实现。 |

## 8. 测试与验证

| 验证项 | 命令或步骤 | 当前结果 | 缺口 |
| --- | --- | --- | --- |
| 单元测试 | `cd services/document && go test ./... -count=1` | pass（本次执行） | env-gated repository DB tests 仍需 `DOCUMENT_TEST_DATABASE_URL`。 |
| 构建 | `cd services/document && go build ./cmd/server` | pass（本次执行） | 无。 |
| 契约/配置解析 | OpenAPI YAML parse、`docker compose --env-file .env.example config --quiet`、`git diff --check` | pass（本次执行） | 仍需 gateway route matrix 在 CI 中复核。 |
| 集成测试 | `DOCUMENT_TEST_DATABASE_URL=... go test ./internal/repository` | not run | 需要 PostgreSQL。 |
| 跨服务 smoke | AI Gateway / Knowledge / File Service / Redis through worker | not run | 需要 gateway/auth/file/document/ai-gateway/knowledge 联调环境。 |

## 9. 建议任务

| 任务 | 类型 | 优先级 | 依据 | 说明 |
| --- | --- | --- | --- | --- |
| 实现 Document MCP tools | 新任务 | P0 | README / requirements 已保留工具目标 | 注册 `generate_report_outline`、`generate_report_text`、`get_generation_status`、`get_report_result`、`export_report_docx` 等工具，并覆盖权限和脱敏输出。 |
| 补更多报告类型生成策略 | 新任务 | P1 | 需求覆盖两类固定报告 | 在 `coal_inventory_audit` 的模板、prompt 和样例验收准备好后接入 AI 生成。 |
| 补 AI Gateway / Knowledge 跨服务 smoke | 测试 / runbook | P1 | 基础 AI 生成已在服务内闭环 | 用可控 AI Gateway fixture 和 Knowledge fixture 验证请求头、任务进度、脱敏错误和 partial_succeeded。 |
| 补 report files/content 跨服务 smoke | 测试 / runbook | P1 | 基础 DOCX 导出已在服务内闭环 | 用 PostgreSQL、Redis、File Service 和 document worker 验证 `POST /report-files` 到 content endpoint 的完整链路。 |
| 回写富 DOCX 预留配置状态 | 回写文档 | P1 | Pandoc/LibreOffice env 当前要求但未使用 | 防部署误判。 |

## 10. 最近检查记录

| 日期 | 检查人/工具 | 代码基准 | 结论 |
| --- | --- | --- | --- |
| 2026-06-30 | Codex C-005 implementation | working tree | Document 已补 `summer_peak_inspection` 基础 AI 大纲/正文生成编排、AI Gateway chat client、可选 Knowledge 检索 client、生成任务 payload 持久化和 OpenAPI/状态文档同步；Document MCP tools、更多报告类型和 Pandoc/LibreOffice 富 DOCX 仍是缺口。 |
| 2026-06-30 | Codex full-day audit | `develop@92d3afc` | 复核今日 PR/issue：Document 已包含 report jobs/attempts/events、基础 DOCX report file creation、settings/statistics/logs、`pgx/v5@v5.9.2` 和安全依赖更新；#101 真实大纲/正文生成、#307 富 DOCX worker toolchain、Document MCP tools 和跨服务 content smoke 当时仍待补齐。 |
| 2026-06-30 | Codex PR #265 review follow-up | working tree | 当时 Document 状态文档已与 report files/content 和基础内置 DOCX 导出对齐；生成编排、Document MCP tools 和 Pandoc/LibreOffice 富 DOCX 仍待后续任务。 |
| 2026-06-30 | Codex C-08 redo | `31711d9` + working tree | 当时 Document 已补 report settings、statistics、operation logs、AI Gateway profile validation 和日志脱敏写入；report files/content、生成编排、Document MCP tools 和 Pandoc/LibreOffice 富 DOCX 仍待后续任务。 |
| 2026-06-29 | Codex after proxy rebase | `0e402ca` + working tree | 当时 Document 已补 report jobs/attempts/events 和 asynq worker 状态机；报告文件、统计/settings、生成编排和 DOCX 导出仍是主要缺口。 |
| 2026-06-29 | Codex goal | `eddf917` + working tree | Document 已有模板、材料、报告、大纲、章节基础能力；当时生成任务、报告文件、统计/settings/worker 仍是主要缺口，后续 `develop` 已补 jobs/worker 状态机。 |
