# 项目文档索引

本文档作为 `docs/` 目录的阅读入口，用于区分需求说明、架构契约和协作维护文档。

## 推荐阅读顺序

1. 先读 [技术监督辅助平台需求说明书](requirements/system.md)，了解完整业务范围。
2. 再读 [服务边界矩阵](architecture/service-boundaries.md)，确认各服务职责归属。
3. 开始前后端联调前，阅读 [前后端集成契约](architecture/frontend-backend-contract.md) 和 [Gateway OpenAPI 契约](api/gateway.openapi.yaml)。
4. 需要实现具体后端服务时，阅读对应服务接口文档。
5. 参与协作、分支、PR 或仓库维护时，阅读协作维护文档。

## 需求说明

| 文档 | 内容 |
| --- | --- |
| [技术监督辅助平台需求说明书](requirements/system.md) | 系统整体需求，覆盖知识管理、知识问答、报告生成、用户权限和非功能性要求。 |
| [知识管理系统需求说明书](requirements/knowledge_management_system.md) | 知识库、文档管理、解析处理、配置和统计需求。 |
| [智能问答系统需求说明书](requirements/smart_quote_system.md) | 智能对话、意图识别、RAG 检索、引用溯源和问答管理需求。 |
| [报告生成系统需求说明书](report_generation/report_generation.md) | 报告大纲、内容生成、导出、记录和模板管理需求。 |

## 架构与接口契约

| 文档 | 内容 |
| --- | --- |
| [服务边界矩阵](architecture/service-boundaries.md) | `gateway`、`auth`、`file`、`knowledge`、`qa`、`document`、`ai-gateway` 的职责边界和缺失契约登记。 |
| [Gateway 服务规划](services/gateway.md) | Gateway 的设计原则、公开 API、认证上下文、响应约定和后续扩展。 |
| [Auth 服务接口文档](services/auth.md) | 用户、会话、权限上下文和 auth 内部服务接口草案。 |
| [File 服务接口文档](services/file.md) | 文件上传、元数据、原文件内容读取和 file 内部服务接口草案。 |
| [Knowledge 服务接口文档](services/knowledge.md) | 知识库、文档处理状态、切片、向量索引和检索接口契约。 |
| [Knowledge Service 实现说明](services/knowledge-service-design.md) | `services/knowledge/` 本地服务实现、Docker Compose 和入库链路说明。 |
| [QA 服务接口文档](services/qa.md) | 智能问答、会话、消息、SSE、引用、配置、检索测试和统计接口草案。 |
| [QA 模块数据库说明](services/qa-database.md) | QA 模块 PostgreSQL 表结构、核心关系、写入流程、索引和迁移规则。 |
| [AI Gateway 服务接口文档](services/ai-gateway.md) | 内部模型配置、OpenAI-compatible chat/embedding、rerank 和 provider 错误归一化接口草案。 |
| [报告生成前端 API 设计](report_generation/report-generation-frontend-api-design.md) | 报告生成模块前端对接 gateway 的页面接口映射、类型约定和旧路径替换说明。 |
| [前后端集成契约](architecture/frontend-backend-contract.md) | 前端调用 gateway 的入口、认证、请求/响应、错误、分页、SSE 和 mock 约定。 |
| [Gateway OpenAPI 契约](api/gateway.openapi.yaml) | 当前稳定的 gateway 公开 API 机器可读契约。 |
| [AI Gateway API 契约](接口契约/AI网关-api契约.md) | 内部模型服务契约，说明 AI Gateway 与 `qa`、`knowledge`、`document` 的协作边界。 |
| [AI Gateway OpenAPI 契约](接口契约/openapi/ai-gateway.openapi.yaml) | AI Gateway 内部服务机器可读契约；前端不得直接调用。 |

## 协作与维护

| 文档 | 内容 |
| --- | --- |
| [前端协作工作流](collaboration/frontend-workflow.md) | 前端目录、Bun 命令、检查、PR 和 CI 建议。 |
| [仓库维护设置](collaboration/repository-settings.md) | GitHub label、分支保护、PR Guard、Auto Label 和 Commitlint 设置。 |

仓库级分支、PR、提交和合并策略以根目录 [CONTRIBUTING.md](../CONTRIBUTING.md) 为准。

## 契约状态

当前已稳定的公开契约和已形成草案的内部契约包括：

- Gateway 健康检查。
- Auth 相关用户与会话接口。
- File-owned 文件上传、元数据更新、删除和原文件内容读取接口。
- Knowledge-owned 知识库、文档处理状态、切片详情和知识检索接口。
- AI Gateway 内部模型配置、OpenAI-compatible chat/embedding 和 OpenAI-style rerank 接口草案。

仍待补齐的契约包括：

- `qa` 的会话、消息、意图路由、引用和流式问答接口已形成服务文档草案，仍需升级 OpenAPI 后才能作为稳定公开契约。
- `document` 的报告记录、大纲、章节、报告文件和导出接口。
- 管理后台聚合指标和跨服务统计接口。

新增或调整公开接口或内部模型接口时，需要同步更新：

- [Gateway OpenAPI 契约](api/gateway.openapi.yaml)
- [前后端集成契约](architecture/frontend-backend-contract.md)
- [服务边界矩阵](architecture/service-boundaries.md)
- 对应服务接口文档
- 涉及内部模型调用时，同步更新 [AI Gateway API 契约](接口契约/AI网关-api契约.md) 和 [AI Gateway OpenAPI 契约](接口契约/openapi/ai-gateway.openapi.yaml)
