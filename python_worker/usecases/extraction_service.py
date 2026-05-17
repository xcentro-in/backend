"""Core extraction orchestrator — the main use case.

Coordinates the extraction pipeline:
  1. Download file from S3.
  2. Attempt template extraction (invoice2data).
  3. Validate the result.
  4. Fallback to LLM if template fails or validation fails.
  5. Persist draft_data, raw_extraction, and final status.

Infrastructure errors (S3, LLM API failures) are allowed to propagate
so the caller (main.py) can decide whether to retry.
"""

from __future__ import annotations

import dataclasses
import logging
import re
import time
import uuid
import os
from pathlib import Path

from domain.entities import Document, ExtractionResult
from domain.enums import DocumentStatus, DocumentType, ProcessedBy
from domain.exceptions import ExtractionError, LLMExtractionError, TemplateGenerationError, TemplateMatchError
from domain.ports import DocumentRepository, FileDownloader, LLMExtractor, TemplateExtractor, TemplateRepository
from infrastructure.invoice2data_wrapper import (
    _OVERCAPTURE_REASON_RE,
    narrow_template_on_overcapture,
    perturb_ocr_for_robustness,
    postprocess_template_regexes,
    _sanitize_keywords_to_ocr,
    verify_template_against_history,
    verify_template_against_perturbations,
    verify_template_roundtrip,
)
from infrastructure.ocr import extract_best_ocr
from usecases.validation import run_all_checks

logger = logging.getLogger(__name__)

_FILENAME_COMPONENT_RE = re.compile(r"[^a-z0-9_-]")


def _usage_from(result: ExtractionResult | None) -> dict:
    """Pull LLM token usage dict off result.raw if the LLM extractor populated it."""
    if result is None or not result.raw:
        return {}
    usage = result.raw.get("_usage")
    return usage if isinstance(usage, dict) else {}


def _normalize_ocr_whitespace(text: str) -> str:
    """Collapse runs of horizontal whitespace to a single space per run.

    OCR engines produce inconsistent spacing (especially for table layouts
    where columns are aligned with many spaces). invoice2data keyword matching
    is a literal substring search, so 'TAX INVOICE' only matches if those
    two words appear with exactly one space. This normalization ensures the
    stored OCR and the generated template keywords use the same spacing.

    Newlines are preserved because invoice2data line patterns depend on them.
    """
    if not text:
        return text
    return re.sub(r"[ \t]+", " ", text)


def _layout_slug_from_yaml(yaml_content: str) -> str:
    """Extract and sanitize the 'issuer' field from generated template YAML.

    The 'issuer' field is a layout descriptor like "Tally ERP Tax Invoice" or
    "Zoho Books GST Invoice" — distinct per invoice software/format, not per
    vendor. Using it in the filename means two different layouts from the same
    vendor produce different template files instead of overwriting each other.

    Falls back to a 6-char content hash when issuer is absent or sanitizes to
    empty, guaranteeing uniqueness even for unexpected YAML shapes.
    """
    import hashlib
    try:
        import yaml as _yaml
        data = _yaml.safe_load(yaml_content)
        if isinstance(data, dict):
            issuer = str(data.get("issuer") or "").strip()
            slug = _sanitize_filename_component(issuer)
            if slug:
                return slug[:40]  # cap length so filenames stay readable
    except Exception:
        pass
    # Fallback: first 6 hex chars of the SHA-1 of the YAML content
    h = hashlib.sha1(yaml_content.encode("utf-8", errors="replace")).hexdigest()
    return h[:6]


def _sanitize_filename_component(value: str) -> str:
    """Lower-case + strip anything outside [a-z0-9_-]; cap length.

    Prevents path traversal (no dots, no slashes) and filesystem noise from
    LLM-extracted vendor names. Returns an empty string if nothing survives.
    """
    cleaned = _FILENAME_COMPONENT_RE.sub("_", value.strip().lower())
    cleaned = cleaned.strip("._-")
    return cleaned[:80]


