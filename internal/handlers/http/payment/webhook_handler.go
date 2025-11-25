package payment

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/adapters/external/payment/easebuzz"
	"xcentro.in/internal/adapters/external/payment/jiopay"
	"xcentro.in/internal/adapters/external/payment/mockpg"
	"xcentro.in/internal/core/payment"
	paymentSvc "xcentro.in/internal/services/payment"
)

// WebhookHandler handles payment gateway webhooks
// Supports both payment status webhooks and settlement webhooks (future)
type WebhookHandler struct {
	paymentRepo     payment.PaymentRepository
	jioPayGateway   payment.PaymentGateway
	easebuzzGateway payment.PaymentGateway
	mockPGGateway   payment.PaymentGateway
	mockPGEnabled   bool // Flag to check if mock PG is enabled
	stateLogger     *paymentSvc.PaymentStateLogger
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(
	paymentRepo payment.PaymentRepository,
	jioPayGateway payment.PaymentGateway,
	easebuzzGateway payment.PaymentGateway,
	mockPGGateway payment.PaymentGateway,
	mockPGEnabled bool,
	stateChangeRepo payment.PaymentStateChangeRepository,
) *WebhookHandler {
	return &WebhookHandler{
		paymentRepo:     paymentRepo,
		jioPayGateway:   jioPayGateway,
		easebuzzGateway: easebuzzGateway,
		mockPGGateway:   mockPGGateway,
		mockPGEnabled:   mockPGEnabled,
		stateLogger:     paymentSvc.NewPaymentStateLogger(stateChangeRepo),
	}
}

// HandlePaymentStatusWebhook routes payment status webhooks to appropriate gateway handler
// Endpoint: POST /webhooks/payment_status?type=<gateway>
// Supports: type=jiopay, type=easebuzz, type=mockpg (testing only)
func (h *WebhookHandler) HandlePaymentStatusWebhook(c echo.Context) error {
	gatewayType := c.QueryParam("type")

	switch gatewayType {
	case "jiopay":
		return h.handleJioPayPaymentWebhook(c)
	case "easebuzz":
		return h.handleEasebuzzPaymentWebhook(c)
	case "mockpg":
		// Only allow mock PG webhooks if mock PG is enabled
		if !h.mockPGEnabled {
			return c.JSON(http.StatusForbidden, map[string]string{
				"status":  "error",
				"message": "Mock PG is not enabled",
			})
		}
		return h.handleMockPGPaymentWebhook(c)
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{
			"status":  "error",
			"message": "Invalid or missing gateway type parameter. Supported: jiopay, easebuzz, mockpg",
		})
	}
}

