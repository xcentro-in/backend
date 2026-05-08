from __future__ import annotations

from infrastructure import invoice2data_wrapper as wrapper


class _FakeTemplate:
    def __init__(self, raw: dict):
        self._raw = raw

    def matches_input(self, _raw_ocr: str) -> bool:
        return True

    def prepare_input(self, raw_ocr: str) -> str:
        return raw_ocr

    def extract(self, _optimized: str, _invoice_file, _input_module):
        return dict(self._raw)


def test_verify_template_roundtrip_requires_all_visible_critical_fields(monkeypatch):
    monkeypatch.setattr(
        wrapper,
        "read_templates",
        lambda _path: [_FakeTemplate({"invoice_number": "INV-42", "amount": "1180.00"})],
    )

    ok, reason = wrapper.verify_template_roundtrip(
        "issuer: x\nkeywords: []\n",
        (
            "TAX INVOICE\n"
            "Invoice No: INV-42\n"
            "PO Number: PO-7\n"
            "Vendor GSTIN: 27AADCB2230M1ZV\n"
            "Buyer GSTIN: 29ABCDE1234F1Z9\n"
            "Grand Total: 1180.00\n"
        ),
        {
            "invoice_number": "INV-42",
            "po_number": "PO-7",
            "vendor_gstin": "27AADCB2230M1ZV",
            "buyer_gstin": "29ABCDE1234F1Z9",
            "total_amount": 1180.0,
        },
    )

    assert ok is False
    assert "required_fields_failed_roundtrip" in reason
    assert "po_number missing" in reason
    assert "vendor_gstin missing" in reason
    assert "buyer_gstin missing" in reason


def test_verify_template_roundtrip_rejects_overmatched_scalar_lists(monkeypatch):
    monkeypatch.setattr(
        wrapper,
        "read_templates",
        lambda _path: [_FakeTemplate({"invoice_number": ["INVOICE", "INV-42"], "amount": "100.00"})],
    )

    ok, reason = wrapper.verify_template_roundtrip(
        "issuer: x\nkeywords: []\n",
        "TAX INVOICE\nInvoice No: INV-42\nGrand Total: 100.00\n",
        {
            "invoice_number": "INV-42",
            "total_amount": 100.0,
        },
    )

    assert ok is False
    assert "invoice_number overmatched" in reason


def test_verify_template_roundtrip_accepts_complete_critical_matches(monkeypatch):
    monkeypatch.setattr(
        wrapper,
        "read_templates",
        lambda _path: [_FakeTemplate({
            "invoice_number": "INV-42",
            "po_number": "PO-7",
            "vendor_gstin": "27AADCB2230M1ZV",
            "buyer_gstin": "29ABCDE1234F1Z9",
            "amount": "1180.00",
        })],
    )

    ok, reason = wrapper.verify_template_roundtrip(
        "issuer: x\nkeywords: []\n",
        (
            "TAX INVOICE\n"
            "Invoice No: INV-42\n"
            "PO Number: PO-7\n"
            "Vendor GSTIN: 27AADCB2230M1ZV\n"
            "Buyer GSTIN: 29ABCDE1234F1Z9\n"
            "Grand Total: 1180.00\n"
        ),
        {
            "invoice_number": "INV-42",
            "po_number": "PO-7",
            "vendor_gstin": "27AADCB2230M1ZV",
            "buyer_gstin": "29ABCDE1234F1Z9",
            "total_amount": 1180.0,
        },
    )

    assert ok is True
    assert reason == "ok"


# ---------------------------------------------------------------------------
# Fix 5: date round-trip validation
# ---------------------------------------------------------------------------

def test_verify_roundtrip_rejects_wrong_extracted_date(monkeypatch):
    """If the extracted date differs from target invoice_date, roundtrip fails."""
    monkeypatch.setattr(
        wrapper,
        "read_templates",
        lambda _path: [_FakeTemplate({
            "invoice_number": "INV-1",
            "date": "2024-01-15",  # extracted date
            "amount": "500.00",
        })],
    )
    ok, reason = wrapper.verify_template_roundtrip(
        "issuer: x\nkeywords: []\n",
        "Invoice No: INV-1\nInvoice Date: 2024-03-20\nTotal: 500.00\n",
        {
            "invoice_number": "INV-1",
            "invoice_date": "2024-03-20",
            "total_amount": 500.0,
        },
    )
    assert ok is False
    assert "date_mismatch" in reason or "date" in reason


def test_verify_roundtrip_accepts_date_matching_invoice_date(monkeypatch):
    """Roundtrip passes when extracted date matches target invoice_date."""
    monkeypatch.setattr(
        wrapper,
        "read_templates",
        lambda _path: [_FakeTemplate({
            "invoice_number": "INV-1",
            "date": "2024-03-20",
            "amount": "500.00",
        })],
    )
    ok, reason = wrapper.verify_template_roundtrip(
        "issuer: x\nkeywords: []\n",
        "Invoice No: INV-1\nInvoice Date: 2024-03-20\nTotal: 500.00\n",
        {
            "invoice_number": "INV-1",
            "invoice_date": "2024-03-20",
            "total_amount": 500.0,
        },
    )
    assert ok is True


