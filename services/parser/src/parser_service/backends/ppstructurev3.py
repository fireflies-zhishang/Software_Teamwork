from __future__ import annotations

import importlib
import inspect
import multiprocessing as mp
import pickle
import tempfile
import threading
import time
from collections.abc import Iterable
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from parser_service.backends.paddleocr.backend import _suffix_for, extract_texts
from parser_service.service import (
    AppError,
    BackendHealth,
    ParsedDocument,
    ParsedPage,
    ParseRequest,
    dependency_error,
    validation_error,
)


@dataclass(frozen=True)
class _ChildSuccess:
    parsed: ParsedDocument


@dataclass(frozen=True)
class _ChildFailure:
    code: str
    message: str
    fields: dict[str, str] | None = None


class PPStructureV3Backend:
    name = "ppstructurev3"

    def __init__(
        self,
        *,
        lang: str = "ch",
        device: str = "cpu",
        engine: str = "paddle",
        paddlex_config: str = "",
        use_doc_orientation_classify: bool = False,
        use_doc_unwarping: bool = False,
        use_textline_orientation: bool = False,
        use_seal_recognition: bool = True,
        use_table_recognition: bool = True,
        use_formula_recognition: bool = True,
        use_chart_recognition: bool = True,
        use_region_detection: bool = True,
        format_block_content: bool = True,
        enable_mkldnn: bool = False,
        layout_detection_model_name: str = "",
        text_detection_model_name: str = "",
        text_recognition_model_name: str = "",
        text_det_limit_side_len: int | None = None,
        text_det_limit_type: str = "",
        text_recognition_batch_size: int | None = None,
        textline_orientation_batch_size: int | None = None,
        seal_text_recognition_batch_size: int | None = None,
        formula_recognition_batch_size: int | None = None,
        chart_recognition_batch_size: int | None = None,
        markdown_ignore_labels: list[str] | None = None,
        profile: str = "accurate",
        default_dpi: int = 180,
        retry_dpi: int = 220,
        max_retry_dpi: int = 300,
        low_confidence_threshold: float = 0.85,
        page_batch_size: int = 1,
        subprocess_isolation: bool = True,
        subprocess_timeout_seconds: float = 0.0,
        memory_limit_mb: int = 14500,
    ) -> None:
        self._lang = lang.strip() or "ch"
        self._device = device.strip()
        self._engine = engine.strip()
        self._paddlex_config = paddlex_config.strip()
        self._use_doc_orientation_classify = use_doc_orientation_classify
        self._use_doc_unwarping = use_doc_unwarping
        self._use_textline_orientation = use_textline_orientation
        self._use_seal_recognition = use_seal_recognition
        self._use_table_recognition = use_table_recognition
        self._use_formula_recognition = use_formula_recognition
        self._use_chart_recognition = use_chart_recognition
        self._use_region_detection = use_region_detection
        self._format_block_content = format_block_content
        self._enable_mkldnn = enable_mkldnn
        self._layout_detection_model_name = layout_detection_model_name.strip()
        self._text_detection_model_name = text_detection_model_name.strip()
        self._text_recognition_model_name = text_recognition_model_name.strip()
        self._text_det_limit_side_len = text_det_limit_side_len
        self._text_det_limit_type = text_det_limit_type.strip()
        self._text_recognition_batch_size = text_recognition_batch_size
        self._textline_orientation_batch_size = textline_orientation_batch_size
        self._seal_text_recognition_batch_size = seal_text_recognition_batch_size
        self._formula_recognition_batch_size = formula_recognition_batch_size
        self._chart_recognition_batch_size = chart_recognition_batch_size
        self._markdown_ignore_labels = markdown_ignore_labels
        self._profile = profile.strip() or "accurate"
        self._default_dpi = default_dpi
        self._retry_dpi = retry_dpi
        self._max_retry_dpi = max_retry_dpi
        self._low_confidence_threshold = low_confidence_threshold
        self._page_batch_size = page_batch_size
        self._subprocess_isolation = subprocess_isolation
        self._subprocess_timeout_seconds = subprocess_timeout_seconds
        self._memory_limit_mb = memory_limit_mb
        self._pipeline: Any | None = None
        self._load_error: str = ""
        self._lock = threading.Lock()

    def health(self) -> BackendHealth:
        if self._load_error:
            return BackendHealth(
                ready=False,
                status="not_ready",
                reason="ppstructurev3 model load failed",
            )
        if self._pipeline is not None:
            return BackendHealth(ready=True, status="ready")
        try:
            module = importlib.import_module("paddleocr")
            pp_structure_v3 = module.PPStructureV3
        except Exception:
            return BackendHealth(
                ready=False,
                status="not_ready",
                reason="paddleocr PPStructureV3 runtime is not installed",
            )
        try:
            importlib.import_module("pypdfium2")
        except Exception:
            return BackendHealth(
                ready=False,
                status="not_ready",
                reason="pypdfium2 runtime is not installed",
            )
        if not _callable_accepts_kwargs(pp_structure_v3, self._pipeline_kwargs()):
            return BackendHealth(
                ready=False,
                status="not_ready",
                reason="ppstructurev3 constructor arguments are not compatible",
            )
        return BackendHealth(ready=True, status="ready")

    def warm_up(self) -> None:
        self._ensure_pipeline()

    def parse(self, request: ParseRequest) -> ParsedDocument:
        suffix = _suffix_for(request.document_name, request.content_type)

        with tempfile.NamedTemporaryFile(suffix=suffix, delete=False) as tmp:
            tmp.write(request.data)
            tmp.flush()
            tmp_path = Path(tmp.name)
        try:
            if self._subprocess_isolation and _is_visual_input(tmp_path, request.content_type):
                if _is_pdf(tmp_path, request.content_type):
                    return _parse_pdf_in_subprocess_batches(
                        tmp_path,
                        document_name=request.document_name,
                        config=self._child_config(),
                    )
                return _parse_path_in_subprocess(
                    tmp_path,
                    document_name=request.document_name,
                    content_type=request.content_type,
                    config=self._child_config(),
                )
            return self._parse_path(
                tmp_path,
                document_name=request.document_name,
                content_type=request.content_type,
            )
        finally:
            try:
                tmp_path.unlink(missing_ok=True)
            except OSError:
                pass

    def _parse_path(
        self,
        input_path: Path,
        *,
        document_name: str,
        content_type: str,
    ) -> ParsedDocument:
        if _is_pdf(input_path, content_type):
            return self._parse_pdf_pages(input_path, document_name=document_name)
        return self._parse_visual_path(
            input_path,
            document_name=document_name,
            page_number=1,
            parse_strategy="ocr",
            text_layer_status="",
            dpi=None,
        )

    def _parse_pdf_pages(
        self,
        input_path: Path,
        *,
        document_name: str,
        page_indexes: list[int] | None = None,
        allow_empty: bool = False,
    ) -> ParsedDocument:
        try:
            import pypdfium2 as pdfium
        except Exception as exc:
            raise dependency_error("pypdfium2 is required for page-level PDF parsing") from exc

        pages: list[ParsedPage] = []
        with tempfile.TemporaryDirectory(prefix="parser-ppstructurev3-pages-") as temp_dir:
            temp_path = Path(temp_dir)
            try:
                doc = pdfium.PdfDocument(str(input_path))
            except Exception as exc:
                raise validation_error(
                    "document could not be parsed",
                    {"file": "invalid pdf"},
                ) from exc
            try:
                page_count = len(doc)
                indexes = page_indexes if page_indexes is not None else list(range(page_count))
                for index in indexes:
                    if index < 0 or index >= page_count:
                        raise validation_error(
                            "document could not be parsed",
                            {"file": "invalid page range"},
                        )
                    page_number = index + 1
                    parsed_page = self._parse_pdf_page(
                        doc,
                        index,
                        page_number=page_number,
                        temp_path=temp_path,
                    )
                    if parsed_page.content:
                        pages.append(parsed_page)
            finally:
                doc.close()

        content = _normalize_text("\n\n".join(page.content for page in pages))
        if not content:
            if allow_empty:
                return ParsedDocument(
                    content="",
                    title=Path(document_name).stem.strip(),
                    backend=self.name,
                    pages=[],
                )
            raise validation_error("document could not be parsed", {"file": "no text content"})

        return ParsedDocument(
            content=content,
            title=Path(document_name).stem.strip(),
            backend=self.name,
            pages=pages,
        )

    def _parse_pdf_page(
        self,
        doc: Any,
        page_index: int,
        *,
        page_number: int,
        temp_path: Path,
    ) -> ParsedPage:
        text_layer = ""
        text_layer_status = "empty"
        best_page: ParsedPage | None = None
        warnings: list[str] = []
        for dpi in self._attempt_dpis():
            page = doc[page_index]
            textpage = None
            bitmap = None
            image = None
            try:
                if not text_layer:
                    textpage = page.get_textpage()
                    text_layer = textpage.get_text_range()
                    text_layer_status = _text_layer_status(text_layer)
                image_path = temp_path / f"page_{page_number}_{dpi}dpi.png"
                bitmap = page.render(scale=dpi / 72)
                image = bitmap.to_pil()
                image.save(image_path)
            finally:
                if textpage is not None:
                    textpage.close()
                _close_quietly(image)
                _close_quietly(bitmap)
                page.close()

            parsed_page = self._parse_visual_page(
                image_path,
                page_number=page_number,
                parse_strategy="ocr",
                text_layer_status=text_layer_status,
                dpi=dpi,
                warnings=warnings,
            )
            if best_page is None or _page_quality_score(parsed_page.content) > _page_quality_score(
                best_page.content
            ):
                best_page = parsed_page
            warning = _page_retry_warning(parsed_page, self._low_confidence_threshold)
            if not warning:
                return parsed_page
            warnings = [*parsed_page.warnings, warning]

        if best_page is None:
            return ParsedPage(
                page_number=page_number,
                content="",
                parse_strategy="ocr",
                text_layer_status=text_layer_status,
                dpi=self._default_dpi,
                warnings=["no_text_content"],
            )
        return ParsedPage(
            page_number=best_page.page_number,
            content=best_page.content,
            parse_strategy=best_page.parse_strategy,
            text_layer_status=best_page.text_layer_status,
            ocr_confidence=best_page.ocr_confidence,
            dpi=best_page.dpi,
            warnings=best_page.warnings,
        )

    def _parse_visual_path(
        self,
        input_path: Path,
        *,
        document_name: str,
        page_number: int,
        parse_strategy: str,
        text_layer_status: str,
        dpi: int | None,
    ) -> ParsedDocument:
        page = self._parse_visual_page(
            input_path,
            page_number=page_number,
            parse_strategy=parse_strategy,
            text_layer_status=text_layer_status,
            dpi=dpi,
            warnings=[],
        )
        if not page.content:
            raise validation_error("document could not be parsed", {"file": "no text content"})
        return ParsedDocument(
            content=page.content,
            title=Path(document_name).stem.strip(),
            backend=self.name,
            pages=[page],
        )

    def _parse_visual_page(
        self,
        input_path: Path,
        *,
        page_number: int,
        parse_strategy: str,
        text_layer_status: str,
        dpi: int | None,
        warnings: list[str],
    ) -> ParsedPage:
        pipeline = self._ensure_pipeline()
        try:
            raw_result = _predict_result(pipeline, str(input_path))
            pages, markdown_items, fallback_texts, confidences = _collect_pages_and_markdown(
                raw_result,
            )
        except Exception as exc:
            raise dependency_error("ppstructurev3 parse failed") from exc

        content = _merged_markdown_from_items(markdown_items, pipeline)
        if not content:
            content = _normalize_text("\n\n".join(page.content for page in pages))
        if not content:
            content = _normalize_text("\n".join(fallback_texts))
        confidence = _mean(confidences)
        page_warnings = list(warnings)
        if confidence is None:
            page_warnings.append("ocr_confidence_unavailable")

        return ParsedPage(
            page_number=page_number,
            content=content,
            parse_strategy=parse_strategy,
            text_layer_status=text_layer_status,
            ocr_confidence=confidence,
            dpi=dpi,
            warnings=_deduplicate(page_warnings),
        )

    def _ensure_pipeline(self) -> Any:
        if self._pipeline is not None:
            return self._pipeline
        with self._lock:
            if self._pipeline is not None:
                return self._pipeline
            try:
                module = importlib.import_module("paddleocr")
                pp_structure_v3 = module.PPStructureV3
                kwargs = self._pipeline_kwargs()
                self._pipeline = pp_structure_v3(**kwargs)
            except Exception as exc:
                self._load_error = "ppstructurev3 model load failed"
                raise dependency_error("ppstructurev3 backend is not ready") from exc
        return self._pipeline

    def _pipeline_kwargs(self) -> dict[str, Any]:
        kwargs: dict[str, Any] = {
            "lang": self._lang,
            "use_doc_orientation_classify": self._use_doc_orientation_classify,
            "use_doc_unwarping": self._use_doc_unwarping,
            "use_textline_orientation": self._use_textline_orientation,
            "use_seal_recognition": self._use_seal_recognition,
            "use_table_recognition": self._use_table_recognition,
            "use_formula_recognition": self._use_formula_recognition,
            "use_chart_recognition": self._use_chart_recognition,
            "use_region_detection": self._use_region_detection,
            "format_block_content": self._format_block_content,
            "enable_mkldnn": self._enable_mkldnn,
        }
        optional_values: dict[str, object | None] = {
            "device": self._device or None,
            "paddlex_config": self._paddlex_config or None,
            "layout_detection_model_name": self._layout_detection_model_name or None,
            "text_detection_model_name": self._text_detection_model_name or None,
            "text_recognition_model_name": self._text_recognition_model_name or None,
            "text_det_limit_side_len": self._text_det_limit_side_len,
            "text_det_limit_type": self._text_det_limit_type or None,
            "text_recognition_batch_size": self._text_recognition_batch_size,
            "textline_orientation_batch_size": self._textline_orientation_batch_size,
            "seal_text_recognition_batch_size": self._seal_text_recognition_batch_size,
            "formula_recognition_batch_size": self._formula_recognition_batch_size,
            "chart_recognition_batch_size": self._chart_recognition_batch_size,
            "markdown_ignore_labels": self._markdown_ignore_labels,
        }
        kwargs.update({key: value for key, value in optional_values.items() if value is not None})
        return kwargs

    def _child_config(self) -> dict[str, Any]:
        return {
            "lang": self._lang,
            "device": self._device,
            "engine": self._engine,
            "paddlex_config": self._paddlex_config,
            "use_doc_orientation_classify": self._use_doc_orientation_classify,
            "use_doc_unwarping": self._use_doc_unwarping,
            "use_textline_orientation": self._use_textline_orientation,
            "use_seal_recognition": self._use_seal_recognition,
            "use_table_recognition": self._use_table_recognition,
            "use_formula_recognition": self._use_formula_recognition,
            "use_chart_recognition": self._use_chart_recognition,
            "use_region_detection": self._use_region_detection,
            "format_block_content": self._format_block_content,
            "enable_mkldnn": self._enable_mkldnn,
            "layout_detection_model_name": self._layout_detection_model_name,
            "text_detection_model_name": self._text_detection_model_name,
            "text_recognition_model_name": self._text_recognition_model_name,
            "text_det_limit_side_len": self._text_det_limit_side_len,
            "text_det_limit_type": self._text_det_limit_type,
            "text_recognition_batch_size": self._text_recognition_batch_size,
            "textline_orientation_batch_size": self._textline_orientation_batch_size,
            "seal_text_recognition_batch_size": self._seal_text_recognition_batch_size,
            "formula_recognition_batch_size": self._formula_recognition_batch_size,
            "chart_recognition_batch_size": self._chart_recognition_batch_size,
            "markdown_ignore_labels": self._markdown_ignore_labels,
            "profile": self._profile,
            "default_dpi": self._default_dpi,
            "retry_dpi": self._retry_dpi,
            "max_retry_dpi": self._max_retry_dpi,
            "low_confidence_threshold": self._low_confidence_threshold,
            "page_batch_size": self._page_batch_size,
            "subprocess_isolation": False,
            "subprocess_timeout_seconds": self._subprocess_timeout_seconds,
            "memory_limit_mb": self._memory_limit_mb,
        }

    def _attempt_dpis(self) -> list[int]:
        default_dpi = max(72, self._default_dpi)
        retry_dpi = max(72, min(self._retry_dpi, self._max_retry_dpi))
        dpis = [default_dpi]
        if retry_dpi > default_dpi:
            dpis.append(retry_dpi)
        return dpis