def _normalize_result(result: ExtractionResult) -> ExtractionResult:
    """Apply business-rule normalization to an ExtractionResult before serialization.

    Rules applied (in order):
    1. GST mutual exclusivity: zero out IGST if intra-state (SGST/CGST present),
       zero out SGST/CGST if inter-state (IGST present).
    2. Line item tax_rate default: if a line item has no tax_rate, fall back to the
       document-level gst_rate; if that is also absent, default to 0.
    3. Line item tax_rate distribution: if NO line item has an individual tax_rate but
       the document has a single gst_rate, assign that rate to all line items.
    4. PO invoice_number: purchase orders do not have invoice numbers — clear it.
    5. (Shipping optional — no action needed, already nullable.)
    """
    # --- Rule 1: GST mutual exclusivity ---
    igst = result.igst or 0.0
    sgst = result.sgst or 0.0
    cgst = result.cgst or 0.0

    if igst > 0 and (sgst > 0 or cgst > 0):
        # Both present — ambiguous. Inter-state (IGST) takes precedence.
        logger.warning(
            "Both IGST (%.2f) and SGST/CGST (%.2f/%.2f) are non-zero. "
            "Treating as inter-state: clearing SGST and CGST.",
            igst, sgst, cgst,
        )
        result.sgst = None
        result.cgst = None
    elif igst > 0:
        # Inter-state: clear SGST/CGST even if they were null
        result.sgst = None
        result.cgst = None
    elif sgst > 0 or cgst > 0:
        # Intra-state: clear IGST
        result.igst = None

    # --- Rule 1b: Infer individual GST components from total_gst when no breakdown is available ---
    total_gst = result.total_gst or 0.0
    if total_gst > 0 and not (result.igst or 0.0) and not (result.sgst or 0.0) and not (result.cgst or 0.0):
        # Determine transaction type from first 2 chars of each GSTIN (state code)
        v_state = (result.vendor_gstin or "")[:2].upper()
        b_state = (result.buyer_gstin or "")[:2].upper()
        if v_state and b_state and v_state == b_state:
            # Same state → intra-state: split total GST equally between CGST and SGST
            half = round(total_gst / 2, 2)
            result.cgst = half
            result.sgst = round(total_gst - half, 2)  # handle odd pennies
            logger.info(
                "Inferred intra-state GST from total_gst=%.2f: cgst=%.2f sgst=%.2f",
                total_gst, result.cgst, result.sgst,
            )
        elif v_state and b_state and v_state != b_state:
            # Different states → inter-state: entire amount is IGST
            result.igst = total_gst
            logger.info("Inferred inter-state GST from total_gst=%.2f: igst=%.2f", total_gst, result.igst)
        else:
            # GSTINs missing or incomplete — cannot infer; leave null for reviewer
            logger.info(
                "total_gst=%.2f present but GSTINs missing/incomplete — leaving SGST/CGST/IGST null for reviewer",
                total_gst,
            )

    # --- Rules 2 & 3: line item tax_rate defaulting ---
    doc_rate = result.gst_rate  # may be None

    # Check whether any line item already carries an individual rate
    any_has_rate = any(li.tax_rate is not None for li in result.line_items)

    for li in result.line_items:
        if li.tax_rate is None:
            if not any_has_rate and doc_rate is not None:
                # Rule 3: distribute document-level rate uniformly
                li.tax_rate = doc_rate
            else:
                # Rule 2: default to 0
                li.tax_rate = 0.0

    # --- Rule 4: PO has no invoice number ---
    if result.doc_type == DocumentType.PURCHASE_ORDER:
        result.invoice_number = None

    return result


