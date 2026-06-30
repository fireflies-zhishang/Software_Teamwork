import pytest

from parser_service.backends.ppstructurev3 import (
    PPStructureV3Backend,
    _callable_accepts_kwargs,
    _ChildSuccess,
    _collect_pages_and_markdown,
    _merged_markdown_from_items,
    _merged_markdown_from_result,
    _page_retry_warning,
    _pages_from_result,
    _parse_pdf_in_subprocess_batches,
    _predict_result,
    _run_child_process,
    _text_layer_status,
    _unwrap_child_result,
    _write_child_result,
)
from parser_service.service import ParsedDocument, ParsedPage


class FakeStructureResult:
    def __init__(self, markdown: dict[str, object]) -> None:
        self.markdown = markdown


class FakeStructureResultWithMarkdownTexts:
    def __init__(self, markdown_texts: str) -> None:
        self.markdown_texts = markdown_texts


class FakeStructureResultWithDict:
    def __init__(self, data: dict[str, object]) -> None:
        self.data = data

    def to_dict(self):
        return self.data


class FakeStructureResultWithJSON:
    def __init__(self, data: dict[str, object]) -> None:
        self.data = data

    def json(self):
        return self.data


class FakeStructurePipeline:
    def __init__(self) -> None:
        self.markdown_items = []

    def concatenate_markdown_pages(self, markdown_items):
        self.markdown_items = markdown_items
        return "\n\n".join(
            item.get("markdown_text") or item.get("markdown_texts") for item in markdown_items
        )


class FakeTupleMarkdownPipeline:
    def concatenate_markdown_pages(self, markdown_items):
        return (
            "\n\n".join(
                item.get("markdown_text") or item.get("markdown_texts") for item in markdown_items
            ),
            [{"image": "ignored"}],
        )


class FakePredictIterPipeline:
    def __init__(self) -> None:
        self.predict_calls = 0
        self.predict_iter_calls = 0

    def predict_iter(self, *, input):
        self.predict_iter_calls += 1
        yield FakeStructureResult({"markdown_texts": f"{input} 第一页"})
        yield FakeStructureResult({"markdown_texts": "第二页"})

    def predict(self, *, input):
        self.predict_calls += 1
        return [FakeStructureResult({"markdown_texts": "predict fallback"})]


class FakeVisualPagePipeline:
    def predict(self, *, input):
        return [
            FakeStructureResultWithDict(
                {
                    "markdown_texts": "绝缘配合\n\n使用导则",
                    "rec_scores": [0.9, 0.8],
                }
            )
        ]

    def concatenate_markdown_pages(self, markdown_items):
        return markdown_items[0]


class FakePPStructureV3:
    def __init__(self, *, lang=None, use_doc_orientation_classify=None, **kwargs) -> None:
        pass


class StrictPPStructureV3:
    def __init__(self, *, lang=None) -> None:
        pass


def _write_large_parsed_document(result_path: str) -> None:
    content = "section\n" * 20000
    _write_child_result(
        result_path,
        _ChildSuccess(
            ParsedDocument(
                content=content,
                title="large",
                backend="ppstructurev3",
                pages=[
                    ParsedPage(
                        page_number=1,
                        content=content,
                    )
                ],
            )
        ),
    )


def _sleep_without_result(result_path: str) -> None:
    import time

    time.sleep(30)


def test_pages_from_result_uses_official_markdown_page_merge():
    pipeline = FakeStructurePipeline()
    result = [
        FakeStructureResult({"markdown_text": "# 标准\n\n第一页"}),
        FakeStructureResult({"markdown_text": "| 项目 | 数值 |\n| --- | --- |\n| A | 1 |"}),
    ]
    pages = _pages_from_result(result)
    merged = _merged_markdown_from_result(result, pipeline)

    assert len(pages) == 2
    assert pages[0].page_number == 1
    assert pages[0].content == "# 标准\n第一页"
    assert pages[1].page_number == 2
    assert pages[1].content == "| 项目 | 数值 |\n| --- | --- |\n| A | 1 |"
    assert merged == "# 标准\n第一页\n| 项目 | 数值 |\n| --- | --- |\n| A | 1 |"
    assert pipeline.markdown_items == [
        {"markdown_text": "# 标准\n\n第一页"},
        {"markdown_text": "| 项目 | 数值 |\n| --- | --- |\n| A | 1 |"},
    ]


