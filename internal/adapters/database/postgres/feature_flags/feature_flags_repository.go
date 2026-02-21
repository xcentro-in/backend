package feature_flags

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	redisClient "xcentro.in/internal/adapters/cache/redis/client"
	"xcentro.in/internal/adapters/cache/redis/keys"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/feature_flags"
	"xcentro.in/internal/shared/logger"
)

// PgFeatureFlagsRepository implements FeatureFlagsRepository using PostgreSQL with Redis caching
type PgFeatureFlagsRepository struct {
	querier     client.Querier
	redisClient *redisClient.RedisClient
	logger      *logger.Logger
}

// NewPgFeatureFlagsRepository creates a new PostgreSQL feature flags repository with Redis caching
func NewPgFeatureFlagsRepository(db client.DB, rclient *redisClient.RedisClient) feature_flags.FeatureFlagsRepository {
	return &PgFeatureFlagsRepository{
		querier:     db,
		redisClient: rclient,
		logger:      logger.GetDefault(),
	}
}

// GetAll retrieves all global feature flags (with Redis caching)
func (r *PgFeatureFlagsRepository) GetAll(ctx context.Context) (feature_flags.FeatureFlags, error) {
	// Try cache first
	cacheKey := keys.FeatureFlags.Global()
	var cachedFlags feature_flags.FeatureFlags
	err := r.redisClient.GetJSON(ctx, cacheKey, &cachedFlags)

	if err == nil && cachedFlags != nil {
		// Cache hit
		r.logger.DebugWith("Feature flags cache hit")
		return cachedFlags, nil
	}

	// Cache miss or error - fetch from database
	if !errors.Is(err, redis.Nil) && err != nil {
		r.logger.WarnWith("Redis cache error for feature flags (continuing with DB)", "error", err)
	}

	r.logger.DebugWith("Feature flags cache miss, fetching from database")

	// Fetch from database
	query := `
		SELECT flag_name, flag_value
		FROM xcentro.feature_flags
	`

	rows, err := r.querier.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query feature flags: %w", err)
	}
	defer rows.Close()

	flags := make(feature_flags.FeatureFlags)
	for rows.Next() {
		var flagName string
		var flagValueJSON []byte

		if err := rows.Scan(&flagName, &flagValueJSON); err != nil {
			return nil, fmt.Errorf("failed to scan feature flag: %w", err)
		}

		// Parse JSONB value
		var flagValue interface{}
		if err := json.Unmarshal(flagValueJSON, &flagValue); err != nil {
			return nil, fmt.Errorf("failed to unmarshal flag value for %s: %w", flagName, err)
		}

		flags[flagName] = flagValue
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating feature flags: %w", err)
	}

	// Store in cache asynchronously (fire-and-forget)
	if len(flags) > 0 {
		go func() {
			bgCtx := context.Background()
			if err := r.redisClient.SetJSON(bgCtx, cacheKey, flags, keys.WithJitter(keys.TTLFeatureFlags)); err != nil {
				r.logger.WarnWith("Failed to cache feature flags", "error", err)
			}
		}()
	}

	return flags, nil
}

// GetByName retrieves a specific feature flag by name
func (r *PgFeatureFlagsRepository) GetByName(ctx context.Context, flagName string) (*feature_flags.FeatureFlag, error) {
	query := `
		SELECT flag_id, flag_name, flag_value, description, created_at, updated_at
		FROM xcentro.feature_flags
		WHERE flag_name = $1
	`

	var flag feature_flags.FeatureFlag
	var flagValueJSON []byte

	err := r.querier.QueryRow(ctx, query, flagName).Scan(
		&flag.FlagID,
		&flag.FlagName,
		&flagValueJSON,
		&flag.Description,
		&flag.CreatedAt,
		&flag.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // Flag not found
		}
		return nil, fmt.Errorf("failed to query feature flag: %w", err)
	}

	// Parse JSONB value
	if err := json.Unmarshal(flagValueJSON, &flag.FlagValue); err != nil {
		return nil, fmt.Errorf("failed to unmarshal flag value: %w", err)
	}

	return &flag, nil
}

// UpsertFlag creates or updates a feature flag and invalidates cache
func (r *PgFeatureFlagsRepository) UpsertFlag(ctx context.Context, flag *feature_flags.FeatureFlag) error {
	// Marshal flag value to JSONB
	flagValueJSON, err := json.Marshal(flag.FlagValue)
	if err != nil {
		return fmt.Errorf("failed to marshal flag value: %w", err)
	}

	query := `
		INSERT INTO xcentro.feature_flags (flag_name, flag_value, description)
		VALUES ($1, $2, $3)
		ON CONFLICT (flag_name)
		DO UPDATE SET
			flag_value = EXCLUDED.flag_value,
			description = EXCLUDED.description,
			updated_at = NOW()
		RETURNING flag_id, created_at, updated_at
	`

	err = r.querier.QueryRow(ctx, query,
		flag.FlagName,
		flagValueJSON,
		flag.Description,
	).Scan(&flag.FlagID, &flag.CreatedAt, &flag.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to upsert feature flag: %w", err)
	}

	// Invalidate cache
	cacheKey := keys.FeatureFlags.Global()
	if err := r.redisClient.Delete(ctx, cacheKey); err != nil {
		r.logger.WarnWith("Failed to invalidate feature flags cache", "error", err)
	}

	return nil
}

// DeleteFlag deletes a feature flag and invalidates cache
func (r *PgFeatureFlagsRepository) DeleteFlag(ctx context.Context, flagName string) error {
	query := `
		DELETE FROM xcentro.feature_flags
		WHERE flag_name = $1
	`

	result, err := r.querier.Exec(ctx, query, flagName)
	if err != nil {
		return fmt.Errorf("failed to delete feature flag: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("feature flag not found")
	}

	// Invalidate cache
	cacheKey := keys.FeatureFlags.Global()
	if err := r.redisClient.Delete(ctx, cacheKey); err != nil {
		r.logger.WarnWith("Failed to invalidate feature flags cache", "error", err)
	}

	return nil
}