// handleJioPayPaymentWebhook handles payment status webhooks from JioPay gateway
// These are async payment status notifications sent by JioPay
// Note: Webhook payload structure might differ from SURL/FURL callback - validate with PG team
func (h *WebhookHandler) handleJioPayPaymentWebhook(c echo.Context) error {
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
			"webhook_type":         "payment_status",
			"gateway":              "jiopay",
			"jiopay_txn_id":        webhook.TxnID,
			"response_code":        webhook.ResponseCode,
			"response_description": webhook.RespDescription,
			"txn_status":           webhook.TxnStatus,
			"txn_response_code":    webhook.TxnResponseCode,
			"merchant_id":          webhook.MerchantID,
			"aggregator_id":        webhook.AggregatorID,
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

		// Initialize settlement status for successful payments
		if newStatus == payment.PaymentStatusSuccess {
			settlementMetadata := map[string]any{
				"settlement_initiated_at": time.Now().Format(time.RFC3339),
				"settlement_gateway":      "jiopay",
			}
			if err := h.paymentRepo.UpdateSettlementStatus(ctx, pmt.ID, payment.SettlementStatusPending, settlementMetadata); err != nil {
				fmt.Printf("⚠️  Failed to initialize settlement status: %v\n", err)
			} else {
				fmt.Println("✅ Settlement status initialized to pending")
			}
		}

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

// handleEasebuzzPaymentWebhook handles payment status webhooks from Easebuzz gateway
// These are async payment status notifications sent by Easebuzz
func (h *WebhookHandler) handleEasebuzzPaymentWebhook(c echo.Context) error {
	ctx := c.Request().Context()

	fmt.Println("=== EASEBUZZ WEBHOOK: Async notification received ===")

	// Step 1: Read request body
	bodyBytes, err := io.ReadAll(c.Request().Body)
	if err != nil {
		fmt.Printf("❌ Failed to read webhook body: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid_request",
		})
	}

	fmt.Printf("📥 Webhook payload size: %d bytes\n", len(bodyBytes))

	// Step 2: Parse webhook payload (same structure as callback)
	var webhook easebuzz.CallbackPayload
	if err := json.Unmarshal(bodyBytes, &webhook); err != nil {
		fmt.Printf("❌ Failed to parse webhook payload: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid_payload",
		})
	}

	fmt.Printf("📋 TxnID: %s\n", webhook.TxnID)
	fmt.Printf("📋 Status: %s\n", webhook.Status)
	fmt.Printf("📋 EasepayID: %s\n", webhook.EasepayID)
	fmt.Printf("📋 SettlementAmount: %s\n", webhook.SettlementAmount)

	// Step 3: Verify signature
	if err := h.easebuzzGateway.ValidateWebhook(bodyBytes, webhook.Hash); err != nil {
		fmt.Printf("❌ Webhook signature verification failed: %v\n", err)
		// Return 200 OK to avoid gateway retries, but log the failure
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "error",
			"message": "Signature verification failed",
		})
	}

	fmt.Println("✅ Webhook signature verified")

	// Step 4: Query payment by txnid (our transaction ID)
	pmt, err := h.paymentRepo.GetPaymentByGatewayPaymentID(ctx, webhook.TxnID)
	if err != nil || pmt == nil {
		fmt.Printf("❌ Payment not found: %s, error: %v\n", webhook.TxnID, err)
		// Return 200 OK to avoid gateway retries
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "error",
			"message": "Payment not found",
		})
	}

	fmt.Printf("✅ Payment found: %s (current status: %s)\n", pmt.ID, pmt.Status)

	// Step 5: Map status
	newStatus := mapCallbackStatusToPaymentStatus(
		easebuzz.MapEasebuzzCallbackStatus(webhook.Status),
	)

	fmt.Printf("📊 Status mapping: %s → %s\n", webhook.Status, newStatus)

	// Step 6: Update payment (idempotency check)
	if !isTerminalStatus(pmt.Status) {
		fmt.Println("🔄 Updating payment status from webhook...")

		metadata := map[string]any{
			"webhook_received_at":  time.Now().Format(time.RFC3339),
			"webhook_type":         "payment_status",
			"gateway":              "easebuzz",
			"easepayid":            webhook.EasepayID,
			"bank_ref_num":         webhook.BankRefNum,
			"payment_source":       webhook.PaymentSource,
			"card_type":            webhook.CardType,
			"mode":                 webhook.Mode,
			"net_amount_debit":     webhook.NetAmountDebit,
			"service_tax":          webhook.ServiceTax,
			"service_charge":       webhook.ServiceCharge,
			"deduction_percentage": webhook.DeductionPercentage,
			"cash_back_percentage": webhook.CashBackPercentage,
			"discount_amount":      webhook.DiscountAmount,
			"payment_category":     webhook.PaymentCategory,
			"error":                webhook.Error,
			"error_message":        webhook.ErrorMessage,
			"error_code":           webhook.ErrorCode,
			"added_on":             webhook.AddedOn,
			"response_code":        webhook.ResponseCode,
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

		// Initialize settlement status for successful payments
		if newStatus == payment.PaymentStatusSuccess {
			settlementMetadata := map[string]any{
				"settlement_initiated_at": time.Now().Format(time.RFC3339),
				"settlement_gateway":      "easebuzz",
			}
			if err := h.paymentRepo.UpdateSettlementStatus(ctx, pmt.ID, payment.SettlementStatusPending, settlementMetadata); err != nil {
				fmt.Printf("⚠️  Failed to initialize settlement status: %v\n", err)
			} else {
				fmt.Println("✅ Settlement status initialized to pending")
			}
		}

		// Update gateway payment ID if not set or if it's still our txnid
		if pmt.GatewayPaymentID == "" || pmt.GatewayPaymentID == webhook.TxnID {
			if err := h.paymentRepo.UpdateGatewayPaymentID(ctx, pmt.ID, webhook.EasepayID, ""); err != nil {
				fmt.Printf("⚠️  Failed to update gateway payment ID: %v\n", err)
			} else {
				fmt.Printf("✅ Gateway payment ID updated to: %s\n", webhook.EasepayID)
			}
		}
	} else {
		fmt.Printf("⚠️  Payment already in terminal state: %s (idempotency - no update)\n", pmt.Status)
	}

	// TODO: Store webhook history for debugging
	// Create webhook_logs table to log all webhook attempts

	fmt.Println("=== EASEBUZZ WEBHOOK: Processing complete ===")

	return c.JSON(http.StatusOK, map[string]string{
		"status":  "success",
		"message": "Webhook processed successfully",
	})
}

