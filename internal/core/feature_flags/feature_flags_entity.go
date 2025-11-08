package feature_flags

import (
	"time"
)

// FeatureFlag represents a single global feature flag
type FeatureFlag struct {
	FlagID      string      `json:"flag_id"`
	FlagName    string      `json:"flag_name"`
	FlagValue   interface{} `json:"flag_value"` // Can be bool, string, array, or object
	Description string      `json:"description,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// FeatureFlags represents a map of all feature flags (global)
type FeatureFlags map[string]interface{}
