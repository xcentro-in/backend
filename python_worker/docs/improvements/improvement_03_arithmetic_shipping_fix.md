# Improvement Plan 03 — Fix Arithmetic Validation to Include Shipping

**Priority:** High (Correctness Bug)  
**Effort:** Low (< 1 hour)  
**Impact:** Eliminates false validation failures on invoices with freight/shipping charges  
**Status:** IMPLEMENTED — see `usecases/validation.py`

---

## Problem

The arithmetic check in `check_arithmetic()` computes:

```python
computed_total = Σ(item.quantity × item.unit_price + item.tax)
computed_total -= discount
diff = |computed_total - result.total_amount|
```

But `total_amount` is defined as the **grand total the buyer must pay**, which includes shipping:

```
total_amount = subtotal + taxes - discount + shipping
```

If an invoice has a shipping charge (e.g., ₹200 freight), the check computes:
```
computed = ₹10,000 (line items + tax) - ₹0 (no discount) = ₹10,000
declared = ₹10,200 (includes ₹200 shipping)
diff = ₹200 >> ₹0.50 tolerance → FALSE FAILURE
```

This means every invoice with shipping charges fails arithmetic validation, triggering LLM fallback even when the template extraction was perfect — wasting LLM API calls and slowing processing.

---

## Root Cause

The `check_arithmetic` function was not updated when `shipping` was added to the `ExtractionResult` entity.

---

## Fix

```python
def check_arithmetic(result: ExtractionResult) -> list[str]:
    """Verify that line-item totals sum to the declared total_amount."""
    failures: list[str] = []

    if result.total_amount is None or not result.line_items:
        return failures

    computed_total = 0.0
    skipped = 0
    for item in result.line_items:
        item_total = _line_item_total(item)
        if item_total is None:
            skipped += 1
            continue
        computed_total += item_total

    if skipped == len(result.line_items):
        return failures

    # Account for discount
    abs_discount = _compute_discount(computed_total, result.discount, result.discount_type)
    computed_total -= abs_discount

    # Account for shipping charges
    computed_total += (result.shipping or 0.0)  # ← THIS IS THE FIX

    diff = abs(computed_total - result.total_amount)
    if diff > _ARITHMETIC_TOLERANCE:
        failures.append(
            f"Arithmetic mismatch: computed {computed_total:.2f} vs declared {result.total_amount:.2f} "
            f"(diff={diff:.2f})"
        )

    return failures
```

---

## Tests Added

```python
def test_arithmetic_with_shipping_included_in_total():
    """Shipping must be added to the computed total before comparing."""
    result = _invoice_result(
        total_amount=10200.0,
        line_items=[LineItem(quantity=10.0, unit_price=1000.0, amount=10000.0)],
        shipping=200.0,
    )
    assert check_arithmetic(result) == []


def test_arithmetic_flags_mismatch_even_with_shipping():
    """Shipping is accounted for, but a genuine mismatch is still flagged."""
    result = _invoice_result(
        total_amount=9999.0,  # wrong — should be 10200
        line_items=[LineItem(quantity=10.0, unit_price=1000.0, amount=10000.0)],
        shipping=200.0,
    )
    failures = check_arithmetic(result)
    assert len(failures) == 1
    assert "10200.00" in failures[0]
```

---

## Impact

- Eliminates false arithmetic failures on invoices with freight/shipping charges.
- Prevents unnecessary LLM fallback (and API cost) for correctly-extracted invoices.
- Particularly relevant for B2B invoices which commonly include courier/freight line items.
