from __future__ import annotations

from types import SimpleNamespace

import pytest

from infrastructure import ocr as ocr_mod
from infrastructure.ocr import (
    _OCRCandidate,
    _clean_text,
    _measure_text_quality,
    _pick_best_candidate,
    _run_engine,
    _score_text_quality,
)


def test_extract_best_ocr_keeps_good_pdftotext(monkeypatch):
    monkeypatch.setattr(
        ocr_mod.pdftotext,
        "to_text",
        lambda _path: (
            "TAX INVOICE\n"
            "Invoice No: INV-42\n"
            "PO Number: PO-7\n"
            "Vendor GSTIN: 27AADCB2230M1ZV\n"
            "Grand Total: 1,180.00\n"
        ),
    )
    monkeypatch.setattr(
        ocr_mod,
        "_load_tesseract_module",
        lambda: pytest.fail("tesseract should not be used for good pdftotext output"),
    )

    result = ocr_mod.extract_best_ocr("/tmp/test.pdf")

    assert result.engine == "pdftotext"
    assert "quality looks acceptable" in result.reason
    assert "INV-42" in result.text


def test_extract_best_ocr_falls_back_to_tesseract_on_low_quality_pdftotext(monkeypatch):
    monkeypatch.setattr(
        ocr_mod.pdftotext,
        "to_text",
        lambda _path: "%PDF-1.4\nobj\nxref\nstream\nendobj\ntrailer\n",
    )
    monkeypatch.setattr(
        ocr_mod,
        "_load_tesseract_module",
        lambda: SimpleNamespace(
            to_text=lambda _path: (
                "TAX INVOICE\n"
                "Invoice No: INV-42\n"
                "PO Number: PO-7\n"
                "Buyer GSTIN: 29ABCDE1234F1Z9\n"
                "Grand Total: 1,180.00\n"
            )
        ),
    )

    result = ocr_mod.extract_best_ocr("/tmp/test.pdf")

    assert result.engine == "tesseract"
    assert "pdftotext text looked low quality" in result.reason
    assert "INV-42" in result.text


# ---------------------------------------------------------------------------
# Gap F — OCR internal helpers
# ---------------------------------------------------------------------------


def test_clean_text_none_returns_empty():
    assert _clean_text(None) == ""


def test_clean_text_bytes_decoded_and_null_stripped():
    raw = b"hello\x00world"
    result = _clean_text(raw)
    assert result == "helloworld"
    assert "\x00" not in result


def test_score_text_quality_empty_text_is_zero():
    metrics = _measure_text_quality("")
    assert _score_text_quality(metrics) == 0.0


def test_score_text_quality_short_text_applies_length_penalty():
    # 50-char invoice-like text: below the 80-char threshold, penalty of -0.75.
    text = "Invoice total 500 INV-1"  # 23 chars — well below 80
    metrics = _measure_text_quality(text)
    score_with_penalty = _score_text_quality(metrics)

    # Build an equivalent but longer text to confirm penalty is the difference.
    long_text = text + " " * 60 + "padding words here today now"  # > 80 chars
    long_metrics = _measure_text_quality(long_text)
    score_without_penalty = _score_text_quality(long_metrics)

    assert score_with_penalty < score_without_penalty


def test_score_text_quality_clamps_to_zero_on_heavy_replacement_chars():
    # A string dominated by replacement chars (U+FFFD) scores so badly it
    # would go negative — the max(score, 0.0) clamp must prevent that.
    garbage = "�" * 200
    metrics = _measure_text_quality(garbage)
    assert _score_text_quality(metrics) == 0.0


def test_pick_best_candidate_tesseract_wins_at_exact_margin():
    empty_metrics = _measure_text_quality("")
    good_metrics = _measure_text_quality(
        "TAX INVOICE Invoice No INV-1 GSTIN amount total sgst cgst subtotal qty description"
    )
    pdf = _OCRCandidate(text="some text", engine="pdftotext", input_module=None,
                        score=2.0, metrics=good_metrics)
    # Exactly 0.25 above pdftotext — should be chosen (>= boundary).
    tess = _OCRCandidate(text="other text", engine="tesseract", input_module=None,
                         score=2.25, metrics=good_metrics)
    assert _pick_best_candidate(pdf, tess).engine == "tesseract"


def test_pick_best_candidate_pdf_wins_when_tesseract_just_under_margin():
    good_metrics = _measure_text_quality(
        "TAX INVOICE Invoice No INV-1 GSTIN amount total sgst cgst subtotal qty description"
    )
    pdf = _OCRCandidate(text="some text", engine="pdftotext", input_module=None,
                        score=2.0, metrics=good_metrics)
    # 0.24 above — does NOT meet the 0.25 margin requirement.
    tess = _OCRCandidate(text="other text", engine="tesseract", input_module=None,
                         score=2.24, metrics=good_metrics)
    assert _pick_best_candidate(pdf, tess).engine == "pdftotext"


def test_run_engine_oserror_returns_empty_candidate_with_error(monkeypatch):
    monkeypatch.setattr(ocr_mod.pdftotext, "to_text", lambda _p: (_ for _ in ()).throw(OSError("disk error")))
    candidate = _run_engine("/tmp/x.pdf", "pdftotext", ocr_mod.pdftotext)
    assert candidate.text == ""
    assert candidate.score == 0.0
    assert candidate.error is not None
    assert "disk error" in candidate.error


def test_extract_best_ocr_tesseract_unavailable_keeps_low_quality_pdftotext(monkeypatch):
    # pdftotext returns garbage (score < 2.2); tesseract module is unavailable.
    monkeypatch.setattr(
        ocr_mod.pdftotext,
        "to_text",
        lambda _path: "%PDF-1.4\nobj\n",
    )
    monkeypatch.setattr(ocr_mod, "_load_tesseract_module", lambda: None)

    result = ocr_mod.extract_best_ocr("/tmp/test.pdf")

    assert result.engine == "pdftotext"
    assert "unavailable or failed" in result.reason
