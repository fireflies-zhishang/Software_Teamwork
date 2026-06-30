# Parser Runtime Service

This directory defines the internal document parser runtime that Knowledge calls
during ingestion.

Parser is not a business owner service. Knowledge remains the owner of
knowledge documents, ingestion jobs, chunks, embeddings, Qdrant indexing,
retrieval, and parser runtime configuration. Parser only converts raw document
bytes into normalized parsed content that Knowledge can validate, chunk, embed,
and index.

## Runtime

This service is a Python runtime managed with `uv`.

Implemented behavior:

- `GET /healthz` returns process liveness.
- `GET /readyz` reports whether the PaddleOCR runtime dependency is available.
- `POST /internal/v1/parsed-documents` accepts document bytes as base64 and
  returns normalized parsed text in the project `{ data, requestId }` envelope.
- TXT/Markdown, DOCX, PPTX, and XLSX text are parsed directly in the service.
- PDF and image input are routed to PP-StructureV3 by default so structured
  Markdown, tables, formulas, charts, seals, and complex layout regions can be
  preserved when PaddleOCR can recognize them.
- PaddleOCR loading is lazy by default so ordinary tests do not download or
  initialize OCR models.

## Directory Shape

```text
services/parser/
  pyproject.toml
  uv.lock
  Dockerfile
  api/
    openapi.yaml
  src/
    parser_service/
      config/
      http/
      service/
      backends/
        document.py
        paddleocr/
  tests/
```

The docs baseline separates service contracts under `docs/services/parser/api/`:

- `public.openapi.yaml` declares that Parser has no Gateway public API.
- `internal.openapi.yaml` defines the service-to-service Parser contract.

`services/parser/api/openapi.yaml` is the implementation-local copy used by the
Parser runtime and should stay aligned with the docs internal contract.

The implementation language is Python because PaddleOCR's maintained runtime
and examples are Python-first. Go stays on the Knowledge side as an HTTP client
to this service, not as the PaddleOCR runtime host.

## Internal Contract

Knowledge calls parser through the internal HTTP API instead of importing parser
implementation code or PaddleOCR dependencies.

Primary route:

```text
POST /internal/v1/parsed-documents
```

The route accepts raw document bytes as base64 plus metadata such as file name,
content type, and size. It returns normalized parsed text and backend metadata.
Full object storage references, provider bodies, raw OCR debug output, internal
file paths, and secrets are not part of the contract.

Current response data schema:

```json
{
  "content": "full Markdown or text",
  "title": "document title or filename stem",
  "backend": "ppstructurev3",
  "pages": [
    {
      "pageNumber": 1,
      "content": "page Markdown or text",
      "parseStrategy": "ocr",
      "textLayerStatus": "broken",
      "ocrConfidence": 0.91,
      "dpi": 180,
      "warnings": ["low_text_quality"]
    }
  ]
}
```

Page images, table images, bounding boxes, block assets, formula assets, MinIO
object keys, and parser-side lifecycle state are intentionally out of scope for
this lightweight parser contract.

## Local Development

Install the non-OCR development dependencies:

```bash
cd services/parser
uv sync --group dev
```

Run checks:

```bash
uv run ruff check .
uv run pytest
uv run python -m compileall src tests
```

The default pytest suite uses fake OCR coverage and skips the real PaddleOCR
model smoke. To validate an installed PaddleOCR runtime and real model loading
locally, install the optional OCR extra and explicitly enable the smoke:

```bash
uv sync --group dev --extra paddleocr
PARSER_PADDLEOCR_SMOKE=1 \
PARSER_PADDLEOCR_ALLOW_DOWNLOAD=1 \
uv run pytest -m paddleocr_smoke -s
```

For an offline or deployment-like run, provide a PaddleX config that points at
prepared local model files instead of allowing runtime downloads:

```bash
PARSER_PADDLEOCR_SMOKE=1 \
PARSER_PADDLEOCR_CONFIG_PATH=/absolute/path/to/paddlex.yaml \
uv run pytest -m paddleocr_smoke -s
```

The smoke decodes `tests/fixtures/paddleocr_smoke.png.b64`, runs it through
the same `PaddleOCRBackend.parse` path used by the service, and asserts that
OCR returns non-empty text. If `PARSER_PADDLEOCR_SMOKE` is unset, the test is
skipped so ordinary CI and developer checkouts do not need PaddleOCR models.

Run the service with the default dependency set:

```bash
uv run parser-service
```

Run with PaddleOCR and PP-StructureV3 dependencies installed locally:

```bash
uv sync --group dev --extra paddleocr
uv run parser-service
```

Build the runtime image:

