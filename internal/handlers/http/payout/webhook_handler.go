package payout

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/payment"
	"xcentro.in/internal/core/payout"
	"xcentro.in/internal/shared/logger"
)

// PayoutWebhookHandler handles webhook callbacks from payout providers
type PayoutWebhookHandler struct {
	transferRepo    payout.PayoutTransferRepository
	stateChangeRepo payment.PaymentStateChangeRepository
	webhookSecret   string // Castler webhook secret for signature verification
	logger          *logger.Logger
}

// NewPayoutWebhookHandler creates a new webhook handler
func NewPayoutWebhookHandler(
	transferRepo payout.PayoutTransferRepository,
	stateChangeRepo payment.PaymentStateChangeRepository,
	webhookSecret string,
) *PayoutWebhookHandler {
	return &PayoutWebhookHandler{
		transferRepo:    transferRepo,
		stateChangeRepo: stateChangeRepo,
		webhookSecret:   webhookSecret,
		logger:          logger.GetDefault(),
	}
}

// CastlerWebhookPayload represents the actual webhook payload from Castler
type CastlerWebhookPayload struct {
	// Core transfer info
	TransferID string  `json:"transferId"` // Castler's transfer ID
	Status     string  `json:"status"`     // "success" or "failed"
	UTR        string  `json:"utr"`        // Bank UTR (on success)
	Remarks    string  `json:"remarks"`    // "success" or failure reason
	TimeStamp  int64   `json:"timeStamp"`  // Unix timestamp in milliseconds

	// Transfer details
	Amount float64 `json:"amount"` // Transfer amount (in rupees, not paise)
	Mode   string  `json:"mode"`   // "imps", "neft", "rtgs"
	Type   string  `json:"type"`   // "single" or "bulk"

	// Beneficiary details
	BeneficiaryName       string `json:"beneficiaryName"`
	BeneficiaryAccNumber  string `json:"beneficiaryAccNumber"`
	BeneficiaryIFSC       string `json:"beneficiaryIFSC"`
	BeneficiaryBankName   string `json:"beneficiaryBankName"`

	// Account details
	AccountNumber    string  `json:"accountNumber"`    // Our account number
	Bank             string  `json:"bank"`             // Our bank ("rbl")
	EffectiveBalance float64 `json:"effectiveBalance"` // Balance after transfer
	AccountBalance   float64 `json:"accountBalance"`   // Current balance

	// Bulk transfer (empty for single transfers)
	BulkTransferID string `json:"bulkTransferId"`
}

// HandleCastlerWebhook processes webhook callbacks from Castler
func (h *PayoutWebhookHandler) HandleCastlerWebhook(c echo.Context) error {
	h.logger.InfoWith("Castler webhook received")

	// Step 1: Parse webhook payload (no signature verification - Castler sends plain POST)
	var payload CastlerWebhookPayload
	if err := c.Bind(&payload); err != nil {
		h.logger.ErrorWith("Failed to parse webhook payload", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid_payload",
			"message": "Failed to parse webhook payload",
		})
	}

	h.logger.InfoWith("Castler webhook parsed",
		"transfer_id", payload.TransferID,
		"status", payload.Status,
		"utr", payload.UTR,
		"amount", payload.Amount,
		"mode", payload.Mode)

	// Get context early for all operations
	ctx := c.Request().Context()

	// Step 2: Convert timestamp from epoch millis to time.Time
	webhookTime := time.UnixMilli(payload.TimeStamp)

	// Step 3: Find transfer request by provider_transfer_id
	transfer, err := h.transferRepo.GetByProviderTransferID(ctx, payload.TransferID)
	if err != nil {
		h.logger.ErrorWith("Transfer not found",
			"provider_transfer_id", payload.TransferID,
			"error", err)
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "transfer_not_found",
			"message": fmt.Sprintf("Transfer not found: %s", payload.TransferID),
		})
	}

	h.logger.InfoWith("Transfer found",
		"transfer_request_id", transfer.TransferRequestID,
		"voucher_id", transfer.VoucherID,
		"webhook_time", webhookTime)

	// Step 4: Handle event based on status
	// Castler only sends settlement webhooks (success/failed)
	// Processing status is set when API call succeeds (not via webhook)

	switch payload.Status {
	case "success":
		if err := h.handleCompleted(ctx, transfer, payload, webhookTime); err != nil {
			h.logger.ErrorWith("Failed to handle success event", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "completion_failed",
				"message": err.Error(),
			})
		}

	case "failed":
		if err := h.handleFailed(ctx, transfer, payload, webhookTime); err != nil {
			h.logger.ErrorWith("Failed to handle failed event", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "failure_handling_failed",
				"message": err.Error(),
			})
		}

	default:
		h.logger.WarnWith("Unknown webhook status", "status", payload.Status)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "unknown_status",
			"message": fmt.Sprintf("Unknown status: %s", payload.Status),
		})
	}

	// Step 5: Store webhook in history
	webhookData := map[string]interface{}{
		"transfer_id":  payload.TransferID,
		"status":       payload.Status,
		"utr":          payload.UTR,
		"amount":       payload.Amount,
		"mode":         payload.Mode,
		"remarks":      payload.Remarks,
		"timestamp":    webhookTime,
		"beneficiary":  payload.BeneficiaryName,
		"full_payload": payload,
	}

	if err := h.transferRepo.AppendWebhookHistory(ctx, transfer.TransferRequestID, webhookData); err != nil {
		// Log error but don't fail the webhook
		h.logger.WarnWith("Failed to append webhook history", "error", err)
	} else {
		h.logger.DebugWith("Webhook stored in history")
	}

	h.logger.InfoWith("Castler webhook processing complete",
		"status", payload.Status,
		"transfer_id", payload.TransferID,
		"utr", payload.UTR)

	return c.JSON(http.StatusOK, map[string]string{
		"status": "ok",
		"message": "Webhook processed successfully",
	})
}