// handleMockPGPaymentWebhook handles payment status webhooks from Mock PG (testing only)
// This simulates the async webhook that real gateways send after payment completion
func (h *WebhookHandler) handleMockPGPaymentWebhook(c echo.Context) error {
	ctx := c.Request().Context()

	fmt.Println("=== MOCK PG WEBHOOK: Async notification received ===")

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
	var webhook mockpg.MockPGWebhookPayload
	if err := json.Unmarshal(bodyBytes, &webhook); err != nil {
		fmt.Printf("❌ Failed to parse webhook payload: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid_payload",
		})
	}

	fmt.Printf("📋 TransactionID: %s\n", webhook.TransactionID)
	fmt.Printf("📋 GatewayPaymentID: %s\n", webhook.GatewayPaymentID)
	fmt.Printf("📋 Status: %s\n", webhook.Status)
	fmt.Printf("📋 Amount: %d %s\n", webhook.Amount, webhook.Currency)
	fmt.Printf("📋 ProcessedAt: %s\n", webhook.ProcessedAt)

	// Step 3: Verify signature (mock validation)
	if err := h.mockPGGateway.ValidateWebhook(bodyBytes, webhook.Signature); err != nil {
		fmt.Printf("❌ Webhook signature verification failed: %v\n", err)
		// Return 200 OK to avoid gateway retries, but log the failure
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "error",
			"message": "Signature verification failed",
		})
	}

	fmt.Println("✅ Webhook signature verified")

	// Step 4: Query payment by transaction ID
	pmt, err := h.paymentRepo.GetPaymentByID(ctx, webhook.TransactionID)
	if err != nil || pmt == nil {
		fmt.Printf("❌ Payment not found: %s, error: %v\n", webhook.TransactionID, err)
		// Return 200 OK to avoid gateway retries
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "error",
			"message": "Payment not found",
		})
	}

	fmt.Printf("✅ Payment found: %s (current status: %s)\n", pmt.ID, pmt.Status)

	// Step 5: Map status
	var newStatus payment.PaymentStatus
	if webhook.Status == "success" {
		newStatus = payment.PaymentStatusSuccess
	} else {
		newStatus = payment.PaymentStatusFailed
	}

	fmt.Printf("📊 Status mapping: %s → %s\n", webhook.Status, newStatus)

	// Step 6: Update payment (idempotency check)
	if !isTerminalStatus(pmt.Status) {
		fmt.Println("🔄 Updating payment status from webhook...")

		metadata := map[string]any{
			"webhook_received_at":   time.Now().Format(time.RFC3339),
			"webhook_type":          "payment_status",
			"gateway":               "mockpg",
			"mock_gateway_payment_id": webhook.GatewayPaymentID,
			"payment_method":        webhook.PaymentMethod,
			"processed_at":          webhook.ProcessedAt,
			"merchant_id":           webhook.MerchantID,
			"mock_simulation":       true,
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

		// Initialize settlement status for successful payments
		if newStatus == payment.PaymentStatusSuccess {
			settlementMetadata := map[string]any{
				"settlement_initiated_at": time.Now().Format(time.RFC3339),
				"settlement_gateway":      "mockpg",
			}
			if err := h.paymentRepo.UpdateSettlementStatus(ctx, pmt.ID, payment.SettlementStatusPending, settlementMetadata); err != nil {
				fmt.Printf("⚠️  Failed to initialize settlement status: %v\n", err)
			} else {
				fmt.Println("✅ Settlement status initialized to pending")
			}
		}

		// Update gateway payment ID if not set
		if pmt.GatewayPaymentID == "" || pmt.GatewayPaymentID != webhook.GatewayPaymentID {
			if err := h.paymentRepo.UpdateGatewayPaymentID(ctx, pmt.ID, webhook.GatewayPaymentID, ""); err != nil {
				fmt.Printf("⚠️  Failed to update gateway payment ID: %v\n", err)
			} else {
				fmt.Printf("✅ Gateway payment ID updated to: %s\n", webhook.GatewayPaymentID)
			}
		}
	} else {
		fmt.Printf("⚠️  Payment already in terminal state: %s (idempotency - no update)\n", pmt.Status)
	}

	fmt.Println("=== MOCK PG WEBHOOK: Processing complete ===")

	return c.JSON(http.StatusOK, map[string]string{
		"status":  "success",
		"message": "Webhook processed successfully",
	})
}

// ============================================================================
// SETTLEMENT WEBHOOK HELPERS
// ============================================================================

// parseAmountToPaise converts rupee string (e.g., "1000.00") to paise int64 (e.g., 100000)
//
// JioPay sends amounts in rupee format with 2 decimal places.
// Our database stores amounts in paise (smallest unit) for precision.
//
// Examples:
//   - "1000.00" → 100000
//   - "15.00" → 1500
//   - "2.70" → 270
//   - "982.30" → 98230
func parseAmountToPaise(amountStr string) (int64, error) {
	if amountStr == "" {
		return 0, fmt.Errorf("amount string is empty")
	}

	// Parse as float64
	rupees, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse amount '%s': %w", amountStr, err)
	}

	// Validate non-negative
	if rupees < 0 {
		return 0, fmt.Errorf("amount cannot be negative: %s", amountStr)
	}

	// Convert to paise (multiply by 100 and round to avoid floating point issues)
	paise := int64(math.Round(rupees * 100))

	return paise, nil
}

// parseSettlementDate parses JioPay settlement date format YYYYMMDD to time.Time
//
// Examples:
//   - "20171101" → 2017-11-01 00:00:00
//   - "20250610" → 2025-06-10 00:00:00
func parseSettlementDate(dateStr string) (time.Time, error) {
	if dateStr == "" {
		return time.Time{}, fmt.Errorf("settlement date string is empty")
	}

	// Parse using Go's reference date layout
	// Reference: "20060102" represents YYYYMMDD format
	settlementDate, err := time.Parse("20060102", dateStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse settlement date '%s': %w", dateStr, err)
	}

	return settlementDate, nil
}

