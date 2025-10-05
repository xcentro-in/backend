package payout

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/payout"
)

// PostgresProviderMappingRepository implements PayoutProviderMappingRepository using PostgreSQL
type PostgresProviderMappingRepository struct {
	querier client.Querier
}

// NewPostgresProviderMappingRepository creates a new PostgreSQL provider mapping repository
func NewPostgresProviderMappingRepository(db client.DB) payout.PayoutProviderMappingRepository {
	return &PostgresProviderMappingRepository{
		querier: db,
	}
}

// Create creates a new provider mapping
func (r *PostgresProviderMappingRepository) Create(ctx context.Context, mapping *payout.PayoutProviderMapping) error {
	query := `
		INSERT INTO xcentro.payee_provider_mappings (
			mapping_id, payee_id, account_id, provider,
			provider_beneficiary_id, provider_account_id,
			onboarding_status, onboarded_at,
			onboarding_error_code, onboarding_error_message,
			provider_metadata, created_at, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
		)
	`

	// Marshal provider metadata
	metadataJSON, err := json.Marshal(mapping.ProviderMetadata)
	if err != nil {
		return fmt.Errorf("failed to marshal provider_metadata: %w", err)
	}

	_, err = r.querier.Exec(ctx, query,
		mapping.MappingID, mapping.PayeeID, mapping.AccountID, mapping.Provider,
		mapping.ProviderBeneficiaryID, mapping.ProviderAccountID,
		mapping.OnboardingStatus, mapping.OnboardedAt,
		mapping.OnboardingErrorCode, mapping.OnboardingErrorMessage,
		metadataJSON, mapping.CreatedAt, mapping.CreatedBy,
	)

	if err != nil {
		// Check for unique constraint violation
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			return payout.ErrMappingAlreadyExists
		}
		return fmt.Errorf("failed to create provider mapping: %w", err)
	}

	return nil
}

// GetByPayeeAndProvider retrieves mapping for a specific payee+account+provider
func (r *PostgresProviderMappingRepository) GetByPayeeAndProvider(ctx context.Context, payeeID string, accountID string, provider payout.ProviderType) (*payout.PayoutProviderMapping, error) {
	query := `
		SELECT mapping_id, payee_id, account_id, provider,
		       provider_beneficiary_id, provider_account_id,
		       onboarding_status, onboarded_at,
		       onboarding_error_code, onboarding_error_message,
		       provider_metadata, created_at, created_by, updated_at, last_synced_at
		FROM xcentro.payee_provider_mappings
		WHERE payee_id = $1 AND account_id = $2 AND provider = $3
	`

	row := r.querier.QueryRow(ctx, query, payeeID, accountID, provider)
	mapping, err := r.scanMapping(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, payout.ErrMappingNotFound
		}
		return nil, fmt.Errorf("failed to get provider mapping: %w", err)
	}

	return mapping, nil
}

// GetByPayeeAndAccount retrieves all provider mappings for a payee+account
func (r *PostgresProviderMappingRepository) GetByPayeeAndAccount(ctx context.Context, payeeID string, accountID string) ([]*payout.PayoutProviderMapping, error) {
	query := `
		SELECT mapping_id, payee_id, account_id, provider,
		       provider_beneficiary_id, provider_account_id,
		       onboarding_status, onboarded_at,
		       onboarding_error_code, onboarding_error_message,
		       provider_metadata, created_at, created_by, updated_at, last_synced_at
		FROM xcentro.payee_provider_mappings
		WHERE payee_id = $1 AND account_id = $2
		ORDER BY
			CASE onboarding_status
				WHEN 'active' THEN 1
				WHEN 'pending' THEN 2
				ELSE 3
			END
	`

	rows, err := r.querier.Query(ctx, query, payeeID, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to query provider mappings: %w", err)
	}
	defer rows.Close()

	var mappings []*payout.PayoutProviderMapping
	for rows.Next() {
		mapping, err := r.scanMappingRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan mapping row: %w", err)
		}
		mappings = append(mappings, mapping)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating mapping rows: %w", err)
	}

	return mappings, nil
}

