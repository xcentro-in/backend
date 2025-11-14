package payout

import (
	"time"
)

// PayoutProviderMapping represents a payee's registration with a payout provider
// Maps to xcentro.payee_provider_mappings table
type PayoutProviderMapping struct {
	MappingID string // UUID stored as string
	PayeeID   string // UUID stored as string
	AccountID string // UUID stored as string

	// Provider details
	Provider              ProviderType
	ProviderBeneficiaryID string
	ProviderAccountID     string

	// Onboarding status
	OnboardingStatus       OnboardingStatus
	OnboardedAt            *time.Time
	OnboardingErrorCode    string
	OnboardingErrorMessage string

	// Provider metadata
	ProviderMetadata map[string]interface{}

	// Audit fields
	CreatedAt    time.Time
	CreatedBy    string // UUID stored as string
	UpdatedAt    *time.Time
	LastSyncedAt *time.Time
}

// OnboardingStatus represents payee onboarding status with provider
type OnboardingStatus string

const (
	OnboardingStatusNotOnboarded OnboardingStatus = "not_onboarded"
	OnboardingStatusPending      OnboardingStatus = "pending"
	OnboardingStatusActive       OnboardingStatus = "active"
	OnboardingStatusFailed       OnboardingStatus = "failed"
	OnboardingStatusSuspended    OnboardingStatus = "suspended"
	OnboardingStatusDeactivated  OnboardingStatus = "deactivated"
)

// IsOnboarded returns true if payee is successfully onboarded and active
func (m *PayoutProviderMapping) IsOnboarded() bool {
	return m.OnboardingStatus == OnboardingStatusActive && m.ProviderBeneficiaryID != ""
}

// CanRetryOnboarding returns true if onboarding can be retried
func (m *PayoutProviderMapping) CanRetryOnboarding() bool {
	return m.OnboardingStatus == OnboardingStatusNotOnboarded || m.OnboardingStatus == OnboardingStatusFailed
}

// PayoutTransferRequest represents a payout transfer request (for audit/tracking)
// Maps to xcentro.payout_transfer_requests table
type PayoutTransferRequest struct {
	TransferRequestID string // UUID stored as string
	VoucherID         string // UUID stored as string

	// Provider details
	Provider              ProviderType
	ProviderBeneficiaryID string
	ProviderTransferID    string

	// Transfer details
	Amount       int64
	Currency     string
	Purpose      string
	Reference    string
	Narration    string
	TransferMode string

	// Approval workflow
	RequiresApproval bool
	ApprovalMode     ApprovalMode
	ApprovedBy       string // UUID stored as string
	ApprovedAt       *time.Time

	// Status tracking (webhook-driven)
	Status       TransferStatusType
	InitiatedAt  *time.Time
	ProcessingAt *time.Time
	CompletedAt  *time.Time
	FailedAt     *time.Time
	FailureReason string
	UTR          string

	// Retry tracking (placeholder for future)
	RetryCount    int
	MaxRetries    int
	NextRetryAt   *time.Time
	LastRetryAt   *time.Time

	// Webhook tracking
	LastWebhookAt     *time.Time
	WebhookTimeoutAt  *time.Time
	WebhookHistory    []map[string]interface{} // Array of webhook payloads
	ProviderMetadata  map[string]interface{}

	// Audit
	CreatedAt time.Time
	UpdatedAt *time.Time
}

// ApprovalMode indicates how transfer was approved
type ApprovalMode string

const (
	ApprovalModeAuto   ApprovalMode = "auto"   // Auto-approved by system (amount < limit)
	ApprovalModeManual ApprovalMode = "manual" // Manually approved by operations
	ApprovalModeNone   ApprovalMode = "none"   // One-step provider, no approval needed
)

// IsApproved returns true if transfer is approved (or doesn't require approval)
func (t *PayoutTransferRequest) IsApproved() bool {
	return !t.RequiresApproval || (t.ApprovedBy != "" && t.ApprovedAt != nil)
}

// CanBeApproved returns true if transfer is pending approval
func (t *PayoutTransferRequest) CanBeApproved() bool {
	return t.RequiresApproval && t.Status == TransferStatusPendingApproval && !t.IsApproved()
}

// PayoutConfiguration represents payout system configuration
// Maps to xcentro.payout_configuration table
type PayoutConfiguration struct {
	ConfigID   string  // UUID stored as string
	BusinessID *string // UUID stored as string, NULL = global default

	// Auto-approval settings
	AutoApprovalEnabled bool
	AutoApprovalLimit   int64 // Amount in paise

	// Transfer preferences
	PreferredTransferMode string // NEFT, RTGS, IMPS

	// Retry settings (placeholder for future)
	MaxRetryAttempts      int
	RetryDelayMinutes     int
	RetryBackoffMultiplier float64

	// Webhook timeout
	WebhookTimeoutMinutes int

	// Worker settings
	BatchSize     int
	WorkerEnabled bool

	// Audit
	CreatedAt time.Time
	UpdatedAt *time.Time
}

// IsGlobalConfig returns true if this is the global default configuration
func (c *PayoutConfiguration) IsGlobalConfig() bool {
	return c.BusinessID == nil
}

// ShouldAutoApprove returns true if amount is within auto-approval limit
func (c *PayoutConfiguration) ShouldAutoApprove(amount int64) bool {
	return c.AutoApprovalEnabled && amount <= c.AutoApprovalLimit
}
