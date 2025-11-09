package feature_flags

import (
	"context"
)

// FeatureFlagsRepository defines the interface for global feature flags data access
type FeatureFlagsRepository interface {
	// GetAll retrieves all feature flags
	// Returns a map of flag_name -> flag_value
	GetAll(ctx context.Context) (FeatureFlags, error)

	// GetByName retrieves a specific feature flag by name
	GetByName(ctx context.Context, flagName string) (*FeatureFlag, error)

	// UpsertFlag creates or updates a feature flag
	UpsertFlag(ctx context.Context, flag *FeatureFlag) error

	// DeleteFlag deletes a feature flag
	DeleteFlag(ctx context.Context, flagName string) error
}
