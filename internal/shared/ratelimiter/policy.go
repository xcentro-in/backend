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

// GetBuiltInPolicies returns all built-in rate limiting policies (default + SES).
// These can be overridden by custom JSON configuration.
// The "default" policy uses config values from environment variables.
func GetBuiltInPolicies(defaultCapacity int64, defaultRefillPerSec float64, defaultRequestedTokens int64, failOpenByDefault bool) map[string]Policy {
	return map[string]Policy{
		// Default policy - used as fallback for any unspecified group
		// Values come from RATE_LIMITER_DEFAULT_* environment variables
		"default": {
			Strategies: []StrategyConfig{
				{
					Type: "token_bucket",
					Config: map[string]interface{}{
						"capacity":          float64(defaultCapacity),
						"refill_per_second": defaultRefillPerSec,
						"requested_tokens":  float64(defaultRequestedTokens),
					},
				},
			},
			FailClosed: !failOpenByDefault, // Inverted: true = fail open, false = fail closed
		},

		// Login policy - strict rate limiting for authentication
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
			FailClosed: true, // Fail closed for security
		},

		// Signup policy - prevent spam registrations
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
			FailClosed: false, // Fail open for user experience
		},

		// High-volume API policy
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
			FailClosed: false, // Fail open for availability
		},

		// AWS SES policy for ap-south-1 region
		// Combines per-second burst and daily quota limits
		"ses:ap-south-1": {
			LogicalOperation: "and", // Both strategies must allow
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
			FailClosed: true, // Fail closed to prevent SES quota violations
		},

		// Email verification resend rate limiting (industry standard: 1 per 60s + 10 per day)
		"resend_email_verification": {
			LogicalOperation: "and",
			Strategies: []StrategyConfig{
				{
					Type: "fixed_window",
					Config: map[string]interface{}{
						"limit":          1.0,  // 1 request per window
						"window_seconds": 60.0, // 60 seconds cooldown
						"tokens":         1.0,
					},
				},
				{
					Type: "fixed_window",
					Config: map[string]interface{}{
						"limit":          10.0,    // 10 requests per day
						"window_seconds": 86400.0, // 24 hours
						"tokens":         1.0,
					},
				},
			},
			FailClosed: false, // Fail open for UX (allow if Redis unavailable)
		},

		// Mobile OTP resend rate limiting (stricter due to SMS cost: 1 per 60s + 5 per day)
		"resend_mobile_otp": {
			LogicalOperation: "and",
			Strategies: []StrategyConfig{
				{
					Type: "fixed_window",
					Config: map[string]interface{}{
						"limit":          1.0,  // 1 request per window
						"window_seconds": 60.0, // 60 seconds cooldown
						"tokens":         1.0,
					},
				},
				{
					Type: "fixed_window",
					Config: map[string]interface{}{
						"limit":          5.0,     // 5 requests per day (SMS cost control)
						"window_seconds": 86400.0, // 24 hours
						"tokens":         1.0,
					},
				},
			},
			FailClosed: true, // Fail closed to prevent SMS cost abuse
		},

		// OTP verification attempt rate limiting (brute-force prevention: 5 attempts with refill)
		"verify_mobile_otp": {
			LogicalOperation: "and",
			Strategies: []StrategyConfig{
				{
					Type: "token_bucket",
					Config: map[string]interface{}{
						"capacity":          5.0,      // 5 verification attempts
						"refill_per_second": 0.000278, // ~1 token per hour (1/3600)
						"requested_tokens":  1.0,
					},
				},
			},
			FailClosed: true, // Fail closed for security (prevent brute-force if Redis down)
		},
	}
}