// GetActiveMapping retrieves the active provider mapping for a payee+account
func (r *PostgresProviderMappingRepository) GetActiveMapping(ctx context.Context, payeeID string, accountID string) (*payout.PayoutProviderMapping, error) {
	query := `
		SELECT mapping_id, payee_id, account_id, provider,
		       provider_beneficiary_id, provider_account_id,
		       onboarding_status, onboarded_at,
		       onboarding_error_code, onboarding_error_message,
		       provider_metadata, created_at, created_by, updated_at, last_synced_at
		FROM xcentro.payee_provider_mappings
		WHERE payee_id = $1 AND account_id = $2 AND onboarding_status = 'active'
		ORDER BY provider  -- Castler comes first alphabetically
		LIMIT 1
	`

	row := r.querier.QueryRow(ctx, query, payeeID, accountID)
	mapping, err := r.scanMapping(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, payout.ErrNoActiveMapping
		}
		return nil, fmt.Errorf("failed to get active provider mapping: %w", err)
	}

	return mapping, nil
}

// UpdateOnboardingStatus updates the onboarding status of a mapping
func (r *PostgresProviderMappingRepository) UpdateOnboardingStatus(ctx context.Context, mappingID string, status payout.OnboardingStatus, providerBeneficiaryID string, providerAccountID string, metadata map[string]interface{}) error {
	query := `
		UPDATE xcentro.payee_provider_mappings
		SET onboarding_status = $2,
		    provider_beneficiary_id = $3,
		    provider_account_id = $4,
		    onboarded_at = CASE WHEN $2 = 'active' THEN NOW() ELSE onboarded_at END,
		    provider_metadata = $5,
		    updated_at = NOW()
		WHERE mapping_id = $1
	`

	// Marshal metadata
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal provider_metadata: %w", err)
	}

	result, err := r.querier.Exec(ctx, query, mappingID, status, providerBeneficiaryID, providerAccountID, metadataJSON)
	if err != nil {
		return fmt.Errorf("failed to update onboarding status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return payout.ErrMappingNotFound
	}

	return nil
}

// UpdateOnboardingError records an onboarding failure
func (r *PostgresProviderMappingRepository) UpdateOnboardingError(ctx context.Context, mappingID string, errorCode string, errorMessage string) error {
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
		return payout.ErrMappingNotFound
	}

	return nil
}

// GetPendingOnboardings retrieves mappings that need onboarding
func (r *PostgresProviderMappingRepository) GetPendingOnboardings(ctx context.Context, limit int) ([]*payout.PayoutProviderMapping, error) {
	query := `
		SELECT mapping_id, payee_id, account_id, provider,
		       provider_beneficiary_id, provider_account_id,
		       onboarding_status, onboarded_at,
		       onboarding_error_code, onboarding_error_message,
		       provider_metadata, created_at, created_by, updated_at, last_synced_at
		FROM xcentro.payee_provider_mappings
		WHERE onboarding_status = 'not_onboarded'
		ORDER BY created_at ASC
		LIMIT $1
	`

	rows, err := r.querier.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending onboardings: %w", err)
	}
	defer rows.Close()

	var mappings []*payout.PayoutProviderMapping
	for rows.Next() {
		mapping, err := r.scanMappingRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan mapping row: %w", err)
		}
		mappings = append(mappings, mapping)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating mapping rows: %w", err)
	}

	return mappings, nil
}

