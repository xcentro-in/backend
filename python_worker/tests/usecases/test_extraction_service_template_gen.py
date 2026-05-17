"""Tests for ExtractionService.generate_template_for_document.

Covers the HITL-feedback template-generation flow: skip-when-template-exists,
force_regen, missing inputs, generate→verify→retry-with-feedback, and the
filename-sanitization defenses.
"""

from __future__ import annotations

import uuid
from pathlib import Path

import pytest

from domain.enums import DocumentStatus, ProcessedBy
from domain.exceptions import LLMExtractionError, TemplateGenerationError
from usecases.extraction_service import ExtractionService, _sanitize_filename_component, _layout_slug_from_yaml


# ---------------------------------------------------------------------------
# LLM stubs
# ---------------------------------------------------------------------------


class _LLMStub:
    """LLM whose generate_template returns canned YAML, ignoring inputs."""

    def __init__(self, yamls: list[str] | None = None) -> None:
        # Default: a single canned response that's reused on every call.
        self._yamls = yamls or ["issuer: x\nkeywords: []\n"]
        self.calls: list[dict] = []

    def extract(self, file_path):
        raise AssertionError("extract should not be called from template gen")

    def generate_template(
        self, raw_ocr, target_data, historical_examples=None,
        prior_attempt=None, prior_error=None,
    ):
        self.calls.append({
            "raw_ocr": raw_ocr,
            "target_data": target_data,
            "historical_examples": historical_examples or [],
            "prior_attempt": prior_attempt,
            "prior_error": prior_error,
        })
        idx = min(len(self.calls) - 1, len(self._yamls) - 1)
        return self._yamls[idx]


class _LLMRaises:
    def __init__(self, exc: Exception) -> None:
        self._exc = exc

    def extract(self, file_path):
        raise AssertionError("not used")

    def generate_template(self, *args, **kwargs):
        raise self._exc


class _UnusedLLM:
    def extract(self, file_path):
        raise AssertionError("not used")

    def generate_template(self, *args, **kwargs):
        raise AssertionError(
            "generate_template should not be called when skipping existing template"
        )


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_doc(
    *,
    raw_ocr: str = "OCR TEXT",
    validated: dict | None = None,
    draft: dict | None = None,
    i2d_template: str | None = None,
    processed_by: ProcessedBy | None = None,
):
    """Return a Document populated for the template-gen happy path."""
    from domain.entities import Document

    return Document(
        id=uuid.uuid4(),
        s3_path="ignored",
        status=DocumentStatus.VALIDATED,
        raw_ocr=raw_ocr,
        validated_data=validated or {
            "vendor_gstin": "27AADCB2230M1ZV",
            "vendor_name": "Acme",
            "doc_type": "INVOICE",
            "invoice_number": "INV-1",
            "total_amount": 1180.0,
        },
        draft_data=draft,
        i2d_template=i2d_template,
        processed_by=processed_by,
    )


@pytest.fixture
def stub_verifiers(monkeypatch):
    """Make all verifier helpers deterministic per-test.

    Tests call ``stub_verifiers(roundtrip=, history=, perturbation=)`` with
    either a (bool, str) tuple or a list of tuples (one per call) to model
    sequence-dependent behaviour like first-fail-then-pass.
    """

    state: dict[str, object] = {
        "roundtrip": (True, "ok"),
        "history": (True, "ok"),
        "perturbation": (True, "ok"),
    }

    def _make_callable(key):
        def _next(*args, **kwargs):
            value = state[key]
            if isinstance(value, list):
                if len(value) == 1:
                    return value[0]
                return value.pop(0)
            return value

        return _next

    monkeypatch.setattr(
        "usecases.extraction_service.verify_template_roundtrip",
        _make_callable("roundtrip"),
    )
    monkeypatch.setattr(
        "usecases.extraction_service.verify_template_against_history",
        _make_callable("history"),
    )
    monkeypatch.setattr(
        "usecases.extraction_service.verify_template_against_perturbations",
        _make_callable("perturbation"),
    )
    monkeypatch.setattr(
        "usecases.extraction_service.postprocess_template_regexes",
        lambda yaml_content: yaml_content + "\n# postprocessed\n",
    )
    monkeypatch.setattr(
        "usecases.extraction_service._sanitize_keywords_to_ocr",
        lambda yaml_content, raw_ocr: yaml_content,
    )

    def configure(**overrides):
        state.update(overrides)

    return configure


