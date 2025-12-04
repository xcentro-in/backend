package payee

import (
	"time"

	"xcentro.in/internal/core/payment"
)

// ############################################################################
// # Polymorphic Onboarding Requests
// ############################################################################

// OnboardPayeeRequest represents a polymorphic request to onboard a new payee
// The PayeeType field determines which type-specific fields are required
type OnboardPayeeRequest struct {
	BusinessID string    `json:"-"` // Our customer (extracted from X-Business-ID header)
	AddedBy    string    `json:"-"` // User who is adding this payee (extracted from JWT context)
	PayeeType  PayeeType `json:"payee_type"` // Type discriminator: vendor, gst_payment, biller, employee

	// Customer-specific metadata (common to all types)
	DisplayName  string `json:"display_name"`            // How customer refers to this payee
	ContactEmail string `json:"contact_email,omitempty"` // Customer's contact point
	ContactPhone string `json:"contact_phone,omitempty"` // Customer's contact point
	Notes        string `json:"notes,omitempty"`         // Private notes

	// Type-specific fields (only populate based on PayeeType)
	Vendor   *OnboardVendorData   `json:"vendor,omitempty"`
	GST      *OnboardGSTData      `json:"gst,omitempty"`
	Biller   *OnboardBillerData   `json:"biller,omitempty"`
	Employee *OnboardEmployeeData `json:"employee,omitempty"`
}

// OnboardVendorData contains vendor-specific onboarding data (GSTIN + bank account)
type OnboardVendorData struct {
	GSTIN string `json:"gstin"` // 15-character GST identification number

	// Bank account details
	AccountNumber     string                  `json:"account_number"`
	IFSCCode          string                  `json:"ifsc_code"`
	AccountHolderName string                  `json:"account_holder_name"`
	BankName          string                  `json:"bank_name"`        // Required - fetched from IFSC check at frontend
	BranchName        string                  `json:"branch_name"`      // Required - fetched from IFSC check at frontend
	AccountType       payment.BankAccountType `json:"account_type,omitempty"` // Optional - defaults to "current"
	AccountLabel      string                  `json:"account_label,omitempty"`
}

// OnboardGSTData contains GST payment onboarding data (virtual account from challan)
type OnboardGSTData struct {
	VirtualAccountNumber string    `json:"virtual_account_number"` // From challan document
	IFSCCode             string    `json:"ifsc_code"`              // Bank IFSC for virtual account
	BankName             string    `json:"bank_name"`              // Bank name
	TaxPeriod            string    `json:"tax_period"`             // e.g., "2024-03"
	FinancialYear        string    `json:"financial_year"`         // e.g., "2024-25"
	TaxType              string    `json:"tax_type,omitempty"`     // CGST, SGST, IGST, etc.
	ValidFrom            time.Time `json:"valid_from"`             // Virtual account validity start
	ValidUntil           time.Time `json:"valid_until"`            // Virtual account validity end
	ChallanDocumentURL   string    `json:"challan_document_url"`   // S3 URL of challan PDF
}

// OnboardBillerData contains biller payment onboarding data (BBPS configuration)
type OnboardBillerData struct {
	BillerID       string         `json:"biller_id"`       // BBPS biller identifier
	MobileNumber   string         `json:"mobile_number"`   // Customer's registered mobile
	CustomerParams map[string]any `json:"customer_params"` // Biller-specific params (consumer number, etc.)
	AutoFetch      bool           `json:"auto_fetch"`      // Enable auto bill fetching
}

// OnboardEmployeeData contains employee payee onboarding data (bank account + employment details)
type OnboardEmployeeData struct {
	EmployeeCode string  `json:"employee_code,omitempty"`
	FullName     string  `json:"full_name"`
	Email        string  `json:"email,omitempty"`
	Phone        string  `json:"phone,omitempty"`
	Designation  string  `json:"designation,omitempty"`
	Department   string  `json:"department,omitempty"`
	DateOfJoining *time.Time `json:"date_of_joining,omitempty"`
	EmploymentType string `json:"employment_type,omitempty"` // full-time, contract, etc.

	// Bank account details
	AccountNumber     string                  `json:"account_number"`
	IFSCCode          string                  `json:"ifsc_code"`
	AccountHolderName string                  `json:"account_holder_name"`
	BankName          string                  `json:"bank_name"`        // Required - fetched from IFSC check at frontend
	BranchName        string                  `json:"branch_name"`      // Required - fetched from IFSC check at frontend
	AccountType       payment.BankAccountType `json:"account_type,omitempty"` // Optional - defaults to "current"
	AccountLabel      string                  `json:"account_label,omitempty"`

	// Note: Remuneration document is uploaded separately via dedicated upload endpoints
	// after employee creation, not during onboarding
}

