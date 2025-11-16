package voucher

import (
	"time"

	"github.com/google/uuid"
)

// VoucherStatus represents the lifecycle state of a voucher
type VoucherStatus string

const (
	StatusDraft     VoucherStatus = "draft"     // Created when user clicks "Next" (before payment)
	StatusFinalized VoucherStatus = "finalized" // After payment success
	StatusPosted    VoucherStatus = "posted"    // Accounting complete
	StatusSynced    VoucherStatus = "synced"    // ERP synchronized
)

// ERPSyncStatus represents the ERP synchronization state
type ERPSyncStatus string

const (
	ERPNotSynced ERPSyncStatus = "not_synced" // Initial state
	ERPPending   ERPSyncStatus = "pending"    // In sync queue
	ERPSynced    ERPSyncStatus = "synced"     // Successfully synced
	ERPFailed    ERPSyncStatus = "failed"     // Sync failed (retry needed)
)

// VoucherType represents different types of vouchers
type VoucherType string

const (
	VoucherTypePayment VoucherType = "payment" // Payment voucher (default)
	VoucherTypeReceipt VoucherType = "receipt" // Receipt voucher (future)
	VoucherTypeJournal VoucherType = "journal" // Journal voucher (future)
)


// Voucher represents a payment voucher domain entity
// It serves as the accounting document linking payments to ERP systems
type Voucher struct {
	// Primary identification
	VoucherID     uuid.UUID     `json:"voucher_id"`
	VoucherNumber string        `json:"voucher_number"` // Format: PV-2024-00001
	VoucherDate   time.Time     `json:"voucher_date"`
	VoucherType   VoucherType   `json:"voucher_type"`
	Status        VoucherStatus `json:"status"`

	// Accounting period
	FiscalYear       string `json:"fiscal_year"`       // FY2024-25
	AccountingPeriod string `json:"accounting_period"` // 2024-04

	// Business context
	BusinessID uuid.UUID `json:"business_id"`

	// Payee reference (composite FK to business_payee_relation)
	PayeeID   uuid.UUID `json:"payee_id"`
	AccountID uuid.UUID `json:"account_id"`

	// Invoice reference
	InvoiceNumber  string     `json:"invoice_number,omitempty"`
	InvoiceDate    *time.Time `json:"invoice_date,omitempty"`
	InvoiceDueDate *time.Time `json:"invoice_due_date,omitempty"`

	// Core amounts (all in paise for INR)
	GrossAmount int64  `json:"gross_amount"` // Total before deductions
	NetAmount   int64  `json:"net_amount"`   // Amount actually paid
	Currency    string `json:"currency"`     // INR, USD, etc.

	// Tax details
	GSTAmount  int64   `json:"gst_amount"`
	GSTRate    float64 `json:"gst_rate,omitempty"`    // e.g., 18.00
	CGSTAmount int64   `json:"cgst_amount,omitempty"` // Central GST
	SGSTAmount int64   `json:"sgst_amount,omitempty"` // State GST
	IGSTAmount int64   `json:"igst_amount,omitempty"` // Integrated GST

	// TDS details
	TDSAmount  int64   `json:"tds_amount"`
	TDSRate    float64 `json:"tds_rate,omitempty"`    // e.g., 1.00
	TDSSection string  `json:"tds_section,omitempty"` // 194C, 194J, etc.

	// Other charges
	FeeAmount      int64 `json:"fee_amount"`      // Payment gateway fees
	DiscountAmount int64 `json:"discount_amount"` // Discounts applied

	// Additional financial details (JSONB)
	FinancialDetails map[string]interface{} `json:"financial_details,omitempty"`

	// Attachments (JSONB)
	Attachments map[string]interface{} `json:"attachments,omitempty"`

	// Classification
	ExpenseCategory string `json:"expense_category,omitempty"`
	Description     string `json:"description,omitempty"`

	// Vendor/payment details
	PaymentTerms string `json:"payment_terms,omitempty"` // e.g., "net30", "eom", "cod"

	// Payment link (NULL when voucher is in draft)
	PaymentID *uuid.UUID `json:"payment_id,omitempty"`

	// ERP integration
	ERPSyncStatus   ERPSyncStatus          `json:"erp_sync_status"`
	ERPReferenceID  string                 `json:"erp_reference_id,omitempty"`
	ERPPostingDate  *time.Time             `json:"erp_posting_date,omitempty"`
	ERPLastSyncAt   *time.Time             `json:"erp_last_sync_at,omitempty"`
	ERPSyncAttempts int                    `json:"erp_sync_attempts"`
	ERPMetadata     map[string]interface{} `json:"erp_metadata,omitempty"`

	// General ledger codes
	GLExpenseAccount string `json:"gl_expense_account,omitempty"` // e.g., 5000
	GLBankAccount    string `json:"gl_bank_account,omitempty"`    // e.g., 1020
	CostCenter       string `json:"cost_center,omitempty"`        // Departmental accounting

	// Timestamps
	CreatedAt   time.Time              `json:"created_at"`
	CreatedByID *uuid.UUID             `json:"created_by_id,omitempty"`
	UpdatedAt   *time.Time             `json:"updated_at,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// ERPSyncQueue represents a queue entry for ERP synchronization
type ERPSyncQueue struct {
	QueueID         uuid.UUID              `json:"queue_id"`
	VoucherID       uuid.UUID              `json:"voucher_id"`
	SyncDirection   string                 `json:"sync_direction"` // push, pull
	QueueStatus     string                 `json:"queue_status"`   // pending, processing, completed, failed
	RetryCount      int                    `json:"retry_count"`
	MaxRetries      int                    `json:"max_retries"`
	NextRetryAt     *time.Time             `json:"next_retry_at,omitempty"`
	RequestPayload  map[string]interface{} `json:"request_payload,omitempty"`
	ResponsePayload map[string]interface{} `json:"response_payload,omitempty"`
	ErrorCode       string                 `json:"error_code,omitempty"`
	ErrorMessage    string                 `json:"error_message,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	StartedAt       *time.Time             `json:"started_at,omitempty"`
	CompletedAt     *time.Time             `json:"completed_at,omitempty"`
}