def _predict_result(pipeline: Any, input_path: str) -> Any:
    predict_iter = getattr(pipeline, "predict_iter", None)
    if callable(predict_iter):
        return predict_iter(input=input_path)
    return pipeline.predict(input=input_path)


def _callable_accepts_kwargs(callable_obj: Any, kwargs: dict[str, Any]) -> bool:
    try:
        signature = inspect.signature(callable_obj)
    except (TypeError, ValueError):
        return True
    parameters = signature.parameters.values()
    accepted_names: set[str] = set()
    accepts_extra_kwargs = False
    for parameter in parameters:
        if parameter.kind is inspect.Parameter.VAR_KEYWORD:
            accepts_extra_kwargs = True
            break
        if parameter.kind in {
            inspect.Parameter.POSITIONAL_OR_KEYWORD,
            inspect.Parameter.KEYWORD_ONLY,
        }:
            accepted_names.add(parameter.name)
    return accepts_extra_kwargs or all(key in accepted_names for key in kwargs)


def _collect_pages_and_markdown(
    result: Any,
) -> tuple[list[ParsedPage], list[dict[str, Any]], list[str], list[float]]:
    pages: list[ParsedPage] = []
    markdown_items: list[dict[str, Any]] = []
    fallback_texts: list[str] = []
    confidences: list[float] = []

    for index, item in enumerate(_iter_result(result), start=1):
        confidences.extend(_result_confidences(item))
        markdown = _result_markdown(item)
        if markdown is not None:
            markdown_items.append(_lightweight_markdown(markdown))
            content = _markdown_text(markdown)
        else:
            content = _normalize_text("\n".join(extract_texts(item)))
            if content:
                fallback_texts.append(content)
        if content:
            pages.append(ParsedPage(page_number=index, content=content))

    return pages, markdown_items, fallback_texts, confidences


