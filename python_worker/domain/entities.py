"""Domain entities representing core business objects."""

from __future__ import annotations

import uuid
from dataclasses import dataclass, field
from typing import Any

from domain.enums import DocumentStatus, DocumentType, ProcessedBy


@dataclass
class LineItem:
    """A single line item extracted from an invoice or purchase order."""
    description: str | None = None
    quantity: float | None = None
    unit_price: float | None = None
    tax: float | None = None
    amount: float | None = None


@dataclass
class ExtractionResult:
    """Structured output from either invoice2data or LLM extraction.

    This is the canonical shape that both extractors must produce
    before it enters validation.
    """
    invoice_number: str | None = None
    invoice_date: str | None = None
    po_number: str | None = None
    vendor_name: str | None = None
    vendor_gstin: str | None = None
    buyer_gstin: str | None = None
    total_amount: float | None = None
    discount: float | None = None
    discount_type: str | None = None  # 'FLAT' or 'PERCENTAGE'
    line_items: list[LineItem] = field(default_factory=list)
    doc_type: DocumentType = DocumentType.UNKNOWN
    raw: dict[str, Any] = field(default_factory=dict)


@dataclass
class Document:
    """Aggregate root for the documents table."""
    id: uuid.UUID
    s3_path: str
    doc_type: DocumentType = DocumentType.UNKNOWN
    status: DocumentStatus = DocumentStatus.QUEUED
    raw_extraction: dict[str, Any] | None = None
    draft_data: dict[str, Any] | None = None
    i2d_template: str | None = None
    processed_by: ProcessedBy | None = None
