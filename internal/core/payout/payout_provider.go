package payout

import (
	"context"
)

// PayoutProvider defines the interface for payout provider integrations (Castler, Razorpay, Bank APIs)
// Supports both one-step and two-step transfer workflows
type PayoutProvider interface {
	// GetProviderType returns the provider identifier (castler, razorpay, bank_api)
	GetProviderType() ProviderType

	// SupportsTwoStepApproval returns true if provider requires separate create+approve calls
	// Castler: true (create beneficiary/transfer + approve transfer)
	// Razorpay: false (direct payout)
	// Bank API: false (direct transfer)
	SupportsTwoStepApproval() bool

	// OnboardBeneficiary registers a payee with the payout provider
	// Returns provider-specific beneficiary ID (Castler: beneficiary_id, Razorpay: contact_id)
	OnboardBeneficiary(ctx context.Context, req *OnboardBeneficiaryRequest) (*OnboardBeneficiaryResponse, error)

	// GetBeneficiaryStatus checks if beneficiary is active with provider
	GetBeneficiaryStatus(ctx context.Context, providerBeneficiaryID string) (*BeneficiaryStatus, error)

	// CreateTransferRequest initiates a payout transfer
	// For two-step providers: Creates transfer in pending state (requires approval)
	// For one-step providers: Creates and executes transfer immediately
	CreateTransferRequest(ctx context.Context, req *CreateTransferRequest) (*CreateTransferResponse, error)

	// ApproveTransferRequest approves a pending transfer (only for two-step providers)
	// For one-step providers: Should return error indicating not supported
	ApproveTransferRequest(ctx context.Context, req *ApproveTransferRequest) (*ApproveTransferResponse, error)

	// GetTransferStatus queries the status of a transfer
	GetTransferStatus(ctx context.Context, providerTransferID string) (*TransferStatus, error)

	// CancelTransfer cancels a pending transfer (if supported by provider)
	CancelTransfer(ctx context.Context, providerTransferID string) error
}

// ProviderType represents different payout providers
type ProviderType string

const (
	ProviderCastler  ProviderType = "castler"   // Castler (primary)
	ProviderRazorpay ProviderType = "razorpay"  // Razorpay (backup)
	ProviderBankAPI  ProviderType = "bank_api"  // Direct bank API (future)
)

// OnboardBeneficiaryRequest represents payee onboarding request
type OnboardBeneficiaryRequest struct {
	// Payee identification
	PayeeID   string // UUID stored as string
	AccountID string // UUID stored as string

	// Bank account details
	AccountNumber     string
	IFSCCode          string
	AccountHolderName string
	BankName          string
	AccountType       string // savings, current

	// Business details (for vendors)
	GSTIN     string // Optional - only for vendors
	LegalName string // From GST verification
	PAN       string // Optional - for employees

	// Contact details
	Email string
	Phone string

	// Provider-specific metadata
	Metadata map[string]interface{}
}

// OnboardBeneficiaryResponse represents provider's beneficiary registration response
type OnboardBeneficiaryResponse struct {
	ProviderBeneficiaryID string                 // Provider's beneficiary ID
	ProviderAccountID     string                 // Provider's account reference (if separate)
	Status                BeneficiaryStatusType  // active, pending, failed
	Message               string                 // Provider message
	Metadata              map[string]interface{} // Provider-specific data
}

// BeneficiaryStatus represents beneficiary status with provider
type BeneficiaryStatus struct {
	ProviderBeneficiaryID string
	Status                BeneficiaryStatusType
	IsActive              bool
	LastVerifiedAt        *string // ISO timestamp
	Metadata              map[string]interface{}
}

// BeneficiaryStatusType represents beneficiary lifecycle states
type BeneficiaryStatusType string

const (
	BeneficiaryStatusActive    BeneficiaryStatusType = "active"
	BeneficiaryStatusPending   BeneficiaryStatusType = "pending"
	BeneficiaryStatusFailed    BeneficiaryStatusType = "failed"
	BeneficiaryStatusSuspended BeneficiaryStatusType = "suspended"
)

