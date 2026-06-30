from __future__ import annotations

import io
import mimetypes
import posixpath
import re
import xml.etree.ElementTree as ET
import zipfile
from collections.abc import Iterable
from pathlib import Path

from parser_service.service import (
    BackendHealth,
    ParsedDocument,
    ParserBackend,
    ParseRequest,
    validation_error,
)

DOCX_MEDIA_TYPE = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
PPTX_MEDIA_TYPE = "application/vnd.openxmlformats-officedocument.presentationml.presentation"
XLSX_MEDIA_TYPE = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

MAX_XML_ENTRY_BYTES = 8 * 1024 * 1024
MAX_XML_TOTAL_BYTES = 16 * 1024 * 1024


class DocumentParserBackend:
    def __init__(self, *, ocr_backend: ParserBackend, name: str = "") -> None:
        self.name = name.strip() or ocr_backend.name
        self._ocr_backend = ocr_backend

    def health(self) -> BackendHealth:
        return self._ocr_backend.health()

    def warm_up(self) -> None:
        self._ocr_backend.warm_up()

    def parse(self, request: ParseRequest) -> ParsedDocument:
        detected = detect_format(request)
        if detected == "text":
            return parse_text(request)
        if detected == "docx":
            return parse_docx(request)
        if detected == "pptx":
            return parse_pptx(request)
        if detected == "xlsx":
            return parse_xlsx(request)
        if detected in {"pdf", "image"}:
            return self._ocr_backend.parse(request)
        if detected == "legacy_office":
            raise validation_error(
                "document format is not supported",
                {"file": "legacy Office documents are not supported"},
            )
        raise validation_error("document format is not supported", {"file": "unsupported format"})


class DisabledOCRBackend:
    name = "document"

    def health(self) -> BackendHealth:
        return BackendHealth(ready=True, status="ready")

    def warm_up(self) -> None:
        return None

    def parse(self, request: ParseRequest) -> ParsedDocument:
        raise validation_error(
            "document format requires OCR backend",
            {"file": "pdf and image parsing require PARSER_BACKEND=ppstructurev3"},
        )


def detect_format(request: ParseRequest) -> str:
    media_type = request.content_type.split(";", 1)[0].strip().lower()
    extension = Path(request.document_name).suffix.lower()
    data = request.data

    if _looks_like_zip(data):
        with _open_zip(data) as archive:
            names = set(archive.namelist())
            if "word/document.xml" in names:
                return "docx"
            if "ppt/presentation.xml" in names or any(
                name.startswith("ppt/slides/") for name in names
            ):
                return "pptx"
            if "xl/workbook.xml" in names or any(
                name.startswith("xl/worksheets/") for name in names
            ):
                return "xlsx"
        return "unknown"

    if data.startswith(b"%PDF-"):
        return "pdf"
    if _has_image_magic(data):
        return "image"
    if _has_legacy_office_magic(data):
        return "legacy_office"
    if media_type in {DOCX_MEDIA_TYPE, PPTX_MEDIA_TYPE, XLSX_MEDIA_TYPE} or extension in {
        ".docx",
        ".pptx",
        ".xlsx",
    }:
        return "unknown"
    if media_type == "application/pdf" or extension == ".pdf":
        return "pdf"
    if media_type.startswith("image/") or extension in {
        ".png",
        ".jpg",
        ".jpeg",
        ".gif",
        ".bmp",
        ".tif",
        ".tiff",
        ".webp",
    }:
        return "image"
    if media_type in {
        "application/msword",
        "application/vnd.ms-excel",
        "application/vnd.ms-powerpoint",
    }:
        return "legacy_office"
    if extension in {".doc", ".xls", ".ppt"}:
        return "legacy_office"
    if media_type in {
        "text/plain",
        "text/markdown",
        "application/markdown",
        "application/x-markdown",
    }:
        return "text"
    if extension in {".txt", ".md", ".markdown"}:
        return "text"
    if _looks_like_utf8_text(data):
        return "text"
    guessed_type, _ = mimetypes.guess_type(request.document_name)
    if guessed_type and guessed_type.startswith("text/"):
        return "text"
    return "unknown"


