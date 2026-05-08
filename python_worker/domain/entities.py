"""Domain entities representing core business objects."""

from __future__ import annotations

import uuid
from dataclasses import dataclass, field
from typing import Any

from domain.enums import DocumentStatus, DocumentType, ProcessedBy


@dataclass
class LineItem:
    """A single line item extracted from an invoice or purchase order."""
    sku: str | None = None           # item code / part number / HSN code
    description: str | None = None
    quantity: float | None = None
    unit_price: float | None = None
    tax_rate: float | None = None    # GST/tax percentage (e.g. 18 for 18%)
    tax: float | None = None         # absolute tax amount in currency units
    amount: float | None = None


@dataclass
class ExtractionResult:
    """Structured output from either invoice2data or LLM extraction.

    This is the canonical shape that both extractors must produce
    before it enters validation.
    """
    invoice_number: str | None = None
    invoice_date: str | None = None
    invoice_due_date: str | None = None
    po_number: str | None = None
    vendor_name: str | None = None
    vendor_gstin: str | None = None
    buyer_gstin: str | None = None
    total_amount: float | None = None
    subtotal: float | None = None       # pre-tax line item total
    gross_amount: float | None = None   # subtotal + all taxes
    discount: float | None = None
    discount_type: str | None = None    # 'FLAT' or 'PERCENTAGE'
    payment_terms: str | None = None    # e.g. "Net 30", "Net 60"
    gst_rate: float | None = None       # document-level GST rate (e.g. 18 for 18%), used when line items share a single rate
    sgst: float | None = None
    cgst: float | None = None
    igst: float | None = None
    total_gst: float | None = None      # lump-sum GST when only a total is shown (no SGST/CGST/IGST breakdown)
    shipping: float | None = None
    line_items: list[LineItem] = field(default_factory=list)
    doc_type: DocumentType = DocumentType.UNKNOWN
    raw: dict[str, Any] = field(default_factory=dict)
    confidence_scores: dict[str, float] = field(default_factory=dict)


@dataclass
class Document:
    """Aggregate root for the documents table."""
    id: uuid.UUID
    s3_path: str
    doc_type: DocumentType = DocumentType.UNKNOWN
    status: DocumentStatus = DocumentStatus.QUEUED
    raw_ocr: str | None = None
    raw_extraction: dict[str, Any] | None = None  # full structured LLM JSON output
    draft_data: dict[str, Any] | None = None
    validated_data: dict[str, Any] | None = None
    i2d_template: str | None = None
    processed_by: ProcessedBy | None = None