```bash
docker build -t software-teamwork-parser:local .
```

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `PARSER_HOST` | `0.0.0.0` | HTTP bind host. |
| `PARSER_PORT` | `8080` | HTTP bind port. |
| `PARSER_SERVICE_TOKEN` | empty | Optional expected `X-Service-Token`. |
| `PARSER_BACKEND` | `ppstructurev3` | Backend selector. `document` parses TXT/Markdown and Office OpenXML without OCR; `ppstructurev3` enables structured PDF/image parsing through PaddleOCR PP-StructureV3; `paddleocr` keeps the legacy line-level OCR path. |
| `PARSER_MAX_DOCUMENT_BYTES` | `8388608` | Maximum decoded document bytes. |
| `PARSER_MAX_CONCURRENCY` | `1` | Maximum concurrent parse jobs in one process. |
| `PARSER_QUEUE_TIMEOUT_SECONDS` | `0` | Queue wait timeout; `0` waits until capacity is available. |
| `PARSER_PARSE_TIMEOUT_SECONDS` | `120` | Per-document backend timeout. |
| `PARSER_LOAD_BACKEND_ON_STARTUP` | `false` | Eagerly load PaddleOCR at startup when true. Keep this false with PP-StructureV3 subprocess isolation in 16 GB deployments to avoid keeping a main-process model resident while child processes load their own models. |
| `PARSER_PROFILE` | `accurate` | Runtime profile marker. Current defaults favor fidelity; lighter profiles must be selected explicitly with model/env overrides. |
| `PARSER_DEFAULT_DPI` | `180` | Initial PDF page render DPI for PP-StructureV3 OCR. |
| `PARSER_RETRY_DPI` | `220` | Retry DPI for pages with low OCR confidence or low text quality. |
| `PARSER_MAX_RETRY_DPI` | `300` | Hard cap for retry DPI. Use 300 only for targeted retries in 16 GB environments. |
| `PARSER_LOW_CONFIDENCE_THRESHOLD` | `0.85` | Page OCR confidence threshold that triggers a higher-DPI retry when confidence is available. |
| `PARSER_PAGE_BATCH_SIZE` | `1` | PDF page batch size per PP-StructureV3 subprocess. Keep `1` for 16 GB precision-first deployments. |
| `PARSER_SUBPROCESS_ISOLATION` | `true` | Runs PP-StructureV3 visual parsing in child processes so Paddle memory is reclaimed after each PDF page batch or image parse. |
| `PARSER_MEMORY_LIMIT_MB` | `14500` | RSS guard for PP-StructureV3 child processes. Exceeding this fails the parse instead of destabilizing WSL. |
| `PADDLEOCR_LANG` | `ch` | PaddleOCR language code. |
| `PADDLEOCR_DEVICE` | `cpu` | PaddleOCR device, for example `cpu` or `gpu`. |
| `PADDLEOCR_ENGINE` | empty | Optional engine override for the legacy `paddleocr` backend. PP-StructureV3 follows the official constructor parameters and does not pass this value. |
| `PADDLEOCR_CONFIG_PATH` | empty | Optional PaddleX config path. |
| `PADDLEOCR_USE_DOC_ORIENTATION_CLASSIFY` | `true` | PaddleOCR document orientation option. Enabled by default for precision-first parsing. |
| `PADDLEOCR_USE_DOC_UNWARPING` | `true` | PaddleOCR document unwarping option. Enabled by default for precision-first parsing. |
| `PADDLEOCR_USE_TEXTLINE_ORIENTATION` | `true` | PaddleOCR textline orientation option. Enabled by default for precision-first parsing. |
| `PADDLEOCR_ENABLE_MKLDNN` | `false` | Enables PaddleX/PaddleOCR MKLDNN on CPU. Defaults to false because PaddlePaddle 3.3.1 can fail on the default MKLDNN path in some CPU environments. |
| `PPSTRUCTUREV3_USE_SEAL_RECOGNITION` | `true` | Enables PP-StructureV3 seal text recognition. |
| `PPSTRUCTUREV3_USE_TABLE_RECOGNITION` | `true` | Enables PP-StructureV3 table recognition. |
| `PPSTRUCTUREV3_USE_FORMULA_RECOGNITION` | `true` | Enables PP-StructureV3 formula recognition. |
| `PPSTRUCTUREV3_USE_CHART_RECOGNITION` | `true` | Enables PP-StructureV3 chart parsing. |
| `PPSTRUCTUREV3_USE_REGION_DETECTION` | `true` | Enables PP-StructureV3 document region detection for complex layouts. |
| `PPSTRUCTUREV3_FORMAT_BLOCK_CONTENT` | `true` | Asks PP-StructureV3 to format block content as Markdown. |
| `PPSTRUCTUREV3_LAYOUT_DETECTION_MODEL_NAME` | empty | Optional layout detection model name. Leave empty to use PaddleOCR's default model selection; set `PP-DocLayout-S` only when explicitly choosing a lighter profile. |
| `PPSTRUCTUREV3_TEXT_DETECTION_MODEL_NAME` | empty | Optional text detection model name, for example `PP-OCRv5_mobile_det`. |
| `PPSTRUCTUREV3_TEXT_RECOGNITION_MODEL_NAME` | empty | Optional text recognition model name, for example `PP-OCRv5_mobile_rec`. |
| `PPSTRUCTUREV3_TEXT_DET_LIMIT_SIDE_LEN` | empty | Optional text detection side limit. PaddleOCR defaults to `960` when unset. |
| `PPSTRUCTUREV3_TEXT_DET_LIMIT_TYPE` | empty | Optional side limit type: `min` or `max`. PaddleOCR defaults to `max` when unset. |
| `PPSTRUCTUREV3_TEXT_RECOGNITION_BATCH_SIZE` | empty | Optional text recognition batch size. PaddleOCR defaults to `1` when unset. |
| `PPSTRUCTUREV3_TEXTLINE_ORIENTATION_BATCH_SIZE` | empty | Optional textline orientation batch size. PaddleOCR defaults to `1` when unset. |
| `PPSTRUCTUREV3_SEAL_TEXT_RECOGNITION_BATCH_SIZE` | empty | Optional seal text recognition batch size. PaddleOCR defaults to `1` when unset. |
| `PPSTRUCTUREV3_FORMULA_RECOGNITION_BATCH_SIZE` | empty | Optional formula recognition batch size. PaddleOCR defaults to `1` when unset. |
| `PPSTRUCTUREV3_CHART_RECOGNITION_BATCH_SIZE` | empty | Optional chart recognition batch size. PaddleOCR defaults to `1` when unset. |
| `PPSTRUCTUREV3_MARKDOWN_IGNORE_LABELS` | empty | Optional comma-separated layout labels to omit from Markdown. |

