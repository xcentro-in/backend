package ratelimiter

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

// Middleware returns an Echo middleware that applies rate limiting for a specific policy group.
// If the group doesn't exist in loaded policies, falls back to "default" policy.
//
// Usage:
//   // Apply to specific route
//   e.POST("/login", handler.Login, ratelimiter.Middleware("login"))
//
//   // Apply to entire route group
//   api := e.Group("/api/v1", ratelimiter.Middleware("api_high"))
func Middleware(group string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			rl := GetInstance()

			// Use provided group, empty string will fallback to default in getPolicy()
			policyGroup := group
			if policyGroup == "" {
				policyGroup = "default"
			}

			// Check rate limit (getPolicy handles fallback to default if group not found)
			result, err := rl.CheckLimit(c, policyGroup)
			if err != nil {
				// Internal error, log and continue (fail-open unless policy says otherwise)
				rl.log.Errorf("Rate limit check error: %v", err)
				return next(c)
			}

			if result.Allowed {
				// Request allowed - add remaining tokens header
				c.Response().Header().Set("X-RateLimit-Remaining", result.RemainingTokens)
				return next(c)
			}

			// Request denied - return 429
			retryAfter := ParseRetryAfter(result.RetryAfterSecs)
			c.Response().Header().Set("Retry-After", strconv.Itoa(retryAfter))
			c.Response().Header().Set("X-RateLimit-Remaining", "0")

			return c.JSON(http.StatusTooManyRequests, map[string]interface{}{
				"error":            "rate_limited",
				"retry_after_secs": retryAfter,
				"scope":            result.Scope,
				"policy_group":     result.PolicyGroup,
			})
		}
	}
}

// MiddlewareDefault returns an Echo middleware that applies the default rate limit policy.
// Convenience function equivalent to Middleware("default").
//
// Usage:
//   e.GET("/health", handler.Health, ratelimiter.MiddlewareDefault())
func MiddlewareDefault() echo.MiddlewareFunc {
	return Middleware("default")
}
