package payout

import (
	"context"
	"database/sql"
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
		transfer.TransferRequestID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		INSERT INTO xcentro.payout_transfer_requests (
			transfer_request_id, payment_id, voucher_id, provider, provider_beneficiary_id, escrow_id,
			amount, currency, purpose, reference, narration, transfer_mode,
			status, retry_count, max_retries,
			provider_metadata, is_deleted, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12,
			$13, $14, $15,
			$16, $17, $18
		)
	`

	// Marshal provider metadata
	metadataJSON, err := json.Marshal(transfer.ProviderMetadata)
	if err != nil {
		return fmt.Errorf("failed to marshal provider_metadata: %w", err)
	}

	_, err = r.querier.Exec(ctx, query,
		transfer.TransferRequestID, transfer.PaymentID, transfer.VoucherID, transfer.Provider, transfer.ProviderBeneficiaryID, transfer.EscrowID,
		transfer.Amount, transfer.Currency, transfer.Purpose, transfer.Reference, transfer.Narration, transfer.TransferMode,
		transfer.Status, transfer.RetryCount, transfer.MaxRetries,
		metadataJSON, transfer.IsDeleted, time.Now(),
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
			transfer_request_id, payment_id, voucher_id, provider, provider_beneficiary_id, provider_transfer_id, escrow_id,
			amount, currency, purpose, reference, narration, transfer_mode,
			status, initiated_at, processing_at, completed_at, failed_at, failure_reason, utr,
			retry_count, max_retries, next_retry_at, last_retry_at,
			last_webhook_at, webhook_timeout_at, webhook_history, provider_metadata,
			created_at, updated_at
		FROM xcentro.payout_transfer_requests
		WHERE transfer_request_id = $1
	`

	var transfer payout.PayoutTransferRequest
	var metadataJSON, historyJSON []byte
	var escrowID sql.NullString

	err := r.querier.QueryRow(ctx, query, transferRequestID).Scan(
		&transfer.TransferRequestID, &transfer.PaymentID, &transfer.VoucherID, &transfer.Provider, &transfer.ProviderBeneficiaryID, &transfer.ProviderTransferID, &escrowID,
		&transfer.Amount, &transfer.Currency, &transfer.Purpose, &transfer.Reference, &transfer.Narration, &transfer.TransferMode,
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

	// Handle nullable escrow_id
	if escrowID.Valid {
		transfer.EscrowID = escrowID.String
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

// GetByVoucherID retrieves the latest active (non-deleted) transfer request for a voucher
func (r *PostgresPayoutTransferRepository) GetByVoucherID(ctx context.Context, voucherID string) (*payout.PayoutTransferRequest, error) {
	query := `
		SELECT
			transfer_request_id, payment_id, voucher_id, provider, provider_beneficiary_id, provider_transfer_id, escrow_id,
			amount, currency, purpose, reference, narration, transfer_mode,
			status, initiated_at, processing_at, completed_at, failed_at, failure_reason, utr,
			retry_count, max_retries, next_retry_at, last_retry_at,
			last_webhook_at, webhook_timeout_at, webhook_history, provider_metadata,
			is_deleted, deleted_at, deleted_reason,
			created_at, updated_at
		FROM xcentro.payout_transfer_requests
		WHERE voucher_id = $1 AND is_deleted = false
		ORDER BY created_at DESC
		LIMIT 1
	`

	var transfer payout.PayoutTransferRequest
	var metadataJSON, historyJSON []byte
	var escrowID, deletedReason sql.NullString
	var deletedAt sql.NullTime

	err := r.querier.QueryRow(ctx, query, voucherID).Scan(
		&transfer.TransferRequestID, &transfer.PaymentID, &transfer.VoucherID, &transfer.Provider, &transfer.ProviderBeneficiaryID, &transfer.ProviderTransferID, &escrowID,
		&transfer.Amount, &transfer.Currency, &transfer.Purpose, &transfer.Reference, &transfer.Narration, &transfer.TransferMode,
		&transfer.Status, &transfer.InitiatedAt, &transfer.ProcessingAt, &transfer.CompletedAt, &transfer.FailedAt, &transfer.FailureReason, &transfer.UTR,
		&transfer.RetryCount, &transfer.MaxRetries, &transfer.NextRetryAt, &transfer.LastRetryAt,
		&transfer.LastWebhookAt, &transfer.WebhookTimeoutAt, &historyJSON, &metadataJSON,
		&transfer.IsDeleted, &deletedAt, &deletedReason,
		&transfer.CreatedAt, &transfer.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("transfer request not found for voucher: %s", voucherID)
		}
		return nil, fmt.Errorf("failed to get transfer request: %w", err)
	}

	// Handle nullable fields
	if escrowID.Valid {
		transfer.EscrowID = escrowID.String
	}
	if deletedAt.Valid {
		transfer.DeletedAt = &deletedAt.Time
	}
	if deletedReason.Valid {
		transfer.DeletedReason = deletedReason.String
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
			transfer_request_id, payment_id, voucher_id, provider, provider_beneficiary_id, provider_transfer_id, escrow_id,
			amount, currency, purpose, reference, narration, transfer_mode,
			status, initiated_at, processing_at, completed_at, failed_at, failure_reason, utr,
			retry_count, max_retries, next_retry_at, last_retry_at,
			last_webhook_at, webhook_timeout_at, webhook_history, provider_metadata,
			created_at, updated_at
		FROM xcentro.payout_transfer_requests
		WHERE provider_transfer_id = $1
	`

	var transfer payout.PayoutTransferRequest
	var metadataJSON, historyJSON []byte
	var escrowID, purpose, reference, narration, transferMode, failureReason, utr sql.NullString
	var processingAt, completedAt, failedAt, nextRetryAt, lastRetryAt, lastWebhookAt, webhookTimeoutAt sql.NullTime

	err := r.querier.QueryRow(ctx, query, providerTransferID).Scan(
		&transfer.TransferRequestID, &transfer.PaymentID, &transfer.VoucherID, &transfer.Provider, &transfer.ProviderBeneficiaryID, &transfer.ProviderTransferID, &escrowID,
		&transfer.Amount, &transfer.Currency, &purpose, &reference, &narration, &transferMode,
		&transfer.Status, &transfer.InitiatedAt, &processingAt, &completedAt, &failedAt, &failureReason, &utr,
		&transfer.RetryCount, &transfer.MaxRetries, &nextRetryAt, &lastRetryAt,
		&lastWebhookAt, &webhookTimeoutAt, &historyJSON, &metadataJSON,
		&transfer.CreatedAt, &transfer.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("transfer request not found for provider_transfer_id: %s", providerTransferID)
		}
		return nil, fmt.Errorf("failed to get transfer request: %w", err)
	}

	// Handle nullable string fields
	if escrowID.Valid {
		transfer.EscrowID = escrowID.String
	}
	if purpose.Valid {
		transfer.Purpose = purpose.String
	}
	if reference.Valid {
		transfer.Reference = reference.String
	}
	if narration.Valid {
		transfer.Narration = narration.String
	}
	if transferMode.Valid {
		transfer.TransferMode = transferMode.String
	}
	if failureReason.Valid {
		transfer.FailureReason = failureReason.String
	}
	if utr.Valid {
		transfer.UTR = utr.String
	}

	// Handle nullable timestamp fields
	if processingAt.Valid {
		transfer.ProcessingAt = &processingAt.Time
	}
	if completedAt.Valid {
		transfer.CompletedAt = &completedAt.Time
	}
	if failedAt.Valid {
		transfer.FailedAt = &failedAt.Time
	}
	if nextRetryAt.Valid {
		transfer.NextRetryAt = &nextRetryAt.Time
	}
	if lastRetryAt.Valid {
		transfer.LastRetryAt = &lastRetryAt.Time
	}
	if lastWebhookAt.Valid {
		transfer.LastWebhookAt = &lastWebhookAt.Time
	}
	if webhookTimeoutAt.Valid {
		transfer.WebhookTimeoutAt = &webhookTimeoutAt.Time
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
			transfer_request_id, payment_id, voucher_id, provider, provider_beneficiary_id, provider_transfer_id, escrow_id,
			amount, currency, purpose, reference, narration, transfer_mode,
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
		var escrowID, providerTransferID sql.NullString

		err := rows.Scan(
			&transfer.TransferRequestID, &transfer.PaymentID, &transfer.VoucherID, &transfer.Provider, &transfer.ProviderBeneficiaryID, &providerTransferID, &escrowID,
			&transfer.Amount, &transfer.Currency, &transfer.Purpose, &transfer.Reference, &transfer.Narration, &transfer.TransferMode,
			&transfer.Status, &transfer.InitiatedAt, &transfer.ProcessingAt, &transfer.CompletedAt, &transfer.FailedAt, &transfer.FailureReason, &transfer.UTR,
			&transfer.RetryCount, &transfer.MaxRetries, &transfer.NextRetryAt, &transfer.LastRetryAt,
			&transfer.LastWebhookAt, &transfer.WebhookTimeoutAt, &historyJSON, &metadataJSON,
			&transfer.CreatedAt, &transfer.UpdatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan transfer: %w", err)
		}

		// Handle nullable fields
		if escrowID.Valid {
			transfer.EscrowID = escrowID.String
		}
		if providerTransferID.Valid {
			transfer.ProviderTransferID = providerTransferID.String
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
	case payout.TransferStatusPending, payout.TransferStatusProcessing:
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
			transfer_request_id, payment_id, voucher_id, provider, provider_beneficiary_id, provider_transfer_id, escrow_id,
			amount, currency, purpose, reference, narration, transfer_mode,
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
		var escrowID sql.NullString

		err := rows.Scan(
			&transfer.TransferRequestID, &transfer.PaymentID, &transfer.VoucherID, &transfer.Provider, &transfer.ProviderBeneficiaryID, &transfer.ProviderTransferID, &escrowID,
			&transfer.Amount, &transfer.Currency, &transfer.Purpose, &transfer.Reference, &transfer.Narration, &transfer.TransferMode,
			&transfer.Status, &transfer.InitiatedAt, &transfer.ProcessingAt, &transfer.CompletedAt, &transfer.FailedAt, &transfer.FailureReason, &transfer.UTR,
			&transfer.RetryCount, &transfer.MaxRetries, &transfer.NextRetryAt, &transfer.LastRetryAt,
			&transfer.LastWebhookAt, &transfer.WebhookTimeoutAt, &historyJSON, &metadataJSON,
			&transfer.CreatedAt, &transfer.UpdatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan transfer: %w", err)
		}

		// Handle nullable escrow_id
		if escrowID.Valid {
			transfer.EscrowID = escrowID.String
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

// GetTransferHistoryByVoucherID retrieves all transfer attempts (including soft deleted) for a voucher in chronological order
func (r *PostgresPayoutTransferRepository) GetTransferHistoryByVoucherID(ctx context.Context, voucherID string) ([]*payout.PayoutTransferRequest, error) {
	query := `
		SELECT
			transfer_request_id, payment_id, voucher_id, status, created_at,
			failed_at, failure_reason, is_deleted, deleted_at, deleted_reason,
			provider_transfer_id, retry_count
		FROM xcentro.payout_transfer_requests
		WHERE voucher_id = $1
		ORDER BY created_at ASC
	`

	rows, err := r.querier.Query(ctx, query, voucherID)
	if err != nil {
		return nil, fmt.Errorf("failed to get transfer history: %w", err)
	}
	defer rows.Close()

	var transfers []*payout.PayoutTransferRequest

	for rows.Next() {
		var transfer payout.PayoutTransferRequest
		var providerTransferID, failureReason, deletedReason sql.NullString
		var failedAt, deletedAt sql.NullTime

		err := rows.Scan(
			&transfer.TransferRequestID, &transfer.PaymentID, &transfer.VoucherID,
			&transfer.Status, &transfer.CreatedAt, &failedAt, &failureReason,
			&transfer.IsDeleted, &deletedAt, &deletedReason, &providerTransferID, &transfer.RetryCount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan transfer history: %w", err)
		}

		// Handle nullable fields
		if failedAt.Valid {
			transfer.FailedAt = &failedAt.Time
		}
		if failureReason.Valid {
			transfer.FailureReason = failureReason.String
		}
		if deletedAt.Valid {
			transfer.DeletedAt = &deletedAt.Time
		}
		if deletedReason.Valid {
			transfer.DeletedReason = deletedReason.String
		}
		if providerTransferID.Valid {
			transfer.ProviderTransferID = providerTransferID.String
		}

		transfers = append(transfers, &transfer)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating transfer history: %w", err)
	}

	return transfers, nil
}

// SoftDelete marks a transfer as deleted (for retry mechanism)
func (r *PostgresPayoutTransferRepository) SoftDelete(ctx context.Context, transferRequestID string, reason string) error {
	query := `
		UPDATE xcentro.payout_transfer_requests
		SET is_deleted = true,
		    deleted_at = NOW(),
		    deleted_reason = $2,
		    updated_at = NOW()
		WHERE transfer_request_id = $1
	`

	result, err := r.querier.Exec(ctx, query, transferRequestID, reason)
	if err != nil {
		return fmt.Errorf("failed to soft delete transfer: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("transfer request not found: %s", transferRequestID)
	}

	return nil
}

// GetRetryCount counts total attempts (including soft deleted) for a voucher
func (r *PostgresPayoutTransferRepository) GetRetryCount(ctx context.Context, voucherID string) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM xcentro.payout_transfer_requests
		WHERE voucher_id = $1
	`

	var count int
	err := r.querier.QueryRow(ctx, query, voucherID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get retry count: %w", err)
	}

	return count, nil
}
