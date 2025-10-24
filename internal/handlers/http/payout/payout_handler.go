package payout

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/payout"
	"xcentro.in/internal/core/voucher"
	payoutSvc "xcentro.in/internal/services/payout"
	settlementSvc "xcentro.in/internal/services/settlement"
	"xcentro.in/internal/shared/logger"
)

// PayoutHandler handles payout operations dashboard endpoints
type PayoutHandler struct {
	service            *payoutSvc.PayoutService
	transferRepo       payout.PayoutTransferRepository
	configRepo         payout.PayoutConfigurationRepository
	voucherRepo        voucher.VoucherRepository
	settlementService  *settlementSvc.SettlementVerificationService
	logger             *logger.Logger
}

// NewPayoutHandler creates a new payout handler
func NewPayoutHandler(
	service *payoutSvc.PayoutService,
	transferRepo payout.PayoutTransferRepository,
	configRepo payout.PayoutConfigurationRepository,
	voucherRepo voucher.VoucherRepository,
	settlementService *settlementSvc.SettlementVerificationService,
	logger *logger.Logger,
) *PayoutHandler {
	return &PayoutHandler{
		service:           service,
		transferRepo:      transferRepo,
		configRepo:        configRepo,
		voucherRepo:       voucherRepo,
		settlementService: settlementService,
		logger:            logger,
	}
}