def _service(repo, llm, *, template_repo=None, template_dir="/tmp/test_templates"):
    return ExtractionService(
        repo=repo,
        template_extractor=None,
        llm_extractor=llm,
        downloader=None,
        template_repo=template_repo,
        template_dir=template_dir,
    )


# ---------------------------------------------------------------------------
# 1-3. Input-missing failure modes (raise non-retryable)
# ---------------------------------------------------------------------------


def test_doc_not_found_raises_non_retryable(fake_document_repo, fake_template_repo):
    service = _service(fake_document_repo, _UnusedLLM(), template_repo=fake_template_repo)

    with pytest.raises(TemplateGenerationError) as exc_info:
        service.generate_template_for_document(uuid.uuid4())

    assert exc_info.value.reason == "not_found"
    assert exc_info.value.retryable is False


def test_missing_raw_ocr_raises_non_retryable(fake_document_repo, fake_template_repo):
    doc = _make_doc(raw_ocr="")
    fake_document_repo.add(doc)
    service = _service(fake_document_repo, _UnusedLLM(), template_repo=fake_template_repo)

    with pytest.raises(TemplateGenerationError) as exc_info:
        service.generate_template_for_document(doc.id)

    assert exc_info.value.reason == "missing_ocr"
    assert exc_info.value.retryable is False


def test_missing_target_data_raises_non_retryable(fake_document_repo, fake_template_repo):
    doc = _make_doc(validated=None, draft=None)
    # Strip the defaults so neither validated_data nor draft_data is set.
    doc.validated_data = None
    doc.draft_data = None
    fake_document_repo.add(doc)
    service = _service(fake_document_repo, _UnusedLLM(), template_repo=fake_template_repo)

    with pytest.raises(TemplateGenerationError) as exc_info:
        service.generate_template_for_document(doc.id)

    assert exc_info.value.reason == "missing_target_data"
    assert exc_info.value.retryable is False


# ---------------------------------------------------------------------------
# 4-5. Skip-existing vs. force-regen
# ---------------------------------------------------------------------------


def test_skip_existing_template_when_invoice2data_with_template(
    fake_document_repo, fake_template_repo,
):
    doc = _make_doc(
        i2d_template="acme.yml",
        processed_by=ProcessedBy.INVOICE2DATA,
    )
    fake_document_repo.add(doc)
    service = _service(fake_document_repo, _UnusedLLM(), template_repo=fake_template_repo)

    service.generate_template_for_document(doc.id, force_regen=False)

    assert fake_template_repo.save_calls == []


def test_force_regen_overrides_skip(
    fake_document_repo, fake_template_repo, stub_verifiers,
):
    doc = _make_doc(
        i2d_template="acme.yml",
        processed_by=ProcessedBy.INVOICE2DATA,
    )
    fake_document_repo.add(doc)
    llm = _LLMStub()
    service = _service(fake_document_repo, llm, template_repo=fake_template_repo)

    service.generate_template_for_document(doc.id, force_regen=True)

    assert len(llm.calls) == 1
    assert len(fake_template_repo.save_calls) == 1
    saved_name, _, _ = fake_template_repo.save_calls[0]
    # vendor_gstin lower-cased + sanitized + layout slug ("x") + doc_type suffix
    assert saved_name == "27aadcb2230m1zv_x_invoice.yml"


# ---------------------------------------------------------------------------
# 6. Happy path → save called with replaced_by="initial_or_correction"
# ---------------------------------------------------------------------------