def _result_to_draft(result: ExtractionResult) -> dict:
    """Normalize and serialize an ExtractionResult into the draft_data JSONB shape."""
    result = _normalize_result(result)

    # Derive gst_type from the (now-normalized) tax components
    gst_type = None
    if result.igst and result.igst > 0:
        gst_type = "INTER_STATE"
    elif (result.sgst and result.sgst > 0) or (result.cgst and result.cgst > 0):
        gst_type = "INTRA_STATE"

    return {
        "invoice_number": result.invoice_number,
        "invoice_date": result.invoice_date,
        "invoice_due_date": result.invoice_due_date,
        "po_number": result.po_number,
        "vendor_name": result.vendor_name,
        "vendor_gstin": result.vendor_gstin,
        "buyer_gstin": result.buyer_gstin,
        "total_amount": result.total_amount,
        "subtotal": result.subtotal,
        "gross_amount": result.gross_amount,
        "discount": result.discount,
        "discount_type": result.discount_type,
        "payment_terms": result.payment_terms,
        "gst_type": gst_type,
        "gst_rate": result.gst_rate,
        "sgst": result.sgst,
        "cgst": result.cgst,
        "igst": result.igst,
        "total_gst": result.total_gst,
        "shipping": result.shipping,
        "doc_type": result.doc_type.value if result.doc_type else None,
        "line_items": [dataclasses.asdict(li) for li in result.line_items],
        "confidence_scores": result.confidence_scores,
    }


