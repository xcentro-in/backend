package payment

import "time"

// Now returns the current time - helper function for mock data
func Now() time.Time {
	return time.Now()
}

// BusinessWallet represents a collection of stored payment sources for a business
type BusinessWallet struct {
	BusinessID   string         `json:"business_id"`
	StoredCards  []*StoredCard  `json:"stored_cards"`
	BankAccounts []*BankAccount `json:"bank_accounts"`
	UPIDetails   []*UPIDetail   `json:"upi_details"`
	CreditLines  []*CreditLine  `json:"credit_lines"`
	LastUpdated  time.Time      `json:"last_updated"`
}

// StoredCard represents a PCI-compliant stored card for a business
type StoredCard struct {
	CardID         string         `json:"card_id"`
	BusinessID     string         `json:"business_id"`
	Token          string         `json:"token"`            // Tokenized card reference
	LastFourDigits string         `json:"last_four_digits"` // Only last 4 digits
	CardType       CardType       `json:"card_type"`        // visa, mastercard, etc.
	CardCategory   CardCategory   `json:"card_category"`    // credit, debit, corporate
	ExpiryMonth    string         `json:"expiry_month"`     // MM format
	ExpiryYear     string         `json:"expiry_year"`      // YY format
	IssuerBank     string         `json:"issuer_bank"`      // Bank name
	CardHolderName         string         `json:"card_holder_name"` // Masked or business name
	IsActive               bool           `json:"is_active"`
	IsFailedTokenization   bool           `json:"is_failed_tokenization"`       // True if tokenization payment failed (vs user deactivation)
	FailureReason          string         `json:"failure_reason,omitempty"`     // Reason for tokenization failure (when is_active = false)
	GatewayUsed            GatewayType    `json:"gateway_used"`                 // Which gateway tokenized this
	AddedAt                time.Time      `json:"added_at"`
	LastUsedAt     *time.Time     `json:"last_used_at,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// BankAccount represents a bank account stored for a business
type BankAccount struct {
	AccountID         string          `json:"account_id"`
	BusinessID        string          `json:"business_id"`
	AccountNumber     string          `json:"account_number"`   // Full number (internal use)
	LastFiveDigits    string          `json:"last_five_digits"` // For display (PCI safe)
	BankName          string          `json:"bank_name"`
	BranchName        string          `json:"branch_name"`
	IFSCCode          string          `json:"ifsc_code"`
	AccountType       BankAccountType `json:"account_type"`
	AccountHolderName string          `json:"account_holder_name"`
	IsActive          bool            `json:"is_active"`
	IsVerified        bool            `json:"is_verified"` // Penny drop verification
	AddedAt           time.Time       `json:"added_at"`
	LastUsedAt        *time.Time      `json:"last_used_at,omitempty"`
	Metadata          map[string]any  `json:"metadata,omitempty"`
}

// UPIDetail represents a UPI ID stored for a business
type UPIDetail struct {
	UPIID      string         `json:"upi_id"`
	BusinessID string         `json:"business_id"`
	UPIAddress string         `json:"upi_address"` // Full UPI ID (can show fully)
	Provider   UPIProvider    `json:"provider"`    // googlepay, phonepe, etc.
	IsActive   bool           `json:"is_active"`
	IsVerified bool           `json:"is_verified"` // Verification status
	AddedAt    time.Time      `json:"added_at"`
	LastUsedAt *time.Time     `json:"last_used_at,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// CreditLine represents a credit line available to a business (placeholder for future)
type CreditLine struct {
	CreditLineID   string           `json:"credit_line_id"`
	BusinessID     string           `json:"business_id"`
	Provider       string           `json:"provider"`        // Credit line provider name
	CreditLimit    int64            `json:"credit_limit"`    // Total credit limit in smallest currency unit
	AvailableLimit int64            `json:"available_limit"` // Available credit in smallest currency unit
	UsedLimit      int64            `json:"used_limit"`      // Used credit amount
	Currency       Currency         `json:"currency"`
	InterestRate   float64          `json:"interest_rate"` // Annual interest rate
	Status         CreditLineStatus `json:"status"`
	IsActive       bool             `json:"is_active"`
	ExpiresAt      *time.Time       `json:"expires_at,omitempty"`
	AddedAt        time.Time        `json:"added_at"`
	LastUsedAt     *time.Time       `json:"last_used_at,omitempty"`
	Metadata       map[string]any   `json:"metadata,omitempty"`
}

// PaymentSource represents a generic payment source interface
type PaymentSource interface {
	GetSourceType() PaymentSourceType
	GetSourceID() string
	GetBusinessID() string
	IsSourceActive() bool
	GetDisplayInfo() map[string]any
}

// Implement PaymentSource interface for all source types

func (sc *StoredCard) GetSourceType() PaymentSourceType {
	return PaymentSourceCard
}

func (sc *StoredCard) GetSourceID() string {
	return sc.CardID
}

func (sc *StoredCard) GetBusinessID() string {
	return sc.BusinessID
}

func (sc *StoredCard) IsSourceActive() bool {
	return sc.IsActive
}

func (sc *StoredCard) GetDisplayInfo() map[string]any {
	return map[string]any{
		"source_type":      "card",
		"last_four_digits": sc.LastFourDigits,
		"card_type":        sc.CardType,
		"card_category":    sc.CardCategory,
		"issuer_bank":      sc.IssuerBank,
		"expiry":           sc.ExpiryMonth + "/" + sc.ExpiryYear,
	}
}

func (ba *BankAccount) GetSourceType() PaymentSourceType {
	return PaymentSourceBankAccount
}

func (ba *BankAccount) GetSourceID() string {
	return ba.AccountID
}

func (ba *BankAccount) GetBusinessID() string {
	return ba.BusinessID
}

func (ba *BankAccount) IsSourceActive() bool {
	return ba.IsActive
}

func (ba *BankAccount) GetDisplayInfo() map[string]any {
	return map[string]any{
		"source_type":      "bank_account",
		"last_five_digits": ba.LastFiveDigits,
		"bank_name":        ba.BankName,
		"ifsc_code":        ba.IFSCCode,
		"account_type":     ba.AccountType,
		"is_verified":      ba.IsVerified,
	}
}

func (ud *UPIDetail) GetSourceType() PaymentSourceType {
	return PaymentSourceUPI
}

func (ud *UPIDetail) GetSourceID() string {
	return ud.UPIID
}

func (ud *UPIDetail) GetBusinessID() string {
	return ud.BusinessID
}

func (ud *UPIDetail) IsSourceActive() bool {
	return ud.IsActive
}

func (ud *UPIDetail) GetDisplayInfo() map[string]any {
	return map[string]any{
		"source_type": "upi",
		"upi_address": ud.UPIAddress,
		"provider":    ud.Provider,
		"is_verified": ud.IsVerified,
	}
}

func (cl *CreditLine) GetSourceType() PaymentSourceType {
	return PaymentSourceCreditLine
}

func (cl *CreditLine) GetSourceID() string {
	return cl.CreditLineID
}

func (cl *CreditLine) GetBusinessID() string {
	return cl.BusinessID
}

func (cl *CreditLine) IsSourceActive() bool {
	return cl.IsActive
}

func (cl *CreditLine) GetDisplayInfo() map[string]any {
	return map[string]any{
		"source_type":     "credit_line",
		"provider":        cl.Provider,
		"available_limit": float64(cl.AvailableLimit) / 100, // Convert to rupees
		"total_limit":     float64(cl.CreditLimit) / 100,
		"currency":        cl.Currency,
		"status":          cl.Status,
	}
}