def test_happy_path_saves_template_and_updates_doc(
    fake_document_repo, fake_template_repo, stub_verifiers,
):
    doc = _make_doc()
    fake_document_repo.add(doc)
    llm = _LLMStub(yamls=["issuer: acme\nkeywords: ['Acme']\n"])
    service = _service(fake_document_repo, llm, template_repo=fake_template_repo)

    service.generate_template_for_document(doc.id)

    assert len(llm.calls) == 1
    assert len(fake_template_repo.save_calls) == 1
    saved_name, saved_content, saved_issuer = fake_template_repo.save_calls[0]
    # Layout slug comes from the 'issuer' field in the LLM YAML ("acme").
    assert saved_name == "27aadcb2230m1zv_acme_invoice.yml"
    assert "postprocessed" in saved_content  # postprocess_template_regexes ran
    assert saved_issuer == "27AADCB2230M1ZV"
    # Doc updated with the generated template name
    final = fake_document_repo.get_by_id(doc.id)
    assert final.i2d_template == saved_name


# ---------------------------------------------------------------------------
# 7. First verify fails → retry succeeds
# ---------------------------------------------------------------------------


def test_first_verify_fails_then_retry_succeeds(
    fake_document_repo, fake_template_repo, stub_verifiers,
):
    doc = _make_doc()
    fake_document_repo.add(doc)
    llm = _LLMStub(yamls=["first attempt\n", "retry attempt\n"])
    service = _service(fake_document_repo, llm, template_repo=fake_template_repo)

    # First roundtrip call fails, second one (during retry) passes.
    stub_verifiers(roundtrip=[(False, "amount_mismatch"), (True, "ok")])

    service.generate_template_for_document(doc.id)

    assert len(llm.calls) == 2, "LLM should be called twice (initial + retry)"
    # Retry call must have been given the prior_attempt + prior_error context.
    assert llm.calls[1]["prior_attempt"] is not None
    assert llm.calls[1]["prior_error"] == "amount_mismatch"
    # The saved YAML is the postprocessed retry candidate.
    _, saved_content, _ = fake_template_repo.save_calls[0]
    assert "retry attempt" in saved_content


# ---------------------------------------------------------------------------
# 8. Both verify attempts fail → non-retryable, nothing saved
# ---------------------------------------------------------------------------


def test_both_verify_attempts_fail_raises_non_retryable(
    fake_document_repo, fake_template_repo, stub_verifiers,
):
    doc = _make_doc()
    fake_document_repo.add(doc)
    llm = _LLMStub(yamls=["a\n", "b\n"])
    service = _service(fake_document_repo, llm, template_repo=fake_template_repo)

    stub_verifiers(roundtrip=[(False, "first_fail"), (False, "second_fail")])

    with pytest.raises(TemplateGenerationError) as exc_info:
        service.generate_template_for_document(doc.id)

    assert exc_info.value.reason == "verification_failed"
    assert exc_info.value.retryable is False
    assert fake_template_repo.save_calls == [], "no template should be saved"


# ---------------------------------------------------------------------------
# 9. LLMExtractionError is wrapped as retryable TemplateGenerationError
# ---------------------------------------------------------------------------


def test_llm_extraction_error_wraps_as_retryable(
    fake_document_repo, fake_template_repo, stub_verifiers,
):
    doc = _make_doc()
    fake_document_repo.add(doc)
    service = _service(
        fake_document_repo,
        _LLMRaises(LLMExtractionError("Gemini API down")),
        template_repo=fake_template_repo,
    )

    with pytest.raises(TemplateGenerationError) as exc_info:
        service.generate_template_for_document(doc.id)

    assert exc_info.value.retryable is True
    assert "Gemini API down" in exc_info.value.reason


# ---------------------------------------------------------------------------
# 10. Path-traversal defense: filename sanitized regardless of vendor input
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    # _sanitize_filename_component lowercases, replaces non-[a-z0-9_-] with
    # underscore, then strips leading/trailing ._- — so "../etc/passwd"
    # becomes "___etc_passwd" → "etc_passwd". Either way the dots and slashes
    # are gone, which is the security-relevant assertion.
    "vendor_name, expected_filename",
    [
        ("../etc/passwd", "etc_passwd_x_invoice.yml"),
        ("..\\..\\windows", "windows_x_invoice.yml"),
        ("legit Vendor #1", "legit_vendor__1_x_invoice.yml"),
    ],
)
def test_filename_sanitization_handles_traversal_attempts(
    vendor_name, expected_filename,
    fake_document_repo, fake_template_repo, stub_verifiers,
):
    # No GSTIN → falls back to vendor_name, so we can drive the sanitizer.
    doc = _make_doc(validated={
        "vendor_name": vendor_name,
        "doc_type": "INVOICE",
        "invoice_number": "INV",
        "total_amount": 1.0,
    })
    fake_document_repo.add(doc)
    llm = _LLMStub()
    service = _service(fake_document_repo, llm, template_repo=fake_template_repo)

    service.generate_template_for_document(doc.id)

    saved_name, _, _ = fake_template_repo.save_calls[0]
    assert saved_name == expected_filename