// ============================================================================
// EASEBUZZ SETTLEMENT WEBHOOK HELPERS
// ============================================================================

// parseFormEncodedWebhook extracts JSON from form-urlencoded webhook data
//
// Easebuzz sends webhook data as form-urlencoded format per their documentation:
// "The data received on the webhook URL will be in form-url-encoded format.
// It needs to be parsed into the JSON first and then use it accordingly."
//
// This function handles:
//  1. Reading raw request body
//  2. Detecting if data is already JSON (fallback)
//  3. Parsing form-urlencoded data
//  4. Extracting JSON from form field (typically "data" field)
//
// Returns:
//   - JSON bytes ready for unmarshaling
//   - Error if parsing fails
func parseFormEncodedWebhook(c echo.Context) ([]byte, error) {
	// Step 1: Read raw body
	bodyBytes, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	// Step 2: Check if it's already JSON (some gateways may send JSON despite docs)
	if json.Valid(bodyBytes) {
		fmt.Println("📥 Body is already valid JSON")
		return bodyBytes, nil
	}

	// Step 3: Parse as form data
	// Reset body reader for form parsing
	c.Request().Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	if err := c.Request().ParseForm(); err != nil {
		return nil, fmt.Errorf("failed to parse form: %w", err)
	}

	fmt.Printf("📥 Form fields received: %d\n", len(c.Request().Form))

	// Step 4: Try common form field names
	// Easebuzz typically uses "data" field for JSON payload
	if dataField := c.Request().FormValue("data"); dataField != "" {
		fmt.Println("✅ Found JSON in 'data' form field")
		return []byte(dataField), nil
	}

	// Step 5: If no "data" field found, log all fields for debugging
	fmt.Println("⚠️  No 'data' field found, available fields:")
	for key := range c.Request().Form {
		fmt.Printf("   - %s\n", key)
	}

	// Fallback: assume entire body is JSON
	return bodyBytes, nil
}

// parseNumberToPaise converts float64 amount to paise int64
//
// Easebuzz sends amounts as numbers (int or float64), never strings.
// This function converts rupee amounts to paise (smallest unit) for database storage.
//
// Examples:
//   - 5.0 → 500 paise
//   - 982.30 → 98230 paise
//   - 0.0 → 0 paise
//
// Handles floating point precision by rounding to nearest integer.
func parseNumberToPaise(amount float64) int64 {
	return int64(math.Round(amount * 100))
}

// parseEasebuzzSettlementDate parses Easebuzz settlement date format to time.Time
//
// Easebuzz date format: "YYYY-MM-DD HH:MM:SS.SSSSSS"
//
// Examples:
//   - "2025-01-30 11:50:21.712996" → 2025-01-30 11:50:21 UTC
//   - "2025-01-30 11:50:21" → 2025-01-30 11:50:21 UTC (without microseconds)
//
// Tries with microseconds first, falls back to without if parsing fails.
func parseEasebuzzSettlementDate(dateStr string) (time.Time, error) {
	if dateStr == "" {
		return time.Time{}, fmt.Errorf("date string is empty")
	}

	// Try with microseconds first: "2006-01-02 15:04:05.999999"
	settlementDate, err := time.Parse("2006-01-02 15:04:05.999999", dateStr)
	if err != nil {
		// Fallback without microseconds: "2006-01-02 15:04:05"
		settlementDate, err = time.Parse("2006-01-02 15:04:05", dateStr)
		if err != nil {
			return time.Time{}, fmt.Errorf("failed to parse date '%s': %w", dateStr, err)
		}
	}

	return settlementDate, nil
}

// ============================================================================
// SETTLEMENT STATUS WEBHOOKS (FUTURE IMPLEMENTATION)
// ============================================================================

// HandleSettlementStatusWebhook processes settlement notifications from payment gateways
// Endpoint: POST /webhooks/settlement_status?type=<gateway>
// Supports: type=jiopay, type=easebuzz (future)
//
// Settlement webhooks differ from payment status webhooks:
//   - Payment status: Immediate transaction result (success/failed/pending)
//   - Settlement: Batch processing notifications (T+1 to T+3) with final amounts
//
// Settlement webhooks update:
//   - settlement_status: 'pending' → 'settled'
//   - settlement_amount: Final settled amount (may differ from total_amount)
//   - mdr_amount: Gateway fees (MDR + GST)
//   - net_settlement_amount: Final amount received (settlement - mdr)
//   - settled_at: Actual settlement date
//   - settlement_reference_id: Settlement batch ID
//   - settlement_metadata: Full webhook payload as JSONB
func (h *WebhookHandler) HandleSettlementStatusWebhook(c echo.Context) error {
	gatewayType := c.QueryParam("type")

	fmt.Printf("📦 Settlement webhook received for gateway: %s\n", gatewayType)

	switch gatewayType {
	case "jiopay":
		return h.handleJioPaySettlementWebhook(c)
	case "easebuzz":
		return h.handleEasebuzzSettlementWebhook(c)
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{
			"status":  "error",
			"message": "Invalid or missing gateway type parameter. Supported: jiopay, easebuzz",
		})
	}
}

