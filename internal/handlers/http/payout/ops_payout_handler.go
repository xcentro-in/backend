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
	payoutListService *payoutSvc.OpsPayoutService // For listing endpoints
	payoutService     *payoutSvc.PayoutService    // For operations (initiate, etc.)
	logger            *logger.Logger
}

// NewOpsPayoutHandler creates a new ops payout handler instance
func NewOpsPayoutHandler(
	payoutListService *payoutSvc.OpsPayoutService,
	payoutService *payoutSvc.PayoutService,
) *OpsPayoutHandler {
	return &OpsPayoutHandler{
		payoutListService: payoutListService,
		payoutService:     payoutService,
		logger:            logger.GetDefault(),
	}
}

// RegisterOpsPayoutRoutes registers all ops dashboard payout routes
func RegisterOpsPayoutRoutes(
	e *echo.Group,
	payoutListService *payoutSvc.OpsPayoutService,
	payoutService *payoutSvc.PayoutService,
) {
	handler := NewOpsPayoutHandler(payoutListService, payoutService)

	payoutGroup := e.Group("/payouts")

	// Listing endpoints
	payoutGroup.GET("", handler.ListPayouts)
	payoutGroup.GET("/summary/:id", handler.GetPayoutSummary)
	payoutGroup.GET("/:id", handler.GetPayoutDetail)

	// Operations endpoints
	payoutGroup.POST("/initiate_payout", handler.InitiatePayoutManually)
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
	response, err := h.payoutListService.ListPayouts(c.Request().Context(), req.Page, req.Limit, string(req.Status))
	if err != nil {
		h.logger.Errorf("Error listing payouts: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to fetch payouts",
		})
	}

	return c.JSON(http.StatusOK, response)
}

// GetPayoutSummary handles GET /payouts/summary/:id
// Returns detailed summary of a single payout transfer
func (h *OpsPayoutHandler) GetPayoutSummary(c echo.Context) error {
	transferRequestID := c.Param("id")
	if transferRequestID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "transfer_request_id is required",
		})
	}

	// Call service layer
	summary, err := h.payoutListService.GetPayoutSummary(c.Request().Context(), transferRequestID)
	if err != nil {
		h.logger.Errorf("Error getting payout summary: %v", err)
		if err.Error() == "payout transfer not found: "+transferRequestID {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "Payout transfer not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to fetch payout summary",
		})
	}

	return c.JSON(http.StatusOK, summary)
}

// GetPayoutDetail handles GET /payouts/:id
// Returns complete detailed information for a payout transfer including voucher, payee, collection, provider, and retry history
func (h *OpsPayoutHandler) GetPayoutDetail(c echo.Context) error {
	transferRequestID := c.Param("id")
	if transferRequestID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "transfer_request_id is required",
		})
	}

	// Call service layer
	detail, err := h.payoutListService.GetPayoutDetail(c.Request().Context(), transferRequestID)
	if err != nil {
		h.logger.Errorf("Error getting payout detail: %v", err)
		if err.Error() == "payout transfer not found: "+transferRequestID {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "Payout transfer not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to fetch payout detail",
		})
	}

	return c.JSON(http.StatusOK, detail)
}

// InitiatePayoutManually handles POST /payouts/initiate_payout
// Manually triggers payout for a payment - processes all vouchers or specific ones
func (h *OpsPayoutHandler) InitiatePayoutManually(c echo.Context) error {
	ctx := c.Request().Context()

	// Bind and validate request
	var req struct {
		PaymentID    string   `json:"payment_id" validate:"required,uuid"`
		VoucherIDs   []string `json:"voucher_ids"` // Optional - empty means all unpaid vouchers
		InitiatedBy  string   `json:"initiated_by" validate:"required,uuid"`
		BypassChecks bool     `json:"bypass_checks"` // Admin override for emergency payouts
		Remarks      string   `json:"remarks"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
	}

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

	h.logger.InfoWith("Ops: Initiating manual payout",
		"payment_id", req.PaymentID,
		"voucher_count", len(req.VoucherIDs),
		"initiated_by", req.InitiatedBy,
		"bypass_checks", req.BypassChecks)

	// Call service to initiate payout and process async
	result, err := h.payoutService.InitiateAndProcessPayout(ctx, req.PaymentID, req.VoucherIDs, req.InitiatedBy, req.BypassChecks)
	if err != nil {
		h.logger.ErrorWith("Failed to initiate payout", "payment_id", req.PaymentID, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":   "failed_to_initiate_payout",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Payout initiated successfully",
		"payment_id", result.PaymentID,
		"transfer_count", result.TransferCount,
		"total_amount", result.TotalAmount)

	return c.JSON(http.StatusCreated, result)
}
