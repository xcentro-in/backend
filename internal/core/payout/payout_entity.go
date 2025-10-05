package payout

import (
	"time"

	"github.com/google/uuid"
)

// PayoutProviderMapping represents a payee's registration with a payout provider
// Maps to xcentro.payee_provider_mappings table
type PayoutProviderMapping struct {
	MappingID uuid.UUID
	PayeeID   uuid.UUID
	AccountID uuid.UUID

	// Provider details
	Provider              ProviderType
	ProviderBeneficiaryID string
	ProviderAccountID     string

	// Onboarding status
	OnboardingStatus      OnboardingStatus
	OnboardedAt           *time.Time
	OnboardingErrorCode   string
	OnboardingErrorMessage string

	// Provider metadata
	ProviderMetadata map[string]interface{}

	// Audit fields
	CreatedAt    time.Time
	CreatedBy    *uuid.UUID
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
// This will be used in Phase 9 for the transfer requests table
type PayoutTransferRequest struct {
	TransferRequestID uuid.UUID
	VoucherID         uuid.UUID

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

	// Status tracking
	Status              TransferStatusType
	RequiresApproval    bool
	ApprovalMode        ApprovalMode
	ApprovedBy          *uuid.UUID
	ApprovedAt          *time.Time
	CompletedAt         *time.Time
	FailedAt            *time.Time
	FailureReason       string
	UTR                 string
	ProviderMetadata    map[string]interface{}

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
	return !t.RequiresApproval || (t.ApprovedBy != nil && t.ApprovedAt != nil)
}

// CanBeApproved returns true if transfer is pending approval
func (t *PayoutTransferRequest) CanBeApproved() bool {
	return t.RequiresApproval && t.Status == TransferStatusPending && !t.IsApproved()
}
