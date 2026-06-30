from __future__ import annotations

import os
from dataclasses import dataclass


@dataclass(frozen=True)
class Settings:
    service_name: str = "parser"
    host: str = "0.0.0.0"
    port: int = 8080
    service_token: str = ""
    backend: str = "ppstructurev3"
    profile: str = "accurate"
    max_document_bytes: int = 8 * 1024 * 1024
    max_concurrency: int = 1
    queue_timeout_seconds: float = 0.0
    parse_timeout_seconds: float = 120.0
    load_backend_on_startup: bool = False
    default_dpi: int = 180
    retry_dpi: int = 220
    max_retry_dpi: int = 300
    low_confidence_threshold: float = 0.85
    page_batch_size: int = 1
    subprocess_isolation: bool = True
    memory_limit_mb: int = 14500
    paddleocr_lang: str = "ch"
    paddleocr_device: str = "cpu"
    paddleocr_engine: str = ""
    paddleocr_config_path: str = ""
    paddleocr_use_doc_orientation_classify: bool = True
    paddleocr_use_doc_unwarping: bool = True
    paddleocr_use_textline_orientation: bool = True
    paddleocr_enable_mkldnn: bool = False
    ppstructurev3_use_seal_recognition: bool = True
    ppstructurev3_use_table_recognition: bool = True
    ppstructurev3_use_formula_recognition: bool = True
    ppstructurev3_use_chart_recognition: bool = True
    ppstructurev3_use_region_detection: bool = True
    ppstructurev3_format_block_content: bool = True
    ppstructurev3_layout_detection_model_name: str = ""
    ppstructurev3_text_detection_model_name: str = ""
    ppstructurev3_text_recognition_model_name: str = ""
    ppstructurev3_text_det_limit_side_len: int | None = None
    ppstructurev3_text_det_limit_type: str = ""
    ppstructurev3_text_recognition_batch_size: int | None = None
    ppstructurev3_textline_orientation_batch_size: int | None = None
    ppstructurev3_seal_text_recognition_batch_size: int | None = None
    ppstructurev3_formula_recognition_batch_size: int | None = None
    ppstructurev3_chart_recognition_batch_size: int | None = None
    ppstructurev3_markdown_ignore_labels: list[str] | None = None

    @classmethod
    def from_env(cls) -> Settings:
        return cls(
            host=_string("PARSER_HOST", cls.host),
            port=_int("PARSER_PORT", cls.port, minimum=1, maximum=65535),
            service_token=_string("PARSER_SERVICE_TOKEN", cls.service_token),
            backend=_string("PARSER_BACKEND", cls.backend),
            profile=_choice("PARSER_PROFILE", cls.profile, choices={"accurate", "balanced"}),
            max_document_bytes=_int(
                "PARSER_MAX_DOCUMENT_BYTES",
                cls.max_document_bytes,
                minimum=1,
            ),
            max_concurrency=_int("PARSER_MAX_CONCURRENCY", cls.max_concurrency, minimum=1),
            queue_timeout_seconds=_float(
                "PARSER_QUEUE_TIMEOUT_SECONDS",
                cls.queue_timeout_seconds,
                minimum=0,
            ),
            parse_timeout_seconds=_float(
                "PARSER_PARSE_TIMEOUT_SECONDS",
                cls.parse_timeout_seconds,
                minimum=1,
            ),
            load_backend_on_startup=_bool(
                "PARSER_LOAD_BACKEND_ON_STARTUP",
                cls.load_backend_on_startup,
            ),
            default_dpi=_int("PARSER_DEFAULT_DPI", cls.default_dpi, minimum=72, maximum=600),
            retry_dpi=_int("PARSER_RETRY_DPI", cls.retry_dpi, minimum=72, maximum=600),
            max_retry_dpi=_int("PARSER_MAX_RETRY_DPI", cls.max_retry_dpi, minimum=72, maximum=600),
            low_confidence_threshold=_float(
                "PARSER_LOW_CONFIDENCE_THRESHOLD",
                cls.low_confidence_threshold,
                minimum=0,
                maximum=1,
            ),
            page_batch_size=_int("PARSER_PAGE_BATCH_SIZE", cls.page_batch_size, minimum=1),
            subprocess_isolation=_bool("PARSER_SUBPROCESS_ISOLATION", cls.subprocess_isolation),
            memory_limit_mb=_int("PARSER_MEMORY_LIMIT_MB", cls.memory_limit_mb, minimum=512),
            paddleocr_lang=_string("PADDLEOCR_LANG", cls.paddleocr_lang),
            paddleocr_device=_string("PADDLEOCR_DEVICE", cls.paddleocr_device),
            paddleocr_engine=_string("PADDLEOCR_ENGINE", cls.paddleocr_engine),
            paddleocr_config_path=_string("PADDLEOCR_CONFIG_PATH", cls.paddleocr_config_path),
            paddleocr_use_doc_orientation_classify=_bool(
                "PADDLEOCR_USE_DOC_ORIENTATION_CLASSIFY",
                cls.paddleocr_use_doc_orientation_classify,
            ),
            paddleocr_use_doc_unwarping=_bool(
                "PADDLEOCR_USE_DOC_UNWARPING",
                cls.paddleocr_use_doc_unwarping,
            ),
            paddleocr_use_textline_orientation=_bool(
                "PADDLEOCR_USE_TEXTLINE_ORIENTATION",
                cls.paddleocr_use_textline_orientation,
            ),
            paddleocr_enable_mkldnn=_bool(
                "PADDLEOCR_ENABLE_MKLDNN",
                cls.paddleocr_enable_mkldnn,
            ),
            ppstructurev3_use_seal_recognition=_bool(
                "PPSTRUCTUREV3_USE_SEAL_RECOGNITION",
                cls.ppstructurev3_use_seal_recognition,
            ),
            ppstructurev3_use_table_recognition=_bool(
                "PPSTRUCTUREV3_USE_TABLE_RECOGNITION",
                cls.ppstructurev3_use_table_recognition,
            ),
            ppstructurev3_use_formula_recognition=_bool(
                "PPSTRUCTUREV3_USE_FORMULA_RECOGNITION",
                cls.ppstructurev3_use_formula_recognition,
            ),
            ppstructurev3_use_chart_recognition=_bool(
                "PPSTRUCTUREV3_USE_CHART_RECOGNITION",
                cls.ppstructurev3_use_chart_recognition,
            ),
            ppstructurev3_use_region_detection=_bool(
                "PPSTRUCTUREV3_USE_REGION_DETECTION",
                cls.ppstructurev3_use_region_detection,
            ),
            ppstructurev3_format_block_content=_bool(
                "PPSTRUCTUREV3_FORMAT_BLOCK_CONTENT",
                cls.ppstructurev3_format_block_content,
            ),
            ppstructurev3_layout_detection_model_name=_string(
                "PPSTRUCTUREV3_LAYOUT_DETECTION_MODEL_NAME",
                cls.ppstructurev3_layout_detection_model_name,
            ),
            ppstructurev3_text_detection_model_name=_string(
                "PPSTRUCTUREV3_TEXT_DETECTION_MODEL_NAME",
                cls.ppstructurev3_text_detection_model_name,
            ),
            ppstructurev3_text_recognition_model_name=_string(
                "PPSTRUCTUREV3_TEXT_RECOGNITION_MODEL_NAME",
                cls.ppstructurev3_text_recognition_model_name,
            ),
            ppstructurev3_text_det_limit_side_len=_optional_int(
                "PPSTRUCTUREV3_TEXT_DET_LIMIT_SIDE_LEN",
                cls.ppstructurev3_text_det_limit_side_len,
                minimum=1,
            ),
            ppstructurev3_text_det_limit_type=_choice(
                "PPSTRUCTUREV3_TEXT_DET_LIMIT_TYPE",
                cls.ppstructurev3_text_det_limit_type,
                choices={"", "min", "max"},
            ),
            ppstructurev3_text_recognition_batch_size=_optional_int(
                "PPSTRUCTUREV3_TEXT_RECOGNITION_BATCH_SIZE",
                cls.ppstructurev3_text_recognition_batch_size,
                minimum=1,
            ),
            ppstructurev3_textline_orientation_batch_size=_optional_int(
                "PPSTRUCTUREV3_TEXTLINE_ORIENTATION_BATCH_SIZE",
                cls.ppstructurev3_textline_orientation_batch_size,
                minimum=1,
            ),
            ppstructurev3_seal_text_recognition_batch_size=_optional_int(
                "PPSTRUCTUREV3_SEAL_TEXT_RECOGNITION_BATCH_SIZE",
                cls.ppstructurev3_seal_text_recognition_batch_size,
                minimum=1,
            ),
            ppstructurev3_formula_recognition_batch_size=_optional_int(
                "PPSTRUCTUREV3_FORMULA_RECOGNITION_BATCH_SIZE",
                cls.ppstructurev3_formula_recognition_batch_size,
                minimum=1,
            ),
            ppstructurev3_chart_recognition_batch_size=_optional_int(
                "PPSTRUCTUREV3_CHART_RECOGNITION_BATCH_SIZE",
                cls.ppstructurev3_chart_recognition_batch_size,
                minimum=1,
            ),
            ppstructurev3_markdown_ignore_labels=_string_list(
                "PPSTRUCTUREV3_MARKDOWN_IGNORE_LABELS",
                cls.ppstructurev3_markdown_ignore_labels,
            ),
        )


