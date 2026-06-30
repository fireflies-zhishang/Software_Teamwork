# Knowledge Service 实现说明

版本：v0.6
日期：2026-06-30
范围：`services/knowledge/` 当前实现、契约对齐、缺口和后续实现约束

## 1. 文档定位

本文档描述 `knowledge` 当前实现状态和后续实现约束。它只补充服务 README、OpenAPI、架构和技术选型文档，不覆盖这些上游契约。

权威来源：

| 类型 | 权威来源 | 本文档关系 |
| --- | --- | --- |
| 服务公开说明 | `docs/services/knowledge/README.md` | 只能补充，不能覆盖 |
| 服务 OpenAPI | `services/knowledge/api/openapi.yaml` | 只能跟随，不能另起契约 |
| Gateway 公开契约 | `docs/services/gateway/api/openapi.yaml` | 前端稳定契约以 gateway 为准 |
| 服务边界 | `docs/architecture/service-boundaries.md` | 必须遵守 |
| 技术基线 | `docs/architecture/technology-decisions.md` | 必须跟随 |
| 代码实现 | `services/knowledge/` | 本文档记录当前状态和差距 |

凡是本文档与上表文件冲突，以上游文件为准；发现冲突时，在“文档与实现出入”中记录并生成回写或实现任务。

## 2. 当前结论

| 项目 | 状态 | 说明 |
| --- | --- | --- |
| 文档状态 | active | README、公开草案、数据模型、内部 OpenAPI 和实现说明存在。 |
| 代码状态 | partial | Go service 已实现知识库 CRUD、文档列表/上传/详情、File Service handoff、PostgreSQL repository、asynq enqueue、parser-configs 运行时管理、ingestion worker、Parser Service client、Knowledge-owned chunker、embedding、chunk 持久化、vector index 写入、文档 chunks/content API 和 `knowledge-queries` 检索。 |
| 契约对齐 | partial | Gateway OpenAPI 已声明 chunks、content、knowledge-queries、parser configs；这些 active routes 已由 Knowledge 和 Gateway proxy 落地。文档 PATCH/DELETE 仍为阶段性 `not_implemented`，真实跨依赖 smoke 仍待补齐。 |
| 数据持久化 | postgres / Redis queue / Qdrant | runtime 使用 PostgreSQL；Redis/asynq 负责任务投递；vector index 支持 Qdrant，未配置时使用 in-memory index。 |
| 测试状态 | partial | 单元、handler、contract 和 platform tests 覆盖 CRUD、权限、上传补偿、queue handoff、worker 入库、File content 读取、Parser HTTP client、chunking、embedding、vector payload、parser-configs、chunks/content、`knowledge-queries`、错误 envelope 和 request id；缺真实依赖端到端联调。 |
| 依赖解耦 | documented | A-12 检索和 A-14 契约测试可依赖 `docs/api-contract.md` 2.6 与 `docs/data-models.md` 6.7 的 seeded chunk/vector fixture，不再要求 A-11 worker runtime 先完成。 |
| 建议动作 | 联调 / 人工复审 | 继续补 document PATCH/DELETE、真实 File/Parser/Redis/Qdrant/AI Gateway 端到端 smoke，以及并发/外部副作用一致性加固；人工复审任务幂等、失败状态收敛和敏感数据不泄漏。 |

## 3. 已实现

