"""invoice2data wrapper implementing the TemplateExtractor port.

Uses a custom extraction loop (instead of the high-level extract_data helper)
so we can capture the matched template's name reliably. Templates are
refreshed from the DB on every extract() call so newly-generated ones take
effect without a worker restart.
"""

from __future__ import annotations

import hashlib
import logging
import os
import re
import shutil
import tempfile
import threading
from typing import Any

import yaml

from invoice2data.extract.loader import read_templates
from invoice2data.input import pdftotext

from domain.entities import ExtractionResult, LineItem
from domain.enums import DocumentType
from domain.exceptions import TemplateMatchError
from domain.ports import TemplateExtractor, TemplateRepository
from infrastructure.ocr import extract_best_ocr

logger = logging.getLogger(__name__)

# Fields that contribute to template scoring. Internal/config keys are
# excluded so they don't inflate a low-quality template's score.
_SCORED_FIELDS = frozenset({
    "invoice_number", "date", "invoice_due_date", "po_number",
    "issuer", "vendor_gstin", "buyer_gstin",
    "amount", "subtotal", "gross_amount",
    "sgst", "cgst", "igst", "gst_rate",
    "discount", "shipping", "payment_terms", "lines",
})


class Invoice2DataExtractor(TemplateExtractor):
    """Wraps the invoice2data library for template-based extraction.

    Templates are re-read from the repository on every extract() call,
    keyed by a content signature so we only re-parse YAML when the DB
    actually changed. This ensures HITL-generated templates are used
    immediately, without waiting for a worker restart.
    """

    def __init__(self, template_dir: str, template_repo: TemplateRepository | None = None) -> None:
        self._template_dir = template_dir
        self._template_repo = template_repo
        self._templates: list = []
        self._cached_signature: str | None = None
        self._lock = threading.Lock()
        # Prime the cache so startup logs reflect the initial state.
        self._refresh_templates()

    def _refresh_templates(self) -> None:
        """Reload templates if the DB signature changed since the last refresh.

        Order of preference:
          1. Database (source of truth — includes LLM-generated templates)
          2. Filesystem directory (bootstrap seed, only used when DB is empty)
          3. invoice2data built-ins (last-resort fallback)
        """
        rows: list[dict] = []
        if self._template_repo is not None:
            try:
                rows = self._template_repo.list_all() or []
            except Exception as exc:  # noqa: BLE001 — never fail extraction due to repo
                logger.warning("Template repo list_all failed: %s — keeping cached set", exc)
                return

        signature = _signature(rows)
        with self._lock:
            if signature == self._cached_signature and self._templates:
                return

            if rows:
                new_templates = _parse_rows(rows)
                logger.info(
                    "Refreshed %d template(s) from database (signature=%s)",
                    len(new_templates), signature[:8],
                )
            elif os.path.isdir(self._template_dir):
                new_templates = read_templates(self._template_dir)
                logger.info(
                    "DB empty — loaded %d template(s) from filesystem: %s",
                    len(new_templates), self._template_dir,
                )
            else:
                logger.warning(
                    "No templates available: DB empty and filesystem dir %s not found",
                    self._template_dir,
                )
                new_templates = read_templates()  # invoice2data built-ins

            self._templates = new_templates
            self._cached_signature = signature

    def extract(self, file_path: str) -> ExtractionResult:
        """Extract data with template matching, capturing the template name.

        Every matching template is tried; the one extracting the most fields
        wins. This avoids silent collisions between format-generic templates
        and ensures we don't pick a weaker match just because it loaded first.
        """
        self._refresh_templates()

        ocr = extract_best_ocr(file_path)
        extracted_str = re.sub(r"[ \t]+", " ", ocr.text) if ocr.text else ocr.text
        input_module = ocr.input_module
        logger.info(
            "Template OCR engine=%s for %s (score=%.2f, pdftotext=%.2f, tesseract=%s): %s",
            ocr.engine,
            file_path,
            ocr.score,
            ocr.pdftotext_score,
            f"{ocr.tesseract_score:.2f}" if ocr.tesseract_score is not None else "n/a",
            ocr.reason,
        )
        if not extracted_str:
            logger.warning("No OCR text extracted from %s. Templates will fail to match.", file_path)

        # Step 2: Score every matching template, pick the best.
        candidates: list[tuple[int, Any, dict]] = []
        matched_template_names: list[str] = []
        extraction_failures: list[str] = []
        empty_extractions: list[str] = []
        for t in self._templates:
            if not t.matches_input(extracted_str):
                continue
            tpl_label = _template_label(t)
            matched_template_names.append(tpl_label)
            optimized_str = t.prepare_input(extracted_str)
            try:
                raw = t.extract(optimized_str, file_path, input_module)
            except Exception as exc:
                logger.warning(
                    "Template %s matched but extraction failed: %s",
                    tpl_label, exc,
                )
                extraction_failures.append(f"{tpl_label}: {exc}")
                continue
            if raw is None:
                empty_extractions.append(tpl_label)
                continue
            score = _score_extraction(raw)
            candidates.append((score, t, raw))

        if not candidates:
            raise TemplateMatchError(
                _build_no_candidate_reason(
                    template_count=len(self._templates),
                    extracted_text=extracted_str,
                    matched_template_names=matched_template_names,
                    extraction_failures=extraction_failures,
                    empty_extractions=empty_extractions,
                )
            )

        # Higher score wins. Python's sort is stable, so ties preserve the
        # DB order (which is ORDER BY name) — making collisions deterministic.
        candidates.sort(key=lambda c: c[0], reverse=True)
        if len(candidates) > 1:
            logger.info(
                "Multiple templates matched (%d); picking best by field count. "
                "scores=%s",
                len(candidates),
                [(c[1].get("issuer", "?"), c[0]) for c in candidates[:5]],
            )

        best_score, best_t, best_raw = candidates[0]
        tpl_name = _template_label(best_t)
        logger.info("Template matched: %s (score=%d)", tpl_name, best_score)
        return self._map(best_raw, template_name=tpl_name)

    # -- mapping helpers ------------------------------------------------------

    @staticmethod
    def _map(raw: dict, template_name: str | None = None) -> ExtractionResult:
        """Convert invoice2data dict → domain ExtractionResult."""
        line_items: list[LineItem] = []
        for li in raw.get("lines", []):
            line_items.append(
                LineItem(
                    sku=li.get("sku") or li.get("hsn") or li.get("item_code") or None,
                    description=li.get("desc") or li.get("description"),
                    quantity=_safe_float(li.get("qty") or li.get("quantity")),
                    unit_price=_safe_float(li.get("price") or li.get("unit_price")),
                    tax_rate=_safe_float(li.get("tax_rate") or li.get("gst_rate")),
                    tax=_safe_float(li.get("tax")),
                    amount=_safe_float(li.get("line_total") or li.get("amount")),
                )
            )

        # Determine document type from template keyword hints
        tpl_name_lower = (template_name or "").lower()
        doc_type = DocumentType.UNKNOWN
        if "purchase" in tpl_name_lower or "po" in tpl_name_lower:
            doc_type = DocumentType.PURCHASE_ORDER
        elif template_name:
            doc_type = DocumentType.INVOICE

        # Store template_name in raw so extraction_service can read it
        raw["template_name"] = template_name

        # Build confidence scores: template extraction is exact-match,
        # so all extracted (non-None) fields get 1.0 confidence.
        field_map = {
            "invoice_number": raw.get("invoice_number"),
            "invoice_date": raw.get("date"),
            "po_number": raw.get("po_number"),
            "vendor_name": raw.get("issuer"),
            "vendor_gstin": raw.get("vendor_gstin") or raw.get("gstin"),
            "buyer_gstin": raw.get("buyer_gstin"),
            "total_amount": raw.get("amount"),
            "discount": raw.get("discount"),
            "sgst": raw.get("sgst"),
            "cgst": raw.get("cgst"),
            "igst": raw.get("igst"),
            "shipping": raw.get("shipping") or raw.get("freight"),
            "doc_type": template_name,  # non-None if template matched
            "line_items": raw.get("lines"),
        }
        confidence_scores = {k: 1.0 for k, v in field_map.items() if v is not None}

        return ExtractionResult(
            invoice_number=raw.get("invoice_number"),
            invoice_date=str(raw["date"]) if raw.get("date") else None,
            invoice_due_date=str(raw["invoice_due_date"]) if raw.get("invoice_due_date") else None,
            po_number=raw.get("po_number"),
            vendor_name=raw.get("issuer"),
            vendor_gstin=raw.get("vendor_gstin") or raw.get("gstin"),
            buyer_gstin=raw.get("buyer_gstin"),
            total_amount=_safe_float(raw.get("amount")),
            subtotal=_safe_float(raw.get("subtotal")),
            gross_amount=_safe_float(raw.get("gross_amount")),
            discount=_safe_float(raw.get("discount")),
            discount_type=raw.get("discount_type"),
            payment_terms=raw.get("payment_terms"),
            gst_rate=_safe_float(raw.get("gst_rate")),
            sgst=_safe_float(raw.get("sgst")),
            cgst=_safe_float(raw.get("cgst")),
            igst=_safe_float(raw.get("igst")),
            shipping=_safe_float(raw.get("shipping") or raw.get("freight")),
            line_items=line_items,
            doc_type=doc_type,
            raw=raw,
            confidence_scores=confidence_scores,
        )


