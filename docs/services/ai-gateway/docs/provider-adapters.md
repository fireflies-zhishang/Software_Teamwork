# AI Gateway Provider Adapter 说明

日期：2026-06-29

本文记录 `services/ai-gateway/internal/provider/` 当前 provider adapter 的运行约束、校验规则和后续扩展方式。服务职责、API 字段和数据模型仍以 AI Gateway README、OpenAPI 和 data-models 为准。

## 当前结论

AI Gateway 当前已经实现三类模型调用：

| 能力 | endpoint | provider path | 当前 adapter |
| --- | --- | --- | --- |
| Chat completion | `POST /internal/v1/chat/completions` | `<baseUrl>/chat/completions` | `provider.HTTPChatClient` |
| Embeddings | `POST /internal/v1/embeddings` | `<baseUrl>/embeddings` | `provider.HTTPClient` |
| Rerankings | `POST /internal/v1/rerankings` | `<baseUrl>/rerank` | `provider.HTTPClient` |

`provider` 字段当前用于配置分类和后续差异化扩展；`openai_compatible`、`siliconflow`、`local_compatible` 在当前 adapter 中共享同一套 HTTP 路径拼接、header、错误归一化和响应校验逻辑。新增 provider 特异行为时，必须先写清兼容差异和测试，而不是在领域服务中绕过 AI Gateway。

## 调用边界

| 规则 | 说明 |
| --- | --- |
| Profile 是唯一 provider 配置源 | 调用方只能传 `profile_id` 和 `model`；base URL、provider、API key、timeout、默认参数由 AI Gateway profile 决定。 |
| `model` 必须精确匹配 profile | 请求 `model` 与选中 profile `model` 不一致时，在调用 provider 前返回 `validation_error`。 |
| API key 只在内存中解密 | 解密后的 key 只用于当前 provider request；响应、日志、错误、调用记录和指标不得包含明文。 |
| AI Gateway 不保存业务正文 | 不保存完整 prompt、embedding payload、rerank 文档正文、provider 原始响应体或用户上传文档全文。 |
| 领域服务仍拥有业务状态 | QA 拥有会话/Agent/MCP，Knowledge 拥有 chunk/vector/retrieval，Document 拥有报告/job/file。 |

## Chat adapter

`HTTPChatClient` 支持非流式和流式 OpenAI-compatible chat completion。

| 行为 | 当前实现 |
| --- | --- |
| 请求路径 | 将 profile `baseUrl` 清理 query/fragment 后追加 `/chat/completions`。 |
| Header | `Content-Type: application/json`、`Authorization: Bearer <apiKey>`，有 request id 时透传 `X-Request-Id`。 |
| 非流式校验 | provider 响应必须是 JSON，`object=chat.completion` 且 `choices` 非空；可解析 `usage`。 |
| 流式校验 | provider HTTP status 成功后转发 SSE；HTTP handler 会清洗 chunk，只保留 OpenAI-compatible 字段和 usage。 |
| Function calling | `tools`、`tool_choice`、`parallel_tool_calls`、assistant `tool_calls` 和 tool message 字段由 chat payload 透传；AI Gateway 不执行工具。 |
| 失败归一化 | provider `401/403/429/5xx` 映射到 OpenAI-style error type 和项目错误码；错误 body 被丢弃，不透传原文。 |

## Embedding adapter

`HTTPClient.CreateEmbeddings` 发送 OpenAI-compatible embedding 请求。

| 行为 | 当前实现 |
| --- | --- |
| 请求路径 | `<baseUrl>/embeddings`。 |
| 请求体 | `model`、`input`、`dimensions`、`encoding_format`、`user` 加 profile `defaultParameters`。 |
| profile 规则 | `purpose=embedding`；profile 必须 enabled、未删除、已配置 active credential。 |
| 维度规则 | 请求 `dimensions` 优先；缺省时使用 profile `dimensions`；最终必须 `> 0`。 |
| 响应形态 | 返回 OpenAI-compatible list：`object=list`、`data[]`、`model`、可选 `usage`。 |
| 响应校验 | `data` 数量必须等于输入数量；每项 `object=embedding`；`index` 必须等于输入位置，不允许重复、越界或乱序；`embedding` 必须存在，且在 `encoding_format=float` 时为 JSON number array，在 `encoding_format=base64` 时为 base64 string。 |
| 持久化摘要 | 只记录 operation、profile、model、provider、input_count、embedding_dimensions、usage、duration、status 和归一化错误。 |

