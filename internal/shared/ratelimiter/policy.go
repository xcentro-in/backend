package ratelimiter

// Policy represents a token-bucket rate limiting policy.
type Policy struct {
	// Capacity is the maximum number of tokens (burst)
	Capacity int64 `json:"capacity"`

	// RefillPerSecond is the rate at which tokens are added per second
	RefillPerSecond float64 `json:"refill_per_second"`

	// RequestedTokens is the number of tokens consumed per request (usually 1)
	RequestedTokens int64 `json:"requested_tokens"`

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
