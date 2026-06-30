# Parser Runtime Service

Parser 是内部文档解析运行时服务，由 Knowledge ingestion 调用，不通过
public gateway API 暴露给前端。

## 职责边界

Parser 只负责把原始文档 bytes 转成规范化解析结果。默认目标后端是
Python + PaddleOCR PP-StructureV3，用于扫描 PDF、图片、表格、公式、图表、
印章和 OCR-heavy 版式；旧的 PaddleOCR 行级 OCR 后端只作为兼容模式保留。

Knowledge 仍然负责：

- 知识库文档资源、processing job 和公开文档状态。
- 从 File Service 获取原始文件引用和做业务可见性校验。
- 切片、chunk 持久化、embedding 生成、Qdrant 写入和检索 hydrate。
- 管理端 parser runtime configuration 的公开 gateway 契约。

Parser 不保存知识库、文档、chunk、embedding、Qdrant point 或权限事实；也不得返回
object key、bucket、内部 URL、签名 URL、provider body、API key、prompt 或完整调试日志。

## 内部 API

Parser 的服务间契约是：

```text
POST /internal/v1/parsed-documents
```

机器可读契约分为两份：

- [`api/public.openapi.yaml`](api/public.openapi.yaml)：Parser 没有 Gateway 公开路径，前端、管理端和 MCP 调用方不得直连 Parser。
- [`api/internal.openapi.yaml`](api/internal.openapi.yaml)：Parser 服务间内部契约，当前只供 Knowledge ingestion 调用。

Knowledge 调用 Parser 时必须传递 `X-Request-Id`、`X-Caller-Service: knowledge`
和必要的内部 service token。`X-User-Id` 只作为审计上下文，不作为 Parser 里的授权事实。

## 运行时方向

Parser 是使用 `uv` 管理的 Python 服务。它用 FastAPI 提供内部 HTTP API，直接解析
TXT/Markdown 和 Office OpenXML 格式，并把 PDF/image 路径默认路由到 PaddleOCR
PP-StructureV3，以 Markdown 作为结构化文本的主输出。
Go 服务只通过 HTTP 调用 Parser，不应在 Knowledge 进程中引入 PaddleOCR、
PaddlePaddle、OpenCV、CUDA 或模型加载依赖。

## 当前状态

当前实现：

- `services/parser/pyproject.toml` 和 `uv.lock` 固定 Python runtime、FastAPI、Uvicorn、测试工具和可选 PaddleOCR extra。
- `services/parser/src/parser_service/http` 实现 `/healthz`、`/readyz` 和 `POST /internal/v1/parsed-documents`。
- `services/parser/src/parser_service/service` 处理 base64 校验、文档大小限制、解析超时、并发限制和响应归一化。
- `services/parser/src/parser_service/backends/document.py` 直接解析 TXT/Markdown、DOCX、PPTX、XLSX，并把 PDF/images 路由到 OCR/structure backend。
- `services/parser/src/parser_service/backends/ppstructurev3.py` 按 PaddleOCR 官方 PP-StructureV3 示例处理 PDF/image：实例化 `PPStructureV3`，传入本地文件路径作为 `input`，读取每页 `res.markdown`，并通过 `concatenate_markdown_pages` 合并 Markdown。为了在 16GB 内存下稳定运行，PDF 默认先由 `pypdfium2` 按页渲染，再按 `PARSER_PAGE_BATCH_SIZE=1` 启动 PP-StructureV3 子进程解析，批次结束后释放 Paddle 内存。
- PP-StructureV3 默认偏向电力行业复杂 PDF 的解析保真度，开启表格、公式、图表、印章和 region 子管线；内存控制通过 `PARSER_MAX_CONCURRENCY=1`、分页/小批、子进程隔离、`PARSER_MEMORY_LIMIT_MB` 和低置信度页 DPI 重试完成。低内存部署可通过 `PPSTRUCTUREV3_LAYOUT_DETECTION_MODEL_NAME=PP-DocLayout-S` 或 `PPSTRUCTUREV3_USE_*` 环境变量显式降级，但这不是默认策略。
- `services/parser/src/parser_service/backends/paddleocr` lazy-load legacy PaddleOCR，并归一化 PaddleOCR 2.x 和 3.x 常见结果形态。
- `services/parser/Dockerfile` 构建带 PaddleOCR extra 的 runtime image。

当前 `POST /internal/v1/parsed-documents` 响应的 `data` schema 只包含：

- `content`：全文 Markdown/text。
- `title`：标题或文件名 stem。
- `backend`：实际 parser backend，例如 `ppstructurev3`。
- `pages[]`：页级结果，至少包含 `pageNumber` 和 `content`；当前可选质量字段为 `parseStrategy`、`textLayerStatus`、`ocrConfidence`、`dpi`、`warnings`。

页图、表格图、bbox、block/table/formula 资产化、MinIO object key 和文档生命周期仍不属于 Parser 当前职责。

验证：

```bash
cd services/parser
uv run ruff check .
uv run pytest
uv run python -m compileall src tests
```

默认测试套件使用 fake OCR backend；真实 PaddleOCR 模型 smoke 只有显式设置
`PARSER_PADDLEOCR_SMOKE=1` 才会运行，因此普通 CI 和普通开发者环境不需要安装或下载模型。

真实模型本地 smoke：

```bash
cd services/parser
uv sync --group dev --extra paddleocr
PARSER_PADDLEOCR_SMOKE=1 \
PARSER_PADDLEOCR_ALLOW_DOWNLOAD=1 \
uv run pytest -m paddleocr_smoke -s
```

离线或部署近似环境应使用准备好的本地模型配置：

```bash
PARSER_PADDLEOCR_SMOKE=1 \
PARSER_PADDLEOCR_CONFIG_PATH=/absolute/path/to/paddlex.yaml \
uv run pytest -m paddleocr_smoke -s
```

资源预估：CPU-only smoke 建议至少 2 vCPU、4 GiB 内存；首次下载或准备模型缓存时预留
1-2 GiB 可写磁盘。该 smoke 只证明 PaddleOCR runtime、模型加载和最小 fixture OCR
可用，不等同于 #125 的跨服务端到端 smoke。