def _parse_rows(rows: list[dict]) -> list:
    """Write template rows to a temp dir and load via invoice2data.

    The temp dir is cleaned up before this function returns; invoice2data
    parses the YAML eagerly so the returned template objects do not rely on
    the files still existing on disk.
    """
    tmp_dir = tempfile.mkdtemp(prefix="i2d_templates_")
    try:
        for row in rows:
            path = os.path.join(tmp_dir, row["name"])
            with open(path, "w", encoding="utf-8") as f:
                f.write(row["content"])
        return read_templates(tmp_dir)
    finally:
        shutil.rmtree(tmp_dir, ignore_errors=True)


def _template_label(template: Any) -> str:
    """Return a stable, readable label for a template object."""
    return (
        template.get("template_name", None)
        or template.get("issuer", None)
        or str(template)
    )


def _build_no_candidate_reason(
    *,
    template_count: int,
    extracted_text: str,
    matched_template_names: list[str],
    extraction_failures: list[str],
    empty_extractions: list[str],
) -> str:
    """Summarize why invoice2data could not produce a template extraction."""
    if template_count == 0:
        return "No invoice2data templates are loaded."

    if not extracted_text:
        return (
            "No OCR text was extracted from the document; pdftotext and "
            "tesseract both returned empty text."
        )

    if matched_template_names:
        parts: list[str] = []
        if extraction_failures:
            preview = "; ".join(extraction_failures[:3])
            extra = ""
            if len(extraction_failures) > 3:
                extra = f"; and {len(extraction_failures) - 3} more template failure(s)"
            parts.append(
                f"{len(extraction_failures)} matched template(s) failed during extraction: "
                f"{preview}{extra}."
            )
        if empty_extractions:
            preview = ", ".join(empty_extractions[:5])
            extra = ""
            if len(empty_extractions) > 5:
                extra = f", and {len(empty_extractions) - 5} more"
            parts.append(
                f"{len(empty_extractions)} matched template(s) returned no structured data: "
                f"{preview}{extra}."
            )
        if parts:
            return " ".join(parts)

    return (
        f"No template regex matched the OCR text "
        f"({template_count} template(s) checked)."
    )


