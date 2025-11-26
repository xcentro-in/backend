package payout

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/payment"
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
	providerFactory    payoutSvc.PayoutProviderFactory
	stateChangeRepo    payment.PaymentStateChangeRepository
	logger             *logger.Logger
}

// NewPayoutHandler creates a new payout handler
func NewPayoutHandler(
	service *payoutSvc.PayoutService,
	transferRepo payout.PayoutTransferRepository,
	configRepo payout.PayoutConfigurationRepository,
	voucherRepo voucher.VoucherRepository,
	settlementService *settlementSvc.SettlementVerificationService,
	providerFactory payoutSvc.PayoutProviderFactory,
	stateChangeRepo payment.PaymentStateChangeRepository,
	logger *logger.Logger,
) *PayoutHandler {
	return &PayoutHandler{
		service:           service,
		transferRepo:      transferRepo,
		configRepo:        configRepo,
		voucherRepo:       voucherRepo,
		settlementService: settlementService,
		providerFactory:   providerFactory,
		stateChangeRepo:   stateChangeRepo,
		logger:            logger,
	}
}

// GetPendingApprovals retrieves transfers pending approval
// GET /payout/pending-approvals?limit=50
// DEPRECATED: Approval workflow removed in payout redesign
func (h *PayoutHandler) GetPendingApprovals(c echo.Context) error {
	h.logger.WarnWith("GetPendingApprovals API called - DEPRECATED")
	return c.JSON(http.StatusGone, map[string]interface{}{
		"error":   "feature_removed",
		"message": "Approval workflow has been removed - all payouts are auto-initiated after settlement",
	})
}

// ApproveTransfer approves a pending transfer
// POST /payout/:transfer_id/approve
// DEPRECATED: Approval workflow removed in payout redesign
func (h *PayoutHandler) ApproveTransfer(c echo.Context) error {
	h.logger.WarnWith("ApproveTransfer API called - DEPRECATED")
	return c.JSON(http.StatusGone, map[string]interface{}{
		"error":   "feature_removed",
		"message": "Approval workflow has been removed - all payouts are auto-initiated after settlement",
	})
}

