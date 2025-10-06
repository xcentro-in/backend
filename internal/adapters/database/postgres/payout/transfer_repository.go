package payout

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/payout"
)

// PostgresPayoutTransferRepository implements PayoutTransferRepository using PostgreSQL
type PostgresPayoutTransferRepository struct {
	querier client.Querier
}

// NewPostgresPayoutTransferRepository creates a new PostgreSQL payout transfer repository
func NewPostgresPayoutTransferRepository(db client.DB) payout.PayoutTransferRepository {
	return &PostgresPayoutTransferRepository{
		querier: db,
	}
}

// Create creates a new transfer request
func (r *PostgresPayoutTransferRepository) Create(ctx context.Context, transfer *payout.PayoutTransferRequest) error {
	// Generate UUID if not provided
	if transfer.TransferRequestID == "" {
		transfer.TransferRequestID = uuid.New().String()
	}

	query := `
		INSERT INTO xcentro.payout_transfer_requests (
			transfer_request_id, voucher_id, provider, provider_beneficiary_id,
			amount, currency, purpose, reference, narration, transfer_mode,
			requires_approval, approval_mode, approved_by, approved_at,
			status, retry_count, max_retries,
			provider_metadata, created_at
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14,
			$15, $16, $17,
			$18, $19
		)
	`

	// Marshal provider metadata
	metadataJSON, err := json.Marshal(transfer.ProviderMetadata)
	if err != nil {
		return fmt.Errorf("failed to marshal provider_metadata: %w", err)
	}

	_, err = r.querier.Exec(ctx, query,
		transfer.TransferRequestID, transfer.VoucherID, transfer.Provider, transfer.ProviderBeneficiaryID,
		transfer.Amount, transfer.Currency, transfer.Purpose, transfer.Reference, transfer.Narration, transfer.TransferMode,
		transfer.RequiresApproval, transfer.ApprovalMode, transfer.ApprovedBy, transfer.ApprovedAt,
		transfer.Status, transfer.RetryCount, transfer.MaxRetries,
		metadataJSON, time.Now(),
	)

	if err != nil {
		return fmt.Errorf("failed to create transfer request: %w", err)
	}

	return nil
}

// GetByID retrieves a transfer request by ID
func (r *PostgresPayoutTransferRepository) GetByID(ctx context.Context, transferRequestID string) (*payout.PayoutTransferRequest, error) {
	query := `
		SELECT
			transfer_request_id, voucher_id, provider, provider_beneficiary_id, provider_transfer_id,
			amount, currency, purpose, reference, narration, transfer_mode,
			requires_approval, approval_mode, approved_by, approved_at,
			status, initiated_at, processing_at, completed_at, failed_at, failure_reason, utr,
			retry_count, max_retries, next_retry_at, last_retry_at,
			last_webhook_at, webhook_timeout_at, webhook_history, provider_metadata,
			created_at, updated_at
		FROM xcentro.payout_transfer_requests
		WHERE transfer_request_id = $1
	`

	var transfer payout.PayoutTransferRequest
	var metadataJSON, historyJSON []byte

	err := r.querier.QueryRow(ctx, query, transferRequestID).Scan(
		&transfer.TransferRequestID, &transfer.VoucherID, &transfer.Provider, &transfer.ProviderBeneficiaryID, &transfer.ProviderTransferID,
		&transfer.Amount, &transfer.Currency, &transfer.Purpose, &transfer.Reference, &transfer.Narration, &transfer.TransferMode,
		&transfer.RequiresApproval, &transfer.ApprovalMode, &transfer.ApprovedBy, &transfer.ApprovedAt,
		&transfer.Status, &transfer.InitiatedAt, &transfer.ProcessingAt, &transfer.CompletedAt, &transfer.FailedAt, &transfer.FailureReason, &transfer.UTR,
		&transfer.RetryCount, &transfer.MaxRetries, &transfer.NextRetryAt, &transfer.LastRetryAt,
		&transfer.LastWebhookAt, &transfer.WebhookTimeoutAt, &historyJSON, &metadataJSON,
		&transfer.CreatedAt, &transfer.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("transfer request not found: %s", transferRequestID)
		}
		return nil, fmt.Errorf("failed to get transfer request: %w", err)
	}

	// Unmarshal JSONB fields
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &transfer.ProviderMetadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal provider_metadata: %w", err)
		}
	}

	if len(historyJSON) > 0 {
		if err := json.Unmarshal(historyJSON, &transfer.WebhookHistory); err != nil {
			return nil, fmt.Errorf("failed to unmarshal webhook_history: %w", err)
		}
	}

	return &transfer, nil
}