def _flatten_values(value: Any) -> list[Any]:
    if value is None:
        return []
    if isinstance(value, (list, tuple, set)):
        flattened: list[Any] = []
        for item in value:
            flattened.extend(_flatten_values(item))
        return flattened
    return [value]


def _norm_scalar(value: Any) -> str:
    return "".join(ch for ch in str(value).lower() if not ch.isspace())


def _norm_identifier(value: Any) -> str:
    return "".join(ch for ch in str(value).lower() if ch.isalnum())


def _amount_variants(value: Any) -> set[str]:
    try:
        amount = float(str(value).replace(",", "").strip())
    except (TypeError, ValueError):
        return set()

    variants = {
        f"{amount:.2f}",
        f"{amount:,.2f}",
        f"{amount:g}",
        f"{amount:,.0f}" if amount.is_integer() else "",
    }
    return {variant for variant in variants if variant}


def _value_visible_in_ocr(raw_ocr: str, target_key: str, value: Any) -> bool:
    if value in (None, ""):
        return False

    haystack = raw_ocr or ""
    if not haystack.strip():
        return False

    if target_key == "total_amount":
        return any(variant in haystack for variant in _amount_variants(value))

    needle = str(value).strip()
    if not needle:
        return False

    if target_key in {"vendor_gstin", "buyer_gstin"}:
        return _norm_identifier(needle) in _norm_identifier(haystack)

    return _norm_scalar(needle) in _norm_scalar(haystack)


