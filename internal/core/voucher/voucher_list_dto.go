package voucher

import "time"

// BillPaymentStatus represents the payment status from a bill perspective
type BillPaymentStatus string

const (
	BillStatusPaid    BillPaymentStatus = "paid"    // Payment completed successfully
	BillStatusOpen    BillPaymentStatus = "open"    // Not paid, due date in future
	BillStatusOverdue BillPaymentStatus = "overdue" // Not paid, due date passed
)

// VoucherListItem represents a single voucher/bill in the list view
// Used for displaying bills with payment status
type VoucherListItem struct {
	VoucherID      string            `json:"voucherId"`
	VoucherNumber  string            `json:"voucherNumber"`  // PV-2024-00001
	InvoiceNumber  string            `json:"invoiceNumber"`  // Vendor's invoice number
	InvoiceDueDate *time.Time        `json:"invoiceDueDate"` // When payment is due
	PayeeName      string            `json:"payeeName"`      // Payee display name
	TotalAmount    float64           `json:"totalAmount"`    // Net amount in rupees
	Status         BillPaymentStatus `json:"status"`         // paid, open, overdue
	VoucherStatus  string            `json:"voucherStatus"`  // draft, finalized, posted, synced
	PaymentDate    *time.Time        `json:"paymentDate"`    // When payment was made (if paid)
}

// VoucherListRequest represents pagination and filter parameters
type VoucherListRequest struct {
	BusinessID string             `json:"business_id"` // From header
	Status     *BillPaymentStatus `json:"status"`      // Filter by payment status (optional)
	Limit      int                `json:"limit"`       // Page size
	Offset     int                `json:"offset"`      // Pagination offset
}

// VoucherListResponse represents the paginated response
type VoucherListResponse struct {
	Vouchers   []VoucherListItem `json:"vouchers"`
	TotalCount int               `json:"total_count"`
	Limit      int               `json:"limit"`
	Offset     int               `json:"offset"`
}
