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
	scriptSHA       string  // SHA of the Lua script
	tokenKeyTTL     int     // TTL for token bucket keys in seconds
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

	// Get scriptSHA and tokenKeyTTL from global instance (will be set during initialization)
	rl := GetInstance()

	return &TokenBucketStrategy{
		capacity:        int64(capacity),
		refillPerSecond: refillPerSecond,
		requestedTokens: int64(requestedTokens),
		scriptSHA:       rl.scriptSHA,
		tokenKeyTTL:     rl.tokenKeyTTL,
	}, nil
}

// Check performs the token bucket rate limit check
func (s *TokenBucketStrategy) Check(ctx context.Context, redisClient *redis.Client, key string) (*StrategyResult, error) {
	now := time.Now().Unix()

	// Execute token bucket Lua script
	result, err := redisClient.EvalSha(
		ctx,
		s.scriptSHA,
		[]string{key},
		s.capacity,
		s.refillPerSecond,
		now,
		s.requestedTokens,
	).Result()

	if err != nil {
		// If script not loaded, try to reload and execute again
		if err.Error() == "NOSCRIPT No matching script. Please use EVAL." {
			// Reload script
			sha, reloadErr := redisClient.ScriptLoad(ctx, TokenBucketLuaScript).Result()
			if reloadErr != nil {
				return nil, fmt.Errorf("failed to reload Lua script: %w", reloadErr)
			}

			// Update SHA and retry
			s.scriptSHA = sha
			result, err = redisClient.EvalSha(
				ctx,
				s.scriptSHA,
				[]string{key},
				s.capacity,
				s.refillPerSecond,
				now,
				s.requestedTokens,
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
