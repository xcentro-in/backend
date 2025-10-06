package payout

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/payout"
	payoutSvc "xcentro.in/internal/services/payout"
)

// PayoutHandler handles payout operations dashboard endpoints
type PayoutHandler struct {
	service      *payoutSvc.PayoutService
	transferRepo payout.PayoutTransferRepository
	configRepo   payout.PayoutConfigurationRepository
}

// NewPayoutHandler creates a new payout handler
func NewPayoutHandler(
	service *payoutSvc.PayoutService,
	transferRepo payout.PayoutTransferRepository,
	configRepo payout.PayoutConfigurationRepository,
) *PayoutHandler {
	return &PayoutHandler{
		service:      service,
		transferRepo: transferRepo,
		configRepo:   configRepo,
	}
}

// GetPendingApprovals retrieves transfers pending approval
// GET /payout/pending-approvals?limit=50
func (h *PayoutHandler) GetPendingApprovals(c echo.Context) error {
	ctx := c.Request().Context()

	// Parse limit query parameter
	limit := 50 // default
	if limitStr := c.QueryParam("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	// Get pending approvals
	transfers, err := h.transferRepo.GetPendingApprovals(ctx, limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error":   "failed_to_fetch_pending_approvals",
			"message": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data": transfers,
		"meta": map[string]interface{}{
			"count": len(transfers),
			"limit": limit,
		},
	})
}

// ApproveTransfer approves a pending transfer
// POST /payout/:transfer_id/approve
// Body: {"approved_by": "user_uuid", "remarks": "optional remarks"}
func (h *PayoutHandler) ApproveTransfer(c echo.Context) error {
	ctx := c.Request().Context()
	transferID := c.Param("transfer_id")

	var req struct {
		ApprovedBy string `json:"approved_by" validate:"required"`
		Remarks    string `json:"remarks"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	// Call service to approve transfer
	if err := h.service.ApproveTransfer(ctx, transferID, req.ApprovedBy, req.Remarks); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error":   "failed_to_approve_transfer",
			"message": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Transfer approved successfully",
		"data": map[string]interface{}{
			"transfer_id": transferID,
			"approved_by": req.ApprovedBy,
		},
	})
}

// CancelTransfer cancels a pending transfer
// POST /payout/:transfer_id/cancel
// Body: {"cancelled_by": "user_uuid", "reason": "cancellation reason"}
func (h *PayoutHandler) CancelTransfer(c echo.Context) error {
	ctx := c.Request().Context()
	transferID := c.Param("transfer_id")

	var req struct {
		CancelledBy string `json:"cancelled_by" validate:"required"`
		Reason      string `json:"reason" validate:"required"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	// Call service to cancel transfer
	if err := h.service.CancelTransfer(ctx, transferID, req.CancelledBy, req.Reason); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error":   "failed_to_cancel_transfer",
			"message": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Transfer cancelled successfully",
		"data": map[string]interface{}{
			"transfer_id":  transferID,
			"cancelled_by": req.CancelledBy,
			"reason":       req.Reason,
		},
	})
}

// GetTransferDetails retrieves details for a specific transfer
// GET /payout/:transfer_id
func (h *PayoutHandler) GetTransferDetails(c echo.Context) error {
	ctx := c.Request().Context()
	transferID := c.Param("transfer_id")

	// Get transfer details
	transfer, err := h.service.GetTransferDetails(ctx, transferID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"error":   "transfer_not_found",
			"message": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data": transfer,
	})
}

// GetWebhookHistory retrieves webhook history for a transfer
// GET /payout/:transfer_id/webhooks
func (h *PayoutHandler) GetWebhookHistory(c echo.Context) error {
	ctx := c.Request().Context()
	transferID := c.Param("transfer_id")

	// Get webhook history
	webhooks, err := h.service.GetWebhookHistory(ctx, transferID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"error":   "transfer_not_found",
			"message": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data": webhooks,
		"meta": map[string]interface{}{
			"count": len(webhooks),
		},
	})
}

// GetTimeoutTransfers retrieves transfers that exceeded webhook timeout
// GET /payout/timeouts?limit=50
func (h *PayoutHandler) GetTimeoutTransfers(c echo.Context) error {
	ctx := c.Request().Context()

	// Parse limit query parameter
	limit := 50 // default
	if limitStr := c.QueryParam("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	// Get timeout transfers
	transfers, err := h.transferRepo.GetWebhookTimeouts(ctx, limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error":   "failed_to_fetch_timeouts",
			"message": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data": transfers,
		"meta": map[string]interface{}{
			"count": len(transfers),
			"limit": limit,
		},
	})
}

// MarkCompletedManually manually marks a transfer as completed
// POST /payout/:transfer_id/mark-completed
// Body: {"utr": "bank_utr", "completed_by": "user_uuid", "remarks": "optional remarks"}
func (h *PayoutHandler) MarkCompletedManually(c echo.Context) error {
	ctx := c.Request().Context()
	transferID := c.Param("transfer_id")

	var req struct {
		UTR         string `json:"utr" validate:"required"`
		CompletedBy string `json:"completed_by" validate:"required"`
		Remarks     string `json:"remarks"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	// Call service to mark completed
	if err := h.service.MarkCompletedManually(ctx, transferID, req.UTR, req.CompletedBy, req.Remarks); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error":   "failed_to_mark_completed",
			"message": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Transfer marked as completed",
		"data": map[string]interface{}{
			"transfer_id":  transferID,
			"utr":          req.UTR,
			"completed_by": req.CompletedBy,
		},
	})
}

// GetConfig retrieves payout configuration
// GET /payout/config?business_id=uuid (optional)
func (h *PayoutHandler) GetConfig(c echo.Context) error {
	ctx := c.Request().Context()

	// Parse optional business_id query parameter
	var businessID *string
	if bid := c.QueryParam("business_id"); bid != "" {
		businessID = &bid
	}

	// Get configuration
	config, err := h.configRepo.GetConfig(ctx, businessID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"error":   "config_not_found",
			"message": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data": config,
	})
}

// UpdateConfig updates payout configuration
// PUT /payout/config
// Body: PayoutConfiguration JSON
func (h *PayoutHandler) UpdateConfig(c echo.Context) error {
	ctx := c.Request().Context()

	var config payout.PayoutConfiguration
	if err := c.Bind(&config); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	// Update configuration
	if err := h.configRepo.UpdateConfig(ctx, &config); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error":   "failed_to_update_config",
			"message": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Configuration updated successfully",
		"data":    config,
	})
}

// CreateConfig creates a new business-specific configuration
// POST /payout/config
// Body: PayoutConfiguration JSON
func (h *PayoutHandler) CreateConfig(c echo.Context) error {
	ctx := c.Request().Context()

	var config payout.PayoutConfiguration
	if err := c.Bind(&config); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	// Create configuration
	if err := h.configRepo.Create(ctx, &config); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error":   "failed_to_create_config",
			"message": err.Error(),
		})
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"message": "Configuration created successfully",
		"data":    config,
	})
}
