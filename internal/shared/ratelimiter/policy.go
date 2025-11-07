package ratelimiter

// Policy represents a rate limiting policy with support for multiple strategies.
type Policy struct {
	// LogicalOperation determines how multiple strategies are combined
	// "and" = all strategies must allow (strictest)
	// "or"  = at least one strategy must allow (most permissive)
	// Only matters when len(Strategies) > 1
	LogicalOperation string `json:"logical_operation"` // "and" | "or" (default: "and")

	// Strategies is a list of rate limiting strategies to apply
	// Each strategy is evaluated independently, then combined based on LogicalOperation
	// Must have at least one strategy
	Strategies []StrategyConfig `json:"strategies"`

	// FailClosed determines whether to fail closed (reject) on Redis errors
	// If false, fails open (allow) on errors
	FailClosed bool `json:"fail_closed"`
}

// PolicyGroup represents policy configuration for a specific group.
type PolicyGroup struct {
	Name   string
	Policy Policy
}

// DefaultPolicies returns predefined policies for common groups.
func DefaultPolicies() map[string]Policy {
	return map[string]Policy{
		"default": {
			Strategies: []StrategyConfig{
				{
					Type: "token_bucket",
					Config: map[string]interface{}{
						"capacity":          120.0,
						"refill_per_second": 2.0, // 120 req/min
						"requested_tokens":  1.0,
					},
				},
			},
			FailClosed: false,
		},
		"login": {
			Strategies: []StrategyConfig{
				{
					Type: "token_bucket",
					Config: map[string]interface{}{
						"capacity":          5.0,
						"refill_per_second": 0.005555, // 5 per 15 minutes
						"requested_tokens":  1.0,
					},
				},
			},
			FailClosed: true, // Sensitive, fail closed
		},
		"signup": {
			Strategies: []StrategyConfig{
				{
					Type: "token_bucket",
					Config: map[string]interface{}{
						"capacity":          10.0,
						"refill_per_second": 0.002777, // 10 per hour
						"requested_tokens":  1.0,
					},
				},
			},
			FailClosed: false,
		},
		"api_high": {
			Strategies: []StrategyConfig{
				{
					Type: "token_bucket",
					Config: map[string]interface{}{
						"capacity":          1000.0,
						"refill_per_second": 16.6667, // 1000 per minute
						"requested_tokens":  1.0,
					},
				},
			},
			FailClosed: false,
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