// OnboardPayeeResponse represents the response after payee onboarding
type OnboardPayeeResponse struct {
	RelationID string `json:"relation_id"` // Surrogate key for simplified frontend API
	PayeeID    string `json:"payee_id"`
	AccountID  string `json:"account_id"`

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
	BusinessID string `json:"-"` // Our customer (extracted from X-Business-ID header)
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

// UpdatePayeeStatusRequest represents a request to toggle payee active status
type UpdatePayeeStatusRequest struct {
	BusinessID string `json:"-"` // Our customer (extracted from X-Business-ID header)
	RelationID string `json:"-"` // Extracted from URL path parameter
	IsActive   bool   `json:"is_active"`
}

// PayeeDetailsResponse represents detailed information about a payee (polymorphic)
// Combines data from payee_master, type-specific tables, and business_payee_relation
type PayeeDetailsResponse struct {
	// Common fields
	PayeeID   string    `json:"payee_id"`
	PayeeType PayeeType `json:"payee_type"` // Type discriminator

	// Account info (polymorphic - could be bank account, virtual account, or biller account)
	AccountID     string `json:"account_id"`
	MaskedAccount string `json:"masked_account,omitempty"` // For bank accounts only
	BankName      string `json:"bank_name,omitempty"`
	IFSCCode      string `json:"ifsc_code,omitempty"`
	AccountType   string `json:"account_type,omitempty"`
	AccountLabel  string `json:"account_label,omitempty"`
	IsVerified    bool   `json:"is_verified"`

	// Relationship info (customer-specific)
	DisplayName  string   `json:"display_name"`
	ContactEmail string   `json:"contact_email,omitempty"`
	ContactPhone string   `json:"contact_phone,omitempty"`
	Notes        string   `json:"notes,omitempty"`
	IsFavorite   bool     `json:"is_favorite"`
	Tags         []string `json:"tags,omitempty"`

	// Type-specific fields (only populated based on PayeeType)
	Vendor   *VendorDetailsData   `json:"vendor,omitempty"`
	GST      *GSTDetailsData      `json:"gst,omitempty"`
	Biller   *BillerDetailsData   `json:"biller,omitempty"`
	Employee *EmployeeDetailsData `json:"employee,omitempty"`

	// Audit
	AddedAt    time.Time  `json:"added_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// VendorDetailsData contains vendor-specific details
type VendorDetailsData struct {
	GSTIN                   string `json:"gstin"`
	LegalName               string `json:"legal_name"`
	BusinessType            string `json:"business_type,omitempty"`
	GSTStatus               string `json:"gst_status"`
	GSTVerificationStatus   string `json:"gst_verification_status"`
	BankVerificationStatus  string `json:"bank_verification_status"`
	CrossVerificationStatus string `json:"cross_verification_status"`
}

// GSTDetailsData contains GST payment-specific details
type GSTDetailsData struct {
	GovernmentEntityName string     `json:"government_entity_name"`
	VirtualAccountNumber string     `json:"virtual_account_number"`
	TaxPeriod            string     `json:"tax_period"`
	FinancialYear        string     `json:"financial_year"`
	TaxType              string     `json:"tax_type,omitempty"`
	ValidFrom            time.Time  `json:"valid_from"`
	ValidUntil           time.Time  `json:"valid_until"`
	IsActive             bool       `json:"is_active"`
	ChallanDocumentURL   string     `json:"challan_document_url,omitempty"`
	UploadedAt           *time.Time `json:"uploaded_at,omitempty"`
}

// BillerDetailsData contains biller payment-specific details
type BillerDetailsData struct {
	BillerID             string         `json:"biller_id"`
	BillerName           string         `json:"biller_name"`
	BillerCategory       string         `json:"biller_category"`
	MobileNumber         string         `json:"mobile_number"`
	CustomerParams       map[string]any `json:"customer_params"`
	AutoFetchEnabled     bool           `json:"auto_fetch_enabled"`
	LastBillAmount       int64          `json:"last_bill_amount,omitempty"`
	LastBillDueDate      *time.Time     `json:"last_bill_due_date,omitempty"`
	LastBillFetchedAt    *time.Time     `json:"last_bill_fetched_at,omitempty"`
}

// EmployeeDetailsData contains employee payee-specific details
type EmployeeDetailsData struct {
	EmployeeCode          string     `json:"employee_code,omitempty"`
	FullName              string     `json:"full_name"`
	Email                 string     `json:"email,omitempty"`
	Phone                 string     `json:"phone,omitempty"`
	Designation           string     `json:"designation,omitempty"`
	Department            string     `json:"department,omitempty"`
	EmploymentType        string     `json:"employment_type,omitempty"`
	BankVerificationStatus string    `json:"bank_verification_status"`
	CrossVerificationStatus string   `json:"cross_verification_status"`
}

// PayeeListResponse represents a paginated list of payees for a business
type PayeeListResponse struct {
	Payees     []*PayeeSummary `json:"payees"`
	TotalCount int             `json:"total_count"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
}

// PayeeSummary represents a summary of payee information for list views (polymorphic)
type PayeeSummary struct {
	PayeeID       string     `json:"payee_id"`
	PayeeType     PayeeType  `json:"payee_type"` // Type discriminator
	DisplayName   string     `json:"display_name"`

	// Type-specific identifiers (only one will be populated)
	LegalName    string `json:"legal_name,omitempty"`    // Vendor
	GSTIN        string `json:"gstin,omitempty"`         // Vendor
	BillerName   string `json:"biller_name,omitempty"`   // Biller
	EmployeeName string `json:"employee_name,omitempty"` // Employee

	MaskedAccount string     `json:"masked_account,omitempty"` // Last 5 digits (bank accounts only)
	BankName      string     `json:"bank_name,omitempty"`
	IsVerified    bool       `json:"is_verified"`
	IsFavorite    bool       `json:"is_favorite"`
	LastUsedAt    *time.Time `json:"last_used_at,omitempty"`
}

// PayeeSearchRequest represents search criteria from HTTP request
type PayeeSearchRequest struct {
	DisplayName string    `json:"display_name,omitempty"`
	PayeeType   PayeeType `json:"payee_type,omitempty"` // Filter by payee type
	IsActive    *bool     `json:"is_active,omitempty"`
	IsFavorite  *bool     `json:"is_favorite,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Page        int       `json:"page"`
	PageSize    int       `json:"page_size"`
}

// ############################################################################
// # Vendor Dropdown (Lightweight API for Voucher Creation)
// ############################################################################

// VendorDropdownItem represents minimal vendor info for dropdown selection
// Used in voucher creation UI where only relation_id is needed
type VendorDropdownItem struct {
	RelationID   string `json:"relation_id"`   // Surrogate key for voucher API
	DisplayName  string `json:"display_name"`  // How customer refers to vendor
	ContactEmail string `json:"contact_email"` // Customer's contact point
	ContactPhone string `json:"contact_phone"` // Customer's contact phone
}

// VendorDropdownListResponse represents the response for vendor dropdown
type VendorDropdownListResponse struct {
	Vendors []*VendorDropdownItem `json:"vendors"`
	Count   int                   `json:"count"`
}

// ############################################################################
// # Employee Lightweight List (For Tabulated Display)
// ############################################################################

// EmployeeLightweight represents minimal employee info for list display
type EmployeeLightweight struct {
	RelationID         string `json:"relation_id"`          // Surrogate key
	FullName           string `json:"full_name"`            // Employee full name
	Email              string `json:"email,omitempty"`      // Employee email
	Phone              string `json:"phone,omitempty"`      // Employee phone
	AccountNumberLast4 string `json:"account_number_last4"` // Last 4 digits of account number
	IFSCCode           string `json:"ifsc_code"`            // Bank IFSC code
	DocVerified        bool   `json:"doc_verified"`         // Whether remuneration document is uploaded and verified
}

// EmployeeLightweightListResponse represents the response for employee list
type EmployeeLightweightListResponse struct {
	Employees []*EmployeeLightweight `json:"employees"`
	Count     int                    `json:"count"`
}

// ############################################################################
// # Payee Full Details (Polymorphic Response by Relation ID)
// ############################################################################

// PayeeFullDetails represents complete payee information for a relation
// Polymorphic - fields populated based on payee_type
type PayeeFullDetails struct {
	// Common fields (from business_payee_relation)
	RelationID   string   `json:"relation_id"`
	PayeeType    string   `json:"payee_type"` // vendor, employee, biller
	DisplayName  string   `json:"display_name"`
	ContactEmail string   `json:"contact_email,omitempty"`
	ContactPhone string   `json:"contact_phone,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	IsFavorite   bool     `json:"is_favorite"`

	// Vendor-specific fields
	GSTIN      string `json:"gstin,omitempty"`       // Vendor only
	LegalName  string `json:"legal_name,omitempty"`  // Vendor only
	GSTStatus  string `json:"gst_status,omitempty"`  // Vendor only
	City       string `json:"city,omitempty"`        // Vendor only
	State      string `json:"state,omitempty"`       // Vendor only

	// Employee-specific fields
	FullName       string `json:"full_name,omitempty"`       // Employee only
	EmployeeCode   string `json:"employee_code,omitempty"`   // Employee only
	Designation    string `json:"designation,omitempty"`     // Employee only
	Department     string `json:"department,omitempty"`      // Employee only
	EmploymentType string `json:"employment_type,omitempty"` // Employee only

	// Biller-specific fields
	BillerName        string  `json:"biller_name,omitempty"`         // Biller only
	BillerCategory    string  `json:"biller_category,omitempty"`     // Biller only
	MobileNumber      string  `json:"mobile_number,omitempty"`       // Biller only
	LastBillAmount    float64 `json:"last_bill_amount,omitempty"`    // Biller only (in rupees)
	LastBillDueDate   string  `json:"last_bill_due_date,omitempty"`  // Biller only

	// Bank details (for vendor and employee only, not for biller)
	BankDetails *PayeeBankDetails `json:"bank_details,omitempty"`
}

// PayeeBankDetails represents bank account information
type PayeeBankDetails struct {
	AccountNumber     string `json:"account_number"`
	IFSCCode          string `json:"ifsc_code"`
	BankName          string `json:"bank_name"`
	BranchName        string `json:"branch_name,omitempty"`
	AccountType       string `json:"account_type"` // savings, current, cc, od
	AccountHolderName string `json:"account_holder_name"`
	IsVerified        bool   `json:"is_verified"`
}

// ############################################################################
// # Payee List Item (For GET /payees endpoint)
// ############################################################################

// PayeeListItem represents a single payee in the list response
// Excludes internal IDs (business_id, payee_id, account_id, added_by)
type PayeeListItem struct {
	RelationID   string     `json:"relation_id"`
	PayeeType    PayeeType  `json:"payee_type"`
	DisplayName  string     `json:"display_name"`
	ContactEmail string     `json:"contact_email,omitempty"`
	ContactPhone string     `json:"contact_phone,omitempty"`
	IsActive     bool       `json:"is_active"`
	IsFavorite   bool       `json:"is_favorite"`
	Tags         []string   `json:"tags,omitempty"`
	AddedAt      time.Time  `json:"added_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// ############################################################################
// # Lightweight Account Details (For GET /payees/:relation_id/acc_details)
// ############################################################################

// PayeeAccountDetails represents minimal account information for a payee
// Returns only last 4 digits of account number, bank name, and IFSC code
type PayeeAccountDetails struct {
	AccountNumberLast4 string `json:"account_number_last_4"`
	BankName           string `json:"bank_name"`
	IFSCCode           string `json:"ifsc_code"`
}
