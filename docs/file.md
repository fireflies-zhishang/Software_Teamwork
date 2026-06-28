# File 服务接口文档

本文档定义 `file` 服务在项目初期的职责边界和接口契约。当前仓库尚未落地 `services/file/` 代码，因此本文档以现有 gateway OpenAPI、服务边界矩阵和前后端集成契约为准，用于指导后续 file 服务实现与联调。

详细的前端公开路径以 [`docs/api/gateway.openapi.yaml`](api/gateway.openapi.yaml) 为准。前端不得直接调用 file 服务内部地址，只能通过 gateway 暴露的 `/api/v1/**` 入口访问文件能力。

## 职责边界

| 范围 | 说明 |
| --- | --- |
| 原始文件上传 | 接收上传文件，校验文件基础属性，将对象写入 MinIO。 |
| 文件元数据 | 维护文件 ID、知识库 ID、原始文件名、内容类型、文件大小、标签、上传人和创建时间等元数据。 |
| 对象存储协调 | 生成服务端 object key，管理 bucket/object 写入、读取和删除，不向前端暴露内部存储路径。 |
| 文件下载 | 根据文档 ID、用户上下文和权限校验结果返回原始文件流。 |
| 文件删除 | 负责文件元数据生命周期和原始对象删除或延迟清理流程。 |
| 上传工作流入口 | 与 `knowledge` 服务协作启动后续解析、切片、向量化流程。 |

`file` 不负责知识库 CRUD、文档解析、文本切片、embedding、向量索引、RAG、问答生成或报告内容生成。`knowledge` 服务拥有文档处理状态、chunks 和索引状态；`file` 服务只拥有原始文件和文件元数据。

## 接入模型

```text
frontend
   |
   v
gateway /api/v1/knowledge-bases/{knowledgeBaseId}/documents
gateway /api/v1/documents/{documentId}/download
   |
   v
file service
   |
   +--> PostgreSQL metadata
   +--> MinIO object storage
   +--> knowledge service ingestion handoff
```

前端侧调用 gateway 公开接口；gateway 将认证后的请求转发给 file，并统一处理响应 envelope、request id 和错误归一化。

Gateway 调用 file 服务时应传递：

| Header | 说明 |
| --- | --- |
| `X-Request-Id` | 贯穿一次前端请求的 request id。 |
| `X-User-Id` | 已认证用户 ID。 |
| `X-User-Roles` | 逗号分隔的角色列表。 |
| `X-User-Permissions` | 逗号分隔的权限列表。 |
| `X-Forwarded-For` | 原始客户端地址链。 |
| `X-Forwarded-Proto` | 原始协议。 |

前端不得设置 `X-User-Id`、`X-User-Roles`、`X-User-Permissions`；这些字段只能由 gateway 在认证后注入。File 服务仍需在自己的服务边界校验用户上下文和资源访问权限。

## 公开接口总览

| Method | Gateway Path | Auth | Owner | 说明 |
| --- | --- | --- | --- | --- |
| `POST` | `/api/v1/knowledge-bases/{knowledgeBaseId}/documents` | 需要 | `file` + `knowledge` | 上传文件到知识库。File 保存原始文件和元数据，knowledge 负责后续处理状态和索引。 |
| `PATCH` | `/api/v1/documents/{documentId}` | 需要 | `file` | 更新文件标签等 file-owned 元数据。 |
| `DELETE` | `/api/v1/documents/{documentId}` | 需要 | `file` | 删除文档对应的原始文件和 file-owned 元数据。 |
| `GET` | `/api/v1/documents/{documentId}/download` | 需要 | `file` | 下载原始文件。 |

相关但非 file-owned 的公开接口：

| Method | Gateway Path | Owner | 说明 |
| --- | --- | --- | --- |
| `GET` | `/api/v1/knowledge-bases/{knowledgeBaseId}/documents` | `knowledge` | 查询知识库内文档列表和处理状态。 |
| `GET` | `/api/v1/documents/{documentId}` | `knowledge` | 查询文档详情和处理状态。 |
| `GET` | `/api/v1/documents/{documentId}/chunks` | `knowledge` | 查询文档切片。 |

## 通用响应结构

JSON 成功响应遵循 gateway 统一 envelope：

```json
{
  "data": {},
  "requestId": "req_123"
}
```

错误响应固定为：

```json
{
  "error": {
    "code": "validation_error",
    "message": "request validation failed",
    "requestId": "req_123",
    "fields": {
      "file": "is required"
    }
  }
}
```

下载接口成功时返回文件流，不包裹 JSON envelope；失败时仍返回统一错误响应。

