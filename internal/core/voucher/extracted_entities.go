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
	TotalAmount   float64    `json:"total_amount"`
	NetAmount     float64    `json:"net_amount"`       // total_amount - tds - discount
	PONumber      string     `json:"po_number"`        // PO reference extracted from document
	MatchStatus   string     `json:"match_status"`     // PENDING_MATCH, MATCHED, ORPHAN_INVOICE
	Currency      string     `json:"currency"`
	InvoiceURL    string     `json:"invoice_url"`
	Comments      string     `json:"comments"`
	VendorGSTIN   string     `json:"vendor_gstin,omitempty"`
	BuyerGSTIN    string     `json:"buyer_gstin,omitempty"`
	DocumentID    uuid.UUID  `json:"document_id"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
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
