# File 服务实现说明

版本：v0.1
日期：2026-06-29
范围：`services/file/` 的 Go service 结构、配置、HTTP、repository、对象存储、迁移、日志和测试落地约定

## 1. 文档定位

本文补充 [`File 服务接口文档`](../README.md) 和 [`File 服务数据模型文档`](data-models.md) 中的实现约束，确保 `services/file/` 后续代码按 [`docs/architecture/technology-decisions.md`](../../../architecture/technology-decisions.md) 的最新技术基线推进。

机器可读内部 API 契约见 [`services/file/api/openapi.yaml`](../../../../services/file/api/openapi.yaml)。前端公开契约仍以 [`docs/services/gateway/api/openapi.yaml`](../../gateway/api/openapi.yaml) 为准。

## 2. 当前实现状态

当前 `services/file/` 是可运行 Go module，已具备：

- `GET /healthz`
- `GET /readyz`
- knowledge-document 形态的 MVP 兼容路由
- memory repository
- memory object store
- `internal/config`、`internal/http`、`internal/service`、`internal/repository` 和 `internal/platform/storage` 分层雏形

这些兼容路由只服务当前联调，不继续承载新增业务字段。目标内部资源是：

```text
POST   /internal/v1/files
GET    /internal/v1/files/{fileId}
DELETE /internal/v1/files/{fileId}
GET    /internal/v1/files/{fileId}/content
```

## 3. 目标目录约定

后续扩展继续保持服务本地边界：

```text
services/file/
  api/
    openapi.yaml
  cmd/server/
  internal/
    config/
    http/
    service/
    repository/
      queries/
      sqlc/
    platform/
      storage/
        minio/
  migrations/
  sqlc.yaml
```

`internal/http` 不直接 import MinIO SDK、`sqlc` 生成包或其他服务的 `internal/` 包。handler 调用 service/use-case，service 调用 repository 和 object store port。

## 4. 配置约定

配置在 `internal/config` 使用 `envconfig` 风格结构化加载。可以先手写解析，但必须有类型化 struct、默认值、必填校验和脱敏输出。

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `FILE_HTTP_ADDR` | `:8082` | HTTP listen address。 |
| `FILE_SHUTDOWN_TIMEOUT` | `10s` | 优雅退出超时。 |
| `FILE_MAX_UPLOAD_BYTES` | `33554432` | multipart 上传大小上限。 |
| `FILE_STORAGE_BACKEND` | `memory` | 当前支持 `memory` 和符合 `ObjectStore` 接口的 `local` adapter；生产应使用 `minio` 或等价持久对象存储。 |
| `FILE_LOCAL_STORAGE_DIR` | `.file-storage` | `FILE_STORAGE_BACKEND=local` 时的本地对象存储根目录。 |
| `FILE_DATABASE_URL` | 无 | PostgreSQL DSN；启用 PostgreSQL repository 时必填。 |
| `FILE_MINIO_ENDPOINT` | 无 | MinIO endpoint。 |
| `FILE_MINIO_ACCESS_KEY` | 无 | MinIO access key，禁止日志输出。 |
| `FILE_MINIO_SECRET_KEY` | 无 | MinIO secret key，禁止日志输出。 |
| `FILE_MINIO_BUCKET` | 无 | File Service 拥有的 bucket。 |
| `FILE_MINIO_USE_SSL` | `false` | 是否使用 HTTPS 连接 MinIO。 |
| `FILE_LOG_LEVEL` | `info` | `slog` 日志级别。 |
| `FILE_REDIS_ADDR` | 无 | 仅当对象清理 worker 使用 `asynq` 时需要。 |

配置校验失败时服务启动失败，不在运行时静默降级到不持久的存储后端。脱敏配置输出只能显示字段是否配置，不能显示 secret、DSN 密码或对象存储凭据。

## 5. HTTP 与中间件

HTTP 路由、统一 envelope、错误响应和 request id 规则以 [`docs/architecture/frontend-backend-contract.md`](../../../architecture/frontend-backend-contract.md) 和 [`docs/architecture/technology-decisions.md`](../../../architecture/technology-decisions.md) 为准。本文只记录 File Service 的服务内补充要求。

中间件顺序建议：

```text
request id -> recover -> timeout -> body limit -> internal auth -> access log -> handler
```

要求：

