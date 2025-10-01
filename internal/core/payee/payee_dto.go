package payee

import (
	"time"

	"xcentro.in/internal/core/payment"
)

// OnboardPayeeRequest represents a request to onboard a new payee
// Used by service layer to orchestrate the complete onboarding flow
type OnboardPayeeRequest struct {
	BusinessID string `json:"business_id"` // Our customer (extracted from JWT)

	// Payee identification
	GSTIN string `json:"gstin"` // 15-character GST identification number

	// Bank account details
	AccountNumber     string                  `json:"account_number"`
	IFSCCode          string                  `json:"ifsc_code"`
	AccountHolderName string                  `json:"account_holder_name"`
	BankName          string                  `json:"bank_name"`
	AccountType       payment.BankAccountType `json:"account_type"` // current, savings, etc.
	AccountLabel      string                  `json:"account_label,omitempty"` // "Retail Operations", etc.

	// Customer-specific metadata
	DisplayName  string `json:"display_name"`            // How customer refers to this payee
	ContactEmail string `json:"contact_email,omitempty"` // Customer's contact point
	ContactPhone string `json:"contact_phone,omitempty"` // Customer's contact point
	Notes        string `json:"notes,omitempty"`         // Private notes
}

// OnboardPayeeResponse represents the response after payee onboarding
type OnboardPayeeResponse struct {
	PayeeID   string `json:"payee_id"`
	AccountID string `json:"account_id"`

	// Verification results
	GSTVerificationStatus   VerificationStatus `json:"gst_verification_status"`
	BankVerificationStatus  VerificationStatus `json:"bank_verification_status"`
	CrossVerificationStatus VerificationStatus `json:"cross_verification_status"`

	// Status
	IsFullyVerified bool   `json:"is_fully_verified"` // All verifications passed
	Message         string `json:"message"`
}

// UpdateRelationRequest represents a request to update customer-payee relationship metadata
type UpdateRelationRequest struct {
	BusinessID string `json:"business_id"` // Our customer
	PayeeID    string `json:"payee_id"`
	AccountID  string `json:"account_id"`

	// Updatable fields
	DisplayName  string   `json:"display_name"`
	ContactEmail string   `json:"contact_email,omitempty"`
	ContactPhone string   `json:"contact_phone,omitempty"`
	Notes        string   `json:"notes,omitempty"`
	IsFavorite   bool     `json:"is_favorite"`
	Tags         []string `json:"tags,omitempty"`
}

// PayeeDetailsResponse represents detailed information about a payee
// Combines data from payee_business, payee_bank_accounts, and business_payee_relation
type PayeeDetailsResponse struct {
	// Payee business info
	PayeeID      string `json:"payee_id"`
	GSTIN        string `json:"gstin"`
	LegalName    string `json:"legal_name"`
	BusinessType string `json:"business_type"`
	GSTStatus    string `json:"gst_status"`

	// Account info
	AccountID     string `json:"account_id"`
	MaskedAccount string `json:"masked_account"` // Shows last 5 digits only
	BankName      string `json:"bank_name"`
	IFSCCode      string `json:"ifsc_code"`
	AccountType   string `json:"account_type"`
	AccountLabel  string `json:"account_label,omitempty"`
	IsVerified    bool   `json:"is_verified"`

	// Relationship info (customer-specific)
	DisplayName  string   `json:"display_name"`
	ContactEmail string   `json:"contact_email,omitempty"`
	ContactPhone string   `json:"contact_phone,omitempty"`
	Notes        string   `json:"notes,omitempty"`
	IsFavorite   bool     `json:"is_favorite"`
	Tags         []string `json:"tags,omitempty"`

	// Verification status
	GSTVerificationStatus   string `json:"gst_verification_status"`
	BankVerificationStatus  string `json:"bank_verification_status"`
	CrossVerificationStatus string `json:"cross_verification_status"`

	// Audit
	AddedAt    time.Time  `json:"added_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// PayeeListResponse represents a paginated list of payees for a business
type PayeeListResponse struct {
	Payees     []*PayeeSummary `json:"payees"`
	TotalCount int             `json:"total_count"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
}

// PayeeSummary represents a summary of payee information for list views
type PayeeSummary struct {
	PayeeID       string     `json:"payee_id"`
	DisplayName   string     `json:"display_name"`
	LegalName     string     `json:"legal_name"`
	MaskedAccount string     `json:"masked_account"` // Last 5 digits
	BankName      string     `json:"bank_name"`
	IsVerified    bool       `json:"is_verified"`
	IsFavorite    bool       `json:"is_favorite"`
	LastUsedAt    *time.Time `json:"last_used_at,omitempty"`
}

// PayeeSearchRequest represents search criteria from HTTP request
type PayeeSearchRequest struct {
	DisplayName string   `json:"display_name,omitempty"`
	IsActive    *bool    `json:"is_active,omitempty"`
	IsFavorite  *bool    `json:"is_favorite,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Page        int      `json:"page"`
	PageSize    int      `json:"page_size"`
}
