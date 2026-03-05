package business_rules

import (
	"context"
	"time"

	"xcentro.in/internal/core/payment"
)

// BankVerificationService defines the interface for bank account verification services
// Used for penny drop verification when adding bank accounts to business wallets
type BankVerificationService interface {
	// VerifyBankAccount performs penny drop verification for a bank account
	// Returns verification status, account holder details, and bank information
	VerifyBankAccount(ctx context.Context, request BankVerificationRequest) (*BankVerificationResponse, error)

	// GetProviderType returns the verification service provider identifier
	GetProviderType() BankVerificationProvider
}

// BankVerificationProvider represents different bank verification service providers
type BankVerificationProvider string

const (
	BankVerificationProviderRazorpay   BankVerificationProvider = "razorpay"
	BankVerificationProviderCashfree   BankVerificationProvider = "cashfree"
	BankVerificationProviderInstamojo  BankVerificationProvider = "instamojo"
	BankVerificationProviderMock       BankVerificationProvider = "mock"
)

// BankVerificationRequest represents a request to verify a bank account via penny drop
type BankVerificationRequest struct {
	BusinessID        string                 `json:"business_id"`
	RequestID         string                 `json:"request_id"`          // Unique request identifier
	AccountNumber     string                 `json:"account_number"`
	IFSCCode          string                 `json:"ifsc_code"`
	AccountHolderName string                 `json:"account_holder_name"` // Name provided by user
	BankName          string                 `json:"bank_name"`
	BranchName        string                 `json:"branch_name,omitempty"`
	AccountType       payment.BankAccountType `json:"account_type"`
	CallbackURL       string                 `json:"callback_url,omitempty"` // For async verification
	Metadata          map[string]any         `json:"metadata,omitempty"`
}

// BankVerificationResponse represents the response from bank account verification
type BankVerificationResponse struct {
	RequestID            string                      `json:"request_id"`
	Status               BankVerificationStatus      `json:"status"`
	VerifiedAccountName  string                      `json:"verified_account_name"`  // Name from bank records
	BankName             string                      `json:"bank_name"`
	BranchName           string                      `json:"branch_name"`
	IFSCCode             string                      `json:"ifsc_code"`
	AccountNumber        string                      `json:"account_number"`        // May be masked
	AccountType          payment.BankAccountType     `json:"account_type"`
	IsAccountActive      bool                        `json:"is_account_active"`
	VerificationAmount   int64                       `json:"verification_amount"`   // Penny drop amount in paise
	TransactionReference string                      `json:"transaction_reference"` // UTR/reference number
	VerifiedAt           time.Time                   `json:"verified_at"`
	Provider             BankVerificationProvider    `json:"provider"`
	FailureReason        string                      `json:"failure_reason,omitempty"`
	Metadata             map[string]any              `json:"metadata,omitempty"`
}

// BankVerificationStatus represents the status of bank verification
type BankVerificationStatus string

const (
	BankVerificationStatusPending   BankVerificationStatus = "pending"
	BankVerificationStatusSuccess   BankVerificationStatus = "success"
	BankVerificationStatusFailed    BankVerificationStatus = "failed"
	BankVerificationStatusTimeout   BankVerificationStatus = "timeout"
)

// BankVerificationError represents specific error types for bank verification
type BankVerificationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

func (e *BankVerificationError) Error() string {
	return e.Message
}

// Common bank verification error codes
const (
	ErrCodeInvalidIFSC         = "INVALID_IFSC"
	ErrCodeInvalidAccountNumber = "INVALID_ACCOUNT_NUMBER"
	ErrCodeAccountClosed        = "ACCOUNT_CLOSED"
	ErrCodeAccountFrozen        = "ACCOUNT_FROZEN"
	ErrCodeInsufficientBalance  = "INSUFFICIENT_BALANCE"
	ErrCodeNameMismatch         = "NAME_MISMATCH"
	ErrCodeBankServiceDown      = "BANK_SERVICE_DOWN"
	ErrCodeAPITimeout           = "API_TIMEOUT"
	ErrCodeUnknownError         = "UNKNOWN_ERROR"
)

// NewBankVerificationError creates a new bank verification error
func NewBankVerificationError(code, message, details string) *BankVerificationError {
	return &BankVerificationError{
		Code:    code,
		Message: message,
		Details: details,
	}
}