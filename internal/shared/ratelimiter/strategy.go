package ratelimiter

import (
	"context"

	redis "github.com/redis/go-redis/v9"
)

// Strategy defines the interface for rate limiting strategies.
// Each strategy implements a specific rate limiting algorithm (token bucket, fixed window, etc.).
type Strategy interface {
	// Check verifies if the request should be allowed based on this strategy
	Check(ctx context.Context, redis *redis.Client, key string) (*StrategyResult, error)

	// GetType returns the strategy type name
	GetType() string
}

// StrategyResult contains the result of a strategy check
type StrategyResult struct {
	Allowed         bool   // Whether the request is allowed
	RemainingTokens string // Remaining capacity (strategy-specific meaning)
	RetryAfterSecs  string // Seconds to wait before retry
	StrategyType    string // Type of strategy that produced this result
}

// StrategyConfig is the configuration for a single strategy within a policy
type StrategyConfig struct {
	Type              string                 `json:"type"`               // "token_bucket", "fixed_window", etc.
	Config            map[string]interface{} `json:"config"`             // Strategy-specific configuration
	strategyInstance  Strategy               // Internal: cached strategy instance
}

// Strategy factory function type
type StrategyFactory func(config map[string]interface{}) (Strategy, error)

var (
	// strategyFactories maps strategy types to their factory functions
	strategyFactories = make(map[string]StrategyFactory)
)

// RegisterStrategy registers a strategy factory for a given type
func RegisterStrategy(strategyType string, factory StrategyFactory) {
	strategyFactories[strategyType] = factory
}

// CreateStrategy creates a strategy instance from config
func CreateStrategy(strategyType string, config map[string]interface{}) (Strategy, error) {
	factory, exists := strategyFactories[strategyType]
	if !exists {
		// Return nil if strategy type not found - caller can decide to use default
		return nil, nil
	}
	return factory(config)
}
