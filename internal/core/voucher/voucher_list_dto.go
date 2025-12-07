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
	Status        BillPaymentStatus `json:"status"` // open or overdue
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
	All     int `json:"all"`     // Total vouchers (all statuses)
	Paid    int `json:"paid"`    // Payment completed
	Open    int `json:"open"`    // Not paid, due date in future
	Overdue int `json:"overdue"` // Not paid, due date passed
}

// CalendarBillItem represents a single bill in the calendar view
// Used for displaying bills month by month with high-level details
type CalendarBillItem struct {
	VoucherID     string     `json:"voucher_id"`
	VoucherNumber string     `json:"voucher_number"` // PV-2024-00001
	InvoiceNumber string     `json:"invoice_number"` // Vendor's invoice number
	DueDate       *time.Time `json:"due_date"`       // Invoice due date
	GrossAmount   int64      `json:"gross_amount"`   // Total amount before deductions (in paise)
	NetAmount     int64      `json:"net_amount"`     // Net amount after deductions (in paise)
	PayeeID       string     `json:"payee_id"`       // Payee ID (part of relation composite key)
	AccountID     string     `json:"account_id"`     // Account ID (part of relation composite key)
	PayeeName     string     `json:"payee_name"`     // Display name from business_payee_relation
	Description   string     `json:"description"`    // Bill description
	VoucherStatus string     `json:"voucher_status"` // draft, finalized, posted, synced
	PaymentStatus *string    `json:"payment_status"` // Payment transaction status (null if no payment)
}

// CalendarViewRequest represents the request for calendar view
type CalendarViewRequest struct {
	BusinessID string `json:"business_id"` // From header
	Year       int    `json:"year"`        // Year to fetch (e.g., 2024)
	Month      int    `json:"month"`       // Month to fetch (1-12)
}

// CalendarViewResponse represents the response for calendar view
type CalendarViewResponse struct {
	Year  int                `json:"year"`
	Month int                `json:"month"`
	Bills []CalendarBillItem `json:"bills"`
	Count int                `json:"count"`
}
