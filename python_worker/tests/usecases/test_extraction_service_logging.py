from __future__ import annotations

import logging

from domain.exceptions import TemplateMatchError
from usecases.extraction_service import ExtractionService


class _FailingTemplateExtractor:
    def extract(self, file_path: str):
        raise TemplateMatchError(
            "No OCR text was extracted from the document; pdftotext and "
            "tesseract both returned empty text."
        )


class _UnusedLLMExtractor:
    def extract(self, file_path: str):
        raise AssertionError("LLM extractor should not be called in this test")

    def generate_template(
        self,
        raw_ocr: str,
        draft_data: dict,
        historical_examples: list[dict] | None = None,
        prior_attempt: str | None = None,
        prior_error: str | None = None,
    ) -> str:
        raise AssertionError("LLM template generation should not be called in this test")


def test_try_template_logs_human_readable_fallback_reason(sample_document, caplog):
    service = ExtractionService(
        repo=None,
        template_extractor=_FailingTemplateExtractor(),
        llm_extractor=_UnusedLLMExtractor(),
        downloader=None,
    )

    with caplog.at_level(logging.INFO):
        result = service._try_template("/tmp/test.pdf", sample_document)

    assert result is None
    assert (
        "Template extraction failed for "
        f"{sample_document.id}; falling back to LLM: No OCR text was extracted "
        "from the document; pdftotext and tesseract both returned empty text."
    ) in caplog.text
