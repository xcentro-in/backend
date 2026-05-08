"""Tests for Invoice2DataExtractor._map — the dict-to-domain mapping layer.

We don't parse real PDFs here; those live in the golden-PDF tests (Phase 3).
This file covers the mapping logic: line-item coalescing across alternative
field names, numeric coercion, template-name → doc_type inference, and the
confidence-score build.
"""

from __future__ import annotations

import yaml

from infrastructure.invoice2data_wrapper import (
    Invoice2DataExtractor,
    _build_no_candidate_reason,
    _sanitize_keywords_to_ocr,
)
from domain.enums import DocumentType


def test_map_minimal_invoice_fields():
    raw = {
        "invoice_number": "INV-42",
        "date": "2026-04-20",
        "issuer": "Acme Corp",
        "amount": "1180.00",
    }

    result = Invoice2DataExtractor._map(raw, template_name="standard_gst_invoice")

    assert result.invoice_number == "INV-42"
    assert result.invoice_date == "2026-04-20"
    assert result.vendor_name == "Acme Corp"
    assert result.total_amount == 1180.0
    assert result.doc_type == DocumentType.INVOICE


def test_map_purchase_order_template_sets_po_doc_type():
    raw = {"po_number": "PO-1", "date": "2026-04-20"}

    result = Invoice2DataExtractor._map(raw, template_name="acme_purchase_order")

    assert result.doc_type == DocumentType.PURCHASE_ORDER
    assert result.po_number == "PO-1"


def test_map_no_template_name_yields_unknown_doc_type():
    raw = {"invoice_number": "X", "date": "2026-04-20"}

    result = Invoice2DataExtractor._map(raw, template_name=None)

    assert result.doc_type == DocumentType.UNKNOWN


def test_map_line_items_normalizes_alternate_field_names():
    raw = {
        "lines": [
            {"desc": "Widget A", "qty": "2", "price": "100.00", "line_total": "200.00", "hsn": "1234"},
            {"description": "Widget B", "quantity": 1, "unit_price": 50.0, "amount": 50.0, "item_code": "SKU-B"},
        ],
    }

    result = Invoice2DataExtractor._map(raw)

    assert len(result.line_items) == 2
    a, b = result.line_items
    assert a.description == "Widget A"
    assert a.quantity == 2.0
    assert a.unit_price == 100.0
    assert a.amount == 200.0
    assert a.sku == "1234"
    assert b.description == "Widget B"
    assert b.sku == "SKU-B"
    assert b.amount == 50.0


def test_map_gstin_falls_back_to_generic_gstin_key():
    # Templates that pre-date the vendor_gstin split used a single "gstin" key.
    raw = {"gstin": "27AADCB2230M1ZV"}

    result = Invoice2DataExtractor._map(raw, template_name="legacy_template")

    assert result.vendor_gstin == "27AADCB2230M1ZV"


def test_map_vendor_gstin_takes_precedence_over_generic_gstin():
    raw = {"vendor_gstin": "27AAAAA0000A1Z5", "gstin": "FALLBACK"}

    result = Invoice2DataExtractor._map(raw, template_name="t")

    assert result.vendor_gstin == "27AAAAA0000A1Z5"


def test_map_shipping_falls_back_to_freight_alias():
    raw = {"freight": "25.00"}

    result = Invoice2DataExtractor._map(raw, template_name="t")

    assert result.shipping == 25.0


def test_map_confidence_scores_are_1_for_extracted_fields():
    raw = {
        "invoice_number": "INV-1",
        "amount": "100",
        "issuer": "Vendor",
    }

    result = Invoice2DataExtractor._map(raw, template_name="t")

    # Template matches are exact-match — every populated field gets 1.0.
    assert result.confidence_scores["invoice_number"] == 1.0
    assert result.confidence_scores["total_amount"] == 1.0
    assert result.confidence_scores["vendor_name"] == 1.0
    # Absent fields don't appear in the map.
    assert "buyer_gstin" not in result.confidence_scores


def test_map_gst_components_are_coerced_to_floats():
    raw = {"sgst": "90.00", "cgst": "90.00", "igst": None}

    result = Invoice2DataExtractor._map(raw, template_name="t")

    assert result.sgst == 90.0
    assert result.cgst == 90.0
    assert result.igst is None


def test_map_non_numeric_amount_becomes_none_not_crash():
    raw = {"amount": "INVALID_STRING"}

    result = Invoice2DataExtractor._map(raw, template_name="t")

    assert result.total_amount is None


def test_map_preserves_raw_dict_and_records_template_name():
    raw = {"invoice_number": "INV-1"}

    result = Invoice2DataExtractor._map(raw, template_name="my_template")

    # extraction_service reads template_name off result.raw to persist on the
    # document; this contract must not quietly break.
    assert result.raw["template_name"] == "my_template"


def test_build_no_candidate_reason_for_empty_ocr_is_human_readable():
    reason = _build_no_candidate_reason(
        template_count=3,
        extracted_text="",
        matched_template_names=[],
        extraction_failures=[],
        empty_extractions=[],
    )

    assert reason == (
        "No OCR text was extracted from the document; pdftotext and "
        "tesseract both returned empty text."
    )


# ---------------------------------------------------------------------------
# _sanitize_keywords_to_ocr
# ---------------------------------------------------------------------------

_BASE_OCR = "TAX INVOICE\nHSN/SAC\nBill To:\nsome vendor text\n12345"


def _make_yaml(keywords: list) -> str:
    return yaml.dump(
        {"keywords": keywords, "fields": {"invoice_number": "(?si)INV\\s*(\\d+)"}},
        default_flow_style=False,
        allow_unicode=True,
        sort_keys=False,
    )


