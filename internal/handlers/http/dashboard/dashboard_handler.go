package dashboard

import (
	"net/http"

	"github.com/labstack/echo/v4"
	dashboardService "xcentro.in/internal/services/dashboard"
	"xcentro.in/internal/shared/logger"
)

// DashboardHandler handles dashboard-related HTTP requests
type DashboardHandler struct {
	dashboardService *dashboardService.DashboardService
	logger           *logger.Logger
}

// NewDashboardHandler creates a new dashboard handler
func NewDashboardHandler(
	dashboardService *dashboardService.DashboardService,
	logger *logger.Logger,
) *DashboardHandler {
	return &DashboardHandler{
		dashboardService: dashboardService,
		logger:           logger,
	}
}

// GetDashboardMetrics handles GET /v1/dashboard
// @Summary Get dashboard metrics
// @Description Fetch all dashboard metrics concurrently (total spend, overdue bills, recent payments, upcoming payments, monthly trends, payment source breakdown)
// @Tags Dashboard
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} dashboard.DashboardMetrics "Dashboard metrics"
// @Failure 401 {object} map[string]any "Unauthorized"
// @Failure 400 {object} map[string]any "Bad request"
// @Failure 500 {object} map[string]any "Internal server error"
// @Router /v1/dashboard [get]
func (h *DashboardHandler) GetDashboardMetrics(c echo.Context) error {
	ctx := c.Request().Context()

	// Get user ID from JWT context
	userID, ok := c.Get("userId").(string)
	if !ok || userID == "" {
		h.logger.Warn("Missing userId in JWT context")
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error":   "MISSING_USER_ID",
			"message": "User ID not found in token",
		})
	}
	h.logger.DebugWith("User authenticated", "user_id", userID)

	// Extract business_id from JWT context (set by JWTMiddleware)
	businessID, _ := c.Get("bid").(string)
	if businessID == "" {
		h.logger.Warn("Missing bid in JWT context")
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error":   "NO_BUSINESS_CONTEXT",
			"message": "No active business in token",
		})
	}
	h.logger.DebugWith("Business ID extracted", "business_id", businessID)

	h.logger.Infof("Fetching dashboard metrics for business: %s", businessID)

	// Fetch all metrics concurrently via service
	metrics, err := h.dashboardService.GetDashboardMetrics(ctx, businessID)
	if err != nil {
		h.logger.Errorf("Failed to fetch dashboard metrics: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "FETCH_METRICS_ERROR",
			"message": "Failed to fetch dashboard metrics",
		})
	}

	h.logger.Infof("Successfully fetched dashboard metrics for business: %s", businessID)
	return c.JSON(http.StatusOK, metrics)
}