// LineItem represents a single line item in the financial details
type LineItem struct {
	Description string  `json:"description"`
	HSNCode     string  `json:"hsn_code,omitempty"` // Harmonized System of Nomenclature
	Quantity    float64 `json:"quantity,omitempty"`
	UnitPrice   int64   `json:"unit_price"` // in paise
	Amount      int64   `json:"amount"`     // in paise
	TaxRate     float64 `json:"tax_rate,omitempty"`
	TaxAmount   int64   `json:"tax_amount,omitempty"` // in paise
}

// Attachment represents a document attachment
type Attachment struct {
	URL        string    `json:"url"`
	FileName   string    `json:"file_name"`
	FileType   string    `json:"file_type"` // pdf, jpg, png
	FileSize   int64     `json:"file_size"` // bytes
	UploadedAt time.Time `json:"uploaded_at"`
	UploadedBy uuid.UUID `json:"uploaded_by"`
}

// IsValid validates the voucher entity
func (v *Voucher) IsValid() error {
	if v.BusinessID == uuid.Nil {
		return ErrInvalidBusinessID
	}
	if v.PayeeID == uuid.Nil {
		return ErrInvalidPayeeID
	}
	if v.AccountID == uuid.Nil {
		return ErrInvalidAccountID
	}
	if v.GrossAmount <= 0 {
		return ErrInvalidGrossAmount
	}
	if v.NetAmount <= 0 {
		return ErrInvalidNetAmount
	}
	if v.Currency == "" {
		return ErrInvalidCurrency
	}
	return nil
}

// IsDraft returns true if voucher is in draft status
func (v *Voucher) IsDraft() bool {
	return v.Status == StatusDraft
}

// IsFinalized returns true if voucher is finalized or beyond
func (v *Voucher) IsFinalized() bool {
	return v.Status == StatusFinalized || v.Status == StatusPosted || v.Status == StatusSynced
}

// CanBeFinalized returns true if voucher can be finalized
func (v *Voucher) CanBeFinalized() bool {
	return v.Status == StatusDraft && v.PaymentID == nil
}

// CanBeSynced returns true if voucher can be synced to ERP
func (v *Voucher) CanBeSynced() bool {
	return v.IsFinalized() && (v.ERPSyncStatus == ERPNotSynced || v.ERPSyncStatus == ERPFailed)
}

// CalculateTotalTax returns the total tax amount (GST)
func (v *Voucher) CalculateTotalTax() int64 {
	return v.CGSTAmount + v.SGSTAmount + v.IGSTAmount
}

// CalculateTotalDeductions returns total deductions (TDS + discount)
func (v *Voucher) CalculateTotalDeductions() int64 {
	return v.TDSAmount + v.DiscountAmount
}

// ValidateAmountCalculation checks if net amount calculation is correct
// Formula: NetAmount = GrossAmount - TDS - Discount
// Note: GST is typically included in GrossAmount for Indian taxation
func (v *Voucher) ValidateAmountCalculation() bool {
	expectedNet := v.GrossAmount - v.TDSAmount - v.DiscountAmount
	return v.NetAmount == expectedNet
}

// VoucherWithPaymentInfo represents a voucher with enriched payment source details
// Used for displaying vouchers with payment method information in frontend tables
type VoucherWithPaymentInfo struct {
	VoucherID     uuid.UUID     `json:"voucher_id"`
	InvoiceNumber string        `json:"invoice_number"`
	NetAmount     int64         `json:"net_amount"`  // Amount actually paid (in paise)
	CreatedAt     time.Time     `json:"created_at"`  // Voucher creation date
	Status        VoucherStatus `json:"status"`      // Voucher status (draft, finalized, etc.)
	Description   string        `json:"description"` // Voucher description

	// Payment information (NULL for draft vouchers)
	PaymentID           *uuid.UUID `json:"payment_id,omitempty"`
	PaymentStatus       string     `json:"payment_status,omitempty"`        // Payment status (success, pending, failed)
	PaymentSourceType   string     `json:"payment_source_type,omitempty"`   // card, bank_account, upi, wallet
	PaymentSourceDigits string     `json:"payment_source_digits,omitempty"` // Last 4/5 digits
}
