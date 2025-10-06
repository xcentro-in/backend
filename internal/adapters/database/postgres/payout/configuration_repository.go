package payout

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/payout"
)

// PostgresPayoutConfigurationRepository implements PayoutConfigurationRepository using PostgreSQL
type PostgresPayoutConfigurationRepository struct {
	querier client.Querier
}

// NewPostgresPayoutConfigurationRepository creates a new PostgreSQL payout configuration repository
func NewPostgresPayoutConfigurationRepository(db client.DB) payout.PayoutConfigurationRepository {
	return &PostgresPayoutConfigurationRepository{
		querier: db,
	}
}

// GetConfig retrieves configuration (business-specific or global default)
func (r *PostgresPayoutConfigurationRepository) GetConfig(ctx context.Context, businessID *string) (*payout.PayoutConfiguration, error) {
	// Try business-specific config first, then fall back to global
	var query string
	var args []interface{}

	if businessID != nil {
		query = `
			SELECT
				config_id, business_id,
				auto_approval_enabled, auto_approval_limit,
				preferred_transfer_mode,
				max_retry_attempts, retry_delay_minutes, retry_backoff_multiplier,
				webhook_timeout_minutes,
				batch_size, worker_enabled,
				created_at, updated_at
			FROM xcentro.payout_configuration
			WHERE business_id = $1 OR business_id IS NULL
			ORDER BY business_id NULLS LAST
			LIMIT 1
		`
		args = []interface{}{businessID}
	} else {
		query = `
			SELECT
				config_id, business_id,
				auto_approval_enabled, auto_approval_limit,
				preferred_transfer_mode,
				max_retry_attempts, retry_delay_minutes, retry_backoff_multiplier,
				webhook_timeout_minutes,
				batch_size, worker_enabled,
				created_at, updated_at
			FROM xcentro.payout_configuration
			WHERE business_id IS NULL
			LIMIT 1
		`
	}

	var config payout.PayoutConfiguration

	err := r.querier.QueryRow(ctx, query, args...).Scan(
		&config.ConfigID, &config.BusinessID,
		&config.AutoApprovalEnabled, &config.AutoApprovalLimit,
		&config.PreferredTransferMode,
		&config.MaxRetryAttempts, &config.RetryDelayMinutes, &config.RetryBackoffMultiplier,
		&config.WebhookTimeoutMinutes,
		&config.BatchSize, &config.WorkerEnabled,
		&config.CreatedAt, &config.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("configuration not found")
		}
		return nil, fmt.Errorf("failed to get configuration: %w", err)
	}

	return &config, nil
}

// UpdateConfig updates configuration
func (r *PostgresPayoutConfigurationRepository) UpdateConfig(ctx context.Context, config *payout.PayoutConfiguration) error {
	query := `
		UPDATE xcentro.payout_configuration
		SET auto_approval_enabled = $2,
			auto_approval_limit = $3,
			preferred_transfer_mode = $4,
			max_retry_attempts = $5,
			retry_delay_minutes = $6,
			retry_backoff_multiplier = $7,
			webhook_timeout_minutes = $8,
			batch_size = $9,
			worker_enabled = $10,
			updated_at = NOW()
		WHERE config_id = $1
	`

	result, err := r.querier.Exec(ctx, query,
		config.ConfigID,
		config.AutoApprovalEnabled,
		config.AutoApprovalLimit,
		config.PreferredTransferMode,
		config.MaxRetryAttempts,
		config.RetryDelayMinutes,
		config.RetryBackoffMultiplier,
		config.WebhookTimeoutMinutes,
		config.BatchSize,
		config.WorkerEnabled,
	)

	if err != nil {
		return fmt.Errorf("failed to update configuration: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("configuration not found: %s", config.ConfigID)
	}

	return nil
}

// Create creates a new business-specific configuration
func (r *PostgresPayoutConfigurationRepository) Create(ctx context.Context, config *payout.PayoutConfiguration) error {
	query := `
		INSERT INTO xcentro.payout_configuration (
			config_id, business_id,
			auto_approval_enabled, auto_approval_limit,
			preferred_transfer_mode,
			max_retry_attempts, retry_delay_minutes, retry_backoff_multiplier,
			webhook_timeout_minutes,
			batch_size, worker_enabled,
			created_at
		) VALUES (
			uuidv7(), $1,
			$2, $3,
			$4,
			$5, $6, $7,
			$8,
			$9, $10,
			NOW()
		)
		RETURNING config_id
	`

	err := r.querier.QueryRow(ctx, query,
		config.BusinessID,
		config.AutoApprovalEnabled,
		config.AutoApprovalLimit,
		config.PreferredTransferMode,
		config.MaxRetryAttempts,
		config.RetryDelayMinutes,
		config.RetryBackoffMultiplier,
		config.WebhookTimeoutMinutes,
		config.BatchSize,
		config.WorkerEnabled,
	).Scan(&config.ConfigID)

	if err != nil {
		return fmt.Errorf("failed to create configuration: %w", err)
	}

	return nil
}
