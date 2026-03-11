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
        try:
            extracted_str = pdftotext.to_text(file_path)
        except Exception as exc:
            logger.warning("pdftotext failed for %s: %s — falling back to extract_data", file_path, exc)
            # Fallback to the high-level helper
            raw = extract_data(file_path, templates=self._templates)
            if raw is None:
                raise TemplateMatchError(f"No template matched for {file_path}")
            return self._map(raw, template_name=raw.get("issuer"))

        # Step 2: Find the first matching template
        for t in self._templates:
            if t.matches_input(extracted_str):
                # t is an InvoiceTemplate object — extract using it
                optimized_str = t.prepare_input(extracted_str)
                raw = t.extract(optimized_str, file_path, pdftotext)
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
            line_items=line_items,
            doc_type=doc_type,
            raw=raw,
        )


def _safe_float(value) -> float | None:
    """Coerce a value to float, returning None on failure."""
    if value is None:
        return None
    try:
        return float(value)
    except (ValueError, TypeError):
        return None
