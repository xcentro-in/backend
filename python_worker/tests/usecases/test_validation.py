"""Tests for the validation checks used to gate template-vs-LLM routing.

Every check is pure: it takes an ExtractionResult and returns a list of
failure descriptions. We table-drive through representative cases rather
than enumerating every field, focusing on the shapes that actually change
routing behavior at [extraction_service.py](../../usecases/extraction_service.py).
"""

from __future__ import annotations

import dataclasses

import pytest

from domain.entities import ExtractionResult, LineItem
from domain.enums import DocumentType
from usecases.validation import (
    check_arithmetic,
    check_gstin,
    check_required_fields,
    run_all_checks,
)


def _invoice_result(**overrides) -> ExtractionResult:
    """Build a complete, valid INVOICE-type ExtractionResult."""
    base = ExtractionResult(
        invoice_number="INV-1",
        invoice_date="2026-04-20",
        vendor_name="Acme",
        total_amount=1180.0,
        line_items=[LineItem(quantity=1.0, unit_price=1000.0, tax=180.0, amount=1180.0)],
        doc_type=DocumentType.INVOICE,
    )
    return dataclasses.replace(base, **overrides)


def _po_result(**overrides) -> ExtractionResult:
    """Build a complete, valid PURCHASE_ORDER-type ExtractionResult."""
    base = ExtractionResult(
        po_number="PO-1",
        vendor_name="Acme",
        total_amount=500.0,
        line_items=[LineItem(quantity=1.0, unit_price=500.0, amount=500.0)],
        doc_type=DocumentType.PURCHASE_ORDER,
    )
    return dataclasses.replace(base, **overrides)


# ---------------------------------------------------------------------------
# check_required_fields
# ---------------------------------------------------------------------------


@pytest.mark.parametrize("missing", ["invoice_number", "invoice_date", "total_amount", "vendor_name"])
def test_required_invoice_field_missing_is_flagged(missing):
    result = _invoice_result(**{missing: None})

    failures = check_required_fields(result)

    assert any(missing in f for f in failures)


@pytest.mark.parametrize("missing", ["po_number", "vendor_name", "total_amount"])
def test_required_po_field_missing_is_flagged(missing):
    result = _po_result(**{missing: None})

    failures = check_required_fields(result)

    assert any(missing in f for f in failures)


def test_empty_line_items_counted_as_missing():
    result = _invoice_result(line_items=[])

    failures = check_required_fields(result)

    assert any("line_items" in f for f in failures)


def test_complete_invoice_passes_required_check():
    assert check_required_fields(_invoice_result()) == []


def test_complete_po_passes_required_check():
    assert check_required_fields(_po_result()) == []


# ---------------------------------------------------------------------------
# check_gstin
# ---------------------------------------------------------------------------


@pytest.mark.parametrize("gstin", [
    "27AADCB2230M1ZV",
    "29ABCDE1234F1Z5",
    "06AAACI1681G1ZM",
])
def test_valid_gstin_passes(gstin):
    result = _invoice_result(vendor_gstin=gstin, buyer_gstin=gstin)

    assert check_gstin(result) == []


@pytest.mark.parametrize("bad_gstin", [
    "27AADCB2230M1Z",   # too short
    "27AADCB2230M1ZVV", # too long
    "27aadcb2230m1zv",  # lowercase is handled by .upper() — but if the first two
                        # chars aren't digits anymore after upper, rejected
    "AAAADCB2230M1ZV",  # non-digit state code
    "27AADCB2230M1AV",  # missing Z in the 14th position
    "1234567890abcde",  # random garbage
])
def test_invalid_gstin_is_flagged(bad_gstin):
    # lowercase valid will PASS because validator upper-cases; skip that shape.
    if bad_gstin == "27aadcb2230m1zv":
        pytest.skip("validator upper-cases input so this shape is valid")
    result = _invoice_result(vendor_gstin=bad_gstin)

    failures = check_gstin(result)

    assert any("vendor_gstin" in f for f in failures)


