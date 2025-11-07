package ratelimiter

// Policy represents a rate limiting policy with support for multiple strategies.
type Policy struct {
	// === BACKWARD COMPATIBILITY: Token bucket fields (single strategy) ===
	// These are used when Strategies is empty/nil
	Capacity        int64   `json:"capacity"`
	RefillPerSecond float64 `json:"refill_per_second"`
	RequestedTokens int64   `json:"requested_tokens"`

	// === NEW: Multi-strategy support ===
	// LogicalOperation determines how multiple strategies are combined
	// "and" = all strategies must allow (strictest)
	// "or"  = at least one strategy must allow (most permissive)
	LogicalOperation string `json:"logical_operation"` // "and" | "or" (default: "and")

	// Strategies is a list of rate limiting strategies to apply
	// Each strategy is evaluated independently, then combined based on LogicalOperation
	Strategies []StrategyConfig `json:"strategies"`

	// === Common fields ===
	// FailClosed determines whether to fail closed (reject) on Redis errors
	// If false, fails open (allow) on errors
	FailClosed bool `json:"fail_closed"`
}

// IsLegacyPolicy returns true if this policy uses the old single-strategy format
func (p *Policy) IsLegacyPolicy() bool {
	return len(p.Strategies) == 0 && p.Capacity > 0
}

// ToStrategyConfigs converts legacy policy to strategy configs
func (p *Policy) ToStrategyConfigs() []StrategyConfig {
	if !p.IsLegacyPolicy() {
		return p.Strategies
	}

	// Convert legacy token bucket config to strategy config
	return []StrategyConfig{
		{
			Type: "token_bucket",
			Config: map[string]interface{}{
				"capacity":         float64(p.Capacity),
				"refill_per_second": p.RefillPerSecond,
				"requested_tokens":  float64(p.RequestedTokens),
			},
		},
	}
}

// PolicyGroup represents policy configuration for a specific group.
type PolicyGroup struct {
	Name   string
	Policy Policy
}

// DefaultPolicies returns predefined policies for common groups.
// These use the legacy token bucket format for simplicity.
func DefaultPolicies() map[string]Policy {
	return map[string]Policy{
		"default": {
			Capacity:        120,
			RefillPerSecond: 2.0, // 120 req/min
			RequestedTokens: 1,
			FailClosed:      false,
		},
		"login": {
			Capacity:        5,
			RefillPerSecond: 0.005555, // 5 per 15 minutes
			RequestedTokens: 1,
			FailClosed:      true, // Sensitive, fail closed
		},
		"signup": {
			Capacity:        10,
			RefillPerSecond: 0.002777, // 10 per hour
			RequestedTokens: 1,
			FailClosed:      false,
		},
		"api_high": {
			Capacity:        1000,
			RefillPerSecond: 16.6667, // 1000 per minute
			RequestedTokens: 1,
			FailClosed:      false,
		},
	}
}

// SESPolicies returns rate limiting policies for AWS SES
// Combines per-second and daily limits
func SESPolicies() map[string]Policy {
	return map[string]Policy{
		"ses:ap-south-1": {
			LogicalOperation: "and", // Both must pass
			Strategies: []StrategyConfig{
				{
					Type: "token_bucket",
					Config: map[string]interface{}{
						"capacity":          14.0, // 14 emails/sec burst
						"refill_per_second": 14.0, // Sustained rate
						"requested_tokens":  1.0,
					},
				},
				{
					Type: "fixed_window",
					Config: map[string]interface{}{
						"limit":          50000.0, // 50k emails per day
						"window_seconds": 86400.0, // 24 hours
						"tokens":         1.0,
					},
				},
			},
			FailClosed: true, // Fail closed for SES (critical)
		},
	}
}