| 能力 | 代码位置 | 契约来源 | 验证方式 | 备注 |
| --- | --- | --- | --- | --- |
| 健康检查 | `services/knowledge/internal/http/server.go` | `services/knowledge/api/openapi.yaml` | `cd services/knowledge && go test ./...` | `GET /healthz`、`GET /readyz`。 |
| 知识库 CRUD | `services/knowledge/internal/http/server.go`、`internal/service/service.go` | `services/knowledge/api/openapi.yaml` | `TestKnowledgeBaseCRUDAndSoftDelete` | 支持列表、创建、详情、更新、软删除。 |
| 用户上下文和权限校验 | `services/knowledge/internal/service/service.go` | `docs/services/knowledge/README.md` | service tests | 依赖 gateway 注入的 user/permission context。 |
| 文档列表和详情 | `services/knowledge/internal/http/server.go` | `services/knowledge/api/openapi.yaml` | `TestDocumentListAndDetailExcludeDeletedKnowledgeBase` | 只覆盖文档元数据/状态。 |
| 文档上传 handoff | `services/knowledge/internal/platform/fileclient/client.go`、`internal/service/service.go` | `docs/services/knowledge/README.md`、`docs/services/file/README.md` | `TestUploadDocumentCreatesDocumentJobAndQueuesIngestion` | multipart 上传后调用 File `/internal/v1/files`，保存 `file_ref`，创建 processing job。 |
| Parser configs 运行时管理 | `services/knowledge/internal/http/server.go`、`internal/service/parser_config.go`、`internal/repository/postgres.go` | `docs/services/gateway/api/openapi.yaml`、`docs/architecture/service-boundaries.md` | `cd services/knowledge && go test ./...`；repository integration CI | 支持 list/get/create/update/delete、默认 builtin seed、上传 parser snapshot、重复名称 conflict、空配置 fallback 和 MIME 匹配选择。 |
| asynq 入队 | `services/knowledge/internal/platform/queue` | `docs/architecture/technology-decisions.md` | service tests with fake queue | 投递 `knowledge:document:ingest`，retry 次数与 `processing_jobs.max_attempts` 默认值对齐。 |
| 文档入库 worker | `services/knowledge/internal/worker`、`internal/service/ingestion.go` | `docs/services/knowledge/README.md` | worker/service tests | 消费 A10 payload，读取 File content，解析、切片、embedding、写 vector index 和 chunks，并推进 ready/failed 状态。 |
| File content reader | `services/knowledge/internal/platform/fileclient/client.go` | `docs/services/file/README.md` | fileclient tests | 调用 `/internal/v1/files/{fileId}/content`，透传 request/user/service headers，失败脱敏为 dependency error。 |
| 文档 chunks HTTP API | `services/knowledge/internal/http/server.go`、`internal/service/ingestion.go`、`internal/repository/postgres.go` | `docs/services/gateway/api/openapi.yaml`、`services/knowledge/api/openapi.yaml` | `TestDocumentChunksAndContentContract`、Gateway proxy tests | 支持 `GET /internal/v1/documents/{documentId}/chunks`，分页返回 Knowledge-owned chunk DTO，不暴露原始向量或 Qdrant payload。 |
| 原始文档 content API | `services/knowledge/internal/http/server.go`、`internal/service/service.go`、`internal/platform/fileclient/client.go` | `docs/services/gateway/api/openapi.yaml`、`docs/architecture/service-boundaries.md` | `TestDocumentChunksAndContentContract`、Gateway binary proxy tests | 先校验 Knowledge 文档可见性，再通过 File Service 内部读取 raw bytes；响应为二进制流，不包 JSON envelope，不暴露 `file_ref`、object key 或内部 URL。 |
| Parser Service client / chunker | `services/knowledge/internal/platform/parser`、`internal/service/chunker.go` | `docs/services/knowledge/README.md`、`docs/services/parser/README.md` | parser/client、worker/service tests | Knowledge 调用独立 Parser Service 获取 parsed content，并在 Knowledge 内完成 chunking；PaddleOCR runtime 不在 Knowledge Go 进程内。 |
| embedding / vector index | `services/knowledge/internal/platform/embedding`、`internal/platform/vector` | `docs/architecture/service-boundaries.md` | platform/worker tests | local hashing 默认；可选 AI Gateway embedding；Qdrant HTTP adapter 或 in-memory index。 |
| `knowledge-queries` 检索 | `services/knowledge/internal/service/retrieval.go`、`internal/http/server.go`、Gateway proxy route | `docs/services/knowledge/docs/api-contract.md`、`docs/services/gateway/api/openapi.yaml` | service retrieval tests、`TestKnowledgeQueryContractWithSeededRepositoryAndFakeVector`、`TestKnowledgeQueriesRouteProxiesToKnowledge` | 基于 embedder + vector index 搜索，回 PostgreSQL hydrate chunks/documents，过滤未 ready/不可见文档，支持 tags、metadata filter、可选 AI Gateway rerank 和 local no-op rerank fallback。 |
| PostgreSQL migration/repository | `services/knowledge/migrations/0001_create_knowledge_core_tables.sql`、`0002_create_parser_configs.sql`、`internal/repository/postgres.go` | `docs/services/knowledge/docs/data-models.md` | `go test ./...`；CI 用 `KNOWLEDGE_TEST_DATABASE_URL` 跑 repository lifecycle integration test | runtime 使用 PostgreSQL，保存文档、job、parser configs 和 chunks。 |

## 4. 未实现

| 缺口 | 文档来源 | 影响范围 | 建议任务 |
| --- | --- | --- | --- |
| document PATCH/DELETE 未实现 | `docs/services/gateway/api/openapi.yaml` | API / frontend | 待确认：实现标签更新、删除和 file/index cleanup。 |
| 真实 Qdrant collection 管理 / smoke 未闭环 | `docs/services/knowledge/docs/api-contract.md`、`docs/architecture/technology-decisions.md` | retrieval / deployment | 当前 Qdrant adapter 已实现 search/upsert/delete，但本地默认 memory index；需要真实 collection 创建、上传入库、query smoke 记录。 |
| 真实 AI Gateway embedding/rerank smoke 未闭环 | `docs/architecture/service-boundaries.md`、`docs/services/knowledge/docs/data-models.md` | retrieval / AI Gateway | embedding 与 rerank adapter 已实现，默认 local hashing/no-op fallback；需要带真实 provider credential 的跨服务 smoke。 |

