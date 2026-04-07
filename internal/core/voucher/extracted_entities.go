package voucher

import (
	"time"

	"github.com/google/uuid"
)

// ExtractedInvoice represents a row in the invoice table linked to a document
type ExtractedInvoice struct {
	ID            uuid.UUID  `json:"id"`
	InvID         string     `json:"inv_id"`
	Type          string     `json:"type"`
	BusinessID    uuid.UUID  `json:"business_id,omitempty"`
	PaymentID     *uuid.UUID `json:"payment_id,omitempty"`
	PayeeID       *uuid.UUID `json:"payee_id,omitempty"`
	InvDate       *time.Time `json:"inv_date,omitempty"`
	InvDueDate    *time.Time `json:"inv_due_date,omitempty"`
	CGST          float64    `json:"cgst,omitempty"`
	IGST          float64    `json:"igst,omitempty"`
	SGST          float64    `json:"sgst,omitempty"`
	TotalAmount     float64    `json:"total_amount"`
	Subtotal        float64    `json:"subtotal,omitempty"`       // pre-tax line item total
	GrossAmount     float64    `json:"gross_amount,omitempty"`   // subtotal + all taxes
	Discount        float64    `json:"discount,omitempty"`       // discount applied before net_amount
	NetAmount       float64    `json:"net_amount"`               // total_amount - tds - discount
	GSTType         string     `json:"gst_type,omitempty"`       // INTER_STATE or INTRA_STATE
	POID            *uuid.UUID `json:"po_id,omitempty"`          // FK to purchase_orders.id; set after successful match
	PONumber        string     `json:"po_number"`                // PO reference extracted from document
	MatchStatus     string     `json:"match_status"`             // PENDING_MATCH, MATCHED, ORPHAN_INVOICE, MISMATCH
	PaymentTerms    string     `json:"payment_terms,omitempty"`  // e.g. "Net 30"
	ExpenseCategory string     `json:"expense_category,omitempty"`
	Currency        string     `json:"currency"`
	InvoiceURL      string     `json:"invoice_url"`
	Comments        string     `json:"comments"`
	VendorGSTIN     string     `json:"vendor_gstin,omitempty"`
	BuyerGSTIN      string     `json:"buyer_gstin,omitempty"`
	DocumentID      uuid.UUID  `json:"document_id"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// PurchaseOrder represents a row in the purchase_orders table
type PurchaseOrder struct {
	ID               uuid.UUID  `json:"id"`
	DocumentID       uuid.UUID  `json:"document_id"`
	BusinessID       *uuid.UUID `json:"business_id,omitempty"`
	PayeeID          *uuid.UUID `json:"payee_id,omitempty"`
	PONumber         string     `json:"po_number"`
	PODate           string     `json:"po_date"`
	VendorName       string     `json:"vendor_name"`
	VendorGSTIN      string     `json:"vendor_gstin"`
	BuyerGSTIN       string     `json:"buyer_gstin"`
	TotalAmount      float64    `json:"total_amount"`
	RemainingBalance float64    `json:"remaining_balance"` // decrements as invoices are matched
	POStatus         string     `json:"po_status"`         // OPEN, PARTIALLY_INVOICED, CLOSED
	Discount         float64    `json:"discount,omitempty"`
	DiscountType     string     `json:"discount_type"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// LineItemDetail represents a typed row from the line_items table
type LineItemDetail struct {
	ID               uuid.UUID `json:"id"`
	PurchaseOrderID  *uuid.UUID `json:"purchase_order_id,omitempty"`
	InvoiceID        *uuid.UUID `json:"invoice_id,omitempty"`
	SKU              string    `json:"sku"`
	Description      string    `json:"description"`
	Quantity         float64   `json:"quantity"`
	QtyReceived      float64   `json:"qty_received"`
	UnitPrice        int64     `json:"unit_price"`   // in paise
	TaxRate          float64   `json:"tax_rate"`
	TaxAmount        int64     `json:"tax_amount"`   // in paise
	TotalAmount      int64     `json:"total_amount"` // in paise
}

// MatchResult represents a row in the match_results audit table
type MatchResult struct {
	ID               uuid.UUID  `json:"id"`
	InvoiceID        uuid.UUID  `json:"invoice_id"`
	PurchaseOrderID  *uuid.UUID `json:"purchase_order_id,omitempty"`
	MatchStatus      string     `json:"match_status"`
	MatchDetails     map[string]interface{} `json:"match_details,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

// Listing Filters

type DocumentListFilter struct {
	Status   *DocumentStatus `json:"status,omitempty"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
}

type ExtractedInvoiceListFilter struct {
	BusinessID *uuid.UUID `json:"business_id,omitempty"`
	Page       int        `json:"page"`
	PageSize   int        `json:"page_size"`
}

type PurchaseOrderListFilter struct {
	DocumentID *uuid.UUID `json:"document_id,omitempty"`
	Page       int        `json:"page"`
	PageSize   int        `json:"page_size"`
}

// PaginatedResponse is a generic wrapper for paginated data
type PaginatedResponse[T any] struct {
	Items      []T   `json:"items"`
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalPages int   `json:"total_pages"`
}

// MatchResultSummary is the flattened view returned by GET /vouchers/match-results.
// It joins match_results → invoice → purchase_orders into one response row so the
// frontend can render both the summary table and the expandable detail panel.
type MatchResultSummary struct {
	// Identity
	MatchResultID   uuid.UUID  `json:"match_result_id"`
	InvoiceID       uuid.UUID  `json:"invoice_id"`
	PurchaseOrderID *uuid.UUID `json:"purchase_order_id,omitempty"`

	// Summary columns
	PONumber      string  `json:"po_number"`
	InvoiceNumber string  `json:"invoice_number"`
	VendorName    string  `json:"vendor_name"`
	PODate        string  `json:"po_date"`
	InvoiceDate   *time.Time `json:"invoice_date,omitempty"`
	POAmount      float64 `json:"po_amount"`
	InvoiceAmount float64 `json:"invoice_amount"` // net_amount
	Variance      float64 `json:"variance"`        // invoice_amount - po_amount

	// Match outcome
	MatchStatus string                 `json:"match_status"`
	MatchNotes  map[string]interface{} `json:"match_notes,omitempty"`
	MatchedAt   time.Time              `json:"matched_at"`

	// PO detail fields
	PORemainingBalance float64    `json:"po_remaining_balance"`
	POStatus           string     `json:"po_status"`
	PODocumentID       *uuid.UUID `json:"po_document_id,omitempty"`

	// Invoice detail fields
	InvoiceNetAmount   float64    `json:"invoice_net_amount"`
	InvoiceTotalAmount float64    `json:"invoice_total_amount"`
	InvoiceDocumentID  *uuid.UUID `json:"invoice_document_id,omitempty"`
}

// MatchResultListFilter controls pagination and scoping for ListMatchResults
type MatchResultListFilter struct {
	BusinessID *uuid.UUID `json:"business_id,omitempty"`
	Page       int        `json:"page"`
	PageSize   int        `json:"page_size"`
}