前端和调用方应优先匹配 `error.code`，不要解析 `message` 文案。

## 数据结构

### UploadKnowledgeBaseDocumentRequest

上传请求使用 `multipart/form-data`。

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `file` | `binary` | 是 | 原始文件内容。 |
| `tags` | `string[]` | 否 | 文件标签。当前 OpenAPI 已声明为数组，具体 multipart 编码方式需在实现时统一。 |

示例：

```http
POST /api/v1/knowledge-bases/kb_123/documents
Authorization: Bearer <accessToken>
Content-Type: multipart/form-data
```

### UpdateDocumentRequest

```json
{
  "tags": ["policy", "inspection"]
}
```

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `tags` | `string[]` | 否 | 替换文档标签。标签命名规则和数量限制需在 file 服务实现时明确。 |

### DocumentStatus

```text
uploaded | parsing | chunking | embedding | ready | failed
```

`uploaded` 可由上传流程产生；`parsing`、`chunking`、`embedding`、`ready`、`failed` 由 `knowledge` 服务拥有并更新。File 服务不得自行伪造解析、切片或向量化状态。

### DocumentSummary

```json
{
  "id": "doc_123",
  "knowledgeBaseId": "kb_123",
  "name": "设备巡检规范.pdf",
  "status": "uploaded",
  "tags": ["policy", "inspection"],
  "errorMessage": null,
  "createdAt": "2026-06-28T10:00:00Z"
}
```

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `id` | `string` | 是 | 文档公开 ID。 |
| `knowledgeBaseId` | `string` | 是 | 所属知识库公开 ID。 |
| `name` | `string` | 是 | 展示用原始文件名或规范化文件名。 |
| `status` | `DocumentStatus` | 是 | 文档处理状态。 |
| `tags` | `string[]` | 否 | 文档标签。 |
| `errorMessage` | `string` | 否 | 处理失败说明。不得包含 object key、内部路径、SQL、MinIO 错误细节或堆栈。 |
| `createdAt` | `string(date-time)` | 是 | 创建时间，使用 RFC 3339 / OpenAPI `date-time`。 |

### DocumentResponse

```json
{
  "data": {
    "id": "doc_123",
    "knowledgeBaseId": "kb_123",
    "name": "设备巡检规范.pdf",
    "status": "uploaded",
    "tags": ["policy", "inspection"],
    "createdAt": "2026-06-28T10:00:00Z"
  },
  "requestId": "req_123"
}
```

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `data` | `DocumentSummary` | 是 | 文档摘要。 |
| `requestId` | `string` | 是 | 请求追踪 ID。 |

## Endpoint 详情

### POST /api/v1/knowledge-bases/{knowledgeBaseId}/documents

上传文件到指定知识库。该接口要求认证。

**Request**

```http
POST /api/v1/knowledge-bases/kb_123/documents
Authorization: Bearer <accessToken>
Content-Type: multipart/form-data
```

Multipart fields:

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `file` | `binary` | 是 | 待上传文件。 |
| `tags` | `string[]` | 否 | 标签列表。 |

**Success**

| Status | Body |
| --- | --- |
| `201 Created` | `DocumentResponse` |

成功后，file 服务应完成：

- 校验认证上下文和上传权限。
- 校验基础文件属性，例如文件名、大小、内容类型和空文件。
- 生成服务端 object key，并将原始文件写入 MinIO。
- 将 file-owned 元数据写入 PostgreSQL。
- 向 `knowledge` 服务提交或触发 ingestion 入口。
- 返回文档公开 ID 和初始文档摘要。

**Error**

当前 OpenAPI 已声明：

| Status | Code | 场景 |
| --- | --- | --- |
| `400` | `validation_error` | 缺少 `file`、multipart 格式错误、文件为空、标签格式非法或文件基础属性不满足规则。 |

后续实现可补充：

| Status | Code | 场景 |
| --- | --- | --- |
| `401` | `unauthorized` | 缺少认证凭据、token 无效或 gateway 未注入有效用户上下文。 |
| `403` | `forbidden` | 已认证但无权向目标知识库上传文件。 |
| `404` | `not_found` | 目标知识库不存在或对当前用户隐藏。 |
| `409` | `conflict` | 文件记录状态冲突，或同一业务范围不允许重复上传。 |
| `429` | `rate_limited` | 上传频率、容量或配额超限。 |

上述补充状态码加入 OpenAPI 前，前端不得作为公开契约依赖。MinIO、PostgreSQL 或 `knowledge` 服务不可用时，gateway 应归一化为 `dependency_error` 或 `internal_error`。

### PATCH /api/v1/documents/{documentId}