// handleCompleted handles the success webhook event
func (h *PayoutWebhookHandler) handleCompleted(ctx context.Context, transfer *payout.PayoutTransferRequest, payload CastlerWebhookPayload, webhookTime time.Time) error {
	h.logger.InfoWith("Handling transfer success",
		"transfer_id", transfer.TransferRequestID,
		"utr", payload.UTR,
		"amount", payload.Amount,
		"mode", payload.Mode)

	// Mark transfer as completed
	if err := h.transferRepo.MarkCompleted(ctx, transfer.TransferRequestID, payload.UTR, webhookTime); err != nil {
		return fmt.Errorf("failed to mark transfer completed: %w", err)
	}

	// Record activity: Payout completed
	transferUUID, err := uuid.Parse(transfer.TransferRequestID)
	if err != nil {
		h.logger.WarnWith("Invalid transfer_request_id UUID", "error", err)
	} else {
		paymentUUID, err := uuid.Parse(transfer.PaymentID)
		if err != nil {
			h.logger.WarnWith("Invalid payment_id UUID", "error", err)
		} else {
			_, err = h.stateChangeRepo.Create(ctx, &payment.CreatePaymentStateChangeRequest{
				PaymentID:       paymentUUID,
				TransferID:      &transferUUID,
				Action:          payment.PayoutActionCompleted,
				ActionTimestamp: webhookTime,
				ActorType:       payment.ActorTypeWebhook,
				ActorName:       "castler_webhook",
				PreviousState:   "pending",
				NewState:        "completed",
				Amount:          transfer.Amount,
				Currency:        transfer.Currency,
				UTR:             payload.UTR,
				Provider:        string(transfer.Provider),
				Metadata: map[string]any{
					"provider_transfer_id": transfer.ProviderTransferID,
					"transfer_mode":        payload.Mode,
					"beneficiary":          payload.BeneficiaryName,
				},
				Remarks: fmt.Sprintf("Payout completed successfully via settlement webhook (UTR: %s)", payload.UTR),
			})
			if err != nil {
				h.logger.WarnWith("Failed to create activity record for completed transfer", "error", err)
			}
		}
	}

	h.logger.InfoWith("Transfer completed successfully",
		"transfer_id", transfer.TransferRequestID,
		"utr", payload.UTR)
	return nil
}

// handleFailed handles the failed webhook event
func (h *PayoutWebhookHandler) handleFailed(ctx context.Context, transfer *payout.PayoutTransferRequest, payload CastlerWebhookPayload, webhookTime time.Time) error {
	failureReason := payload.Remarks // Castler sends failure reason in remarks field
	h.logger.WarnWith("Handling transfer failure",
		"transfer_id", transfer.TransferRequestID,
		"failure_reason", failureReason)

	// Mark transfer as failed
	if err := h.transferRepo.MarkFailed(ctx, transfer.TransferRequestID, failureReason, webhookTime); err != nil {
		return fmt.Errorf("failed to mark transfer failed: %w", err)
	}

	// Record activity: Payout failed
	transferUUID, err := uuid.Parse(transfer.TransferRequestID)
	if err != nil {
		h.logger.WarnWith("Invalid transfer_request_id UUID", "error", err)
	} else {
		paymentUUID, err := uuid.Parse(transfer.PaymentID)
		if err != nil {
			h.logger.WarnWith("Invalid payment_id UUID", "error", err)
		} else {
			_, err = h.stateChangeRepo.Create(ctx, &payment.CreatePaymentStateChangeRequest{
				PaymentID:       paymentUUID,
				TransferID:      &transferUUID,
				Action:          payment.PayoutActionFailed,
				ActionTimestamp: webhookTime,
				ActorType:       payment.ActorTypeWebhook,
				ActorName:       "castler_webhook",
				PreviousState:   "pending",
				NewState:        "failed",
				Amount:          transfer.Amount,
				Currency:        transfer.Currency,
				Provider:        string(transfer.Provider),
				Metadata: map[string]any{
					"provider_transfer_id": transfer.ProviderTransferID,
					"failure_reason":       failureReason,
					"transfer_mode":        payload.Mode,
					"beneficiary":          payload.BeneficiaryName,
				},
				Remarks: fmt.Sprintf("Payout failed via webhook: %s", failureReason),
			})
			if err != nil {
				h.logger.WarnWith("Failed to create activity record for failed transfer", "error", err)
			}
		}
	}

	// TODO: Implement retry strategy
	h.logger.InfoWith("Transfer marked as failed - manual intervention required",
		"transfer_id", transfer.TransferRequestID,
		"failure_reason", failureReason,
		"note", "MVP: Ops team handles failed transfers manually")

	return nil
}