## 5. 文档与实现出入

| 出入点 | 文档要求 | 当前实现 | 风险 | 建议处理 |
| --- | --- | --- | --- | --- |
| Gateway active Knowledge paths | Gateway OpenAPI 将 Knowledge operations 设为 active | chunks、content、knowledge-queries、parser-configs 已转为 Knowledge proxy；`PATCH/DELETE /documents` 仍标为 `NotImplemented` | 未落地的 PATCH/DELETE 前端生成 client 后调用会得到 501 | 对剩余 document lifecycle routes 补实现，或在契约/owner map 标注阶段性不可调用。 |
| AI Gateway rerank smoke 状态 | AI Gateway 已实现 embeddings/rerankings endpoint，Knowledge 支持 embedding 与 rerank adapter | `knowledge-queries` 可选 rerank 已接入；本地未配置 `RERANK_MODEL` 时使用 no-op fallback | 容易把 no-op fallback 误读为真实 provider rerank 已验收 | 保留 fake/seeded 契约测试，同时补带真实 provider credential 的跨服务 smoke。 |
| 公开 Knowledge 草案范围 | `docs/services/knowledge/api/public.openapi.yaml` 覆盖 deletion jobs、processing jobs、query tests、support materials、settings、statistics | runtime 只实现 KB CRUD 和文档 upload/list/detail | 公开草案可能被误读为已落地 | 保留为设计草案，在 implementation 中明确缺口。 |
| File handoff 边界 | Knowledge 拥有文档资源，File 只保存基础 file object | 当前已按 `/internal/v1/files` 保存 raw file，并通过 content API 读取；delete cleanup 未闭环 | 删除文档时 file reference 可能残留 | 实现 document lifecycle cleanup。 |

## 6. MVP / mock / memory backend / 占位

| 项目 | 当前用途 | 退出条件 | 关联任务 |
| --- | --- | --- | --- |
| memory repository | 单元测试 | PostgreSQL integration tests 覆盖关键 CRUD 后仍可保留测试用 | 保留测试用 |
| fake file client / fake queue | 上传补偿和入队测试 | 真实 file/Redis 集成测试补齐 | File/Redis integration smoke |
| fake parser client | A-11 worker、A-12/A-14 契约测试的 parsed content 输入 | 真实 Parser service smoke 稳定后仍可保留为快速契约测试 | Parser contract tests |
| seeded chunk/vector fixture | A-12 retrieval 和 A-14 contract tests | 真实 worker + Qdrant + AI Gateway smoke 稳定后仍可保留为快速契约测试 | A-12/A-14 并行开发 |
| fake vector / fake AI adapter | 检索过滤、rerank trace、错误 envelope 和 request id 测试 | 真实依赖集成测试补齐；不替代端到端 smoke | Retrieval contract tests |
| gateway `NotImplemented` document lifecycle routes | 暂时占住 document PATCH/DELETE active contract | 对应服务实现或契约阶段状态经管理组确认 | Knowledge document lifecycle follow-up |

## 7. 运行与配置

| 项目 | 当前状态 | 缺口 |
| --- | --- | --- |
| 启动命令 | `cd services/knowledge && go run ./cmd/server` | 需要 PostgreSQL、File Service 和 Redis。 |
| 环境变量 | `DATABASE_URL`、`FILE_SERVICE_BASE_URL`、`PARSER_SERVICE_BASE_URL`、`KNOWLEDGE_REDIS_ADDR`、`KNOWLEDGE_SERVICE_TOKEN` 必填；另有 embedding、AI Gateway、Qdrant、HTTP/version/env/max upload/shutdown 配置 | 仍需按部署环境补真实依赖连通性检查。 |
| PostgreSQL / migration | `migrations/0001_create_knowledge_core_tables.sql`、`0002_create_parser_configs.sql`，runtime `pgx/v5` | goose apply CI 已覆盖 migration；repository lifecycle 由 `KNOWLEDGE_TEST_DATABASE_URL` 集成测试覆盖。 |
| Redis / queue | 使用 `asynq` client 投递 ingestion，worker 在同进程消费 `knowledge:document:ingest` | 后续可按部署形态拆分独立 worker 进程。 |
| Object storage / vector store / AI provider | 通过 File Service 保存和读取 raw file；默认 local hashing + memory vector index；可选 Qdrant adapter、AI Gateway embedding adapter 和 rerank adapter 已接入 | 仍需真实 File/Qdrant/AI Gateway 端到端联调。 |
| Parser runtime | Knowledge 通过 `PARSER_SERVICE_BASE_URL` 调 `services/parser` 的 `/internal/v1/parsed-documents`；Parser Service 以 Python/FastAPI/PaddleOCR 独立部署 | 仍需真实 PaddleOCR 模型 smoke 和部署环境资源配置。 |

