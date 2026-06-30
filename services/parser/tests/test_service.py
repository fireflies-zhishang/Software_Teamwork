import asyncio
import base64
import time

import pytest

from parser_service.service import BackendHealth, ParsedDocument, ParsedPage, ParserService
from parser_service.service.errors import AppError


class EchoBackend:
    name = "echo"

    def health(self) -> BackendHealth:
        return BackendHealth(ready=True, status="ready")

    def warm_up(self) -> None:
        return None

    def parse(self, request):
        return ParsedDocument(
            content=request.data.decode("utf-8"),
            backend=self.name,
        )


class SlowOnceBackend(EchoBackend):
    def __init__(self) -> None:
        self.calls = 0

    def parse(self, request):
        self.calls += 1
        if self.calls == 1:
            time.sleep(0.2)
        return ParsedDocument(content="done", backend=self.name)


class EmptyBackend(EchoBackend):
    def parse(self, request):
        return ParsedDocument(content="   \n ", backend=self.name)


class MetadataBackend(EchoBackend):
    def parse(self, request):
        return ParsedDocument(
            content=" parsed content ",
            backend=self.name,
            pages=[
                ParsedPage(
                    page_number=1,
                    content=" page one ",
                    parse_strategy=" ocr ",
                    text_layer_status=" broken ",
                    ocr_confidence=1.2,
                    dpi=180,
                    warnings=[" low_text_quality ", ""],
                ),
                ParsedPage(
                    page_number=2,
                    content="empty dpi",
                    ocr_confidence=-0.5,
                    dpi=0,
                ),
                ParsedPage(page_number=0, content="ignored"),
            ],
        )


def test_parse_document_decodes_base64_and_normalizes_text():
    service = ParserService(backend=EchoBackend(), max_document_bytes=128)

    parsed = asyncio.run(
        service.parse_document(
            document_name="notes.txt",
            content_type="text/plain",
            size_bytes=14,
            data_base64=_b64(b" hello \nworld "),
        )
    )

    assert parsed.content == "hello\nworld"
    assert parsed.backend == "echo"


def test_parse_document_rejects_empty_parsed_content():
    service = ParserService(backend=EmptyBackend(), max_document_bytes=128)

    with pytest.raises(AppError) as raised:
        asyncio.run(
            service.parse_document(
                document_name="empty.pdf",
                content_type="application/pdf",
                size_bytes=5,
                data_base64=_b64(b"empty"),
            )
        )

    assert raised.value.status_code == 400
    assert raised.value.code == "validation_error"
    assert raised.value.fields == {"file": "no text content"}


def test_parse_document_normalizes_page_metadata():
    service = ParserService(backend=MetadataBackend(), max_document_bytes=128)

    parsed = asyncio.run(
        service.parse_document(
            document_name="scan.pdf",
            content_type="application/pdf",
            size_bytes=5,
            data_base64=_b64(b"hello"),
        )
    )

    assert parsed.content == "parsed content"
    assert len(parsed.pages) == 2
    assert parsed.pages[0] == ParsedPage(
        page_number=1,
        content="page one",
        parse_strategy="ocr",
        text_layer_status="broken",
        ocr_confidence=1.0,
        dpi=180,
        warnings=["low_text_quality"],
    )
    assert parsed.pages[1] == ParsedPage(
        page_number=2,
        content="empty dpi",
        ocr_confidence=0.0,
    )


def test_parse_document_rejects_invalid_base64():
    service = ParserService(backend=EchoBackend(), max_document_bytes=128)

    with pytest.raises(AppError) as raised:
        asyncio.run(
            service.parse_document(
                document_name="scan.pdf",
                content_type="application/pdf",
                size_bytes=None,
                data_base64="not base64",
            )
        )

    assert raised.value.status_code == 400
    assert raised.value.code == "validation_error"
    assert raised.value.fields == {"dataBase64": "must be valid base64"}


def test_parse_document_rejects_oversized_payload():
    service = ParserService(backend=EchoBackend(), max_document_bytes=4)

    with pytest.raises(AppError) as raised:
        asyncio.run(
            service.parse_document(
                document_name="scan.pdf",
                content_type="application/pdf",
                size_bytes=5,
                data_base64=_b64(b"hello"),
            )
        )

    assert raised.value.status_code == 413
    assert raised.value.code == "validation_error"


def test_parse_document_rejects_size_mismatch():
    service = ParserService(backend=EchoBackend(), max_document_bytes=128)

    with pytest.raises(AppError) as raised:
        asyncio.run(
            service.parse_document(
                document_name="scan.pdf",
                content_type="application/pdf",
                size_bytes=99,
                data_base64=_b64(b"hello"),
            )
        )

    assert raised.value.status_code == 400
    assert raised.value.fields == {"sizeBytes": "does not match decoded data length"}


def test_parse_timeout_keeps_concurrency_slot_until_backend_finishes():
    async def run() -> None:
        service = ParserService(
            backend=SlowOnceBackend(),
            max_document_bytes=128,
            max_concurrency=1,
            queue_timeout_seconds=0.01,
            parse_timeout_seconds=0.01,
        )
        with pytest.raises(AppError) as timeout_error:
            await service.parse_document(
                document_name="scan.pdf",
                content_type="application/pdf",
                size_bytes=5,
                data_base64=_b64(b"hello"),
            )
        assert timeout_error.value.status_code == 502

        with pytest.raises(AppError) as limit_error:
            await service.parse_document(
                document_name="scan.pdf",
                content_type="application/pdf",
                size_bytes=5,
                data_base64=_b64(b"hello"),
            )
        assert limit_error.value.status_code == 429

        await asyncio.sleep(0.25)
        parsed = await service.parse_document(
            document_name="scan.pdf",
            content_type="application/pdf",
            size_bytes=5,
            data_base64=_b64(b"hello"),
        )
        assert parsed.content == "done"

    asyncio.run(run())


def _b64(value: bytes) -> str:
    return base64.b64encode(value).decode("ascii")
