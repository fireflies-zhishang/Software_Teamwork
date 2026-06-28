# Gateway 服务规划

本文档定义 `gateway` 服务在项目初期的职责边界和基础契约。目标是让前端只依赖一个稳定入口，同时让 `auth`、`file`、`knowledge`、`qa`、`document` 等服务可以按清晰边界并行开发。

## 设计原则

- `gateway` 是面向前端的后端统一入口，不是业务大单体。
- 前端只调用 `gateway` 暴露的 `/api/v1/**` 接口，不直接调用内部服务。
- `gateway` 通过 HTTP/REST 调用内部服务，不 import 其他服务的 Go `internal/` 包。
- 领域业务规则尽量留在拥有该领域数据和流程的服务中。
- 跨服务聚合接口必须有明确前端场景，不能把所有服务编排都放进 `gateway`。
- OpenAPI 契约先行，代码实现必须跟随契约变更。

## Gateway 应负责

| 能力 | 说明 |
| --- | --- |
| Public API surface | 暴露前端使用的 `/api/v1/**` HTTP API。 |
| Routing | 将已确定的公开请求转发到 `auth`、`file` 等内部服务；未定下游服务只保留缺失占位。 |
| Auth context | 基于 Redis 会话缓存读取用户身份，并向下游传递用户、角色、权限和 request id。 |
| Session cache | 登录或注册成功后缓存 auth 返回的会话身份信息，后续请求优先从 Redis 获取会话上下文。 |
| Response contract | 对前端保持统一成功响应、分页响应和错误响应结构。 |
| Request correlation | 生成或透传 `X-Request-Id`，并要求下游服务保留该 request id。 |
| Cross-service aggregation | 仅在前后端契约明确后提供聚合读接口；本轮管理后台概览暂标缺失。 |
| Streaming entrypoint | 问答和报告生成的 SSE/流式入口暂未确定，本轮只记录缺失状态。 |
| Edge policy | 集中处理 CORS、基础请求头、请求大小原则、健康检查和公开 API 命名。 |

## Gateway 不应负责

| 领域 | 归属服务 | Gateway 不做什么 |
| --- | --- | --- |
| 用户、密码、会话、角色权限源数据 | `auth` | 不保存密码，不维护用户表，不实现 RBAC 持久化；只在 Redis 保存运行时会话缓存。 |
| 文件对象、文件元数据、对象存储协调 | `file` | 不直接操作 MinIO，不生成业务 object key。 |
| 知识库、文档切片、向量索引、检索策略 | `knowledge` | 不执行切片、嵌入、Qdrant 查询或重排序。 |
| 问答、意图识别、RAG、LLM 调用 | `qa` | 不拼 prompt，不执行 RAG pipeline，不保存对话业务状态。 |
| 报告大纲、章节生成、DOCX 导出 | `document` | 不生成报告内容，不操作报告模板业务规则。 |
| 服务数据库迁移 | 各领域服务 | 不拥有其他服务的 migrations。 |

## Public API 命名

第一版公开 API 使用统一版本前缀：

```text
/api/v1
```

健康检查接口不带版本前缀：

```text
/healthz
/readyz
```

当前已确定路径分组：

| Gateway path | 初始 owner | 说明 |
| --- | --- | --- |
| `/healthz` | `gateway` | 进程存活检查。 |
| `/readyz` | `gateway` | 就绪检查。 |
| `/api/v1/auth/**` | `auth` | 注册、登录、登出、当前用户。 |
| `/api/v1/knowledge-bases/{knowledgeBaseId}/documents` | `file` | 文件上传入口。知识库存在性校验和 ingestion handoff 契约暂未确定。 |
| `/api/v1/documents/{documentId}` | `file` | 更新 file-owned 文档元数据、删除原始文件记录。 |
| `/api/v1/documents/{documentId}/download` | `file` | 下载原始文件。 |

暂缺的下游接口：