def _pages_from_result(result: Any) -> list[ParsedPage]:
    pages, _, _, _ = _collect_pages_and_markdown(result)
    return pages


def _merged_markdown_from_result(result: Any, pipeline: Any) -> str:
    _, markdown_items, _, _ = _collect_pages_and_markdown(result)
    return _merged_markdown_from_items(markdown_items, pipeline)


def _merged_markdown_from_items(markdown_items: list[dict[str, Any]], pipeline: Any) -> str:
    if not markdown_items:
        return ""
    if len(markdown_items) == 1:
        return _markdown_text(markdown_items[0])
    concatenate = getattr(pipeline, "concatenate_markdown_pages", None)
    if callable(concatenate):
        try:
            return _concatenated_markdown_text(concatenate(markdown_items))
        except Exception:
            return ""
    return _normalize_text("\n\n".join(_markdown_text(markdown) for markdown in markdown_items))


def _iter_result(result: Any):
    if isinstance(result, dict | str | bytes):
        yield result
        return
    if isinstance(result, Iterable):
        yield from result
        return
    yield result


def _concatenated_markdown_text(value: Any) -> str:
    if isinstance(value, str):
        return _normalize_text(value)
    if isinstance(value, dict):
        return _markdown_text(value)
    if isinstance(value, tuple | list):
        for item in value:
            if not isinstance(item, str | dict | tuple | list):
                continue
            text = _concatenated_markdown_text(item)
            if text:
                return text
    return ""