def _match_scalar_field(raw_value: Any, target_value: Any, *, key: str) -> tuple[bool, str | None]:
    candidates = _flatten_values(raw_value)
    if not candidates:
        return False, f"{key} missing (want {target_value!r})"

    norm_target = _norm_identifier(target_value) if key.endswith("gstin") else _norm_scalar(target_value)
    norm_candidates = {
        _norm_identifier(candidate) if key.endswith("gstin") else _norm_scalar(candidate)
        for candidate in candidates
        if str(candidate).strip()
    }
    if not norm_candidates:
        return False, f"{key} missing (want {target_value!r})"
    if norm_candidates == {norm_target}:
        return True, None
    if norm_target in norm_candidates:
        return False, f"{key} overmatched {raw_value!r} want only {target_value!r}"
    return False, f"{key} got {raw_value!r} want {target_value!r}"


def _signature(rows: list[dict]) -> str:
    """Stable content-hash of the (sorted) template set.

    Used to skip re-parsing YAML when nothing in the DB changed since the
    last refresh — the expensive step is invoice2data's YAML → Template
    object construction, not the list_all() query.
    """
    h = hashlib.sha1()
    for row in sorted(rows, key=lambda r: r.get("name", "")):
        h.update(str(row.get("name", "")).encode("utf-8"))
        h.update(b"\0")
        h.update(str(row.get("content", "")).encode("utf-8"))
        h.update(b"\n")
    return h.hexdigest()


def _score_extraction(raw: dict) -> int:
    """Count how many scorable fields this template populated.

    Line items are worth 2 points (their presence is a strong signal that
    the template actually understood the document's structure, not just
    the header fields). Everything else in _SCORED_FIELDS is worth 1.
    """
    score = 0
    for key, value in raw.items():
        if key not in _SCORED_FIELDS:
            continue
        if value in (None, "", [], {}):
            continue
        score += 2 if key == "lines" and isinstance(value, list) else 1
    return score


def _add_flags_to_regex(pattern: str) -> str:
    """Prepend (?si) to a regex string unless inline flags are already present.

    invoice2data's regex parser calls re.findall(regex, text) with no flags,
    so regexes are case-sensitive and . does not match newlines by default.
    Adding (?si) makes anchors case-insensitive and lets .* span lines, which
    is necessary for LLM-generated templates whose anchors may not exactly
    match the case of the OCR output.
    """
    if not isinstance(pattern, str):
        return pattern
    if re.match(r"^\(\?[a-zA-Z]*[si]", pattern):
        return pattern
    return "(?si)" + pattern


# Fields whose values can be split across an OCR line break (e.g. a scanner
# breaks "INV/2026/34951/003" into "INV/2026/3495\n1/003"). We widen their
# capture groups to optionally span one whitespace gap.
_IDENTIFIER_FIELDS = frozenset({"invoice_number", "po_number"})

# The continuation appended to the last capture group.  Allows one optional
# run of identifier-safe characters after a whitespace gap.
#
# The negative lookahead (?!\d{1,4}[-/]\d{1,2}[-/]\d{1,4}\b) prevents the
# continuation from greedily swallowing a date that appears on the same line
# as the identifier (e.g. "INV/2024/77157/002 2025-08-03"). Without it the
# character class [A-Z0-9/_-] would match digits and hyphens in ISO/numeric
# dates and incorrectly fold the date into the invoice_number capture.
# Colon and period are also removed from the class: they are not valid in
# invoice/PO numbers and only widen the attack surface (timestamps, amounts).
_LINE_WRAP_CONTINUATION = r"(?:\s+(?!\d{1,4}[-/]\d{1,2}[-/]\d{1,4}\b)[A-Z0-9/_-]+)?"


def _allow_identifier_line_wrap(pattern: str) -> str:
    """Widen the last capture group in an identifier regex to span OCR line breaks.

    OCR can split a value like ``INV/2026/34951/003`` into two lines:
    ``INV/2026/3495`` and ``1/003``. A character-class run like
    ``[A-Z0-9/_-]+`` stops at the newline and captures only the first part.
    This function appends ``(?:\\s+[A-Z0-9/_:.-]+)?`` inside the last simple
    (non-nested) capture group so the regex picks up one continuation token
    across the gap.

    Only modifies patterns whose last non-nested capture group ends with ``+``
    or ``+?`` and does not already contain a ``\\s`` class.  Returns the
    original pattern unchanged if the heuristic cannot safely apply.
    """
    if not isinstance(pattern, str):
        return pattern
    if _LINE_WRAP_CONTINUATION in pattern:
        return pattern  # already widened

    # Find all top-level (non-nested) capturing groups that end with + or +?
    # A "top-level" group contains no ( or ) inside it.
    matches = list(re.finditer(r"\([^()]*\+\??(?=\))", pattern))
    if not matches:
        return pattern

    # Skip if any capture already handles whitespace
    for m in matches:
        if r"\s" in m.group():
            return pattern

    last_m = matches[-1]
    # Insert the continuation before the closing )
    close_pos = last_m.end()  # points just after +? — the ) is next
    return pattern[:close_pos] + _LINE_WRAP_CONTINUATION + pattern[close_pos:]


