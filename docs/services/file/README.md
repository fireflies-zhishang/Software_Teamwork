# File 服务接口文档

本文档定义 `file` 服务的职责边界和内部文件能力契约。前端公开契约以 [`docs/services/gateway/api/openapi.yaml`](../gateway/api/openapi.yaml) 为准；前端不得直接调用 `file` 服务内部地址，只能通过 gateway 暴露的 `/api/v1/**` 入口访问文件能力。

RESTful 路径、统一响应和错误 envelope 以 [前后端集成契约](../../architecture/frontend-backend-contract.md) 为准。知识库原始文件流使用 `documents/{documentId}/content` 子资源表示；报告模板、素材和导出文件使用 `document` 服务拥有的 report 资源路径，由 `document` 在服务边界内复用 `file` 服务的基础文件能力。

## 文档入口

| 文档 | 说明 |
| --- | --- |
| [数据模型](docs/data-models.md) | File Service 拥有的基础文件对象元数据、对象存储引用和清理模型。 |
| [实现说明](docs/implementation.md) | File Service 按技术选型基线落地 `pgx/sqlc`、`goose`、`ServeMux`、`slog`、MinIO SDK 和测试的约定。 |
| [服务 OpenAPI](../../../services/file/api/openapi.yaml) | File Service 内部 `/internal/v1/files/**` API 契约；不是前端公开契约。 |

## 技术基线

File Service 必须遵循 [技术选型基线](../../architecture/technology-decisions.md)。本服务只补充文件域特有约束：

- 上传大小限制必须在 HTTP 层和 multipart 解析层同时生效。
- 使用官方 MinIO Go SDK 或符合 `ObjectStore` 接口的本地存储 adapter；`file` 服务封装 bucket、object key、etag、version id 和对象 URL，owner service 与前端都不得直接依赖这些内部字段。
- 对象物理清理可由 `asynq` worker 执行，任务类型使用 `file:object:purge`。PostgreSQL 中的文件状态、失败摘要、重试次数和最终结果仍是权威来源。
- handler 测试重点覆盖 envelope、错误码、request id、multipart 边界和内容流响应；repository 测试覆盖 migration 后的 SQL 行为。
- File Service 不使用 ORM，不把 MinIO SDK 泄露到 handler 或 owner service client，不把缓存或队列作为基础文件元数据的事实来源。

## 职责边界

| 范围 | 说明 |
| --- | --- |
| 原始文件对象 | 接收后端 owner service 转交的文件流，校验基础文件属性，并写入 MinIO 或等价对象存储。 |
| 基础文件元数据 | 只维护 file ID、原始文件名、内容类型、文件大小、checksum、内部存储引用、创建时间和删除时间等基础元数据。 |
| 对象存储协调 | 生成服务端 object key，管理 bucket/object 写入、读取和删除，不向前端暴露内部存储路径、签名 URL 或存储凭据。 |
| 文件内容读取 | 根据内部 file ID 返回原始文件流，供 `knowledge`、`document` 等 owner service 在自己的资源边界内使用。 |
| 文件删除 | 负责基础文件元数据生命周期和原始对象删除或延迟清理流程。 |

`file` 是后端内与文件对象和对象存储交互的基础中间件服务。它不负责知识库 CRUD、知识库文档处理状态、文档解析、文本切片、embedding、向量索引、RAG、问答生成、报告内容生成、报告模板配置、报告素材业务状态或报告导出状态。

`knowledge` 服务拥有知识库、知识库文档、文档处理状态、chunks 和索引状态。`document` 服务拥有报告、报告模板、报告素材、报告文件和生成流程业务状态。`file` 服务只承诺原始文件对象、对象存储协调和最小基础 file 元数据。

`file` 服务不得存储以下业务数据：

- `knowledgeBaseId`、知识库文档处理状态、chunks、parser 配置、向量索引状态。
- `reportId`、`templateId`、`materialId`、`reportFileId`、导出状态、报告模板结构、报告素材引用关系。
- 业务可见性、业务软删除规则、业务权限 ACL。

## 接入模型

