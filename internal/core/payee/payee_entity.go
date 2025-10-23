package payee

import (
	"time"

	"xcentro.in/internal/core/payment"
)

// ############################################################################
// # Payee Type Enum (Polymorphic Discriminator)
// ############################################################################

// PayeeType represents the different types of payees in the system
type PayeeType string

const (
	PayeeTypeVendor     PayeeType = "vendor"
	PayeeTypeGST        PayeeType = "gst_payment"
	PayeeTypeBiller     PayeeType = "biller"
	PayeeTypeEmployee   PayeeType = "employee"
)

// ############################################################################
// # Verification Status Enum
// ############################################################################

// VerificationStatus represents the verification status of GST, bank account, PAN, or cross-verification
type VerificationStatus string

const (
	VerificationStatusPending    VerificationStatus = "pending"
	VerificationStatusInProgress VerificationStatus = "in_progress"
	VerificationStatusSuccess    VerificationStatus = "success"
	VerificationStatusFailed     VerificationStatus = "failed"
)

// ############################################################################
// # Shared Types
// ############################################################################

// Address represents a physical address from GST data
type Address struct {
	BuildingName string `json:"building_name"`
	Street       string `json:"street"`
	City         string `json:"city"`
	State        string `json:"state"`
	Pincode      string `json:"pincode"`
}

// ############################################################################
// # Payee Master (Routing Table)
// ############################################################################

// PayeeMaster represents the master routing entry for all payees
// Thin table with minimal shared fields
type PayeeMaster struct {
	PayeeID   string    `json:"payee_id"`
	PayeeType PayeeType `json:"payee_type"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ############################################################################
// # Type 1: Vendor Payee (Business with GSTIN)
// ############################################################################

// PayeeVendor represents a vendor payee (business identified by GSTIN)
// Canonical business entity (one per GSTIN globally)
type PayeeVendor struct {
	PayeeID string `json:"payee_id"` // FK → payee_master.payee_id
	GSTIN   string `json:"gstin"`    // Unique 15-character identifier

	// Business details from GST verification
	LegalName        string     `json:"legal_name"`
	BusinessType     string     `json:"business_type"`
	RegistrationDate *time.Time `json:"registration_date,omitempty"`
	GSTStatus        string     `json:"gst_status"` // Active, Cancelled, Suspended

	// Primary address from GST
	PrimaryAddress Address `json:"primary_address"`

	// GST verification
	GSTVerificationStatus VerificationStatus `json:"gst_verification_status"`
	GSTVerifiedAt         *time.Time         `json:"gst_verified_at,omitempty"`
	GSTData               map[string]any     `json:"gst_data,omitempty"` // Full GST API response

	// Audit
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Metadata  map[string]any `json:"metadata,omitempty"` // Directors, state_code, etc.
}

// ############################################################################
// # Type 2: GST Payment Payee (Government)
// ############################################################################

// PayeeGST represents the government GST payment entity (singleton)
// Only one row will exist for this type
type PayeeGST struct {
	PayeeID              string         `json:"payee_id"` // FK → payee_master.payee_id
	GovernmentEntityName string         `json:"government_entity_name"`
	GSTPortalURL         string         `json:"gst_portal_url"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
	Metadata             map[string]any `json:"metadata,omitempty"`
}

// PayeeGSTVirtualAccount represents a virtual bank account for GST payment
// Each tax period generates a new virtual ID
type PayeeGSTVirtualAccount struct {
	VirtualAccountID     string    `json:"virtual_account_id"` // UUID
	PayeeID              string    `json:"payee_id"`           // FK → payee_master.payee_id
	VirtualAccountNumber string    `json:"virtual_account_number"`
	IFSCCode             string    `json:"ifsc_code"`
	BankName             string    `json:"bank_name"`
	TaxPeriod            string    `json:"tax_period"`     // "Jan-2024", "Q1-2024"
	FinancialYear        string    `json:"financial_year"` // "2023-24"
	TaxType              string    `json:"tax_type,omitempty"`
	ValidFrom            time.Time `json:"valid_from"`
	ValidUntil           time.Time `json:"valid_until"`
	IsActive             bool      `json:"is_active"`
	ChallanDocumentURL   string    `json:"challan_document_url,omitempty"` // S3 URL
	UploadedAt           *time.Time     `json:"uploaded_at,omitempty"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
	Metadata             map[string]any `json:"metadata,omitempty"`
}

// ############################################################################
// # Type 3: Biller Payee (BBPS System)
// ############################################################################

// PayeeBiller represents a biller in the BBPS system
type PayeeBiller struct {
	PayeeID            string         `json:"payee_id"` // FK → payee_master.payee_id
	BillerID           string         `json:"biller_id"`
	BillerName         string         `json:"biller_name"`
	BillerCategory     string         `json:"biller_category"`
	BillerDescription  string         `json:"biller_description,omitempty"`
	BBPSBillerCode     string         `json:"bbps_biller_code,omitempty"`
	NPCIRegistered     bool           `json:"npci_registered"`
	SupportsAdhoc      bool           `json:"supports_adhoc"`
	SupportsRecurring  bool           `json:"supports_recurring"`
	PaymentModes       []string       `json:"payment_modes"`
	AvgProcessingTimeHours int        `json:"avg_processing_time_hours,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	Metadata           map[string]any `json:"metadata,omitempty"`
}

