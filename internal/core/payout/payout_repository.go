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

	// GetByVoucherID retrieves transfer request for a voucher
	GetByVoucherID(ctx context.Context, voucherID string) (*PayoutTransferRequest, error)

	// GetByProviderTransferID retrieves transfer by provider's transfer ID (for webhooks)
	GetByProviderTransferID(ctx context.Context, providerTransferID string) (*PayoutTransferRequest, error)

	// GetByStatus retrieves transfers with specific status (for worker)
	GetByStatus(ctx context.Context, status TransferStatusType, limit int) ([]*PayoutTransferRequest, error)

	// UpdateStatus updates the transfer status with timestamp
	UpdateStatus(ctx context.Context, transferRequestID string, status TransferStatusType, timestamp time.Time) error

	// UpdateProviderTransferID sets the provider's transfer ID after initiation
	UpdateProviderTransferID(ctx context.Context, transferRequestID string, providerTransferID string) error

	// MarkCompleted marks transfer as completed
	MarkCompleted(ctx context.Context, transferRequestID string, utr string, completedAt time.Time) error

	// MarkFailed marks transfer as failed
	MarkFailed(ctx context.Context, transferRequestID string, failureReason string, failedAt time.Time) error

	// SetWebhookTimeout sets the webhook timeout timestamp
	SetWebhookTimeout(ctx context.Context, transferRequestID string, timeoutAt time.Time) error

	// AppendWebhookHistory appends a webhook payload to history
	AppendWebhookHistory(ctx context.Context, transferRequestID string, payload map[string]interface{}) error

	// GetWebhookTimeouts retrieves transfers that exceeded webhook timeout
	GetWebhookTimeouts(ctx context.Context, limit int) ([]*PayoutTransferRequest, error)
}

// PayoutConfigurationRepository defines the interface for payout configuration persistence
type PayoutConfigurationRepository interface {
	// GetConfig retrieves configuration (business-specific or global default)
	GetConfig(ctx context.Context, businessID *string) (*PayoutConfiguration, error)

	// UpdateConfig updates configuration
	UpdateConfig(ctx context.Context, config *PayoutConfiguration) error

	// Create creates a new business-specific configuration
	Create(ctx context.Context, config *PayoutConfiguration) error
}

// OpsPayoutRepository defines the interface for ops dashboard payout queries
type OpsPayoutRepository interface {
	// ListPayoutsPaginated retrieves paginated list of payout transfers with recipient info
	// status can be: "in_progress" (initiated/processing), "completed", "failed" (failed/timeout)
	ListPayoutsPaginated(ctx context.Context, page int, limit int, status string) ([]*OpsPayoutListItem, int, error)

	// GetPayoutSummary retrieves detailed summary for a single payout transfer
	GetPayoutSummary(ctx context.Context, transferRequestID string) (*OpsPayoutSummary, error)
}
