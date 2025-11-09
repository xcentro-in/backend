package feature_flags

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/services/feature_flags"
	"xcentro.in/internal/shared/logger"
)

// FeatureFlagsHandler handles HTTP requests for feature flags
type FeatureFlagsHandler struct {
	service feature_flags.FeatureFlagsService
	logger  *logger.Logger
}

// NewFeatureFlagsHandler creates a new feature flags HTTP handler
func NewFeatureFlagsHandler(service feature_flags.FeatureFlagsService) *FeatureFlagsHandler {
	return &FeatureFlagsHandler{
		service: service,
		logger:  logger.GetDefault(),
	}
}

// GetFeatureFlags handles GET /v1/auth/feature-flags?businessId={businessId}
// Note: businessId query param is optional (for backward compatibility) but ignored - returns global flags
func (h *FeatureFlagsHandler) GetFeatureFlags(c echo.Context) error {
	h.logger.DebugWith("Fetching global feature flags")

	response, err := h.service.GetAllFeatureFlags(c.Request().Context())
	if err != nil {
		h.logger.ErrorWith("Failed to fetch feature flags", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"status":  "error",
			"message": "Failed to fetch feature flags",
		})
	}

	return c.JSON(http.StatusOK, response)
}
