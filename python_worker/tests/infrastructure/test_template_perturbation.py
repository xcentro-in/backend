"""Tests for synthetic OCR perturbation — confirms that templates which only
work for the exact source OCR are rejected by perturb-and-replay."""

from __future__ import annotations

from infrastructure import invoice2data_wrapper as wrapper


def test_perturb_shifts_digits_preserves_length():
    text = "Invoice No: INV-12345 Total: 1000.00"
    perturbations = wrapper.perturb_ocr_for_robustness(text)
    labels = [label for label, _ in perturbations]
    assert "digit_shift" in labels
    shifted = next(p for label, p in perturbations if label == "digit_shift")
    # Letters are untouched; digits all changed; total length preserved.
    assert len(shifted) == len(text)
    assert "INV-" in shifted
    assert "12345" not in shifted


def test_perturb_no_digits_returns_no_digit_shift():
    text = "Invoice No: ABC Total: AB.CD"
    perturbations = wrapper.perturb_ocr_for_robustness(text)
    labels = [label for label, _ in perturbations]
    assert "digit_shift" not in labels


def test_perturb_collapses_extra_spaces():
    text = "Item       Qty     Rate     Amount"  # 7 spaces between cols
    perturbations = wrapper.perturb_ocr_for_robustness(text)
    labels = [label for label, _ in perturbations]
    assert "whitespace_collapse" in labels
    collapsed = next(p for label, p in perturbations if label == "whitespace_collapse")
    assert "       " not in collapsed
    assert "  " in collapsed


def test_perturb_empty_returns_empty():
    assert wrapper.perturb_ocr_for_robustness("") == []
    assert wrapper.perturb_ocr_for_robustness("   \n  ") == []


class _StaticFakeTemplate:
    """Stub template whose extract result depends on the OCR — used to model
    a template that overfits the source: it only matches and extracts the
    exact source OCR; all perturbed variants fail."""

    def __init__(self, accepted_ocr: str, response: dict):
        self._accepted = accepted_ocr
        self._response = response

    def matches_input(self, ocr: str) -> bool:
        return ocr == self._accepted

    def prepare_input(self, ocr: str) -> str:
        return ocr

    def extract(self, optimized: str, _file, _input_module):
        if optimized == self._accepted:
            return dict(self._response)
        return None


def test_perturbation_verifier_rejects_overfit_template(monkeypatch):
    source = "Invoice No: INV-99 Total: 500.00"
    monkeypatch.setattr(
        wrapper, "read_templates",
        lambda _path: [_StaticFakeTemplate(source, {"invoice_number": "INV-99", "amount": "500.00"})],
    )

    ok, reason = wrapper.verify_template_against_perturbations(
        "issuer: x\nkeywords: []\n", source,
    )
    # The fake template only matches the exact source OCR, so each perturbed
    # variant trips the keywords-no-longer-match arm of the verifier.
    assert ok is False
    assert "perturbation_failed" in reason


class _PermissiveFakeTemplate:
    """Stub that matches and extracts SOMETHING for every OCR variant."""

    def matches_input(self, _ocr: str) -> bool:
        return True

    def prepare_input(self, ocr: str) -> str:
        return ocr

    def extract(self, optimized: str, _file, _input_module):
        return {"invoice_number": "anything", "amount": "1.00"}


def test_perturbation_verifier_accepts_robust_template(monkeypatch):
    monkeypatch.setattr(
        wrapper, "read_templates", lambda _path: [_PermissiveFakeTemplate()],
    )
    ok, reason = wrapper.verify_template_against_perturbations(
        "issuer: x\nkeywords: []\n",
        "Invoice No: INV-1 Total: 99.99",
    )
    assert ok is True
    assert reason == "ok"
