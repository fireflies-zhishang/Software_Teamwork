from __future__ import annotations

import hmac
import logging
import uuid
from typing import Annotated

from fastapi import Depends, FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse

from parser_service.backends.document import DisabledOCRBackend, DocumentParserBackend
from parser_service.backends.paddleocr import PaddleOCRBackend
from parser_service.backends.ppstructurev3 import PPStructureV3Backend
from parser_service.config import Settings
from parser_service.http.schemas import CreateParsedDocumentRequest
from parser_service.service import AppError, ParsedDocument, ParserService

logger = logging.getLogger(__name__)


def create_app(
    *,
    settings: Settings | None = None,
    parser_service: ParserService | None = None,
) -> FastAPI:
    settings = settings or Settings.from_env()
    parser_service = parser_service or build_parser_service(settings)

    app = FastAPI(
        title="Parser Runtime Internal API",
        version="0.1.0",
        docs_url=None,
        redoc_url=None,
        openapi_url=None,
    )
    app.state.settings = settings
    app.state.parser_service = parser_service

    @app.middleware("http")
    async def request_id_middleware(request: Request, call_next):
        request.state.request_id = _request_id(request)
        return await call_next(request)

    @app.exception_handler(AppError)
    async def app_error_handler(request: Request, exc: AppError) -> JSONResponse:
        return _error_response(request, exc.status_code, exc.code, exc.message, exc.fields)

    @app.exception_handler(RequestValidationError)
    async def request_validation_handler(
        request: Request,
        exc: RequestValidationError,
    ) -> JSONResponse:
        fields: dict[str, str] = {}
        for item in exc.errors():
            loc = ".".join(str(part) for part in item.get("loc", []) if part != "body")
            fields[loc or "body"] = "invalid"
        return _error_response(
            request,
            400,
            "validation_error",
            "request validation failed",
            fields or None,
        )

    @app.exception_handler(Exception)
    async def unexpected_error_handler(request: Request, exc: Exception) -> JSONResponse:
        logger.exception(
            "parser request failed",
            extra={
                "service": settings.service_name,
                "request_id": getattr(request.state, "request_id", ""),
                "operation": request.url.path,
                "status": "failed",
            },
        )
        return _error_response(request, 500, "internal_error", "internal server error")

    @app.get("/healthz")
    async def healthz(request: Request) -> JSONResponse:
        return _success_response(
            request,
            {
                "service": settings.service_name,
                "status": "ok",
            },
        )

    @app.get("/readyz")
    async def readyz(request: Request) -> JSONResponse:
        health = parser_service.health()
        data = {
            "service": settings.service_name,
            "status": health.status,
            "backend": parser_service.backend_name,
        }
        if health.reason:
            data["reason"] = health.reason
        return _success_response(request, data, status_code=200 if health.ready else 503)

    @app.post("/internal/v1/parsed-documents")
    async def create_parsed_document(
        request: Request,
        payload: CreateParsedDocumentRequest,
        _: Annotated[None, Depends(require_service_token)],
    ) -> JSONResponse:
        parsed = await parser_service.parse_document(
            document_name=payload.document_name or "",
            content_type=payload.content_type or "",
            size_bytes=payload.size_bytes,
            data_base64=payload.data_base64,
        )
        return _success_response(request, _parsed_document_data(parsed))

    return app


