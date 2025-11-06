package ratelimiter

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

// GroupFunc is a function that determines the rate limit group for a request.
type GroupFunc func(c echo.Context) string

// Middleware returns an Echo middleware that applies rate limiting.
// The groupFunc parameter determines which policy group to use for each request.
func Middleware(groupFunc GroupFunc) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			rl := GetInstance()

			// Determine group for this request
			group := groupFunc(c)
			if group == "" {
				group = "default"
			}

			// Check rate limit
			result, err := rl.CheckLimit(c, group)
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

// DefaultGroupFunc returns a simple group function that always returns "default".
func DefaultGroupFunc() GroupFunc {
	return func(c echo.Context) string {
		return "default"
	}
}

// PathBasedGroupFunc returns a group function that maps routes to groups.
// It checks the request path and returns an appropriate group name.
func PathBasedGroupFunc() GroupFunc {
	return func(c echo.Context) string {
		path := c.Path()

		// Map paths to groups
		switch {
		case path == "/v1/auth/login" || path == "/v1/auth/refresh":
			return "login"
		case path == "/v1/auth/signup" || path == "/v1/auth/verify-email":
			return "signup"
		case path == "/v1/user/onboard/aadhaar/webhook" || path == "/v1/user/onboard/aadhaar/callback":
			return "api_high" // High throughput for callbacks
		default:
			return "default"
		}
	}
}

// MethodGroupFunc returns a group function that considers both path and method.
func MethodGroupFunc() GroupFunc {
	return func(c echo.Context) string {
		path := c.Path()
		method := c.Request().Method

		// Special handling for specific routes
		if method == http.MethodPost {
			switch path {
			case "/v1/auth/login":
				return "login"
			case "/v1/auth/signup":
				return "signup"
			case "/v1/auth/request-reset-password":
				return "login" // Same policy as login
			}
		}

		// High throughput for webhooks
		if method == http.MethodPost && (c.Path() == "/v1/user/onboard/aadhaar/webhook" ||
			c.Path() == "/v1/process_payment/webhooks/payment_status" ||
			c.Path() == "/v1/process_payment/webhooks/settlement_status") {
			return "api_high"
		}

		return "default"
	}
}