// GetByVoucherID retrieves transfer request for a voucher
func (r *PostgresPayoutTransferRepository) GetByVoucherID(ctx context.Context, voucherID string) (*payout.PayoutTransferRequest, error) {
	query := `
		SELECT
			transfer_request_id, voucher_id, provider, provider_beneficiary_id, provider_transfer_id,
			amount, currency, purpose, reference, narration, transfer_mode,
			requires_approval, approval_mode, approved_by, approved_at,
			status, initiated_at, processing_at, completed_at, failed_at, failure_reason, utr,
			retry_count, max_retries, next_retry_at, last_retry_at,
			last_webhook_at, webhook_timeout_at, webhook_history, provider_metadata,
			created_at, updated_at
		FROM xcentro.payout_transfer_requests
		WHERE voucher_id = $1
	`

	var transfer payout.PayoutTransferRequest
	var metadataJSON, historyJSON []byte

	err := r.querier.QueryRow(ctx, query, voucherID).Scan(
		&transfer.TransferRequestID, &transfer.VoucherID, &transfer.Provider, &transfer.ProviderBeneficiaryID, &transfer.ProviderTransferID,
		&transfer.Amount, &transfer.Currency, &transfer.Purpose, &transfer.Reference, &transfer.Narration, &transfer.TransferMode,
		&transfer.RequiresApproval, &transfer.ApprovalMode, &transfer.ApprovedBy, &transfer.ApprovedAt,
		&transfer.Status, &transfer.InitiatedAt, &transfer.ProcessingAt, &transfer.CompletedAt, &transfer.FailedAt, &transfer.FailureReason, &transfer.UTR,
		&transfer.RetryCount, &transfer.MaxRetries, &transfer.NextRetryAt, &transfer.LastRetryAt,
		&transfer.LastWebhookAt, &transfer.WebhookTimeoutAt, &historyJSON, &metadataJSON,
		&transfer.CreatedAt, &transfer.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("transfer request not found for voucher: %s", voucherID)
		}
		return nil, fmt.Errorf("failed to get transfer request: %w", err)
	}

	// Unmarshal JSONB fields
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &transfer.ProviderMetadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal provider_metadata: %w", err)
		}
	}

	if len(historyJSON) > 0 {
		if err := json.Unmarshal(historyJSON, &transfer.WebhookHistory); err != nil {
			return nil, fmt.Errorf("failed to unmarshal webhook_history: %w", err)
		}
	}

	return &transfer, nil
}

// GetByProviderTransferID retrieves transfer by provider's transfer ID (for webhooks)
func (r *PostgresPayoutTransferRepository) GetByProviderTransferID(ctx context.Context, providerTransferID string) (*payout.PayoutTransferRequest, error) {
	query := `
		SELECT
			transfer_request_id, voucher_id, provider, provider_beneficiary_id, provider_transfer_id,
			amount, currency, purpose, reference, narration, transfer_mode,
			requires_approval, approval_mode, approved_by, approved_at,
			status, initiated_at, processing_at, completed_at, failed_at, failure_reason, utr,
			retry_count, max_retries, next_retry_at, last_retry_at,
			last_webhook_at, webhook_timeout_at, webhook_history, provider_metadata,
			created_at, updated_at
		FROM xcentro.payout_transfer_requests
		WHERE provider_transfer_id = $1
	`

	var transfer payout.PayoutTransferRequest
	var metadataJSON, historyJSON []byte

	err := r.querier.QueryRow(ctx, query, providerTransferID).Scan(
		&transfer.TransferRequestID, &transfer.VoucherID, &transfer.Provider, &transfer.ProviderBeneficiaryID, &transfer.ProviderTransferID,
		&transfer.Amount, &transfer.Currency, &transfer.Purpose, &transfer.Reference, &transfer.Narration, &transfer.TransferMode,
		&transfer.RequiresApproval, &transfer.ApprovalMode, &transfer.ApprovedBy, &transfer.ApprovedAt,
		&transfer.Status, &transfer.InitiatedAt, &transfer.ProcessingAt, &transfer.CompletedAt, &transfer.FailedAt, &transfer.FailureReason, &transfer.UTR,
		&transfer.RetryCount, &transfer.MaxRetries, &transfer.NextRetryAt, &transfer.LastRetryAt,
		&transfer.LastWebhookAt, &transfer.WebhookTimeoutAt, &historyJSON, &metadataJSON,
		&transfer.CreatedAt, &transfer.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("transfer request not found for provider_transfer_id: %s", providerTransferID)
		}
		return nil, fmt.Errorf("failed to get transfer request: %w", err)
	}

	// Unmarshal JSONB fields
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &transfer.ProviderMetadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal provider_metadata: %w", err)
		}
	}

	if len(historyJSON) > 0 {
		if err := json.Unmarshal(historyJSON, &transfer.WebhookHistory); err != nil {
			return nil, fmt.Errorf("failed to unmarshal webhook_history: %w", err)
		}
	}

	return &transfer, nil
}

