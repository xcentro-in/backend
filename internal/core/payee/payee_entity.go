package payee

import (
	"time"

	"xcentro.in/internal/core/payment"
)

// VerificationStatus represents the verification status of GST, bank account, or cross-verification
type VerificationStatus string

const (
	VerificationStatusPending    VerificationStatus = "pending"
	VerificationStatusInProgress VerificationStatus = "in_progress"
	VerificationStatusSuccess    VerificationStatus = "success"
	VerificationStatusFailed     VerificationStatus = "failed"
)

// Address represents a physical address from GST data
type Address struct {
	BuildingName string `json:"building_name"`
	Street       string `json:"street"`
	City         string `json:"city"`
	State        string `json:"state"`
	Pincode      string `json:"pincode"`
}

// PayeeBusiness represents the canonical business entity as a payee
// Identified uniquely by GSTIN - shared across all customers
// This is the global payee entity (one entry per GSTIN)
type PayeeBusiness struct {
	PayeeID string `json:"payee_id"` // UUID - internal identifier
	GSTIN   string `json:"gstin"`    // Unique business identifier (15 chars)

	// Business details
	LegalName string `json:"legal_name"` // Official registered name from GST

	// GST verification
	GSTData                map[string]any     `json:"gst_data,omitempty"`            // Full GST API response
	GSTVerifiedAt          *time.Time         `json:"gst_verified_at,omitempty"`
	GSTVerificationStatus  VerificationStatus `json:"gst_verification_status"`

	// Business metadata from GST
	BusinessType     string     `json:"business_type"`              // proprietorship, partnership, company, etc.
	RegistrationDate *time.Time `json:"registration_date,omitempty"` // From GST data
	GSTStatus        string     `json:"gst_status"`                 // Active, Cancelled, Suspended
	PrimaryAddress   Address    `json:"primary_address"`

	// Audit
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// BusinessPayeeRelation represents how a specific customer (business)
// relates to a payee and WHICH bank account they use for payments
// This is the many-to-many relationship table (multiple entries per payee)
type BusinessPayeeRelation struct {
	BusinessID string `json:"business_id"` // Our customer (FK → business.bid)
	PayeeID    string `json:"payee_id"`    // The payee (FK → payee_business.payee_id)
	AccountID  string `json:"account_id"`  // Which account (FK → payee_bank_accounts.account_id)

	// Customer-specific metadata (how THIS customer sees the payee)
	DisplayName  string `json:"display_name"`            // "ABC Corp" vs "ABC Industries"
	ContactEmail string `json:"contact_email,omitempty"` // finance@abc.com vs accounts@abc.com
	ContactPhone string `json:"contact_phone,omitempty"` // Different contact points
	Notes        string `json:"notes,omitempty"`         // Customer's private notes about payee

	// Relationship metadata
	IsActive   bool     `json:"is_active"`
	IsFavorite bool     `json:"is_favorite"`          // Quick access flag
	Tags       []string `json:"tags,omitempty"`       // Custom categorization

	// Audit
	AddedAt    time.Time  `json:"added_at"`              // When customer added this payee
	AddedBy    string     `json:"added_by"`              // Which user added (FK → user_table.id)
	LastUsedAt *time.Time `json:"last_used_at,omitempty"` // Last payment to this payee
	UpdatedAt  time.Time  `json:"updated_at"`
	Metadata   map[string]any `json:"metadata,omitempty"` // Custom fields per customer
}

// PayeeBankAccount represents a bank account belonging to a payee
// A payee can have MULTIPLE bank accounts (current, savings, different banks)
// One-to-many relationship: one payee can have many bank accounts
type PayeeBankAccount struct {
	AccountID string `json:"account_id"` // UUID - internal identifier
	PayeeID   string `json:"payee_id"`   // FK → payee_business.payee_id

	// Bank details (encrypted at rest)
	AccountNumber     string                  `json:"account_number"`      // Encrypted
	AccountNumberHash string                  `json:"account_number_hash"` // SHA-256 for duplicate detection
	IFSCCode          string                  `json:"ifsc_code"`
	BankName          string                  `json:"bank_name"`
	BranchName        string                  `json:"branch_name,omitempty"`
	AccountType       payment.BankAccountType `json:"account_type"`
	AccountHolderName string                  `json:"account_holder_name"` // From bank records

	// Verification (penny drop)
	IsVerified             bool               `json:"is_verified"`
	VerifiedAt             *time.Time         `json:"verified_at,omitempty"`
	BankVerificationStatus VerificationStatus `json:"bank_verification_status"`
	VerificationData       map[string]any     `json:"verification_data,omitempty"` // Penny drop response

	// Cross-verification with GST
	CrossVerificationStatus VerificationStatus `json:"cross_verification_status"`
	NameMatchScore          float64            `json:"name_match_score"` // 0.0 to 1.0

	// Account status
	IsActive     bool   `json:"is_active"`
	IsPrimary    bool   `json:"is_primary"`              // Default account for this payee
	AccountLabel string `json:"account_label,omitempty"` // "Retail Operations", "Wholesale", etc.

	// Audit
	AddedAt  time.Time      `json:"added_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	Metadata map[string]any `json:"metadata,omitempty"`
}