def test_lowercase_gstin_is_normalized_and_passes():
    result = _invoice_result(vendor_gstin="27aadcb2230m1zv")

    assert check_gstin(result) == []


def test_gstin_with_surrounding_whitespace_is_stripped():
    result = _invoice_result(vendor_gstin="  27AADCB2230M1ZV  ")

    assert check_gstin(result) == []


def test_none_gstin_is_skipped():
    result = _invoice_result(vendor_gstin=None, buyer_gstin=None)

    assert check_gstin(result) == []


# ---------------------------------------------------------------------------
# check_arithmetic
# ---------------------------------------------------------------------------


def test_arithmetic_exact_match_passes():
    result = _invoice_result(
        total_amount=2000.0,
        line_items=[
            LineItem(quantity=2.0, unit_price=500.0, amount=1000.0),
            LineItem(quantity=5.0, unit_price=200.0, amount=1000.0),
        ],
    )

    assert check_arithmetic(result) == []


def test_arithmetic_within_tolerance_passes():
    # Computed = 1000.30; declared = 1000.00; diff = 0.30 < 0.50 tolerance.
    result = _invoice_result(
        total_amount=1000.0,
        line_items=[LineItem(quantity=1.0, unit_price=1000.30, amount=1000.30)],
    )

    assert check_arithmetic(result) == []


def test_arithmetic_beyond_tolerance_is_flagged():
    result = _invoice_result(
        total_amount=1000.0,
        line_items=[LineItem(quantity=1.0, unit_price=1500.0, amount=1500.0)],
    )

    failures = check_arithmetic(result)

    assert len(failures) == 1
    assert "1500.00" in failures[0]
    assert "1000.00" in failures[0]


def test_arithmetic_short_circuits_when_no_total():
    result = _invoice_result(total_amount=None)

    assert check_arithmetic(result) == []


def test_arithmetic_short_circuits_when_no_line_items():
    result = _invoice_result(line_items=[])

    assert check_arithmetic(result) == []


def test_arithmetic_skips_incomplete_line_items():
    # Only the second item has quantity + unit_price; the first is dropped.
    # Computed = 1.0 * 500.0 = 500.0. Declared = 500.0. Passes.
    result = _invoice_result(
        total_amount=500.0,
        line_items=[
            LineItem(description="half-baked", quantity=None, unit_price=500.0),
            LineItem(quantity=1.0, unit_price=500.0, amount=500.0),
        ],
    )

    assert check_arithmetic(result) == []


def test_arithmetic_all_incomplete_line_items_is_noop():
    result = _invoice_result(
        total_amount=999.0,
        line_items=[
            LineItem(quantity=None, unit_price=100.0),
            LineItem(quantity=1.0, unit_price=None),
        ],
    )

    assert check_arithmetic(result) == []


def test_arithmetic_with_flat_discount():
    # Subtotal = 1000; flat discount 50; expected net = 950 == declared.
    result = _invoice_result(
        total_amount=950.0,
        line_items=[LineItem(quantity=1.0, unit_price=1000.0, amount=1000.0)],
        discount=50.0,
        discount_type="FLAT",
    )

    assert check_arithmetic(result) == []


def test_arithmetic_with_percentage_discount():
    # Subtotal = 1000; 10% discount = 100; net = 900 == declared.
    result = _invoice_result(
        total_amount=900.0,
        line_items=[LineItem(quantity=1.0, unit_price=1000.0, amount=1000.0)],
        discount=10.0,
        discount_type="PERCENTAGE",
    )

    assert check_arithmetic(result) == []


def test_arithmetic_with_line_item_tax_included():
    # Subtotal = 1000; tax = 180; net = 1180 == declared.
    result = _invoice_result(
        total_amount=1180.0,
        line_items=[LineItem(quantity=1.0, unit_price=1000.0, tax=180.0, amount=1180.0)],
    )

    assert check_arithmetic(result) == []


