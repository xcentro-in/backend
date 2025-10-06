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

// PostgresPayoutProviderMappingRepository implements PayoutProviderMappingRepository using PostgreSQL
type PostgresPayoutProviderMappingRepository struct {
	querier client.Querier
}

// NewPostgresPayoutProviderMappingRepository creates a new PostgreSQL payout provider mapping repository
func NewPostgresPayoutProviderMappingRepository(db client.DB) payout.PayoutProviderMappingRepository {
	return &PostgresPayoutProviderMappingRepository{
		querier: db,
	}
}

// Create creates a new provider mapping
func (r *PostgresPayoutProviderMappingRepository) Create(ctx context.Context, mapping *payout.PayoutProviderMapping) error {
	// Generate UUID if not provided
	if mapping.MappingID == "" {
		mapping.MappingID = uuid.New().String()
	}

	query := `
		INSERT INTO xcentro.payee_provider_mappings (
			mapping_id, payee_id, account_id,
			provider, provider_beneficiary_id, provider_account_id,
			onboarding_status, onboarded_at,
			onboarding_error_code, onboarding_error_message,
			provider_metadata, created_at, created_by
		) VALUES (
			$1, $2, $3,
			$4, $5, $6,
			$7, $8,
			$9, $10,
			$11, $12, $13
		)
	`

	// Marshal provider metadata
	var metadataJSON []byte
	var err error
	if mapping.ProviderMetadata != nil {
		metadataJSON, err = json.Marshal(mapping.ProviderMetadata)
		if err != nil {
			return fmt.Errorf("failed to marshal provider_metadata: %w", err)
		}
	}

	_, err = r.querier.Exec(ctx, query,
		mapping.MappingID, mapping.PayeeID, mapping.AccountID,
		mapping.Provider, mapping.ProviderBeneficiaryID, mapping.ProviderAccountID,
		mapping.OnboardingStatus, mapping.OnboardedAt,
		mapping.OnboardingErrorCode, mapping.OnboardingErrorMessage,
		metadataJSON, time.Now(), mapping.CreatedBy,
	)

	if err != nil {
		return fmt.Errorf("failed to create provider mapping: %w", err)
	}

	return nil
}

// GetByPayeeAndProvider retrieves mapping for a specific payee+account+provider
func (r *PostgresPayoutProviderMappingRepository) GetByPayeeAndProvider(ctx context.Context, payeeID string, accountID string, provider payout.ProviderType) (*payout.PayoutProviderMapping, error) {
	query := `
		SELECT
			mapping_id, payee_id, account_id,
			provider, provider_beneficiary_id, provider_account_id,
			onboarding_status, onboarded_at,
			onboarding_error_code, onboarding_error_message,
			provider_metadata,
			created_at, created_by, updated_at, last_synced_at
		FROM xcentro.payee_provider_mappings
		WHERE payee_id = $1 AND account_id = $2 AND provider = $3
	`

	var mapping payout.PayoutProviderMapping
	var metadataJSON []byte

	err := r.querier.QueryRow(ctx, query, payeeID, accountID, provider).Scan(
		&mapping.MappingID, &mapping.PayeeID, &mapping.AccountID,
		&mapping.Provider, &mapping.ProviderBeneficiaryID, &mapping.ProviderAccountID,
		&mapping.OnboardingStatus, &mapping.OnboardedAt,
		&mapping.OnboardingErrorCode, &mapping.OnboardingErrorMessage,
		&metadataJSON,
		&mapping.CreatedAt, &mapping.CreatedBy, &mapping.UpdatedAt, &mapping.LastSyncedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("provider mapping not found: payee=%s, account=%s, provider=%s", payeeID, accountID, provider)
		}
		return nil, fmt.Errorf("failed to get provider mapping: %w", err)
	}

	// Unmarshal metadata
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &mapping.ProviderMetadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal provider_metadata: %w", err)
		}
	}

	return &mapping, nil
}

