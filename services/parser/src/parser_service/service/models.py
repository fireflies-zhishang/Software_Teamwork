from __future__ import annotations

from dataclasses import dataclass, field


@dataclass(frozen=True)
class ParseRequest:
    document_name: str
    content_type: str
    size_bytes: int | None
    data: bytes


@dataclass(frozen=True)
class ParsedPage:
    page_number: int
    content: str
    parse_strategy: str = ""
    text_layer_status: str = ""
    ocr_confidence: float | None = None
    dpi: int | None = None
    warnings: list[str] = field(default_factory=list)


@dataclass(frozen=True)
class ParsedDocument:
    content: str
    backend: str
    title: str = ""
    pages: list[ParsedPage] = field(default_factory=list)


@dataclass(frozen=True)
class BackendHealth:
    ready: bool
    status: str
    reason: str = ""
