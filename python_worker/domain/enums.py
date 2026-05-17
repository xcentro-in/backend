"""Domain enums for document processing statuses and types."""

from enum import Enum


class DocumentStatus(str, Enum):
    """Lifecycle status of a document in the extraction pipeline."""
    QUEUED = "QUEUED"
    OCR_PROCESSING = "OCR_PROCESSING"
    PENDING_LLM = "PENDING_LLM"
    LLM_PROCESSING = "LLM_PROCESSING"
    REVIEW_REQUIRED = "REVIEW_REQUIRED"
    VALIDATED = "VALIDATED"
    FAILED = "FAILED"


class DocumentType(str, Enum):
    """Classification of the uploaded document."""
    INVOICE = "INVOICE"
    PURCHASE_ORDER = "PURCHASE_ORDER"
    OTHER = "OTHER"
    UNKNOWN = "UNKNOWN"


class ProcessedBy(str, Enum):
    """Identifies which extraction engine processed the document."""
    INVOICE2DATA = "INVOICE2DATA"
    LLM = "LLM"
