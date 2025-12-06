package middleware

import (
	"errors"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/opsauth"
	"xcentro.in/internal/shared/config"
)

// OpsJWTMiddleware creates a middleware that validates JWT access tokens for ops users
func OpsJWTMiddleware(authService opsauth.OpsAuthService, cfg *config.Config) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip validation if configured (dev/testing only)
			if cfg.OpsOAuth.SkipValidation {
				c.Set("opsUserId", "dev-user")
				c.Set("opsUserRoles", []string{})
				return next(c)
			}

			// Extract access token from cookie
			cookie, err := c.Cookie("ops_access_token")
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

			// Validate token
			claims, err := authService.ValidateAccessToken(tokenString)
			if err != nil {
				// Check if it's an expiration error
				if errors.Is(err, jwt.ErrTokenExpired) {
					return c.JSON(401, map[string]any{
						"error":   "TOKEN_EXPIRED",
						"message": "Access token has expired, please refresh.",
					})
				}

				// Generic invalid token error
				return c.JSON(401, map[string]any{
					"error":   "INVALID_TOKEN",
					"message": "Invalid token",
				})
			}

			// Enforce role-based access control
			// Only users with "access.all" role can access ops dashboard
			if !hasRole(claims.Roles, "access.all") {
				return c.JSON(403, map[string]any{
					"error":   "UNAUTHORIZED",
					"message": "Insufficient permissions to access ops dashboard",
				})
			}

			// Set ops user context for downstream handlers
			// Only OID and roles are in the token (for security)
			// Email/name should be fetched from database if needed
			c.Set("opsUserId", claims.Subject)
			c.Set("opsUserRoles", claims.Roles)

			// Continue to next handler
			return next(c)
		}
	}
}

// hasRole checks if a role exists in the roles slice
func hasRole(roles []string, role string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}