def test_verify_roundtrip_accepts_date_alternate_format(monkeypatch):
    """Date in DD/MM/YYYY format that parses to same day as ISO target is ok."""
    monkeypatch.setattr(
        wrapper,
        "read_templates",
        lambda _path: [_FakeTemplate({
            "invoice_number": "INV-1",
            # dateparser can parse "20/03/2024"
            "date": "20/03/2024",
            "amount": "500.00",
        })],
    )
    ok, reason = wrapper.verify_template_roundtrip(
        "issuer: x\nkeywords: []\n",
        "Invoice No: INV-1\nDate: 20/03/2024\nTotal: 500.00\n",
        {
            "invoice_number": "INV-1",
            "invoice_date": "2024-03-20",
            "total_amount": 500.0,
        },
    )
    assert ok is True


# ---------------------------------------------------------------------------
# Fix 4: narrow_template_on_overcapture
# ---------------------------------------------------------------------------

import yaml

from infrastructure.invoice2data_wrapper import (
    _LINE_WRAP_CONTINUATION,
    narrow_template_on_overcapture,
)


def _make_widened_yaml(invoice_number_regex: str) -> str:
    """Build a minimal template YAML with the given invoice_number regex."""
    data = {
        "issuer": "Test Layout",
        "keywords": ["TAX INVOICE"],
        "fields": {
            "invoice_number": invoice_number_regex,
        },
        "options": {"currency": "INR"},
    }
    return yaml.dump(data, default_flow_style=False, allow_unicode=True, sort_keys=False)


def test_narrow_template_returns_none_when_no_continuation_present(monkeypatch):
    """If no line-wrap continuation is in the YAML, narrowing returns None."""
    monkeypatch.setattr(
        wrapper,
        "read_templates",
        lambda _path: [_FakeTemplate({"invoice_number": "INV-1", "amount": "100.00"})],
    )
    yaml_content = _make_widened_yaml(
        "(?si)Invoice\\s*No[.:]?\\s*(?P<invoice_number>[A-Z0-9][A-Z0-9/_-]+)"
    )
    result = narrow_template_on_overcapture(
        yaml_content,
        "Invoice No: INV-1 2025-08-03\nTotal: 100.00",
        {"invoice_number": "INV-1", "total_amount": 100.0},
    )
    assert result is None


def test_narrow_template_returns_corrected_yaml_when_overcapture_fixed(monkeypatch):
    """When the continuation is present and its removal makes roundtrip pass,
    narrow_template_on_overcapture returns the fixed YAML."""
    # Template whose invoice_number has the line-wrap continuation widening.
    widened_regex = (
        "(?si)Invoice\\s*No[.:]?\\s*(?P<invoice_number>[A-Z0-9][A-Z0-9/_-]+"
        + _LINE_WRAP_CONTINUATION
        + ")"
    )

    # First read_templates call (within narrow_template_on_overcapture's
    # verify_template_roundtrip) must return the narrowed template (no
    # continuation), producing only the invoice number without the date.
    def _fake_read(path: str):
        content = open(__import__("os").path.join(path, "candidate.yml")).read()
        parsed_yaml = yaml.safe_load(content)
        regex = parsed_yaml.get("fields", {}).get("invoice_number", "")
        # Simulate extraction: if continuation is gone, return clean value
        if _LINE_WRAP_CONTINUATION not in regex:
            return [_FakeTemplate({"invoice_number": "INV/2024/002", "amount": "1180.00"})]
        # With continuation still present, return the overcaptured value
        return [_FakeTemplate({"invoice_number": "INV/2024/002 2025-08-03", "amount": "1180.00"})]

    monkeypatch.setattr(wrapper, "read_templates", _fake_read)

    yaml_content = _make_widened_yaml(widened_regex)
    ocr = "TAX INVOICE\nInvoice No: INV/2024/002 2025-08-03\nTotal: 1,180.00"
    target = {"invoice_number": "INV/2024/002", "total_amount": 1180.0}

    result = narrow_template_on_overcapture(yaml_content, ocr, target)
    assert result is not None, "should return corrected YAML"
    assert _LINE_WRAP_CONTINUATION not in result, "corrected YAML must not contain continuation"


def test_narrow_template_returns_none_when_narrowing_does_not_help(monkeypatch):
    """If stripping the continuation still leaves the template broken (wrong
    anchor, etc.), narrow_template_on_overcapture returns None."""
    widened_regex = (
        "(?si)Invoice\\s*No[.:]?\\s*(?P<invoice_number>[A-Z0-9][A-Z0-9/_-]+"
        + _LINE_WRAP_CONTINUATION
        + ")"
    )

    # Simulate a template that always returns a wrong value regardless of narrowing
    monkeypatch.setattr(
        wrapper,
        "read_templates",
        lambda _path: [_FakeTemplate({"invoice_number": "WRONG-VALUE", "amount": "100.00"})],
    )

    yaml_content = _make_widened_yaml(widened_regex)
    ocr = "Invoice No: INV-CORRECT 2025-01-01\nTotal: 100.00"
    target = {"invoice_number": "INV-CORRECT", "total_amount": 100.0}

    result = narrow_template_on_overcapture(yaml_content, ocr, target)
    assert result is None, "should return None when narrowing does not fix the mismatch"