```text
frontend
   |
   v
gateway /api/v1/knowledge-bases/{knowledgeBaseId}/documents
gateway /api/v1/documents/{documentId}/content
gateway /api/v1/report-templates
gateway /api/v1/report-materials
gateway /api/v1/report-files/{reportFileId}/content
   |
   v
owner service
   |-- knowledge owns knowledge documents and document content resources
   |-- document owns report templates, materials, and report files
   |
   v
file service /internal/v1/files/**
   |
   |-- PostgreSQL basic file metadata
   +-- MinIO object storage
```

前端侧只调用 gateway 公开接口。gateway 将认证后的请求转发给 owner service，并统一处理响应 envelope、request id 和错误归一化。

知识库文件上传、文档详情、文档标签、删除和原始文件流公开资源由 `knowledge` 拥有。`knowledge` 在服务边界内保存内部 file reference、知识库归属、处理状态和索引状态，并调用 `file` 读写底层原始文件对象。

报告模板、素材和导出文件公开资源由 `document` 拥有。`document` 保存 `reportTemplateId`、`materialId`、`reportFileId`、业务状态和 file reference，并通过内部 client 调用 `file` 完成对象写入、读取和删除。前端仍只看到 report 资源 ID 和 content 子资源，不接触 file 内部 ID、bucket、object key 或 MinIO URL。

## 公开接口总览

`file` 服务不直接拥有前端公开 API。以下 gateway path 会复用 file 的基础能力，但 owner service 不是 `file`：

| Method | Gateway Path | Owner | 说明 |
| --- | --- | --- | --- |
| `POST` | `/api/v1/knowledge-bases/{knowledgeBaseId}/documents` | `knowledge` | 上传原始文件并创建知识库文档资源；`knowledge` 保存知识库归属和处理状态，并在内部调用 `file` 保存底层文件对象。 |
| `GET` | `/api/v1/knowledge-bases/{knowledgeBaseId}/documents` | `knowledge` | 查询知识库内文档列表和处理状态。 |
| `GET` | `/api/v1/documents/{documentId}` | `knowledge` | 查询知识库文档详情和处理状态。 |
| `PATCH` | `/api/v1/documents/{documentId}` | `knowledge` | 更新知识库文档元数据，例如标签；不得修改 file 基础对象存储信息。 |
| `DELETE` | `/api/v1/documents/{documentId}` | `knowledge` | 删除知识库文档资源，并由 `knowledge` 协调 chunks、索引和底层 file 引用清理。 |
| `GET` | `/api/v1/documents/{documentId}/chunks` | `knowledge` | 查询文档切片。 |
| `GET` | `/api/v1/documents/{documentId}/content` | `knowledge` | 获取知识库原始文件流；gateway 仍只暴露 `documents/{documentId}/content` 子资源。 |
| `GET/POST/DELETE` | `/api/v1/report-templates/**` | `document` | 报告模板业务资源；底层模板文件通过 `document` 内部复用 `file`。 |
| `GET/POST/DELETE` | `/api/v1/report-materials/**` | `document` | 报告素材业务资源；底层素材文件通过 `document` 内部复用 `file`。 |
| `GET/POST/DELETE` | `/api/v1/report-files/**` | `document` | 报告导出文件业务资源；`/content` 子资源读取生成文件内容，底层生成文件通过 `document` 内部复用 `file`。 |

## 通用响应结构

JSON 成功、分页和错误响应遵循 [前后端集成契约](../../architecture/frontend-backend-contract.md)。文件内容接口成功时返回文件流，不包裹 JSON envelope；失败时仍返回统一错误响应。前端和调用方应优先匹配 `error.code`，不要解析 `message` 文案。

## 内部服务接口目标契约

`file` 的目标内部契约只表达基础文件对象，不表达 knowledge document、report material、report template 或 report file 等业务语义。

| Method | File Service Path | 说明 |
| --- | --- | --- |
| `GET` | `/healthz` | file 进程存活检查。 |
| `GET` | `/readyz` | file 就绪检查。生产 PostgreSQL/MinIO 适配器落地后需覆盖关键依赖。 |
| `POST` | `/internal/v1/files` | 创建基础文件对象，接收 multipart 文件流并返回内部 `fileId` 和基础元数据。 |
| `GET` | `/internal/v1/files/{fileId}` | 查询基础文件元数据。 |
| `DELETE` | `/internal/v1/files/{fileId}` | 删除或标记删除基础文件对象。 |
| `GET` | `/internal/v1/files/{fileId}/content` | 读取原始文件流。 |