def test_filesystem_fallback_blocks_path_traversal(
    fake_document_repo, stub_verifiers, tmp_path,
):
    """When no template_repo is wired, the filesystem fallback must keep the
    written file under template_dir even with hostile vendor input."""
    doc = _make_doc(validated={
        "vendor_name": "../../etc/passwd",
        "doc_type": "INVOICE",
        "invoice_number": "INV",
        "total_amount": 1.0,
    })
    fake_document_repo.add(doc)
    llm = _LLMStub()
    service = _service(
        fake_document_repo, llm, template_repo=None, template_dir=str(tmp_path),
    )

    service.generate_template_for_document(doc.id)

    written = list(tmp_path.iterdir())
    assert len(written) == 1
    # The resolved file is strictly inside tmp_path (would have raised
    # ExtractionError → wrapped as path_traversal_refused if it escaped).
    Path(written[0]).resolve().relative_to(tmp_path.resolve())


# ---------------------------------------------------------------------------
# 11. Vendor identity priority: vendor_gstin wins over vendor_name
# ---------------------------------------------------------------------------


def test_vendor_gstin_takes_priority_over_vendor_name(
    fake_document_repo, fake_template_repo, stub_verifiers,
):
    doc = _make_doc(validated={
        "vendor_gstin": "27AADCB2230M1ZV",
        "vendor_name": "fallback name should not be used",
        "doc_type": "PURCHASE_ORDER",
        "po_number": "PO-1",
        "total_amount": 1.0,
    })
    fake_document_repo.add(doc)
    llm = _LLMStub()
    service = _service(fake_document_repo, llm, template_repo=fake_template_repo)

    service.generate_template_for_document(doc.id)

    saved_name, _, _ = fake_template_repo.save_calls[0]
    # Layout slug "x" comes from the default _LLMStub YAML (issuer: x).
    assert saved_name == "27aadcb2230m1zv_x_purchase_order.yml"


# ---------------------------------------------------------------------------
# 12. Historical examples are forwarded into the LLM call
# ---------------------------------------------------------------------------


def test_historical_examples_forwarded_to_llm(
    fake_document_repo, fake_template_repo, stub_verifiers,
):
    doc = _make_doc(i2d_template="acme.yml")
    fake_document_repo.add(doc)
    fake_document_repo.validated_by_template["acme.yml"] = [
        {"invoice_number": f"PRIOR-{i}"} for i in range(3)
    ]
    llm = _LLMStub()
    service = _service(fake_document_repo, llm, template_repo=fake_template_repo,
                       )

    # force_regen=True bypasses the skip path so the LLM is invoked.
    service.generate_template_for_document(doc.id, force_regen=True)

    assert llm.calls
    forwarded = llm.calls[0]["historical_examples"]
    assert len(forwarded) == 3
    assert forwarded[0]["invoice_number"] == "PRIOR-0"


# ---------------------------------------------------------------------------
# Gap G — _full_template_verify control flow
# ---------------------------------------------------------------------------


def _make_service_with_stubs(monkeypatch):
    """Return an ExtractionService with verifiers pre-wired via monkeypatch."""
    return ExtractionService(
        repo=None, template_extractor=None, llm_extractor=None,
        downloader=None, template_repo=None,
    )


def test_full_template_verify_short_circuits_when_roundtrip_fails(monkeypatch):
    monkeypatch.setattr(
        "usecases.extraction_service.verify_template_roundtrip",
        lambda *_a, **_kw: (False, "roundtrip_failed"),
    )
    monkeypatch.setattr(
        "usecases.extraction_service.verify_template_against_history",
        lambda *_a, **_kw: pytest.fail("history must not be called after roundtrip fails"),
    )
    monkeypatch.setattr(
        "usecases.extraction_service.verify_template_against_perturbations",
        lambda *_a, **_kw: pytest.fail("perturbation must not be called after roundtrip fails"),
    )
    svc = _make_service_with_stubs(monkeypatch)
    ok, reason = svc._full_template_verify("yaml", "ocr", {}, history_pairs=[{"invoice_number": "X"}])
    assert not ok
    assert reason == "roundtrip_failed"