def parse_text(request: ParseRequest) -> ParsedDocument:
    try:
        content = request.data.decode("utf-8")
    except UnicodeDecodeError as exc:
        raise validation_error(
            "document text encoding is not supported",
            {"file": "must be utf-8 text"},
        ) from exc
    content = _normalize_text(content)
    if not content:
        raise validation_error("document is empty", {"file": "no text content"})
    return ParsedDocument(
        content=content,
        title=_first_non_empty_line(content).removeprefix("# ").strip(),
        backend="text",
    )


def parse_docx(request: ParseRequest) -> ParsedDocument:
    with _open_zip(request.data) as archive:
        xml = _read_zip_text(archive, "word/document.xml")
    paragraphs = _paragraph_texts(xml)
    content = _normalize_text("\n\n".join(paragraphs))
    if not content:
        raise validation_error("document is empty", {"file": "no text content"})
    return ParsedDocument(content=content, title=_first_non_empty_line(content), backend="docx")


def parse_pptx(request: ParseRequest) -> ParsedDocument:
    with _open_zip(request.data) as archive:
        slide_names = _sorted_numbered_names(
            name
            for name in archive.namelist()
            if name.startswith("ppt/slides/") and name.endswith(".xml")
        )
        if not slide_names:
            raise validation_error("presentation has no slides", {"file": "missing slides"})
        sections: list[str] = []
        for index, slide_name in enumerate(slide_names, start=1):
            paragraphs = _paragraph_texts(_read_zip_text(archive, slide_name))
            slide_text = _normalize_text("\n".join(paragraphs))
            if slide_text:
                sections.append(f"Slide {index}\n{slide_text}")
    content = _normalize_text("\n\n".join(sections))
    if not content:
        raise validation_error("document is empty", {"file": "no text content"})
    return ParsedDocument(content=content, title=_first_presentation_title(content), backend="pptx")


def parse_xlsx(request: ParseRequest) -> ParsedDocument:
    with _open_zip(request.data) as archive:
        shared_strings = _shared_strings(archive)
        sheet_names = _sorted_numbered_names(
            name
            for name in archive.namelist()
            if name.startswith("xl/worksheets/") and name.endswith(".xml")
        )
        if not sheet_names:
            raise validation_error("spreadsheet has no worksheets", {"file": "missing worksheets"})
        sections: list[str] = []
        for index, sheet_name in enumerate(sheet_names, start=1):
            rows = _worksheet_rows(_read_zip_text(archive, sheet_name), shared_strings)
            if rows:
                sections.append("\n".join([f"Sheet {index}", *rows]))
    content = _normalize_text("\n\n".join(sections))
    if not content:
        raise validation_error("document is empty", {"file": "no text content"})
    return ParsedDocument(content=content, title=_first_non_empty_line(content), backend="xlsx")


def _open_zip(data: bytes) -> zipfile.ZipFile:
    try:
        return zipfile.ZipFile(io.BytesIO(data))
    except zipfile.BadZipFile as exc:
        raise validation_error(
            "document archive could not be read",
            {"file": "invalid archive"},
        ) from exc


def _read_zip_text(archive: zipfile.ZipFile, name: str) -> str:
    try:
        info = archive.getinfo(name)
    except KeyError as exc:
        raise validation_error(
            "document archive is missing required content",
            {"file": name},
        ) from exc
    if info.file_size > MAX_XML_ENTRY_BYTES:
        raise validation_error("document archive entry is too large", {"file": name})
    total_size = sum(
        item.file_size for item in archive.infolist() if item.filename.endswith(".xml")
    )
    if total_size > MAX_XML_TOTAL_BYTES:
        raise validation_error("document archive expanded content is too large", {"file": "xml"})
    with archive.open(info) as file:
        data = file.read(MAX_XML_ENTRY_BYTES + 1)
    if len(data) > MAX_XML_ENTRY_BYTES:
        raise validation_error("document archive entry is too large", {"file": name})
    try:
        return data.decode("utf-8")
    except UnicodeDecodeError as exc:
        raise validation_error(
            "document archive text encoding is not supported",
            {"file": name},
        ) from exc


def _paragraph_texts(xml: str) -> list[str]:
    root = _parse_xml(xml)
    paragraphs: list[str] = []
    for paragraph in _elements_by_local_name(root.iter(), "p"):
        text = "".join(node.text or "" for node in _elements_by_local_name(paragraph.iter(), "t"))
        text = _normalize_line(text)
        if text:
            paragraphs.append(text)
    if paragraphs:
        return paragraphs
    text = " ".join(node.text or "" for node in _elements_by_local_name(root.iter(), "t"))
    text = _normalize_line(text)
    return [text] if text else []


