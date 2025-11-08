package ratelimiter

import (
	"context"
	"fmt"
)

// AllowSES checks if an SES email send request should be allowed for a given region.
// Uses the ses:{region} policy group which combines per-second and daily limits.
//
// Usage:
//   result, err := ratelimiter.GetInstance().AllowSES(ctx, "ap-south-1", 1)
//   if err != nil || !result.Allowed {
//       // Rate limited, wait before retry
//   }
func (rl *RateLimiter) AllowSES(ctx context.Context, region string, emailCount int64) (*LimitResult, error) {
	// Build SES policy group name
	group := fmt.Sprintf("ses:%s", region)

	// Build Redis key for SES
	key := fmt.Sprintf("rl:ses:%s", region)

	// Check rate limit using the SES policy
	return rl.CheckLimitWithKey(ctx, key, group)
}

// AllowSESDefault checks if an SES email send request should be allowed using default region (ap-south-1).
// Convenience function equivalent to AllowSES(ctx, "ap-south-1", emailCount).
func (rl *RateLimiter) AllowSESDefault(ctx context.Context, emailCount int64) (*LimitResult, error) {
	return rl.AllowSES(ctx, "ap-south-1", emailCount)
}

// AllowGeneric checks rate limit for any custom key and policy group.
// Useful for domain-specific rate limiting beyond HTTP and SES.
//
// Usage:
//   // SMS rate limiting
//   result, err := ratelimiter.GetInstance().AllowGeneric(ctx, "sms:twilio:account-123", "sms-limit")
//
//   // Database writes
//   result, err := ratelimiter.GetInstance().AllowGeneric(ctx, "db:write:tenant-456", "db-write-limit")
func (rl *RateLimiter) AllowGeneric(ctx context.Context, key string, group string) (*LimitResult, error) {
	return rl.CheckLimitWithKey(ctx, key, group)
}
