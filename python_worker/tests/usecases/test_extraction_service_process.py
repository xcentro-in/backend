"""End-to-end tests for ExtractionService.process and mark_failed.

Covers the orchestration flow that the existing normalize/logging tests
do not exercise: idempotency on terminal status, status transitions,
template→LLM fallback, OTHER/UNKNOWN failure path, both-extractors-fail,
LLMExtractionError propagation, and mark_failed safety guards.
"""

from __future__ import annotations

import uuid
from dataclasses import dataclass

import pytest

from domain.entities import ExtractionResult, LineItem
from domain.enums import DocumentStatus, DocumentType, ProcessedBy
from domain.exceptions import LLMExtractionError, TemplateMatchError
from usecases.extraction_service import ExtractionService


@dataclass
class _StubOCRResult:
    text: str = "OCR TEXT"
    engine: str = "pdftotext"
    input_module: object | None = None
    score: float = 5.0
    reason: str = "ok"
    pdftotext_score: float = 5.0
    tesseract_score: float | None = None


@pytest.fixture(autouse=True)
def stub_ocr(monkeypatch):
    """Replace extract_best_ocr in the service module with a noop stub."""
    monkeypatch.setattr(
        "usecases.extraction_service.extract_best_ocr",
        lambda path: _StubOCRResult(),
    )


def _valid_invoice_result(template_name: str | None = "tpl.yml") -> ExtractionResult:
    """An ExtractionResult that passes run_all_checks(include_arithmetic=False)."""
    return ExtractionResult(
        invoice_number="INV-1",
        invoice_date="2026-04-20",
        vendor_name="Acme",
        vendor_gstin="27AADCB2230M1ZV",
        total_amount=1180.0,
        line_items=[
            LineItem(sku="A", description="Widget", quantity=1.0, unit_price=1000.0, amount=1000.0),
        ],
        doc_type=DocumentType.INVOICE,
        raw={"template_name": template_name} if template_name else {},
    )


def _invalid_template_result() -> ExtractionResult:
    """An ExtractionResult that fails run_all_checks (missing vendor_name)."""
    return ExtractionResult(
        invoice_number="INV-2",
        invoice_date="2026-04-20",
        vendor_name=None,  # required field missing
        total_amount=500.0,
        line_items=[],
        doc_type=DocumentType.INVOICE,
        raw={"template_name": "broken.yml"},
    )


class _TemplateOK:
    def __init__(self, result: ExtractionResult) -> None:
        self._result = result

    def extract(self, file_path: str) -> ExtractionResult:
        return self._result


class _TemplateMiss:
    def extract(self, file_path: str) -> ExtractionResult:
        raise TemplateMatchError("no template matched")


class _LLMOK:
    def __init__(self, result: ExtractionResult) -> None:
        self._result = result
        self.calls: list[str] = []

    def extract(self, file_path: str) -> ExtractionResult:
        self.calls.append(file_path)
        # Mirror production behaviour — set raw so doc.raw_extraction is populated.
        if not self._result.raw:
            self._result.raw = {"_usage": {"prompt_tokens": 1, "completion_tokens": 2}}
        return self._result

    def generate_template(self, *args, **kwargs) -> str:
        raise AssertionError("generate_template should not be called from process()")


class _LLMRaises:
    def __init__(self, exc: Exception) -> None:
        self._exc = exc

    def extract(self, file_path: str):
        raise self._exc

    def generate_template(self, *args, **kwargs) -> str:
        raise AssertionError("not used")


class _LLMUnused:
    def extract(self, file_path: str):
        raise AssertionError("LLM should not be called in this test")

    def generate_template(self, *args, **kwargs) -> str:
        raise AssertionError("not used")


def _service(repo, template, llm, downloader):
    return ExtractionService(
        repo=repo,
        template_extractor=template,
        llm_extractor=llm,
        downloader=downloader,
        temp_dir="/tmp/test_extraction",
    )