当前 `services/file/` 代码仍保留知识库文档形态的 MVP 兼容路由：

- `POST /internal/v1/knowledge-bases/{knowledgeBaseId}/documents`
- `GET /internal/v1/documents/{documentId}`
- `PATCH /internal/v1/documents/{documentId}`
- `DELETE /internal/v1/documents/{documentId}`
- `GET /internal/v1/documents/{documentId}/content`

这些兼容路由仅用于现阶段联调，不作为新的服务边界继续扩展。后续实现应迁移到 `/internal/v1/files/**`，并由 `knowledge` 自己创建和维护知识库文档资源。

## 内部数据结构

### CreateFileRequest

上传请求使用 `multipart/form-data`。

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `file` | `binary` | 是 | 原始文件内容。 |
| `checksumSha256` | `string` | 否 | 调用方已计算的 SHA-256；缺失时可由 `file` 服务计算。 |

`file` 基础接口不接收 `knowledgeBaseId`、`reportId`、`templateId`、`materialId`、`reportFileId`、业务标签或处理状态。需要这些字段时，由 owner service 在自己的数据库中保存。

### FileObject

```json
{
  "id": "file_123",
  "filename": "设备巡检规范.pdf",
  "contentType": "application/pdf",
  "sizeBytes": 1048576,
  "checksumSha256": "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
  "createdAt": "2026-06-28T10:00:00Z",
  "deletedAt": null
}
```

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `id` | `string` | 是 | file 服务内部文件 ID。前端公开响应不得直接暴露报告文件、模板或素材使用的内部 file ID。 |
| `filename` | `string` | 是 | 展示用原始文件名或规范化文件名。 |
| `contentType` | `string` | 是 | 文件内容类型；缺失或不可信时使用 `application/octet-stream`。 |
| `sizeBytes` | `integer` | 是 | 文件大小。 |
| `checksumSha256` | `string` | 否 | 文件内容 checksum。 |
| `createdAt` | `string(date-time)` | 是 | 创建时间。 |
| `deletedAt` | `string(date-time)` | 否 | 删除时间；未删除时为 `null`。 |

内部响应可包含 `contentType`、`sizeBytes`、`checksumSha256` 等基础 file 对接字段，但不得包含 bucket、object key、内部对象 URL 或存储凭据。

## 文件内容读取

`GET /internal/v1/files/{fileId}/content` 成功时返回原始文件二进制流。

推荐响应头：

| Header | 说明 |
| --- | --- |
| `Content-Type` | 原始文件内容类型；缺失或不可信时使用 `application/octet-stream`。 |
| `Content-Disposition` | 使用安全转义后的原始文件名，通常为 `attachment`。 |
| `Content-Length` | 文件大小，能可靠获得时返回。 |
| `X-Request-Id` | 与响应体或日志一致的 request id。 |

当前目标契约不要求断点续传或 Range 内容读取；如后续支持，需要同步更新 gateway OpenAPI、前后端集成契约和本文档。`file` 服务不得把 MinIO 内部 URL、bucket、object key 或 access key 返回给前端或 owner service 的公开响应。

## 权限与上下文要求

`file` 服务是内部基础服务，必须只接受 gateway 或后端 owner service 的可信调用。前端不得设置 `X-User-Id`、`X-User-Roles`、`X-User-Permissions`；这些字段只能由 gateway 在认证后注入，并由 owner service 透传或转换为服务间调用上下文。

业务资源可见性由 owner service 判断：知识库文档由 `knowledge` 定义，报告素材、模板和报告文件由 `document` 定义，`file` 不复制这些业务权限规则。`file` 只校验调用方服务身份、request id、基础文件操作权限和必要的服务边界约束。

## 对象存储与元数据要求

- PostgreSQL 只存储基础文件元数据：file ID、原始文件名、内容类型、大小、checksum、内部存储引用、创建时间、删除时间。
- PostgreSQL 不存储 `knowledgeBaseId`、`documentId`、`reportId`、`templateId`、`materialId`、`reportFileId`、业务标签、处理状态或业务权限 ACL。
- MinIO 存储原始文件对象；bucket 名和 object key 是 `file` 服务内部实现细节。
- Object key、bucket、内部对象 URL、MinIO 错误和 access key 不得进入前端响应。
- 上传文件名必须做展示层安全处理，不能直接用于 object key。
- 文件删除应优先保证 owner service 无法再通过 file reference 读取；物理删除失败时应有可重试的清理机制。

