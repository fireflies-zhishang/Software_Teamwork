from __future__ import annotations

import asyncio
import base64
import binascii
from typing import Protocol

from parser_service.service.errors import (
    AppError,
    dependency_error,
    payload_too_large,
    rate_limited,
    validation_error,
)
from parser_service.service.models import BackendHealth, ParsedDocument, ParsedPage, ParseRequest


class ParserBackend(Protocol):
    name: str

    def health(self) -> BackendHealth:
        pass

    def parse(self, request: ParseRequest) -> ParsedDocument:
        pass

    def warm_up(self) -> None:
        pass


class ParserService:
    def __init__(
        self,
        *,
        backend: ParserBackend,
        max_document_bytes: int,
        max_concurrency: int = 1,
        queue_timeout_seconds: float = 0.0,
        parse_timeout_seconds: float = 120.0,
    ) -> None:
        if max_document_bytes < 1:
            raise ValueError("max_document_bytes must be positive")
        if max_concurrency < 1:
            raise ValueError("max_concurrency must be positive")
        if queue_timeout_seconds < 0:
            raise ValueError("queue_timeout_seconds must be non-negative")
        if parse_timeout_seconds <= 0:
            raise ValueError("parse_timeout_seconds must be positive")
        self._backend = backend
        self._max_document_bytes = max_document_bytes
        self._queue_timeout_seconds = queue_timeout_seconds
        self._parse_timeout_seconds = parse_timeout_seconds
        self._semaphore = asyncio.Semaphore(max_concurrency)

    @property
    def backend_name(self) -> str:
        return self._backend.name

    def health(self) -> BackendHealth:
        return self._backend.health()

    def warm_up(self) -> None:
        self._backend.warm_up()

    async def parse_document(
        self,
        *,
        document_name: str,
        content_type: str,
        size_bytes: int | None,
        data_base64: str,
    ) -> ParsedDocument:
        request = self._request_from_payload(
            document_name=document_name,
            content_type=content_type,
            size_bytes=size_bytes,
            data_base64=data_base64,
        )

        acquired = False
        try:
            if self._queue_timeout_seconds > 0:
                await asyncio.wait_for(
                    self._semaphore.acquire(),
                    timeout=self._queue_timeout_seconds,
                )
            else:
                await self._semaphore.acquire()
            acquired = True
        except TimeoutError as exc:
            raise rate_limited("parser concurrency limit reached") from exc

        parse_task = asyncio.create_task(asyncio.to_thread(self._backend.parse, request))
        try:
            parsed = await asyncio.wait_for(
                asyncio.shield(parse_task),
                timeout=self._parse_timeout_seconds,
            )
        except TimeoutError as exc:
            parse_task.add_done_callback(self._release_after_background_parse)
            acquired = False
            raise dependency_error("parser backend timed out") from exc
        except AppError:
            raise
        except Exception as exc:
            raise dependency_error("parser backend failed") from exc
        finally:
            if acquired:
                self._semaphore.release()

        return self._normalize_parsed(parsed)

    def _release_after_background_parse(self, task: asyncio.Task[ParsedDocument]) -> None:
        try:
            task.exception()
        except asyncio.CancelledError:
            pass
        self._semaphore.release()

    def _request_from_payload(
        self,
        *,
        document_name: str,
        content_type: str,
        size_bytes: int | None,
        data_base64: str,
    ) -> ParseRequest:
        if size_bytes is not None and size_bytes < 0:
            raise validation_error(
                "request validation failed",
                {"sizeBytes": "must be non-negative"},
            )
        if size_bytes is not None and size_bytes > self._max_document_bytes:
            raise payload_too_large("document is too large", {"dataBase64": "exceeds parser limit"})

        encoded = data_base64.strip()
        if not encoded:
            raise validation_error("request validation failed", {"dataBase64": "is required"})
        max_encoded_bytes = ((self._max_document_bytes + 2) // 3) * 4
        if len(encoded) > max_encoded_bytes + 4:
            raise payload_too_large("document is too large", {"dataBase64": "exceeds parser limit"})

        try:
            data = base64.b64decode(encoded, validate=True)
        except (binascii.Error, ValueError) as exc:
            raise validation_error(
                "request validation failed",
                {"dataBase64": "must be valid base64"},
            ) from exc

        if len(data) > self._max_document_bytes:
            raise payload_too_large("document is too large", {"dataBase64": "exceeds parser limit"})
        if len(data) == 0:
            raise validation_error("request validation failed", {"dataBase64": "must not be empty"})
        if size_bytes is not None and size_bytes != len(data):
            raise validation_error(
                "request validation failed",
                {"sizeBytes": "does not match decoded data length"},
            )

        return ParseRequest(
            document_name=document_name.strip(),
            content_type=content_type.strip(),
            size_bytes=size_bytes,
            data=data,
        )

    def _normalize_parsed(self, parsed: ParsedDocument) -> ParsedDocument:
        content = _normalize_text(parsed.content)
        if not content:
            raise validation_error("document could not be parsed", {"file": "no text content"})

        pages: list[ParsedPage] = []
        for page in parsed.pages:
            page_content = _normalize_text(page.content)
            if page.page_number > 0 and page_content:
                pages.append(
                    ParsedPage(
                        page_number=page.page_number,
                        content=page_content,
                        parse_strategy=page.parse_strategy.strip(),
                        text_layer_status=page.text_layer_status.strip(),
                        ocr_confidence=_normalize_confidence(page.ocr_confidence),
                        dpi=page.dpi if page.dpi and page.dpi > 0 else None,
                        warnings=_normalize_warnings(page.warnings),
                    )
                )

        return ParsedDocument(
            content=content,
            title=parsed.title.strip(),
            backend=parsed.backend.strip() or self._backend.name,
            pages=pages,
        )


def _normalize_text(value: str) -> str:
    return "\n".join(
        line for line in (_normalize_line(line) for line in value.splitlines()) if line
    )


def _normalize_line(value: str) -> str:
    return " ".join(value.strip().split())


def _normalize_confidence(value: float | None) -> float | None:
    if value is None:
        return None
    if value < 0:
        return 0.0
    if value > 1:
        return 1.0
    return value


def _normalize_warnings(values: list[str]) -> list[str]:
    warnings: list[str] = []
    for value in values:
        normalized = _normalize_line(value)
        if normalized:
            warnings.append(normalized)
    return warnings
