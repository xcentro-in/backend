package middleware

import (
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/shared/config"
)

// OpsAccessMiddleware creates a middleware that controls access to ops endpoints
// Access is rejected by default unless explicitly enabled via environment variable
func OpsAccessMiddleware(cfg *config.Config) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if cfg.OpsAccess.RejectAccess {
				return c.JSON(403, map[string]any{
					"error":   "OPS_ACCESS_DISABLED",
					"message": "Operations dashboard access is currently disabled",
				})
			}

			// Access enabled, continue to next handler
			return next(c)
		}
	}
}