// GetByStatus retrieves transfers with specific status (for worker)
func (r *PostgresPayoutTransferRepository) GetByStatus(ctx context.Context, status payout.TransferStatusType, limit int) ([]*payout.PayoutTransferRequest, error) {
	query := `
		SELECT
			transfer_request_id, voucher_id, provider, provider_beneficiary_id, provider_transfer_id,
			amount, currency, purpose, reference, narration, transfer_mode,
			requires_approval, approval_mode, approved_by, approved_at,
			status, initiated_at, processing_at, completed_at, failed_at, failure_reason, utr,
			retry_count, max_retries, next_retry_at, last_retry_at,
			last_webhook_at, webhook_timeout_at, webhook_history, provider_metadata,
			created_at, updated_at
		FROM xcentro.payout_transfer_requests
		WHERE status = $1
		ORDER BY created_at ASC
		LIMIT $2
	`

	rows, err := r.querier.Query(ctx, query, status, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get transfers by status: %w", err)
	}
	defer rows.Close()

	var transfers []*payout.PayoutTransferRequest

	for rows.Next() {
		var transfer payout.PayoutTransferRequest
		var metadataJSON, historyJSON []byte

		err := rows.Scan(
			&transfer.TransferRequestID, &transfer.VoucherID, &transfer.Provider, &transfer.ProviderBeneficiaryID, &transfer.ProviderTransferID,
			&transfer.Amount, &transfer.Currency, &transfer.Purpose, &transfer.Reference, &transfer.Narration, &transfer.TransferMode,
			&transfer.RequiresApproval, &transfer.ApprovalMode, &transfer.ApprovedBy, &transfer.ApprovedAt,
			&transfer.Status, &transfer.InitiatedAt, &transfer.ProcessingAt, &transfer.CompletedAt, &transfer.FailedAt, &transfer.FailureReason, &transfer.UTR,
			&transfer.RetryCount, &transfer.MaxRetries, &transfer.NextRetryAt, &transfer.LastRetryAt,
			&transfer.LastWebhookAt, &transfer.WebhookTimeoutAt, &historyJSON, &metadataJSON,
			&transfer.CreatedAt, &transfer.UpdatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan transfer: %w", err)
		}

		// Unmarshal JSONB fields
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &transfer.ProviderMetadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal provider_metadata: %w", err)
			}
		}

		if len(historyJSON) > 0 {
			if err := json.Unmarshal(historyJSON, &transfer.WebhookHistory); err != nil {
				return nil, fmt.Errorf("failed to unmarshal webhook_history: %w", err)
			}
		}

		transfers = append(transfers, &transfer)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating transfer rows: %w", err)
	}

	return transfers, nil
}