// CancelTransfer cancels a pending transfer
// POST /payout/:transfer_id/cancel
// DEPRECATED: Cancellation workflow removed in payout redesign
func (h *PayoutHandler) CancelTransfer(c echo.Context) error {
	h.logger.WarnWith("CancelTransfer API called - DEPRECATED")
	return c.JSON(http.StatusGone, map[string]interface{}{
		"error":   "feature_removed",
		"message": "Cancellation workflow has been removed - contact support for payout issues",
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

// InitiatePayoutManually manually triggers payout for a payment (MVP: Ops button)
// POST /payout/initiate
// Body: {"payment_id": "uuid", "voucher_ids": ["uuid1", "uuid2"], "initiated_by": "user_uuid", "bypass_checks": false, "remarks": "optional"}
// voucher_ids is optional - if empty, processes all unpaid vouchers for the payment
func (h *PayoutHandler) InitiatePayoutManually(c echo.Context) error {
	h.logger.InfoWith("InitiatePayoutManually API called")
	ctx := c.Request().Context()

	var req struct {
		PaymentID    string   `json:"payment_id" validate:"required"`
		VoucherIDs   []string `json:"voucher_ids"` // Optional - empty means all unpaid vouchers
		InitiatedBy  string   `json:"initiated_by" validate:"required"`
		BypassChecks bool     `json:"bypass_checks"` // Admin override for emergency payouts
		Remarks      string   `json:"remarks"`
	}

	if err := c.Bind(&req); err != nil {
		h.logger.ErrorWith("Error binding request", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	h.logger.DebugWith("Manual payout initiation request",
		"payment_id", req.PaymentID,
		"voucher_ids", req.VoucherIDs,
		"initiated_by", req.InitiatedBy,
		"bypass_checks", req.BypassChecks)

	// Step 1: Optional settlement verification (MVP: always passes)
	if !req.BypassChecks && h.settlementService != nil {
		h.logger.DebugWith("Running settlement verification")
		verified, discrepancy, err := h.settlementService.VerifySettlement(ctx, "", req.PaymentID)
		if err != nil {
			h.logger.ErrorWith("Settlement verification error", "error", err)
			// Don't block - just log for MVP
		} else if !verified {
			h.logger.WarnWith("Settlement verification failed (discrepancy detected)",
				"payment_id", req.PaymentID,
				"discrepancy", discrepancy)
			// MVP: Log warning but allow ops to proceed
		} else {
			h.logger.DebugWith("Settlement verification passed")
		}
	}

	// Step 2: Call service to create transfer requests (status='initiated')
	// Pass bypass_checks flag to allow settlement status override if needed
	h.logger.DebugWith("Calling PayoutService.InitiateManualPayout")
	transfers, err := h.service.InitiateManualPayout(ctx, req.PaymentID, req.VoucherIDs, req.BypassChecks)
	if err != nil {
		h.logger.ErrorWith("Failed to initiate payout", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error":   "failed_to_initiate_payout",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Payout transfer requests created",
		"payment_id", req.PaymentID,
		"count", len(transfers),
		"initiated_by", req.InitiatedBy)

	// Step 3: IMMEDIATELY trigger async processing (no worker delay)
	go h.processPayoutsAsync(context.Background(), transfers)

	h.logger.InfoWith("Async payout processing triggered", "count", len(transfers))

	// Step 4: Return success immediately
	transferIDs := make([]string, len(transfers))
	totalAmount := int64(0)
	for i, t := range transfers {
		transferIDs[i] = t.TransferRequestID
		totalAmount += t.Amount
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"message": "Payout initiated - processing in background",
		"status":  "success",
		"data": map[string]interface{}{
			"payment_id":         req.PaymentID,
			"transfer_count":     len(transfers),
			"transfer_ids":       transferIDs,
			"total_amount":       totalAmount,
			"currency":           "INR",
			"status":             "initiated",
			"next_action":        "Castler API calls in progress - check status via transfer_ids",
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
		"status", vch.Status)

	// Check 1: Voucher must be finalized
	if vch.Status != voucher.StatusFinalized {
		return fmt.Errorf("voucher not finalized (current status: %s)", vch.Status)
	}

	// Check 2: Must have linked payment
	if vch.PaymentID == nil {
		return fmt.Errorf("voucher has no linked payment (still in draft or payment failed)")
	}

	// Check 3: No existing transfer request (idempotency check)
	existing, err := h.transferRepo.GetByVoucherID(ctx, voucherID)
	if err == nil && existing != nil {
		return fmt.Errorf("transfer already exists (transfer_id: %s, status: %s)",
			existing.TransferRequestID, existing.Status)
	}

	h.logger.DebugWith("All validation checks passed")
	return nil
}

// processPayoutsAsync processes multiple payout transfers in parallel
// Runs in background goroutine - calls Castler API for each transfer independently
func (h *PayoutHandler) processPayoutsAsync(ctx context.Context, transfers []*payout.PayoutTransferRequest) {
	h.logger.InfoWith("processPayoutsAsync started", "count", len(transfers))

	// Process all transfers in parallel
	for _, transfer := range transfers {
		// Launch goroutine for each transfer (independent processing)
		go h.processSinglePayout(ctx, transfer)
	}

	h.logger.InfoWith("All payout goroutines launched", "count", len(transfers))
}

// processSinglePayout processes a single payout transfer
// Called in goroutine - updates status to 'pending' on success or 'failed' on error
func (h *PayoutHandler) processSinglePayout(ctx context.Context, transfer *payout.PayoutTransferRequest) {
	h.logger.InfoWith("Processing single payout transfer",
		"transfer_id", transfer.TransferRequestID,
		"voucher_id", transfer.VoucherID,
		"amount", transfer.Amount)

	// Parse UUIDs for activity tracking
	transferUUID, err := uuid.Parse(transfer.TransferRequestID)
	if err != nil {
		h.logger.ErrorWith("Invalid transfer_request_id UUID", "error", err)
		return
	}

	paymentUUID, err := uuid.Parse(transfer.PaymentID)
	if err != nil {
		h.logger.ErrorWith("Invalid payment_id UUID", "error", err)
		return
	}

	// Record activity: Payout initiated
	now := time.Now()
	_, err = h.stateChangeRepo.Create(ctx, &payment.CreatePaymentStateChangeRequest{
		PaymentID:       paymentUUID,
		TransferID:      &transferUUID,
		Action:          payment.PayoutActionInitiated,
		ActionTimestamp: now,
		ActorType:       payment.ActorTypeSystem,
		ActorName:       "payout_handler",
		PreviousState:   "",
		NewState:        "initiated",
		Amount:          transfer.Amount,
		Currency:        transfer.Currency,
		Provider:        string(transfer.Provider),
		Metadata: map[string]any{
			"transfer_mode": transfer.TransferMode,
			"purpose":       transfer.Purpose,
		},
		Remarks: fmt.Sprintf("Manual payout initiated for transfer %s", transfer.TransferRequestID),
	})
	if err != nil {
		h.logger.ErrorWith("Failed to create activity record (initiated)", "error", err)
		// Continue processing even if activity tracking fails
	}

	// Get provider
	provider, err := h.providerFactory.GetProvider(transfer.Provider)
	if err != nil {
		h.logger.ErrorWith("Failed to get provider", "error", err, "transfer_id", transfer.TransferRequestID)
		h.markTransferFailed(ctx, transfer, transferUUID, paymentUUID, fmt.Sprintf("Failed to get provider: %v", err))
		return
	}

	// Call Castler payout API
	createReq := &payout.CreateTransferRequest{
		TransferRequestID:     transfer.TransferRequestID,
		VoucherID:             transfer.VoucherID,
		ProviderBeneficiaryID: transfer.ProviderBeneficiaryID,
		EscrowID:              transfer.EscrowID,
		Amount:                transfer.Amount,
		Currency:              transfer.Currency,
		Purpose:               transfer.Purpose,
		Reference:             transfer.Reference,
		Narration:             transfer.Narration,
		TransferMode:          transfer.TransferMode,
	}

	resp, err := provider.CreateTransferRequest(ctx, createReq)
	if err != nil {
		h.logger.ErrorWith("Castler API error",
			"error", err,
			"transfer_id", transfer.TransferRequestID)
		h.markTransferFailed(ctx, transfer, transferUUID, paymentUUID, fmt.Sprintf("Castler API error: %v", err))
		return
	}

	h.logger.InfoWith("Castler API success",
		"transfer_id", transfer.TransferRequestID,
		"provider_transfer_id", resp.ProviderTransferID)

	// Update status to 'pending' (Castler API success)
	pendingTime := time.Now()
	if err := h.transferRepo.UpdateStatus(ctx, transfer.TransferRequestID, payout.TransferStatusPending, pendingTime); err != nil {
		h.logger.ErrorWith("Failed to update status to pending", "error", err)
		return
	}

	// Store Castler's provider transfer ID
	if err := h.transferRepo.UpdateProviderTransferID(ctx, transfer.TransferRequestID, resp.ProviderTransferID); err != nil {
		h.logger.ErrorWith("Failed to update provider transfer ID", "error", err)
		return
	}

	// Record activity: Payout pending (API success = pending status)
	_, err = h.stateChangeRepo.Create(ctx, &payment.CreatePaymentStateChangeRequest{
		PaymentID:       paymentUUID,
		TransferID:      &transferUUID,
		Action:          payment.PayoutActionProcessing, // Use 'processing' action for 'pending' state
		ActionTimestamp: pendingTime,
		ActorType:       payment.ActorTypeSystem,
		ActorName:       "payout_handler",
		PreviousState:   "initiated",
		NewState:        "pending",
		Amount:          transfer.Amount,
		Currency:        transfer.Currency,
		Provider:        string(transfer.Provider),
		Metadata: map[string]any{
			"provider_transfer_id": resp.ProviderTransferID,
			"transfer_mode":        transfer.TransferMode,
		},
		Remarks: fmt.Sprintf("Castler API success - now pending settlement (Castler transferId: %s)", resp.ProviderTransferID),
	})
	if err != nil {
		h.logger.ErrorWith("Failed to create activity record (pending)", "error", err)
	}

	// Set webhook timeout
	config, err := h.configRepo.GetConfig(ctx, nil)
	if err == nil {
		timeoutAt := pendingTime.Add(time.Duration(config.WebhookTimeoutMinutes) * time.Minute)
		if err := h.transferRepo.SetWebhookTimeout(ctx, transfer.TransferRequestID, timeoutAt); err != nil {
			h.logger.ErrorWith("Failed to set webhook timeout", "error", err)
		}
	}

	h.logger.InfoWith("Payout transfer processed successfully",
		"transfer_id", transfer.TransferRequestID,
		"status", "pending")
}

// markTransferFailed updates transfer status to failed and records activity
func (h *PayoutHandler) markTransferFailed(ctx context.Context, transfer *payout.PayoutTransferRequest, transferUUID, paymentUUID uuid.UUID, reason string) {
	failedTime := time.Now()

	// Update status to failed
	if err := h.transferRepo.MarkFailed(ctx, transfer.TransferRequestID, reason, failedTime); err != nil {
		h.logger.ErrorWith("Failed to mark transfer as failed", "error", err)
		return
	}

	// Record activity: Payout failed
	_, err := h.stateChangeRepo.Create(ctx, &payment.CreatePaymentStateChangeRequest{
		PaymentID:       paymentUUID,
		TransferID:      &transferUUID,
		Action:          payment.PayoutActionFailed,
		ActionTimestamp: failedTime,
		ActorType:       payment.ActorTypeSystem,
		ActorName:       "payout_handler",
		PreviousState:   "initiated",
		NewState:        "failed",
		Amount:          transfer.Amount,
		Currency:        transfer.Currency,
		Provider:        string(transfer.Provider),
		Metadata: map[string]any{
			"failure_reason": reason,
		},
		Remarks: fmt.Sprintf("Payout failed: %s", reason),
	})
	if err != nil {
		h.logger.ErrorWith("Failed to create activity record (failed)", "error", err)
	}

	h.logger.ErrorWith("Transfer marked as failed",
		"transfer_id", transfer.TransferRequestID,
		"reason", reason)
}