def _lightweight_markdown(markdown: dict[str, Any]) -> dict[str, Any]:
    lightweight: dict[str, Any] = {}
    for key in (
        "markdown_texts",
        "markdown_text",
        "markdown",
        "text",
        "content",
        "page_continuation_flags",
    ):
        value = markdown.get(key)
        if value is not None:
            lightweight[key] = _lightweight_markdown(value) if isinstance(value, dict) else value
    return lightweight


def _result_markdown(value: Any) -> dict[str, Any] | None:
    markdown_texts = getattr(value, "markdown_texts", None)
    if isinstance(markdown_texts, str):
        return {"markdown_texts": markdown_texts}

    markdown = getattr(value, "markdown", None)
    if isinstance(markdown, dict):
        return markdown
    if callable(markdown):
        try:
            candidate = markdown()
        except TypeError:
            return None
        if isinstance(candidate, dict):
            return candidate
    to_dict = getattr(value, "to_dict", None)
    if callable(to_dict):
        try:
            candidate = to_dict()
        except TypeError:
            return None
        if isinstance(candidate, dict):
            return _markdown_from_mapping(candidate)
    json_data = getattr(value, "json", None)
    if isinstance(json_data, dict):
        return _markdown_from_mapping(json_data)
    if callable(json_data):
        try:
            candidate = json_data()
        except TypeError:
            return None
        if isinstance(candidate, dict):
            return _markdown_from_mapping(candidate)
    return None


