package payment

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/adapters/external/payment/easebuzz"
	"xcentro.in/internal/adapters/external/payment/jiopay"
	"xcentro.in/internal/core/payment"
)

// WebhookHandler handles payment gateway webhooks
// Supports both payment status webhooks and settlement webhooks (future)
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

// HandlePaymentStatusWebhook routes payment status webhooks to appropriate gateway handler
// Endpoint: POST /webhooks/payment_status?type=<gateway>
// Supports: type=jiopay, type=easebuzz
func (h *WebhookHandler) HandlePaymentStatusWebhook(c echo.Context) error {
	gatewayType := c.QueryParam("type")

	switch gatewayType {
	case "jiopay":
		return h.handleJioPayPaymentWebhook(c)
	case "easebuzz":
		return h.handleEasebuzzPaymentWebhook(c)
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{
			"status":  "error",
			"message": "Invalid or missing gateway type parameter. Supported: jiopay, easebuzz",
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
// TODO: Implement when Easebuzz settlement webhook format is provided
func (h *WebhookHandler) handleEasebuzzSettlementWebhook(c echo.Context) error {
	fmt.Println("⚠️  Easebuzz settlement webhook not yet implemented")

	return c.JSON(http.StatusNotImplemented, map[string]string{
		"status":  "error",
		"message": "Easebuzz settlement webhooks not yet implemented. Format specification pending.",
		"gateway": "easebuzz",
	})
}