// UpdateStatus updates the transfer status with timestamp
func (r *PostgresPayoutTransferRepository) UpdateStatus(ctx context.Context, transferRequestID string, status payout.TransferStatusType, timestamp time.Time) error {
	// Determine which timestamp field to update based on status
	var timestampField string
	switch status {
	case payout.TransferStatusInitiated:
		timestampField = "initiated_at"
	case payout.TransferStatusProcessing:
		timestampField = "processing_at"
	case payout.TransferStatusCompleted:
		timestampField = "completed_at"
	case payout.TransferStatusFailed, payout.TransferStatusTimeout:
		timestampField = "failed_at"
	}

	query := fmt.Sprintf(`
		UPDATE xcentro.payout_transfer_requests
		SET status = $2, %s = $3, updated_at = NOW()
		WHERE transfer_request_id = $1
	`, timestampField)

	result, err := r.querier.Exec(ctx, query, transferRequestID, status, timestamp)
	if err != nil {
		return fmt.Errorf("failed to update transfer status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("transfer request not found: %s", transferRequestID)
	}

	return nil
}

// UpdateProviderTransferID sets the provider's transfer ID after initiation
func (r *PostgresPayoutTransferRepository) UpdateProviderTransferID(ctx context.Context, transferRequestID string, providerTransferID string) error {
	query := `
		UPDATE xcentro.payout_transfer_requests
		SET provider_transfer_id = $2, updated_at = NOW()
		WHERE transfer_request_id = $1
	`

	result, err := r.querier.Exec(ctx, query, transferRequestID, providerTransferID)
	if err != nil {
		return fmt.Errorf("failed to update provider_transfer_id: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("transfer request not found: %s", transferRequestID)
	}

	return nil
}

// ApproveTransfer records transfer approval
func (r *PostgresPayoutTransferRepository) ApproveTransfer(ctx context.Context, transferRequestID string, approvedBy string, approvalMode payout.ApprovalMode, timestamp time.Time) error {
	query := `
		UPDATE xcentro.payout_transfer_requests
		SET status = $2, approved_by = $3, approval_mode = $4, approved_at = $5, updated_at = NOW()
		WHERE transfer_request_id = $1
	`

	result, err := r.querier.Exec(ctx, query, transferRequestID, payout.TransferStatusApproved, approvedBy, approvalMode, timestamp)
	if err != nil {
		return fmt.Errorf("failed to approve transfer: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("transfer request not found: %s", transferRequestID)
	}

	return nil
}

// MarkCompleted marks transfer as completed
func (r *PostgresPayoutTransferRepository) MarkCompleted(ctx context.Context, transferRequestID string, utr string, completedAt time.Time) error {
	query := `
		UPDATE xcentro.payout_transfer_requests
		SET status = $2, utr = $3, completed_at = $4, last_webhook_at = $4, updated_at = NOW()
		WHERE transfer_request_id = $1
	`

	result, err := r.querier.Exec(ctx, query, transferRequestID, payout.TransferStatusCompleted, utr, completedAt)
	if err != nil {
		return fmt.Errorf("failed to mark transfer completed: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("transfer request not found: %s", transferRequestID)
	}

	return nil
}

// MarkFailed marks transfer as failed
func (r *PostgresPayoutTransferRepository) MarkFailed(ctx context.Context, transferRequestID string, failureReason string, failedAt time.Time) error {
	query := `
		UPDATE xcentro.payout_transfer_requests
		SET status = $2, failure_reason = $3, failed_at = $4, last_webhook_at = $4, updated_at = NOW()
		WHERE transfer_request_id = $1
	`

	result, err := r.querier.Exec(ctx, query, transferRequestID, payout.TransferStatusFailed, failureReason, failedAt)
	if err != nil {
		return fmt.Errorf("failed to mark transfer failed: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("transfer request not found: %s", transferRequestID)
	}

	return nil
}

// SetWebhookTimeout sets the webhook timeout timestamp
func (r *PostgresPayoutTransferRepository) SetWebhookTimeout(ctx context.Context, transferRequestID string, timeoutAt time.Time) error {
	query := `
		UPDATE xcentro.payout_transfer_requests
		SET webhook_timeout_at = $2, updated_at = NOW()
		WHERE transfer_request_id = $1
	`

	result, err := r.querier.Exec(ctx, query, transferRequestID, timeoutAt)
	if err != nil {
		return fmt.Errorf("failed to set webhook timeout: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("transfer request not found: %s", transferRequestID)
	}

	return nil
}

// AppendWebhookHistory appends a webhook payload to history
func (r *PostgresPayoutTransferRepository) AppendWebhookHistory(ctx context.Context, transferRequestID string, payload map[string]interface{}) error {
	query := `
		UPDATE xcentro.payout_transfer_requests
		SET webhook_history = webhook_history || $2::jsonb,
		    last_webhook_at = NOW(),
		    updated_at = NOW()
		WHERE transfer_request_id = $1
	`

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	result, err := r.querier.Exec(ctx, query, transferRequestID, payloadJSON)
	if err != nil {
		return fmt.Errorf("failed to append webhook history: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("transfer request not found: %s", transferRequestID)
	}

	return nil
}

// GetWebhookTimeouts retrieves transfers that exceeded webhook timeout
func (r *PostgresPayoutTransferRepository) GetWebhookTimeouts(ctx context.Context, limit int) ([]*payout.PayoutTransferRequest, error) {
	query := `
		SELECT
			transfer_request_id, voucher_id, provider, provider_beneficiary_id, provider_transfer_id,
			amount, currency, purpose, reference, narration, transfer_mode,
			requires_approval, approval_mode, approved_by, approved_at,
			status, initiated_at, processing_at, completed_at, failed_at, failure_reason, utr,
			retry_count, max_retries, next_retry_at, last_retry_at,
			last_webhook_at, webhook_timeout_at, webhook_history, provider_metadata,
			created_at, updated_at
		FROM xcentro.payout_transfer_requests
		WHERE status IN ('initiated', 'processing')
			AND webhook_timeout_at IS NOT NULL
			AND webhook_timeout_at < NOW()
		ORDER BY webhook_timeout_at ASC
		LIMIT $1
	`

	rows, err := r.querier.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhook timeouts: %w", err)
	}
	defer rows.Close()

	var transfers []*payout.PayoutTransferRequest

	for rows.Next() {
		var transfer payout.PayoutTransferRequest
		var metadataJSON, historyJSON []byte

		err := rows.Scan(
			&transfer.TransferRequestID, &transfer.VoucherID, &transfer.Provider, &transfer.ProviderBeneficiaryID, &transfer.ProviderTransferID,
			&transfer.Amount, &transfer.Currency, &transfer.Purpose, &transfer.Reference, &transfer.Narration, &transfer.TransferMode,
			&transfer.RequiresApproval, &transfer.ApprovalMode, &transfer.ApprovedBy, &transfer.ApprovedAt,
			&transfer.Status, &transfer.InitiatedAt, &transfer.ProcessingAt, &transfer.CompletedAt, &transfer.FailedAt, &transfer.FailureReason, &transfer.UTR,
			&transfer.RetryCount, &transfer.MaxRetries, &transfer.NextRetryAt, &transfer.LastRetryAt,
			&transfer.LastWebhookAt, &transfer.WebhookTimeoutAt, &historyJSON, &metadataJSON,
			&transfer.CreatedAt, &transfer.UpdatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan transfer: %w", err)
		}

		// Unmarshal JSONB fields
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &transfer.ProviderMetadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal provider_metadata: %w", err)
			}
		}

		if len(historyJSON) > 0 {
			if err := json.Unmarshal(historyJSON, &transfer.WebhookHistory); err != nil {
				return nil, fmt.Errorf("failed to unmarshal webhook_history: %w", err)
			}
		}

		transfers = append(transfers, &transfer)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating transfer rows: %w", err)
	}

	return transfers, nil
}

// GetPendingApprovals retrieves transfers pending approval (for operations dashboard)
func (r *PostgresPayoutTransferRepository) GetPendingApprovals(ctx context.Context, limit int) ([]*payout.PayoutTransferRequest, error) {
	query := `
		SELECT
			transfer_request_id, voucher_id, provider, provider_beneficiary_id, provider_transfer_id,
			amount, currency, purpose, reference, narration, transfer_mode,
			requires_approval, approval_mode, approved_by, approved_at,
			status, initiated_at, processing_at, completed_at, failed_at, failure_reason, utr,
			retry_count, max_retries, next_retry_at, last_retry_at,
			last_webhook_at, webhook_timeout_at, webhook_history, provider_metadata,
			created_at, updated_at
		FROM xcentro.payout_transfer_requests
		WHERE status = 'pending_approval'
		ORDER BY created_at ASC
		LIMIT $1
	`

	rows, err := r.querier.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending approvals: %w", err)
	}
	defer rows.Close()

	var transfers []*payout.PayoutTransferRequest

	for rows.Next() {
		var transfer payout.PayoutTransferRequest
		var metadataJSON, historyJSON []byte

		err := rows.Scan(
			&transfer.TransferRequestID, &transfer.VoucherID, &transfer.Provider, &transfer.ProviderBeneficiaryID, &transfer.ProviderTransferID,
			&transfer.Amount, &transfer.Currency, &transfer.Purpose, &transfer.Reference, &transfer.Narration, &transfer.TransferMode,
			&transfer.RequiresApproval, &transfer.ApprovalMode, &transfer.ApprovedBy, &transfer.ApprovedAt,
			&transfer.Status, &transfer.InitiatedAt, &transfer.ProcessingAt, &transfer.CompletedAt, &transfer.FailedAt, &transfer.FailureReason, &transfer.UTR,
			&transfer.RetryCount, &transfer.MaxRetries, &transfer.NextRetryAt, &transfer.LastRetryAt,
			&transfer.LastWebhookAt, &transfer.WebhookTimeoutAt, &historyJSON, &metadataJSON,
			&transfer.CreatedAt, &transfer.UpdatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan transfer: %w", err)
		}

		// Unmarshal JSONB fields
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &transfer.ProviderMetadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal provider_metadata: %w", err)
			}
		}

		if len(historyJSON) > 0 {
			if err := json.Unmarshal(historyJSON, &transfer.WebhookHistory); err != nil {
				return nil, fmt.Errorf("failed to unmarshal webhook_history: %w", err)
			}
		}

		transfers = append(transfers, &transfer)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating transfer rows: %w", err)
	}

	return transfers, nil
}
