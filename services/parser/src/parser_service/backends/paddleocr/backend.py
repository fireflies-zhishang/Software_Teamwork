from __future__ import annotations

import importlib
import json
import mimetypes
import tempfile
import threading
from collections.abc import Mapping
from pathlib import Path
from typing import Any

from parser_service.service import (
    BackendHealth,
    ParsedDocument,
    ParsedPage,
    ParseRequest,
    dependency_error,
    validation_error,
)


class PaddleOCRBackend:
    name = "paddleocr"

    def __init__(
        self,
        *,
        lang: str = "ch",
        device: str = "cpu",
        engine: str = "",
        paddlex_config: str = "",
        use_doc_orientation_classify: bool = False,
        use_doc_unwarping: bool = False,
        use_textline_orientation: bool = False,
        enable_mkldnn: bool = False,
    ) -> None:
        self._lang = lang.strip() or "ch"
        self._device = device.strip()
        self._engine = engine.strip()
        self._paddlex_config = paddlex_config.strip()
        self._use_doc_orientation_classify = use_doc_orientation_classify
        self._use_doc_unwarping = use_doc_unwarping
        self._use_textline_orientation = use_textline_orientation
        self._enable_mkldnn = enable_mkldnn
        self._pipeline: Any | None = None
        self._load_error: str = ""
        self._lock = threading.Lock()

    def health(self) -> BackendHealth:
        if self._load_error:
            return BackendHealth(
                ready=False,
                status="not_ready",
                reason="paddleocr model load failed",
            )
        try:
            importlib.import_module("paddleocr")
        except Exception:
            return BackendHealth(
                ready=False,
                status="not_ready",
                reason="paddleocr package is not installed",
            )
        return BackendHealth(ready=True, status="ready")

    def warm_up(self) -> None:
        self._ensure_pipeline()

    def parse(self, request: ParseRequest) -> ParsedDocument:
        pipeline = self._ensure_pipeline()
        suffix = _suffix_for(request.document_name, request.content_type)

        with tempfile.NamedTemporaryFile(suffix=suffix) as tmp:
            tmp.write(request.data)
            tmp.flush()
            try:
                raw_result = _predict(pipeline, tmp.name)
            except Exception as exc:
                raise dependency_error("paddleocr parse failed") from exc

        pages = _pages_from_result(raw_result)
        texts = []
        for page in pages:
            texts.append(page.content)
        if not texts:
            texts = extract_texts(raw_result)

        content = _normalize_text("\n".join(texts))
        if not content:
            raise validation_error("document could not be parsed", {"file": "no text content"})

        title = _title_from_name(request.document_name)
        return ParsedDocument(content=content, title=title, backend=self.name, pages=pages)

    def _ensure_pipeline(self) -> Any:
        if self._pipeline is not None:
            return self._pipeline
        with self._lock:
            if self._pipeline is not None:
                return self._pipeline
            try:
                module = importlib.import_module("paddleocr")
                paddle_ocr = module.PaddleOCR
                kwargs: dict[str, Any] = {
                    "lang": self._lang,
                    "use_doc_orientation_classify": self._use_doc_orientation_classify,
                    "use_doc_unwarping": self._use_doc_unwarping,
                    "use_textline_orientation": self._use_textline_orientation,
                    "enable_mkldnn": self._enable_mkldnn,
                }
                if self._device:
                    kwargs["device"] = self._device
                if self._engine:
                    kwargs["engine"] = self._engine
                if self._paddlex_config:
                    kwargs["paddlex_config"] = self._paddlex_config
                self._pipeline = paddle_ocr(**kwargs)
            except Exception as exc:
                self._load_error = "paddleocr model load failed"
                raise dependency_error("paddleocr backend is not ready") from exc
        return self._pipeline


def _predict(pipeline: Any, path: str) -> Any:
    predict = getattr(pipeline, "predict", None)
    if callable(predict):
        return predict(path)
    ocr = getattr(pipeline, "ocr", None)
    if callable(ocr):
        return ocr(path, cls=True)
    raise RuntimeError("PaddleOCR pipeline does not expose predict or ocr")


def extract_texts(result: Any) -> list[str]:
    texts: list[str] = []
    _collect_texts(result, texts)
    return [_normalize_line(text) for text in texts if _normalize_line(text)]


def _pages_from_result(result: Any) -> list[ParsedPage]:
    if not isinstance(result, list):
        content = _normalize_text("\n".join(extract_texts(result)))
        return [ParsedPage(page_number=1, content=content)] if content else []

    pages: list[ParsedPage] = []
    for index, page_result in enumerate(result, start=1):
        page_content = _normalize_text("\n".join(extract_texts(page_result)))
        if page_content:
            pages.append(ParsedPage(page_number=index, content=page_content))
    return pages


def _collect_texts(value: Any, texts: list[str]) -> None:
    if value is None:
        return

    structured = _structured_value(value)
    if structured is not None and structured is not value:
        _collect_texts(structured, texts)
        return

    if isinstance(value, Mapping):
        for key in ("rec_texts", "texts"):
            if key in value:
                _collect_string_list(value[key], texts)
        for key in ("text", "content", "recognized_text"):
            item = value.get(key)
            if isinstance(item, str):
                texts.append(item)
        for key in ("res", "result", "results", "data", "pages", "ocr"):
            if key in value:
                _collect_texts(value[key], texts)
        return

    if isinstance(value, list | tuple):
        if _looks_like_v2_text_line(value):
            texts.append(str(value[1][0]))
            return
        if value and all(isinstance(item, str) for item in value):
            texts.extend(value)
            return
        for item in value:
            _collect_texts(item, texts)


def _structured_value(value: Any) -> Any | None:
    for attr in ("json", "to_dict", "dict", "model_dump"):
        candidate = getattr(value, attr, None)
        if candidate is None:
            continue
        try:
            candidate_value = candidate() if callable(candidate) else candidate
        except TypeError:
            continue
        if isinstance(candidate_value, str):
            try:
                return json.loads(candidate_value)
            except json.JSONDecodeError:
                continue
        if isinstance(candidate_value, Mapping | list | tuple):
            return candidate_value
    return None


def _collect_string_list(value: Any, texts: list[str]) -> None:
    if isinstance(value, list | tuple):
        for item in value:
            if isinstance(item, str):
                texts.append(item)


def _looks_like_v2_text_line(value: list[Any] | tuple[Any, ...]) -> bool:
    return (
        len(value) == 2
        and isinstance(value[1], list | tuple)
        and len(value[1]) >= 1
        and isinstance(value[1][0], str)
    )


def _suffix_for(document_name: str, content_type: str) -> str:
    allowed = {".pdf", ".png", ".jpg", ".jpeg", ".tif", ".tiff", ".bmp", ".webp"}
    suffix = Path(document_name).suffix.lower()
    if suffix in allowed:
        return suffix
    guessed = mimetypes.guess_extension(content_type.split(";")[0].strip().lower())
    if guessed == ".jpe":
        return ".jpg"
    if guessed in allowed:
        return guessed
    return ".bin"


def _title_from_name(document_name: str) -> str:
    return Path(document_name).stem.strip()


def _normalize_text(value: str) -> str:
    return "\n".join(
        line for line in (_normalize_line(line) for line in value.splitlines()) if line
    )


def _normalize_line(value: str) -> str:
    return " ".join(value.strip().split())