def _markdown_text(markdown: dict[str, Any]) -> str:
    for key in ("markdown_texts", "markdown_text", "markdown", "text", "content"):
        value = markdown.get(key)
        if isinstance(value, str):
            return _normalize_text(value)
        if isinstance(value, list):
            text = _normalize_text("\n".join(str(item) for item in value if item is not None))
            if text:
                return text
        if isinstance(value, dict):
            text = _markdown_text(value)
            if text:
                return text
    return ""


def _markdown_from_mapping(value: dict[str, Any]) -> dict[str, Any] | None:
    markdown = value.get("markdown")
    if isinstance(markdown, dict):
        return markdown
    for key in ("markdown_texts", "markdown_text", "markdown", "text", "content"):
        if key in value:
            return {key: value[key]}
    res = value.get("res")
    if isinstance(res, dict):
        return _markdown_from_mapping(res)
    return None


def _result_confidences(value: Any) -> list[float]:
    mapping = _result_mapping(value)
    if mapping is None:
        return []
    confidences: list[float] = []
    _collect_confidences(mapping, confidences)
    return confidences


def _result_mapping(value: Any) -> dict[str, Any] | None:
    if isinstance(value, dict):
        return value
    to_dict = getattr(value, "to_dict", None)
    if callable(to_dict):
        try:
            candidate = to_dict()
        except TypeError:
            return None
        if isinstance(candidate, dict):
            return candidate
    json_data = getattr(value, "json", None)
    if isinstance(json_data, dict):
        return json_data
    if callable(json_data):
        try:
            candidate = json_data()
        except TypeError:
            return None
        if isinstance(candidate, dict):
            return candidate
    return None


