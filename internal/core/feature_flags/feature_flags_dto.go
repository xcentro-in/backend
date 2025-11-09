package feature_flags

// GetFeatureFlagsResponse represents the API response for feature flags
type GetFeatureFlagsResponse struct {
	Status  string        `json:"status"`
	Message string        `json:"message"`
	Data    FeatureFlags  `json:"data"`
}
