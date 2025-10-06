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

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/payout"
	"xcentro.in/internal/core/voucher"
)

// PayoutWebhookHandler handles webhook callbacks from payout providers
type PayoutWebhookHandler struct {
	transferRepo    payout.PayoutTransferRepository
	voucherRepo     voucher.VoucherRepository
	webhookSecret   string // Castler webhook secret for signature verification
}

// NewPayoutWebhookHandler creates a new webhook handler
func NewPayoutWebhookHandler(
	transferRepo payout.PayoutTransferRepository,
	voucherRepo voucher.VoucherRepository,
	webhookSecret string,
) *PayoutWebhookHandler {
	return &PayoutWebhookHandler{
		transferRepo:  transferRepo,
		voucherRepo:   voucherRepo,
		webhookSecret: webhookSecret,
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
	fmt.Println("=== CASTLER WEBHOOK: Received callback ===")

	// Step 1: Read request body
	bodyBytes, err := io.ReadAll(c.Request().Body)
	if err != nil {
		fmt.Printf("❌ Failed to read request body: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid_request",
			"message": "Failed to read request body",
		})
	}

	// Step 2: Verify webhook signature
	signature := c.Request().Header.Get("X-Castler-Signature")
	if !h.verifyCastlerSignature(signature, bodyBytes) {
		fmt.Printf("❌ Invalid webhook signature: %s\n", signature)
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "invalid_signature",
			"message": "Webhook signature verification failed",
		})
	}

	fmt.Println("✅ Webhook signature verified")

	// Step 3: Parse webhook payload
	var payload CastlerWebhookPayload
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		fmt.Printf("❌ Failed to parse webhook payload: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid_payload",
			"message": "Failed to parse webhook payload",
		})
	}

	fmt.Printf("📦 Webhook event: %s, TransferID: %s, Status: %s\n",
		payload.EventType, payload.TransferID, payload.Status)

	// Get context early for all operations
	ctx := c.Request().Context()

	// Step 4: Find transfer request by provider_transfer_id
	transfer, err := h.transferRepo.GetByProviderTransferID(ctx, payload.TransferID)
	if err != nil {
		fmt.Printf("❌ Transfer not found for provider_transfer_id: %s, error: %v\n", payload.TransferID, err)
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "transfer_not_found",
			"message": fmt.Sprintf("Transfer not found: %s", payload.TransferID),
		})
	}

	fmt.Printf("✅ Found transfer: %s (voucher: %s)\n", transfer.TransferRequestID, transfer.VoucherID)

	// Step 5: Handle event based on type

	switch payload.EventType {
	case "transfer.processing":
		if err := h.handleProcessing(ctx, transfer, payload); err != nil {
			fmt.Printf("❌ Failed to handle processing event: %v\n", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "processing_failed",
				"message": err.Error(),
			})
		}

	case "transfer.completed":
		if err := h.handleCompleted(ctx, transfer, payload); err != nil {
			fmt.Printf("❌ Failed to handle completed event: %v\n", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "completion_failed",
				"message": err.Error(),
			})
		}

	case "transfer.failed":
		if err := h.handleFailed(ctx, transfer, payload); err != nil {
			fmt.Printf("❌ Failed to handle failed event: %v\n", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "failure_handling_failed",
				"message": err.Error(),
			})
		}

	default:
		fmt.Printf("⚠️  Unknown webhook event type: %s\n", payload.EventType)
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
		fmt.Printf("⚠️  Failed to append webhook history: %v\n", err)
	} else {
		fmt.Println("✅ Webhook stored in history")
	}

	fmt.Println("=== CASTLER WEBHOOK: Processing complete ===")

	return c.JSON(http.StatusOK, map[string]string{
		"status": "ok",
		"message": "Webhook processed successfully",
	})
}

// handleProcessing handles the "transfer.processing" webhook event
func (h *PayoutWebhookHandler) handleProcessing(ctx context.Context, transfer *payout.PayoutTransferRequest, payload CastlerWebhookPayload) error {
	fmt.Println("  🔄 Processing: transfer.processing event")

	// Update transfer status to 'processing'
	if err := h.transferRepo.UpdateStatus(ctx, transfer.TransferRequestID, payout.TransferStatusProcessing, payload.Timestamp); err != nil {
		return fmt.Errorf("failed to update transfer status: %w", err)
	}

	// Update voucher payout status to 'processing'
	if err := h.voucherRepo.UpdatePayoutStatus(ctx, transfer.VoucherID, voucher.PayoutStatusProcessing, ""); err != nil {
		return fmt.Errorf("failed to update voucher status: %w", err)
	}

	fmt.Println("  ✅ Transfer marked as processing")
	return nil
}

// handleCompleted handles the "transfer.completed" webhook event
func (h *PayoutWebhookHandler) handleCompleted(ctx context.Context, transfer *payout.PayoutTransferRequest, payload CastlerWebhookPayload) error {
	fmt.Printf("  ✅ Success: transfer.completed event (UTR: %s)\n", payload.UTR)

	// Mark transfer as completed
	if err := h.transferRepo.MarkCompleted(ctx, transfer.TransferRequestID, payload.UTR, payload.Timestamp); err != nil {
		return fmt.Errorf("failed to mark transfer completed: %w", err)
	}

	// Update voucher payout status to 'completed' with UTR
	if err := h.voucherRepo.UpdatePayoutStatus(ctx, transfer.VoucherID, voucher.PayoutStatusCompleted, payload.UTR); err != nil {
		return fmt.Errorf("failed to update voucher status: %w", err)
	}

	fmt.Printf("  ✅ Transfer completed successfully (UTR: %s)\n", payload.UTR)
	return nil
}

// handleFailed handles the "transfer.failed" webhook event
func (h *PayoutWebhookHandler) handleFailed(ctx context.Context, transfer *payout.PayoutTransferRequest, payload CastlerWebhookPayload) error {
	failureReason := fmt.Sprintf("%s: %s", payload.FailureCode, payload.FailureMsg)
	fmt.Printf("  ❌ Failure: transfer.failed event (reason: %s)\n", failureReason)

	// Mark transfer as failed
	if err := h.transferRepo.MarkFailed(ctx, transfer.TransferRequestID, failureReason, payload.Timestamp); err != nil {
		return fmt.Errorf("failed to mark transfer failed: %w", err)
	}

	// Update voucher payout status to 'failed'
	if err := h.voucherRepo.UpdatePayoutStatus(ctx, transfer.VoucherID, voucher.PayoutStatusFailed, failureReason); err != nil {
		return fmt.Errorf("failed to update voucher status: %w", err)
	}

	// TODO: Implement retry strategy
	fmt.Println("  TODO: Implement retry strategy for failed transfer:", transfer.TransferRequestID)
	fmt.Println("  TODO: Consider unified retry strategy across payments/payouts")
	fmt.Println("  ℹ️  For MVP: Ops team handles failed transfers manually")

	fmt.Printf("  ✅ Transfer marked as failed (reason: %s)\n", failureReason)
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
