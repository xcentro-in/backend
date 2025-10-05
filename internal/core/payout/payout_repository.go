package payout

import (
	"context"
	"time"
)

// PayoutProviderMappingRepository defines the interface for provider mapping persistence
type PayoutProviderMappingRepository interface {
	// Create creates a new provider mapping
	Create(ctx context.Context, mapping *PayoutProviderMapping) error

	// GetByPayeeAndProvider retrieves mapping for a specific payee+account+provider
	GetByPayeeAndProvider(ctx context.Context, payeeID string, accountID string, provider ProviderType) (*PayoutProviderMapping, error)

	// GetByPayeeAndAccount retrieves all provider mappings for a payee+account
	// Returns mappings for all providers (Castler, Razorpay, etc.)
	GetByPayeeAndAccount(ctx context.Context, payeeID string, accountID string) ([]*PayoutProviderMapping, error)

	// GetActiveMapping retrieves the active provider mapping for a payee+account
	// Returns the first active mapping (typically Castler, fallback to Razorpay)
	GetActiveMapping(ctx context.Context, payeeID string, accountID string) (*PayoutProviderMapping, error)

	// UpdateOnboardingStatus updates the onboarding status of a mapping
	UpdateOnboardingStatus(ctx context.Context, mappingID string, status OnboardingStatus, providerBeneficiaryID string, providerAccountID string, metadata map[string]interface{}) error

	// UpdateOnboardingError records an onboarding failure
	UpdateOnboardingError(ctx context.Context, mappingID string, errorCode string, errorMessage string) error

	// GetPendingOnboardings retrieves mappings that need onboarding (for worker)
	GetPendingOnboardings(ctx context.Context, limit int) ([]*PayoutProviderMapping, error)

	// GetFailedOnboardings retrieves failed onboardings for retry (for worker)
	GetFailedOnboardings(ctx context.Context, limit int, olderThan time.Duration) ([]*PayoutProviderMapping, error)

	// UpdateLastSyncedAt updates the last sync timestamp
	UpdateLastSyncedAt(ctx context.Context, mappingID string, timestamp time.Time) error
}

// PayoutTransferRepository defines the interface for transfer request persistence (Phase 9)
type PayoutTransferRepository interface {
	// Create creates a new transfer request
	Create(ctx context.Context, transfer *PayoutTransferRequest) error

	// GetByID retrieves a transfer request by ID
	GetByID(ctx context.Context, transferRequestID string) (*PayoutTransferRequest, error)

	// GetByVoucherID retrieves all transfer requests for a voucher
	GetByVoucherID(ctx context.Context, voucherID string) ([]*PayoutTransferRequest, error)

	// UpdateStatus updates the transfer status
	UpdateStatus(ctx context.Context, transferRequestID string, status TransferStatusType, utr string, metadata map[string]interface{}) error

	// ApproveTransfer records transfer approval
	ApproveTransfer(ctx context.Context, transferRequestID string, approvedBy string, approvalMode ApprovalMode, timestamp time.Time) error

	// MarkCompleted marks transfer as completed
	MarkCompleted(ctx context.Context, transferRequestID string, utr string, completedAt time.Time) error

	// MarkFailed marks transfer as failed
	MarkFailed(ctx context.Context, transferRequestID string, failureReason string, failedAt time.Time) error

	// GetPendingApprovals retrieves transfers pending approval (for operations dashboard)
	GetPendingApprovals(ctx context.Context, limit int) ([]*PayoutTransferRequest, error)

	// GetPendingProcessing retrieves approved transfers pending processing (for worker)
	GetPendingProcessing(ctx context.Context, limit int) ([]*PayoutTransferRequest, error)
}
