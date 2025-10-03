package business_rules

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ERPProvider represents different ERP system types
type ERPProvider string

const (
	ERPProviderZohoBooks ERPProvider = "zoho_books"
	ERPProviderTally     ERPProvider = "tally"
	ERPProviderSAP       ERPProvider = "sap"
	ERPProviderQuickBooks ERPProvider = "quickbooks"
	ERPProviderCustom    ERPProvider = "custom"
)

// ERPVoucher represents a voucher in ERP system format
type ERPVoucher struct {
	// Business context
	BusinessID uuid.UUID `json:"business_id"`
	VoucherID  uuid.UUID `json:"voucher_id"`

	// Voucher details
	VoucherNumber string    `json:"voucher_number"`
	VoucherDate   time.Time `json:"voucher_date"`
	VoucherType   string    `json:"voucher_type"`

	// Payee information
	PayeeName  string `json:"payee_name"`
	PayeeGSTIN string `json:"payee_gstin,omitempty"`

	// Invoice reference
	InvoiceNumber string     `json:"invoice_number,omitempty"`
	InvoiceDate   *time.Time `json:"invoice_date,omitempty"`

	// Amounts (in paise)
	GrossAmount int64 `json:"gross_amount"`
	NetAmount   int64 `json:"net_amount"`

	// Tax details
	GSTAmount  int64   `json:"gst_amount"`
	GSTRate    float64 `json:"gst_rate,omitempty"`
	CGSTAmount int64   `json:"cgst_amount"`
	SGSTAmount int64   `json:"sgst_amount"`
	IGSTAmount int64   `json:"igst_amount"`

	// TDS details
	TDSAmount  int64   `json:"tds_amount"`
	TDSRate    float64 `json:"tds_rate,omitempty"`
	TDSSection string  `json:"tds_section,omitempty"`

	// Other charges
	FeeAmount      int64 `json:"fee_amount"`
	DiscountAmount int64 `json:"discount_amount"`

	// Classification
	ExpenseCategory string `json:"expense_category,omitempty"`
	Description     string `json:"description,omitempty"`

	// General ledger
	GLExpenseAccount string `json:"gl_expense_account,omitempty"`
	GLBankAccount    string `json:"gl_bank_account,omitempty"`
	CostCenter       string `json:"cost_center,omitempty"`

	// Line items
	LineItems []ERPLineItem `json:"line_items,omitempty"`

	// Additional metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ERPLineItem represents a line item in ERP format
type ERPLineItem struct {
	Description string  `json:"description"`
	HSNCode     string  `json:"hsn_code,omitempty"`
	Quantity    float64 `json:"quantity,omitempty"`
	UnitPrice   int64   `json:"unit_price"` // in paise
	Amount      int64   `json:"amount"`     // in paise
	TaxRate     float64 `json:"tax_rate,omitempty"`
	TaxAmount   int64   `json:"tax_amount,omitempty"` // in paise
}

// ERPSyncRequest represents a request to sync a voucher to ERP
type ERPSyncRequest struct {
	Provider ERPProvider `json:"provider"`
	Voucher  ERPVoucher  `json:"voucher"`

	// Configuration (from business.erp_metadata)
	Config map[string]interface{} `json:"config,omitempty"`
}

// ERPSyncResponse represents the response from ERP system
type ERPSyncResponse struct {
	Success        bool      `json:"success"`
	ERPReferenceID string    `json:"erp_reference_id"` // ERP system's voucher ID
	PostingDate    time.Time `json:"posting_date"`
	Message        string    `json:"message"`
	ErrorCode      string    `json:"error_code,omitempty"`
	ErrorMessage   string    `json:"error_message,omitempty"`

	// Raw response from ERP API
	RawResponse map[string]interface{} `json:"raw_response,omitempty"`
}

// ERPIntegrationService defines the interface for ERP integration operations
type ERPIntegrationService interface {
	// PushVoucher pushes a voucher to ERP system
	PushVoucher(ctx context.Context, request *ERPSyncRequest) (*ERPSyncResponse, error)

	// PullVoucher retrieves a voucher from ERP system
	PullVoucher(ctx context.Context, provider ERPProvider, erpReferenceID string, config map[string]interface{}) (*ERPVoucher, error)

	// ValidateConnection validates ERP connection and credentials
	ValidateConnection(ctx context.Context, provider ERPProvider, config map[string]interface{}) error

	// GetSupportedProviders returns list of supported ERP providers
	GetSupportedProviders() []ERPProvider

	// TestConnection tests ERP connection with a ping request
	TestConnection(ctx context.Context, provider ERPProvider, config map[string]interface{}) (bool, error)
}

// ERPIntegrationError represents an ERP integration error
type ERPIntegrationError struct {
	Provider     ERPProvider
	Operation    string // push, pull, validate
	ErrorCode    string
	ErrorMessage string
	Retryable    bool // Whether the error is retryable
}

func (e *ERPIntegrationError) Error() string {
	return e.ErrorMessage
}

// Common ERP error codes
const (
	ERPErrorAuthFailed       = "auth_failed"
	ERPErrorInvalidRequest   = "invalid_request"
	ERPErrorDuplicateVoucher = "duplicate_voucher"
	ERPErrorNetworkTimeout   = "network_timeout"
	ERPErrorRateLimitExceeded = "rate_limit_exceeded"
	ERPErrorServerError      = "server_error"
	ERPErrorNotConfigured    = "not_configured"
)
