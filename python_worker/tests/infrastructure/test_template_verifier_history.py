"""Tests for verify_template_against_history — the cross-document
generalisation check that runs a freshly-generated template against prior
validated documents from the same vendor."""

from __future__ import annotations

from infrastructure import invoice2data_wrapper as wrapper


class _FakeTemplate:
    """Stub for invoice2data Template with configurable per-OCR responses."""

    def __init__(self, responses: dict[str, dict]):
        # Map OCR text -> dict to return from extract().
        self._responses = responses

    def matches_input(self, ocr: str) -> bool:
        return ocr in self._responses

    def prepare_input(self, ocr: str) -> str:
        return ocr

    def extract(self, optimized: str, _file, _input_module):
        return dict(self._responses.get(optimized, {}))


def test_history_verifier_skips_when_no_history():
    ok, reason = wrapper.verify_template_against_history(
        "issuer: x\nkeywords: []\n",
        history=[],
    )
    assert ok is True
    assert "no_history" in reason


def test_history_verifier_passes_when_all_docs_roundtrip(monkeypatch):
    responses = {
        "OCR-A Invoice No: A1 Total: 100.00": {"invoice_number": "A1", "amount": "100.00"},
        "OCR-B Invoice No: B2 Total: 200.00": {"invoice_number": "B2", "amount": "200.00"},
    }
    monkeypatch.setattr(
        wrapper, "read_templates", lambda _path: [_FakeTemplate(responses)],
    )

    history = [
        {
            "raw_ocr": "OCR-A Invoice No: A1 Total: 100.00",
            "validated_data": {"invoice_number": "A1", "total_amount": 100.0},
        },
        {
            "raw_ocr": "OCR-B Invoice No: B2 Total: 200.00",
            "validated_data": {"invoice_number": "B2", "total_amount": 200.0},
        },
    ]
    ok, reason = wrapper.verify_template_against_history(
        "issuer: x\nkeywords: []\n", history=history,
    )
    assert ok is True
    assert reason == "ok"


def test_history_verifier_fails_when_one_doc_breaks(monkeypatch):
    """Template handles doc A correctly but extracts the wrong invoice number
    on doc B — a real overfit-to-source failure mode."""
    responses = {
        "OCR-A Invoice No: A1 Total: 100.00": {"invoice_number": "A1", "amount": "100.00"},
        "OCR-B Invoice No: B2 Total: 200.00": {"invoice_number": "WRONG", "amount": "200.00"},
    }
    monkeypatch.setattr(
        wrapper, "read_templates", lambda _path: [_FakeTemplate(responses)],
    )

    history = [
        {
            "raw_ocr": "OCR-A Invoice No: A1 Total: 100.00",
            "validated_data": {"invoice_number": "A1", "total_amount": 100.0},
        },
        {
            "raw_ocr": "OCR-B Invoice No: B2 Total: 200.00",
            "validated_data": {"invoice_number": "B2", "total_amount": 200.0},
        },
    ]
    ok, reason = wrapper.verify_template_against_history(
        "issuer: x\nkeywords: []\n", history=history,
    )
    assert ok is False
    assert "history_roundtrip_failed" in reason
    assert "history[1]" in reason
