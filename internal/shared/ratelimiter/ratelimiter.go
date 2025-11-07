package ratelimiter

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

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
	redis           *redis.Client
	deviceHandler   *DeviceHandler
	policies        map[string]Policy
	defaultPolicy   Policy
	failOpenDefault bool
	tokenKeyTTL     int // in seconds
	scriptSHA       string
	log             *logger.Logger
	mu              sync.RWMutex
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
func Initialize(redisClient *redis.Client, cfg *config.RateLimiterConfig, cookieSecure bool, cookieDomain string) error {
	var initErr error
	once.Do(func() {
		log := logger.GetDefault()

		// Load Lua script
		sha, err := redisClient.ScriptLoad(context.Background(), TokenBucketLuaScript).Result()
		if err != nil {
			initErr = fmt.Errorf("failed to load Lua script: %w", err)
			return
		}

		// Create device handler
		deviceHandler := NewDeviceHandler(
			redisClient,
			cfg.CookieName,
			cfg.CookieTTL,
			cfg.DeviceMappingTTL,
			cfg.DeviceCreationThrottlePerIP,
			cfg.DeviceCreationThrottleWindow,
			cookieSecure,
			cookieDomain,
		)

		// Build default policy from config
		defaultPolicy := Policy{
			Capacity:        cfg.DefaultCapacity,
			RefillPerSecond: cfg.DefaultRefillPerSec,
			RequestedTokens: cfg.DefaultRequestedTokens,
			FailClosed:      false, // Default always fails open
		}

		// Parse custom group policies from JSON
		policies, err := parsePoliciesFromJSON(cfg.GroupPoliciesJSON, defaultPolicy, log)
		if err != nil {
			log.Warnf("Failed to parse group policies JSON, using only default policy: %v", err)
			policies = map[string]Policy{"default": defaultPolicy}
		}

		// Merge SES policies (for email rate limiting)
		sesPolicies := SESPolicies()
		for group, policy := range sesPolicies {
			if _, exists := policies[group]; !exists {
				policies[group] = policy
				log.Infof("Loaded SES policy: %s", group)
			}
		}

		instance = &RateLimiter{
			redis:           redisClient,
			deviceHandler:   deviceHandler,
			policies:        policies,
			defaultPolicy:   policies["default"],
			failOpenDefault: cfg.FailOpenByDefault,
			tokenKeyTTL:     int(cfg.TokenKeyTTL.Seconds()),
			scriptSHA:       sha,
			log:             log,
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
		rl.log.Errorf("Failed to determine scope key: %v", err)
		// Fail open or closed based on policy
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
	userID := c.Get("user_id")
	if userID != nil {
		userIDStr, ok := userID.(string)
		if ok && userIDStr != "" {
			return fmt.Sprintf("rl:user:%s|%s", userIDStr, group), "user", nil
		}
	}

	// Priority 2: Device ID
	deviceID, created, err := rl.deviceHandler.EnsureDeviceID(c)
	if err == nil && deviceID != "" {
		if created {
			rl.log.Infof("Created new device for rate limiting: %s", deviceID)
		}
		return fmt.Sprintf("rl:device:%s|%s", deviceID, group), "device", nil
	}

	// Log device error but continue to IP fallback
	if err != nil {
		rl.log.Warnf("Device handling failed, falling back to IP: %v", err)
	}

	// Priority 3: IP address (fallback)
	ip := c.RealIP()
	if ip == "" {
		return "", "", fmt.Errorf("unable to determine client IP")
	}
	return fmt.Sprintf("rl:ip:%s|%s", ip, group), "ip", nil
}

// executeRateLimit executes rate limiting check with support for multiple strategies
func (rl *RateLimiter) executeRateLimit(ctx context.Context, key string, policy Policy) (*LimitResult, error) {
	// Get strategy configs (handles both legacy and new format)
	strategyConfigs := policy.ToStrategyConfigs()

	if len(strategyConfigs) == 0 {
		return nil, fmt.Errorf("no strategies configured for policy")
	}

	// Single strategy - simple case (backward compatible)
	if len(strategyConfigs) == 1 {
		return rl.executeStrategy(ctx, key, strategyConfigs[0])
	}

	// Multiple strategies - apply logical operation
	logicalOp := policy.LogicalOperation
	if logicalOp == "" {
		logicalOp = "and" // Default to AND (strictest)
	}

	return rl.executeMultipleStrategies(ctx, key, strategyConfigs, logicalOp)
}

// executeStrategy executes a single rate limiting strategy
func (rl *RateLimiter) executeStrategy(ctx context.Context, key string, config StrategyConfig) (*LimitResult, error) {
	// Get or create strategy instance
	if config.strategyInstance == nil {
		strategy, err := CreateStrategy(config.Type, config.Config)
		if err != nil {
			return nil, fmt.Errorf("failed to create strategy %s: %w", config.Type, err)
		}
		if strategy == nil {
			return nil, fmt.Errorf("unknown strategy type: %s", config.Type)
		}
		config.strategyInstance = strategy
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

// executeTokenBucket executes the Lua token bucket script.
func (rl *RateLimiter) executeTokenBucket(ctx context.Context, key string, policy Policy) (*LimitResult, error) {
	now := time.Now().UnixMilli()

	// Execute Lua script
	result, err := rl.redis.EvalSha(ctx, rl.scriptSHA, []string{key},
		policy.Capacity,
		fmt.Sprintf("%f", policy.RefillPerSecond),
		now,
		policy.RequestedTokens,
	).Result()

	if err != nil {
		// Script not found, reload it
		if err.Error() == "NOSCRIPT No matching script. Please use EVAL." {
			rl.log.Warn("Lua script not found, reloading...")
			sha, loadErr := rl.redis.ScriptLoad(ctx, TokenBucketLuaScript).Result()
			if loadErr != nil {
				return nil, fmt.Errorf("failed to reload Lua script: %w", loadErr)
			}
			rl.mu.Lock()
			rl.scriptSHA = sha
			rl.mu.Unlock()

			// Retry execution
			result, err = rl.redis.EvalSha(ctx, rl.scriptSHA, []string{key},
				policy.Capacity,
				fmt.Sprintf("%f", policy.RefillPerSecond),
				now,
				policy.RequestedTokens,
			).Result()

			if err != nil {
				return nil, fmt.Errorf("token bucket execution failed after reload: %w", err)
			}
		} else {
			return nil, fmt.Errorf("token bucket execution failed: %w", err)
		}
	}

	// Parse Lua result: {allowed, remaining_tokens, retry_after_seconds}
	resultArray, ok := result.([]interface{})
	if !ok || len(resultArray) != 3 {
		return nil, fmt.Errorf("unexpected Lua script result format")
	}

	allowed := resultArray[0].(int64) == 1
	remainingTokens := resultArray[1].(string)
	retryAfterSecs := resultArray[2].(string)

	return &LimitResult{
		Allowed:         allowed,
		RemainingTokens: remainingTokens,
		RetryAfterSecs:  retryAfterSecs,
	}, nil
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
	if policy.FailClosed {
		return true
	}
	return !rl.failOpenDefault
}

// AddPolicy adds or updates a policy for a specific group.
func (rl *RateLimiter) AddPolicy(group string, policy Policy) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.policies[group] = policy
	rl.log.Infof("Added/updated policy for group: %s", group)
}

// RemovePolicy removes a policy for a specific group.
func (rl *RateLimiter) RemovePolicy(group string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.policies, group)
	rl.log.Infof("Removed policy for group: %s", group)
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

// parsePoliciesFromJSON parses the group policies from JSON string and merges with default policy.
// The defaultPolicy is always included under "default" key unless JSON overrides it.
func parsePoliciesFromJSON(jsonStr string, defaultPolicy Policy, log *logger.Logger) (map[string]Policy, error) {
	// Start with default policy
	policies := map[string]Policy{
		"default": defaultPolicy,
	}

	// If JSON string is empty, return just the default
	if jsonStr == "" {
		log.Info("No group policies JSON configured, using only default policy from config")
		return policies, nil
	}

	// Parse JSON into map
	var customPolicies map[string]Policy
	if err := json.Unmarshal([]byte(jsonStr), &customPolicies); err != nil {
		return nil, fmt.Errorf("failed to unmarshal group policies JSON: %w", err)
	}

	// Merge custom policies (including potential "default" override)
	for group, policy := range customPolicies {
		policies[group] = policy
		if group == "default" {
			log.Info("JSON overrides default policy from config")
		}
	}

	log.Infof("Loaded %d group policies (%d from JSON + default from config)", len(policies), len(customPolicies))
	return policies, nil
}
