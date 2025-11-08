package feature_flags

import (
	"context"

	"xcentro.in/internal/core/feature_flags"
)

// FeatureFlagsService defines the interface for feature flags business logic
type FeatureFlagsService interface {
	// GetAllFeatureFlags retrieves all global feature flags
	GetAllFeatureFlags(ctx context.Context) (*feature_flags.GetFeatureFlagsResponse, error)

	// GetFeatureFlag retrieves a specific feature flag by name
	GetFeatureFlag(ctx context.Context, flagName string) (*feature_flags.FeatureFlag, error)

	// UpsertFeatureFlag creates or updates a feature flag
	UpsertFeatureFlag(ctx context.Context, flag *feature_flags.FeatureFlag) error

	// DeleteFeatureFlag deletes a feature flag
	DeleteFeatureFlag(ctx context.Context, flagName string) error
}

// FeatureFlagsServiceImpl implements FeatureFlagsService
type FeatureFlagsServiceImpl struct {
	repo feature_flags.FeatureFlagsRepository
}

// NewFeatureFlagsService creates a new feature flags service
func NewFeatureFlagsService(repo feature_flags.FeatureFlagsRepository) FeatureFlagsService {
	return &FeatureFlagsServiceImpl{
		repo: repo,
	}
}

// GetAllFeatureFlags retrieves all global feature flags
func (s *FeatureFlagsServiceImpl) GetAllFeatureFlags(ctx context.Context) (*feature_flags.GetFeatureFlagsResponse, error) {
	flags, err := s.repo.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	// If no flags found, return empty map (not an error)
	if flags == nil {
		flags = make(feature_flags.FeatureFlags)
	}

	return &feature_flags.GetFeatureFlagsResponse{
		Status:  "success",
		Message: "Feature flags retrieved successfully",
		Data:    flags,
	}, nil
}

// GetFeatureFlag retrieves a specific feature flag by name
func (s *FeatureFlagsServiceImpl) GetFeatureFlag(ctx context.Context, flagName string) (*feature_flags.FeatureFlag, error) {
	return s.repo.GetByName(ctx, flagName)
}

// UpsertFeatureFlag creates or updates a feature flag
func (s *FeatureFlagsServiceImpl) UpsertFeatureFlag(ctx context.Context, flag *feature_flags.FeatureFlag) error {
	return s.repo.UpsertFlag(ctx, flag)
}

// DeleteFeatureFlag deletes a feature flag
func (s *FeatureFlagsServiceImpl) DeleteFeatureFlag(ctx context.Context, flagName string) error {
	return s.repo.DeleteFlag(ctx, flagName)
}
