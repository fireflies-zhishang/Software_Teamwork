import base64

from fastapi.testclient import TestClient

from parser_service.config import Settings
from parser_service.http import create_app
from parser_service.http.app import build_parser_service
from parser_service.service import BackendHealth, ParsedDocument, ParsedPage, ParserService


class FakeBackend:
    name = "fake"

    def __init__(self, *, ready: bool = True, content: str = " parsed text ") -> None:
        self.ready = ready
        self.content = content
        self.requests = []

    def health(self) -> BackendHealth:
        return BackendHealth(
            ready=self.ready,
            status="ready" if self.ready else "not_ready",
            reason="" if self.ready else "backend unavailable",
        )

    def warm_up(self) -> None:
        return None

    def parse(self, request):
        self.requests.append(request)
        return ParsedDocument(
            content=self.content,
            title=" Remote Title ",
            backend=self.name,
            pages=[
                ParsedPage(
                    page_number=1,
                    content=" page one ",
                    parse_strategy="ocr",
                    text_layer_status="broken",
                    ocr_confidence=0.91,
                    dpi=180,
                    warnings=["low_text_quality"],
                )
            ],
        )


def test_healthz_does_not_require_service_token():
    client = _client(service_token="secret")

    response = client.get("/healthz", headers={"X-Request-Id": "req_123"})

    assert response.status_code == 200
    assert response.json() == {
        "data": {"service": "parser", "status": "ok"},
        "requestId": "req_123",
    }


def test_readyz_reports_backend_unavailable():
    client = _client(backend=FakeBackend(ready=False))

    response = client.get("/readyz", headers={"X-Request-Id": "req_123"})

    assert response.status_code == 503
    assert response.json()["data"] == {
        "service": "parser",
        "status": "not_ready",
        "backend": "fake",
        "reason": "backend unavailable",
    }


def test_create_parsed_document_requires_token_when_configured():
    client = _client(service_token="secret")

    response = client.post(
        "/internal/v1/parsed-documents",
        json={"dataBase64": _b64(b"hello")},
        headers={"X-Request-Id": "req_123"},
    )

    assert response.status_code == 401
    assert response.json() == {
        "error": {
            "code": "unauthorized",
            "message": "service token is required",
            "requestId": "req_123",
        }
    }


def test_create_parsed_document_rejects_invalid_token():
    client = _client(service_token="secret")

    response = client.post(
        "/internal/v1/parsed-documents",
        json={"dataBase64": _b64(b"hello")},
        headers={"X-Service-Token": "wrong", "X-Request-Id": "req_123"},
    )

    assert response.status_code == 403
    assert response.json()["error"]["code"] == "forbidden"


def test_create_parsed_document_returns_standard_envelope():
    backend = FakeBackend(content=" line one \n\n line two ")
    client = _client(backend=backend, service_token="secret")

    response = client.post(
        "/internal/v1/parsed-documents",
        json={
            "documentName": "scan.pdf",
            "contentType": "application/pdf",
            "sizeBytes": 5,
            "dataBase64": _b64(b"hello"),
        },
        headers={"X-Service-Token": "secret", "X-Request-Id": "req_123"},
    )

    assert response.status_code == 200
    assert response.json() == {
        "data": {
            "content": "line one\nline two",
            "title": "Remote Title",
            "backend": "fake",
            "pages": [
                {
                    "pageNumber": 1,
                    "content": "page one",
                    "parseStrategy": "ocr",
                    "textLayerStatus": "broken",
                    "ocrConfidence": 0.91,
                    "dpi": 180,
                    "warnings": ["low_text_quality"],
                }
            ],
        },
        "requestId": "req_123",
    }
    assert backend.requests[0].document_name == "scan.pdf"
    assert backend.requests[0].content_type == "application/pdf"
    assert backend.requests[0].data == b"hello"


def test_create_parsed_document_validation_uses_project_error_shape():
    client = _client()

    response = client.post(
        "/internal/v1/parsed-documents",
        json={},
        headers={"X-Request-Id": "req_123"},
    )

    assert response.status_code == 400
    assert response.json() == {
        "error": {
            "code": "validation_error",
            "message": "request validation failed",
            "requestId": "req_123",
            "fields": {"dataBase64": "invalid"},
        }
    }


def test_document_backend_mode_starts_without_paddleocr_and_parses_text():
    service = build_parser_service(Settings(backend="document"))
    app = create_app(settings=Settings(backend="document"), parser_service=service)
    client = TestClient(app)

    ready = client.get("/readyz", headers={"X-Request-Id": "req_ready"})
    assert ready.status_code == 200
    assert ready.json()["data"]["backend"] == "document"

    response = client.post(
        "/internal/v1/parsed-documents",
        json={
            "documentName": "notes.md",
            "contentType": "text/markdown",
            "dataBase64": _b64(b"# Title\n\nbody"),
        },
        headers={"X-Request-Id": "req_123"},
    )

    assert response.status_code == 200
    assert response.json()["data"] == {
        "content": "# Title\nbody",
        "title": "Title",
        "backend": "text",
    }


