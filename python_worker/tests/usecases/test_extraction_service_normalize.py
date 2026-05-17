"""Tests for _normalize_result — the GST mutual-exclusivity and line-item
tax-rate defaulting rules applied before serialization.

Covers the business rules documented inline in extraction_service.py:

  1.  GST mutual exclusivity (IGST xor SGST+CGST).
  1b. Infer SGST/CGST (or IGST) from a lump-sum total_gst using GSTIN state codes.
  2.  Line items without tax_rate default to 0 when no doc-level rate exists.
  3.  Line items without tax_rate inherit doc-level gst_rate when no line has one.
  4.  Purchase orders have no invoice_number.
"""

from __future__ import annotations

import pytest

from domain.entities import ExtractionResult, LineItem
from domain.enums import DocumentType
from usecases.extraction_service import _normalize_result, _result_to_draft


# ---- Rule 1: GST mutual exclusivity -----------------------------------------

def _r(**overrides):
    """Minimal ExtractionResult factory."""
    return ExtractionResult(**overrides)


def test_igst_only_is_left_alone():
    result = _normalize_result(_r(igst=180.0))
    assert result.igst == 180.0
    assert result.sgst is None
    assert result.cgst is None


def test_sgst_cgst_only_is_left_alone():
    result = _normalize_result(_r(sgst=90.0, cgst=90.0))
    assert result.igst is None
    assert result.sgst == 90.0
    assert result.cgst == 90.0


def test_igst_and_sgst_cgst_both_present_igst_wins():
    """Ambiguous input — the business rule is 'inter-state takes precedence'."""
    result = _normalize_result(_r(igst=180.0, sgst=90.0, cgst=90.0))
    assert result.igst == 180.0
    assert result.sgst is None
    assert result.cgst is None


def test_igst_present_clears_null_sgst_cgst():
    # Even when SGST/CGST are already null, the branch is exercised.
    result = _normalize_result(_r(igst=18.0))
    assert result.sgst is None
    assert result.cgst is None


# ---- Rule 1b: infer from lump-sum total_gst ---------------------------------

def test_total_gst_with_same_state_gstins_splits_into_cgst_sgst():
    result = _normalize_result(_r(
        total_gst=180.0,
        vendor_gstin="27AAAAA0000A1Z5",
        buyer_gstin="27BBBBB1111B2Z4",
    ))
    assert result.cgst == 90.0
    assert result.sgst == 90.0
    assert result.igst is None


def test_total_gst_with_odd_amount_splits_to_sum_exactly():
    result = _normalize_result(_r(
        total_gst=181.01,
        vendor_gstin="27AAAAA0000A1Z5",
        buyer_gstin="27BBBBB1111B2Z4",
    ))
    # The exact split depends on Python's rounding, but the invariant is that
    # cgst + sgst == total_gst exactly (any odd penny absorbed by whichever
    # half the implementation chooses).
    assert result.cgst is not None and result.sgst is not None
    assert round(result.cgst + result.sgst, 2) == 181.01


def test_total_gst_with_different_state_gstins_becomes_igst():
    result = _normalize_result(_r(
        total_gst=180.0,
        vendor_gstin="27AAAAA0000A1Z5",
        buyer_gstin="33BBBBB1111B2Z4",
    ))
    assert result.igst == 180.0
    assert result.sgst is None
    assert result.cgst is None


def test_total_gst_without_gstins_is_not_inferred():
    """Missing GSTINs should leave the split to the human reviewer, not guess."""
    result = _normalize_result(_r(total_gst=180.0))
    assert result.sgst is None
    assert result.cgst is None
    assert result.igst is None


def test_total_gst_ignored_when_explicit_breakdown_exists():
    """If the doc already shows sgst+cgst, don't double-count from total_gst."""
    result = _normalize_result(_r(
        total_gst=200.0,
        sgst=90.0,
        cgst=90.0,
        vendor_gstin="27AAAAA0000A1Z5",
        buyer_gstin="27BBBBB1111B2Z4",
    ))
    assert result.sgst == 90.0
    assert result.cgst == 90.0


# ---- Rules 2 & 3: line item tax_rate defaulting ------------------------------

def test_line_items_inherit_doc_level_gst_rate_when_none_have_individual():
    result = _normalize_result(_r(
        gst_rate=18.0,
        line_items=[
            LineItem(description="A", amount=100.0),
            LineItem(description="B", amount=200.0),
        ],
    ))
    assert result.line_items[0].tax_rate == 18.0
    assert result.line_items[1].tax_rate == 18.0


def test_line_items_missing_rate_default_to_zero_when_some_have_individual():
    """Rule 2: once ANY line has its own rate, don't fan out the doc rate —
    the unmarked lines genuinely have no tax (e.g. non-taxable goods)."""
    result = _normalize_result(_r(
        gst_rate=18.0,
        line_items=[
            LineItem(description="Taxable", amount=100.0, tax_rate=18.0),
            LineItem(description="Exempt", amount=50.0),
        ],
    ))
    assert result.line_items[0].tax_rate == 18.0
    assert result.line_items[1].tax_rate == 0.0


def test_line_items_with_no_doc_rate_and_no_line_rate_default_to_zero():
    result = _normalize_result(_r(
        line_items=[LineItem(description="X", amount=100.0)],
    ))
    assert result.line_items[0].tax_rate == 0.0


# ---- Rule 4: PO has no invoice_number ---------------------------------------

def test_purchase_order_invoice_number_is_cleared():
    result = _normalize_result(_r(
        doc_type=DocumentType.PURCHASE_ORDER,
        invoice_number="SHOULD_NOT_BE_HERE",
        po_number="PO-1",
    ))
    assert result.invoice_number is None
    assert result.po_number == "PO-1"


def test_invoice_keeps_its_invoice_number():
    result = _normalize_result(_r(
        doc_type=DocumentType.INVOICE,
        invoice_number="INV-42",
    ))
    assert result.invoice_number == "INV-42"


# ---- Gap I: _result_to_draft gst_type derivation ----------------------------


def test_result_to_draft_gst_type_inter_state_when_igst_present():
    draft = _result_to_draft(_r(igst=180.0))
    assert draft["gst_type"] == "INTER_STATE"


def test_result_to_draft_gst_type_intra_state_when_sgst_cgst_present():
    draft = _result_to_draft(_r(sgst=90.0, cgst=90.0))
    assert draft["gst_type"] == "INTRA_STATE"


def test_result_to_draft_gst_type_none_when_no_gst_components():
    draft = _result_to_draft(_r())
    assert draft["gst_type"] is None