def _shared_strings(archive: zipfile.ZipFile) -> list[str]:
    if "xl/sharedStrings.xml" not in archive.namelist():
        return []
    xml = _read_zip_text(archive, "xl/sharedStrings.xml")
    root = _parse_xml(xml)
    values: list[str] = []
    for item in _elements_by_local_name(root.iter(), "si"):
        text = " ".join(node.text or "" for node in _elements_by_local_name(item.iter(), "t"))
        values.append(_normalize_line(text))
    return values


def _worksheet_rows(xml: str, shared_strings: list[str]) -> list[str]:
    root = _parse_xml(xml)
    rows: list[str] = []
    for row in _elements_by_local_name(root.iter(), "row"):
        cells: list[str] = []
        for cell in _elements_by_local_name(row.iter(), "c"):
            value = _cell_value(cell, shared_strings)
            if value:
                cells.append(value)
        if cells:
            rows.append("\t".join(cells))
    return rows


def _cell_value(cell: ET.Element, shared_strings: list[str]) -> str:
    cell_type = cell.attrib.get("t", "")
    value_element = next(_elements_by_local_name(cell.iter(), "v"), None)
    if value_element is None or value_element.text is None:
        text = " ".join(node.text or "" for node in _elements_by_local_name(cell.iter(), "t"))
        return _normalize_line(text)
    raw = value_element.text.strip()
    if cell_type == "s":
        try:
            return shared_strings[int(raw)]
        except (ValueError, IndexError):
            return ""
    return raw


def _elements_by_local_name(
    elements: Iterable[ET.Element],
    local_name: str,
) -> Iterable[ET.Element]:
    for element in elements:
        if _local_name(element.tag) == local_name:
            yield element


def _local_name(tag: str) -> str:
    if "}" in tag:
        return tag.rsplit("}", 1)[1]
    return tag


def _parse_xml(xml: str) -> ET.Element:
    try:
        return ET.fromstring(xml)
    except ET.ParseError as exc:
        raise validation_error("document archive XML could not be parsed", {"file": "xml"}) from exc


def _sorted_numbered_names(names: Iterable[str]) -> list[str]:
    return sorted(names, key=lambda name: (_trailing_number(name), name))


def _trailing_number(name: str) -> int:
    match = re.search(r"(\d+)(?=\.[^.]+$)", posixpath.basename(name))
    if not match:
        return 0
    return int(match.group(1))


def _first_presentation_title(content: str) -> str:
    for line in content.splitlines():
        line = line.strip()
        if line and not line.startswith("Slide "):
            return line
    return _first_non_empty_line(content)


def _first_non_empty_line(content: str) -> str:
    for line in content.splitlines():
        line = line.strip()
        if line:
            return line
    return ""


def _looks_like_utf8_text(data: bytes) -> bool:
    if b"\x00" in data:
        return False
    try:
        text = data.decode("utf-8")
    except UnicodeDecodeError:
        return False
    if not text.strip():
        return True
    control_count = sum(1 for char in text if ord(char) < 32 and char not in "\n\r\t\f")
    return control_count / max(len(text), 1) < 0.05


def _looks_like_zip(data: bytes) -> bool:
    return data.startswith((b"PK\x03\x04", b"PK\x05\x06", b"PK\x07\x08"))


def _has_image_magic(data: bytes) -> bool:
    return (
        data.startswith(b"\x89PNG\r\n\x1a\n")
        or data.startswith(b"\xff\xd8\xff")
        or data.startswith((b"GIF87a", b"GIF89a"))
        or data.startswith(b"BM")
        or data.startswith((b"II*\x00", b"MM\x00*"))
        or (len(data) >= 12 and data.startswith(b"RIFF") and data[8:12] == b"WEBP")
    )


def _has_legacy_office_magic(data: bytes) -> bool:
    return data.startswith(b"\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1")


def _normalize_text(value: str) -> str:
    return "\n".join(
        line for line in (_normalize_line(line) for line in value.splitlines()) if line
    )


def _normalize_line(value: str) -> str:
    return " ".join(value.strip().split())