def _collect_confidences(value: Any, confidences: list[float]) -> None:
    if isinstance(value, dict):
        for key, item in value.items():
            if key in {"score", "scores", "rec_score", "rec_scores", "confidence", "confidences"}:
                _append_confidence(item, confidences)
            elif isinstance(item, dict | list | tuple):
                _collect_confidences(item, confidences)
    elif isinstance(value, list | tuple):
        for item in value:
            if isinstance(item, dict | list | tuple):
                _collect_confidences(item, confidences)


def _append_confidence(value: Any, confidences: list[float]) -> None:
    if isinstance(value, int | float) and not isinstance(value, bool):
        if 0 <= float(value) <= 1:
            confidences.append(float(value))
        return
    if isinstance(value, list | tuple):
        for item in value:
            _append_confidence(item, confidences)


def _parse_path_in_subprocess(
    input_path: Path,
    *,
    document_name: str,
    content_type: str,
    config: dict[str, Any],
) -> ParsedDocument:
    result = _run_child_process(
        target=_child_parse_path,
        args=(str(input_path), document_name, content_type, config),
        config=config,
    )
    return _unwrap_child_result(result)


def _parse_pdf_in_subprocess_batches(
    input_path: Path,
    *,
    document_name: str,
    config: dict[str, Any],
) -> ParsedDocument:
    page_count = _pdf_page_count(input_path)
    batch_size = max(1, int(config.get("page_batch_size", 1)))
    pages: list[ParsedPage] = []
    for start in range(0, page_count, batch_size):
        page_indexes = list(range(start, min(start + batch_size, page_count)))
        parsed = _parse_pdf_batch_in_subprocess(
            input_path,
            document_name=document_name,
            config=config,
            page_indexes=page_indexes,
        )
        pages.extend(parsed.pages)

    content = _normalize_text("\n\n".join(page.content for page in pages))
    if not content:
        raise validation_error("document could not be parsed", {"file": "no text content"})
    return ParsedDocument(
        content=content,
        title=Path(document_name).stem.strip(),
        backend=PPStructureV3Backend.name,
        pages=pages,
    )


