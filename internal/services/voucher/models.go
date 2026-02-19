package voucher

import (
	"time"

	"github.com/google/uuid"
	"xcentro.in/internal/core/voucher"
	"xcentro.in/internal/shared/types"
)

// CreateDraftVoucherRequest represents the request to create a draft voucher
type CreateDraftVoucherRequest struct {
	// Business context (populated from X-Business-ID header and JWT)
	BusinessID uuid.UUID `json:"-"` // Populated from X-Business-ID header
	UserID     uuid.UUID `json:"-"` // Populated from JWT token (created_by)

	// Payee reference (simplified with surrogate key)
	RelationID uuid.UUID `json:"relation_id" validate:"required"` // Single identifier for business-payee-account relationship

	// Invoice details
	InvoiceNumber  string      `json:"invoice_number" validate:"required,min=1,max=100,safe_text"`
	InvoiceDate    *types.Date `json:"invoice_date,omitempty"`
	InvoiceDueDate *types.Date `json:"invoice_due_date,omitempty"`

	// Amounts (all in paise)
	GrossAmount    int64   `json:"gross_amount" validate:"required,gt=0"`
	NetAmount      int64   `json:"net_amount" validate:"required,gt=0"`
	Currency       string  `json:"currency" validate:"required,oneof=INR USD"` // Default: INR
	GSTAmount      int64   `json:"gst_amount" validate:"gte=0"`
	GSTRate        float64 `json:"gst_rate,omitempty"`
	CGSTAmount     int64   `json:"cgst_amount,omitempty" validate:"gte=0"`
	SGSTAmount     int64   `json:"sgst_amount,omitempty" validate:"gte=0"`
	IGSTAmount     int64   `json:"igst_amount,omitempty" validate:"gte=0"`
	TDSAmount      int64   `json:"tds_amount" validate:"gte=0"`
	TDSRate        float64 `json:"tds_rate,omitempty"`
	TDSSection     string  `json:"tds_section,omitempty" validate:"omitempty,max=50,safe_text"`
	FeeAmount      int64   `json:"fee_amount,omitempty" validate:"gte=0"`
	DiscountAmount int64   `json:"discount_amount,omitempty" validate:"gte=0"`

	// Additional details
	FinancialDetails map[string]interface{} `json:"financial_details,omitempty"`
	Attachments      map[string]interface{} `json:"attachments,omitempty"`
	ExpenseCategory  string                 `json:"expense_category,omitempty" validate:"omitempty,max=100,safe_text"`
	Description      string                 `json:"description,omitempty" validate:"omitempty,max=500,safe_text"`

	// General ledger codes
	GLExpenseAccount string `json:"gl_expense_account,omitempty" validate:"omitempty,max=50,safe_text"`
	GLBankAccount    string `json:"gl_bank_account,omitempty" validate:"omitempty,max=50,safe_text"`
	CostCenter       string `json:"cost_center,omitempty" validate:"omitempty,max=100,safe_text"`

	// Payment terms and vendor reference
	PaymentTerms          string `json:"payment_terms,omitempty" validate:"omitempty,max=50,safe_text"`
	VendorReferenceNumber string `json:"vendor_reference_number,omitempty" validate:"omitempty,max=100,safe_text"`

	// Metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// CreateDraftVoucherResponse represents the response after creating a draft voucher
type CreateDraftVoucherResponse struct {
	VoucherID     uuid.UUID             `json:"voucher_id"`
	InvoiceNumber string                `json:"invoice_number"`
	Status        voucher.VoucherStatus `json:"status"`
	Message       string                `json:"message"`
}

// UpdateVoucherRequest represents the request to update a draft voucher
type UpdateVoucherRequest struct {
	VoucherID    uuid.UUID `json:"-"` // From URL path parameter, not request body
	EditorUserID string    `json:"-"` // Set from JWT context, not from request body

	// Payee reference (can be updated to change payee/account)
	RelationID *uuid.UUID `json:"relation_id,omitempty"`

	// Invoice details
	InvoiceNumber  *string     `json:"invoice_number,omitempty" validate:"omitempty,min=1,max=100,safe_text"`
	InvoiceDate    *types.Date `json:"invoice_date,omitempty"`
	InvoiceDueDate *types.Date `json:"invoice_due_date,omitempty"`

	// Amounts (all in paise)
	GrossAmount    *int64   `json:"gross_amount,omitempty" validate:"omitempty,gt=0"`
	NetAmount      *int64   `json:"net_amount,omitempty" validate:"omitempty,gt=0"`
	Currency       *string  `json:"currency,omitempty" validate:"omitempty,oneof=INR USD"`
	GSTAmount      *int64   `json:"gst_amount,omitempty" validate:"omitempty,gte=0"`
	GSTRate        *float64 `json:"gst_rate,omitempty"`
	CGSTAmount     *int64   `json:"cgst_amount,omitempty" validate:"omitempty,gte=0"`
	SGSTAmount     *int64   `json:"sgst_amount,omitempty" validate:"omitempty,gte=0"`
	IGSTAmount     *int64   `json:"igst_amount,omitempty" validate:"omitempty,gte=0"`
	TDSAmount      *int64   `json:"tds_amount,omitempty" validate:"omitempty,gte=0"`
	TDSRate        *float64 `json:"tds_rate,omitempty"`
	TDSSection     *string  `json:"tds_section,omitempty" validate:"omitempty,min=1,max=50,safe_text"`
	FeeAmount      *int64   `json:"fee_amount,omitempty" validate:"omitempty,gte=0"`
	DiscountAmount *int64   `json:"discount_amount,omitempty" validate:"omitempty,gte=0"`

	// Additional details
	FinancialDetails *map[string]interface{} `json:"financial_details,omitempty"`
	Attachments      *map[string]interface{} `json:"attachments,omitempty"`
	ExpenseCategory  *string                 `json:"expense_category,omitempty" validate:"omitempty,min=1,max=100,safe_text"`
	Description      *string                 `json:"description,omitempty" validate:"omitempty,min=1,max=500,safe_text"`

	// General ledger codes
	GLExpenseAccount *string `json:"gl_expense_account,omitempty" validate:"omitempty,min=1,max=50,safe_text"`
	GLBankAccount    *string `json:"gl_bank_account,omitempty" validate:"omitempty,min=1,max=50,safe_text"`
	CostCenter       *string `json:"cost_center,omitempty" validate:"omitempty,min=1,max=100,safe_text"`

	// Metadata
	Metadata *map[string]interface{} `json:"metadata,omitempty"`
}

// UpdateVoucherResponse represents the response after updating a voucher
type UpdateVoucherResponse struct {
	VoucherID     uuid.UUID             `json:"voucher_id"`
	VoucherNumber string                `json:"voucher_number"`
	Status        voucher.VoucherStatus `json:"status"`
	Message       string                `json:"message"`
	UpdatedFields []string              `json:"updated_fields"` // List of fields that were updated
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

// PayeeDetails represents payee information in voucher details
type PayeeDetails struct {
	RelationID        uuid.UUID `json:"relation_id"`         // business_payee_relation ID
	PayeeID           uuid.UUID `json:"payee_id"`            // Payee entity ID
	Name              string    `json:"name"`                // Legal/Display name
	GSTIN             string    `json:"gstin,omitempty"`     // GST number
	Type              string    `json:"type"`                // vendor, employee, biller
	BankAccountMasked string    `json:"bank_account_masked"` // ****3456
	AccountID         uuid.UUID `json:"account_id"`          // Bank account ID
	IFSCCode          string    `json:"ifsc_code"`           // IFSC code
	BankName          string    `json:"bank_name"`           // Bank name
	AccountType       string    `json:"account_type"`        // savings, current
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
	Status           voucher.VoucherStatus `json:"voucher_status"` // Voucher lifecycle: draft, finalized, posted, synced

	// Payee information (nested object)
	Payee *PayeeDetails `json:"payee"`

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
	FinancialDetails map[string]interface{} `json:"financial_details"`
	Attachments      map[string]interface{} `json:"attachments"`
	ExpenseCategory  string                 `json:"expense_category,omitempty"`
	Description      string                 `json:"description,omitempty"`

	// Status fields - Three independent status dimensions
	ApprovalStatus *string `json:"approval_status"` // Latest approval action: pending, approved, rejected, request_changes (null if no approvals)
	PaymentStatus  *string `json:"payment_status"`  // Payment status from payments table: pending, success, failed, cancelled (null if no payment)

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
	Status          voucher.VoucherStatus `json:"voucher_status"` // Voucher lifecycle: draft, finalized, posted, synced
	ApprovalStatus  *string               `json:"approval_status"` // Latest approval action: pending, approved, rejected, request_changes (null if no approvals)
	PaymentStatus   *string               `json:"payment_status"`  // Payment status: pending, success, failed, cancelled (null if no payment)
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
	BusinessID uuid.UUID            `json:"-"` // Populated from X-Business-ID header
	UserID     uuid.UUID            `json:"-"` // Populated from JWT token
	Vouchers   []BillVoucherRequest `json:"vouchers" validate:"required,min=1,dive"`
}

// BillVoucherRequest represents a single bill in multi-bill payment
type BillVoucherRequest struct {
	RelationID       uuid.UUID              `json:"relation_id" validate:"required"` // Single identifier for business-payee-account relationship
	InvoiceNumber    string                 `json:"invoice_number" validate:"required,min=1,max=100,safe_text"`
	InvoiceDate      *types.Date            `json:"invoice_date,omitempty"`
	GrossAmount      int64                  `json:"gross_amount" validate:"required,gt=0"`
	NetAmount        int64                  `json:"net_amount" validate:"required,gt=0"`
	GSTAmount        int64                  `json:"gst_amount" validate:"gte=0"`
	TDSAmount        int64                  `json:"tds_amount" validate:"gte=0"`
	ExpenseCategory  string                 `json:"expense_category,omitempty" validate:"omitempty,max=100,safe_text"`
	Description      string                 `json:"description,omitempty" validate:"omitempty,max=500,safe_text"`
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

// GetVouchersByPayeeRequest represents request to get vouchers for a specific payee
type GetVouchersByPayeeRequest struct {
	BusinessID uuid.UUID `json:"business_id"` // From X-Business-ID header
	RelationID uuid.UUID `json:"relation_id"` // From path parameter
	Page       int       `json:"page"`        // Page number (1-based)
	PageSize   int       `json:"page_size"`   // Items per page
}

// VoucherByPayeeItem represents a single voucher item in payee voucher list
type VoucherByPayeeItem struct {
	VoucherID     uuid.UUID `json:"voucher_id"`
	InvoiceNumber string    `json:"invoice_number"`
	NetAmount     int64     `json:"net_amount"`     // Amount actually paid (in paise)
	CreatedAt     time.Time `json:"created_at"`     // Voucher creation date
	Status        string    `json:"status"`         // "Draft" or payment status
	PaymentMethod string    `json:"payment_method"` // e.g., "Card ****1234", "Bank ****67890"
	Description   string    `json:"description"`    // Voucher description
}

// GetVouchersByPayeeResponse represents paginated response of vouchers for a payee
type GetVouchersByPayeeResponse struct {
	Vouchers   []VoucherByPayeeItem      `json:"vouchers"`
	Pagination *voucher.PaginationResult `json:"pagination"`
	Summary    *VouchersByPayeeSummary   `json:"summary,omitempty"`
}

// VouchersByPayeeSummary provides aggregate statistics for payee vouchers
type VouchersByPayeeSummary struct {
	TotalVouchers  int64 `json:"total_vouchers"`
	TotalAmount    int64 `json:"total_amount"` // Sum of net amounts (in paise)
	DraftCount     int64 `json:"draft_count"`
	CompletedCount int64 `json:"completed_count"`
}

// VoucherPaymentDetailsResponse represents payment details for a voucher (customer-facing)
type VoucherPaymentDetailsResponse struct {
	// Payment identification
	PaymentID     string    `json:"payment_id"`
	VoucherID     string    `json:"voucher_id"`
	VoucherNumber string    `json:"voucher_number"`
	PaymentStatus string    `json:"payment_status"` // success, pending, failed
	PaymentDate   time.Time `json:"payment_date"`   // When payment was made

	// Payment source details
	PaymentMethod  string `json:"payment_method"`   // card, net_banking, upi
	CardType       string `json:"card_type"`        // visa, mastercard, rupay, amex, diners (for cards)
	CardCategory   string `json:"card_category"`    // credit, debit (for cards)
	LastFourDigits string `json:"last_four_digits"` // Last 4 digits of card/account
	BankName       string `json:"bank_name"`        // Bank name for net banking or card issuer

	// Payment amount breakdown (all in paise)
	VoucherAmount      int64  `json:"voucher_amount"`       // Original voucher amount (what vendor gets)
	ConvenienceFee     int64  `json:"convenience_fee"`      // Service charge (before GST)
	ConvenienceFeeRate string `json:"convenience_fee_rate"` // e.g., "2.00%" or "₹28.00 (fixed)"
	GSTOnFee           int64  `json:"gst_on_fee"`           // 18% GST on convenience fee
	GSTRate            string `json:"gst_rate"`             // e.g., "18.00%"
	TotalFeeAmount     int64  `json:"total_fee_amount"`     // convenience_fee + gst_on_fee
	TotalAmountPaid    int64  `json:"total_amount_paid"`    // voucher_amount + total_fee_amount (what customer paid)

	// Payout details (vendor/payee receipt)
	PayoutUTR         string     `json:"payout_utr"`          // Bank UTR from payout provider (if payout completed to vendor)
	PayoutCompletedAt *time.Time `json:"payout_completed_at"` // When payout completed to vendor (null if not yet completed)

	Description string `json:"description"`
}
