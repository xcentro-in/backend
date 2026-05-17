package voucher

import (
	"time"

	"github.com/google/uuid"
)

// DocumentStatus represents the lifecycle status of a voucher document in the extraction pipeline
type DocumentStatus string

const (
	DocStatusQueued         DocumentStatus = "QUEUED"
	DocStatusOCRProcessing  DocumentStatus = "OCR_PROCESSING"
	DocStatusPendingLLM     DocumentStatus = "PENDING_LLM"
	DocStatusLLMProcessing  DocumentStatus = "LLM_PROCESSING"
	DocStatusReviewRequired DocumentStatus = "REVIEW_REQUIRED"
	DocStatusValidated      DocumentStatus = "VALIDATED"
	DocStatusFailed         DocumentStatus = "FAILED"
)

// DocumentType represents the classification of an uploaded document
type DocumentType string

const (
	DocTypeInvoice       DocumentType = "INVOICE"
	DocTypePurchaseOrder DocumentType = "PURCHASE_ORDER"
	DocTypeUnknown       DocumentType = "UNKNOWN"
)

// Invoice match status values — stored in invoice.match_status column
const (
	InvoiceMatchPending  = "PENDING_MATCH"  // default; awaiting matching
	InvoiceMatchMatched  = "MATCHED"         // fully matched to a PO
	InvoiceMatchOrphan   = "ORPHAN_INVOICE"  // no matching PO found
	InvoiceMatchMismatch = "MISMATCH"        // PO found but price/qty variance; flagged for Procurement Manager Review
)

// PO status values — stored in purchase_orders.po_status column
const (
	POStatusOpen              = "OPEN"               // no invoices matched yet
	POStatusPartiallyInvoiced = "PARTIALLY_INVOICED"  // some invoices matched, balance remains
	POStatusClosed            = "CLOSED"              // fully invoiced, remaining_balance = 0
)


// VoucherDocument represents a row in the voucher_documents table.
// It tracks the lifecycle of an uploaded document from ingestion through extraction to validation.
type VoucherDocument struct {
	ID             uuid.UUID              `json:"id"`
	BusinessID     *uuid.UUID             `json:"business_id,omitempty"` // owning business — enforced for authorization
	S3Path         string                 `json:"s3_path"`
	DocType        string                 `json:"doc_type"`
	Status         DocumentStatus         `json:"status"`
	RawOCR         string                 `json:"raw_ocr,omitempty"`
	RawExtraction  map[string]interface{} `json:"raw_extraction,omitempty"` // full structured LLM JSON
	DraftData      map[string]interface{} `json:"draft_data,omitempty"`
	I2DTemplate    string                 `json:"i2d_template"`
	ProcessedBy    string                 `json:"processed_by"`
	ValidatedData  map[string]interface{} `json:"validated_data,omitempty"`
	FileHash       string                 `json:"file_hash,omitempty"` // SHA-256 hex for duplicate detection
	UploadedAt     time.Time              `json:"uploaded_at"`
}