| Placeholder | 预期 owner | 状态 |
| --- | --- | --- |
| `GET/POST /api/v1/knowledge-bases` 和 `GET/PATCH/DELETE /api/v1/knowledge-bases/{knowledgeBaseId}` | `knowledge` | 缺失：知识库 CRUD 契约未定。 |
| `GET /api/v1/knowledge-bases/{knowledgeBaseId}/documents`、`GET /api/v1/documents/{documentId}`、`GET /api/v1/documents/{documentId}/chunks` | `knowledge` | 缺失：知识库内文档列表、文档详情和 chunks 契约未定。 |
| `/api/v1/search` | `knowledge` | 缺失：检索请求、过滤、排序、返回引用格式未定。 |
| `/api/v1/chat/**` | `qa` | 缺失：会话、消息、非流式/流式回答、引用事件格式未定。 |
| `/api/v1/reports/**` | `document` | 缺失：报告记录、大纲、章节生成、导出和下载契约未定。 |
| `/api/v1/admin/**` | `gateway` + domain services | 缺失：聚合指标来源和展示字段未定。 |

当某个 endpoint 涉及两个服务时，文档必须显式标注 workflow owner。默认规则是：拥有核心业务状态的服务拥有流程，gateway 只做入口和上下文传递。

## 认证与上下文传递

认证机制初期采用 bearer token + Redis 会话缓存。Auth 服务负责认证、签发会话身份和撤销会话；Gateway 负责在登录或注册成功后写入 Redis，并在后续请求中从 Redis 读取会话上下文。

前端请求：

- 登录类接口不要求认证。
- 业务接口必须携带认证凭据。
- 前端不直接设置用户身份 header，用户身份由 gateway 认证后注入。
- 后续请求使用 `Authorization: Bearer <accessToken>` 携带 gateway 返回的访问令牌。

会话缓存流程：

1. 前端调用 `/api/v1/auth/login` 或 `/api/v1/auth/register`。
2. Gateway 将请求转发给 auth 服务。
3. Auth 服务校验凭证，返回用户身份、角色、权限、`sessionId`、`accessToken` 和 `expiresAt`。
4. Gateway 将完整会话身份写入 Redis，缓存键使用 `gateway:session:<accessTokenHash>`，TTL 与 `expiresAt` 对齐。
5. 前端后续请求携带 `Authorization: Bearer <accessToken>`。
6. Gateway 从 Redis 查询会话；命中且未过期时，不需要每次调用 auth 服务。
7. Gateway 基于缓存的会话身份向下游服务注入 `X-User-Id`、`X-User-Roles`、`X-User-Permissions` 和 `X-Request-Id`。
8. 登出时 Gateway 调用 auth 撤销会话，并删除 Redis 中的对应缓存。

Redis 会话缓存值应至少包含：

| 字段 | 说明 |
| --- | --- |
| `sessionId` | Auth 服务签发的会话 ID。 |
| `userId` | 已认证用户 ID。 |
| `username` | 用户名，用于审计和调试，不作为权限判断唯一依据。 |
| `roles` | 角色列表。 |
| `permissions` | 权限字符串列表。 |
| `accessTokenHash` | 访问令牌哈希，避免把原始 token 当作可读缓存字段。 |
| `expiresAt` | 会话过期时间，使用 RFC 3339 / OpenAPI `date-time`。 |
| `issuedAt` | 会话签发时间。 |

缓存规则：

- Redis 是运行时会话缓存，不是用户、角色、权限的持久化源数据。
- 每条会话缓存必须设置明确 TTL。
- Gateway 日志和错误响应不得输出原始 token、session secret 或 Redis 连接信息。
- Redis 未命中、会话过期或缓存内容无效时，Gateway 返回 `401 unauthorized`，前端回到登录流程。
- Redis 不可用时，业务接口返回 `502 dependency_error`；登录、注册和登出等 auth 流程可以按实现策略选择失败或降级，但必须保持错误 envelope 一致。
- 权限变更、用户禁用或安全事件需要让旧会话失效时，auth 服务应提供撤销能力，Gateway 删除对应 Redis 会话缓存。

Gateway 调用下游服务时应传递：