def test_full_template_verify_skips_history_when_no_history_pairs(monkeypatch):
    monkeypatch.setattr(
        "usecases.extraction_service.verify_template_roundtrip",
        lambda *_a, **_kw: (True, "ok"),
    )
    monkeypatch.setattr(
        "usecases.extraction_service.verify_template_against_history",
        lambda *_a, **_kw: pytest.fail("history must not be called when history_pairs is empty"),
    )
    monkeypatch.setattr(
        "usecases.extraction_service.verify_template_against_perturbations",
        lambda *_a, **_kw: (True, "ok"),
    )
    svc = _make_service_with_stubs(monkeypatch)
    ok, _ = svc._full_template_verify("yaml", "ocr", {}, history_pairs=[])
    assert ok


def test_full_template_verify_strict_perturbation_rejects_on_failure(monkeypatch):
    monkeypatch.setattr(
        "usecases.extraction_service.verify_template_roundtrip",
        lambda *_a, **_kw: (True, "ok"),
    )
    monkeypatch.setattr(
        "usecases.extraction_service.verify_template_against_perturbations",
        lambda *_a, **_kw: (False, "perturbation_failed"),
    )
    svc = _make_service_with_stubs(monkeypatch)
    svc._STRICT_PERTURBATION = True
    ok, reason = svc._full_template_verify("yaml", "ocr", {}, history_pairs=[])
    assert not ok
    assert reason == "perturbation_failed"


def test_full_template_verify_permissive_perturbation_passes_despite_failure(monkeypatch):
    monkeypatch.setattr(
        "usecases.extraction_service.verify_template_roundtrip",
        lambda *_a, **_kw: (True, "ok"),
    )
    monkeypatch.setattr(
        "usecases.extraction_service.verify_template_against_perturbations",
        lambda *_a, **_kw: (False, "noise"),
    )
    svc = _make_service_with_stubs(monkeypatch)
    svc._STRICT_PERTURBATION = False
    ok, _ = svc._full_template_verify("yaml", "ocr", {}, history_pairs=[])
    assert ok


# ---------------------------------------------------------------------------
# Gap H — _generate_candidates K>1 multi-candidate path
# ---------------------------------------------------------------------------


def test_generate_candidates_k2_short_circuits_on_first_pass(
    fake_document_repo, fake_template_repo, stub_verifiers, monkeypatch,
):
    """K=2: if the first candidate passes, the second should never be requested."""
    monkeypatch.setenv("TEMPLATE_GEN_CANDIDATES", "2")
    doc = _make_doc()
    fake_document_repo.add(doc)
    llm = _LLMStub(yamls=["first\n", "second\n"])
    service = _service(fake_document_repo, llm, template_repo=fake_template_repo)

    # First call passes, second would also pass but should never be reached.
    stub_verifiers(roundtrip=(True, "ok"))

    service.generate_template_for_document(doc.id)

    # generate_template called only once (short-circuit) + 0 retry calls.
    assert len(llm.calls) == 1


