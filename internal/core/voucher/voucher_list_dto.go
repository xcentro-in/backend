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
	TotalAmount    int64             `json:"totalAmount"`    // Net amount in paise
	Status         BillPaymentStatus `json:"status"`         // paid, open, overdue
	VoucherStatus  string            `json:"voucherStatus"`  // draft, finalized, posted, synced
	PaymentStatus  *string           `json:"paymentStatus"`  // Payment transaction status: pending, success, failed, cancelled (null if no payment)
	ApprovalStatus string            `json:"approvalStatus"` // pending, approved, rejected, request_changes (empty if no approval)
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

// UnpaidBillsRequest represents the request to fetch unpaid bills
type UnpaidBillsRequest struct {
	BusinessID string `json:"business_id"` // From header
	Limit      int    `json:"limit"`       // Optional pagination
	Offset     int    `json:"offset"`      // Optional pagination
}

// UnpaidBillItem represents a single unpaid bill
type UnpaidBillItem struct {
	VoucherID     string            `json:"voucher_id"`
	InvoiceNumber string            `json:"invoice_number"`
	TotalAmount   int64             `json:"total_amount"` // Net amount in paise
	DueDate       *time.Time        `json:"due_date"`
	Status        BillPaymentStatus `json:"status"`       // open or overdue
	VendorName    string            `json:"vendor_name"`
	VendorGSTIN   string            `json:"vendor_gstin"`
}

// UnpaidBillsResponse represents the response with unpaid bills
type UnpaidBillsResponse struct {
	Bills      []UnpaidBillItem `json:"bills"`
	TotalCount int              `json:"total_count"`
	Limit      int              `json:"limit"`
	Offset     int              `json:"offset"`
}

// VoucherCountsResponse represents the counts of vouchers by payment status
type VoucherCountsResponse struct {
	All     int `json:"all"`      // Total vouchers (all statuses)
	Paid    int `json:"paid"`     // Payment completed
	Open    int `json:"open"`     // Not paid, due date in future
	Overdue int `json:"overdue"`  // Not paid, due date passed
}
