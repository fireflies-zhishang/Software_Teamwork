# PP-StructureV3 Backend

This backend wraps PaddleOCR's `PPStructureV3` pipeline for PDF and image
documents that need structured output.

The implementation follows the official PaddleOCR usage pattern:

```python
from paddleocr import PPStructureV3

pipeline = PPStructureV3(...)
output = pipeline.predict_iter(input=file_path)
for res in output:
    markdown = res.markdown
```

For image input, it calls the official pipeline directly on the local image
path. For PDF input, Parser first renders pages with `pypdfium2`, then sends
each rendered page image through the official PP-StructureV3 pipeline. This
keeps the official `predict(input=tmp_file)`/`predict_iter(input=tmp_file)`
runtime path while letting Parser control DPI, page batching, subprocess
isolation, and RSS limits.

When multiple markdown page results are available from a single pipeline call,
the backend merges pages through
`pipeline.concatenate_markdown_pages(markdown_list)`. When `predict_iter()` is
unavailable in the installed PaddleOCR runtime, the backend falls back to
`predict(input=file_path)`.

The backend only retains lightweight Markdown/text content for the current
contract. Page images, table images, bounding boxes, and block assets are not
returned or persisted by Parser in this phase. Page-level quality metadata such
as render DPI, text-layer status, OCR confidence, and warnings is returned as
optional fields on `ParsedPage`.

Subprocess isolation returns results through a temporary pickle file, not
`multiprocessing.Queue`. A full `ParsedDocument` can be large for long or
table-heavy PDFs, and queue feeder flushing can keep the child process alive
while the parent waits for process exit. File-backed results keep the parent
free to monitor RSS, enforce subprocess timeout, and terminate the child before
the outer `ParserService` timeout releases the request with a stuck backend
thread.

Page retry heuristics should stay narrow. Missing text and low OCR confidence
still trigger a DPI retry, but short English labels, numeric tables, and other
non-CJK pages should not be treated as low-quality by default. Only obvious
encoding-artefact pages such as `I¥J` / `B"J` sequences keep the retry warning.

Default deployments keep `PARSER_LOAD_BACKEND_ON_STARTUP=false` when
`PARSER_SUBPROCESS_ISOLATION=true`. Loading PP-StructureV3 in the main process
while each parse batch also loads it in a child process can double model memory
pressure. `/readyz` should therefore use lightweight runtime dependency and
constructor-argument checks unless an operator explicitly chooses eager model
loading.

The backend keeps PP-StructureV3 inside the Parser runtime boundary. Knowledge
continues to receive normalized parsed content over HTTP and remains
responsible for chunking, embedding, indexing, and retrieval.