def test_pages_from_result_falls_back_to_text_extraction():
    pipeline = FakeStructurePipeline()
    result = [{"res": {"rec_texts": ["绝缘配合", "使用导则"]}}]
    pages = _pages_from_result(result)

    assert len(pages) == 1
    assert pages[0].content == "绝缘配合\n使用导则"
    assert _merged_markdown_from_result(result, pipeline) == ""


def test_pages_from_result_accepts_paddleocr_37_markdown_texts():
    result = [
        FakeStructureResult({"markdown_texts": "# 第一页\n\n绝缘配合"}),
        FakeStructureResult({"markdown_texts": "# 第二页\n\n使用导则"}),
    ]
    pipeline = FakeStructurePipeline()

    pages = _pages_from_result(result)
    merged = _merged_markdown_from_result(result, pipeline)

    assert [page.page_number for page in pages] == [1, 2]
    assert pages[0].content == "# 第一页\n绝缘配合"
    assert pages[1].content == "# 第二页\n使用导则"
    assert merged == "# 第一页\n绝缘配合\n# 第二页\n使用导则"
    assert pipeline.markdown_items == [
        {"markdown_texts": "# 第一页\n\n绝缘配合"},
        {"markdown_texts": "# 第二页\n\n使用导则"},
    ]


def test_pages_from_result_accepts_markdown_texts_attribute_and_to_dict():
    result = [
        FakeStructureResultWithMarkdownTexts("属性页\n\n内容"),
        FakeStructureResultWithDict({"res": {"markdown": {"markdown_texts": "字典页\n\n内容"}}}),
        FakeStructureResultWithJSON({"markdown_text": "JSON页\n\n内容"}),
    ]

    pages = _pages_from_result(result)

    assert [page.content for page in pages] == ["属性页\n内容", "字典页\n内容", "JSON页\n内容"]


def test_predict_result_prefers_official_predict_iter_for_incremental_processing():
    pipeline = FakePredictIterPipeline()

    result = _predict_result(pipeline, "scan.pdf")
    pages, markdown_items, fallback_texts, confidences = _collect_pages_and_markdown(result)

    assert pipeline.predict_iter_calls == 1
    assert pipeline.predict_calls == 0
    assert [page.content for page in pages] == ["scan.pdf 第一页", "第二页"]
    assert markdown_items == [{"markdown_texts": "scan.pdf 第一页"}, {"markdown_texts": "第二页"}]
    assert fallback_texts == []
    assert confidences == []


def test_merged_markdown_accepts_official_tuple_return_and_ignores_images():
    pipeline = FakeTupleMarkdownPipeline()
    markdown_items = [
        {"markdown_texts": "第一页", "markdown_images": {"heavy": object()}},
        {"markdown_texts": "第二页"},
    ]

    merged = _merged_markdown_from_items(markdown_items, pipeline)

    assert merged == "第一页\n第二页"


def test_backend_pipeline_kwargs_use_high_fidelity_defaults_and_official_params():
    backend = PPStructureV3Backend(
        layout_detection_model_name="PP-DocLayout-S",
        text_detection_model_name="PP-OCRv5_mobile_det",
        text_recognition_model_name="PP-OCRv5_mobile_rec",
        text_det_limit_side_len=768,
        text_det_limit_type="max",
        text_recognition_batch_size=1,
        textline_orientation_batch_size=1,
        seal_text_recognition_batch_size=1,
        formula_recognition_batch_size=1,
        chart_recognition_batch_size=1,
        markdown_ignore_labels=["header", "footer"],
    )

    kwargs = backend._pipeline_kwargs()

    assert kwargs["layout_detection_model_name"] == "PP-DocLayout-S"
    assert kwargs["text_detection_model_name"] == "PP-OCRv5_mobile_det"
    assert kwargs["text_recognition_model_name"] == "PP-OCRv5_mobile_rec"
    assert kwargs["text_det_limit_side_len"] == 768
    assert kwargs["text_det_limit_type"] == "max"
    assert kwargs["text_recognition_batch_size"] == 1
    assert kwargs["textline_orientation_batch_size"] == 1
    assert kwargs["seal_text_recognition_batch_size"] == 1
    assert kwargs["formula_recognition_batch_size"] == 1
    assert kwargs["chart_recognition_batch_size"] == 1
    assert kwargs["markdown_ignore_labels"] == ["header", "footer"]
    assert kwargs["use_seal_recognition"] is True
    assert kwargs["use_table_recognition"] is True
    assert kwargs["use_formula_recognition"] is True
    assert kwargs["use_chart_recognition"] is True
    assert kwargs["use_region_detection"] is True