// handleJioPaySettlementWebhook processes settlement notifications from JioPay
// Sent when JioPay completes settlement to merchant account (T+1 to T+3 days)
func (h *WebhookHandler) handleJioPaySettlementWebhook(c echo.Context) error {
	ctx := c.Request().Context()

	fmt.Println("=== JIOPAY SETTLEMENT WEBHOOK: Processing settlement notification ===")

	// Step 1: Read request body
	bodyBytes, err := io.ReadAll(c.Request().Body)
	if err != nil {
		fmt.Printf("❌ Failed to read webhook body: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid_request",
		})
	}

	fmt.Printf("📥 Webhook payload size: %d bytes\n", len(bodyBytes))

	// Step 2: Parse settlement webhook payload
	var webhook jiopay.SettlementWebhook
	if err := json.Unmarshal(bodyBytes, &webhook); err != nil {
		fmt.Printf("❌ Failed to parse webhook payload: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid_payload",
		})
	}

	fmt.Printf("📋 MerchantTxnNo: %s\n", webhook.MerchantTxnNo)
	fmt.Printf("📋 SettlementID: %s\n", webhook.SettlementID)
	fmt.Printf("📋 SettlementDate: %s\n", webhook.SettlementDate)
	fmt.Printf("📋 TxnAmount: %s\n", webhook.TxnAmount)
	fmt.Printf("📋 TxnCharges: %s\n", webhook.TxnCharges)
	fmt.Printf("📋 ServiceTax: %s\n", webhook.ServiceTax)
	fmt.Printf("📋 SettledAmount: %s\n", webhook.SettledAmount)

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

	// Step 4: Find payment by merchantTxnNo (our payment_id)
	pmt, err := h.paymentRepo.GetPaymentByGatewayPaymentID(ctx, webhook.MerchantTxnNo)
	if err != nil || pmt == nil {
		fmt.Printf("❌ Payment not found: %s, error: %v\n", webhook.MerchantTxnNo, err)
		// Return 200 OK to avoid gateway retries
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "error",
			"message": "Payment not found",
		})
	}

	fmt.Printf("✅ Payment found: %s (status: %s, settlement_status: %s)\n",
		pmt.ID, pmt.Status, pmt.SettlementStatus)

	// Step 5: Validate payment state
	if pmt.Status != payment.PaymentStatusSuccess {
		fmt.Printf("⚠️  Payment status is not 'success': %s (cannot settle)\n", pmt.Status)
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "error",
			"message": "Payment not in success state",
		})
	}

	// Step 6: Check idempotency - if already settled, verify amounts
	if pmt.SettlementStatus == payment.SettlementStatusSettled {
		fmt.Println("⚠️  Payment already settled - checking idempotency...")

		// Parse webhook amounts
		webhookSettlementAmount, err := parseAmountToPaise(webhook.TxnAmount)
		if err != nil {
			fmt.Printf("❌ Failed to parse txnAmount: %v\n", err)
			return c.JSON(http.StatusOK, map[string]string{
				"status":  "error",
				"message": "Invalid amount format",
			})
		}

		webhookTxnCharges, err := parseAmountToPaise(webhook.TxnCharges)
		if err != nil {
			fmt.Printf("❌ Failed to parse txnCharges: %v\n", err)
			return c.JSON(http.StatusOK, map[string]string{
				"status":  "error",
				"message": "Invalid charges format",
			})
		}

		webhookServiceTax, err := parseAmountToPaise(webhook.ServiceTax)
		if err != nil {
			fmt.Printf("❌ Failed to parse serviceTax: %v\n", err)
			return c.JSON(http.StatusOK, map[string]string{
				"status":  "error",
				"message": "Invalid service tax format",
			})
		}

		webhookMDRAmount := webhookTxnCharges + webhookServiceTax
		webhookNetAmount, err := parseAmountToPaise(webhook.SettledAmount)
		if err != nil {
			fmt.Printf("❌ Failed to parse settledAmount: %v\n", err)
			return c.JSON(http.StatusOK, map[string]string{
				"status":  "error",
				"message": "Invalid settled amount format",
			})
		}

		// Verify amounts match (idempotency check)
		if pmt.SettlementAmount != nil && *pmt.SettlementAmount != webhookSettlementAmount {
			fmt.Printf("⚠️  Settlement amount mismatch! DB: %d, Webhook: %d\n",
				*pmt.SettlementAmount, webhookSettlementAmount)
		}

		if pmt.MDRAmount != webhookMDRAmount {
			fmt.Printf("⚠️  MDR amount mismatch! DB: %d, Webhook: %d\n",
				pmt.MDRAmount, webhookMDRAmount)
		}

		if pmt.NetSettlementAmount != nil && *pmt.NetSettlementAmount != webhookNetAmount {
			fmt.Printf("⚠️  Net settlement amount mismatch! DB: %d, Webhook: %d\n",
				*pmt.NetSettlementAmount, webhookNetAmount)
		}

		fmt.Println("✅ Payment already settled - idempotent request, returning success")
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "success",
			"message": "Settlement already processed (idempotent)",
		})
	}

	// Step 7: Validate settlement status is 'pending'
	if pmt.SettlementStatus != payment.SettlementStatusPending {
		fmt.Printf("⚠️  Settlement status is not 'pending': %s\n", pmt.SettlementStatus)
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "error",
			"message": "Invalid settlement status",
		})
	}

	// Step 8: Parse and validate amounts
	settlementAmount, err := parseAmountToPaise(webhook.TxnAmount)
	if err != nil {
		fmt.Printf("❌ Failed to parse txnAmount: %v\n", err)
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "error",
			"message": "Invalid txnAmount format",
		})
	}

	txnCharges, err := parseAmountToPaise(webhook.TxnCharges)
	if err != nil {
		fmt.Printf("❌ Failed to parse txnCharges: %v\n", err)
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "error",
			"message": "Invalid txnCharges format",
		})
	}

	serviceTax, err := parseAmountToPaise(webhook.ServiceTax)
	if err != nil {
		fmt.Printf("❌ Failed to parse serviceTax: %v\n", err)
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "error",
			"message": "Invalid serviceTax format",
		})
	}

	netSettlementAmount, err := parseAmountToPaise(webhook.SettledAmount)
	if err != nil {
		fmt.Printf("❌ Failed to parse settledAmount: %v\n", err)
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "error",
			"message": "Invalid settledAmount format",
		})
	}

	// Calculate total MDR (charges + GST)
	mdrAmount := txnCharges + serviceTax

	// Validate calculation: settledAmount = txnAmount - charges - tax
	expectedNetAmount := settlementAmount - mdrAmount
	if netSettlementAmount != expectedNetAmount {
		fmt.Printf("⚠️  Settlement calculation mismatch! Expected: %d, Got: %d\n",
			expectedNetAmount, netSettlementAmount)
	}

	fmt.Printf("💰 Settlement amounts (in paise):\n")
	fmt.Printf("   - Settlement Amount: %d (₹%.2f)\n", settlementAmount, float64(settlementAmount)/100)
	fmt.Printf("   - MDR Amount: %d (₹%.2f) [Charges: %d + Tax: %d]\n",
		mdrAmount, float64(mdrAmount)/100, txnCharges, serviceTax)
	fmt.Printf("   - Net Settlement: %d (₹%.2f)\n", netSettlementAmount, float64(netSettlementAmount)/100)

	// Step 9: Parse settlement date
	settlementDate, err := parseSettlementDate(webhook.SettlementDate)
	if err != nil {
		fmt.Printf("❌ Failed to parse settlement date: %v\n", err)
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "error",
			"message": "Invalid settlement date format",
		})
	}

	fmt.Printf("📅 Settlement date: %s\n", settlementDate.Format("2006-01-02"))

	// Step 10: Prepare settlement metadata (store full webhook payload)
	var webhookData map[string]any
	if err := json.Unmarshal(bodyBytes, &webhookData); err != nil {
		fmt.Printf("⚠️  Failed to parse webhook for metadata: %v\n", err)
		webhookData = make(map[string]any)
	}

	settlementMetadata := map[string]any{
		"settlement_webhook_received_at": time.Now().Format(time.RFC3339),
		"settlement_gateway":             "jiopay",
		"settlement_batch_id":            webhook.SettlementID,
		"settlement_account":             webhook.SettlementAccount,
		"settlement_account_ifsc":        webhook.SettlementAccountIFSC,
		"payment_mode":                   webhook.PaymentMode,
		"payment_sub_inst_type":          webhook.PaymentSubInstType,
		"card_network":                   webhook.CardNetwork,
		"webhook_payload":                webhookData, // Store full webhook
	}

	// Step 11: Update settlement status to 'settled'
	fmt.Println("🔄 Updating settlement status to 'settled'...")
	if err := h.paymentRepo.UpdateSettlementStatus(ctx, pmt.ID, payment.SettlementStatusSettled, settlementMetadata); err != nil {
		fmt.Printf("❌ Failed to update settlement status: %v\n", err)
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "error",
			"message": "Failed to update settlement status",
		})
	}

	fmt.Println("✅ Settlement status updated to 'settled'")

	// Step 12: Update settlement amounts
	fmt.Println("🔄 Updating settlement amounts...")
	if err := h.paymentRepo.UpdateSettlementAmounts(ctx, pmt.ID, settlementAmount, mdrAmount, netSettlementAmount); err != nil {
		fmt.Printf("❌ Failed to update settlement amounts: %v\n", err)
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "error",
			"message": "Failed to update settlement amounts",
		})
	}

	fmt.Println("✅ Settlement amounts updated successfully")

	// Step 13: Log settlement received for audit trail
	paymentUUID, parseErr := uuid.Parse(pmt.ID)
	if parseErr != nil {
		fmt.Printf("⚠️  Failed to parse payment ID for logging: %v\n", parseErr)
		// Non-fatal: continue without logging
	} else {
		// Use SettlementID as UTR reference for JioPay
		settlementUTR := webhook.SettlementID
		if logErr := h.stateLogger.LogSettlementReceived(ctx, paymentUUID, netSettlementAmount, settlementUTR, settlementMetadata); logErr != nil {
			fmt.Printf("⚠️  Failed to log settlement received (non-fatal): %v\n", logErr)
			// Non-fatal: continue even if logging fails
		} else {
			fmt.Println("✅ Settlement event logged to audit trail")
		}
	}

	// Note: settled_at is automatically set by UpdateSettlementStatus when status becomes 'settled'

	fmt.Println("=== JIOPAY SETTLEMENT WEBHOOK: Processing complete ===")
	fmt.Printf("✅ Settlement processed: Payment %s → ₹%.2f (after ₹%.2f MDR)\n",
		pmt.ID, float64(netSettlementAmount)/100, float64(mdrAmount)/100)

	return c.JSON(http.StatusOK, map[string]string{
		"status":  "success",
		"message": "Settlement processed successfully",
	})
}