- 文件流成功响应不包 JSON envelope，失败仍返回统一错误响应。
- `X-Request-Id` 应进入响应头、响应体、日志和内部下游调用。
- 上传文件名写入 `Content-Disposition` 前必须安全转义。

## 6. Repository 与迁移

PostgreSQL 访问使用 `pgx` + `sqlc`。不使用 ORM。

约定：

- `services/file/sqlc.yaml` 由 File Service 自己维护。
- SQL 文件放在 `services/file/internal/repository/queries/`。
- 生成代码放在 `services/file/internal/repository/sqlc/`。
- `sqlc` 生成代码只被 repository 适配层使用，不进入 handler。
- 事务由 service/use-case 层发起；repository 接收 `pgx.Tx` 或抽象 querier。
- SQL 必须显式列名，不使用 `SELECT *`。
- 用户输入只能通过参数绑定进入 SQL。

迁移使用 `goose`，文件放在 `services/file/migrations/`。首批迁移建议从 `file_objects` 开始，再按需要增加 `file_deletion_tasks` 和 `file_object_events`。第一阶段允许 forward-only migration；如果提供 down migration，必须在本地和 CI 可验证。

## 7. 对象存储

对象存储使用官方 MinIO Go SDK，封装在 `internal/platform/storage/minio` 的 object store adapter 内。

实现要求：

- object key 由 File Service 服务端生成，不能直接使用用户上传文件名。
- bucket、object key、etag、version id、内部 URL 和 MinIO 错误详情不得返回给前端公开 API。
- owner service 只保存 `file_ref` 和必要的展示快照，例如文件名、大小和 content type。
- checksum 可由调用方传入，也可由 File Service 计算；如果两者都存在，必须校验一致。
- memory object store 只用于单元测试和早期本地联调，不代表持久化能力。
- local object store 可用于本地持久化 smoke test，仍不得把本地路径或 object key 返回给 handler 或 owner service client。

## 8. 删除与清理

删除流程以 PostgreSQL 状态为准：

```text
available -> delete_requested -> purging -> purged
                         \-> failed
```

`DELETE /internal/v1/files/{fileId}` 至少应保证普通读取不再返回该文件。物理对象删除可以同步执行，也可以创建 `file_deletion_tasks` 并由 `asynq` worker 执行。若使用队列，任务类型使用：

```text
file:object:purge
```

asynq payload 只包含 `requestId`、`jobId`、`fileId`、`callerService` 等可追踪字段，不包含 bucket、object key、内部 URL、secret 或文件内容。

## 9. 日志、指标和安全

日志使用 `log/slog`，生产默认 JSON 输出。

建议日志字段：

| 字段 | 说明 |
| --- | --- |
| `service` | 固定 `file`。 |
| `request_id` | 请求追踪 ID。 |
| `operation` | `create_file`、`get_file`、`delete_file`、`get_file_content` 等。 |
| `status` | `succeeded` 或 `failed`。 |
| `file_id` | 内部 file ID，可记录。 |
| `caller_service` | 调用方服务。 |
| `content_type` | 文件 MIME type。 |
| `size_bytes` | 文件大小。 |

禁止记录：

- object key、bucket 内部路径、预签名 URL、MinIO access key 或 secret key
- bearer token、服务间 token、数据库连接串
- 原始文件内容、文档全文、prompt 或 provider 原始响应体
- 未脱敏的存储错误详情

指标第一阶段采用 Prometheus 风格 endpoint。指标 label 不得包含用户输入正文、文件名、object key、token、API key 指纹或原始错误信息。

## 10. 测试要求

使用 Go 标准 `testing` 和 `httptest`。

最低覆盖建议：

- health/readiness handler。
- multipart 创建文件：缺少文件、空文件、超出大小、checksum 非法、成功创建。
- metadata 读取：存在、删除后不可读、不存在。
- content 读取：成功返回二进制流和响应头；失败返回统一错误 envelope。
- 删除流程：状态流转、幂等行为和物理清理失败摘要。
- repository：迁移后 query 的约束、索引相关查询和状态更新。
- storage adapter：MinIO 错误映射、checksum 校验和 context timeout。

接口契约变化必须同步更新：

- [`services/file/api/openapi.yaml`](../../../../services/file/api/openapi.yaml)
- [`docs/services/file/README.md`](../README.md)
- [`docs/services/file/docs/data-models.md`](data-models.md)
- [`docs/services/gateway/api/openapi.yaml`](../../gateway/api/openapi.yaml)，仅当公开契约受影响时