The PP-StructureV3 backend follows the official PaddleOCR pattern: instantiate
`PPStructureV3`, pass the local PDF/image path as `input`, read each page's
`res.markdown`, and merge pages with `concatenate_markdown_pages`. The runtime
prefers the official `predict_iter()` API when available because it processes
prediction results incrementally and reduces retention of full-page result
objects for large PDFs.

The default configuration favors fidelity for power-industry PDFs. For a
resource-constrained environment, use explicit environment overrides such as
`PPSTRUCTUREV3_LAYOUT_DETECTION_MODEL_NAME=PP-DocLayout-S`,
`PPSTRUCTUREV3_TEXT_DETECTION_MODEL_NAME=PP-OCRv5_mobile_det`, or selective
`PPSTRUCTUREV3_USE_*_RECOGNITION=false`.

Real smoke aliases:

| Variable | Default | Description |
| --- | --- | --- |
| `PARSER_PADDLEOCR_SMOKE` | empty | Set to `1` to run the real PaddleOCR model smoke. |
| `PARSER_PADDLEOCR_ALLOW_DOWNLOAD` | `false` | Allows PaddleOCR to use its default model download/cache behavior during the smoke when no local config path is provided. |
| `PARSER_PADDLEOCR_CONFIG_PATH` | `PADDLEOCR_CONFIG_PATH` | PaddleX config path for prepared local model files. |
| `PARSER_PADDLEOCR_LANG` | `PADDLEOCR_LANG` or `ch` | Smoke-only alias for language selection. |
| `PARSER_PADDLEOCR_DEVICE` | `PADDLEOCR_DEVICE` or `cpu` | Smoke-only alias for CPU/GPU device selection. |
| `PARSER_PADDLEOCR_ENGINE` | `PADDLEOCR_ENGINE` | Smoke-only alias for engine override. |

For CPU-only local validation, reserve at least 2 vCPU and 4 GiB memory; first
model downloads or larger model packs may need 1-2 GiB of writable cache space.
Use GPU only after the local PaddlePaddle runtime, driver, and model config have
already been validated outside the ordinary test suite.

Troubleshooting:

- Missing `paddleocr` or `paddle` modules: run `uv sync --group dev --extra paddleocr`.
- Missing model files in offline mode: set `PARSER_PADDLEOCR_CONFIG_PATH` or
  `PADDLEOCR_CONFIG_PATH` to an existing PaddleX config with local model paths.
- Unexpected downloads in restricted environments: do not set
  `PARSER_PADDLEOCR_ALLOW_DOWNLOAD`; use prepared local model paths instead.
- Empty OCR output: verify the model language/device, model cache completeness,
  and that the fixture image can be read by the local PaddleOCR install.

## Deployment Boundary

Parser is deployed separately from Knowledge so PP-StructureV3/OCR model
loading, GPU/CPU scheduling, and document parsing concurrency can evolve
without coupling those dependencies to the Knowledge service process.