def postprocess_template_regexes(yaml_content: str) -> str:
    """Add (?si) inline flags to all field regexes in a generated template.

    Parses the YAML, patches each regex in the ``fields`` block, and returns
    the re-serialised YAML. The ``lines`` block is left untouched because its
    start/end patterns anchor on column headers where case is well-defined.
    Returns the original string unchanged on any parse error so callers can
    still attempt verification and surface the real problem.
    """
    if not yaml_content or not yaml_content.strip():
        return yaml_content
    try:
        data = yaml.safe_load(yaml_content)
    except yaml.YAMLError:
        return yaml_content

    if not isinstance(data, dict):
        return yaml_content

    fields = data.get("fields")
    if not isinstance(fields, dict):
        return yaml_content

    for key, value in fields.items():
        widen = key in _IDENTIFIER_FIELDS
        if isinstance(value, str):
            p = _add_flags_to_regex(value)
            fields[key] = _allow_identifier_line_wrap(p) if widen else p
        elif isinstance(value, list):
            processed = [_add_flags_to_regex(v) for v in value]
            fields[key] = [_allow_identifier_line_wrap(p) for p in processed] if widen else processed
        elif isinstance(value, dict):
            # Parser-style field: {parser: regex, regex: "..."}
            if isinstance(value.get("regex"), str):
                p = _add_flags_to_regex(value["regex"])
                value["regex"] = _allow_identifier_line_wrap(p) if widen else p
            elif isinstance(value.get("regex"), list):
                processed = [_add_flags_to_regex(v) for v in value["regex"]]
                value["regex"] = [_allow_identifier_line_wrap(p) for p in processed] if widen else processed

    return yaml.dump(data, default_flow_style=False, allow_unicode=True, sort_keys=False)


def _sanitize_keywords_to_ocr(yaml_content: str, raw_ocr: str) -> str:
    """Strip template keywords that are not literally present in the OCR text.

    invoice2data requires every keyword to be a literal substring of the input.
    When the LLM hallucinates or misquotes a keyword the roundtrip check fails.
    This step removes non-matching keywords so the template can at least match
    its own source document.

    If all keywords are stripped, falls back to the first short all-caps line
    found in the OCR. Returns the original YAML unchanged if no fallback exists
    so that verification can surface the real problem.
    """
    if not yaml_content or not raw_ocr:
        return yaml_content
    try:
        data = yaml.safe_load(yaml_content)
    except yaml.YAMLError:
        return yaml_content

    if not isinstance(data, dict):
        return yaml_content

    keywords = data.get("keywords")
    if not isinstance(keywords, list) or not keywords:
        return yaml_content

    def _kw_matches(kw: str) -> bool:
        # Mirror invoice2data's matches_input: re.search(keyword, text) with no
        # flags — case-sensitive, keyword treated as a regex. Fall back to a
        # literal case-sensitive search when the keyword is not a valid regex.
        try:
            return bool(re.search(kw, raw_ocr))
        except re.error:
            return bool(re.search(re.escape(kw), raw_ocr))

    surviving = [kw for kw in keywords if isinstance(kw, str) and _kw_matches(kw)]

    if not surviving:
        for line in raw_ocr.splitlines():
            stripped = line.strip()
            if not stripped or len(stripped) > 60:
                continue
            if re.fullmatch(r"[\d\s.,/:%-]+", stripped):
                continue
            tokens = stripped.split()
            if any(t.isupper() and len(t) >= 3 for t in tokens):
                surviving = [stripped]
                break

    if not surviving:
        return yaml_content

    data["keywords"] = surviving
    return yaml.dump(data, default_flow_style=False, allow_unicode=True, sort_keys=False)


