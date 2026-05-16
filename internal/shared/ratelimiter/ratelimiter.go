package ratelimiter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
)

var (
	instance *RateLimiter
	once     sync.Once
)

// RateLimiter manages rate limiting with token-bucket algorithm.
type RateLimiter struct {
	redis                   *redis.Client
	deviceHandler           *DeviceHandler
	policies                map[string]Policy
	defaultPolicy           Policy
	invalidSignaturePenalty int // in seconds
	enabled                 bool
	log                     *logger.Logger
	mu                      sync.RWMutex
}

// LimitResult represents the result of a rate limit check.
type LimitResult struct {
	Allowed         bool
	RemainingTokens string
	RetryAfterSecs  string
	Scope           string
	PolicyGroup     string
}

// Initialize initializes the singleton RateLimiter instance.
// Must be called once at application startup before any middleware usage.
func Initialize(redisClient *redis.Client, cfg *config.RateLimiterConfig, cookieSecure bool, cookieSameSite string, cookieDomain string) error {
	var initErr error
	once.Do(func() {
		log := logger.GetDefault()

		// Create device handler with HMAC signing
		deviceHandler := NewDeviceHandler(
			redisClient,
			cfg.HMACSecret,
			cfg.CookieName,
			cfg.CookieTTL,
			cfg.DeviceCreationThrottlePerIP,
			cfg.DeviceCreationThrottleWindow,
			cookieSecure,
			cookieSameSite,
			cookieDomain,
			cfg.SkipDeviceValidation,
		)

		// Log dev mode warning if device validation is disabled
		if cfg.SkipDeviceValidation {
			log.Warn("⚠️  Device signature validation is DISABLED (dev mode) - all device cookies will be accepted")
		}

		// Get built-in policies (includes default, login, signup, api_high, SES, etc.)
		// Default policy uses values from config (environment variables)
		builtInPolicies := GetBuiltInPolicies(
			cfg.DefaultCapacity,
			cfg.DefaultRefillPerSec,
			cfg.DefaultRequestedTokens,
			cfg.FailOpenByDefault,
		)

		// Parse custom group policies from JSON (can override built-in policies)
		policies, err := parsePoliciesFromJSON(cfg.GroupPoliciesJSON, builtInPolicies, log)
		if err != nil {
			log.Warnf("Failed to parse group policies JSON, using only built-in policies: %v", err)
			policies = builtInPolicies
		}

		instance = &RateLimiter{
			redis:                   redisClient,
			deviceHandler:           deviceHandler,
			policies:                policies,
			defaultPolicy:           policies["default"],
			invalidSignaturePenalty: int(cfg.InvalidSignaturePenalty.Seconds()),
			enabled:                 cfg.Enabled,
			log:                     log,
		}

		if !cfg.Enabled {
			log.Warn("⚠️  Rate limiter is DISABLED (RATE_LIMITER_ENABLED=false) - all requests will be allowed")
		}

		log.Info("RateLimiter initialized successfully")
	})

	return initErr
}

// GetInstance returns the singleton RateLimiter instance.
// Panics if Initialize has not been called.
func GetInstance() *RateLimiter {
	if instance == nil {
		panic("RateLimiter not initialized. Call Initialize() first")
	}
	return instance
}

// CheckLimit checks if a request should be allowed based on rate limits.
func (rl *RateLimiter) CheckLimit(c echo.Context, group string) (*LimitResult, error) {
	ctx := c.Request().Context()

	// Get policy for group
	policy := rl.getPolicy(group)

	// Determine scope key
	scopeKey, scopeType, err := rl.determineScopeKey(c, group)
	if err != nil {
		// Check for invalid device signature (suspected tampering/DDoS)
		if errors.Is(err, ErrInvalidDeviceSignature) {
			rl.log.Errorf("Invalid device signature - rejecting request")
			return &LimitResult{
				Allowed:         false,
				RemainingTokens: "0",
				RetryAfterSecs:  strconv.Itoa(rl.invalidSignaturePenalty),
				Scope:           "invalid_signature",
				PolicyGroup:     group,
			}, nil
		}

		// Other errors - fail open or closed based on policy
		rl.log.Errorf("Failed to determine scope key: %v", err)
		if rl.shouldFailClosed(policy) {
			return &LimitResult{
				Allowed:     false,
				Scope:       "error",
				PolicyGroup: group,
			}, nil
		}
		return &LimitResult{
			Allowed:     true,
			Scope:       "error",
			PolicyGroup: group,
		}, nil
	}

	// Execute rate limit check (supports both legacy and multi-strategy)
	result, err := rl.executeRateLimit(ctx, scopeKey, policy)
	if err != nil {
		rl.log.Errorf("Rate limit execution failed: %v", err)
		// Fail open or closed based on policy
		if rl.shouldFailClosed(policy) {
			return &LimitResult{
				Allowed:     false,
				Scope:       scopeKey,
				PolicyGroup: group,
			}, nil
		}
		return &LimitResult{
			Allowed:     true,
			Scope:       scopeKey,
			PolicyGroup: group,
		}, nil
	}

	result.Scope = scopeKey
	result.PolicyGroup = group
	rl.log.Infof("Rate limit check: scope=%s, group=%s, allowed=%v, remaining=%s",
		scopeType, group, result.Allowed, result.RemainingTokens)

	return result, nil
}