| Header | 说明 |
| --- | --- |
| `X-Request-Id` | 贯穿一次前端请求的 request id。 |
| `X-User-Id` | 已认证用户 ID。 |
| `X-User-Roles` | 逗号分隔的角色列表。 |
| `X-User-Permissions` | 逗号分隔的权限列表，字段细节由 auth 契约细化。 |
| `X-Forwarded-For` | 原始客户端地址链。 |
| `X-Forwarded-Proto` | 原始协议。 |

下游服务仍需在自己的边界做权限校验，不能只依赖前端传参。

## Gateway Auth 接口

Gateway 对前端暴露 auth 相关公开接口，具体 schema 以 [`docs/api/gateway.openapi.yaml`](api/gateway.openapi.yaml) 为准。

| Method | Path | Auth | Gateway 行为 | Auth service 行为 |
| --- | --- | --- | --- | --- |
| `POST` | `/api/v1/auth/register` | 不需要 | 转发注册请求，成功后写入 Redis 会话缓存并返回统一 envelope。 | 创建用户、计算角色权限、签发会话身份。 |
| `POST` | `/api/v1/auth/login` | 不需要 | 转发登录请求，成功后写入 Redis 会话缓存并返回统一 envelope。 | 校验凭证、计算角色权限、签发会话身份。 |
| `POST` | `/api/v1/auth/logout` | 需要 | 从 Redis 定位当前会话，调用 auth 撤销会话，删除 Redis 缓存。 | 撤销会话或令牌，记录安全事件。 |
| `GET` | `/api/v1/auth/me` | 需要 | 从 Redis 会话缓存读取当前用户并返回 `UserResponse`。 | 拥有用户和权限源数据；默认不参与每次 `/me` 查询。 |

登录和注册成功响应包含：

```json
{
  "data": {
    "user": {
      "id": "usr_123",
      "username": "alice",
      "roles": ["admin"],
      "permissions": ["knowledge:read", "document:upload"]
    },
    "session": {
      "sessionId": "sess_123",
      "accessToken": "eyJ...",
      "tokenType": "Bearer",
      "expiresAt": "2026-06-28T12:00:00Z"
    }
  },
  "requestId": "req_123"
}
```

Gateway 必须只把 `data.session.accessToken` 返回给前端，不得把 Redis key、token hash、内部 auth URL 或 session secret 暴露给前端。

## 响应约定

成功响应使用稳定 JSON envelope：

```json
{
  "data": {},
  "requestId": "req_123"
}
```

分页响应：

```json
{
  "data": [],
  "page": {
    "page": 1,
    "pageSize": 20,
    "total": 100
  },
  "requestId": "req_123"
}
```

错误响应遵循后端规范：

```json
{
  "error": {
    "code": "validation_error",
    "message": "request validation failed",
    "requestId": "req_123",
    "fields": {
      "name": "is required"
    }
  }
}
```

错误码初始集合：

| Code | HTTP status |
| --- | --- |
| `validation_error` | `400` |
| `unauthorized` | `401` |
| `forbidden` | `403` |
| `not_found` | `404` |
| `conflict` | `409` |
| `rate_limited` | `429` |
| `dependency_error` | `502` |
| `internal_error` | `500` |

## 缺失下游接口

知识库、问答、报告生成和管理后台聚合的前后端接口尚未完全确定。当前 OpenAPI 只在顶层 `x-missing-contracts` 标记缺失范围，不把这些 endpoint 作为可依赖的公开契约。

后续补齐任一缺失接口时，需要同步更新：

- `docs/api/gateway.openapi.yaml`
- `docs/frontend-backend-contract.md`
- `docs/service-boundaries.md`
- 对应服务接口文档

## 健康检查

| Endpoint | 说明 |
| --- | --- |
| `GET /healthz` | 进程存活检查，只表示 gateway 进程可响应。 |
| `GET /readyz` | 就绪检查，后续可包含关键下游依赖状态。 |

## 后续扩展

本轮只定义基础契约包。以下内容后续单独细化：

- 下游服务完整内部 API 索引。
- 超时、重试、熔断和断线重连策略。
- API 版本兼容策略。
- 限流、审计和安全事件记录。
- 多端 BFF 拆分条件。