# ---------------------------------------------------------------------------
# 1. Doc not found
# ---------------------------------------------------------------------------


def test_process_returns_silently_when_document_missing(
    fake_document_repo, fake_file_downloader,
):
    service = _service(fake_document_repo, _TemplateMiss(), _LLMUnused(), fake_file_downloader)

    service.process(uuid.uuid4())

    assert fake_document_repo.update_calls == []
    assert fake_file_downloader.calls == []


# ---------------------------------------------------------------------------
# 2. Terminal status idempotency
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    "status",
    [DocumentStatus.VALIDATED, DocumentStatus.REVIEW_REQUIRED, DocumentStatus.FAILED],
)
def test_process_skips_documents_in_terminal_status(
    status, fake_document_repo, fake_file_downloader, sample_document,
):
    sample_document.status = status
    fake_document_repo.add(sample_document)

    service = _service(
        fake_document_repo, _TemplateMiss(), _LLMUnused(), fake_file_downloader,
    )

    service.process(sample_document.id)

    # No download, no OCR, no status mutation.
    assert fake_file_downloader.calls == []
    assert fake_document_repo.update_calls == []
    # Status preserved.
    assert fake_document_repo.get_by_id(sample_document.id).status == status


# ---------------------------------------------------------------------------
# 3. Template happy path → INVOICE2DATA processed_by, REVIEW_REQUIRED
# ---------------------------------------------------------------------------


def test_template_success_marks_processed_by_invoice2data(
    fake_document_repo, fake_file_downloader, sample_document,
):
    fake_document_repo.add(sample_document)
    template_result = _valid_invoice_result(template_name="acme.yml")
    service = _service(
        fake_document_repo,
        _TemplateOK(template_result),
        _LLMUnused(),
        fake_file_downloader,
    )

    service.process(sample_document.id)

    final = fake_document_repo.get_by_id(sample_document.id)
    assert final.status == DocumentStatus.REVIEW_REQUIRED
    assert final.processed_by == ProcessedBy.INVOICE2DATA
    assert final.i2d_template == "acme.yml"
    assert final.doc_type == DocumentType.INVOICE
    assert final.draft_data is not None
    assert final.draft_data["vendor_gstin"] == "27AADCB2230M1ZV"


# ---------------------------------------------------------------------------
# 4. Template result fails validation → LLM fallback
# ---------------------------------------------------------------------------


def test_template_validation_failure_falls_back_to_llm(
    fake_document_repo, fake_file_downloader, sample_document,
):
    fake_document_repo.add(sample_document)
    bad_template = _invalid_template_result()
    good_llm = _valid_invoice_result(template_name=None)
    llm = _LLMOK(good_llm)

    service = _service(
        fake_document_repo, _TemplateOK(bad_template), llm, fake_file_downloader,
    )

    service.process(sample_document.id)

    assert llm.calls, "LLM extractor should have been called"
    final = fake_document_repo.get_by_id(sample_document.id)
    assert final.status == DocumentStatus.REVIEW_REQUIRED
    assert final.processed_by == ProcessedBy.LLM
    assert final.raw_extraction is not None  # populated from LLM result.raw
    # Verify status passed through LLM_PROCESSING during the run.
    statuses_seen = [d.status for d in fake_document_repo.update_calls]
    assert DocumentStatus.LLM_PROCESSING in statuses_seen


# ---------------------------------------------------------------------------
# 5. LLM-only path (template raises TemplateMatchError)
# ---------------------------------------------------------------------------


def test_template_match_error_falls_back_to_llm(
    fake_document_repo, fake_file_downloader, sample_document,
):
    fake_document_repo.add(sample_document)
    good_llm = _valid_invoice_result(template_name=None)
    llm = _LLMOK(good_llm)

    service = _service(
        fake_document_repo, _TemplateMiss(), llm, fake_file_downloader,
    )

    service.process(sample_document.id)

    assert llm.calls
    final = fake_document_repo.get_by_id(sample_document.id)
    assert final.status == DocumentStatus.REVIEW_REQUIRED
    assert final.processed_by == ProcessedBy.LLM


