# Legacy PaddleOCR Backend

Legacy PaddleOCR-specific Python runtime code lives in this package area.

The adapter lazily imports and initializes `paddleocr.PaddleOCR`, then supports
both the PaddleOCR 3.x `predict` interface and the older `ocr` interface. It
extracts text from common PaddleOCR result shapes and returns normalized parsed
document data to the service layer.

The default PDF/image parser backend is `ppstructurev3`, implemented in
`parser_service.backends.ppstructurev3`, because standards, manuals, and other
document-like PDFs need Markdown, table, formula, chart, seal, and layout-aware
output instead of only line-level OCR text. Set `PARSER_BACKEND=paddleocr` only
when the legacy plain OCR path is explicitly required.

PaddleOCR is an optional dependency in local development:

```bash
uv sync --group dev --extra paddleocr
```

The runtime Dockerfile installs the `paddleocr` extra by default.