def _to_date(value: Any):
    """Parse a date-like value; returns datetime.date or None.

    invoice2data's parse_date returns datetime.datetime objects; the target
    data from the extractor is always a plain string (YYYY-MM-DD). We
    normalise both to datetime.date so the comparison is type-safe.
    """
    import datetime
    if value is None:
        return None
    # datetime.datetime is a subclass of datetime.date, so check it first.
    if isinstance(value, datetime.datetime):
        return value.date()
    if isinstance(value, datetime.date):
        return value
    try:
        import dateparser
        result = dateparser.parse(str(value))
        return result.date() if result else None
    except Exception:
        return None


def verify_template_roundtrip(
    yaml_content: str,
    raw_ocr: str,
    target_data: dict,
    *,
    amount_tolerance: float = 0.5,
) -> tuple[bool, str]:
    """Check that a freshly-generated template actually extracts its source.

    Writes the YAML to a temp dir, loads it via invoice2data, and runs
    matches_input + extract against the same OCR text that was used to
    generate it. If the template cannot recover the values it was trained
    on, it will never work on future documents of the same layout — so we
    reject it here rather than polluting the DB.

    Returns (ok, reason). `reason` is always a short human-readable tag.
    """
    if not yaml_content or not yaml_content.strip():
        return False, "empty_yaml"
    if not raw_ocr or not raw_ocr.strip():
        return False, "empty_raw_ocr"

    tmp_dir = tempfile.mkdtemp(prefix="i2d_verify_")
    try:
        with open(os.path.join(tmp_dir, "candidate.yml"), "w", encoding="utf-8") as f:
            f.write(yaml_content)
        try:
            templates = read_templates(tmp_dir)
        except Exception as exc:
            return False, f"yaml_parse_error: {exc}"
    finally:
        shutil.rmtree(tmp_dir, ignore_errors=True)

    if not templates:
        return False, "no_templates_loaded"
    t = templates[0]

    if not t.matches_input(raw_ocr):
        try:
            import yaml as _yaml
            _parsed = _yaml.safe_load(yaml_content)
            _keywords = _parsed.get("keywords", [])
            _missing = [kw for kw in _keywords
                        if not re.search(re.escape(str(kw)), raw_ocr, re.IGNORECASE)]
        except Exception:
            _missing = []
        detail = f": {_missing}" if _missing else ""
        return False, f"keywords_do_not_match_source_ocr{detail}"

    optimized = t.prepare_input(raw_ocr)
    try:
        raw = t.extract(optimized, None, pdftotext)
    except Exception as exc:
        return False, f"extract_raised: {exc}"

    if raw is None or not isinstance(raw, dict):
        return False, "extract_returned_none"

    def _to_float(v: Any) -> float | None:
        if v is None:
            return None
        try:
            s = str(v).replace(",", "").strip()
            return float(s) if s else None
        except (ValueError, TypeError):
            return None

    identity_mismatches: list[str] = []
    critical_fields: list[tuple[str, str]] = [
        ("invoice_number", "invoice_number"),
        ("po_number", "po_number"),
        ("vendor_gstin", "vendor_gstin"),
        ("buyer_gstin", "buyer_gstin"),
    ]

    required_identity_fields: list[str] = []
    for raw_key, target_key in critical_fields:
        tv = target_data.get(target_key)
        if not _value_visible_in_ocr(raw_ocr, target_key, tv):
            continue
        required_identity_fields.append(raw_key)
        ok, mismatch = _match_scalar_field(raw.get(raw_key), tv, key=raw_key)
        if not ok and mismatch:
            identity_mismatches.append(mismatch)

    target_amount = _to_float(target_data.get("total_amount"))
    amount_ok: bool | None = None
    amount_mismatch: str | None = None
    if target_amount is not None:
        raw_amount = _to_float(raw.get("amount"))
        if raw_amount is None:
            amount_mismatch = f"amount missing (want {target_amount})"
            amount_ok = False
        elif abs(raw_amount - target_amount) > amount_tolerance:
            amount_mismatch = f"amount {raw_amount} vs {target_amount}"
            amount_ok = False
        else:
            amount_ok = True

    # Validate date field when target provides invoice_date and it was
    # extracted. A wrong-format date (e.g. month/day swap) passes invoice2data's
    # own required-fields check but would silently produce incorrect data.
    date_mismatch: str | None = None
    target_invoice_date = target_data.get("invoice_date")
    if target_invoice_date and raw.get("date"):
        expected_date = _to_date(target_invoice_date)
        extracted_date = _to_date(raw.get("date"))
        if expected_date and extracted_date and expected_date != extracted_date:
            date_mismatch = f"date {extracted_date} vs expected {expected_date}"

    if identity_mismatches:
        fields = ", ".join(required_identity_fields) or "critical identity fields"
        details = list(identity_mismatches)
        if amount_mismatch:
            details.append(amount_mismatch)
        if date_mismatch:
            details.append(date_mismatch)
        return False, f"required_fields_failed_roundtrip ({fields}): " + "; ".join(details)
    if amount_ok is False:
        return False, f"amount_mismatch: {amount_mismatch}"
    if date_mismatch:
        return False, f"date_mismatch: {date_mismatch}"

    return True, "ok"