// CreateTransferRequest represents payout transfer initiation
type CreateTransferRequest struct {
	// Transfer identifiers
	TransferRequestID string // Our internal transfer request ID (used as customerRefId in Castler)
	VoucherID         string // UUID stored as string

	// Beneficiary
	ProviderBeneficiaryID string

	// Escrow configuration
	EscrowID string // Castler escrow account ID

	// Amount (in paise)
	Amount   int64
	Currency string // INR, USD

	// Transfer details
	Purpose     string // "vendor_payment", "salary", etc.
	Reference   string // Our internal reference (voucher_number)
	Narration   string // Bank statement narration
	UTRRequired bool   // Request UTR from provider

	// Transfer mode (IMPS, NEFT, RTGS - if provider supports choice)
	TransferMode string

	// Metadata
	Metadata map[string]interface{}
}

// CreateTransferResponse represents provider's transfer creation response
type CreateTransferResponse struct {
	ProviderTransferID string                // Provider's transfer ID
	Status             TransferStatusType    // pending, processing, completed
	RequiresApproval   bool                  // True for two-step providers
	Message            string                // Provider message
	EstimatedSettlement *string              // ISO timestamp (optional)
	Metadata            map[string]interface{}
}

// ApproveTransferRequest represents transfer approval request (for two-step providers)
type ApproveTransferRequest struct {
	ProviderTransferID string
	ApprovedBy         string // Operations user ID (UUID as string)
	Remarks            string // Optional approval remarks
}

// ApproveTransferResponse represents provider's approval response
type ApproveTransferResponse struct {
	ProviderTransferID string                 // Provider's transfer ID
	Status             TransferStatusType     // processing, completed
	Message            string                 // Provider message
	UTR                string                 // Bank UTR (if available)
	Metadata           map[string]interface{}
}

// TransferStatus represents current status of a transfer
type TransferStatus struct {
	ProviderTransferID string
	Status             TransferStatusType
	UTR                string // Bank UTR
	FailureReason      string
	ProcessedAt        *string // ISO timestamp
	SettledAt          *string // ISO timestamp
	Metadata           map[string]interface{}
}

// TransferStatusType represents transfer lifecycle states (simplified - no approval workflow)
type TransferStatusType string

const (
	TransferStatusInitiated  TransferStatusType = "initiated"  // Sent to Castler, awaiting webhook
	TransferStatusProcessing TransferStatusType = "processing" // Castler confirmed processing (from webhook)
	TransferStatusCompleted  TransferStatusType = "completed"  // Successfully completed (from webhook)
	TransferStatusFailed     TransferStatusType = "failed"     // Failed (from webhook)
	TransferStatusTimeout    TransferStatusType = "timeout"    // No webhook received (needs investigation)
)

// PayoutProviderError represents provider-specific errors
type PayoutProviderError struct {
	Provider    ProviderType
	ErrorCode   string
	Message     string
	IsRetryable bool
	Metadata    map[string]interface{}
}

func (e *PayoutProviderError) Error() string {
	return e.Message
}

// Common error codes
const (
	ErrCodeInvalidBeneficiary    = "INVALID_BENEFICIARY"
	ErrCodeInsufficientBalance   = "INSUFFICIENT_BALANCE"
	ErrCodeTransferLimitExceeded = "TRANSFER_LIMIT_EXCEEDED"
	ErrCodeBeneficiaryNotActive  = "BENEFICIARY_NOT_ACTIVE"
	ErrCodeTransferNotFound      = "TRANSFER_NOT_FOUND"
	ErrCodeAPITimeout            = "API_TIMEOUT"
	ErrCodeRateLimitExceeded     = "RATE_LIMIT_EXCEEDED"
	ErrCodeProviderDown          = "PROVIDER_DOWN"
)