def test_sanitize_keeps_matching_keywords():
    result = _sanitize_keywords_to_ocr(_make_yaml(["TAX INVOICE", "HSN/SAC"]), _BASE_OCR)
    data = yaml.safe_load(result)
    assert data["keywords"] == ["TAX INVOICE", "HSN/SAC"]


def test_sanitize_removes_hallucinated_keyword():
    result = _sanitize_keywords_to_ocr(
        _make_yaml(["HALLUCINATED_KEYWORD", "TAX INVOICE"]), _BASE_OCR
    )
    data = yaml.safe_load(result)
    assert data["keywords"] == ["TAX INVOICE"]


def test_sanitize_keyword_matching_is_case_sensitive_like_invoice2data():
    # invoice2data's matches_input uses re.search(kw, ocr) with no IGNORECASE.
    # "tax invoice" (lowercase) does NOT match "TAX INVOICE" in the OCR.
    result = _sanitize_keywords_to_ocr(_make_yaml(["tax invoice"]), _BASE_OCR)
    data = yaml.safe_load(result)
    # lowercase keyword stripped; fallback picks first all-caps line "TAX INVOICE"
    assert data["keywords"] == ["TAX INVOICE"]


def test_sanitize_falls_back_to_ocr_line_when_all_missing():
    result = _sanitize_keywords_to_ocr(_make_yaml(["NOT_IN_DOC"]), _BASE_OCR)
    data = yaml.safe_load(result)
    # Fallback picks the first all-caps line: "TAX INVOICE"
    assert data["keywords"] == ["TAX INVOICE"]


def test_sanitize_returns_original_when_no_fallback_available():
    ocr = "12345\n67890\n0.00"  # all-numeric lines — no valid fallback
    original = _make_yaml(["NOT_HERE"])
    result = _sanitize_keywords_to_ocr(original, ocr)
    assert result == original


def test_sanitize_returns_unchanged_on_invalid_yaml():
    bad_yaml = "keywords: [unclosed\nfields: {}"
    result = _sanitize_keywords_to_ocr(bad_yaml, _BASE_OCR)
    assert result == bad_yaml


def test_sanitize_preserves_fields_block_unchanged():
    result = _sanitize_keywords_to_ocr(_make_yaml(["TAX INVOICE"]), _BASE_OCR)
    data = yaml.safe_load(result)
    assert data["fields"]["invoice_number"] == "(?si)INV\\s*(\\d+)"


# ---------------------------------------------------------------------------
# _LINE_WRAP_CONTINUATION / _allow_identifier_line_wrap
# ---------------------------------------------------------------------------

from infrastructure.invoice2data_wrapper import (
    _LINE_WRAP_CONTINUATION,
    _allow_identifier_line_wrap,
    postprocess_template_regexes,
)
import re


def test_line_wrap_continuation_still_captures_split_invoice_number():
    """Continuation must still match a token that looks like the tail of an
    invoice number (alpha-numeric, slashes) to handle genuine OCR line splits."""
    pattern = _LINE_WRAP_CONTINUATION
    # A legitimate split continuation: space then more identifier chars
    assert re.search(pattern, " 1/003"), "should match split identifier tail"
    assert re.search(pattern, " ABC"), "should match split alpha tail"


def test_line_wrap_continuation_does_not_capture_iso_date_tail():
    """The continuation must NOT match a trailing ISO date (YYYY-MM-DD)
    because that would cause invoice_number to greedily swallow the date."""
    pattern = _LINE_WRAP_CONTINUATION
    # The negative lookahead should prevent matching ' 2025-08-03'
    # Anchoring the pattern to the start of the candidate suffix.
    full = re.compile(r"^" + pattern + r"$")
    assert not full.match(" 2025-08-03"), "should not capture ISO date"
    assert not full.match(" 2024/08/03"), "should not capture slash-separated date"


def test_line_wrap_continuation_does_not_capture_amount_tail():
    """Colon and period are dropped from the char class so amounts / timestamps
    are not mistakenly folded into the identifier capture."""
    pattern = _LINE_WRAP_CONTINUATION
    full = re.compile(r"^" + pattern + r"$")
    # Purely digit + period + digit is not a valid identifier suffix
    assert not full.match(" 1200.00"), "should not capture amount"
    assert not full.match(" 09:30:00"), "should not capture time"


def test_postprocess_does_not_fold_adjacent_date_into_invoice_number():
    """Regression test for the production failure: the LLM-generated regex for
    invoice_number must not capture a date that appears on the same OCR line."""
    yaml_input = yaml.dump({
        "issuer": "Test",
        "keywords": ["TAX INVOICE"],
        "fields": {
            "invoice_number": "(?si)Invoice\\s*No[.:]?\\s*(?P<invoice_number>[A-Z0-9][A-Z0-9/_-]+)",
        },
        "options": {"currency": "INR"},
    }, default_flow_style=False)

    result = postprocess_template_regexes(yaml_input)
    data = yaml.safe_load(result)
    regex_str = data["fields"]["invoice_number"]
    # After widening, attempt to match OCR with invoice number and adjacent ISO date
    ocr_line = "Invoice No: INV/2024/77157/002 2025-08-03"
    m = re.search(regex_str, ocr_line)
    assert m is not None, "regex must match the line"
    captured = m.group("invoice_number")
    assert "2025" not in captured, (
        f"regex must NOT capture the date portion; got {captured!r}"
    )
    assert captured == "INV/2024/77157/002", f"expected only invoice number; got {captured!r}"
