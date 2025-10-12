package middleware

import (
	"errors"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/auth"
)

// JWTMiddleware creates a middleware that validates JWT access tokens from cookies
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

			// Set userID in context for downstream handlers
			c.Set("userId", claims.Subject)

			// Continue to next handler
			return next(c)
		}
	}
}
