package auth

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/auth"
)

// PgDeviceVersionRepository implements auth.DeviceVersionRepository using PostgreSQL
type PgDeviceVersionRepository struct {
	querier client.Querier
}

// NewPgDeviceVersionRepository creates a new PostgreSQL device version repository
func NewPgDeviceVersionRepository(db client.DB) *PgDeviceVersionRepository {
	return &PgDeviceVersionRepository{
		querier: db,
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgDeviceVersionRepository) WithTx(tx interface{}) auth.DeviceVersionRepository {
	return &PgDeviceVersionRepository{
		querier: tx.(client.Tx),
	}
}

// GetDeviceVersion retrieves the current version for a user's device type
func (r *PgDeviceVersionRepository) GetDeviceVersion(ctx context.Context, userID string, deviceType string) (int, error) {
	query := `
		SELECT current_version
		FROM xcentro.user_device_versions
		WHERE user_id = $1 AND device_type = $2
	`

	var version int
	err := r.querier.QueryRow(ctx, query, userID, deviceType).Scan(&version)
	if err != nil {
		if err == pgx.ErrNoRows {
			// No version exists yet - return 1 as default
			return 1, nil
		}
		return 0, fmt.Errorf("failed to get device version: %w", err)
	}

	return version, nil
}

// IncrementDeviceVersion increments the version for a user's device type and returns new version
func (r *PgDeviceVersionRepository) IncrementDeviceVersion(ctx context.Context, userID string, deviceType string) (int, error) {
	query := `
		INSERT INTO xcentro.user_device_versions (user_id, device_type, current_version)
		VALUES ($1, $2, 2)
		ON CONFLICT (user_id, device_type)
		DO UPDATE SET
			current_version = xcentro.user_device_versions.current_version + 1,
			updated_at = NOW()
		RETURNING current_version
	`

	var newVersion int
	err := r.querier.QueryRow(ctx, query, userID, deviceType).Scan(&newVersion)
	if err != nil {
		return 0, fmt.Errorf("failed to increment device version: %w", err)
	}

	return newVersion, nil
}

// GetDeviceVersionWithRoles retrieves device version and user roles in a single query
func (r *PgDeviceVersionRepository) GetDeviceVersionWithRoles(ctx context.Context, userID string, deviceType string) (int, interface{}, error) {
	query := `
		SELECT
			udv.current_version,
			COALESCE(
				json_agg(
					json_build_object(
						'userid', bur.user_id,
						'bid', bur.bid,
						'role', bur.role,
						'roleClaim', bur.role_claim,
						'addedAt', bur.added_at::text,
						'kybInitiated', bur.kyb_initiated,
						'kycNeeded', bur.kyc_needed,
						'timestamp', COALESCE(bur.updated_at::text, '')
					)
				) FILTER (WHERE bur.user_id IS NOT NULL),
				'[]'::json
			) as roles
		FROM xcentro.user_device_versions udv
		LEFT JOIN xcentro.business_user_relation bur ON bur.user_id = udv.user_id
		WHERE udv.user_id = $1 AND udv.device_type = $2
		GROUP BY udv.current_version
	`

	var version int
	var rolesJSON []byte
	err := r.querier.QueryRow(ctx, query, userID, deviceType).Scan(&version, &rolesJSON)
	if err != nil {
		if err == pgx.ErrNoRows {
			// No version exists yet - return default version 1 with empty roles
			return 1, []map[string]interface{}{}, nil
		}
		return 0, nil, fmt.Errorf("failed to get device version with roles: %w", err)
	}

	// Parse JSON into raw slice first
	var rolesRaw []map[string]interface{}
	if err := json.Unmarshal(rolesJSON, &rolesRaw); err != nil {
		return 0, nil, fmt.Errorf("failed to unmarshal roles: %w", err)
	}

	// Convert to proper business.BusinessUserRelation slice
	// This is needed because the query returns JSON which needs to be converted
	// to match the type returned by businessUserRelRepo.FindByUser()
	return version, rolesRaw, nil
}

// UpsertDeviceVersion creates or updates device version
func (r *PgDeviceVersionRepository) UpsertDeviceVersion(ctx context.Context, userID string, deviceType string, version int) error {
	query := `
		INSERT INTO xcentro.user_device_versions (user_id, device_type, current_version)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, device_type)
		DO UPDATE SET
			current_version = $3,
			updated_at = NOW()
	`

	_, err := r.querier.Exec(ctx, query, userID, deviceType, version)
	if err != nil {
		return fmt.Errorf("failed to upsert device version: %w", err)
	}

	return nil
}