def build_parser_service(settings: Settings) -> ParserService:
    backend_name = settings.backend.strip().lower()
    if backend_name == "document":
        backend = DocumentParserBackend(ocr_backend=DisabledOCRBackend(), name="document")
    elif backend_name == "paddleocr":
        ocr_backend = PaddleOCRBackend(
            lang=settings.paddleocr_lang,
            device=settings.paddleocr_device,
            engine=settings.paddleocr_engine,
            paddlex_config=settings.paddleocr_config_path,
            use_doc_orientation_classify=settings.paddleocr_use_doc_orientation_classify,
            use_doc_unwarping=settings.paddleocr_use_doc_unwarping,
            use_textline_orientation=settings.paddleocr_use_textline_orientation,
            enable_mkldnn=settings.paddleocr_enable_mkldnn,
        )
        backend = DocumentParserBackend(ocr_backend=ocr_backend, name="paddleocr")
    elif backend_name == "ppstructurev3":
        structure_backend = PPStructureV3Backend(
            lang=settings.paddleocr_lang,
            device=settings.paddleocr_device,
            engine="",
            paddlex_config=settings.paddleocr_config_path,
            use_doc_orientation_classify=settings.paddleocr_use_doc_orientation_classify,
            use_doc_unwarping=settings.paddleocr_use_doc_unwarping,
            use_textline_orientation=settings.paddleocr_use_textline_orientation,
            use_seal_recognition=settings.ppstructurev3_use_seal_recognition,
            use_table_recognition=settings.ppstructurev3_use_table_recognition,
            use_formula_recognition=settings.ppstructurev3_use_formula_recognition,
            use_chart_recognition=settings.ppstructurev3_use_chart_recognition,
            use_region_detection=settings.ppstructurev3_use_region_detection,
            format_block_content=settings.ppstructurev3_format_block_content,
            enable_mkldnn=settings.paddleocr_enable_mkldnn,
            layout_detection_model_name=settings.ppstructurev3_layout_detection_model_name,
            text_detection_model_name=settings.ppstructurev3_text_detection_model_name,
            text_recognition_model_name=settings.ppstructurev3_text_recognition_model_name,
            text_det_limit_side_len=settings.ppstructurev3_text_det_limit_side_len,
            text_det_limit_type=settings.ppstructurev3_text_det_limit_type,
            text_recognition_batch_size=settings.ppstructurev3_text_recognition_batch_size,
            textline_orientation_batch_size=settings.ppstructurev3_textline_orientation_batch_size,
            seal_text_recognition_batch_size=(
                settings.ppstructurev3_seal_text_recognition_batch_size
            ),
            formula_recognition_batch_size=settings.ppstructurev3_formula_recognition_batch_size,
            chart_recognition_batch_size=settings.ppstructurev3_chart_recognition_batch_size,
            markdown_ignore_labels=settings.ppstructurev3_markdown_ignore_labels,
            profile=settings.profile,
            default_dpi=settings.default_dpi,
            retry_dpi=settings.retry_dpi,
            max_retry_dpi=settings.max_retry_dpi,
            low_confidence_threshold=settings.low_confidence_threshold,
            page_batch_size=settings.page_batch_size,
            subprocess_isolation=settings.subprocess_isolation,
            subprocess_timeout_seconds=max(0.1, settings.parse_timeout_seconds - 1.0),
            memory_limit_mb=settings.memory_limit_mb,
        )
        backend = DocumentParserBackend(ocr_backend=structure_backend, name="ppstructurev3")
    else:
        raise ValueError("PARSER_BACKEND must be document, paddleocr, or ppstructurev3")
    service = ParserService(
        backend=backend,
        max_document_bytes=settings.max_document_bytes,
        max_concurrency=settings.max_concurrency,
        queue_timeout_seconds=settings.queue_timeout_seconds,
        parse_timeout_seconds=settings.parse_timeout_seconds,
    )
    if settings.load_backend_on_startup:
        service.warm_up()
    return service


def require_service_token(request: Request) -> None:
    settings: Settings = request.app.state.settings
    expected = settings.service_token
    if not expected:
        return
    supplied = request.headers.get("X-Service-Token", "").strip()
    if not supplied:
        raise AppError(code="unauthorized", message="service token is required", status_code=401)
    if not hmac.compare_digest(supplied, expected):
        raise AppError(code="forbidden", message="service token is invalid", status_code=403)


def _success_response(request: Request, data: object, status_code: int = 200) -> JSONResponse:
    return JSONResponse(
        status_code=status_code,
        content={
            "data": data,
            "requestId": request.state.request_id,
        },
    )


def _error_response(
    request: Request,
    status_code: int,
    code: str,
    message: str,
    fields: dict[str, str] | None = None,
) -> JSONResponse:
    error: dict[str, object] = {
        "code": code,
        "message": message,
        "requestId": getattr(request.state, "request_id", _request_id(request)),
    }
    if fields:
        error["fields"] = fields
    return JSONResponse(status_code=status_code, content={"error": error})


def _request_id(request: Request) -> str:
    supplied = request.headers.get("X-Request-Id", "").strip()
    if supplied:
        return supplied
    return "req_" + uuid.uuid4().hex[:16]


def _parsed_document_data(parsed: ParsedDocument) -> dict[str, object]:
    data: dict[str, object] = {
        "content": parsed.content,
        "title": parsed.title or None,
        "backend": parsed.backend,
    }
    if parsed.pages:
        pages: list[dict[str, object]] = []
        for page in parsed.pages:
            page_data: dict[str, object] = {
                "pageNumber": page.page_number,
                "content": page.content,
            }
            if page.parse_strategy:
                page_data["parseStrategy"] = page.parse_strategy
            if page.text_layer_status:
                page_data["textLayerStatus"] = page.text_layer_status
            if page.ocr_confidence is not None:
                page_data["ocrConfidence"] = page.ocr_confidence
            if page.dpi is not None:
                page_data["dpi"] = page.dpi
            if page.warnings:
                page_data["warnings"] = page.warnings
            pages.append(page_data)
        data["pages"] = pages
    return data
