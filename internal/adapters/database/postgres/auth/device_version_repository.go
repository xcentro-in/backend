package auth

import (
	"context"
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