def _string(name: str, default: str) -> str:
    return os.environ.get(name, default).strip()


def _int(name: str, default: int, *, minimum: int, maximum: int | None = None) -> int:
    raw = os.environ.get(name, "").strip()
    if raw == "":
        return default
    try:
        value = int(raw)
    except ValueError as exc:
        raise ValueError(f"{name} must be an integer") from exc
    if value < minimum:
        raise ValueError(f"{name} must be >= {minimum}")
    if maximum is not None and value > maximum:
        raise ValueError(f"{name} must be <= {maximum}")
    return value


def _optional_int(name: str, default: int | None, *, minimum: int) -> int | None:
    raw = os.environ.get(name, "").strip()
    if raw == "":
        return default
    try:
        value = int(raw)
    except ValueError as exc:
        raise ValueError(f"{name} must be an integer") from exc
    if value < minimum:
        raise ValueError(f"{name} must be >= {minimum}")
    return value


def _choice(name: str, default: str, *, choices: set[str]) -> str:
    value = _string(name, default)
    if value not in choices:
        allowed = ", ".join(sorted(item or "<empty>" for item in choices))
        raise ValueError(f"{name} must be one of: {allowed}")
    return value


def _string_list(name: str, default: list[str] | None) -> list[str] | None:
    raw = os.environ.get(name, "").strip()
    if raw == "":
        return default
    values = [item.strip() for item in raw.split(",") if item.strip()]
    return values or None


def _float(
    name: str,
    default: float,
    *,
    minimum: float,
    maximum: float | None = None,
) -> float:
    raw = os.environ.get(name, "").strip()
    if raw == "":
        return default
    try:
        value = float(raw)
    except ValueError as exc:
        raise ValueError(f"{name} must be a number") from exc
    if value < minimum:
        raise ValueError(f"{name} must be >= {minimum}")
    if maximum is not None and value > maximum:
        raise ValueError(f"{name} must be <= {maximum}")
    return value


def _bool(name: str, default: bool) -> bool:
    raw = os.environ.get(name, "").strip().lower()
    if raw == "":
        return default
    if raw in {"1", "true", "yes", "on"}:
        return True
    if raw in {"0", "false", "no", "off"}:
        return False
    raise ValueError(f"{name} must be a boolean")