更新文档 file-owned 元数据。当前公开契约只包含 `tags`。

**Request**

```http
PATCH /api/v1/documents/doc_123
Authorization: Bearer <accessToken>
Content-Type: application/json
```

```json
{
  "tags": ["policy", "inspection"]
}
```

**Success**

| Status | Body |
| --- | --- |
| `200 OK` | `DocumentResponse` |

**Error**

当前 OpenAPI 已声明：

| Status | Code | 场景 |
| --- | --- | --- |
| `404` | `not_found` | 文档不存在、已删除或对当前用户隐藏。 |

后续实现可补充：

| Status | Code | 场景 |
| --- | --- | --- |
| `400` | `validation_error` | 请求体格式错误、标签格式非法或标签数量超过限制。 |
| `401` | `unauthorized` | 缺少认证凭据或用户上下文无效。 |
| `403` | `forbidden` | 已认证但无权修改该文档。 |
| `409` | `conflict` | 文档处于不允许修改标签的状态。 |

上述补充状态码加入 OpenAPI 前，前端不得作为公开契约依赖。

### DELETE /api/v1/documents/{documentId}

删除文档。该接口要求认证。

**Request**

```http
DELETE /api/v1/documents/doc_123
Authorization: Bearer <accessToken>
```

**Success**

| Status | Body |
| --- | --- |
| `204 No Content` | 无响应体。 |

`204` 表示前端可认为该文档不再可用。物理对象删除、索引清理和异步补偿策略可由后续实现细化，但不能让前端依赖内部 object key 或存储路径。

**Error**

当前 OpenAPI 已声明：

| Status | Code | 场景 |
| --- | --- | --- |
| `404` | `not_found` | 文档不存在、已删除或对当前用户隐藏。 |

后续实现可补充：

| Status | Code | 场景 |
| --- | --- | --- |
| `401` | `unauthorized` | 缺少认证凭据或用户上下文无效。 |
| `403` | `forbidden` | 已认证但无权删除该文档。 |
| `409` | `conflict` | 文档处于不允许删除的状态。 |

上述补充状态码加入 OpenAPI 前，前端不得作为公开契约依赖。删除需要联动 `knowledge` 清理切片和索引时，file 服务不得自行操作 Qdrant；应通过 `knowledge` 拥有的接口或后续事件机制完成。

### GET /api/v1/documents/{documentId}/download

下载原始文件。该接口要求认证。

**Request**

```http
GET /api/v1/documents/doc_123/download
Authorization: Bearer <accessToken>
```

**Success**

| Status | Content-Type | Body |
| --- | --- | --- |
| `200 OK` | `application/octet-stream` 或原始内容类型 | 原始文件二进制流。 |

推荐响应头：

| Header | 说明 |
| --- | --- |
| `Content-Type` | 原始文件内容类型；缺失或不可信时使用 `application/octet-stream`。 |
| `Content-Disposition` | 使用安全转义后的原始文件名，通常为 `attachment`。 |
| `Content-Length` | 文件大小，能可靠获得时返回。 |
| `X-Request-Id` | 与响应体或日志一致的 request id。 |

当前 MVP 不要求断点续传或 Range 下载；如后续支持，需要同步更新 gateway OpenAPI、前后端集成契约和本文档。

**Error**

当前 OpenAPI 已声明：

| Status | Code | 场景 |
| --- | --- | --- |
| `404` | `not_found` | 文档不存在、原始文件不存在、已删除或对当前用户隐藏。 |

后续实现可补充：

| Status | Code | 场景 |
| --- | --- | --- |
| `401` | `unauthorized` | 缺少认证凭据或用户上下文无效。 |
| `403` | `forbidden` | 已认证但无权下载该文档。 |

上述补充状态码加入 OpenAPI 前，前端不得作为公开契约依赖。File 服务不得把 MinIO 内部 URL、bucket、object key 或 access key 返回给前端。

## 内部服务接口初稿

公开契约由 gateway OpenAPI 决定。后续落地 `services/file/` 时，可先让 gateway 使用与公开路径接近的内部 HTTP API，便于联调和测试：

| Method | File Service Path | 说明 |
| --- | --- | --- |
| `GET` | `/healthz` | file 进程存活检查。 |
| `GET` | `/readyz` | file 就绪检查，应覆盖 PostgreSQL 和 MinIO 等关键依赖。 |
| `POST` | `/internal/v1/knowledge-bases/{knowledgeBaseId}/documents` | 接收 gateway 转发的 multipart 上传请求。 |
| `PATCH` | `/internal/v1/documents/{documentId}` | 更新 file-owned 元数据。 |
| `DELETE` | `/internal/v1/documents/{documentId}` | 删除文件记录和原始对象，或标记删除并触发清理。 |
| `GET` | `/internal/v1/documents/{documentId}/download` | 返回原始文件流给 gateway。 |