// GetByPayeeAndAccount retrieves all provider mappings for a payee+account
func (r *PostgresPayoutProviderMappingRepository) GetByPayeeAndAccount(ctx context.Context, payeeID string, accountID string) ([]*payout.PayoutProviderMapping, error) {
	query := `
		SELECT
			mapping_id, payee_id, account_id,
			provider, provider_beneficiary_id, provider_account_id,
			onboarding_status, onboarded_at,
			onboarding_error_code, onboarding_error_message,
			provider_metadata,
			created_at, created_by, updated_at, last_synced_at
		FROM xcentro.payee_provider_mappings
		WHERE payee_id = $1 AND account_id = $2
		ORDER BY created_at ASC
	`

	rows, err := r.querier.Query(ctx, query, payeeID, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider mappings: %w", err)
	}
	defer rows.Close()

	var mappings []*payout.PayoutProviderMapping

	for rows.Next() {
		var mapping payout.PayoutProviderMapping
		var metadataJSON []byte

		err := rows.Scan(
			&mapping.MappingID, &mapping.PayeeID, &mapping.AccountID,
			&mapping.Provider, &mapping.ProviderBeneficiaryID, &mapping.ProviderAccountID,
			&mapping.OnboardingStatus, &mapping.OnboardedAt,
			&mapping.OnboardingErrorCode, &mapping.OnboardingErrorMessage,
			&metadataJSON,
			&mapping.CreatedAt, &mapping.CreatedBy, &mapping.UpdatedAt, &mapping.LastSyncedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan provider mapping: %w", err)
		}

		// Unmarshal metadata
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &mapping.ProviderMetadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal provider_metadata: %w", err)
			}
		}

		mappings = append(mappings, &mapping)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating provider mapping rows: %w", err)
	}

	return mappings, nil
}

// GetActiveMapping retrieves the active provider mapping for a payee+account
// Returns the first active mapping (typically Castler, fallback to Razorpay)
func (r *PostgresPayoutProviderMappingRepository) GetActiveMapping(ctx context.Context, payeeID string, accountID string) (*payout.PayoutProviderMapping, error) {
	query := `
		SELECT
			mapping_id, payee_id, account_id,
			provider, provider_beneficiary_id, provider_account_id,
			onboarding_status, onboarded_at,
			onboarding_error_code, onboarding_error_message,
			provider_metadata,
			created_at, created_by, updated_at, last_synced_at
		FROM xcentro.payee_provider_mappings
		WHERE payee_id = $1
			AND account_id = $2
			AND onboarding_status = 'active'
			AND provider_beneficiary_id IS NOT NULL
		ORDER BY created_at ASC
		LIMIT 1
	`

	var mapping payout.PayoutProviderMapping
	var metadataJSON []byte

	err := r.querier.QueryRow(ctx, query, payeeID, accountID).Scan(
		&mapping.MappingID, &mapping.PayeeID, &mapping.AccountID,
		&mapping.Provider, &mapping.ProviderBeneficiaryID, &mapping.ProviderAccountID,
		&mapping.OnboardingStatus, &mapping.OnboardedAt,
		&mapping.OnboardingErrorCode, &mapping.OnboardingErrorMessage,
		&metadataJSON,
		&mapping.CreatedAt, &mapping.CreatedBy, &mapping.UpdatedAt, &mapping.LastSyncedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("no active provider mapping found for payee=%s, account=%s", payeeID, accountID)
		}
		return nil, fmt.Errorf("failed to get active provider mapping: %w", err)
	}

	// Unmarshal metadata
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &mapping.ProviderMetadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal provider_metadata: %w", err)
		}
	}

	return &mapping, nil
}

// UpdateOnboardingStatus updates the onboarding status of a mapping
func (r *PostgresPayoutProviderMappingRepository) UpdateOnboardingStatus(ctx context.Context, mappingID string, status payout.OnboardingStatus, providerBeneficiaryID string, providerAccountID string, metadata map[string]interface{}) error {
	query := `
		UPDATE xcentro.payee_provider_mappings
		SET onboarding_status = $2,
			provider_beneficiary_id = $3,
			provider_account_id = $4,
			provider_metadata = $5,
			onboarded_at = CASE WHEN $2 = 'active' THEN NOW() ELSE onboarded_at END,
			updated_at = NOW()
		WHERE mapping_id = $1
	`

	// Marshal metadata
	var metadataJSON []byte
	var err error
	if metadata != nil {
		metadataJSON, err = json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal provider_metadata: %w", err)
		}
	}

	result, err := r.querier.Exec(ctx, query, mappingID, status, providerBeneficiaryID, providerAccountID, metadataJSON)
	if err != nil {
		return fmt.Errorf("failed to update onboarding status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("provider mapping not found: %s", mappingID)
	}

	return nil
}