// CheckLimitWithKey checks rate limit using a direct Redis key (for non-HTTP usage like SES)
func (rl *RateLimiter) CheckLimitWithKey(ctx context.Context, key string, group string) (*LimitResult, error) {
	// Get policy for group
	policy := rl.getPolicy(group)

	// Execute rate limit check
	result, err := rl.executeRateLimit(ctx, key, policy)
	if err != nil {
		rl.log.Errorf("Rate limit execution failed: %v", err)
		// Fail open or closed based on policy
		if rl.shouldFailClosed(policy) {
			return &LimitResult{
				Allowed:     false,
				Scope:       key,
				PolicyGroup: group,
			}, nil
		}
		return &LimitResult{
			Allowed:     true,
			Scope:       key,
			PolicyGroup: group,
		}, nil
	}

	result.Scope = key
	result.PolicyGroup = group
	return result, nil
}

// determineScopeKey determines the rate limit scope key based on priority: user > device > IP.
func (rl *RateLimiter) determineScopeKey(c echo.Context, group string) (string, string, error) {
	// Priority 1: Authenticated user
	userID := c.Get("userId")
	if userID != nil {
		userIDStr, ok := userID.(string)
		if ok && userIDStr != "" {
			return fmt.Sprintf("xcentro:rl:user:%s|%s", userIDStr, group), "user", nil
		}
	}

	// Priority 2: Device ID (HMAC-signed cookie)
	deviceID, created, err := rl.deviceHandler.EnsureDeviceID(c)
	if err == nil && deviceID != "" {
		if created {
			rl.log.Infof("Created new device for rate limiting: %s", deviceID)
		}
		return fmt.Sprintf("xcentro:rl:device:%s|%s", deviceID, group), "device", nil
	}

	// Check for invalid signature error (suspected tampering/DDoS)
	if err != nil && errors.Is(err, ErrInvalidDeviceSignature) {
		rl.log.Errorf("Invalid device signature detected from IP %s - rejecting request", c.RealIP())
		// Return special error that will be treated as rate limit exceeded
		return "", "invalid_signature", ErrInvalidDeviceSignature
	}

	// Log other device errors and continue to IP fallback
	if err != nil {
		rl.log.Warnf("Device handling failed, falling back to IP: %v", err)
	}

	// Priority 3: IP address (fallback)
	ip := c.RealIP()
	if ip == "" {
		return "", "", fmt.Errorf("unable to determine client IP")
	}
	return fmt.Sprintf("xcentro:rl:ip:%s|%s", ip, group), "ip", nil
}

// executeRateLimit executes rate limiting check with support for multiple strategies
func (rl *RateLimiter) executeRateLimit(ctx context.Context, key string, policy Policy) (*LimitResult, error) {
	if len(policy.Strategies) == 0 {
		return nil, fmt.Errorf("no strategies configured for policy")
	}

	// Single strategy - simple case
	if len(policy.Strategies) == 1 {
		return rl.executeStrategy(ctx, key, policy.Strategies[0])
	}

	// Multiple strategies - apply logical operation
	logicalOp := policy.LogicalOperation
	if logicalOp == "" {
		logicalOp = "and" // Default to AND (strictest)
	}

	return rl.executeMultipleStrategies(ctx, key, policy.Strategies, logicalOp)
}

// executeStrategy executes a single rate limiting strategy
func (rl *RateLimiter) executeStrategy(ctx context.Context, key string, config StrategyConfig) (*LimitResult, error) {
	// Strategy instance should have been initialized during policy parsing
	if config.strategyInstance == nil {
		return nil, fmt.Errorf("strategy instance not initialized for type: %s", config.Type)
	}

	// Execute strategy check
	result, err := config.strategyInstance.Check(ctx, rl.redis, key)
	if err != nil {
		return nil, fmt.Errorf("strategy %s failed: %w", config.Type, err)
	}

	// Convert StrategyResult to LimitResult
	return &LimitResult{
		Allowed:         result.Allowed,
		RemainingTokens: result.RemainingTokens,
		RetryAfterSecs:  result.RetryAfterSecs,
	}, nil
}

// executeMultipleStrategies executes multiple strategies with AND/OR logic
func (rl *RateLimiter) executeMultipleStrategies(ctx context.Context, key string, configs []StrategyConfig, logicalOp string) (*LimitResult, error) {
	results := make([]*LimitResult, len(configs))

	// Execute all strategies
	for i, config := range configs {
		result, err := rl.executeStrategy(ctx, key, config)
		if err != nil {
			return nil, fmt.Errorf("strategy %d failed: %w", i, err)
		}
		results[i] = result
	}

	// Apply logical operation
	if logicalOp == "or" {
		// OR: At least one must allow (most permissive)
		return rl.combineResultsOR(results), nil
	}

	// AND: All must allow (strictest) - default
	return rl.combineResultsAND(results), nil
}

