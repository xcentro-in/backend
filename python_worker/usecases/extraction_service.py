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
import uuid
import os

from invoice2data.input import pdftotext
from domain.entities import Document, ExtractionResult
from domain.enums import DocumentStatus, DocumentType, ProcessedBy
from domain.exceptions import ExtractionError, LLMExtractionError, TemplateMatchError
from domain.ports import DocumentRepository, FileDownloader, LLMExtractor, TemplateExtractor
from usecases.validation import run_all_checks

logger = logging.getLogger(__name__)


def _result_to_draft(result: ExtractionResult) -> dict:
    """Serialize an ExtractionResult into the draft_data JSONB shape."""
    return {
        "invoice_number": result.invoice_number,
        "invoice_date": result.invoice_date,
        "po_number": result.po_number,
        "vendor_name": result.vendor_name,
        "vendor_gstin": result.vendor_gstin,
        "buyer_gstin": result.buyer_gstin,
        "total_amount": result.total_amount,
        "discount": result.discount,
        "discount_type": result.discount_type,
        "sgst": result.sgst,
        "cgst": result.cgst,
        "igst": result.igst,
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
    ) -> None:
        self._repo = repo
        self._template = template_extractor
        self._llm = llm_extractor
        self._downloader = downloader
        self._temp_dir = temp_dir
        self._template_dir = template_dir

    # -- public entry point --------------------------------------------------

    def process(self, document_id: uuid.UUID) -> None:
        """Run the full extraction + validation pipeline for one document.

        Raises:
            Exception: Infrastructure errors (S3, LLM API) are NOT caught
                       here so the caller can implement retry logic.
        """
        doc = self._repo.get_by_id(document_id)
        if doc is None:
            logger.error("Document %s not found in database — skipping", document_id)
            return

        # Mark as processing
        doc.status = DocumentStatus.OCR_PROCESSING
        self._repo.update(doc)

        # Download — may raise on S3 / network errors (retryable)
        local_path = self._downloader.download(doc.s3_path, self._temp_dir)
        logger.info("Downloaded s3_path=%s → %s", doc.s3_path, local_path)

        # Extract RAW OCR early
        try:
            raw = pdftotext.to_text(local_path)
            if isinstance(raw, bytes):
                raw = raw.decode("utf-8", errors="replace")
            
            # Strip null bytes — psycopg2 rejects \x00 in TEXT columns
            raw = raw.replace("\x00", "").strip()

            if not raw:
                logger.info("pdftotext returned empty text for %s. Attempting tesseract OCR fallback.", doc.id)
                try:
                    from invoice2data.input import tesseract
                    raw_bytes = tesseract.to_text(local_path)
                    if isinstance(raw_bytes, bytes):
                        raw = raw_bytes.decode("utf-8", errors="replace")
                    raw = raw.replace("\x00", "").strip()
                except Exception as t_exc:
                    logger.warning("tesseract OCR failed for %s: %s", doc.id, t_exc)

            doc.raw_ocr = raw
            self._repo.update(doc)
        except Exception as exc:
            logger.warning("Failed to extract raw OCR for %s: %s", doc.id, exc)

        # Step 1: Try template extraction
        result = self._try_template(local_path, doc)

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
            else:
                doc.processed_by = ProcessedBy.INVOICE2DATA
        else:
            result = self._try_llm(local_path, doc)  # may raise LLMExtractionError

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
            return

        # Step 4: Final validation & persist
        if result is None:
            logger.error("Both extractors failed for document %s", document_id)
            doc.status = DocumentStatus.REVIEW_REQUIRED
            doc.draft_data = {"error": "All extraction methods failed"}
            self._repo.update(doc)
            return

        self._finalize(doc, result)

    def mark_failed(self, document_id: uuid.UUID, reason: str) -> None:
        """Mark a document as FAILED after exhausting all retries."""
        doc = self._repo.get_by_id(document_id)
        if doc is None:
            logger.error("Cannot mark_failed: document %s not found", document_id)
            return

        doc.status = DocumentStatus.FAILED
        doc.draft_data = {"error": reason}
        self._repo.update(doc)
        logger.error("Document %s marked FAILED: %s", document_id, reason)

    def generate_template_for_document(self, document_id: uuid.UUID) -> None:
        """Use LLM to generate an invoice2data template after HITL validation."""
        doc = self._repo.get_by_id(document_id)
        if doc is None:
            logger.error("Cannot generate template: document %s not found", document_id)
            return

        if not doc.raw_ocr:
            logger.error("Cannot generate template for %s: missing raw_ocr", document_id)
            return

        # Use validated_data (post-HITL) as the target; fallback to draft_data
        target_data = doc.validated_data or doc.draft_data
        if not target_data:
            logger.error("Cannot generate template for %s: missing validated_data and draft_data", document_id)
            return

        # Prevent overwriting a working template: if extraction was done by
        # INVOICE2DATA and a template already exists, skip regeneration.
        if doc.processed_by == ProcessedBy.INVOICE2DATA and doc.i2d_template:
            logger.info(
                "Skipping template generation for %s: already extracted by INVOICE2DATA with template %s",
                document_id, doc.i2d_template,
            )
            return

        logger.info("Generating invoice template for document %s", document_id)
        try:
            yaml_content = self._llm.generate_template(doc.raw_ocr, target_data)

            # Build filename: prefer vendor_gstin, fallback to vendor_name
            vendor_id = str(target_data.get("vendor_gstin") or "").strip()
            if not vendor_id:
                vendor_id = str(target_data.get("vendor_name") or "unknown_vendor")
            vendor_id = vendor_id.replace(" ", "_").replace("/", "").lower()

            doc_type = str(target_data.get("doc_type") or "invoice").lower()
            filename = f"{vendor_id}_{doc_type}.yml"
            filepath = os.path.join(self._template_dir, filename)

            # Ensure templates dir exists
            os.makedirs(self._template_dir, exist_ok=True)

            with open(filepath, "w", encoding="utf-8") as f:
                f.write(yaml_content)

            # Update document with the generated template name
            doc.i2d_template = filename
            self._repo.update(doc)

            logger.info("Successfully generated template at %s for document %s", filepath, document_id)
        except Exception as exc:
            logger.error("Failed to generate template for document %s: %s", document_id, exc)

    # -- private helpers ------------------------------------------------------

    def _try_template(self, path: str, doc: Document) -> ExtractionResult | None:
        """Attempt invoice2data template matching."""
        try:
            result = self._template.extract(path)
            doc.i2d_template = result.raw.get("template_name")
            logger.info("Template matched: %s", doc.i2d_template)
            logger.info("Template extraction result: \n%s\n\n", result)
            return result
        except TemplateMatchError:
            logger.info("No template matched for %s", doc.id)
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
