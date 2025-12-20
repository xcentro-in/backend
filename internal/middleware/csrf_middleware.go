package middleware

import (
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/shared/httputil"
	"xcentro.in/internal/shared/logger"
)

// CSRFMiddleware validates CSRF tokens for all authenticated requests
// It extracts the session_id from the JWT claims (set by JWTMiddleware)
// and validates the CSRF token from the X-CSRF-Token header
func CSRFMiddleware(cookieManager *httputil.CookieManager) echo.MiddlewareFunc {
	log := logger.GetDefault()

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Extract session_id from context (set by JWTMiddleware)
			sessionIDRaw := c.Get("sessionId")
			if sessionIDRaw == nil {
				log.Warn("CSRF validation skipped: no session ID in context")
				return c.JSON(401, map[string]interface{}{
					"message": "Unauthorized",
				})
			}

			sessionID, ok := sessionIDRaw.(string)
			if !ok || sessionID == "" {
				log.Warn("CSRF validation failed: invalid session ID type")
				return c.JSON(401, map[string]interface{}{
					"message": "Unauthorized",
				})
			}

			// Extract CSRF token from X-CSRF-Token header
			csrfToken := c.Request().Header.Get("X-CSRF-Token")
			if csrfToken == "" {
				log.Warnf("CSRF validation failed: missing X-CSRF-Token header for session %s", sessionID)
				return c.JSON(401, map[string]interface{}{
					"message": "Unauthorized",
				})
			}

			// Validate CSRF token
			if !cookieManager.ValidateCSRFToken(sessionID, csrfToken) {
				log.Warnf("CSRF validation failed: invalid token for session %s", sessionID)
				return c.JSON(401, map[string]interface{}{
					"message": "Unauthorized",
				})
			}

			// CSRF validation passed
			return next(c)
		}
	}
}
