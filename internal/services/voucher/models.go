package voucher

import (
	"time"

	"github.com/google/uuid"
	"xcentro.in/internal/core/voucher"
)

// CreateDraftVoucherRequest represents the request to create a draft voucher
type CreateDraftVoucherRequest struct {
	// Business context
	BusinessID uuid.UUID `json:"business_id"`
	UserID     uuid.UUID `json:"user_id"` // created_by

	// Payee reference
	PayeeID   uuid.UUID `json:"payee_id"`
	AccountID uuid.UUID `json:"account_id"`

	// Invoice details
	InvoiceNumber  string     `json:"invoice_number"`
	InvoiceDate    *time.Time `json:"invoice_date,omitempty"`
	InvoiceDueDate *time.Time `json:"invoice_due_date,omitempty"`

	// Amounts (all in paise)
	GrossAmount    int64   `json:"gross_amount"`
	NetAmount      int64   `json:"net_amount"`
	Currency       string  `json:"currency"` // Default: INR
	GSTAmount      int64   `json:"gst_amount"`
	GSTRate        float64 `json:"gst_rate,omitempty"`
	CGSTAmount     int64   `json:"cgst_amount,omitempty"`
	SGSTAmount     int64   `json:"sgst_amount,omitempty"`
	IGSTAmount     int64   `json:"igst_amount,omitempty"`
	TDSAmount      int64   `json:"tds_amount"`
	TDSRate        float64 `json:"tds_rate,omitempty"`
	TDSSection     string  `json:"tds_section,omitempty"`
	FeeAmount      int64   `json:"fee_amount,omitempty"`
	DiscountAmount int64   `json:"discount_amount,omitempty"`

	// Additional details
	FinancialDetails map[string]interface{} `json:"financial_details,omitempty"`
	Attachments      map[string]interface{} `json:"attachments,omitempty"`
	ExpenseCategory  string                 `json:"expense_category,omitempty"`
	Description      string                 `json:"description,omitempty"`

	// General ledger codes
	GLExpenseAccount string `json:"gl_expense_account,omitempty"`
	GLBankAccount    string `json:"gl_bank_account,omitempty"`
	CostCenter       string `json:"cost_center,omitempty"`

	// Metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// CreateDraftVoucherResponse represents the response after creating a draft voucher
type CreateDraftVoucherResponse struct {
	VoucherID        uuid.UUID             `json:"voucher_id"`
	VoucherNumber    string                `json:"voucher_number"`
	VoucherDate      time.Time             `json:"voucher_date"`
	FiscalYear       string                `json:"fiscal_year"`
	AccountingPeriod string                `json:"accounting_period"`
	Status           voucher.VoucherStatus `json:"status"`
	ERPSyncStatus    voucher.ERPSyncStatus `json:"erp_sync_status"`
	Message          string                `json:"message"`
}

// FinalizeVoucherRequest represents the request to finalize a voucher
type FinalizeVoucherRequest struct {
	VoucherID uuid.UUID `json:"voucher_id"`
	PaymentID uuid.UUID `json:"payment_id"`
}

// FinalizeVoucherResponse represents the response after finalizing a voucher
type FinalizeVoucherResponse struct {
	VoucherID     uuid.UUID             `json:"voucher_id"`
	VoucherNumber string                `json:"voucher_number"`
	Status        voucher.VoucherStatus `json:"status"`
	PaymentID     uuid.UUID             `json:"payment_id"`
	Message       string                `json:"message"`
}

// VoucherDetailsResponse represents enriched voucher details with payee information
type VoucherDetailsResponse struct {
	// Voucher details
	VoucherID        uuid.UUID             `json:"voucher_id"`
	VoucherNumber    string                `json:"voucher_number"`
	VoucherDate      time.Time             `json:"voucher_date"`
	FiscalYear       string                `json:"fiscal_year"`
	AccountingPeriod string                `json:"accounting_period"`
	VoucherType      voucher.VoucherType   `json:"voucher_type"`
	Status           voucher.VoucherStatus `json:"status"`

	// Payee information (enriched)
	PayeeName  string `json:"payee_name"`
	PayeeGSTIN string `json:"payee_gstin,omitempty"`
	PayeeType  string `json:"payee_type"`

	// Bank account information (enriched)
	BankAccountMasked string `json:"bank_account_masked"` // ****3456
	IFSCCode          string `json:"ifsc_code"`
	BankName          string `json:"bank_name"`

	// Invoice details
	InvoiceNumber  string     `json:"invoice_number"`
	InvoiceDate    *time.Time `json:"invoice_date,omitempty"`
	InvoiceDueDate *time.Time `json:"invoice_due_date,omitempty"`

	// Amounts (in paise)
	GrossAmount    int64   `json:"gross_amount"`
	NetAmount      int64   `json:"net_amount"`
	Currency       string  `json:"currency"`
	GSTAmount      int64   `json:"gst_amount"`
	GSTRate        float64 `json:"gst_rate,omitempty"`
	CGSTAmount     int64   `json:"cgst_amount,omitempty"`
	SGSTAmount     int64   `json:"sgst_amount,omitempty"`
	IGSTAmount     int64   `json:"igst_amount,omitempty"`
	TDSAmount      int64   `json:"tds_amount"`
	TDSRate        float64 `json:"tds_rate,omitempty"`
	TDSSection     string  `json:"tds_section,omitempty"`
	FeeAmount      int64   `json:"fee_amount,omitempty"`
	DiscountAmount int64   `json:"discount_amount,omitempty"`

	// Additional details
	FinancialDetails map[string]interface{} `json:"financial_details,omitempty"`
	Attachments      map[string]interface{} `json:"attachments,omitempty"`
	ExpenseCategory  string                 `json:"expense_category,omitempty"`
	Description      string                 `json:"description,omitempty"`

	// Payment link
	PaymentID *uuid.UUID `json:"payment_id,omitempty"`

	// ERP integration
	ERPSyncStatus   voucher.ERPSyncStatus `json:"erp_sync_status"`
	ERPReferenceID  string                `json:"erp_reference_id,omitempty"`
	ERPPostingDate  *time.Time            `json:"erp_posting_date,omitempty"`
	ERPLastSyncAt   *time.Time            `json:"erp_last_sync_at,omitempty"`
	ERPSyncAttempts int                   `json:"erp_sync_attempts"`

	// General ledger
	GLExpenseAccount string `json:"gl_expense_account,omitempty"`
	GLBankAccount    string `json:"gl_bank_account,omitempty"`
	CostCenter       string `json:"cost_center,omitempty"`

	// Timestamps
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// VoucherListItem represents a summary item in voucher list
type VoucherListItem struct {
	VoucherID       uuid.UUID             `json:"voucher_id"`
	VoucherNumber   string                `json:"voucher_number"`
	VoucherDate     time.Time             `json:"voucher_date"`
	FiscalYear      string                `json:"fiscal_year"`
	PayeeName       string                `json:"payee_name"`
	InvoiceNumber   string                `json:"invoice_number"`
	NetAmount       int64                 `json:"net_amount"`
	Currency        string                `json:"currency"`
	Status          voucher.VoucherStatus `json:"status"`
	ERPSyncStatus   voucher.ERPSyncStatus `json:"erp_sync_status"`
	ExpenseCategory string                `json:"expense_category,omitempty"`
	CreatedAt       time.Time             `json:"created_at"`
}

// VoucherListResponse represents paginated list of vouchers
type VoucherListResponse struct {
	Vouchers   []VoucherListItem         `json:"vouchers"`
	Pagination *voucher.PaginationResult `json:"pagination"`
	Summary    *VoucherListSummary       `json:"summary,omitempty"`
}

// VoucherListSummary provides aggregate statistics
type VoucherListSummary struct {
	TotalVouchers    int64 `json:"total_vouchers"`
	TotalAmount      int64 `json:"total_amount"` // Sum of net amounts
	DraftCount       int64 `json:"draft_count"`
	FinalizedCount   int64 `json:"finalized_count"`
	SyncedCount      int64 `json:"synced_count"`
	PendingSyncCount int64 `json:"pending_sync_count"`
}

// CreateMultiVouchersRequest represents request to create multiple vouchers for multi-bill payment
type CreateMultiVouchersRequest struct {
	BusinessID uuid.UUID            `json:"business_id"`
	UserID     uuid.UUID            `json:"user_id"`
	Vouchers   []BillVoucherRequest `json:"vouchers"`
}

// BillVoucherRequest represents a single bill in multi-bill payment
type BillVoucherRequest struct {
	PayeeID          uuid.UUID              `json:"payee_id"`
	AccountID        uuid.UUID              `json:"account_id"`
	InvoiceNumber    string                 `json:"invoice_number"`
	InvoiceDate      *time.Time             `json:"invoice_date,omitempty"`
	GrossAmount      int64                  `json:"gross_amount"`
	NetAmount        int64                  `json:"net_amount"`
	GSTAmount        int64                  `json:"gst_amount"`
	TDSAmount        int64                  `json:"tds_amount"`
	ExpenseCategory  string                 `json:"expense_category,omitempty"`
	Description      string                 `json:"description,omitempty"`
	FinancialDetails map[string]interface{} `json:"financial_details,omitempty"`
}

// CreateMultiVouchersResponse represents response after creating multiple vouchers
type CreateMultiVouchersResponse struct {
	Vouchers []CreateDraftVoucherResponse `json:"vouchers"`
	Message  string                       `json:"message"`
}

// GetDraftRemindersRequest represents request to get abandoned draft vouchers
type GetDraftRemindersRequest struct {
	BusinessID uuid.UUID     `json:"business_id"`
	OlderThan  time.Duration `json:"older_than"` // e.g., 24 hours
}

// DraftReminderItem represents a draft voucher needing reminder
type DraftReminderItem struct {
	VoucherID     uuid.UUID `json:"voucher_id"`
	VoucherNumber string    `json:"voucher_number"`
	PayeeName     string    `json:"payee_name"`
	NetAmount     int64     `json:"net_amount"`
	CreatedAt     time.Time `json:"created_at"`
	AgeHours      int       `json:"age_hours"`
}

// GetDraftRemindersResponse represents response with abandoned drafts
type GetDraftRemindersResponse struct {
	Drafts  []DraftReminderItem `json:"drafts"`
	Count   int                 `json:"count"`
	Message string              `json:"message"`
}

// RequestCancellationRequest represents request to cancel a voucher (Phase 7)
type RequestCancellationRequest struct {
	VoucherID uuid.UUID `json:"voucher_id"`
	UserID    uuid.UUID `json:"user_id"` // Business user requesting cancellation
	Reason    string    `json:"reason"`  // Cancellation reason
}

// RequestCancellationResponse represents response after requesting cancellation
type RequestCancellationResponse struct {
	VoucherID          uuid.UUID `json:"voucher_id"`
	VoucherNumber      string    `json:"voucher_number"`
	CancellationStatus string    `json:"cancellation_status"`
	Message            string    `json:"message"`
}

// ApproveCancellationRequest represents request to approve or reject cancellation (Phase 7)
type ApproveCancellationRequest struct {
	VoucherID uuid.UUID `json:"voucher_id"`
	UserID    uuid.UUID `json:"user_id"` // Operations/Finance user approving
	Action    string    `json:"action"`  // "approve" or "reject"
	Remarks   string    `json:"remarks,omitempty"`
}

// ApproveCancellationResponse represents response after approving/rejecting cancellation
type ApproveCancellationResponse struct {
	VoucherID          uuid.UUID `json:"voucher_id"`
	VoucherNumber      string    `json:"voucher_number"`
	CancellationStatus string    `json:"cancellation_status"`
	RefundStatus       string    `json:"refund_status,omitempty"`
	Message            string    `json:"message"`
}
