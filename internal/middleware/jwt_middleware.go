package middleware

import (
	"errors"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/auth"
)

// JWTMiddlewareNoKYCCheck validates JWT and sets context values identically to
// JWTMiddleware but does NOT enforce the KYC verification gate. Use this only
// for endpoints that must be reachable regardless of KYC state (e.g. /verify-session).
func JWTMiddlewareNoKYCCheck(authService auth.AuthService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			cookie, err := c.Cookie("access_token")
			if err != nil {
				return c.JSON(401, map[string]any{
					"error":   "MISSING_TOKEN",
					"message": "Access token cookie required",
				})
			}

			tokenString := cookie.Value
			if tokenString == "" {
				return c.JSON(401, map[string]any{
					"error":   "MISSING_TOKEN",
					"message": "Access token cookie is empty",
				})
			}

			claims, err := authService.ValidateAccessTokenWithVersion(c.Request().Context(), tokenString)
			if err != nil {
				if errors.Is(err, jwt.ErrTokenExpired) {
					return c.JSON(401, map[string]any{
						"error":   "TOKEN_EXPIRED",
						"message": "Access token has expired, please refresh.",
					})
				}
				if err.Error() == "SESSION_INVALIDATED" {
					return c.JSON(401, map[string]any{
						"error":   "SESSION_INVALIDATED",
						"message": "Your session has been invalidated. Please login again.",
					})
				}
				return c.JSON(401, map[string]any{
					"error":   "INVALID_TOKEN",
					"message": "Invalid token",
				})
			}

			c.Set("userId", claims.Subject)
			c.Set("sessionId", claims.SessionID)
			c.Set("bid", claims.Bid)
			c.Set("role", claims.Role)
			c.Set("kyc_status", claims.KycStatus)
			c.Set("kyc_needed", claims.KycNeeded)

			return next(c)
		}
	}
}

// JWTMiddleware creates a middleware that validates JWT access tokens from cookies
// and validates device version for session management (cached in Redis)
func JWTMiddleware(authService auth.AuthService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Extract access token from cookie
			cookie, err := c.Cookie("access_token")
			if err != nil {
				return c.JSON(401, map[string]any{
					"error":   "MISSING_TOKEN",
					"message": "Access token cookie required",
				})
			}

			tokenString := cookie.Value
			if tokenString == "" {
				return c.JSON(401, map[string]any{
					"error":   "MISSING_TOKEN",
					"message": "Access token cookie is empty",
				})
			}

			// Validate token with device version check (for session management)
			// Device version is cached in Redis for performance
			claims, err := authService.ValidateAccessTokenWithVersion(c.Request().Context(), tokenString)
			if err != nil {
				// Check if it's an expiration error
				if errors.Is(err, jwt.ErrTokenExpired) {
					return c.JSON(401, map[string]any{
						"error":   "TOKEN_EXPIRED",
						"message": "Access token has expired, please refresh.",
					})
				}

				// Check if it's a session invalidation error
				if err.Error() == "SESSION_INVALIDATED" {
					return c.JSON(401, map[string]any{
						"error":   "SESSION_INVALIDATED",
						"message": "Your session has been invalidated. Please login again.",
					})
				}

				// Generic invalid token error
				return c.JSON(401, map[string]any{
					"error":   "INVALID_TOKEN",
					"message": "Invalid token",
				})
			}

			// Set userID and sessionID in context for downstream handlers
			c.Set("userId", claims.Subject)
			c.Set("sessionId", claims.SessionID)
			c.Set("bid", claims.Bid)              // from CustomClaims.Bid — empty string if token predates Phase 3
			c.Set("role", claims.Role)            // from CustomClaims.Role — empty string if token predates Phase 3
			c.Set("kyc_status", claims.KycStatus) // from CustomClaims.KycStatus — empty string if token predates this change
			c.Set("kyc_needed", claims.KycNeeded) // from CustomClaims.KycNeeded — false if token predates this change

			// Enforce KYC verification requirement
			if claims.KycNeeded && claims.KycStatus != "completed" && claims.KycStatus != "verified" {
				return c.JSON(403, map[string]any{
					"error":   "KYC_REQUIRED",
					"message": "Please complete KYC verification to access this resource",
				})
			}

			// Continue to next handler
			return next(c)
		}
	}
}
