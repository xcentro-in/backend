"""Validation logic for extracted invoice / purchase order data.

Each check is a pure function that returns a list of failure descriptions.
An empty list means the check passed.
"""

from __future__ import annotations

import re
from domain.entities import ExtractionResult, LineItem
from domain.enums import DocumentType

# 15-character alphanumeric Indian GSTIN pattern
_GSTIN_RE = re.compile(
    r"^[0-9]{2}[A-Z]{5}[0-9]{4}[A-Z]{1}[A-Z0-9]{1}Z[A-Z0-9]{1}$"
)

# Mandatory fields per document type
_INVOICE_REQUIRED = ["invoice_number", "invoice_date", "total_amount", "vendor_name", "line_items"]
_PO_REQUIRED = ["po_number", "vendor_name", "total_amount", "line_items"]

# Tolerance for floating-point arithmetic comparisons (in currency units)
_ARITHMETIC_TOLERANCE = 0.50


def check_required_fields(result: ExtractionResult) -> list[str]:
    """Verify that mandatory fields are present based on document type."""
    failures: list[str] = []
    required = _INVOICE_REQUIRED if result.doc_type == DocumentType.INVOICE else _PO_REQUIRED

    for field_name in required:
        value = getattr(result, field_name, None)
        if value is None or (isinstance(value, list) and len(value) == 0):
            failures.append(f"Missing required field: {field_name}")

    return failures


def check_gstin(result: ExtractionResult) -> list[str]:
    """Validate GSTIN format for vendor and buyer (if present)."""
    failures: list[str] = []

    for label, gstin in [("vendor_gstin", result.vendor_gstin), ("buyer_gstin", result.buyer_gstin)]:
        if gstin is not None and not _GSTIN_RE.match(gstin.strip().upper()):
            masked = gstin[:5] + "***" if gstin and len(gstin) >= 5 else "***"
            failures.append(f"Invalid GSTIN format for {label}: {masked}")

    return failures


def _line_item_total(item: LineItem) -> tuple[float, bool] | None:
    """Compute expected amount for a single line item.

    Returns (total, tax_was_applied) or None when quantity/price are missing.
    tax_was_applied is True when the total includes a tax component so the
    caller knows not to also add top-level GST amounts.
    """
    if item.quantity is None or item.unit_price is None:
        return None
    base = item.quantity * item.unit_price
    if item.tax is not None:
        return base + item.tax, True
    if item.tax_rate is not None:
        return base + base * (item.tax_rate / 100.0), True
    return base, False


def _compute_discount(subtotal: float, discount: float | None, discount_type: str | None) -> float:
    """Return the absolute discount amount based on type.

    - FLAT: the discount value is already in currency units.
    - PERCENTAGE: discount is a rate (e.g. 10 means 10%), applied to the subtotal.
    - None / unknown: fall back to treating the value as a flat amount.
    """
    if discount is None:
        return 0.0

    if discount_type and discount_type.upper() == "PERCENTAGE":
        return subtotal * (discount / 100.0)

    # FLAT or unspecified — treat the value as an absolute amount
    return discount


def check_arithmetic(result: ExtractionResult) -> list[str]:
    """Verify that line-item totals sum to the declared total_amount."""
    failures: list[str] = []

    if result.total_amount is None or not result.line_items:
        return failures  # nothing to cross-check

    computed_total = 0.0
    skipped = 0
    any_tax_applied = False
    for item in result.line_items:
        ret = _line_item_total(item)
        if ret is None:
            skipped += 1
            continue
        item_total, tax_applied = ret
        computed_total += item_total
        if tax_applied:
            any_tax_applied = True

    # Only flag if we actually computed something meaningful
    if skipped == len(result.line_items):
        return failures

    # When no line item carried a tax component, add the document-level GST
    # totals so computed_total reflects the full invoice amount.
    if not any_tax_applied:
        computed_total += (result.igst or 0.0) + (result.cgst or 0.0) + (result.sgst or 0.0)
        computed_total += (result.total_gst or 0.0)

    # Account for discount (flat or percentage) before comparing
    abs_discount = _compute_discount(computed_total, result.discount, result.discount_type)
    computed_total -= abs_discount

    # Account for shipping/freight charges — total_amount is the grand total
    # the buyer must pay, which includes shipping on top of taxed line items.
    # Without this, any invoice with freight charges produces a false failure.
    computed_total += (result.shipping or 0.0)

    diff = abs(computed_total - result.total_amount)
    if diff > _ARITHMETIC_TOLERANCE:
        failures.append(
            f"Arithmetic mismatch: computed {computed_total:.2f} vs declared {result.total_amount:.2f} "
            f"(diff={diff:.2f})"
        )

    return failures


def run_all_checks(result: ExtractionResult, include_arithmetic: bool = True) -> list[str]:
    """Execute every validation check and return aggregated failures.

    Args:
        result: The extraction result to validate.
        include_arithmetic: If False, skip arithmetic checks (used to
            prevent arithmetic mismatches from triggering LLM fallback).
    """
    failures: list[str] = []
    failures.extend(check_required_fields(result))
    failures.extend(check_gstin(result))
    if include_arithmetic:
        failures.extend(check_arithmetic(result))
    return failures
