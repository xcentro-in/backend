package payment

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/adapters/external/payment/jiopay"
	"xcentro.in/internal/core/payment"
)

// WebhookHandler handles payment gateway webhooks (async settlement notifications)
// These are separate from SURL/FURL callbacks and are used for async settlement confirmations
type WebhookHandler struct {
	paymentRepo     payment.PaymentRepository
	jioPayGateway   payment.PaymentGateway
	easebuzzGateway payment.PaymentGateway
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(
	paymentRepo payment.PaymentRepository,
	jioPayGateway payment.PaymentGateway,
	easebuzzGateway payment.PaymentGateway,
) *WebhookHandler {
	return &WebhookHandler{
		paymentRepo:     paymentRepo,
		jioPayGateway:   jioPayGateway,
		easebuzzGateway: easebuzzGateway,
	}
}

// HandleJioPayWebhook handles async webhooks from JioPay gateway
// These are settlement notifications sent by JioPay asynchronously
// Note: Webhook payload structure might differ from SURL/FURL callback - validate with PG team
func (h *WebhookHandler) HandleJioPayWebhook(c echo.Context) error {
	ctx := c.Request().Context()

	fmt.Println("=== JIOPAY WEBHOOK: Async notification received ===")

	// Step 1: Read request body
	bodyBytes, err := io.ReadAll(c.Request().Body)
	if err != nil {
		fmt.Printf("❌ Failed to read webhook body: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid_request",
		})
	}

	fmt.Printf("📥 Webhook payload size: %d bytes\n", len(bodyBytes))

	// Step 2: Parse webhook payload
	// Note: Assuming same structure as callback payload - validate with PG team
	var webhook jiopay.CallbackPayload
	if err := json.Unmarshal(bodyBytes, &webhook); err != nil {
		fmt.Printf("❌ Failed to parse webhook payload: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid_payload",
		})
	}

	fmt.Printf("📋 MerchantTxnNo: %s\n", webhook.MerchantTxnNo)
	fmt.Printf("📋 TxnStatus: %s\n", webhook.TxnStatus)
	fmt.Printf("📋 ResponseCode: %s\n", webhook.ResponseCode)
	fmt.Printf("📋 Settlement Status: %s\n", webhook.SettlementStatus)

	// Step 3: Verify signature
	if err := h.jioPayGateway.ValidateWebhook(bodyBytes, webhook.SecureHash); err != nil {
		fmt.Printf("❌ Webhook signature verification failed: %v\n", err)
		// Return 200 OK to avoid gateway retries, but log the failure
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "error",
			"message": "Signature verification failed",
		})
	}

	fmt.Println("✅ Webhook signature verified")

	// Step 4: Query payment by merchantTxnNo
	pmt, err := h.paymentRepo.GetPaymentByGatewayPaymentID(ctx, webhook.MerchantTxnNo)
	if err != nil || pmt == nil {
		fmt.Printf("❌ Payment not found: %s, error: %v\n", webhook.MerchantTxnNo, err)
		// Return 200 OK to avoid gateway retries
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "error",
			"message": "Payment not found",
		})
	}

	fmt.Printf("✅ Payment found: %s (current status: %s)\n", pmt.ID, pmt.Status)

	// Step 5: Map status
	newStatus := mapCallbackStatusToPaymentStatus(
		jiopay.MapJioPayCallbackStatus(webhook.ResponseCode, webhook.TxnStatus),
	)

	fmt.Printf("📊 Status mapping: %s + %s → %s\n",
		webhook.ResponseCode, webhook.TxnStatus, newStatus)

	// Step 6: Update payment (idempotency check)
	if !isTerminalStatus(pmt.Status) {
		fmt.Println("🔄 Updating payment status from webhook...")

		metadata := map[string]any{
			"webhook_received_at":  time.Now().Format(time.RFC3339),
			"jiopay_txn_id":        webhook.TxnID,
			"response_code":        webhook.ResponseCode,
			"response_description": webhook.RespDescription,
			"txn_status":           webhook.TxnStatus,
			"txn_response_code":    webhook.TxnResponseCode,
			"settlement_status":    webhook.SettlementStatus,
			"settlement_id":        webhook.SettlementID,
			"settlement_date":      webhook.SettlementDate,
			"merchant_id":          webhook.MerchantID,
			"webhook_type":         "async_settlement",
		}

		if err := h.paymentRepo.UpdatePaymentStatus(ctx, pmt.ID, newStatus, metadata); err != nil {
			fmt.Printf("❌ Failed to update payment: %v\n", err)
			// Return 200 OK to avoid gateway retries
			return c.JSON(http.StatusOK, map[string]string{
				"status":  "error",
				"message": "Failed to update payment",
			})
		}

		fmt.Printf("✅ Payment status updated to: %s\n", newStatus)

		// Update gateway payment ID if not set
		if pmt.GatewayPaymentID == "" || pmt.GatewayPaymentID == webhook.MerchantTxnNo {
			if err := h.paymentRepo.UpdateGatewayPaymentID(ctx, pmt.ID, webhook.TxnID, ""); err != nil {
				fmt.Printf("⚠️  Failed to update gateway payment ID: %v\n", err)
			} else {
				fmt.Printf("✅ Gateway payment ID updated to: %s\n", webhook.TxnID)
			}
		}
	} else {
		fmt.Printf("⚠️  Payment already in terminal state: %s (idempotency - no update)\n", pmt.Status)
	}

	// TODO: Store webhook history for debugging
	// Create webhook_logs table to log all webhook attempts

	fmt.Println("=== JIOPAY WEBHOOK: Processing complete ===")

	return c.JSON(http.StatusOK, map[string]string{
		"status":  "success",
		"message": "Webhook processed successfully",
	})
}

// HandleEasebuzzWebhook handles webhooks from Easebuzz gateway
// TODO: Implement Easebuzz webhook handling (similar structure to JioPay)
func (h *WebhookHandler) HandleEasebuzzWebhook(c echo.Context) error {
	fmt.Println("=== EASEBUZZ WEBHOOK: Async notification received ===")

	// TODO: Implement Easebuzz-specific webhook handling
	// Similar to JioPay but with Easebuzz payload structure and signature verification

	return c.JSON(http.StatusOK, map[string]string{
		"status":  "pending",
		"message": "Easebuzz webhook not yet implemented",
	})
}