// handleEasebuzzSettlementWebhook processes settlement notifications from Easebuzz
// Sent when Easebuzz completes settlement to merchant account (T+1 to T+3 days)
//
// Key Differences from JioPay:
//   - Data format: Form-urlencoded (must parse to JSON)
//   - Batch processing: Multiple transactions in settled_transactions[] array
//   - Amount format: Numbers (int/float64), not strings
//   - Date format: "YYYY-MM-DD HH:MM:SS.SSSSSS"
func (h *WebhookHandler) handleEasebuzzSettlementWebhook(c echo.Context) error {
	ctx := c.Request().Context()

	fmt.Println("=== EASEBUZZ SETTLEMENT WEBHOOK: Processing batch settlement notification ===")

	// Step 1: Parse form-urlencoded data to JSON
	jsonData, err := parseFormEncodedWebhook(c)
	if err != nil {
		fmt.Printf("❌ Failed to parse form data: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid_form_data",
		})
	}

	fmt.Printf("📥 Parsed JSON data: %d bytes\n", len(jsonData))

	// Step 2: Unmarshal to settlement webhook response
	var webhook easebuzz.SettlementWebhookResponse
	if err := json.Unmarshal(jsonData, &webhook); err != nil {
		fmt.Printf("❌ Failed to unmarshal settlement webhook: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid_json_structure",
		})
	}

	// Step 3: Validate response status
	if webhook.Status != 1 {
		fmt.Printf("❌ Invalid webhook status: %d (expected 1)\n", webhook.Status)
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "error",
			"message": "Invalid webhook status",
		})
	}

	fmt.Printf("📋 PayoutID: %s\n", webhook.Data.PayoutID)
	fmt.Printf("📋 PayoutDate: %s\n", webhook.Data.PayoutDate)
	fmt.Printf("📋 Transactions to process: %d\n", len(webhook.Data.SettledTransactions))
	fmt.Printf("📋 Batch totals - Total: %.2f, Payout: %.2f, MDR: %.2f\n",
		webhook.Data.TotalAmount, webhook.Data.PayoutAmount, webhook.Data.ServiceChargeAmount)

	// Step 4: Hash validation (TODO - algorithm TBD for settlement webhooks)
	// For now, log the hash for reference and skip validation with warning
	fmt.Printf("⚠️  Hash validation skipped for settlement webhooks (algorithm TBD)\n")
	fmt.Printf("   Received hash: %s\n", webhook.Data.Hash)
	fmt.Printf("   TODO: Implement hash validation once settlement webhook hash algorithm confirmed\n")

	// Step 5: Parse payout date
	payoutDate, err := parseEasebuzzSettlementDate(webhook.Data.PayoutDate)
	if err != nil {
		fmt.Printf("❌ Failed to parse payout date: %v\n", err)
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "error",
			"message": "Invalid payout date format",
		})
	}

	fmt.Printf("📅 Payout date: %s\n", payoutDate.Format("2006-01-02 15:04:05"))

	// Step 6: Initialize batch processing counters
	totalCount := len(webhook.Data.SettledTransactions)
	successCount := 0
	skipCount := 0
	errorCount := 0

	fmt.Printf("\n🔄 Processing %d settlements in batch...\n\n", totalCount)

	// Step 7: Loop through settled_transactions array
	for idx, settlement := range webhook.Data.SettledTransactions {
		fmt.Printf("--- Processing settlement %d/%d: TxnID %s ---\n", idx+1, totalCount, settlement.TxnID)

		// Find payment by txnid (our payment_id)
		pmt, err := h.paymentRepo.GetPaymentByGatewayPaymentID(ctx, settlement.TxnID)
		if err != nil || pmt == nil {
			fmt.Printf("❌ Payment not found: %s, error: %v\n", settlement.TxnID, err)
			errorCount++
			continue
		}

		fmt.Printf("✅ Payment found: %s (status: %s, settlement_status: %s)\n",
			pmt.ID, pmt.Status, pmt.SettlementStatus)

		// Validate payment state
		if pmt.Status != payment.PaymentStatusSuccess {
			fmt.Printf("⚠️  Payment status is not 'success': %s (skipping)\n", pmt.Status)
			skipCount++
			continue
		}

		// Idempotency check - if already settled, skip
		if pmt.SettlementStatus == payment.SettlementStatusSettled {
			fmt.Printf("⚠️  Payment already settled (idempotent - skipping)\n")
			skipCount++
			continue
		}

		// Validate settlement_status is 'pending'
		if pmt.SettlementStatus != payment.SettlementStatusPending {
			fmt.Printf("⚠️  Settlement status is not 'pending': %s (skipping)\n", pmt.SettlementStatus)
			skipCount++
			continue
		}

		// Parse amounts (numbers to paise)
		settlementAmount := parseNumberToPaise(settlement.TransactionAmount)
		serviceCharge := parseNumberToPaise(settlement.ServiceCharge)
		serviceTax := parseNumberToPaise(settlement.ServiceTax)
		netAmount := parseNumberToPaise(settlement.SettlementAmount)

		// Calculate MDR (charges + GST)
		mdrAmount := serviceCharge + serviceTax

		// Validate calculation
		expectedNet := settlementAmount - mdrAmount
		if netAmount != expectedNet {
			fmt.Printf("⚠️  Settlement calculation mismatch! Expected: %d, Got: %d\n",
				expectedNet, netAmount)
		}

		fmt.Printf("💰 Settlement amounts (in paise):\n")
		fmt.Printf("   - Transaction Amount: %d (₹%.2f)\n", settlementAmount, float64(settlementAmount)/100)
		fmt.Printf("   - MDR Amount: %d (₹%.2f) [Charge: %d + Tax: %d]\n",
			mdrAmount, float64(mdrAmount)/100, serviceCharge, serviceTax)
		fmt.Printf("   - Net Settlement: %d (₹%.2f)\n", netAmount, float64(netAmount)/100)

		// Prepare settlement metadata
		settlementMetadata := map[string]any{
			"settlement_webhook_received_at": time.Now().Format(time.RFC3339),
			"settlement_gateway":             "easebuzz",
			"settlement_batch_id":            webhook.Data.PayoutID,
			"settlement_bank_transaction_id": webhook.Data.BankTransactionID,
			"settlement_bank_name":           webhook.Data.BankName,
			"settlement_account":             webhook.Data.AccountNumber,
			"settlement_ifsc":                webhook.Data.IFSCCode,
			"batch_total_amount":             webhook.Data.TotalAmount,
			"batch_payout_amount":            webhook.Data.PayoutAmount,
			"transaction_type":               settlement.TransactionType,
			"easepayid":                      settlement.EasepayID,
		}

		// Update settlement status to 'settled'
		if err := h.paymentRepo.UpdateSettlementStatus(ctx, pmt.ID, payment.SettlementStatusSettled, settlementMetadata); err != nil {
			fmt.Printf("❌ Failed to update settlement status: %v\n", err)
			errorCount++
			continue
		}

		// Update settlement amounts
		if err := h.paymentRepo.UpdateSettlementAmounts(ctx, pmt.ID, settlementAmount, mdrAmount, netAmount); err != nil {
			fmt.Printf("❌ Failed to update settlement amounts: %v\n", err)
			errorCount++
			continue
		}

		// Log settlement received for audit trail
		paymentUUID, parseErr := uuid.Parse(pmt.ID)
		if parseErr != nil {
			fmt.Printf("⚠️  Failed to parse payment ID for logging: %v\n", parseErr)
			// Non-fatal: continue without logging
		} else {
			// Use BankTransactionID as UTR reference for Easebuzz
			settlementUTR := webhook.Data.BankTransactionID
			if logErr := h.stateLogger.LogSettlementReceived(ctx, paymentUUID, netAmount, settlementUTR, settlementMetadata); logErr != nil {
				fmt.Printf("⚠️  Failed to log settlement received (non-fatal): %v\n", logErr)
				// Non-fatal: continue even if logging fails
			} else {
				fmt.Println("✅ Settlement event logged to audit trail")
			}
		}

		fmt.Printf("✅ Settlement processed successfully for %s\n\n", settlement.TxnID)
		successCount++
	}

	// Step 8: Return batch summary
	fmt.Println("=== EASEBUZZ SETTLEMENT WEBHOOK: Batch processing complete ===")
	fmt.Printf("📊 Summary: Total: %d, Processed: %d, Skipped: %d, Errors: %d\n",
		totalCount, successCount, skipCount, errorCount)

	return c.JSON(http.StatusOK, map[string]any{
		"status":  "success",
		"message": "Batch settlement processed",
		"summary": map[string]int{
			"total_transactions": totalCount,
			"processed":          successCount,
			"skipped":            skipCount,
			"errors":             errorCount,
		},
	})
}