# Pattern that detects an overcapture mismatch caused by the line-wrap
# continuation: the extracted value is a superset of the target (extra tokens
# were gobbled). Used by narrow_template_on_overcapture to decide when
# narrowing is worth attempting.
_OVERCAPTURE_REASON_RE = re.compile(
    r"(?:invoice_number|po_number) got '[^']+' want '[^']+'"
)


def narrow_template_on_overcapture(
    yaml_content: str,
    raw_ocr: str,
    target_data: dict,
    *,
    amount_tolerance: float = 0.5,
) -> str | None:
    """Attempt to fix an overcapture in invoice_number/po_number by removing
    the line-wrap continuation that was unconditionally added by
    postprocess_template_regexes.

    Returns the corrected YAML string if stripping the continuation makes the
    template pass verify_template_roundtrip, or None if narrowing does not
    help (caller should fall through to the normal retry path).

    This is a cheap programmatic repair that avoids an extra LLM call when
    the only thing wrong is the continuation greedily capturing trailing
    content (e.g. a date) from the same line.
    """
    if not yaml_content or not yaml_content.strip():
        return None
    try:
        data = yaml.safe_load(yaml_content)
    except yaml.YAMLError:
        return None

    if not isinstance(data, dict) or not isinstance(data.get("fields"), dict):
        return None

    fields = data["fields"]
    changed = False
    for key in _IDENTIFIER_FIELDS:
        value = fields.get(key)
        if isinstance(value, str) and _LINE_WRAP_CONTINUATION in value:
            fields[key] = value.replace(_LINE_WRAP_CONTINUATION, "")
            changed = True
        elif isinstance(value, list):
            new_list = []
            for item in value:
                if isinstance(item, str) and _LINE_WRAP_CONTINUATION in item:
                    new_list.append(item.replace(_LINE_WRAP_CONTINUATION, ""))
                    changed = True
                else:
                    new_list.append(item)
            if changed:
                fields[key] = new_list
        elif isinstance(value, dict) and isinstance(value.get("regex"), str):
            if _LINE_WRAP_CONTINUATION in value["regex"]:
                value["regex"] = value["regex"].replace(_LINE_WRAP_CONTINUATION, "")
                changed = True
        elif isinstance(value, dict) and isinstance(value.get("regex"), list):
            new_list = []
            for item in value["regex"]:
                if isinstance(item, str) and _LINE_WRAP_CONTINUATION in item:
                    new_list.append(item.replace(_LINE_WRAP_CONTINUATION, ""))
                    changed = True
                else:
                    new_list.append(item)
            if changed:
                value["regex"] = new_list

    if not changed:
        return None

    narrowed_yaml = yaml.dump(data, default_flow_style=False, allow_unicode=True, sort_keys=False)
    ok, _ = verify_template_roundtrip(
        narrowed_yaml, raw_ocr, target_data, amount_tolerance=amount_tolerance
    )
    if ok:
        return narrowed_yaml
    return None


def _shift_digit_runs(text: str, shift: int = 7) -> str:
    """Replace every digit run with a different value of equal length.

    Each digit is rotated by ``shift`` mod 10, preserving the run's character
    count so column alignment in line-item tables is not disturbed. A regex
    that matches the perturbed text must therefore use a numeric character
    class (e.g. ``\\d+``) rather than any literal value from the source.
    """
    def _shift(match: re.Match[str]) -> str:
        return "".join(str((int(d) + shift) % 10) for d in match.group(0))

    return re.sub(r"\d+", _shift, text)


def _collapse_double_spaces(text: str) -> str:
    """Collapse runs of 3+ spaces down to 2 spaces.

    Mimics minor OCR-engine differences in column padding. Templates that
    rely on a precise space count between columns will fail here, which is
    exactly the brittleness we want to catch before saving.
    """
    return re.sub(r"  {2,}", "  ", text)


