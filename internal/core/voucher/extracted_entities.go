package voucher

import (
	"time"

	"github.com/google/uuid"
)

// ExtractedInvoice represents a row in the invoice table linked to a document
type ExtractedInvoice struct {
	ID            uuid.UUID  `json:"id"`
	InvID         string    `json:"inv_id"`
	Type          string    `json:"type"`
	BusinessID    uuid.UUID  `json:"business_id,omitempty"`
	PaymentID     *uuid.UUID `json:"payment_id,omitempty"`
	PayeeID       *uuid.UUID `json:"payee_id,omitempty"`
	InvDate       *time.Time `json:"inv_date,omitempty"`
	InvDueDate    *time.Time `json:"inv_due_date,omitempty"`
	CGST          float64    `json:"cgst,omitempty"`
	IGST          float64    `json:"igst,omitempty"`
	SGST          float64    `json:"sgst,omitempty"`
	TotalAmount   float64    `json:"total_amount"`
	Currency      string     `json:"currency"`
	InvoiceURL    string     `json:"invoice_url"`
	Comments      string     `json:"comments"`
	DocumentID    uuid.UUID  `json:"document_id"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// PurchaseOrder represents a row in the purchase_orders table
type PurchaseOrder struct {
	ID           uuid.UUID `json:"id"`
	DocumentID   uuid.UUID `json:"document_id"`
	PONumber     string    `json:"po_number"`
	PODate       string    `json:"po_date"`
	VendorName   string    `json:"vendor_name"`
	VendorGSTIN  string    `json:"vendor_gstin"`
	BuyerGSTIN   string    `json:"buyer_gstin"`
	TotalAmount  float64   `json:"total_amount"`
	Discount     float64   `json:"discount,omitempty"`
	DiscountType string    `json:"discount_type"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
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