class ExtractionService:
    """Application service that drives the full extraction pipeline.

    Dependencies are injected via constructor so the class remains
    framework-agnostic and easily testable.
    """

    def __init__(
        self,
        repo: DocumentRepository,
        template_extractor: TemplateExtractor,
        llm_extractor: LLMExtractor,
        downloader: FileDownloader,
        temp_dir: str = "/tmp/extraction",
        template_dir: str = "/app/templates",
        template_repo: TemplateRepository | None = None,
    ) -> None:
        self._repo = repo
        self._template = template_extractor
        self._llm = llm_extractor
        self._downloader = downloader
        self._temp_dir = temp_dir
        self._template_dir = template_dir
        self._template_repo = template_repo

    # Terminal statuses that must never be overwritten by a stale queue task.
    # The reconciler and crash-recovery reaper can produce duplicate tasks;
    # without this guard a VALIDATED document would revert to FAILED.
    _TERMINAL_STATUSES = frozenset({
        DocumentStatus.VALIDATED,
        DocumentStatus.REVIEW_REQUIRED,
        DocumentStatus.FAILED,
    })

    # -- public entry point --------------------------------------------------

    def process(self, document_id: uuid.UUID) -> None:
        """Run the full extraction + validation pipeline for one document.

        Raises:
            Exception: Infrastructure errors (S3, LLM API) are NOT caught
                       here so the caller can implement retry logic.
        """
        started_at = time.monotonic()
        logger.info("event=extraction.start doc_id=%s", document_id)
        path_taken: str = "unknown"
        final_status: str = "unknown"
        usage: dict = {}
        try:
            path_taken, final_status, usage = self._process_inner(document_id)
        finally:
            duration_ms = int((time.monotonic() - started_at) * 1000)
            prompt_tokens = usage.get("prompt_tokens", 0)
            completion_tokens = usage.get("completion_tokens", 0)
            logger.info(
                "event=extraction.done doc_id=%s duration_ms=%d path=%s status=%s "
                "prompt_tokens=%d completion_tokens=%d",
                document_id, duration_ms, path_taken, final_status,
                prompt_tokens, completion_tokens,
            )

    def _process_inner(self, document_id: uuid.UUID) -> tuple[str, str, dict]:
        """Internal process() body. Returns (path_taken, final_status, llm_usage) for logging."""
        doc = self._repo.get_by_id(document_id)
        if doc is None:
            logger.error("Document %s not found in database — skipping", document_id)
            return "skip", "not_found", {}

        # Stale tasks (from the reconciler re-enqueue or crash-recovery reap)
        # must not reprocess a document that has already reached a terminal
        # state. The Go retry endpoint explicitly resets status → QUEUED before
        # re-enqueuing, so legitimate retries are unaffected.
        if doc.status in self._TERMINAL_STATUSES:
            logger.info(
                "Document %s is already in terminal status %s — skipping stale task",
                document_id, doc.status.value,
            )
            return "skip", f"terminal_{doc.status.value}", {}

        # Mark as processing
        doc.status = DocumentStatus.OCR_PROCESSING
        self._repo.update(doc)

        # Download — may raise on S3 / network errors (retryable)
        local_path = self._downloader.download(doc.s3_path, self._temp_dir)
        logger.info("Downloaded s3_path=%s → %s", doc.s3_path, local_path)

        ocr = extract_best_ocr(local_path)
        logger.info(
            "Selected OCR engine=%s for %s (score=%.2f, pdftotext=%.2f, tesseract=%s): %s",
            ocr.engine,
            doc.id,
            ocr.score,
            ocr.pdftotext_score,
            f"{ocr.tesseract_score:.2f}" if ocr.tesseract_score is not None else "n/a",
            ocr.reason,
        )

        doc.raw_ocr = _normalize_ocr_whitespace(ocr.text)
        self._repo.update(doc)

        # Step 1: Try template extraction
        result = self._try_template(local_path, doc)
        path_taken = "template"

        # Step 2: If template succeeded, validate (skip arithmetic); fallback to LLM on failure
        if result is not None:
            failures = run_all_checks(result, include_arithmetic=False)
            if failures:
                logger.warning(
                    "Template extraction passed but validation failed for %s: %s",
                    document_id,
                    failures,
                )
                result = self._try_llm(local_path, doc)  # may raise LLMExtractionError
                path_taken = "template_then_llm"
            else:
                doc.processed_by = ProcessedBy.INVOICE2DATA
        else:
            result = self._try_llm(local_path, doc)  # may raise LLMExtractionError
            path_taken = "llm"

        # Step 3: Check for non-invoice/PO documents
        if result is not None and result.doc_type in (DocumentType.OTHER, DocumentType.UNKNOWN):
            logger.warning(
                "Document %s classified as %s — marking FAILED (violation)",
                document_id,
                result.doc_type.value,
            )
            doc.status = DocumentStatus.FAILED
            doc.doc_type = result.doc_type
            doc.draft_data = {
                "error": "Document is not an invoice or purchase order",
                "violation": True,
                "detected_type": result.doc_type.value,
            }
            self._repo.update(doc)
            return path_taken, "failed_doc_type", _usage_from(result)

        # Step 4: Final validation & persist
        if result is None:
            logger.error("Both extractors failed for document %s", document_id)
            doc.status = DocumentStatus.REVIEW_REQUIRED
            doc.draft_data = {"error": "All extraction methods failed"}
            self._repo.update(doc)
            return path_taken, "all_failed", {}

        self._finalize(doc, result)
        return path_taken, doc.status.value, _usage_from(result)

    def mark_failed(self, document_id: uuid.UUID, reason: str) -> None:
        """Mark a document as FAILED after exhausting all retries."""
        doc = self._repo.get_by_id(document_id)
        if doc is None:
            logger.error("Cannot mark_failed: document %s not found", document_id)
            return

        # Never demote a document that has already been validated or is
        # waiting for human review — a stale task exhausting retries must
        # not overwrite work the user has already done.
        if doc.status in (DocumentStatus.VALIDATED, DocumentStatus.REVIEW_REQUIRED):
            logger.warning(
                "Refusing to mark_failed document %s: already in status %s",
                document_id, doc.status.value,
            )
            return

        doc.status = DocumentStatus.FAILED
        doc.draft_data = {"error": reason}
        self._repo.update(doc)
        logger.error("Document %s marked FAILED: %s", document_id, reason)

    def generate_template_for_document(
        self, document_id: uuid.UUID, *, force_regen: bool = False,
    ) -> None:
        """Use LLM to generate an invoice2data template after HITL validation.

        ``force_regen`` is set when the HITL feedback loop has flagged the
        existing template as degraded and asked us to regenerate even though a
        working template already exists for this vendor.
        """
        started_at = time.monotonic()
        logger.info(
            "event=template_gen.start doc_id=%s force_regen=%s",
            document_id, force_regen,
        )
        outcome = "unknown"
        try:
            outcome = self._generate_template_inner(document_id, force_regen=force_regen)
        except TemplateGenerationError as exc:
            outcome = exc.reason
            raise
        finally:
            duration_ms = int((time.monotonic() - started_at) * 1000)
            logger.info(
                "event=template_gen.done doc_id=%s duration_ms=%d outcome=%s",
                document_id, duration_ms, outcome,
            )

    def _generate_template_inner(self, document_id: uuid.UUID, *, force_regen: bool = False) -> str:
        """Internal body. Returns a short outcome tag for the done-event log."""
        doc = self._repo.get_by_id(document_id)
        if doc is None:
            logger.error("Cannot generate template: document %s not found", document_id)
            raise TemplateGenerationError("not_found", retryable=False)

        if not doc.raw_ocr:
            logger.error("Cannot generate template for %s: missing raw_ocr", document_id)
            raise TemplateGenerationError("missing_ocr", retryable=False)

        # Normalize whitespace so keywords generated from the OCR are
        # consistent with what invoice2data's literal substring check will see.
        # Old documents were stored before _normalize_ocr_whitespace was added
        # to the extraction pipeline, so we re-normalize here at use time.
        ocr_for_template = _normalize_ocr_whitespace(doc.raw_ocr)

        # Use validated_data (post-HITL) as the target; fallback to draft_data
        target_data = doc.validated_data or doc.draft_data
        if not target_data:
            logger.error("Cannot generate template for %s: missing validated_data and draft_data", document_id)
            raise TemplateGenerationError("missing_target_data", retryable=False)

        # Prevent overwriting a working template unless the HITL feedback loop
        # has explicitly flagged it as degraded — in that case we want to
        # regenerate even though a template already exists.
        if doc.processed_by == ProcessedBy.INVOICE2DATA and doc.i2d_template and not force_regen:
            logger.info(
                "Skipping template generation for %s: already extracted by INVOICE2DATA with template %s",
                document_id, doc.i2d_template,
            )
            return "skip_existing_template"

        logger.info("Generating invoice template for document %s", document_id)
        try:
            # Fetch prior validated extractions from the same template to improve generation accuracy
            historical_examples: list[dict] = []
            if doc.i2d_template:
                historical_examples = self._repo.get_validated_by_template(doc.i2d_template, limit=5)
                if historical_examples:
                    logger.info(
                        "Including %d historical example(s) from template %s in prompt",
                        len(historical_examples), doc.i2d_template,
                    )

            # Pull historical OCR/value pairs so we can prove the template
            # generalises across multiple documents from this vendor — not just
            # the source it was generated from.
            history_pairs: list[dict] = []
            if doc.i2d_template:
                try:
                    history_pairs = self._repo.get_validated_with_ocr_by_template(
                        doc.i2d_template, limit=5
                    )
                except Exception as exc:
                    logger.warning(
                        "Could not fetch historical OCR pairs for %s: %s — "
                        "skipping cross-doc verification",
                        doc.i2d_template, exc,
                    )

            # Generate + verify. Default flow (K=1): one shot; on failure, one
            # retry with the failure fed back to the LLM. K>1: sample K
            # candidates and keep the first that passes; if none pass, fall
            # through to the retry-with-feedback path using the best loser as
            # the prior_attempt context.
            yaml_raw, yaml_content, ok, reason = self._generate_candidates(
                ocr_for_template, target_data, historical_examples, history_pairs,
            )

            if not ok:
                logger.warning(
                    "event=template.verify.failed doc_id=%s reason=%s — retrying with feedback",
                    document_id, reason,
                )
                yaml_retry_raw = self._llm.generate_template(
                    ocr_for_template, target_data,
                    historical_examples=historical_examples,
                    prior_attempt=yaml_raw,
                    prior_error=reason,
                )
                yaml_content_retry = postprocess_template_regexes(yaml_retry_raw)
                yaml_content_retry = _sanitize_keywords_to_ocr(yaml_content_retry, ocr_for_template)
                ok2, reason2 = self._full_template_verify(
                    yaml_content_retry, ocr_for_template, target_data, history_pairs,
                )
                # Try programmatic narrowing on the retry result too.
                if not ok2 and _OVERCAPTURE_REASON_RE.search(reason2):
                    narrowed = narrow_template_on_overcapture(
                        yaml_content_retry, ocr_for_template, target_data,
                    )
                    if narrowed is not None:
                        ok_n, reason_n = self._full_template_verify(
                            narrowed, ocr_for_template, target_data, history_pairs,
                        )
                        if ok_n:
                            logger.info(
                                "event=template.verify.narrowed doc_id=%s "
                                "retry_reason=%s",
                                document_id, reason2,
                            )
                            yaml_content_retry = narrowed
                            ok2 = True
                if not ok2:
                    logger.error(
                        "Template for %s still fails verification after retry "
                        "(first=%s, second=%s) — refusing to save.",
                        document_id, reason, reason2,
                    )
                    raise TemplateGenerationError("verification_failed", retryable=False)
                yaml_content = yaml_content_retry

            # Build filename: {vendor_id}_{layout_slug}_{doc_type}.yml
            #
            # vendor_id   — GSTIN (preferred) or sanitized vendor_name; both are
            #               attacker-controlled so characters are whitelisted and
            #               the resolved path is asserted inside the root.
            # layout_slug — sanitized 'issuer' from the generated YAML, which
            #               describes the invoice layout/software (e.g.
            #               "tally_erp_tax_invoice"). Using the layout slug means
            #               two different invoice formats from the same vendor
            #               get distinct template files instead of overwriting
            #               each other. Falls back to a 6-char content hash when
            #               the issuer is absent or sanitizes to empty.
            vendor_id = str(target_data.get("vendor_gstin") or "").strip()
            if not vendor_id:
                vendor_id = str(target_data.get("vendor_name") or "unknown_vendor")
            vendor_id = _sanitize_filename_component(vendor_id) or "unknown_vendor"

            doc_type = _sanitize_filename_component(
                str(target_data.get("doc_type") or "invoice")
            ) or "invoice"

            layout_slug = _layout_slug_from_yaml(yaml_content)
            filename = f"{vendor_id}_{layout_slug}_{doc_type}.yml"

            if self._template_repo is not None:
                self._template_repo.save(
                    name=filename,
                    content=yaml_content,
                    issuer=str(target_data.get("vendor_gstin") or "") or None,
                    replaced_by="hitl_regen" if force_regen else "initial_or_correction",
                )
                sink = f"db:{filename}"
            else:
                # Fallback: write to filesystem when no DB repo is wired up
                template_root = Path(self._template_dir).resolve()
                template_root.mkdir(parents=True, exist_ok=True)
                filepath = (template_root / filename).resolve()

                # Assert resolved path is strictly inside the templates root.
                try:
                    filepath.relative_to(template_root)
                except ValueError:
                    raise ExtractionError(
                        f"Refusing to write template outside root: {filepath}"
                    )

                with open(filepath, "w", encoding="utf-8") as f:
                    f.write(yaml_content)
                sink = str(filepath)

            # Update document with the generated template name
            doc.i2d_template = filename
            self._repo.update(doc)

            logger.info("Successfully generated template at %s for document %s", sink, document_id)
            return "ok"
        except TemplateGenerationError:
            raise
        except LLMExtractionError as exc:
            logger.error("LLM error generating template for document %s: %s", document_id, exc)
            raise TemplateGenerationError(str(exc), retryable=True) from exc
        except ExtractionError as exc:
            logger.error("Failed to generate template for document %s: %s", document_id, exc)
            raise TemplateGenerationError("path_traversal_refused", retryable=False) from exc
        except OSError as exc:
            logger.error("OS error generating template for document %s: %s", document_id, exc)
            raise TemplateGenerationError(str(exc), retryable=True) from exc
        except Exception as exc:
            logger.error(
                "Unexpected error generating template for document %s: %s",
                document_id, exc, exc_info=True,
            )
            raise TemplateGenerationError("unexpected_error", retryable=True) from exc

    # -- private helpers ------------------------------------------------------

    # Perturbation strictness:
    #   permissive (default) — log perturbation failures but don't reject
    #   strict                — perturbation failure rejects the template
    # The strict mode can be enabled in production once we have observed the
    # baseline failure rate; permissive avoids over-rejecting on day one.
    _STRICT_PERTURBATION = os.environ.get("TEMPLATE_GEN_STRICT_PERTURBATION", "0") == "1"

    # Number of LLM candidates to sample per template-generation attempt. K=1
    # preserves the historical cost profile; K>1 trades extra Gemini calls
    # for higher template quality on critical vendors.
    @staticmethod
    def _candidate_count() -> int:
        try:
            return max(1, int(os.environ.get("TEMPLATE_GEN_CANDIDATES", "1")))
        except ValueError:
            return 1

    def _generate_candidates(
        self,
        source_ocr: str,
        target_data: dict,
        historical_examples: list[dict],
        history_pairs: list[dict],
    ) -> tuple[str, str, bool, str]:
        """Generate up to K candidate templates and return the best.

        Returns (yaml_raw, yaml_postprocessed, ok, reason). When K==1 this is
        a single LLM call followed by verification — identical to the
        pre-existing single-attempt path. When K>1 the first candidate that
        passes full verification short-circuits; if none pass, the candidate
        with the best partial-verify outcome is returned so the caller's
        retry-with-feedback loop has a meaningful prior_attempt to learn from.
        """
        k = self._candidate_count()
        best: tuple[int, str, str, bool, str] | None = None
        for attempt in range(k):
            yaml_raw = self._llm.generate_template(
                source_ocr, target_data, historical_examples=historical_examples,
            )
            yaml_content = postprocess_template_regexes(yaml_raw)
            yaml_content = _sanitize_keywords_to_ocr(yaml_content, source_ocr)
            ok, reason = self._full_template_verify(
                yaml_content, source_ocr, target_data, history_pairs,
            )
            # Programmatic narrowing: if the only failure is the line-wrap
            # continuation greedily capturing trailing content (e.g. a date
            # adjacent to the invoice number), strip the continuation and
            # re-verify. This avoids an extra LLM call for a purely mechanical
            # issue introduced by postprocess_template_regexes.
            if not ok and _OVERCAPTURE_REASON_RE.search(reason):
                narrowed = narrow_template_on_overcapture(
                    yaml_content, source_ocr, target_data,
                )
                if narrowed is not None:
                    ok_n, reason_n = self._full_template_verify(
                        narrowed, source_ocr, target_data, history_pairs,
                    )
                    if ok_n:
                        logger.info(
                            "event=template.verify.narrowed doc attempt=%d "
                            "original_reason=%s",
                            attempt + 1, reason,
                        )
                        yaml_content = narrowed
                        ok, reason = True, "ok_after_narrowing"

            score = self._score_partial(yaml_content, source_ocr, target_data, history_pairs)
            if k > 1:
                logger.info(
                    "event=template.candidate attempt=%d/%d ok=%s score=%d reason=%s",
                    attempt + 1, k, ok, score, reason,
                )
            if ok:
                return yaml_raw, yaml_content, True, reason
            if best is None or score > best[0]:
                best = (score, yaml_raw, yaml_content, ok, reason)

        assert best is not None
        _, yaml_raw, yaml_content, ok, reason = best
        return yaml_raw, yaml_content, ok, reason

    def _score_partial(
        self,
        yaml_content: str,
        source_ocr: str,
        target_data: dict,
        history_pairs: list[dict],
    ) -> int:
        """Cheap score for picking a candidate when no candidate fully passes.

        Counts independently passing checks rather than re-extracting field
        counts (which would require parsing invoice2data internals). Higher
        score = more checks passed = a better starting point for retry.
        """
        score = 0
        ok_round, _ = verify_template_roundtrip(yaml_content, source_ocr, target_data)
        if ok_round:
            score += 10
        if history_pairs:
            ok_h, _ = verify_template_against_history(yaml_content, history_pairs)
            if ok_h:
                score += 5
        ok_p, _ = verify_template_against_perturbations(yaml_content, source_ocr)
        if ok_p:
            score += 2
        return score

    def _full_template_verify(
        self,
        yaml_content: str,
        source_ocr: str,
        target_data: dict,
        history_pairs: list[dict],
    ) -> tuple[bool, str]:
        """Run all generalisation checks against a freshly-generated template.

        Order matters: source roundtrip is cheapest and catches the most
        failures, so we run it first and short-circuit on failure. Historical
        verification only fires when this vendor has prior validated docs;
        perturbation is always tried but soft-fails by default (see
        _STRICT_PERTURBATION).
        """
        ok, reason = verify_template_roundtrip(yaml_content, source_ocr, target_data)
        if not ok:
            return False, reason

        if history_pairs:
            ok_h, reason_h = verify_template_against_history(yaml_content, history_pairs)
            logger.info(
                "event=template.verify.history docs=%d ok=%s reason=%s",
                len(history_pairs), ok_h, reason_h,
            )
            if not ok_h:
                return False, reason_h

        ok_p, reason_p = verify_template_against_perturbations(yaml_content, source_ocr)
        logger.info(
            "event=template.verify.perturbation ok=%s reason=%s strict=%s",
            ok_p, reason_p, self._STRICT_PERTURBATION,
        )
        if not ok_p and self._STRICT_PERTURBATION:
            return False, reason_p

        return True, "ok"

    def _try_template(self, path: str, doc: Document) -> ExtractionResult | None:
        """Attempt invoice2data template matching."""
        try:
            result = self._template.extract(path)
            doc.i2d_template = result.raw.get("template_name")
            logger.info("Template matched: %s", doc.i2d_template)
            logger.info("Template extraction result: \n%s\n\n", result)
            return result
        except TemplateMatchError as exc:
            logger.info(
                "Template extraction failed for %s; falling back to LLM: %s",
                doc.id, exc,
            )
            return None

    def _try_llm(self, path: str, doc: Document) -> ExtractionResult | None:
        """Attempt LLM-based extraction.

        Raises:
            LLMExtractionError: When the LLM API itself fails (retryable).
        """
        doc.status = DocumentStatus.LLM_PROCESSING
        self._repo.update(doc)

        # Let LLMExtractionError propagate so main.py can retry
        result = self._llm.extract(path)
        logger.info("LLM extraction successful for %s", doc.id)
        logger.info("LLM extraction result: \n%s\n\n", result)
        doc.processed_by = ProcessedBy.LLM
        # Preserve full structured LLM JSON for auditing
        if result.raw:
            doc.raw_extraction = result.raw
        return result

    def _finalize(self, doc: Document, result: ExtractionResult) -> None:
        """Validate the final result and persist to the database."""
        failures = run_all_checks(result)

        doc.draft_data = _result_to_draft(result)
        doc.doc_type = result.doc_type

        if failures:
            logger.warning("Final validation issues for %s: %s", doc.id, failures)
            doc.draft_data["validation_failures"] = failures
        doc.status = DocumentStatus.REVIEW_REQUIRED

        self._repo.update(doc)
        logger.info("Document %s finalized with status %s", doc.id, doc.status.value)