def test_backend_omits_layout_model_name_by_default_for_official_model_selection():
    kwargs = PPStructureV3Backend()._pipeline_kwargs()

    assert "layout_detection_model_name" not in kwargs
    assert "engine" not in kwargs


def test_backend_omits_engine_even_when_configured_for_official_ppstructurev3():
    kwargs = PPStructureV3Backend(engine="paddle")._pipeline_kwargs()

    assert "engine" not in kwargs


def test_callable_accepts_kwargs_respects_constructor_signature():
    assert _callable_accepts_kwargs(FakePPStructureV3, {"lang": "ch", "device": "cpu"}) is True
    assert _callable_accepts_kwargs(StrictPPStructureV3, {"lang": "ch"}) is True
    assert _callable_accepts_kwargs(StrictPPStructureV3, {"lang": "ch", "device": "cpu"}) is False


def test_health_uses_lightweight_runtime_probe_without_loading_model(monkeypatch):
    class FakeModule:
        PPStructureV3 = FakePPStructureV3

    imports = []

    def fake_import_module(name):
        imports.append(name)
        if name == "paddleocr":
            return FakeModule()
        if name == "pypdfium2":
            return object()
        raise AssertionError(name)

    class FakeBackend(PPStructureV3Backend):
        def _ensure_pipeline(self):
            raise AssertionError("health must not load the model")

    monkeypatch.setattr(
        "parser_service.backends.ppstructurev3.importlib.import_module",
        fake_import_module,
    )

    health = FakeBackend().health()

    assert health.ready is True
    assert health.status == "ready"
    assert imports == ["paddleocr", "pypdfium2"]


def test_health_reports_incompatible_constructor_args(monkeypatch):
    class FakeModule:
        PPStructureV3 = StrictPPStructureV3

    def fake_import_module(name):
        if name == "paddleocr":
            return FakeModule()
        if name == "pypdfium2":
            return object()
        raise AssertionError(name)

    monkeypatch.setattr(
        "parser_service.backends.ppstructurev3.importlib.import_module",
        fake_import_module,
    )

    health = PPStructureV3Backend(device="cpu").health()

    assert health.ready is False
    assert health.reason == "ppstructurev3 constructor arguments are not compatible"


def test_backend_pipeline_kwargs_enable_precision_preprocessing_when_configured():
    kwargs = PPStructureV3Backend(
        use_doc_orientation_classify=True,
        use_doc_unwarping=True,
        use_textline_orientation=True,
    )._pipeline_kwargs()

    assert kwargs["use_doc_orientation_classify"] is True
    assert kwargs["use_doc_unwarping"] is True
    assert kwargs["use_textline_orientation"] is True


def test_collect_pages_and_markdown_extracts_confidences_from_result_mappings():
    result = [
        FakeStructureResultWithDict(
            {
                "res": {
                    "markdown": {"markdown_texts": "第一页"},
                    "rec_scores": [0.91, 0.87],
                    "blocks": [{"score": 0.8}, {"confidence": 1.2}],
                }
            }
        ),
        {"res": {"rec_texts": ["第二页"], "scores": [0.72]}},
    ]

    pages, markdown_items, fallback_texts, confidences = _collect_pages_and_markdown(result)

    assert [page.content for page in pages] == ["第一页", "第二页"]
    assert markdown_items == [{"markdown_texts": "第一页"}]
    assert fallback_texts == ["第二页"]
    assert confidences == [0.91, 0.87, 0.8, 0.72]


def test_text_layer_status_marks_broken_pdf_encoding_artifacts():
    broken = "I¥J B\"J !!!! #### //// (((( )))) " * 4

    assert _text_layer_status("") == "empty"
    assert _text_layer_status("短文本") == "short"
    assert _text_layer_status(broken) == "broken"
    assert _text_layer_status("绝缘配合 第2部分 使用导则 正常中文文本内容") == "usable"


def test_backend_attempt_dpis_are_capped_and_monotonic():
    assert PPStructureV3Backend(
        default_dpi=180,
        retry_dpi=220,
        max_retry_dpi=300,
    )._attempt_dpis() == [180, 220]
    assert PPStructureV3Backend(
        default_dpi=220,
        retry_dpi=300,
        max_retry_dpi=240,
    )._attempt_dpis() == [220, 240]
    assert PPStructureV3Backend(
        default_dpi=300,
        retry_dpi=220,
        max_retry_dpi=300,
    )._attempt_dpis() == [300]