// combineResultsAND combines results with AND logic - all must allow
func (rl *RateLimiter) combineResultsAND(results []*LimitResult) *LimitResult {
	// Find the most restrictive result
	allowed := true
	var maxRetryAfter string = "0"
	var minRemaining string

	for i, result := range results {
		if !result.Allowed {
			allowed = false
		}

		// Track maximum retry time (most restrictive)
		if result.RetryAfterSecs > maxRetryAfter {
			maxRetryAfter = result.RetryAfterSecs
		}

		// Track minimum remaining tokens
		if i == 0 || result.RemainingTokens < minRemaining {
			minRemaining = result.RemainingTokens
		}
	}

	return &LimitResult{
		Allowed:         allowed,
		RemainingTokens: minRemaining,
		RetryAfterSecs:  maxRetryAfter,
	}
}

// combineResultsOR combines results with OR logic - at least one must allow
func (rl *RateLimiter) combineResultsOR(results []*LimitResult) *LimitResult {
	// Find the most permissive result
	allowed := false
	var minRetryAfter string
	var maxRemaining string = "0"

	for i, result := range results {
		if result.Allowed {
			allowed = true
		}

		// Track minimum retry time (most permissive)
		if i == 0 || result.RetryAfterSecs < minRetryAfter {
			minRetryAfter = result.RetryAfterSecs
		}

		// Track maximum remaining tokens
		if result.RemainingTokens > maxRemaining {
			maxRemaining = result.RemainingTokens
		}
	}

	// If any strategy allows, use its retry time; otherwise use minimum
	if allowed {
		minRetryAfter = "0"
	}

	return &LimitResult{
		Allowed:         allowed,
		RemainingTokens: maxRemaining,
		RetryAfterSecs:  minRetryAfter,
	}
}

// getPolicy returns the policy for a given group, falling back to default.
func (rl *RateLimiter) getPolicy(group string) Policy {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	if policy, exists := rl.policies[group]; exists {
		return policy
	}
	return rl.defaultPolicy
}

// shouldFailClosed determines if rate limiter should fail closed on errors.
func (rl *RateLimiter) shouldFailClosed(policy Policy) bool {
	return policy.FailClosed
}

// ParseRetryAfter converts retry_after string to integer seconds for header.
func ParseRetryAfter(retryAfterStr string) int {
	retryAfter, err := strconv.ParseFloat(retryAfterStr, 64)
	if err != nil {
		return 1 // Default to 1 second if parse fails
	}
	// Ceil to next whole second
	return int(retryAfter) + 1
}

// parsePoliciesFromJSON parses the group policies from JSON string and merges with built-in policies.
// Built-in policies can be overridden by JSON configuration.
func parsePoliciesFromJSON(jsonStr string, builtInPolicies map[string]Policy, log *logger.Logger) (map[string]Policy, error) {
	// Start with built-in policies (includes default, login, signup, api_high, SES, etc.)
	policies := make(map[string]Policy, len(builtInPolicies))
	for group, policy := range builtInPolicies {
		policies[group] = policy
	}

	// If JSON string is empty, return built-in policies
	if jsonStr == "" {
		log.Infof("No group policies JSON configured, using %d built-in policies", len(builtInPolicies))
		// Initialize all strategy instances
		for group, policy := range policies {
			for i := range policy.Strategies {
				strategy, err := CreateStrategy(policy.Strategies[i].Type, policy.Strategies[i].Config)
				if err != nil {
					return nil, fmt.Errorf("failed to create strategy %s for group %s: %w", policy.Strategies[i].Type, group, err)
				}
				if strategy == nil {
					return nil, fmt.Errorf("unknown strategy type %s for group %s", policy.Strategies[i].Type, group)
				}
				policy.Strategies[i].strategyInstance = strategy
			}
			policies[group] = policy
		}
		return policies, nil
	}

	// Parse JSON into map
	var customPolicies map[string]Policy
	if err := json.Unmarshal([]byte(jsonStr), &customPolicies); err != nil {
		return nil, fmt.Errorf("failed to unmarshal group policies JSON: %w", err)
	}

	// Merge custom policies (can override built-in policies)
	for group, policy := range customPolicies {
		if _, wasBuiltIn := builtInPolicies[group]; wasBuiltIn {
			log.Infof("JSON overrides built-in policy: %s", group)
		}
		policies[group] = policy
	}

	// Initialize all strategy instances to avoid race conditions
	for group, policy := range policies {
		for i := range policy.Strategies {
			strategy, err := CreateStrategy(policy.Strategies[i].Type, policy.Strategies[i].Config)
			if err != nil {
				return nil, fmt.Errorf("failed to create strategy %s for group %s: %w", policy.Strategies[i].Type, group, err)
			}
			if strategy == nil {
				return nil, fmt.Errorf("unknown strategy type %s for group %s", policy.Strategies[i].Type, group)
			}
			policy.Strategies[i].strategyInstance = strategy
		}
		// Update policy in map with initialized strategies
		policies[group] = policy
	}

	log.Infof("Loaded %d group policies (%d built-in + %d from JSON)", len(policies), len(builtInPolicies), len(customPolicies))
	return policies, nil
}