// GetFailedOnboardings retrieves failed onboardings for retry
func (r *PostgresProviderMappingRepository) GetFailedOnboardings(ctx context.Context, limit int, olderThan time.Duration) ([]*payout.PayoutProviderMapping, error) {
	query := `
		SELECT mapping_id, payee_id, account_id, provider,
		       provider_beneficiary_id, provider_account_id,
		       onboarding_status, onboarded_at,
		       onboarding_error_code, onboarding_error_message,
		       provider_metadata, created_at, created_by, updated_at, last_synced_at
		FROM xcentro.payee_provider_mappings
		WHERE onboarding_status = 'failed'
		  AND updated_at < NOW() - $1::INTERVAL
		ORDER BY updated_at ASC
		LIMIT $2
	`

	rows, err := r.querier.Query(ctx, query, olderThan, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query failed onboardings: %w", err)
	}
	defer rows.Close()

	var mappings []*payout.PayoutProviderMapping
	for rows.Next() {
		mapping, err := r.scanMappingRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan mapping row: %w", err)
		}
		mappings = append(mappings, mapping)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating mapping rows: %w", err)
	}

	return mappings, nil
}

// UpdateLastSyncedAt updates the last sync timestamp
func (r *PostgresProviderMappingRepository) UpdateLastSyncedAt(ctx context.Context, mappingID string, timestamp time.Time) error {
	query := `
		UPDATE xcentro.payee_provider_mappings
		SET last_synced_at = $2,
		    updated_at = NOW()
		WHERE mapping_id = $1
	`

	result, err := r.querier.Exec(ctx, query, mappingID, timestamp)
	if err != nil {
		return fmt.Errorf("failed to update last_synced_at: %w", err)
	}

	if result.RowsAffected() == 0 {
		return payout.ErrMappingNotFound
	}

	return nil
}

// Helper methods

// scanMapping scans a single row into a PayoutProviderMapping struct
func (r *PostgresProviderMappingRepository) scanMapping(row pgx.Row) (*payout.PayoutProviderMapping, error) {
	var m payout.PayoutProviderMapping
	var providerMetadataJSON []byte
	var providerStr string

	err := row.Scan(
		&m.MappingID, &m.PayeeID, &m.AccountID, &providerStr,
		&m.ProviderBeneficiaryID, &m.ProviderAccountID,
		&m.OnboardingStatus, &m.OnboardedAt,
		&m.OnboardingErrorCode, &m.OnboardingErrorMessage,
		&providerMetadataJSON, &m.CreatedAt, &m.CreatedBy, &m.UpdatedAt, &m.LastSyncedAt,
	)
	if err != nil {
		return nil, err
	}

	// Convert provider string to ProviderType
	m.Provider = payout.ProviderType(providerStr)

	// Unmarshal provider metadata
	if len(providerMetadataJSON) > 0 {
		if err := json.Unmarshal(providerMetadataJSON, &m.ProviderMetadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal provider_metadata: %w", err)
		}
	}

	return &m, nil
}

// scanMappingRow scans rows (for Query) into a PayoutProviderMapping struct
func (r *PostgresProviderMappingRepository) scanMappingRow(rows pgx.Rows) (*payout.PayoutProviderMapping, error) {
	var m payout.PayoutProviderMapping
	var providerMetadataJSON []byte
	var providerStr string

	err := rows.Scan(
		&m.MappingID, &m.PayeeID, &m.AccountID, &providerStr,
		&m.ProviderBeneficiaryID, &m.ProviderAccountID,
		&m.OnboardingStatus, &m.OnboardedAt,
		&m.OnboardingErrorCode, &m.OnboardingErrorMessage,
		&providerMetadataJSON, &m.CreatedAt, &m.CreatedBy, &m.UpdatedAt, &m.LastSyncedAt,
	)
	if err != nil {
		return nil, err
	}

	// Convert provider string to ProviderType
	m.Provider = payout.ProviderType(providerStr)

	// Unmarshal provider metadata
	if len(providerMetadataJSON) > 0 {
		if err := json.Unmarshal(providerMetadataJSON, &m.ProviderMetadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal provider_metadata: %w", err)
		}
	}

	return &m, nil
}