def test_document_backend_mode_rejects_ocr_formats_without_loading_paddleocr():
    service = build_parser_service(Settings(backend="document"))
    app = create_app(settings=Settings(backend="document"), parser_service=service)
    client = TestClient(app)

    response = client.post(
        "/internal/v1/parsed-documents",
        json={
            "documentName": "scan.pdf",
            "contentType": "application/pdf",
            "dataBase64": _b64(b"%PDF-1.7\n"),
        },
        headers={"X-Request-Id": "req_123"},
    )

    assert response.status_code == 400
    assert response.json() == {
        "error": {
            "code": "validation_error",
            "message": "document format requires OCR backend",
            "requestId": "req_123",
            "fields": {"file": "pdf and image parsing require PARSER_BACKEND=ppstructurev3"},
        }
    }


def test_default_backend_uses_ppstructurev3():
    service = build_parser_service(Settings())

    assert service.backend_name == "ppstructurev3"


def test_ppstructurev3_startup_warmup_runs_when_enabled(monkeypatch):
    warmed = False

    class WarmupBackend:
        name = "ppstructurev3"

        def __init__(self, **kwargs):
            pass

        def health(self):
            return BackendHealth(ready=True, status="ready")

        def warm_up(self):
            nonlocal warmed
            warmed = True

        def parse(self, request):
            raise NotImplementedError

    monkeypatch.setattr("parser_service.http.app.PPStructureV3Backend", WarmupBackend)

    service = build_parser_service(Settings(load_backend_on_startup=True))

    assert service.backend_name == "ppstructurev3"
    assert warmed is True


def test_ppstructurev3_backend_accepts_official_tuning_settings():
    service = build_parser_service(
        Settings(
            ppstructurev3_layout_detection_model_name="PP-DocLayout-S",
            ppstructurev3_text_detection_model_name="PP-OCRv5_mobile_det",
            ppstructurev3_text_recognition_model_name="PP-OCRv5_mobile_rec",
            ppstructurev3_text_det_limit_side_len=768,
            ppstructurev3_text_det_limit_type="max",
            ppstructurev3_text_recognition_batch_size=1,
            ppstructurev3_markdown_ignore_labels=["header", "footer"],
            paddleocr_engine="paddle",
            default_dpi=180,
            retry_dpi=220,
            max_retry_dpi=300,
            low_confidence_threshold=0.8,
            page_batch_size=1,
            subprocess_isolation=True,
            memory_limit_mb=14500,
        )
    )

    ocr_backend = service._backend._ocr_backend
    kwargs = ocr_backend._pipeline_kwargs()

    assert kwargs["layout_detection_model_name"] == "PP-DocLayout-S"
    assert kwargs["text_detection_model_name"] == "PP-OCRv5_mobile_det"
    assert kwargs["text_recognition_model_name"] == "PP-OCRv5_mobile_rec"
    assert kwargs["text_det_limit_side_len"] == 768
    assert kwargs["text_det_limit_type"] == "max"
    assert kwargs["text_recognition_batch_size"] == 1
    assert kwargs["markdown_ignore_labels"] == ["header", "footer"]
    assert "engine" not in kwargs
    assert kwargs["use_doc_orientation_classify"] is True
    assert kwargs["use_doc_unwarping"] is True
    assert kwargs["use_textline_orientation"] is True
    assert kwargs["use_seal_recognition"] is True
    assert kwargs["use_table_recognition"] is True
    assert kwargs["use_formula_recognition"] is True
    assert kwargs["use_chart_recognition"] is True
    assert kwargs["use_region_detection"] is True
    assert ocr_backend._default_dpi == 180
    assert ocr_backend._retry_dpi == 220
    assert ocr_backend._max_retry_dpi == 300
    assert ocr_backend._low_confidence_threshold == 0.8
    assert ocr_backend._page_batch_size == 1
    assert ocr_backend._subprocess_isolation is True
    assert ocr_backend._memory_limit_mb == 14500


def _client(
    *,
    backend: FakeBackend | None = None,
    service_token: str = "",
    max_document_bytes: int = 1024,
) -> TestClient:
    backend = backend or FakeBackend()
    service = ParserService(
        backend=backend,
        max_document_bytes=max_document_bytes,
        parse_timeout_seconds=5,
    )
    app = create_app(
        settings=Settings(service_token=service_token),
        parser_service=service,
    )
    return TestClient(app)


def _b64(value: bytes) -> str:
    return base64.b64encode(value).decode("ascii")