def _parse_pdf_batch_in_subprocess(
    input_path: Path,
    *,
    document_name: str,
    config: dict[str, Any],
    page_indexes: list[int],
) -> ParsedDocument:
    result = _run_child_process(
        target=_child_parse_pdf_batch,
        args=(str(input_path), document_name, config, page_indexes),
        config=config,
    )
    return _unwrap_child_result(result)


def _run_child_process(
    *,
    target: Any,
    args: tuple[Any, ...],
    config: dict[str, Any],
) -> Any:
    ctx = mp.get_context("spawn")
    with tempfile.TemporaryDirectory(prefix="parser-ppstructurev3-child-") as temp_dir:
        result_path = Path(temp_dir) / "result.pkl"
        proc = ctx.Process(
            target=target,
            args=(*args, str(result_path)),
        )
        proc.start()
        memory_limit_kb = max(1, int(config.get("memory_limit_mb", 14500))) * 1024
        timeout_seconds = max(0.0, float(config.get("subprocess_timeout_seconds", 0.0) or 0.0))
        started_at = time.monotonic()
        try:
            while proc.is_alive():
                if timeout_seconds > 0 and time.monotonic() - started_at >= timeout_seconds:
                    _terminate_child_process(proc)
                    raise dependency_error("ppstructurev3 subprocess timed out")
                rss_kb = _rss_kb(proc.pid)
                if rss_kb is not None and rss_kb >= memory_limit_kb:
                    _terminate_child_process(proc)
                    raise dependency_error("ppstructurev3 parse exceeded memory limit")
                time.sleep(0.25)
            proc.join()
            if not result_path.exists():
                if proc.exitcode == 0:
                    raise dependency_error("ppstructurev3 subprocess returned no result") from None
                raise dependency_error("ppstructurev3 subprocess failed") from None
            try:
                with result_path.open("rb") as handle:
                    return pickle.load(handle)
            except (OSError, pickle.PickleError, EOFError) as exc:
                raise dependency_error("ppstructurev3 subprocess returned invalid result") from exc
        finally:
            if proc.is_alive():
                _terminate_child_process(proc)


def _terminate_child_process(proc: mp.Process) -> None:
    proc.terminate()
    try:
        proc.join(timeout=5)
    finally:
        if proc.is_alive():
            proc.kill()
            proc.join(timeout=5)


def _unwrap_child_result(result: Any) -> ParsedDocument:
    if isinstance(result, _ChildSuccess):
        return result.parsed
    if isinstance(result, _ChildFailure):
        if result.code == "validation_error":
            raise validation_error(result.message, result.fields or {"file": "could not be parsed"})
        raise dependency_error(result.message)
    raise dependency_error("ppstructurev3 subprocess returned invalid result")


def _child_parse_path(
    input_path: str,
    document_name: str,
    content_type: str,
    config: dict[str, Any],
    result_path: str,
) -> None:
    try:
        backend = PPStructureV3Backend(**config)
        parsed = backend._parse_path(
            Path(input_path),
            document_name=document_name,
            content_type=content_type,
        )
        _write_child_result(result_path, _ChildSuccess(parsed))
    except AppError as exc:
        _write_child_result(result_path, _ChildFailure(exc.code, exc.message, exc.fields))
    except Exception:
        _write_child_result(
            result_path,
            _ChildFailure("dependency_error", "ppstructurev3 subprocess failed"),
        )


def _child_parse_pdf_batch(
    input_path: str,
    document_name: str,
    config: dict[str, Any],
    page_indexes: list[int],
    result_path: str,
) -> None:
    try:
        backend = PPStructureV3Backend(**config)
        parsed = backend._parse_pdf_pages(
            Path(input_path),
            document_name=document_name,
            page_indexes=page_indexes,
            allow_empty=True,
        )
        _write_child_result(result_path, _ChildSuccess(parsed))
    except AppError as exc:
        _write_child_result(result_path, _ChildFailure(exc.code, exc.message, exc.fields))
    except Exception:
        _write_child_result(
            result_path,
            _ChildFailure("dependency_error", "ppstructurev3 subprocess failed"),
        )