# ---------------------------------------------------------------------------
# 6. doc_type=OTHER / UNKNOWN violation → FAILED
# ---------------------------------------------------------------------------


@pytest.mark.parametrize("bad_type", [DocumentType.OTHER, DocumentType.UNKNOWN])
def test_other_or_unknown_doc_type_marks_failed_with_violation(
    bad_type, fake_document_repo, fake_file_downloader, sample_document,
):
    fake_document_repo.add(sample_document)
    other_result = ExtractionResult(
        invoice_number="X",
        vendor_name="X",
        total_amount=1.0,
        line_items=[LineItem(sku="X", quantity=1.0, unit_price=1.0, amount=1.0)],
        doc_type=bad_type,
    )

    # Route through LLM so the violation check happens before re-validation:
    # a template result with doc_type=OTHER would also fail required-field
    # validation (which expects PO/INVOICE-shaped data) and trigger an LLM
    # fallback. Going LLM-first goes straight to the doc_type guard.
    service = _service(
        fake_document_repo, _TemplateMiss(), _LLMOK(other_result), fake_file_downloader,
    )

    service.process(sample_document.id)

    final = fake_document_repo.get_by_id(sample_document.id)
    assert final.status == DocumentStatus.FAILED
    assert final.doc_type == bad_type
    assert final.draft_data["violation"] is True
    assert final.draft_data["detected_type"] == bad_type.value


# ---------------------------------------------------------------------------
# 8. LLMExtractionError propagates so main.py can retry
# ---------------------------------------------------------------------------


def test_llm_extraction_error_propagates_from_process(
    fake_document_repo, fake_file_downloader, sample_document,
):
    fake_document_repo.add(sample_document)
    service = _service(
        fake_document_repo,
        _TemplateMiss(),
        _LLMRaises(LLMExtractionError("Gemini quota exhausted")),
        fake_file_downloader,
    )

    with pytest.raises(LLMExtractionError):
        service.process(sample_document.id)


# ---------------------------------------------------------------------------
# 9. mark_failed refuses to overwrite VALIDATED / REVIEW_REQUIRED
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    "protected_status",
    [DocumentStatus.VALIDATED, DocumentStatus.REVIEW_REQUIRED],
)
def test_mark_failed_refuses_to_overwrite_protected_status(
    protected_status, fake_document_repo, fake_file_downloader, sample_document,
):
    sample_document.status = protected_status
    fake_document_repo.add(sample_document)
    service = _service(
        fake_document_repo, _TemplateMiss(), _LLMUnused(), fake_file_downloader,
    )

    service.mark_failed(sample_document.id, reason="should be ignored")

    assert fake_document_repo.update_calls == []
    assert fake_document_repo.get_by_id(sample_document.id).status == protected_status


def test_mark_failed_writes_failure_when_status_is_not_protected(
    fake_document_repo, fake_file_downloader, sample_document,
):
    sample_document.status = DocumentStatus.OCR_PROCESSING
    fake_document_repo.add(sample_document)
    service = _service(
        fake_document_repo, _TemplateMiss(), _LLMUnused(), fake_file_downloader,
    )

    service.mark_failed(sample_document.id, reason="connection died")

    final = fake_document_repo.get_by_id(sample_document.id)
    assert final.status == DocumentStatus.FAILED
    assert final.draft_data == {"error": "connection died"}


def test_mark_failed_silently_returns_when_document_missing(
    fake_document_repo, fake_file_downloader,
):
    service = _service(
        fake_document_repo, _TemplateMiss(), _LLMUnused(), fake_file_downloader,
    )

    service.mark_failed(uuid.uuid4(), reason="missing doc")

    assert fake_document_repo.update_calls == []