def test_child_config_disables_nested_subprocess_isolation():
    config = PPStructureV3Backend(
        default_dpi=180,
        retry_dpi=220,
        max_retry_dpi=300,
        subprocess_isolation=True,
        subprocess_timeout_seconds=120,
        memory_limit_mb=14500,
    )._child_config()

    assert config["default_dpi"] == 180
    assert config["retry_dpi"] == 220
    assert config["max_retry_dpi"] == 300
    assert config["subprocess_isolation"] is False
    assert config["subprocess_timeout_seconds"] == 120
    assert config["memory_limit_mb"] == 14500


def test_parse_pdf_in_subprocess_batches_splits_pages(monkeypatch, tmp_path):
    calls = []

    def fake_page_count(input_path):
        assert input_path == tmp_path / "scan.pdf"
        return 3

    def fake_parse_batch(input_path, *, document_name, config, page_indexes):
        calls.append(page_indexes)
        return ParsedDocument(
            content="\n".join(f"page {index + 1}" for index in page_indexes),
            title="scan",
            backend="ppstructurev3",
            pages=[
                ParsedPage(
                    page_number=index + 1,
                    content=f"page {index + 1}",
                    dpi=config["default_dpi"],
                )
                for index in page_indexes
            ],
        )

    monkeypatch.setattr(
        "parser_service.backends.ppstructurev3._pdf_page_count",
        fake_page_count,
    )
    monkeypatch.setattr(
        "parser_service.backends.ppstructurev3._parse_pdf_batch_in_subprocess",
        fake_parse_batch,
    )
    pdf = tmp_path / "scan.pdf"
    pdf.write_bytes(b"%PDF")

    parsed = _parse_pdf_in_subprocess_batches(
        pdf,
        document_name="scan.pdf",
        config={"page_batch_size": 2, "default_dpi": 180},
    )

    assert calls == [[0, 1], [2]]
    assert parsed.content == "page 1\npage 2\npage 3"
    assert [page.page_number for page in parsed.pages] == [1, 2, 3]


def test_parse_path_in_subprocess_returns_large_result_without_queue(tmp_path):
    result = _run_child_process(
        target=_write_large_parsed_document,
        args=(),
        config={"memory_limit_mb": 14500},
    )
    parsed = _unwrap_child_result(result)

    assert parsed.backend == "ppstructurev3"
    assert parsed.pages[0].page_number == 1
    assert parsed.pages[0].content.startswith("section")
    assert len(parsed.content) > 100_000


def test_parse_path_in_subprocess_terminates_child_on_backend_timeout(tmp_path):
    with pytest.raises(Exception) as exc_info:
        _run_child_process(
            target=_sleep_without_result,
            args=(),
            config={
                "subprocess_timeout_seconds": 0.1,
                "memory_limit_mb": 14500,
            },
        )

    assert "ppstructurev3 subprocess timed out" in str(exc_info.value)


def test_parse_visual_page_preserves_page_metadata_and_confidence(tmp_path):
    class FakeBackend(PPStructureV3Backend):
        def _ensure_pipeline(self):
            return FakeVisualPagePipeline()

    image_path = tmp_path / "page.png"
    image_path.write_bytes(b"fake")
    page = FakeBackend()._parse_visual_page(
        image_path,
        page_number=7,
        parse_strategy="ocr",
        text_layer_status="broken",
        dpi=180,
        warnings=["low_text_quality"],
    )

    assert page.page_number == 7
    assert page.content == "绝缘配合\n使用导则"
    assert page.parse_strategy == "ocr"
    assert page.text_layer_status == "broken"
    assert page.ocr_confidence == pytest.approx(0.85)
    assert page.dpi == 180
    assert page.warnings == ["low_text_quality"]


def test_page_retry_warning_uses_confidence_without_retrying_short_or_non_cjk_text():
    assert (
        _page_retry_warning(
            page=ParsedPage(
                page_number=1,
                content="绝缘配合 使用导则 正常中文文本内容",
                ocr_confidence=0.7,
            ),
            threshold=0.85,
        )
        == "low_ocr_confidence"
    )
    assert (
        _page_retry_warning(
            page=ParsedPage(page_number=2, content="Table 1"),
            threshold=0.85,
        )
        == ""
    )
    assert (
        _page_retry_warning(
            page=ParsedPage(page_number=3, content="| Voltage | kV |\n| --- | --- |\n| 500 | kV |"),
            threshold=0.85,
        )
        == ""
    )
    assert (
        _page_retry_warning(
            page=ParsedPage(page_number=4, content='I¥J B"J !!!! #### //// (((( ))))'),
            threshold=0.85,
        )
        == "broken_text_encoding"
    )