def _write_child_result(result_path: str, result: _ChildSuccess | _ChildFailure) -> None:
    with Path(result_path).open("wb") as handle:
        pickle.dump(result, handle, protocol=pickle.HIGHEST_PROTOCOL)


def _pdf_page_count(input_path: Path) -> int:
    try:
        import pypdfium2 as pdfium
    except Exception as exc:
        raise dependency_error("pypdfium2 is required for page-level PDF parsing") from exc
    try:
        doc = pdfium.PdfDocument(str(input_path))
    except Exception as exc:
        raise validation_error("document could not be parsed", {"file": "invalid pdf"}) from exc
    try:
        page_count = len(doc)
    finally:
        doc.close()
    if page_count <= 0:
        raise validation_error("document could not be parsed", {"file": "empty pdf"})
    return page_count


def _rss_kb(pid: int | None) -> int | None:
    if not pid:
        return None
    try:
        with Path(f"/proc/{pid}/status").open(encoding="utf-8") as handle:
            for line in handle:
                if line.startswith("VmRSS:"):
                    return int(line.split()[1])
    except (FileNotFoundError, OSError, ValueError):
        return None
    return None


def _close_quietly(value: Any) -> None:
    close = getattr(value, "close", None)
    if callable(close):
        try:
            close()
        except Exception:
            pass


def _is_visual_input(path: Path, content_type: str) -> bool:
    return _is_pdf(path, content_type) or _is_image(path, content_type)


def _is_pdf(path: Path, content_type: str) -> bool:
    normalized = content_type.lower().strip()
    return normalized == "application/pdf" or path.suffix.lower() == ".pdf"


def _is_image(path: Path, content_type: str) -> bool:
    normalized = content_type.lower().strip()
    if normalized.startswith("image/"):
        return True
    return path.suffix.lower() in {".png", ".jpg", ".jpeg", ".bmp", ".tif", ".tiff"}


def _text_layer_status(text: str) -> str:
    normalized = _normalize_text(text)
    if not normalized:
        return "empty"
    total = len(normalized)
    if total < 20:
        return "short"
    ascii_symbols = sum(1 for char in normalized if char.isascii() and not char.isalnum())
    cjk = sum(1 for char in normalized if "\u4e00" <= char <= "\u9fff")
    if cjk / total < 0.1 and ascii_symbols / total > 0.25:
        return "broken"
    if "I¥J" in normalized or "B\"J" in normalized:
        return "broken"
    return "usable"


def _page_retry_warning(page: ParsedPage, threshold: float) -> str:
    if not page.content:
        return "no_text_content"
    if page.ocr_confidence is not None and page.ocr_confidence < threshold:
        return "low_ocr_confidence"
    if _looks_broken_encoding(page.content):
        return "broken_text_encoding"
    return ""


def _looks_broken_encoding(content: str) -> bool:
    normalized = _normalize_text(content)
    return "I¥J" in normalized or "B\"J" in normalized


def _page_quality_score(content: str) -> float:
    normalized = _normalize_text(content)
    if not normalized:
        return 0
    cjk = sum(1 for char in normalized if "\u4e00" <= char <= "\u9fff")
    return len(normalized) + cjk * 2


def _mean(values: list[float]) -> float | None:
    if not values:
        return None
    return sum(values) / len(values)


def _deduplicate(values: list[str]) -> list[str]:
    seen: set[str] = set()
    deduplicated: list[str] = []
    for value in values:
        normalized = _normalize_line(value)
        if normalized and normalized not in seen:
            seen.add(normalized)
            deduplicated.append(normalized)
    return deduplicated


def _normalize_text(value: str) -> str:
    return "\n".join(
        line for line in (_normalize_line(line) for line in value.splitlines()) if line
    )


def _normalize_line(value: str) -> str:
    return " ".join(value.strip().split())