def test_arithmetic_unknown_discount_type_treated_as_flat():
    # discount_type="MYSTERY" should fall back to treating discount as flat.
    result = _invoice_result(
        total_amount=950.0,
        line_items=[LineItem(quantity=1.0, unit_price=1000.0, amount=1000.0)],
        discount=50.0,
        discount_type="MYSTERY",
    )

    assert check_arithmetic(result) == []


def test_arithmetic_with_shipping_included_in_total():
    # Invoices with freight/shipping charges add shipping to total_amount.
    # Computed = 10000 (line items) + 200 (shipping) = 10200 == declared.
    result = _invoice_result(
        total_amount=10200.0,
        line_items=[LineItem(quantity=10.0, unit_price=1000.0, amount=10000.0)],
        shipping=200.0,
    )

    assert check_arithmetic(result) == []


def test_arithmetic_flags_mismatch_even_after_shipping_accounted():
    # Shipping is included, but a genuine mismatch is still caught.
    result = _invoice_result(
        total_amount=9999.0,  # wrong — should be 10200
        line_items=[LineItem(quantity=10.0, unit_price=1000.0, amount=10000.0)],
        shipping=200.0,
    )

    failures = check_arithmetic(result)

    assert len(failures) == 1
    assert "10200.00" in failures[0]


def test_arithmetic_with_none_shipping_treated_as_zero():
    # When shipping is None (not provided), it should not affect the total.
    result = _invoice_result(
        total_amount=1000.0,
        line_items=[LineItem(quantity=1.0, unit_price=1000.0, amount=1000.0)],
        shipping=None,
    )

    assert check_arithmetic(result) == []


def test_arithmetic_with_line_item_tax_rate_no_tax():
    # tax_rate=12% on a 12245.16 subtotal → tax=1469.42 → total=13714.58.
    # Mirrors the real-world case where invoice2data extracts tax_rate but not tax.
    result = _invoice_result(
        total_amount=13714.58,
        line_items=[LineItem(quantity=3.0, unit_price=4081.72, tax_rate=12.0)],
    )

    assert check_arithmetic(result) == []


def test_arithmetic_with_top_level_igst_fallback():
    # No per-line tax or tax_rate; top-level igst carries the GST total.
    result = _invoice_result(
        total_amount=13714.58,
        line_items=[LineItem(quantity=3.0, unit_price=4081.72)],
        igst=1469.42,
    )

    assert check_arithmetic(result) == []


def test_arithmetic_top_level_gst_not_double_counted_when_per_line_tax_rate_set():
    # tax_rate is set on line items AND igst is also present at doc level.
    # Only per-line computation should be used — igst must not be added again.
    result = _invoice_result(
        total_amount=13714.58,
        line_items=[LineItem(quantity=3.0, unit_price=4081.72, tax_rate=12.0)],
        igst=1469.42,
    )

    assert check_arithmetic(result) == []


# ---------------------------------------------------------------------------
# run_all_checks
# ---------------------------------------------------------------------------


def test_run_all_checks_aggregates_failures():
    # Missing vendor_name + bad GSTIN + arithmetic mismatch.
    result = _invoice_result(
        vendor_name=None,
        vendor_gstin="not-a-gstin",
        total_amount=1.0,  # computed would be 1180.0
    )

    failures = run_all_checks(result)

    # All three categories must contribute at least one message.
    assert any("vendor_name" in f for f in failures)
    assert any("vendor_gstin" in f for f in failures)
    assert any("mismatch" in f.lower() or "computed" in f.lower() for f in failures)


def test_run_all_checks_include_arithmetic_false_suppresses_only_arithmetic():
    result = _invoice_result(
        vendor_gstin="not-a-gstin",
        total_amount=1.0,  # would normally fail arithmetic
    )

    failures = run_all_checks(result, include_arithmetic=False)

    # GSTIN failure present, arithmetic failure suppressed.
    assert any("vendor_gstin" in f for f in failures)
    assert not any("mismatch" in f.lower() or "computed" in f.lower() for f in failures)


def test_run_all_checks_valid_result_returns_empty():
    assert run_all_checks(_invoice_result()) == []