内部接口也应使用稳定 JSON error shape，并保留 `X-Request-Id`。除下载成功响应外，不要返回裸数据结构。

## 权限与上下文要求

File 服务需要基于 gateway 注入的认证上下文做服务边界校验。初始权限可按以下语义设计，具体权限字符串需与 auth 服务实现保持一致：

| 能力 | 建议权限语义 |
| --- | --- |
| 上传文件 | `document:upload` 或知识库级写权限。 |
| 更新标签 | `document:update` 或知识库级写权限。 |
| 删除文件 | `document:delete` 或知识库级管理权限。 |
| 下载文件 | `document:download`、知识库级读权限或资源所有者权限。 |

资源不存在和无权访问都可以返回 `404 not_found`，用于隐藏资源存在性；需要前端明确展示“无权限”时才返回 `403 forbidden`。

## 对象存储与元数据要求

- PostgreSQL 存储文件元数据、所有权、知识库关联、标签、大小、内容类型、checksum、创建时间和删除状态。
- MinIO 存储原始文件对象；bucket 名按业务目的命名，object key 由 file 服务生成。
- Object key、bucket、内部下载 URL、MinIO 错误和 access key 不得进入前端响应。
- 上传文件名必须做展示层安全处理，不能直接用于 object key。
- 文件删除应优先保证公开 API 视角下不可访问；物理删除失败时应有可重试的清理机制。
- 如果生成报告文件也复用 file 服务存储，应由 `document` 服务拥有报告业务状态，file 只提供对象存储和下载能力。

## 错误码约定

File 相关接口使用项目统一错误码：

| Code | HTTP status | File 场景 |
| --- | --- | --- |
| `validation_error` | `400` | multipart 解析失败、缺少文件、文件为空、标签非法、文件类型或大小不满足规则。 |
| `unauthorized` | `401` | 缺少认证凭据、认证无效或 gateway 未提供有效用户上下文。 |
| `forbidden` | `403` | 已认证但缺少上传、修改、删除或下载权限。 |
| `not_found` | `404` | 文档、知识库或原始对象不存在，或资源对当前用户隐藏。 |
| `conflict` | `409` | 资源状态不允许当前操作，例如已删除或正在执行互斥流程。 |
| `rate_limited` | `429` | 上传频率、容量、数量或租户配额超限。 |
| `dependency_error` | `502` | PostgreSQL、MinIO、knowledge 服务或其他依赖失败并由 gateway 归一化。 |
| `internal_error` | `500` | 未预期服务端错误。 |

## 安全与日志要求

- 不得在日志、错误响应或追踪字段中记录 object key、内部 URL、access key、token、数据库连接串或完整敏感文件内容。
- 日志建议包含 `service`、`request_id`、`operation`、`document_id`、`knowledge_base_id`、`user_id`、`status`、`content_type`、`size_bytes` 等可排查字段。
- 文件名来自用户输入，写入响应头前必须安全转义，避免 header injection。
- 所有跨服务 HTTP client 后续实现必须设置超时，并传递 `context.Context`。
- 上传大小、内容类型白名单、危险文件扫描和租户配额属于实现前必须明确的安全策略。

## 后续实现建议

后续落地 `services/file/` 时，建议服务本地维护：

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
│   ├── platform/
│   │   └── storage/
│   └── client/
│       └── knowledge/
├── migrations/
└── README.md
```

实现前需要补齐或确认：

- 最大上传大小、允许文件类型、空文件和重复文件策略。
- `tags` 在 multipart 中的编码方式，例如重复字段、JSON 字符串或逗号分隔字符串。
- 文件元数据表结构、索引、软删除和物理清理策略。
- MinIO bucket 命名、object key 生成规则和本地开发配置。
- Upload workflow 的最终 owner，以及 file 与 knowledge 的 ingestion handoff 契约。
- 删除时 knowledge chunks、向量索引和原始对象之间的一致性策略。
- 是否支持秒传、checksum 去重、断点续传、预签名下载或 Range 下载。
- 报告导出文件是否通过 file 服务存储，以及 document 服务与 file 服务之间的内部接口。

如果上述决策影响公开字段、错误码或状态码，必须同步更新：

- [`docs/api/gateway.openapi.yaml`](api/gateway.openapi.yaml)
- [`docs/frontend-backend-contract.md`](frontend-backend-contract.md)
- [`docs/service-boundaries.md`](service-boundaries.md)
- 本文档