## 错误码约定

File 相关接口使用项目统一错误码：

| Code | HTTP status | File 场景 |
| --- | --- | --- |
| `validation_error` | `400` | multipart 解析失败、缺少文件、文件为空、checksum 格式非法、文件类型或大小不满足规则。 |
| `unauthorized` | `401` | 缺少可信服务调用上下文或认证无效。 |
| `forbidden` | `403` | 调用方服务无权执行该基础文件操作。 |
| `not_found` | `404` | 文件元数据或原始对象不存在，或已删除。 |
| `conflict` | `409` | 文件状态不允许当前操作。 |
| `rate_limited` | `429` | 上传频率、容量、数量或租户配额超限；配额归属由 owner service 定义。 |
| `dependency_error` | `502` | PostgreSQL、MinIO 或其他已确定依赖失败。 |
| `internal_error` | `500` | 未预期服务端错误。 |

## 安全与日志要求

- 不得在日志、错误响应或追踪字段中记录 object key、内部 URL、access key、token、数据库连接串或完整敏感文件内容。
- 日志建议包含 `service`、`request_id`、`operation`、`file_id`、`caller_service`、`status`、`content_type`、`size_bytes` 等可排查字段。
- 文件名来自用户输入，写入响应头前必须安全转义，避免 header injection。
- 所有跨服务 HTTP client 必须设置超时，并传递 `context.Context`。
- 上传大小、内容类型白名单、危险文件扫描和租户配额属于实现前必须明确的安全策略；业务配额归属仍由 owner service 持有。
- `file` 服务可以记录文件域内结构化日志和 request id，但不拥有全局审计日志查询能力；后续如审计日志独立成服务，file/domain 服务应只对接审计事件生产或查询授权契约。

## 后续实现建议

当前 `services/file/` 已有分层雏形，后续扩展应继续保持服务本地边界并收敛到以下目标结构：

```text
services/file/
├── api/
│   └── openapi.yaml
├── cmd/server/
├── internal/
│   ├── config/
│   ├── http/
│   ├── service/
│   ├── repository/
│   │   ├── queries/
│   │   └── sqlc/
│   ├── platform/
│   │   └── storage/
│   │       └── minio/
├── migrations/
├── sqlc.yaml
└── README.md
```

实现前需要补齐或确认：

- 将当前 knowledge-document MVP 兼容路由迁移为基础 `/internal/v1/files/**` 资源路由。
- 按 `pgx` + `sqlc` 补齐 `services/file/sqlc.yaml`、query 文件、生成代码目录和 repository 适配层。
- 按 `goose` 补齐 `services/file/migrations/` 下的真实建表迁移，并在 CI 中验证迁移可应用。
- 使用官方 MinIO Go SDK 增加生产对象存储适配器；当前 memory adapter 只能用于测试和早期本地联调，local adapter 仅用于本地持久化 smoke test。
- 按 `envconfig` 风格补齐 PostgreSQL、MinIO、HTTP、上传限制、日志和 shutdown 配置校验。
- 接入 `slog` JSON 日志、request id 贯穿和基础 Prometheus 风格指标。
- 最大上传大小、允许文件类型、空文件和重复文件策略。
- 文件元数据表结构、索引、软删除和物理清理策略。
- MinIO bucket 命名、object key 生成规则和本地开发配置。
- 物理删除是否同步执行，或通过 `asynq` `file:object:purge` 任务异步清理；无论执行方式如何，PostgreSQL 状态必须可追溯。
- `knowledge` 上传文档时的内部 file reference 保存、ingestion job 创建和失败补偿由 `knowledge` 自己实现。
- `document` 复用 file 服务时的 report template、material、report file 到 file reference 映射由 `document` 自己实现。
- 是否支持秒传、checksum 去重、断点续传、预签名内容 URL 或 Range 内容读取。

如果上述决策影响公开字段、错误码或状态码，必须同步更新：

- [`docs/services/gateway/api/openapi.yaml`](../gateway/api/openapi.yaml)
- [`docs/architecture/frontend-backend-contract.md`](../../architecture/frontend-backend-contract.md)
- [`docs/architecture/service-boundaries.md`](../../architecture/service-boundaries.md)
- 本文档
