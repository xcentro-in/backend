"""invoice2data wrapper implementing the TemplateExtractor port.

Uses a custom extraction loop (instead of the high-level extract_data helper)
so we can capture the matched template's name reliably.
"""

from __future__ import annotations

import logging
import os

from invoice2data import extract_data
from invoice2data.extract.loader import read_templates
from invoice2data.input import pdftotext

from domain.entities import ExtractionResult, LineItem
from domain.enums import DocumentType
from domain.exceptions import TemplateMatchError
from domain.ports import TemplateExtractor

logger = logging.getLogger(__name__)


class Invoice2DataExtractor(TemplateExtractor):
    """Wraps the invoice2data library for template-based extraction."""

    def __init__(self, template_dir: str) -> None:
        if not os.path.isdir(template_dir):
            logger.warning("Template directory not found: %s — using built-in templates", template_dir)
            self._templates = read_templates()
        else:
            self._templates = read_templates(template_dir)
            logger.info("Loaded %d templates from %s", len(self._templates), template_dir)

    def extract(self, file_path: str) -> ExtractionResult:
        """Extract data with template matching, capturing the template name.

        We iterate over templates manually (mirroring invoice2data's own
        extract_data logic) so we can reliably capture the matched template's
        ``template_name`` or ``issuer`` field.
        """
        # Step 1: Extract text from PDF using pdftotext
        extracted_str = ""
        try:
            raw_bytes = pdftotext.to_text(file_path)
            if isinstance(raw_bytes, bytes):
                extracted_str = raw_bytes.decode("utf-8", errors="replace")
        except Exception as exc:
            logger.warning("pdftotext failed for %s: %s", file_path, exc)

        extracted_str = extracted_str.strip().replace("\x00", "")

        # Target fallback: if pdftotext returns empty (e.g. scanned PDF), try tesseract
        if not extracted_str:
            logger.info("Empty text from pdftotext. Attempting tesseract OCR for %s", file_path)
            try:
                from invoice2data.input import tesseract
                raw_bytes = tesseract.to_text(file_path)
                if isinstance(raw_bytes, bytes):
                    extracted_str = raw_bytes.decode("utf-8", errors="replace")
                extracted_str = extracted_str.strip().replace("\x00", "")
            except Exception as t_exc:
                logger.warning("tesseract OCR failed for %s: %s", file_path, t_exc)

        if not extracted_str:
            logger.warning("No OCR text extracted from %s. Templates will fail to match.", file_path)

        # Step 2: Find the first matching template
        for t in self._templates:
            if t.matches_input(extracted_str):
                # t is an InvoiceTemplate object — extract using it
                optimized_str = t.prepare_input(extracted_str)
                try:
                    raw = t.extract(optimized_str, file_path, pdftotext)
                except Exception as exc:
                    logger.warning("Template %s matched but extraction failed: %s", t.get('issuer', 'unknown'), exc)
                    continue

                if raw is not None:
                    # Capture the template name from the template object
                    tpl_name = t.get("template_name", None) or t.get("issuer", None) or str(t)
                    logger.info("Template matched: %s", tpl_name)
                    return self._map(raw, template_name=tpl_name)

        raise TemplateMatchError(f"No template matched for {file_path}")

    # -- mapping helpers ------------------------------------------------------

    @staticmethod
    def _map(raw: dict, template_name: str | None = None) -> ExtractionResult:
        """Convert invoice2data dict → domain ExtractionResult."""
        line_items: list[LineItem] = []
        for li in raw.get("lines", []):
            line_items.append(
                LineItem(
                    description=li.get("desc") or li.get("description"),
                    quantity=_safe_float(li.get("qty") or li.get("quantity")),
                    unit_price=_safe_float(li.get("price") or li.get("unit_price")),
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
            po_number=raw.get("po_number"),
            vendor_name=raw.get("issuer"),
            vendor_gstin=raw.get("vendor_gstin") or raw.get("gstin"),
            buyer_gstin=raw.get("buyer_gstin"),
            total_amount=_safe_float(raw.get("amount")),
            discount=_safe_float(raw.get("discount")),
            discount_type=raw.get("discount_type"),
            sgst=_safe_float(raw.get("sgst")),
            cgst=_safe_float(raw.get("cgst")),
            igst=_safe_float(raw.get("igst")),
            shipping=_safe_float(raw.get("shipping") or raw.get("freight")),
            line_items=line_items,
            doc_type=doc_type,
            raw=raw,
            confidence_scores=confidence_scores,
        )


def _safe_float(value) -> float | None:
    """Coerce a value to float, returning None on failure."""
    if value is None:
        return None
    try:
        return float(value)
    except (ValueError, TypeError):
        return None
