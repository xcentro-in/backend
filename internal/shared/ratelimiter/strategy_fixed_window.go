package ratelimiter

import (
	"context"
	"fmt"
	"strconv"
	"time"

	redis "github.com/redis/go-redis/v9"
)

func init() {
	// Register fixed window strategy on package init
	RegisterStrategy("fixed_window", NewFixedWindowStrategy)
}

// FixedWindowStrategy implements the fixed window rate limiting algorithm
// Useful for limits like "X requests per day" or "Y emails per hour"
type FixedWindowStrategy struct {
	limit         int64 // Maximum requests allowed in the window
	windowSeconds int64 // Window duration in seconds (e.g., 86400 for daily)
	tokens        int64 // Number of tokens to consume per request
}

// NewFixedWindowStrategy creates a new fixed window strategy from config
func NewFixedWindowStrategy(config map[string]interface{}) (Strategy, error) {
	limit, ok := config["limit"].(float64)
	if !ok {
		return nil, fmt.Errorf("fixed_window: missing or invalid 'limit'")
	}

	windowSeconds, ok := config["window_seconds"].(float64)
	if !ok {
		return nil, fmt.Errorf("fixed_window: missing or invalid 'window_seconds'")
	}

	tokens, ok := config["tokens"].(float64)
	if !ok {
		tokens = 1 // Default to 1 token per request
	}

	return &FixedWindowStrategy{
		limit:         int64(limit),
		windowSeconds: int64(windowSeconds),
		tokens:        int64(tokens),
	}, nil
}

// Check performs the fixed window rate limit check
func (s *FixedWindowStrategy) Check(ctx context.Context, redisClient *redis.Client, key string) (*StrategyResult, error) {
	now := time.Now().Unix()

	// Calculate window start time (aligned to window boundaries)
	windowStart := (now / s.windowSeconds) * s.windowSeconds

	// Key includes window start time to create fixed windows
	windowKey := fmt.Sprintf("%s:fw:%d", key, windowStart)

	// Use Lua script for atomic increment and check
	script := redis.NewScript(FixedWindowLuaScript)

	result, err := script.Run(
		ctx,
		redisClient,
		[]string{windowKey},
		s.limit,
		s.windowSeconds,
		s.tokens,
	).Result()

	if err != nil {
		return nil, fmt.Errorf("fixed window execution failed: %w", err)
	}

	// Parse result: {allowed, remaining, retry_after_seconds}
	resultArray, ok := result.([]interface{})
	if !ok || len(resultArray) != 3 {
		return nil, fmt.Errorf("unexpected Lua script result format")
	}

	allowed := resultArray[0].(int64) == 1
	remaining := resultArray[1].(string)
	retryAfter := resultArray[2].(string)

	// If retry_after is 0 but request denied, calculate actual retry time
	if !allowed && retryAfter == "0" {
		// Window should reset soon, get actual TTL
		ttl, _ := redisClient.TTL(ctx, windowKey).Result()
		retryAfter = strconv.FormatInt(int64(ttl.Seconds()), 10)
	}

	return &StrategyResult{
		Allowed:         allowed,
		RemainingTokens: remaining,
		RetryAfterSecs:  retryAfter,
		StrategyType:    "fixed_window",
	}, nil
}

// GetType returns the strategy type
func (s *FixedWindowStrategy) GetType() string {
	return "fixed_window"
}