def perturb_ocr_for_robustness(raw_ocr: str) -> list[tuple[str, str]]:
    """Return a small set of (label, perturbed_ocr) pairs for cold-start
    robustness testing of a freshly-generated template.

    The perturbations are intentionally conservative — they target failure
    modes specific to overfit regexes (literal numeric values, brittle
    whitespace anchors) without disturbing form labels, column headers, or
    GSTIN/PAN identifiers.
    """
    if not raw_ocr or not raw_ocr.strip():
        return []
    out: list[tuple[str, str]] = []
    shifted = _shift_digit_runs(raw_ocr)
    if shifted != raw_ocr:
        out.append(("digit_shift", shifted))
    collapsed = _collapse_double_spaces(raw_ocr)
    if collapsed != raw_ocr:
        out.append(("whitespace_collapse", collapsed))
    return out


def verify_template_against_perturbations(
    yaml_content: str,
    raw_ocr: str,
) -> tuple[bool, str]:
    """Run a generated template against perturbed OCR variants.

    For each perturbation we only require the template's `keywords` list to
    still match (i.e. matches_input returns True) and that the field regexes
    still produce *some* extraction without raising. We do NOT compare the
    perturbed values against the source target_data because the perturbation
    deliberately changes them; this is purely a generalisation smoke test.

    Returns (True, "ok") when all perturbations pass, or (False, reason)
    naming the first perturbation that broke.
    """
    if not yaml_content or not yaml_content.strip():
        return False, "empty_yaml"
    perturbations = perturb_ocr_for_robustness(raw_ocr)
    if not perturbations:
        return True, "ok_no_perturbations"

    tmp_dir = tempfile.mkdtemp(prefix="i2d_perturb_")
    try:
        with open(os.path.join(tmp_dir, "candidate.yml"), "w", encoding="utf-8") as f:
            f.write(yaml_content)
        try:
            templates = read_templates(tmp_dir)
        except Exception as exc:
            return False, f"yaml_parse_error: {exc}"
    finally:
        shutil.rmtree(tmp_dir, ignore_errors=True)

    if not templates:
        return False, "no_templates_loaded"
    t = templates[0]

    failures: list[str] = []
    for label, perturbed in perturbations:
        if not t.matches_input(perturbed):
            failures.append(f"{label}: keywords_no_longer_match")
            continue
        try:
            optimized = t.prepare_input(perturbed)
            raw = t.extract(optimized, None, pdftotext)
        except Exception as exc:
            failures.append(f"{label}: extract_raised: {exc}")
            continue
        if raw is None or not isinstance(raw, dict) or not raw:
            failures.append(f"{label}: extract_returned_empty")

    if failures:
        return False, "perturbation_failed: " + "; ".join(failures)
    return True, "ok"


def verify_template_against_history(
    yaml_content: str,
    history: list[dict],
    *,
    amount_tolerance: float = 0.5,
) -> tuple[bool, str]:
    """Run a freshly-generated template against prior validated documents.

    A template that round-trips its own source OCR has only proved it can
    recover one document — overfitted regexes (literal values, vendor-specific
    anchors) still pass. To catch that, we replay the template against the
    raw_ocr of recently-validated documents from the same vendor and check
    that critical fields still come out matching the human-validated truth.

    `history` is a list of {"raw_ocr": str, "validated_data": dict} dicts
    (as produced by DocumentRepository.get_validated_with_ocr_by_template).
    Returns ("ok", _) when every entry passes, or (False, reason) describing
    the first failing document.
    """
    if not history:
        return True, "ok_no_history"
    if not yaml_content or not yaml_content.strip():
        return False, "empty_yaml"

    failures: list[str] = []
    for idx, entry in enumerate(history):
        raw_ocr = entry.get("raw_ocr") or ""
        target = entry.get("validated_data") or {}
        if not raw_ocr.strip() or not target:
            continue
        ok, reason = verify_template_roundtrip(
            yaml_content, raw_ocr, target, amount_tolerance=amount_tolerance
        )
        if not ok:
            failures.append(f"history[{idx}]: {reason}")
            if len(failures) >= 3:
                break

    if failures:
        return False, "history_roundtrip_failed: " + "; ".join(failures)
    return True, "ok"


def _safe_float(value) -> float | None:
    """Coerce a value to float, returning None on failure."""
    if value is None:
        return None
    try:
        return float(value)
    except (ValueError, TypeError):
        return None