// GetPendingApprovals retrieves transfers pending approval
// GET /payout/pending-approvals?limit=50
func (h *PayoutHandler) GetPendingApprovals(c echo.Context) error {
	h.logger.InfoWith("GetPendingApprovals API called")
	ctx := c.Request().Context()

	// Parse limit query parameter
	limit := 50 // default
	if limitStr := c.QueryParam("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	h.logger.DebugWith("Query parameters", "limit", limit)

	// Get pending approvals
	transfers, err := h.transferRepo.GetPendingApprovals(ctx, limit)
	if err != nil {
		h.logger.ErrorWith("Failed to fetch pending approvals", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error":   "failed_to_fetch_pending_approvals",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Pending approvals fetched", "count", len(transfers))
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
	h.logger.InfoWith("ApproveTransfer API called")
	ctx := c.Request().Context()
	transferID := c.Param("transfer_id")

	var req struct {
		ApprovedBy string `json:"approved_by" validate:"required"`
		Remarks    string `json:"remarks"`
	}

	if err := c.Bind(&req); err != nil {
		h.logger.ErrorWith("Error binding request", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	h.logger.DebugWith("Approve transfer request",
		"transfer_id", transferID,
		"approved_by", req.ApprovedBy,
		"has_remarks", req.Remarks != "")

	// Call service to approve transfer
	if err := h.service.ApproveTransfer(ctx, transferID, req.ApprovedBy, req.Remarks); err != nil {
		h.logger.ErrorWith("Failed to approve transfer", "transfer_id", transferID, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error":   "failed_to_approve_transfer",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Transfer approved successfully",
		"transfer_id", transferID,
		"approved_by", req.ApprovedBy)

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
	h.logger.InfoWith("CancelTransfer API called")
	ctx := c.Request().Context()
	transferID := c.Param("transfer_id")

	var req struct {
		CancelledBy string `json:"cancelled_by" validate:"required"`
		Reason      string `json:"reason" validate:"required"`
	}

	if err := c.Bind(&req); err != nil {
		h.logger.ErrorWith("Error binding request", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	h.logger.DebugWith("Cancel transfer request",
		"transfer_id", transferID,
		"cancelled_by", req.CancelledBy,
		"reason", req.Reason)

	// Call service to cancel transfer
	if err := h.service.CancelTransfer(ctx, transferID, req.CancelledBy, req.Reason); err != nil {
		h.logger.ErrorWith("Failed to cancel transfer", "transfer_id", transferID, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error":   "failed_to_cancel_transfer",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Transfer cancelled successfully",
		"transfer_id", transferID,
		"cancelled_by", req.CancelledBy)

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
	h.logger.InfoWith("GetTransferDetails API called")
	ctx := c.Request().Context()
	transferID := c.Param("transfer_id")

	h.logger.DebugWith("Request parameters", "transfer_id", transferID)

	// Get transfer details
	transfer, err := h.service.GetTransferDetails(ctx, transferID)
	if err != nil {
		h.logger.ErrorWith("Transfer not found", "transfer_id", transferID, "error", err)
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"error":   "transfer_not_found",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Transfer details fetched", "transfer_id", transferID)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"data": transfer,
	})
}

// GetWebhookHistory retrieves webhook history for a transfer
// GET /payout/:transfer_id/webhooks
func (h *PayoutHandler) GetWebhookHistory(c echo.Context) error {
	h.logger.InfoWith("GetWebhookHistory API called")
	ctx := c.Request().Context()
	transferID := c.Param("transfer_id")

	h.logger.DebugWith("Request parameters", "transfer_id", transferID)

	// Get webhook history
	webhooks, err := h.service.GetWebhookHistory(ctx, transferID)
	if err != nil {
		h.logger.ErrorWith("Transfer not found", "transfer_id", transferID, "error", err)
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"error":   "transfer_not_found",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Webhook history fetched",
		"transfer_id", transferID,
		"webhook_count", len(webhooks))

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
	h.logger.InfoWith("GetTimeoutTransfers API called")
	ctx := c.Request().Context()

	// Parse limit query parameter
	limit := 50 // default
	if limitStr := c.QueryParam("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	h.logger.DebugWith("Query parameters", "limit", limit)

	// Get timeout transfers
	transfers, err := h.transferRepo.GetWebhookTimeouts(ctx, limit)
	if err != nil {
		h.logger.ErrorWith("Failed to fetch timeout transfers", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error":   "failed_to_fetch_timeouts",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Timeout transfers fetched", "count", len(transfers))
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
	h.logger.InfoWith("MarkCompletedManually API called")
	ctx := c.Request().Context()
	transferID := c.Param("transfer_id")

	var req struct {
		UTR         string `json:"utr" validate:"required"`
		CompletedBy string `json:"completed_by" validate:"required"`
		Remarks     string `json:"remarks"`
	}

	if err := c.Bind(&req); err != nil {
		h.logger.ErrorWith("Error binding request", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	h.logger.DebugWith("Mark completed request",
		"transfer_id", transferID,
		"utr", req.UTR,
		"completed_by", req.CompletedBy,
		"has_remarks", req.Remarks != "")

	// Call service to mark completed
	if err := h.service.MarkCompletedManually(ctx, transferID, req.UTR, req.CompletedBy, req.Remarks); err != nil {
		h.logger.ErrorWith("Failed to mark transfer as completed",
			"transfer_id", transferID,
			"error", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error":   "failed_to_mark_completed",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Transfer marked as completed",
		"transfer_id", transferID,
		"utr", req.UTR,
		"completed_by", req.CompletedBy)

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
	h.logger.InfoWith("GetConfig API called")
	ctx := c.Request().Context()

	// Parse optional business_id query parameter
	var businessID *string
	if bid := c.QueryParam("business_id"); bid != "" {
		businessID = &bid
	}

	h.logger.DebugWith("Query parameters",
		"business_id", func() string {
			if businessID != nil {
				return *businessID
			}
			return "global"
		}())

	// Get configuration
	config, err := h.configRepo.GetConfig(ctx, businessID)
	if err != nil {
		h.logger.ErrorWith("Config not found", "error", err)
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"error":   "config_not_found",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Config fetched successfully")
	return c.JSON(http.StatusOK, map[string]interface{}{
		"data": config,
	})
}

// UpdateConfig updates payout configuration
// PUT /payout/config
// Body: PayoutConfiguration JSON
func (h *PayoutHandler) UpdateConfig(c echo.Context) error {
	h.logger.InfoWith("UpdateConfig API called")
	ctx := c.Request().Context()

	var config payout.PayoutConfiguration
	if err := c.Bind(&config); err != nil {
		h.logger.ErrorWith("Error binding request", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	h.logger.DebugWith("Update config request",
		"config_id", config.ConfigID,
		"auto_approval_limit", config.AutoApprovalLimit)

	// Update configuration
	if err := h.configRepo.UpdateConfig(ctx, &config); err != nil {
		h.logger.ErrorWith("Failed to update config", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error":   "failed_to_update_config",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Config updated successfully", "config_id", config.ConfigID)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Configuration updated successfully",
		"data":    config,
	})
}

// CreateConfig creates a new business-specific configuration
// POST /payout/config
// Body: PayoutConfiguration JSON
func (h *PayoutHandler) CreateConfig(c echo.Context) error {
	h.logger.InfoWith("CreateConfig API called")
	ctx := c.Request().Context()

	var config payout.PayoutConfiguration
	if err := c.Bind(&config); err != nil {
		h.logger.ErrorWith("Error binding request", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	h.logger.DebugWith("Create config request",
		"business_id", func() string {
			if config.BusinessID != nil {
				return *config.BusinessID
			}
			return "global"
		}(),
		"auto_approval_limit", config.AutoApprovalLimit)

	// Create configuration
	if err := h.configRepo.Create(ctx, &config); err != nil {
		h.logger.ErrorWith("Failed to create config", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error":   "failed_to_create_config",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Config created successfully", "config_id", config.ConfigID)
	return c.JSON(http.StatusCreated, map[string]interface{}{
		"message": "Configuration created successfully",
		"data":    config,
	})
}

// InitiatePayoutManually manually triggers payout for a voucher (MVP: Ops button)
// POST /payout/initiate
// Body: {"voucher_id": "uuid", "initiated_by": "user_uuid", "bypass_checks": false, "remarks": "optional"}
func (h *PayoutHandler) InitiatePayoutManually(c echo.Context) error {
	h.logger.InfoWith("InitiatePayoutManually API called")
	ctx := c.Request().Context()

	var req struct {
		VoucherID    string `json:"voucher_id" validate:"required"`
		InitiatedBy  string `json:"initiated_by" validate:"required"`
		BypassChecks bool   `json:"bypass_checks"` // Admin override for emergency payouts
		Remarks      string `json:"remarks"`
	}

	if err := c.Bind(&req); err != nil {
		h.logger.ErrorWith("Error binding request", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	h.logger.DebugWith("Manual payout initiation request",
		"voucher_id", req.VoucherID,
		"initiated_by", req.InitiatedBy,
		"bypass_checks", req.BypassChecks)

	// Step 1: Pre-flight validation (unless bypassed)
	if !req.BypassChecks {
		h.logger.DebugWith("Running pre-flight validation")
		if err := h.validateVoucherForPayout(ctx, req.VoucherID); err != nil {
			h.logger.WarnWith("Pre-flight validation failed", "error", err)
			return c.JSON(http.StatusBadRequest, map[string]interface{}{
				"error":   "validation_failed",
				"message": err.Error(),
			})
		}
		h.logger.DebugWith("Pre-flight validation passed")
	} else {
		h.logger.WarnWith("Pre-flight checks bypassed (admin override)",
			"initiated_by", req.InitiatedBy,
			"remarks", req.Remarks)
	}

	// Step 2: Optional settlement verification (MVP: always passes)
	if !req.BypassChecks && h.settlementService != nil {
		h.logger.DebugWith("Running settlement verification")

		// Get voucher to extract payment_id
		voucherUUID, _ := uuid.Parse(req.VoucherID)
		vch, err := h.voucherRepo.GetByID(ctx, voucherUUID)
		if err == nil && vch.PaymentID != nil {
			verified, discrepancy, err := h.settlementService.VerifySettlement(
				ctx,
				req.VoucherID,
				vch.PaymentID.String(),
			)

			if err != nil {
				h.logger.ErrorWith("Settlement verification error", "error", err)
				// Don't block - just log for MVP
			} else if !verified {
				h.logger.WarnWith("Settlement verification failed (discrepancy detected)",
					"voucher_id", req.VoucherID,
					"discrepancy", discrepancy)
				// MVP: Log warning but allow ops to proceed
			} else {
				h.logger.DebugWith("Settlement verification passed")
			}
		}
	}

	// Step 3: Call the SAME service method that worker would call
	h.logger.DebugWith("Calling PayoutService.InitiatePayoutForVoucher")
	transfer, err := h.service.InitiatePayoutForVoucher(ctx, req.VoucherID)
	if err != nil {
		h.logger.ErrorWith("Failed to initiate payout", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error":   "failed_to_initiate_payout",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Payout initiated successfully",
		"transfer_request_id", transfer.TransferRequestID,
		"voucher_id", req.VoucherID,
		"amount", transfer.Amount,
		"status", transfer.Status,
		"initiated_by", req.InitiatedBy)

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"message": "Payout initiated successfully",
		"status":  "success",
		"data": map[string]interface{}{
			"transfer_request_id": transfer.TransferRequestID,
			"voucher_id":          req.VoucherID,
			"amount":              transfer.Amount,
			"currency":            transfer.Currency,
			"status":              transfer.Status,
			"requires_approval":   transfer.RequiresApproval,
			"next_action": func() string {
				if transfer.RequiresApproval {
					return "Transfer pending approval - use POST /payout/:transfer_id/approve"
				}
				return "Transfer auto-approved - worker will initiate to Castler"
			}(),
		},
	})
}

// validateVoucherForPayout performs pre-flight checks before initiating payout
func (h *PayoutHandler) validateVoucherForPayout(ctx context.Context, voucherID string) error {
	h.logger.DebugWith("Validating voucher for payout", "voucher_id", voucherID)

	// Parse UUID
	voucherUUID, err := uuid.Parse(voucherID)
	if err != nil {
		return fmt.Errorf("invalid voucher_id format: %w", err)
	}

	// Get voucher
	vch, err := h.voucherRepo.GetByID(ctx, voucherUUID)
	if err != nil {
		return fmt.Errorf("voucher not found: %w", err)
	}

	h.logger.DebugWith("Voucher retrieved",
		"voucher_number", vch.VoucherNumber,
		"status", vch.Status,
		"payout_status", vch.PayoutStatus,
		"cancellation_status", vch.CancellationStatus)

	// Check 1: Voucher must be finalized
	if vch.Status != voucher.StatusFinalized {
		return fmt.Errorf("voucher not finalized (current status: %s)", vch.Status)
	}

	// Check 2: Must have linked payment
	if vch.PaymentID == nil {
		return fmt.Errorf("voucher has no linked payment (still in draft or payment failed)")
	}

	// Check 3: Must not be cancelled
	if vch.CancellationStatus != voucher.CancellationStatusNotCancelled {
		return fmt.Errorf("voucher is cancelled (cancellation_status: %s)", vch.CancellationStatus)
	}

	// Check 4: Payout must be pending (not already initiated/completed)
	if vch.PayoutStatus != "" && voucher.PayoutStatus(vch.PayoutStatus) != voucher.PayoutStatusPending {
		return fmt.Errorf("payout already initiated (payout_status: %s)", vch.PayoutStatus)
	}

	// Check 5: No existing transfer request (idempotency)
	existing, err := h.transferRepo.GetByVoucherID(ctx, voucherID)
	if err == nil && existing != nil {
		return fmt.Errorf("transfer already exists (transfer_id: %s, status: %s)",
			existing.TransferRequestID, existing.Status)
	}

	h.logger.DebugWith("All validation checks passed")
	return nil
}