def test_generate_candidates_k2_both_fail_best_loser_used_as_prior_attempt(
    fake_document_repo, fake_template_repo, stub_verifiers, monkeypatch,
):
    """K=2, both fail: the candidate with the higher _score_partial is forwarded
    to the retry-with-feedback path as prior_attempt."""
    monkeypatch.setenv("TEMPLATE_GEN_CANDIDATES", "2")
    doc = _make_doc()
    fake_document_repo.add(doc)

    # Two candidates with distinct YAML so we can tell which was forwarded.
    llm = _LLMStub(yamls=["loser_yaml\n", "better_yaml\n", "retry_yaml\n"])
    service = _service(fake_document_repo, llm, template_repo=fake_template_repo)

    # Both candidates fail roundtrip. After K attempts, roundtrip passes on
    # the retry so the template is ultimately saved.
    # Note: _score_partial also calls verify_template_roundtrip once per
    # candidate, so with K=2 there are 4 roundtrip calls before the retry
    # (2 from _full_template_verify + 2 from _score_partial).
    call_count = [0]
    def roundtrip_side_effect(*_a, **_kw):
        call_count[0] += 1
        if call_count[0] <= 4:
            return (False, f"fail_{call_count[0]}")
        return (True, "ok")

    monkeypatch.setattr(
        "usecases.extraction_service.verify_template_roundtrip",
        roundtrip_side_effect,
    )
    monkeypatch.setattr(
        "usecases.extraction_service.verify_template_against_perturbations",
        lambda *_a, **_kw: (True, "ok"),
    )
    monkeypatch.setattr(
        "usecases.extraction_service.postprocess_template_regexes",
        lambda y: y,
    )

    service.generate_template_for_document(doc.id)

    # 2 candidate calls + 1 retry call = 3 total generate_template calls.
    assert len(llm.calls) == 3
    # The retry call must have prior_attempt set (the best loser).
    assert llm.calls[2]["prior_attempt"] is not None


def test_candidate_count_falls_back_to_one_on_invalid_env_var(monkeypatch):
    monkeypatch.setenv("TEMPLATE_GEN_CANDIDATES", "not-an-int")
    assert ExtractionService._candidate_count() == 1


# ---------------------------------------------------------------------------
# Gap J — _sanitize_filename_component edge cases
# ---------------------------------------------------------------------------


@pytest.mark.parametrize("value, expected", [
    # All punctuation → stripped to "" → caller substitutes "unknown_vendor".
    ("../../../", ""),
    # Exactly 81 'a' chars → truncated to 80.
    ("a" * 81, "a" * 80),
    # Unicode: non-ASCII chars replaced by "_"; ASCII letters at both ends survive.
    ("abc Ачм xyz", "abc_____xyz"),
])
def test_sanitize_filename_component(value, expected):
    assert _sanitize_filename_component(value) == expected


# ---------------------------------------------------------------------------
# _layout_slug_from_yaml
# ---------------------------------------------------------------------------


import yaml as _yaml_mod


def test_layout_slug_extracts_issuer():
    yaml_str = _yaml_mod.dump({"issuer": "Tally ERP Tax Invoice", "keywords": ["TAX INVOICE"]})
    assert _layout_slug_from_yaml(yaml_str) == "tally_erp_tax_invoice"


def test_layout_slug_caps_at_40_chars():
    issuer = "A" * 80
    yaml_str = _yaml_mod.dump({"issuer": issuer, "keywords": []})
    slug = _layout_slug_from_yaml(yaml_str)
    assert len(slug) <= 40


def test_layout_slug_falls_back_to_hash_when_issuer_empty():
    yaml_str = _yaml_mod.dump({"issuer": "", "keywords": []})
    slug = _layout_slug_from_yaml(yaml_str)
    # Hash fallback: 6 hex chars
    assert len(slug) == 6
    assert slug.isalnum()


def test_layout_slug_falls_back_to_hash_on_invalid_yaml():
    slug = _layout_slug_from_yaml("not: valid: yaml: [[[")
    assert len(slug) == 6


def test_layout_slug_falls_back_to_hash_when_issuer_is_all_punctuation():
    yaml_str = _yaml_mod.dump({"issuer": "...---...", "keywords": []})
    slug = _layout_slug_from_yaml(yaml_str)
    # Sanitized issuer is empty → hash fallback
    assert len(slug) == 6


def test_layout_slug_different_layouts_produce_different_slugs():
    """Two YAML templates with different issuers must produce different slugs."""
    yaml_a = _yaml_mod.dump({"issuer": "Tally ERP Invoice", "keywords": ["A"]})
    yaml_b = _yaml_mod.dump({"issuer": "Zoho Books Invoice", "keywords": ["B"]})
    assert _layout_slug_from_yaml(yaml_a) != _layout_slug_from_yaml(yaml_b)