// PayeeBillerAccount represents a customer-biller configuration
// Each business can have multiple accounts with same biller
type PayeeBillerAccount struct {
	BillerAccountID   string         `json:"biller_account_id"` // UUID
	PayeeID           string         `json:"payee_id"`          // FK → payee_master.payee_id
	BusinessID        string         `json:"business_id"`       // FK → business.bid
	MobileNumber      string         `json:"mobile_number"`
	CustomerParams    map[string]any `json:"customer_params"` // BBPS parameters (key-value pairs)
	AutoFetchEnabled  bool           `json:"auto_fetch_enabled"`
	FetchSchedule     string         `json:"fetch_schedule,omitempty"`
	PreferredFetchDay int            `json:"preferred_fetch_day,omitempty"`
	LastBillFetchedAt *time.Time     `json:"last_bill_fetched_at,omitempty"`
	LastBillAmount    int64          `json:"last_bill_amount,omitempty"`
	LastBillDueDate   *time.Time     `json:"last_bill_due_date,omitempty"`
	IsActive          bool           `json:"is_active"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}

// ############################################################################
// # Type 4: Employee Payee (Payroll)
// ############################################################################

// PayeeEmployee represents an employee payee (individual, not business)
type PayeeEmployee struct {
	PayeeID                 string             `json:"payee_id"` // FK → payee_master.payee_id
	EmployeeCode            string             `json:"employee_code,omitempty"`
	FullName                string             `json:"full_name"`
	PAN                     string             `json:"pan"` // Permanent Account Number
	Email                   string             `json:"email,omitempty"`
	Phone                   string             `json:"phone,omitempty"`
	Designation             string             `json:"designation,omitempty"`
	Department              string             `json:"department,omitempty"`
	DateOfJoining           *time.Time         `json:"date_of_joining,omitempty"`
	EmploymentType          string             `json:"employment_type,omitempty"`
	RemunerationDocURL      string             `json:"remuneration_doc_url,omitempty"` // S3 URL
	RemunerationDocUploadedAt *time.Time       `json:"remuneration_doc_uploaded_at,omitempty"`
	IsSensitive             bool               `json:"is_sensitive"` // Always true
	PANVerificationStatus   VerificationStatus `json:"pan_verification_status"`
	PANVerifiedAt           *time.Time         `json:"pan_verified_at,omitempty"`
	CreatedAt               time.Time          `json:"created_at"`
	UpdatedAt               time.Time          `json:"updated_at"`
	Metadata                map[string]any     `json:"metadata,omitempty"`
}

// ############################################################################
// # Shared: Bank Accounts (Vendor + Employee)
// ############################################################################

// PayeeBankAccount represents a bank account belonging to a payee
// Used by Vendor and Employee payee types (NOT by GST or Biller)
type PayeeBankAccount struct {
	AccountID string `json:"account_id"` // UUID
	PayeeID   string `json:"payee_id"`   // FK → payee_master.payee_id

	// Bank details (encrypted at rest)
	AccountNumber     string                  `json:"account_number"`      // Encrypted
	AccountNumberHash string                  `json:"account_number_hash"` // SHA-256
	IFSCCode          string                  `json:"ifsc_code"`
	BankName          string                  `json:"bank_name"`
	BranchName        string                  `json:"branch_name,omitempty"`
	AccountType       payment.BankAccountType `json:"account_type"`
	AccountHolderName string                  `json:"account_holder_name"`

	// Verification (penny drop)
	IsVerified             bool               `json:"is_verified"`
	VerifiedAt             *time.Time         `json:"verified_at,omitempty"`
	BankVerificationStatus VerificationStatus `json:"bank_verification_status"`
	VerificationData       map[string]any     `json:"verification_data,omitempty"`

	// Cross-verification with GST/PAN (money rotation check)
	CrossVerificationStatus VerificationStatus `json:"cross_verification_status"`

	// Account status
	IsActive     bool   `json:"is_active"`
	IsPrimary    bool   `json:"is_primary"`
	AccountLabel string `json:"account_label,omitempty"`

	// Audit
	AddedAt   time.Time      `json:"added_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// ############################################################################
// # Business-Payee Relation (Polymorphic Many-to-Many)
// ############################################################################

// BusinessPayeeRelation represents how a specific customer (business)
// relates to a payee and WHICH account they use for payments
// Polymorphic: account_id can reference bank_account, virtual_account, or biller_account
type BusinessPayeeRelation struct {
	RelationID string    `json:"relation_id"` // Surrogate key (UUID) for simplified frontend API
	BusinessID string    `json:"business_id"` // FK → business.bid
	PayeeID    string    `json:"payee_id"`    // FK → payee_master.payee_id
	PayeeType  PayeeType `json:"payee_type"`  // Type discriminator from payee_master (vendor, employee, biller, gst_payment)
	AccountID  string    `json:"account_id"`  // Polymorphic FK

	// Customer-specific metadata (how THIS customer sees the payee)
	DisplayName  string `json:"display_name"`
	ContactEmail string `json:"contact_email,omitempty"`
	ContactPhone string `json:"contact_phone,omitempty"`
	Notes        string `json:"notes,omitempty"`

	// Relationship metadata
	IsActive   bool     `json:"is_active"`
	IsFavorite bool     `json:"is_favorite"`
	Tags       []string `json:"tags,omitempty"`

	// Type-specific data (flexible JSONB)
	TypeSpecificData map[string]any `json:"type_specific_data,omitempty"`

	// Audit
	AddedAt    time.Time  `json:"added_at"`
	AddedBy    string     `json:"added_by"`              // FK → user_table.id
	LastUsedAt *time.Time `json:"last_used_at,omitempty"` // Last payment to this payee
	UpdatedAt  time.Time  `json:"updated_at"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}
