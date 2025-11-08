package ratelimiter

import (
	"context"
	"fmt"
	"time"

	redis "github.com/redis/go-redis/v9"
)

func init() {
	// Register token bucket strategy on package init
	RegisterStrategy("token_bucket", NewTokenBucketStrategy)
}

// TokenBucketStrategy implements the token bucket rate limiting algorithm
type TokenBucketStrategy struct {
	capacity        int64   // Maximum number of tokens in the bucket
	refillPerSecond float64 // Rate at which tokens are added to the bucket
	requestedTokens int64   // Number of tokens to consume per request
}

// NewTokenBucketStrategy creates a new token bucket strategy from config
func NewTokenBucketStrategy(config map[string]interface{}) (Strategy, error) {
	capacity, ok := config["capacity"].(float64)
	if !ok {
		return nil, fmt.Errorf("token_bucket: missing or invalid 'capacity'")
	}

	refillPerSecond, ok := config["refill_per_second"].(float64)
	if !ok {
		return nil, fmt.Errorf("token_bucket: missing or invalid 'refill_per_second'")
	}

	requestedTokens, ok := config["requested_tokens"].(float64)
	if !ok {
		requestedTokens = 1 // Default to 1 token per request
	}

	return &TokenBucketStrategy{
		capacity:        int64(capacity),
		refillPerSecond: refillPerSecond,
		requestedTokens: int64(requestedTokens),
	}, nil
}

// Check performs the token bucket rate limit check
func (s *TokenBucketStrategy) Check(ctx context.Context, redisClient *redis.Client, key string) (*StrategyResult, error) {
	now := time.Now().UnixMilli()

	// Use Lua script for atomic token bucket operations
	script := redis.NewScript(TokenBucketLuaScript)

	result, err := script.Run(
		ctx,
		redisClient,
		[]string{key},
		s.capacity,
		s.refillPerSecond,
		now,
		s.requestedTokens,
	).Result()

	if err != nil {
		return nil, fmt.Errorf("token bucket execution failed: %w", err)
	}

	// Parse Lua result: {allowed, remaining_tokens, retry_after_seconds}
	resultArray, ok := result.([]interface{})
	if !ok || len(resultArray) != 3 {
		return nil, fmt.Errorf("unexpected Lua script result format")
	}

	allowed := resultArray[0].(int64) == 1
	remainingTokens := resultArray[1].(string)
	retryAfterSecs := resultArray[2].(string)

	return &StrategyResult{
		Allowed:         allowed,
		RemainingTokens: remainingTokens,
		RetryAfterSecs:  retryAfterSecs,
		StrategyType:    "token_bucket",
	}, nil
}

// GetType returns the strategy type
func (s *TokenBucketStrategy) GetType() string {
	return "token_bucket"
}