Embedding payload 可能是 float array，也可能在 `encoding_format=base64` 时是 base64 string；两种形态都可能泄露原文语义，不能进入普通日志、数据库调用摘要、错误响应或指标 label。向量持久化只归 Knowledge/Qdrant。

## Reranking adapter

`HTTPClient.CreateReranking` 发送 OpenAI-style rerank 请求。OpenAI 官方 API 没有标准 rerank endpoint，本项目约定 `/rerank` 为兼容扩展路径。

| 行为 | 当前实现 |
| --- | --- |
| 请求路径 | `<baseUrl>/rerank`。 |
| 请求体 | `model`、`query`、`documents`、`top_n`、`metadata` 加 profile `defaultParameters`。 |
| 文档传输 | provider 请求只发送 `documents[]` 文本；AI Gateway 响应不返回文档正文。 |
| profile 规则 | `purpose=rerank`；profile 必须 enabled、未删除、已配置 active credential。 |
| topN 规则 | 请求 `top_n` 优先；缺省时使用 profile `topN`；最终 effective topN 必须 `> 0`，并用于裁剪最终响应 `data`。 |
| 兼容响应 | 支持 provider 返回 `data[]` 或 `results[]`；`score` 或 `relevance_score` 会归一化为 `score`。 |
| 响应校验 | `index` 必须落在请求 documents 范围内；不允许重复；如 provider 返回 `document_id`，必须匹配对应请求文档 ID；最终响应必须有 `document_id`；provider 返回结果多于 effective topN 时，最终响应 `data` 必须截断到 effective topN。 |
| usage 归一化 | 支持 OpenAI-style `usage`；也可从 `meta.tokens.input_tokens/output_tokens` 映射。 |

Rerank `documents[].text` 只能用于 provider request，不能写入调用记录或错误文案。业务过滤、引用格式和召回候选集由 Knowledge/QA 决定。

## 调用记录和用量聚合

模型调用会写入 `provider_invocations`，并按小时更新 `model_usage_aggregates`。

| 字段 | 说明 |
| --- | --- |
| `operation` | `chat_completion`、`embedding` 或 `reranking`。 |
| `profile_id` / `provider` / `model` | 记录调用配置摘要。 |
| `caller_service` / `external_user_id` / `request_id` | 只用于审计、排障和未来配额，不做领域权限判断。 |
| `prompt_tokens` / `completion_tokens` / `total_tokens` | 来自 provider usage 或兼容映射。 |
| `input_count` / `embedding_dimensions` / `rerank_top_n` | embedding/rerank 的低敏调用摘要。 |
| `normalized_error_code` / `normalized_error_type` | 记录归一化错误，不记录 provider 原始 body。 |

当前未实现真实配额扣减、成本报表、Prometheus metrics 或 OpenTelemetry tracing。新增这些能力时，指标 label 只能使用低基数字段，不得包含 prompt、query、document text、embedding payload、API key 或 object key。

## 新增 provider 适配检查清单

1. 确认 provider 是否能兼容 `/chat/completions`、`/embeddings`、`/rerank` 路径；不能兼容时先在 adapter 层显式分支。
2. 为 provider request/response 写 fake HTTP tests，覆盖成功、401、403、429、5xx、timeout 和非契约 body。
3. 验证 profile `model` exact-match 仍在调用 provider 前生效。
4. 验证 API key、prompt、embedding、rerank 文档正文、provider 原始错误不会出现在响应、日志、调用记录和测试失败输出中。
5. 对 streaming provider，验证 `[DONE]`、usage chunk、取消和缺失终止事件的行为。
6. 对 embedding/rerank provider，验证数量、index、document_id、score、usage 和乱序/重复/越界防护。
7. 更新 AI Gateway README、OpenAPI、data-models、implementation 和本文档。

## 当前测试覆盖和缺口

| 范围 | 当前覆盖 | 缺口 |
| --- | --- | --- |
| Chat | fake provider HTTP tests 覆盖非流式、流式、function-calling 字段、失败记录和脱敏。 | 真实 provider smoke、stream cancel、provider 特异 chunk 行为。 |
| Embeddings | HTTP/service tests 覆盖 OpenAI shape、profile model mismatch、count/index 校验和脱敏。 | 真实 provider smoke、不同维度/encoding_format 的兼容性验证。 |
| Rerankings | HTTP/service tests 覆盖 document_id 映射、document text 不泄露、provider response 校验。 | 真实 provider smoke、不同 rerank response shape 的回归样本。 |
| PostgreSQL | 调用摘要和 usage aggregate repository path 已接入 runtime。 | 独立 DB migration/repository smoke 需要本地或 CI PostgreSQL 环境。 |