当 `EMBEDDING_PROVIDER=ai_gateway` 时，`EMBEDDING_MODEL` 必须匹配解析出的 AI Gateway embedding profile `model`。`AI_GATEWAY_EMBEDDING_PROFILE_ID` 可留空以使用 AI Gateway 默认启用的 embedding profile，但 provider 调用前仍会强制校验 model 匹配。

## 8. 测试与验证

| 验证项 | 命令或步骤 | 当前结果 | 缺口 |
| --- | --- | --- | --- |
| 单元测试 | `cd services/knowledge && go test ./...` | pass（2026-06-30，Docker Go 1.25；本地 ignored `data/` 目录需 root 容器遍历） | 主要使用 memory/fake 依赖，并覆盖 parser-configs 管理、fallback、conflict、上传 snapshot、chunks/content、`knowledge-queries`、错误 envelope 和 request id。 |
| Repository 集成测试 | `KNOWLEDGE_TEST_DATABASE_URL=... go test ./internal/repository -count=1` | CI 覆盖 repository lifecycle；无 env 时本地跳过 | 只覆盖 PostgreSQL repository，不覆盖 File/Redis/Qdrant。 |
| Parser 服务测试 | `cd services/parser && uv run ruff check . && uv run pytest && uv run python -m compileall src tests` | not run（A-014 未改 parser） | 使用 fake OCR backend，不下载 PaddleOCR 模型。 |
| 端到端上传联调 | PostgreSQL + File + Redis end-to-end upload | missing | 需要真实依赖联调；A-14 契约测试已用 seeded/fake 依赖覆盖 active operations。 |
| 契约测试 | gateway route matrix + Knowledge handler tests | pass（2026-06-30） | chunks、content、knowledge-queries、parser-configs 等 active path 已补 contract/request-id/error envelope 覆盖；document PATCH/DELETE 仍为 `not_implemented`。 |
| 手工 smoke | 启动 PostgreSQL、File、Redis 后上传文档 | not run | 需要可复现脚本或 Compose。 |

## 9. 建议任务

| 任务 | 类型 | 优先级 | 依据 | 说明 |
| --- | --- | --- | --- | --- |
| 实现 Knowledge 文档 PATCH/DELETE lifecycle | 新任务 | P0 | Gateway active contract | 解除 `PATCH/DELETE /api/v1/documents/{documentId}` 相关 501，并协调 tags、soft delete、file/index cleanup。 |
| 补真实 File/Parser/Qdrant/AI Gateway 端到端 smoke | 新任务 | P0 | 当前 worker 与 contract 主要由 fake/memory 依赖覆盖 | 覆盖上传、入队、worker 消费、Parser Service parse、document ready、chunks 可查、content 可读、Qdrant points 存在和 `knowledge-queries` 可命中。 |

## 10. 最近检查记录

| 日期 | 检查人/工具 | 代码基准 | 结论 |
| --- | --- | --- | --- |
| 2026-06-30 | Codex | A-014 working tree | 补齐 chunks/content internal route、Gateway proxy、seeded/fake-backed `knowledge-queries` contract、错误 envelope 和 request id 测试；document PATCH/DELETE 与真实 Qdrant/AI Gateway smoke 仍待后续任务。 |
| 2026-06-30 | Codex | working tree | 补充 A-11/A-12/A-14 解耦契约：A-12/A-14 可用 seeded chunks、fake vector/AI adapter 做契约和 handler 测试；完整 ingestion runtime 仍由 A-11 交付。 |
| 2026-06-30 | Codex | A-13 PR #249 | parser-configs 运行时管理已落地并合入：Knowledge 内部 API、Gateway proxy、默认 builtin seed、上传 snapshot、conflict 映射和前端管理入口均已覆盖。 |
| 2026-06-30 | Codex | PR #226 docs extraction | 从 PR #226 单独抽出 Parser Runtime 文档和 OpenAPI 契约；当前分支只记录契约，未引入 Knowledge worker 实现代码。 |
| 2026-06-29 | Codex goal | `eddf917` + working tree | Knowledge 已完成 KB CRUD 和文档上传 handoff；当时 parser config 与入库 worker、chunks、content、retrieval 均为关键缺口，其中 parser config 已由 A-13 补齐。 |
| 2026-06-29 | Codex | A11 branch | Knowledge 已完成文档上传 handoff、入库 worker、Parser Service 解析调用、切片、embedding、vector 写入和 chunk 持久化；当时 content 与 knowledge-queries 仍待补齐，A-14 working tree 已补 active route/contract 覆盖。 |
