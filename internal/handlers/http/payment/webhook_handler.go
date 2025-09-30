package payment

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/payment"
)

// WebhookHandler handles payment gateway webhooks
type WebhookHandler struct {
	orchestrator payment.PaymentOrchestrator
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(orchestrator payment.PaymentOrchestrator) *WebhookHandler {
	return &WebhookHandler{
		orchestrator: orchestrator,
	}
}

// HandleJioPayWebhook handles webhooks from JioPay gateway
func (h *WebhookHandler) HandleJioPayWebhook(c echo.Context) error {
	fmt.Println("=== WEBHOOK: JioPay Webhook Called ===")

	// TODO-DB-4.1-CRITICAL: Query payment by gateway_payment_id
	// - Parse gateway_payment_id from webhook payload
	// - Call: paymentRepo.GetPaymentByGatewayPaymentID(ctx, gatewayPaymentID)
	// - Verify payment exists and matches expected state

	// TODO: Phase 3C Implementation
	// 1. Parse JioPay webhook payload
	// 2. Validate webhook signature using JioPay gateway
	// 3. Extract payment status and transaction details
	// 4. Update payment status in database
	// 5. Trigger callback to frontend if needed
	// 6. Send acknowledgment to JioPay

	var payload map[string]any
	if err := c.Bind(&payload); err != nil {
		fmt.Printf("❌ Error parsing JioPay webhook: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid_payload",
		})
	}

	fmt.Printf("📥 JioPay Webhook Payload: %+v\n", payload)

	// TODO-DB-4.1-CRITICAL: Update payment status from webhook
	// - Map gateway status to PaymentStatus enum
	// - Call: paymentRepo.UpdatePaymentStatus(ctx, payment.ID, newStatus, webhookData)
	// - Store complete webhook payload in payment.metadata JSONB
	// - Update payment.updated_at timestamp

	// TODO-DB-4.1: Log webhook attempts for debugging
	// - Create webhook_logs table (future phase)
	// - Log all webhook attempts even if signature fails
	// - Helps diagnose integration issues

	// Mock webhook processing
	fmt.Println("🔄 Processing JioPay webhook...")
	fmt.Println("✅ JioPay webhook processed successfully (mock)")
	fmt.Println("=== WEBHOOK: JioPay Webhook Complete ===")

	return c.JSON(http.StatusOK, map[string]string{
		"status": "success",
		"message": "Webhook processed successfully",
	})
}

// HandleEasebuzzWebhook handles webhooks from Easebuzz gateway
func (h *WebhookHandler) HandleEasebuzzWebhook(c echo.Context) error {
	fmt.Println("=== WEBHOOK: Easebuzz Webhook Called ===")

	// TODO-DB-4.1-CRITICAL: Query payment by gateway_payment_id
	// - Parse gateway_payment_id (txnid) from webhook payload
	// - Call: paymentRepo.GetPaymentByGatewayPaymentID(ctx, gatewayPaymentID)
	// - Verify payment exists and matches expected state

	// TODO: Phase 3C Implementation
	// 1. Parse Easebuzz webhook payload
	// 2. Validate webhook signature using Easebuzz gateway
	// 3. Extract payment status and transaction details
	// 4. Update payment status in database
	// 5. Trigger callback to frontend if needed
	// 6. Send acknowledgment to Easebuzz

	var payload map[string]any
	if err := c.Bind(&payload); err != nil {
		fmt.Printf("❌ Error parsing Easebuzz webhook: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid_payload",
		})
	}

	fmt.Printf("📥 Easebuzz Webhook Payload: %+v\n", payload)

	// TODO-DB-4.1-CRITICAL: Update payment status from webhook
	// - Map gateway status to PaymentStatus enum
	// - Call: paymentRepo.UpdatePaymentStatus(ctx, payment.ID, newStatus, webhookData)
	// - Store complete webhook payload in payment.metadata JSONB
	// - Update payment.updated_at timestamp

	// TODO-DB-4.1: Log webhook attempts for debugging
	// - Create webhook_logs table (future phase)
	// - Log all webhook attempts even if signature fails
	// - Helps diagnose integration issues

	// Mock webhook processing
	fmt.Println("🔄 Processing Easebuzz webhook...")
	fmt.Println("✅ Easebuzz webhook processed successfully (mock)")
	fmt.Println("=== WEBHOOK: Easebuzz Webhook Complete ===")

	return c.JSON(http.StatusOK, map[string]string{
		"status": "success",
		"message": "Webhook processed successfully",
	})
}

// RegisterWebhookRoutes registers webhook routes for payment gateways
func RegisterWebhookRoutes(group *echo.Group, orchestrator payment.PaymentOrchestrator) {
	handler := NewWebhookHandler(orchestrator)

	// Webhook endpoints for different gateways
	group.POST("/jiopay", handler.HandleJioPayWebhook)
	group.POST("/easebuzz", handler.HandleEasebuzzWebhook)
}

// === WEBHOOK PAYLOAD STRUCTURES ===

// JioPayWebhookPayload represents JioPay webhook payload structure
// TODO: Update with actual JioPay webhook format
type JioPayWebhookPayload struct {
	PaymentID     string `json:"payment_id"`
	Status        string `json:"status"`
	Amount        string `json:"amount"`
	Currency      string `json:"currency"`
	TransactionID string `json:"transaction_id"`
	BankRef       string `json:"bank_ref,omitempty"`
	Signature     string `json:"signature"`
	Timestamp     string `json:"timestamp"`
}

// EasebuzzWebhookPayload represents Easebuzz webhook payload structure
// TODO: Update with actual Easebuzz webhook format
type EasebuzzWebhookPayload struct {
	Txnid         string `json:"txnid"`
	Status        string `json:"status"`
	Amount        string `json:"amount"`
	ProductInfo   string `json:"productinfo"`
	Email         string `json:"email"`
	Phone         string `json:"phone"`
	Hash          string `json:"hash"`
	BankRefNum    string `json:"bank_ref_num,omitempty"`
	Error         string `json:"error,omitempty"`
	ErrorMessage  string `json:"error_Message,omitempty"`
}