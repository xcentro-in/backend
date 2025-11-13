package payout

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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

// CastlerWebhookPayload represents the webhook payload from Castler
type CastlerWebhookPayload struct {
	EventType    string                 `json:"event_type"`     // "transfer.processing", "transfer.completed", "transfer.failed"
	TransferID   string                 `json:"transfer_id"`    // Castler's transfer ID
	Status       string                 `json:"status"`         // "processing", "success", "failed"
	UTR          string                 `json:"utr,omitempty"`  // Bank UTR (on success)
	FailureCode  string                 `json:"failure_code,omitempty"`
	FailureMsg   string                 `json:"failure_message,omitempty"`
	Timestamp    time.Time              `json:"timestamp"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// HandleCastlerWebhook processes webhook callbacks from Castler
func (h *PayoutWebhookHandler) HandleCastlerWebhook(c echo.Context) error {
	h.logger.InfoWith("Castler webhook received")

	// Step 1: Read request body
	bodyBytes, err := io.ReadAll(c.Request().Body)
	if err != nil {
		h.logger.ErrorWith("Failed to read webhook request body", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid_request",
			"message": "Failed to read request body",
		})
	}

	// Step 2: Verify webhook signature
	signature := c.Request().Header.Get("X-Castler-Signature")
	if !h.verifyCastlerSignature(signature, bodyBytes) {
		h.logger.WarnWith("Invalid webhook signature", "signature", signature)
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "invalid_signature",
			"message": "Webhook signature verification failed",
		})
	}

	h.logger.DebugWith("Webhook signature verified successfully")

	// Step 3: Parse webhook payload
	var payload CastlerWebhookPayload
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		h.logger.ErrorWith("Failed to parse webhook payload", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid_payload",
			"message": "Failed to parse webhook payload",
		})
	}

	h.logger.InfoWith("Webhook event received",
		"event_type", payload.EventType,
		"transfer_id", payload.TransferID,
		"status", payload.Status)

	// Get context early for all operations
	ctx := c.Request().Context()

	// Step 4: Find transfer request by provider_transfer_id
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
		"voucher_id", transfer.VoucherID)

	// Step 5: Handle event based on type
	// NOTE: Castler webhooks only send settlement events (completed/failed)
	// Processing status is set when API call succeeds (not via webhook)

	switch payload.EventType {
	case "transfer.processing":
		// DEPRECATED: Castler does not send processing events via webhook
		// Processing status is set immediately when API call succeeds
		h.logger.WarnWith("Received unexpected 'transfer.processing' event - ignoring")
		return c.JSON(http.StatusOK, map[string]string{
			"status": "ok",
			"message": "Processing event not expected - status already set on API success",
		})

	case "transfer.completed":
		if err := h.handleCompleted(ctx, transfer, payload); err != nil {
			h.logger.ErrorWith("Failed to handle completed event", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "completion_failed",
				"message": err.Error(),
			})
		}

	case "transfer.failed":
		if err := h.handleFailed(ctx, transfer, payload); err != nil {
			h.logger.ErrorWith("Failed to handle failed event", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "failure_handling_failed",
				"message": err.Error(),
			})
		}

	default:
		h.logger.WarnWith("Unknown webhook event type", "event_type", payload.EventType)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "unknown_event",
			"message": fmt.Sprintf("Unknown event type: %s", payload.EventType),
		})
	}

	// Step 6: Store webhook in history
	webhookData := map[string]interface{}{
		"event_type":  payload.EventType,
		"transfer_id": payload.TransferID,
		"status":      payload.Status,
		"timestamp":   payload.Timestamp,
		"payload":     payload,
	}

	if err := h.transferRepo.AppendWebhookHistory(ctx, transfer.TransferRequestID, webhookData); err != nil {
		// Log error but don't fail the webhook
		h.logger.WarnWith("Failed to append webhook history", "error", err)
	} else {
		h.logger.DebugWith("Webhook stored in history")
	}

	h.logger.InfoWith("Castler webhook processing complete",
		"event_type", payload.EventType,
		"transfer_id", payload.TransferID)

	return c.JSON(http.StatusOK, map[string]string{
		"status": "ok",
		"message": "Webhook processed successfully",
	})
}

// handleProcessing handles the "transfer.processing" webhook event
// DEPRECATED: Castler does not send processing events via webhook
// Processing status is set immediately when payout API call succeeds
func (h *PayoutWebhookHandler) handleProcessing(ctx context.Context, transfer *payout.PayoutTransferRequest, payload CastlerWebhookPayload) error {
	h.logger.WarnWith("Received DEPRECATED transfer.processing event",
		"transfer_id", transfer.TransferRequestID)

	// Update transfer status to 'processing'
	if err := h.transferRepo.UpdateStatus(ctx, transfer.TransferRequestID, payout.TransferStatusProcessing, payload.Timestamp); err != nil {
		return fmt.Errorf("failed to update transfer status: %w", err)
	}

	h.logger.InfoWith("Transfer marked as processing", "transfer_id", transfer.TransferRequestID)
	return nil
}

// handleCompleted handles the "transfer.completed" webhook event
func (h *PayoutWebhookHandler) handleCompleted(ctx context.Context, transfer *payout.PayoutTransferRequest, payload CastlerWebhookPayload) error {
	h.logger.InfoWith("Handling transfer.completed event",
		"transfer_id", transfer.TransferRequestID,
		"utr", payload.UTR)

	// Mark transfer as completed
	if err := h.transferRepo.MarkCompleted(ctx, transfer.TransferRequestID, payload.UTR, payload.Timestamp); err != nil {
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
				ActionTimestamp: payload.Timestamp,
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
					"event_type":           payload.EventType,
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

// handleFailed handles the "transfer.failed" webhook event
func (h *PayoutWebhookHandler) handleFailed(ctx context.Context, transfer *payout.PayoutTransferRequest, payload CastlerWebhookPayload) error {
	failureReason := fmt.Sprintf("%s: %s", payload.FailureCode, payload.FailureMsg)
	h.logger.WarnWith("Handling transfer.failed event",
		"transfer_id", transfer.TransferRequestID,
		"failure_reason", failureReason)

	// Mark transfer as failed
	if err := h.transferRepo.MarkFailed(ctx, transfer.TransferRequestID, failureReason, payload.Timestamp); err != nil {
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
				ActionTimestamp: payload.Timestamp,
				ActorType:       payment.ActorTypeWebhook,
				ActorName:       "castler_webhook",
				PreviousState:   "pending",
				NewState:        "failed",
				Amount:          transfer.Amount,
				Currency:        transfer.Currency,
				Provider:        string(transfer.Provider),
				Metadata: map[string]any{
					"provider_transfer_id": transfer.ProviderTransferID,
					"failure_code":         payload.FailureCode,
					"failure_message":      payload.FailureMsg,
					"event_type":           payload.EventType,
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

// verifyCastlerSignature verifies the HMAC-SHA256 signature from Castler
func (h *PayoutWebhookHandler) verifyCastlerSignature(signature string, body []byte) bool {
	// Create HMAC-SHA256 hash
	mac := hmac.New(sha256.New, []byte(h.webhookSecret))
	mac.Write(body)
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	// Compare signatures (constant-time comparison to prevent timing attacks)
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}