// UpdateOnboardingError records an onboarding failure
func (r *PostgresPayoutProviderMappingRepository) UpdateOnboardingError(ctx context.Context, mappingID string, errorCode string, errorMessage string) error {
	query := `
		UPDATE xcentro.payee_provider_mappings
		SET onboarding_status = 'failed',
			onboarding_error_code = $2,
			onboarding_error_message = $3,
			updated_at = NOW()
		WHERE mapping_id = $1
	`

	result, err := r.querier.Exec(ctx, query, mappingID, errorCode, errorMessage)
	if err != nil {
		return fmt.Errorf("failed to update onboarding error: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("provider mapping not found: %s", mappingID)
	}

	return nil
}

// GetPendingOnboardings retrieves mappings that need onboarding (for worker)
func (r *PostgresPayoutProviderMappingRepository) GetPendingOnboardings(ctx context.Context, limit int) ([]*payout.PayoutProviderMapping, error) {
	query := `
		SELECT
			mapping_id, payee_id, account_id,
			provider, provider_beneficiary_id, provider_account_id,
			onboarding_status, onboarded_at,
			onboarding_error_code, onboarding_error_message,
			provider_metadata,
			created_at, created_by, updated_at, last_synced_at
		FROM xcentro.payee_provider_mappings
		WHERE onboarding_status = 'pending'
		ORDER BY created_at ASC
		LIMIT $1
	`

	rows, err := r.querier.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending onboardings: %w", err)
	}
	defer rows.Close()

	var mappings []*payout.PayoutProviderMapping

	for rows.Next() {
		var mapping payout.PayoutProviderMapping
		var metadataJSON []byte

		err := rows.Scan(
			&mapping.MappingID, &mapping.PayeeID, &mapping.AccountID,
			&mapping.Provider, &mapping.ProviderBeneficiaryID, &mapping.ProviderAccountID,
			&mapping.OnboardingStatus, &mapping.OnboardedAt,
			&mapping.OnboardingErrorCode, &mapping.OnboardingErrorMessage,
			&metadataJSON,
			&mapping.CreatedAt, &mapping.CreatedBy, &mapping.UpdatedAt, &mapping.LastSyncedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan provider mapping: %w", err)
		}

		// Unmarshal metadata
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &mapping.ProviderMetadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal provider_metadata: %w", err)
			}
		}

		mappings = append(mappings, &mapping)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating provider mapping rows: %w", err)
	}

	return mappings, nil
}

// GetFailedOnboardings retrieves failed onboardings for retry (for worker)
func (r *PostgresPayoutProviderMappingRepository) GetFailedOnboardings(ctx context.Context, limit int, olderThan time.Duration) ([]*payout.PayoutProviderMapping, error) {
	query := `
		SELECT
			mapping_id, payee_id, account_id,
			provider, provider_beneficiary_id, provider_account_id,
			onboarding_status, onboarded_at,
			onboarding_error_code, onboarding_error_message,
			provider_metadata,
			created_at, created_by, updated_at, last_synced_at
		FROM xcentro.payee_provider_mappings
		WHERE onboarding_status = 'failed'
			AND updated_at < NOW() - $2::interval
		ORDER BY updated_at ASC
		LIMIT $1
	`

	// Convert duration to PostgreSQL interval format (e.g., "1 hour")
	intervalStr := fmt.Sprintf("%d seconds", int(olderThan.Seconds()))

	rows, err := r.querier.Query(ctx, query, limit, intervalStr)
	if err != nil {
		return nil, fmt.Errorf("failed to get failed onboardings: %w", err)
	}
	defer rows.Close()

	var mappings []*payout.PayoutProviderMapping

	for rows.Next() {
		var mapping payout.PayoutProviderMapping
		var metadataJSON []byte

		err := rows.Scan(
			&mapping.MappingID, &mapping.PayeeID, &mapping.AccountID,
			&mapping.Provider, &mapping.ProviderBeneficiaryID, &mapping.ProviderAccountID,
			&mapping.OnboardingStatus, &mapping.OnboardedAt,
			&mapping.OnboardingErrorCode, &mapping.OnboardingErrorMessage,
			&metadataJSON,
			&mapping.CreatedAt, &mapping.CreatedBy, &mapping.UpdatedAt, &mapping.LastSyncedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan provider mapping: %w", err)
		}

		// Unmarshal metadata
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &mapping.ProviderMetadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal provider_metadata: %w", err)
			}
		}

		mappings = append(mappings, &mapping)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating provider mapping rows: %w", err)
	}

	return mappings, nil
}

// UpdateLastSyncedAt updates the last sync timestamp
func (r *PostgresPayoutProviderMappingRepository) UpdateLastSyncedAt(ctx context.Context, mappingID string, timestamp time.Time) error {
	query := `
		UPDATE xcentro.payee_provider_mappings
		SET last_synced_at = $2, updated_at = NOW()
		WHERE mapping_id = $1
	`

	result, err := r.querier.Exec(ctx, query, mappingID, timestamp)
	if err != nil {
		return fmt.Errorf("failed to update last_synced_at: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("provider mapping not found: %s", mappingID)
	}

	return nil
}