def test_generate_template_all_punctuation_vendor_falls_back_to_unknown_vendor(
    fake_document_repo, fake_template_repo, stub_verifiers,
):
    """When sanitized vendor identifier is empty, filename uses 'unknown_vendor'."""
    doc = _make_doc(validated={
        "vendor_gstin": "../../../",  # sanitizes to "" → fallback
        "doc_type": "INVOICE",
        "invoice_number": "INV-1",
        "total_amount": 1.0,
    })
    fake_document_repo.add(doc)
    service = _service(fake_document_repo, _LLMStub(), template_repo=fake_template_repo)

    service.generate_template_for_document(doc.id)

    saved_name, _, _ = fake_template_repo.save_calls[0]
    assert saved_name == "unknown_vendor_x_invoice.yml"


# ---------------------------------------------------------------------------
# Regression: ISO-date invoice with adjacent invoice_number succeeds
# in a single attempt (fixes the production DLQ failure from 2026-05-08)
# ---------------------------------------------------------------------------

def test_iso_date_invoice_with_adjacent_invoice_number_uses_narrowing_or_succeeds(
    fake_document_repo, fake_template_repo, monkeypatch,
):
    """Regression test for doc 019e07fb-...: when the OCR has the invoice
    number and an ISO date on the same line, the pipeline must save a template
    that captures only the invoice number.

    The test drives the real postprocess_template_regexes path and patches only
    the verifier to simulate the overcapture scenario then confirm narrowing
    recovers without needing a second LLM call.
    """
    from infrastructure.invoice2data_wrapper import _LINE_WRAP_CONTINUATION
    import yaml as _yaml

    OCR = (
        "TAX INVOICE\n"
        "GSTIN\n"
        "Invoice No: INV/2024/77157/002 2025-08-03\n"
        "Invoice Date: 2025-08-03\n"
        "Grand Total: 1180.00\n"
    )
    TARGET = {
        "vendor_gstin": "27AADCB2230M1ZV",
        "doc_type": "INVOICE",
        "invoice_number": "INV/2024/77157/002",
        "invoice_date": "2025-08-03",
        "total_amount": 1180.0,
    }
    # The LLM generates a YAML with a correct base regex for invoice_number.
    # postprocess_template_regexes will add the line-wrap continuation.
    LLM_YAML = _yaml.dump({
        "issuer": "Test Invoice Layout",
        "keywords": ["TAX INVOICE", "GSTIN"],
        "fields": {
            "invoice_number": (
                "(?si)Invoice\\s*No[.:]?\\s*(?P<invoice_number>[A-Z0-9][A-Z0-9/_-]+)"
            ),
            "date": "(?si)Invoice\\s*Date[:\\s]+(?P<date>\\d{4}-\\d{1,2}-\\d{1,2})",
            # invoice2data requires 'amount' as a default required field
            "amount": "(?si)Grand\\s*Total[:\\s]+(?P<amount>[\\d,]+\\.\\d{2})",
        },
        "options": {
            "currency": "INR",
            "date_formats": ["%Y-%m-%d", "%d/%m/%Y"],
        },
    }, default_flow_style=False)

    doc = _make_doc(raw_ocr=OCR, validated=TARGET)
    fake_document_repo.add(doc)

    monkeypatch.setattr(
        "usecases.extraction_service._sanitize_keywords_to_ocr",
        lambda yaml_content, raw_ocr: yaml_content,
    )

    # Track how many LLM calls were made.
    llm = _LLMStub(yamls=[LLM_YAML])

    service = _service(fake_document_repo, llm, template_repo=fake_template_repo)
    service.generate_template_for_document(doc.id)

    # Should succeed in a single LLM call (narrowing path or tight continuation).
    assert llm.calls, "LLM must have been called"

    _, saved_content, _ = fake_template_repo.save_calls[0]

    # Verify the saved template's invoice_number regex does NOT capture the date.
    import re
    saved_data = _yaml.safe_load(saved_content)
    regex_str = saved_data["fields"]["invoice_number"]
    if isinstance(regex_str, list):
        regex_str = regex_str[0]
    m = re.search(regex_str, "Invoice No: INV/2024/77157/002 2025-08-03")
    assert m is not None, "regex must match OCR line"
    captured = m.group("invoice_number")
    assert "2025" not in captured, (
        f"saved template must not capture date; got invoice_number={captured!r}"
    )
