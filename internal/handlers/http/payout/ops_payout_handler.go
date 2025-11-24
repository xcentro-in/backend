package payout

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/payout"
	payoutSvc "xcentro.in/internal/services/payout"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/validator"
)

// OpsPayoutHandler handles ops dashboard payout-related endpoints
type OpsPayoutHandler struct {
	payoutService *payoutSvc.OpsPayoutService
	logger        *logger.Logger
}

// NewOpsPayoutHandler creates a new ops payout handler instance
func NewOpsPayoutHandler(payoutService *payoutSvc.OpsPayoutService) *OpsPayoutHandler {
	return &OpsPayoutHandler{
		payoutService: payoutService,
		logger:        logger.GetDefault(),
	}
}

// RegisterOpsPayoutRoutes registers all ops dashboard payout routes
func RegisterOpsPayoutRoutes(e *echo.Group, payoutService *payoutSvc.OpsPayoutService) {
	handler := NewOpsPayoutHandler(payoutService)

	payoutGroup := e.Group("/payouts")

	// Endpoint: Paginated list of payout transfers
	payoutGroup.GET("", handler.ListPayouts)
}

// ListPayouts handles GET /payouts?page=1&limit=20&status=in_progress|completed|failed
func (h *OpsPayoutHandler) ListPayouts(c echo.Context) error {
	var req payout.OpsListPayoutsRequest

	// Bind query parameters
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request parameters",
		})
	}

	// Set defaults
	if req.Page == 0 {
		req.Page = 1
	}
	if req.Limit == 0 {
		req.Limit = 20
	}

	// Trim and validate
	validator.TrimStrings(&req)
	if err := c.Validate(&req); err != nil {
		if ve, ok := err.(*validator.ValidationError); ok {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{
				"error":  "Validation failed",
				"errors": ve.Errors,
			})
		}
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
	}

	// Call service layer
	response, err := h.payoutService.ListPayouts(c.Request().Context(), req.Page, req.Limit, string(req.Status))
	if err != nil {
		h.logger.Errorf("Error listing payouts: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to fetch payouts",
		})
	}

	return c.JSON(http.StatusOK, response)
}
