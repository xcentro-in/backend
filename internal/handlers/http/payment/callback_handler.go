package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/adapters/database/postgres/client"
	paymentdb "xcentro.in/internal/adapters/database/postgres/payment"
	voucherdb "xcentro.in/internal/adapters/database/postgres/voucher"
	"xcentro.in/internal/adapters/external/payment/easebuzz"
	"xcentro.in/internal/adapters/external/payment/jiopay"
	"xcentro.in/internal/adapters/external/payment/mockpg"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/business_rules"
	corenotification "xcentro.in/internal/core/notification"
	"xcentro.in/internal/core/payment"
	invoiceService "xcentro.in/internal/services/invoice"
	"xcentro.in/internal/services/notification"
	paymentSvc "xcentro.in/internal/services/payment"
	"xcentro.in/internal/shared/logger"
)

// CallbackHandler handles payment gateway callbacks (SURL/FURL)
// These are user-facing endpoints where gateways POST payment results
// after user completes payment. Response is HTML page for visual feedback.
type CallbackHandler struct {
	paymentRepo        payment.PaymentRepository
	storedCardRepo     payment.StoredCardRepository
	walletService      business_rules.WalletService
	jioPayGateway      payment.PaymentGateway
	easebuzzGateway    payment.PaymentGateway
	mockPGGateway      payment.PaymentGateway
	mockPGEnabled      bool
	pendingVoucherRepo *paymentdb.PaymentVoucherPendingRepository
	voucherRepo        *voucherdb.PostgresVoucherRepository
	txManager          client.TxManager
	stateChangeRepo    payment.PaymentStateChangeRepository
	stateLogger        *paymentSvc.PaymentStateLogger
	businessRepo       business.BusinessRepository
	invoiceService     *invoiceService.InvoiceService
	logger             *logger.Logger
}

// NewCallbackHandler creates a new callback handler
func NewCallbackHandler(
	paymentRepoParam payment.PaymentRepository,
	storedCardRepo payment.StoredCardRepository,
	walletService business_rules.WalletService,
	jioPayGateway payment.PaymentGateway,
	easebuzzGateway payment.PaymentGateway,
	mockPGGateway payment.PaymentGateway,
	mockPGEnabled bool,
	pendingVoucherRepo interface{},
	voucherRepo interface{},
	txManager interface{},
	stateChangeRepo payment.PaymentStateChangeRepository,
	businessRepo business.BusinessRepository,
	invoiceService *invoiceService.InvoiceService,
) *CallbackHandler {
	return &CallbackHandler{
		paymentRepo:        paymentRepoParam,
		storedCardRepo:     storedCardRepo,
		walletService:      walletService,
		jioPayGateway:      jioPayGateway,
		easebuzzGateway:    easebuzzGateway,
		mockPGGateway:      mockPGGateway,
		mockPGEnabled:      mockPGEnabled,
		pendingVoucherRepo: pendingVoucherRepo.(*paymentdb.PaymentVoucherPendingRepository),
		voucherRepo:        voucherRepo.(*voucherdb.PostgresVoucherRepository),
		txManager:          txManager.(client.TxManager),
		stateChangeRepo:    stateChangeRepo,
		stateLogger:        paymentSvc.NewPaymentStateLogger(stateChangeRepo),
		businessRepo:       businessRepo,
		invoiceService:     invoiceService,
		logger:             logger.GetDefault(),
	}
}

// HandleCallback handles payment gateway callbacks (SURL/FURL)
// Route: POST /api/v1/process_payment/callback?type=jiopay
// Route: POST /api/v1/process_payment/callback?type=easebuzz
// Route: POST /api/v1/process_payment/callback?type=mockpg
//
// This endpoint is called by payment gateways after user completes payment.
// It returns HTML page to show success/failure message to user.
func (h *CallbackHandler) HandleCallback(c echo.Context) error {
	gatewayType := c.QueryParam("type")

	h.logger.InfoWith("Callback received", "gateway_type", gatewayType)

	switch gatewayType {
	case "jiopay":
		return h.handleJioPayCallback(c)
	case "easebuzz":
		return h.handleEasebuzzCallback(c)
	case "mockpg":
		// Only allow mock PG callbacks if mock PG is enabled
		if !h.mockPGEnabled {
			fmt.Printf("❌ Mock PG callback received but Mock PG is not enabled\n")
			return c.HTML(http.StatusForbidden, generateErrorHTML("Mock PG is not enabled"))
		}
		return h.handleMockPGCallback(c)
	default:
		h.logger.ErrorWith("Invalid gateway type in callback", "gateway_type", gatewayType)
		return c.HTML(http.StatusBadRequest, generateErrorHTML("Invalid gateway type"))
	}
}

// handleJioPayCallback processes JioPay callback (SURL/FURL)
//
// Data Flow:
//   - merchantTxnNo in callback = OUR payment_id (use this to find the payment)
//   - txnID in callback = JioPay's transaction ID (store as gateway_payment_id)
//
// Payment Lookup:
//   - Use merchantTxnNo to find payment by payment_id
//   - Then update with JioPay's txnID as gateway_payment_id
func (h *CallbackHandler) handleJioPayCallback(c echo.Context) error {
	ctx := c.Request().Context()

	h.logger.Info("JioPay callback processing started")

	// Step 1: Read request body for signature verification
	bodyBytes, err := io.ReadAll(c.Request().Body)
	if err != nil {
		h.logger.ErrorWith("Failed to read JioPay callback body", "error", err)
		return c.HTML(http.StatusBadRequest, generateErrorHTML("Invalid request"))
	}

	h.logger.DebugWith("JioPay callback payload received", "payload_size", len(bodyBytes), "content_type", c.Request().Header.Get("Content-Type"))

	// Step 2: Parse callback payload from form data
	// JioPay sends application/x-www-form-urlencoded, not JSON
	formData, err := url.ParseQuery(string(bodyBytes))
	if err != nil {
		h.logger.ErrorWith("Failed to parse JioPay form data", "error", err)
		return c.HTML(http.StatusBadRequest, generateErrorHTML("Invalid form data"))
	}

	h.logger.DebugWith("JioPay callback form data parsed", "fields_count", len(formData))

	// Map form fields to CallbackPayload struct
	callback := jiopay.CallbackPayload{
		ResponseCode:     formData.Get("responseCode"),
		RespDescription:  formData.Get("respDescription"),
		MerchantID:       formData.Get("merchantId"),
		AggregatorID:     formData.Get("aggregatorID"),
		MerchantTxnNo:    formData.Get("merchantTxnNo"),
		SettlementStatus: formData.Get("settlementStatus"),
		TxnStatus:        formData.Get("txnStatus"),
		TxnResponseCode:  formData.Get("txnResponseCode"),
		TxnID:            formData.Get("txnID"),
		SettlementID:     formData.Get("settlementID"),
		SettlementDate:   formData.Get("settlementDate"),
		SecureHash:       formData.Get("secureHash"),
	}

	h.logger.DebugWith("JioPay callback parsed",
		"merchant_txn_no", callback.MerchantTxnNo,
		"txn_status", callback.TxnStatus,
		"response_code", callback.ResponseCode,
		"jiopay_txn_id", callback.TxnID)

	// Step 3: Verify signature (security validation)
	// Build field map from form data (excluding secureHash) for signature verification
	fieldMap := make(map[string]string)
	for key, values := range formData {
		if key != "secureHash" && len(values) > 0 {
			fieldMap[key] = values[0]
		}
	}

	// Validate signature using field map
	// Note: We can't use ValidateWebhook directly because it expects JSON, but we have form data
	// So we'll need to create a JSON representation or validate the hash directly
	// For now, convert field map to JSON for validation
	jsonPayload, err := json.Marshal(fieldMap)
	if err != nil {
		h.logger.ErrorWith("Failed to marshal callback data for validation", "error", err)
		return c.HTML(http.StatusBadRequest, generateErrorHTML("Invalid callback data"))
	}

	if err := h.jioPayGateway.ValidateWebhook(jsonPayload, callback.SecureHash); err != nil {
		h.logger.ErrorWith("JioPay signature verification failed", "error", err, "merchant_txn_no", callback.MerchantTxnNo)
		return c.HTML(http.StatusUnauthorized, generateErrorHTML("Security verification failed"))
	}

	h.logger.Debug("JioPay signature verified successfully")

	// Step 4: Query payment by merchantTxnNo (our transaction ID)
	pmt, err := h.paymentRepo.GetPaymentByGatewayPaymentID(ctx, callback.MerchantTxnNo)
	if err != nil || pmt == nil {
		h.logger.ErrorWith("Payment not found for JioPay callback", "merchant_txn_no", callback.MerchantTxnNo, "error", err)
		return c.HTML(http.StatusNotFound, generateErrorHTML("Payment not found"))
	}

	h.logger.DebugWith("Payment found for JioPay callback", "payment_id", pmt.ID, "current_status", pmt.Status)

	// Step 5: Map callback status to internal status
	newStatus := mapCallbackStatusToPaymentStatus(
		jiopay.MapJioPayCallbackStatus(callback.ResponseCode, callback.TxnStatus),
	)

	h.logger.DebugWith("JioPay status mapping",
		"response_code", callback.ResponseCode,
		"txn_status", callback.TxnStatus,
		"new_status", newStatus)

	// Step 6: Update payment (only if not already in terminal state)
	if !isTerminalStatus(pmt.Status) {
		h.logger.DebugWith("Updating JioPay payment status", "payment_id", pmt.ID, "new_status", newStatus)

		metadata := map[string]any{
			"callback_received_at": time.Now().Format(time.RFC3339),
			"callback_type":        "payment_status",
			"gateway":              "jiopay",
			"jiopay_txn_id":        callback.TxnID,
			"response_code":        callback.ResponseCode,
			"response_description": callback.RespDescription,
			"txn_status":           callback.TxnStatus,
			"txn_response_code":    callback.TxnResponseCode,
			"settlement_status":    callback.SettlementStatus,
			"settlement_id":        callback.SettlementID,
			"settlement_date":      callback.SettlementDate,
			"merchant_id":          callback.MerchantID,
		}

		if err := h.paymentRepo.UpdatePaymentStatus(ctx, pmt.ID, newStatus, metadata); err != nil {
			h.logger.ErrorWith("Failed to update JioPay payment status", "payment_id", pmt.ID, "error", err)
			// Don't fail callback - still show page to user
		} else {
			h.logger.InfoWith("JioPay payment status updated", "payment_id", pmt.ID, "new_status", newStatus)

			// Step 6.0: Update gateway payment ID with JioPay's txnID
			// This is JioPay's transaction ID (different from our merchantTxnNo)
			if callback.TxnID != "" {
				if err := h.paymentRepo.UpdateGatewayPaymentID(ctx, pmt.ID, callback.TxnID, pmt.RedirectURL); err != nil {
					h.logger.ErrorWith("Failed to update gateway payment ID", "payment_id", pmt.ID, "jiopay_txn_id", callback.TxnID, "error", err)
					// Don't fail - this is not critical for callback success
				} else {
					h.logger.DebugWith("Gateway payment ID updated", "payment_id", pmt.ID, "gateway_payment_id", callback.TxnID)
				}
			}

			// Step 6.1: Launch tokenization in background if this is a tokenization payment
			if pmt.PaymentType == payment.PaymentTypeTokenization && newStatus == payment.PaymentStatusSuccess {
				h.logger.InfoWith("JioPay tokenization payment succeeded, starting background tokenization", "payment_id", pmt.ID)
				// Launch goroutine to handle tokenization asynchronously
				go func(paymentCopy *payment.Payment) {
					// Create a new context with timeout for background work
					bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()

					h.logger.Info("Background card tokenization started")
					if err := h.handleTokenizationAfterPayment(bgCtx, paymentCopy); err != nil {
						h.logger.WarnWith("Background tokenization failed", "payment_id", paymentCopy.ID, "error", err)

						// Mark card as failed
						if paymentCopy.PaymentSourceID != nil {
							h.logger.ErrorWith("Marking card as failed", "card_id", *paymentCopy.PaymentSourceID)
							_ = h.storedCardRepo.UpdateCardStatus(bgCtx, *paymentCopy.PaymentSourceID, false, err.Error())
						}
					} else {
						h.logger.Info("Background card tokenization completed successfully")
					}
				}(pmt)
			} else if pmt.PaymentType == payment.PaymentTypeTokenization && newStatus == payment.PaymentStatusFailed {
				// Step 6.1b: Mark card as inactive if tokenization payment failed
				h.logger.ErrorWith("JioPay tokenization payment failed, marking card as inactive", "payment_id", pmt.ID)
				go func(paymentCopy *payment.Payment) {
					bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()

					if err := h.handleTokenizationFailure(bgCtx, paymentCopy, callback.RespDescription); err != nil {
						h.logger.WarnWith("Background failed to mark card inactive", "payment_id", paymentCopy.ID, "error", err)
					} else {
						h.logger.Info("Background card marked as inactive")
					}
				}(pmt)
			}

			// Step 6.2: Handle post-update operations (state changes, voucher finalization)
			if err := h.handlePostPaymentUpdate(ctx, pmt, newStatus, "jiopay", metadata); err != nil {
				h.logger.WarnWith("JioPay post-update operations had issues", "payment_id", pmt.ID, "error", err)
				// Non-fatal: callback continues
			}
		}

		// Update gateway payment ID if not set
		if pmt.GatewayPaymentID == "" || pmt.GatewayPaymentID == callback.MerchantTxnNo {
			if err := h.paymentRepo.UpdateGatewayPaymentID(ctx, pmt.ID, callback.TxnID, ""); err != nil {
				h.logger.WarnWith("Failed to update JioPay gateway payment ID", "payment_id", pmt.ID, "error", err)
			} else {
				h.logger.DebugWith("JioPay gateway payment ID updated", "payment_id", pmt.ID, "gateway_txn_id", callback.TxnID)
			}
		}
	} else {
		h.logger.WarnWith("JioPay payment already in terminal state, skipping update", "payment_id", pmt.ID, "current_status", pmt.Status)
	}

	// Step 7: Send 200 OK response immediately (before tokenization completes)
	h.logger.InfoWith("JioPay callback processing complete, sending response", "payment_id", pmt.ID, "status", newStatus)

	switch newStatus {
	case payment.PaymentStatusSuccess:
		return c.HTML(http.StatusOK, generateSuccessHTML(pmt, callback.MerchantTxnNo, callback.RespDescription))
	case payment.PaymentStatusPending:
		return c.HTML(http.StatusOK, generatePendingHTML(pmt, callback.MerchantTxnNo, callback.RespDescription))
	default:
		return c.HTML(http.StatusOK, generateFailureHTML(pmt, callback.MerchantTxnNo, callback.RespDescription))
	}
}

// handleEasebuzzCallback processes Easebuzz callback (SURL/FURL)
func (h *CallbackHandler) handleEasebuzzCallback(c echo.Context) error {
	ctx := c.Request().Context()

	h.logger.Info("Easebuzz callback processing started")

	// Step 1: Read request body
	bodyBytes, err := io.ReadAll(c.Request().Body)
	if err != nil {
		h.logger.ErrorWith("Failed to read Easebuzz callback body", "error", err)
		return c.HTML(http.StatusBadRequest, generateErrorHTML("Invalid request"))
	}

	h.logger.DebugWith("Easebuzz callback payload received", "payload_size", len(bodyBytes))

	// Step 2: Parse callback payload
	var callback easebuzz.CallbackPayload
	if err := json.Unmarshal(bodyBytes, &callback); err != nil {
		h.logger.ErrorWith("Failed to parse Easebuzz callback payload", "error", err)
		return c.HTML(http.StatusBadRequest, generateErrorHTML("Invalid payload format"))
	}

	h.logger.DebugWith("Easebuzz callback parsed",
		"txn_id", callback.TxnID,
		"status", callback.Status,
		"amount", callback.Amount,
		"easepay_id", callback.EasepayID)

	// Step 3: Verify signature (security validation)
	if err := h.easebuzzGateway.ValidateWebhook(bodyBytes, callback.Hash); err != nil {
		h.logger.ErrorWith("Easebuzz signature verification failed", "error", err, "txn_id", callback.TxnID)
		return c.HTML(http.StatusUnauthorized, generateErrorHTML("Security verification failed"))
	}

	h.logger.Debug("Easebuzz signature verified successfully")

	// Step 4: Query payment by txnid (our transaction ID)
	pmt, err := h.paymentRepo.GetPaymentByGatewayPaymentID(ctx, callback.TxnID)
	if err != nil || pmt == nil {
		h.logger.ErrorWith("Payment not found for Easebuzz callback", "txn_id", callback.TxnID, "error", err)
		return c.HTML(http.StatusNotFound, generateErrorHTML("Payment not found"))
	}

	h.logger.DebugWith("Payment found for Easebuzz callback", "payment_id", pmt.ID, "current_status", pmt.Status)

	// Step 5: Map callback status to internal status
	newStatus := mapCallbackStatusToPaymentStatus(
		easebuzz.MapEasebuzzCallbackStatus(callback.Status),
	)

	h.logger.DebugWith("Easebuzz status mapping", "gateway_status", callback.Status, "new_status", newStatus)

	// Step 6: Update payment (only if not already in terminal state)
	if !isTerminalStatus(pmt.Status) {
		h.logger.DebugWith("Updating Easebuzz payment status", "payment_id", pmt.ID, "new_status", newStatus)

		metadata := map[string]any{
			"callback_received_at": time.Now().Format(time.RFC3339),
			"callback_type":        "payment_status",
			"gateway":              "easebuzz",
			"easepayid":            callback.EasepayID,
			"bank_ref_num":         callback.BankRefNum,
			"payment_source":       callback.PaymentSource,
			"card_type":            callback.CardType,
			"mode":                 callback.Mode,
			"net_amount_debit":     callback.NetAmountDebit,
			"deduction_percentage": callback.DeductionPercentage,
			"cash_back_percentage": callback.CashBackPercentage,
			"error":                callback.Error,
			"error_message":        callback.ErrorMessage,
			"added_on":             callback.AddedOn,
			"response_code":        callback.ResponseCode,
			"card_num":             callback.CardNum,
			"bank_name":            callback.BankName,
			"auth_code":            callback.AuthCode,
		}

		if err := h.paymentRepo.UpdatePaymentStatus(ctx, pmt.ID, newStatus, metadata); err != nil {
			h.logger.ErrorWith("Failed to update Easebuzz payment status", "payment_id", pmt.ID, "error", err)
			// Don't fail callback - still show page to user
		} else {
			h.logger.InfoWith("Easebuzz payment status updated", "payment_id", pmt.ID, "new_status", newStatus)

			// Step 6.1: Handle post-update operations (state changes, voucher finalization)
			if err := h.handlePostPaymentUpdate(ctx, pmt, newStatus, "easebuzz", metadata); err != nil {
				h.logger.WarnWith("Easebuzz post-update operations had issues", "payment_id", pmt.ID, "error", err)
				// Non-fatal: callback continues
			}
		}

		// Update gateway payment ID if not set or if it's still our txnid
		if pmt.GatewayPaymentID == "" || pmt.GatewayPaymentID == callback.TxnID {
			if err := h.paymentRepo.UpdateGatewayPaymentID(ctx, pmt.ID, callback.EasepayID, ""); err != nil {
				h.logger.WarnWith("Failed to update Easebuzz gateway payment ID", "payment_id", pmt.ID, "error", err)
			} else {
				h.logger.DebugWith("Easebuzz gateway payment ID updated", "payment_id", pmt.ID, "easepay_id", callback.EasepayID)
			}
		}
	} else {
		h.logger.WarnWith("Easebuzz payment already in terminal state, skipping update", "payment_id", pmt.ID, "current_status", pmt.Status)
	}

	// Step 7: Return HTML response to user
	h.logger.InfoWith("Easebuzz callback processing complete, sending response", "payment_id", pmt.ID, "status", newStatus)

	switch newStatus {
	case payment.PaymentStatusSuccess:
		return c.HTML(http.StatusOK, generateSuccessHTML(pmt, callback.TxnID, callback.Error))
	case payment.PaymentStatusPending:
		return c.HTML(http.StatusOK, generatePendingHTML(pmt, callback.TxnID, callback.Error))
	default:
		return c.HTML(http.StatusOK, generateFailureHTML(pmt, callback.TxnID, callback.ErrorMessage))
	}
}

// handleMockPGCallback processes Mock PG callback (testing only)
// This simulates the callback flow that real gateways use
func (h *CallbackHandler) handleMockPGCallback(c echo.Context) error {
	ctx := c.Request().Context()

	fmt.Println("=== MOCK PG CALLBACK: Processing ===")

	// Step 1: Read form data (traditional gateway POST)
	payloadStr := c.FormValue("payload")
	if payloadStr == "" {
		fmt.Println("❌ Missing payload in form data")
		return c.HTML(http.StatusBadRequest, generateErrorHTML("Invalid request"))
	}

	fmt.Printf("📥 Callback payload size: %d bytes\n", len(payloadStr))

	// Step 2: Parse callback payload (JSON string from form data)
	var callback mockpg.MockPGWebhookPayload
	if err := json.Unmarshal([]byte(payloadStr), &callback); err != nil {
		fmt.Printf("❌ Failed to parse payload: %v\n", err)
		return c.HTML(http.StatusBadRequest, generateErrorHTML("Invalid payload format"))
	}

	fmt.Printf("📋 TransactionID: %s\n", callback.TransactionID)
	fmt.Printf("📋 GatewayPaymentID: %s\n", callback.GatewayPaymentID)
	fmt.Printf("📋 Status: %s\n", callback.Status)
	fmt.Printf("📋 Amount: %d %s\n", callback.Amount, callback.Currency)

	// Step 3: Verify signature (mock validation)
	if err := h.mockPGGateway.ValidateWebhook([]byte(payloadStr), callback.Signature); err != nil {
		fmt.Printf("❌ Signature verification failed: %v\n", err)
		return c.HTML(http.StatusUnauthorized, generateErrorHTML("Security verification failed"))
	}

	fmt.Println("✅ Signature verified")

	// Step 4: Query payment by transaction ID
	pmt, err := h.paymentRepo.GetPaymentByID(ctx, callback.TransactionID)
	if err != nil || pmt == nil {
		fmt.Printf("❌ Payment not found: %s, error: %v\n", callback.TransactionID, err)
		return c.HTML(http.StatusNotFound, generateErrorHTML("Payment not found"))
	}

	fmt.Printf("✅ Payment found: %s (current status: %s)\n", pmt.ID, pmt.Status)

	// Step 5: Map callback status to internal status
	var newStatus payment.PaymentStatus
	if callback.Status == string(payment.PaymentStatusSuccess) {
		newStatus = payment.PaymentStatusSuccess
	} else {
		newStatus = payment.PaymentStatusFailed
	}

	fmt.Printf("📊 Status mapping: %s → %s\n", callback.Status, newStatus)

	// Step 6: Update payment (only if not already in terminal state)
	if !isTerminalStatus(pmt.Status) {
		fmt.Println("🔄 Updating payment status...")

		metadata := map[string]any{
			"callback_received_at":    time.Now().Format(time.RFC3339),
			"callback_type":           "payment_status",
			"gateway":                 "mockpg",
			"mock_gateway_payment_id": callback.GatewayPaymentID,
			"payment_method":          callback.PaymentMethod,
			"processed_at":            callback.ProcessedAt,
			"merchant_id":             callback.MerchantID,
			"mock_simulation":         true,
		}

		if err := h.paymentRepo.UpdatePaymentStatus(ctx, pmt.ID, newStatus, metadata); err != nil {
			fmt.Printf("❌ Failed to update payment: %v\n", err)
			// Don't fail callback - still show page to user
		} else {
			fmt.Printf("✅ Payment status updated to: %s\n", newStatus)

			// Step 6.1: Launch tokenization in background if this is a tokenization payment
			if pmt.PaymentType == payment.PaymentTypeTokenization && newStatus == payment.PaymentStatusSuccess {
				fmt.Println("🔑 TOKENIZATION PAYMENT: Will handle tokenization in background after sending response")
				// Launch goroutine to handle tokenization asynchronously
				go func(paymentCopy *payment.Payment) {
					// Create a new context with timeout for background work
					bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()

					fmt.Println("🔑 [Background] Starting card tokenization...")
					if err := h.handleTokenizationAfterPayment(bgCtx, paymentCopy); err != nil {
						fmt.Printf("⚠️  [Background] Tokenization failed: %v\n", err)

						// Mark card as failed
						if paymentCopy.PaymentSourceID != nil {
							fmt.Printf("❌ Marking card as failed: %s\n", *paymentCopy.PaymentSourceID)
							_ = h.storedCardRepo.UpdateCardStatus(bgCtx, *paymentCopy.PaymentSourceID, false, err.Error())
						}
					} else {
						fmt.Println("✅ [Background] Card tokenized successfully")
					}
				}(pmt)
			} else if pmt.PaymentType == payment.PaymentTypeTokenization && newStatus == payment.PaymentStatusFailed {
				// Step 6.1b: Mark card as inactive if tokenization payment failed
				fmt.Println("❌ TOKENIZATION PAYMENT FAILED: Marking card as inactive")
				go func(paymentCopy *payment.Payment) {
					bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()

					if err := h.handleTokenizationFailure(bgCtx, paymentCopy, "Payment failed"); err != nil {
						fmt.Printf("⚠️  [Background] Failed to mark card inactive: %v\n", err)
					} else {
						fmt.Println("✅ [Background] Card marked as inactive")
					}
				}(pmt)
			}

			// Step 6.2: Handle post-update operations (state changes, voucher finalization)
			if err := h.handlePostPaymentUpdate(ctx, pmt, newStatus, "mockpg", metadata); err != nil {
				fmt.Printf("⚠️  Post-update operations had issues: %v\n", err)
				// Non-fatal: callback continues
			}
		}

		// Update gateway payment ID if not set
		if pmt.GatewayPaymentID == "" || pmt.GatewayPaymentID != callback.GatewayPaymentID {
			if err := h.paymentRepo.UpdateGatewayPaymentID(ctx, pmt.ID, callback.GatewayPaymentID, ""); err != nil {
				fmt.Printf("⚠️  Failed to update gateway payment ID: %v\n", err)
			} else {
				fmt.Printf("✅ Gateway payment ID updated to: %s\n", callback.GatewayPaymentID)
			}
		}
	} else {
		fmt.Printf("⚠️  Payment already in terminal state: %s (skipping update)\n", pmt.Status)
	}

	// Step 7: Send 200 OK response immediately (before tokenization completes)
	fmt.Println("=== MOCK PG CALLBACK: Sending response to gateway ===")

	switch newStatus {
	case payment.PaymentStatusSuccess:
		return c.HTML(http.StatusOK, generateSuccessHTML(pmt, callback.TransactionID, "Payment completed successfully"))
	case payment.PaymentStatusPending:
		return c.HTML(http.StatusOK, generatePendingHTML(pmt, callback.TransactionID, "Payment is being processed"))
	default:
		return c.HTML(http.StatusOK, generateFailureHTML(pmt, callback.TransactionID, "Payment failed"))
	}
}

// handleTokenizationAfterPayment handles card tokenization for immature PGs (Mock PG, JioPay)
// Called after successful ₹1 tokenization payment
// PHASE 2: Updates the stored_card record created in Phase 1 with token and issuing bank
func (h *CallbackHandler) handleTokenizationAfterPayment(ctx context.Context, pmt *payment.Payment) error {
	h.logger.InfoWith("Tokenization phase 2 started", "payment_id", pmt.ID)

	// Step 1: Get the stored_card_id from payment.payment_source_id
	if pmt.PaymentSourceID == nil {
		h.logger.ErrorWith("Payment has no payment_source_id, cannot track stored_card", "payment_id", pmt.ID)
		return fmt.Errorf("payment has no payment_source_id")
	}

	storedCardID := *pmt.PaymentSourceID
	h.logger.DebugWith("Payment linked to stored_card", "payment_id", pmt.ID, "stored_card_id", storedCardID)

	// Step 2: Load the existing stored_card record (created in Phase 1)
	storedCard, err := h.storedCardRepo.GetStoredCardByID(ctx, storedCardID)
	if err != nil {
		h.logger.ErrorWith("Failed to load stored_card", "stored_card_id", storedCardID, "error", err)
		return fmt.Errorf("failed to load stored_card: %w", err)
	}
	if storedCard == nil {
		fmt.Printf("❌ Stored card not found: %s\n", storedCardID)
		return fmt.Errorf("stored card not found: %s", storedCardID)
	}

	h.logger.DebugWith("Found stored_card",
		"stored_card_id", storedCardID,
		"card_category", storedCard.CardCategory,
		"card_type", storedCard.CardType,
		"last_four_digits", storedCard.LastFourDigits)

	// Step 3: Build tokenization request
	// PCI DSS: We do NOT send full card number - gateway already has it from payment transaction
	// Gateway will tokenize based on transaction_id (references the payment where card was used)
	tokenizeReq := payment.GatewayTokenizeRequest{
		BusinessID:    pmt.BusinessID,
		TransactionID: pmt.ID, // Gateway uses this to reference the card from payment
		CardDetails: payment.Card{
			// Card details for reference/verification only - gateway already has full card number
			LastFourDigits: storedCard.LastFourDigits,
			ExpiryMonth:    storedCard.ExpiryMonth,
			ExpiryYear:     storedCard.ExpiryYear,
			CardType:       storedCard.CardType,
			CardCategory:   storedCard.CardCategory,
			HolderName:     storedCard.CardHolderName,
			// Number field is intentionally empty - PCI DSS compliant
		},
		CallbackURL: pmt.CallbackURL,
	}

	h.logger.DebugWith("Tokenization request built (PCI compliant)", "payment_id", pmt.ID, "transaction_id", pmt.ID)

	// Step 4: Call gateway tokenization API
	var tokenizeResp *payment.GatewayTokenizeResponse

	switch pmt.GatewayUsed {
	case payment.GatewayMockPG:
		fmt.Println("🔑 Calling Mock PG tokenization API...")
		tokenizeResp, err = h.mockPGGateway.TokenizeCard(ctx, tokenizeReq)
	case payment.GatewayJioPay:
		h.logger.DebugWith("Calling JioPay tokenization API", "payment_id", pmt.ID)
		tokenizeResp, err = h.jioPayGateway.TokenizeCard(ctx, tokenizeReq)
	case payment.GatewayEasebuzz:
		h.logger.DebugWith("Calling Easebuzz tokenization API", "payment_id", pmt.ID)
		tokenizeResp, err = h.easebuzzGateway.TokenizeCard(ctx, tokenizeReq)
	default:
		h.logger.ErrorWith("Unknown gateway for tokenization", "payment_id", pmt.ID, "gateway", pmt.GatewayUsed)
		return fmt.Errorf("unknown gateway: %s", pmt.GatewayUsed)
	}

	if err != nil {
		h.logger.ErrorWith("Tokenization API call failed", "payment_id", pmt.ID, "gateway", pmt.GatewayUsed, "error", err)
		return fmt.Errorf("tokenization failed: %w", err)
	}

	h.logger.InfoWith("Tokenization API call successful",
		"payment_id", pmt.ID,
		"gateway_card_id", tokenizeResp.CardID)

	// Step 5: Complete card tokenization via WalletService
	// This updates the stored_card record AND sets hasAddedPaymentMethod flag
	// Ensures consistent behavior with AddPaymentSource (bank accounts)
	h.logger.DebugWith("Completing card tokenization via WalletService", "stored_card_id", storedCardID)
	err = h.walletService.CompleteCardTokenization(
		ctx,
		pmt.BusinessID,
		storedCardID,
		tokenizeResp.Token,
		pmt.GatewayUsed,
		tokenizeResp.CardID,
	)
	if err != nil {
		h.logger.ErrorWith("Failed to complete card tokenization", "stored_card_id", storedCardID, "error", err)
		return fmt.Errorf("failed to complete card tokenization: %w", err)
	}

	h.logger.InfoWith("Card tokenization phase 2 completed successfully",
		"stored_card_id", storedCardID,
		"payment_id", pmt.ID)

	return nil
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

// isTerminalStatus checks if payment is in a terminal state (no further updates)
func isTerminalStatus(status payment.PaymentStatus) bool {
	return status == payment.PaymentStatusSuccess ||
		status == payment.PaymentStatusFailed ||
		status == payment.PaymentStatusCancelled
}

// mapCallbackStatusToPaymentStatus converts string status to PaymentStatus enum
func mapCallbackStatusToPaymentStatus(status string) payment.PaymentStatus {
	switch status {
	case string(payment.PaymentStatusSuccess):
		return payment.PaymentStatusSuccess
	case string(payment.PaymentStatusPending):
		return payment.PaymentStatusPending
	case string(payment.PaymentStatusFailed):
		return payment.PaymentStatusFailed
	default:
		return payment.PaymentStatusFailed
	}
}

// finalizeVouchersOnSuccess finalizes vouchers when payment succeeds
// This is called from callback handlers after payment status is updated to SUCCESS
// It fetches pending vouchers, finalizes them, and cleans up pending records - all in a transaction
func (h *CallbackHandler) finalizeVouchersOnSuccess(ctx context.Context, paymentID string, gatewayName string) error {
	h.logger.InfoWith("Checking for pending vouchers to finalize", "payment_id", paymentID)

	// Parse payment ID
	paymentUUID, err := uuid.Parse(paymentID)
	if err != nil {
		h.logger.ErrorWith("Invalid payment ID", "payment_id", paymentID, "error", err)
		return fmt.Errorf("invalid payment ID: %w", err)
	}

	// Step 1: Get pending vouchers for this payment
	voucherIDs, err := h.pendingVoucherRepo.GetPendingVouchers(ctx, paymentUUID)
	if err != nil {
		h.logger.ErrorWith("Failed to fetch pending vouchers", "error", err)
		return fmt.Errorf("failed to fetch pending vouchers: %w", err)
	}

	if len(voucherIDs) == 0 {
		h.logger.InfoWith("No pending vouchers found for payment", "payment_id", paymentID)
		return nil // No vouchers to finalize
	}

	h.logger.InfoWith("Found pending vouchers, starting finalization", "payment_id", paymentID, "voucher_count", len(voucherIDs))

	// Step 2: Finalize vouchers and cleanup in a single transaction
	err = h.txManager.WithTx(ctx, func(txCtx context.Context, tx client.Tx) error {
		// Create transactional repositories
		txVoucherRepo := h.voucherRepo.WithTx(tx)
		txPendingRepo := h.pendingVoucherRepo.WithTx(tx)

		// Finalize each voucher
		for _, voucherID := range voucherIDs {
			h.logger.DebugWith("Finalizing voucher", "voucher_id", voucherID.String(), "payment_id", paymentID)
			if err := txVoucherRepo.FinalizeVoucher(txCtx, voucherID, paymentUUID); err != nil {
				h.logger.ErrorWith("Failed to finalize voucher", "voucher_id", voucherID.String(), "error", err)
				return fmt.Errorf("failed to finalize voucher %s: %w", voucherID.String(), err)
			}
		}

		h.logger.InfoWith("All vouchers finalized successfully", "count", len(voucherIDs))

		// Delete pending voucher records (cleanup)
		h.logger.DebugWith("Deleting pending voucher records", "payment_id", paymentID)
		if err := txPendingRepo.DeletePendingVouchers(txCtx, paymentUUID); err != nil {
			h.logger.ErrorWith("Failed to delete pending vouchers", "error", err)
			return fmt.Errorf("failed to delete pending vouchers: %w", err)
		}

		h.logger.InfoWith("Pending voucher records deleted", "payment_id", paymentID)
		return nil
	})

	if err != nil {
		h.logger.ErrorWith("Voucher finalization transaction failed", "error", err)
		return fmt.Errorf("voucher finalization failed: %w", err)
	}

	h.logger.InfoWith("Voucher finalization completed successfully", "payment_id", paymentID, "voucher_count", len(voucherIDs))
	return nil
}

// handlePostPaymentUpdate handles common post-payment-update operations for all gateways
// This includes: 1) Recording state changes in audit trail, 2) Finalizing vouchers on success
// This method is non-fatal - errors are logged but don't fail the callback
func (h *CallbackHandler) handlePostPaymentUpdate(
	ctx context.Context,
	pmt *payment.Payment,
	newStatus payment.PaymentStatus,
	gatewayName string,
	metadata map[string]any,
) error {
	// Step 1: Record payment state change in audit trail using PaymentStateLogger
	paymentUUID, err := uuid.Parse(pmt.ID)
	if err != nil {
		h.logger.ErrorWith("Invalid payment ID", "payment_id", pmt.ID, "error", err)
		return fmt.Errorf("invalid payment ID: %w", err)
	}

	// Log state change based on payment status
	switch newStatus {
	case payment.PaymentStatusSuccess:
		// Get gateway payment ID from metadata or payment object
		gatewayPaymentID := pmt.GatewayPaymentID
		if gatewayPaymentID == "" {
			gatewayPaymentID = pmt.ID // fallback to payment ID
		}

		if err := h.stateLogger.LogPaymentSuccess(ctx, paymentUUID, pmt.TotalAmount, gatewayName, gatewayPaymentID, metadata); err != nil {
			h.logger.ErrorWith("Failed to record payment success (non-fatal)", "payment_id", pmt.ID, "gateway", gatewayName, "error", err)
			// Non-fatal: Continue to voucher finalization
		} else {
			h.logger.InfoWith("Payment success recorded", "payment_id", pmt.ID, "gateway", gatewayName)
		}

	case payment.PaymentStatusFailed:
		// Extract failure reason from metadata
		failureReason := "Payment failed"
		if msg, ok := metadata["error_message"].(string); ok && msg != "" {
			failureReason = msg
		} else if msg, ok := metadata["response_description"].(string); ok && msg != "" {
			failureReason = msg
		}

		if err := h.stateLogger.LogPaymentFailed(ctx, paymentUUID, gatewayName, failureReason, metadata); err != nil {
			h.logger.ErrorWith("Failed to record payment failure (non-fatal)", "payment_id", pmt.ID, "gateway", gatewayName, "error", err)
			// Non-fatal: Continue
		} else {
			h.logger.InfoWith("Payment failure recorded", "payment_id", pmt.ID, "gateway", gatewayName)
		}

	default:
		// For pending or other statuses, skip logging for now
		h.logger.InfoWith("Payment status changed but not logging (pending/unknown)", "payment_id", pmt.ID, "status", newStatus)
	}

	// Step 2: Create invoice if payment succeeded
	if newStatus == payment.PaymentStatusSuccess {
		// Step 2: Create invoice (if payment succeeded)
		h.logger.InfoWith("Payment succeeded, attempting invoice creation", "payment_id", pmt.ID, "gateway", gatewayName)
		fmt.Println("📄 Payment succeeded, creating invoice...")

		if h.invoiceService != nil && h.businessRepo != nil {
			// Fetch business to get GSTIN
			bizEntity, err := h.businessRepo.FindByID(ctx, pmt.BusinessID)
			if err != nil {
				h.logger.ErrorWith("Failed to fetch business for invoice creation (non-fatal)", "payment_id", pmt.ID, "gateway", gatewayName, "error", err)
				fmt.Printf("⚠️  Failed to fetch business for invoice creation: %v\n", err)
			} else if bizEntity != nil && bizEntity.GSTIN != "" {
				inv, err := h.invoiceService.CreateInvoiceForPayment(ctx, pmt.ID, bizEntity.GSTIN, pmt.ConvenienceFee, pmt.ConvenienceFeeGST)
				if err != nil {
					// Check if it's a verification error (invoice exists but doesn't match)
					if inv != nil {
						h.logger.ErrorWith("Invoice verification failed - existing invoice doesn't match payment (non-fatal)",
							"payment_id", pmt.ID,
							"invoice_id", inv.InvoiceID,
							"gateway", gatewayName,
							"error", err)
						fmt.Printf("❌ Invoice verification failed: %v (InvoiceID: %d)\n", err, inv.InvoiceID)
					} else {
						h.logger.ErrorWith("Failed to create invoice (non-fatal)", "payment_id", pmt.ID, "gateway", gatewayName, "error", err)
						fmt.Printf("⚠️  Failed to create invoice: %v\n", err)
					}
				} else {
					if inv != nil {
						h.logger.InfoWith("Invoice processed successfully", "payment_id", pmt.ID, "invoice_id", inv.InvoiceID, "gateway", gatewayName)
						fmt.Printf("✅ Invoice processed successfully (InvoiceID: %d)\n", inv.InvoiceID)
					}
				}
			} else {
				h.logger.WarnWith("Business GSTIN not available for invoice creation", "payment_id", pmt.ID, "gateway", gatewayName)
				fmt.Printf("⚠️  Business GSTIN not available for invoice creation\n")
			}
		}

		// Step 3: Finalize vouchers
		h.logger.InfoWith("Attempting voucher finalization", "payment_id", pmt.ID, "gateway", gatewayName)

		if err := h.finalizeVouchersOnSuccess(ctx, pmt.ID, gatewayName); err != nil {
			h.logger.ErrorWith("Failed to finalize vouchers (non-fatal)", "payment_id", pmt.ID, "gateway", gatewayName, "error", err)
			// Non-fatal: Payment is still successful, vouchers can be finalized manually or by recovery job
		} else {
			h.logger.InfoWith("Vouchers finalized successfully", "payment_id", pmt.ID, "gateway", gatewayName)
		}

		// Step 3: Send notification to owner about successful payment
		// Only notify for actual business payments, not internal tokenization payments
		if pmt.PaymentType != payment.PaymentTypeTokenization {
			h.logger.InfoWith("Triggering payment success notification", "payment_id", pmt.ID, "business_id", pmt.BusinessID, "payment_type", pmt.PaymentType)

			// Format amount for display (convert paise to rupees)
			amountINR := float64(pmt.TotalAmount) / 100.0

			// Create notification payload
			paymentIDStr := pmt.ID
			err := notification.TriggerMulticast(
				ctx,
				pmt.BusinessID,                         // business_id
				corenotification.EventPaymentCompleted, // event_type
				"Payment Successful",                   // title
				fmt.Sprintf("Payment of ₹%.2f has been processed successfully", amountINR), // body
				corenotification.RoleOwner, // minimum_role (owner only)
				nil,
				corenotification.EntityReferences{ // entities
					BusinessID: &pmt.BusinessID,
					PaymentID:  &paymentIDStr,
				},
				[]corenotification.ActionButton{ // actions
					{
						Type: corenotification.ButtonTypeNeutral,
						Text: "View Details",
						Href: fmt.Sprintf("/payments/%s", pmt.ID),
					},
				},
			)

			if err != nil {
				h.logger.WarnWith("Failed to trigger payment success notification (non-fatal)", "payment_id", pmt.ID, "error", err)
				// Non-fatal: Payment is still successful
			} else {
				h.logger.InfoWith("Payment success notification triggered", "payment_id", pmt.ID)
			}
		}
	}

	return nil
}

// handleTokenizationFailure marks card as inactive when tokenization payment fails
// Called after failed tokenization payment (FURL callback)
func (h *CallbackHandler) handleTokenizationFailure(ctx context.Context, pmt *payment.Payment, failureReason string) error {
	h.logger.InfoWith("Tokenization failure handler started", "payment_id", pmt.ID, "failure_reason", failureReason)

	// Step 1: Get the stored_card_id from payment.payment_source_id
	if pmt.PaymentSourceID == nil || *pmt.PaymentSourceID == "" {
		h.logger.WarnWith("No payment_source_id found, card might not have been created yet", "payment_id", pmt.ID)
		return nil // Non-fatal: card might not have been created yet
	}

	storedCardID := *pmt.PaymentSourceID
	h.logger.DebugWith("Payment linked to stored_card for failure handling", "payment_id", pmt.ID, "stored_card_id", storedCardID)

	// Step 2: Update card status to inactive with failure reason
	if err := h.storedCardRepo.UpdateCardStatus(ctx, storedCardID, false, failureReason); err != nil {
		h.logger.ErrorWith("Failed to update card status", "stored_card_id", storedCardID, "error", err)
		return fmt.Errorf("failed to update card status: %w", err)
	}

	h.logger.InfoWith("Card marked as inactive due to tokenization failure",
		"stored_card_id", storedCardID,
		"failure_reason", failureReason)

	return nil
}

// ============================================================================
// HTML RESPONSE GENERATORS
// ============================================================================

// generateSuccessHTML creates success page HTML
func generateSuccessHTML(pmt *payment.Payment, txnRef, message string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Payment Successful</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 20px;
        }
        .container {
            background: white;
            border-radius: 16px;
            box-shadow: 0 20px 60px rgba(0,0,0,0.3);
            max-width: 500px;
            width: 100%%;
            padding: 40px;
            text-align: center;
        }
        .icon {
            width: 80px;
            height: 80px;
            background: #4CAF50;
            border-radius: 50%%;
            display: flex;
            align-items: center;
            justify-content: center;
            margin: 0 auto 24px;
            animation: scaleIn 0.5s ease-out;
        }
        .icon svg {
            width: 48px;
            height: 48px;
            stroke: white;
            stroke-width: 3;
            stroke-linecap: round;
            stroke-linejoin: round;
            fill: none;
        }
        @keyframes scaleIn {
            from { transform: scale(0); }
            to { transform: scale(1); }
        }
        h1 {
            color: #333;
            font-size: 28px;
            margin-bottom: 12px;
            font-weight: 600;
        }
        .message {
            color: #666;
            font-size: 16px;
            margin-bottom: 32px;
            line-height: 1.5;
        }
        .details {
            background: #f5f5f5;
            border-radius: 12px;
            padding: 24px;
            margin-bottom: 32px;
            text-align: left;
        }
        .detail-row {
            display: flex;
            justify-content: space-between;
            padding: 12px 0;
            border-bottom: 1px solid #e0e0e0;
        }
        .detail-row:last-child {
            border-bottom: none;
        }
        .detail-label {
            color: #666;
            font-size: 14px;
            font-weight: 500;
        }
        .detail-value {
            color: #333;
            font-size: 14px;
            font-weight: 600;
            text-align: right;
            max-width: 60%%;
            word-break: break-all;
        }
        .redirect-message {
            color: #999;
            font-size: 14px;
            margin-top: 24px;
        }
        .spinner {
            display: inline-block;
            width: 16px;
            height: 16px;
            border: 2px solid #ddd;
            border-top-color: #667eea;
            border-radius: 50%%;
            animation: spin 0.8s linear infinite;
            vertical-align: middle;
            margin-left: 8px;
        }
        @keyframes spin {
            to { transform: rotate(360deg); }
        }
        .button {
            display: inline-block;
            background: #667eea;
            color: white;
            padding: 12px 32px;
            border-radius: 8px;
            text-decoration: none;
            font-weight: 600;
            font-size: 16px;
            border: none;
            cursor: pointer;
            transition: background 0.3s;
        }
        .button:hover {
            background: #5568d3;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="icon">
            <svg viewBox="0 0 24 24">
                <polyline points="20 6 9 17 4 12"></polyline>
            </svg>
        </div>
        <h1>Payment Successful!</h1>
        <p class="message">%s</p>
        <div class="details">
            <div class="detail-row">
                <span class="detail-label">Transaction ID</span>
                <span class="detail-value">%s</span>
            </div>
            <div class="detail-row">
                <span class="detail-label">Amount</span>
                <span class="detail-value">₹%.2f</span>
            </div>
            <div class="detail-row">
                <span class="detail-label">Status</span>
                <span class="detail-value">Completed</span>
            </div>
        </div>
        <p class="redirect-message" id="closeMessage">
            This window will close automatically in <span id="countdown">3</span> seconds...
        </p>
        <button id="closeButton" class="button" style="display:none; margin-top: 16px;" onclick="window.close()">
            Close Window
        </button>
    </div>
    <script>
        var countdown = 3;
        var countdownElement = document.getElementById('countdown');
        var closeMessage = document.getElementById('closeMessage');
        var closeButton = document.getElementById('closeButton');

        var timer = setInterval(function() {
            countdown--;
            countdownElement.textContent = countdown;

            if (countdown <= 0) {
                clearInterval(timer);
                // Try to close the window
                window.close();

                // If window.close() fails (window not opened by script), show manual close button
                setTimeout(function() {
                    closeMessage.textContent = 'Please close this window manually';
                    closeButton.style.display = 'inline-block';
                }, 500);
            }
        }, 1000);
    </script>
</body>
</html>`,
		escapeHTML(message),
		escapeHTML(txnRef),
		float64(pmt.TotalAmount)/100.0,
	)
}

// generateFailureHTML creates failure page HTML
func generateFailureHTML(pmt *payment.Payment, txnRef, message string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Payment Failed</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            background: linear-gradient(135deg, #f093fb 0%%, #f5576c 100%%);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 20px;
        }
        .container {
            background: white;
            border-radius: 16px;
            box-shadow: 0 20px 60px rgba(0,0,0,0.3);
            max-width: 500px;
            width: 100%%;
            padding: 40px;
            text-align: center;
        }
        .icon {
            width: 80px;
            height: 80px;
            background: #f44336;
            border-radius: 50%%;
            display: flex;
            align-items: center;
            justify-content: center;
            margin: 0 auto 24px;
            animation: scaleIn 0.5s ease-out;
        }
        .icon svg {
            width: 48px;
            height: 48px;
            stroke: white;
            stroke-width: 3;
            stroke-linecap: round;
            stroke-linejoin: round;
            fill: none;
        }
        @keyframes scaleIn {
            from { transform: scale(0); }
            to { transform: scale(1); }
        }
        h1 {
            color: #333;
            font-size: 28px;
            margin-bottom: 12px;
            font-weight: 600;
        }
        .message {
            color: #666;
            font-size: 16px;
            margin-bottom: 32px;
            line-height: 1.5;
        }
        .details {
            background: #f5f5f5;
            border-radius: 12px;
            padding: 24px;
            margin-bottom: 32px;
            text-align: left;
        }
        .detail-row {
            display: flex;
            justify-content: space-between;
            padding: 12px 0;
            border-bottom: 1px solid #e0e0e0;
        }
        .detail-row:last-child {
            border-bottom: none;
        }
        .detail-label {
            color: #666;
            font-size: 14px;
            font-weight: 500;
        }
        .detail-value {
            color: #333;
            font-size: 14px;
            font-weight: 600;
            text-align: right;
            max-width: 60%%;
            word-break: break-all;
        }
        .button {
            display: inline-block;
            background: #667eea;
            color: white;
            padding: 14px 32px;
            border-radius: 8px;
            text-decoration: none;
            font-weight: 600;
            font-size: 16px;
            border: none;
            cursor: pointer;
            transition: background 0.3s;
        }
        .button:hover {
            background: #5568d3;
        }
        .redirect-message {
            color: #999;
            font-size: 14px;
            margin-top: 24px;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="icon">
            <svg viewBox="0 0 24 24">
                <line x1="18" y1="6" x2="6" y2="18"></line>
                <line x1="6" y1="6" x2="18" y2="18"></line>
            </svg>
        </div>
        <h1>Payment Failed</h1>
        <p class="message">%s</p>
        <div class="details">
            <div class="detail-row">
                <span class="detail-label">Transaction ID</span>
                <span class="detail-value">%s</span>
            </div>
            <div class="detail-row">
                <span class="detail-label">Amount</span>
                <span class="detail-value">₹%.2f</span>
            </div>
            <div class="detail-row">
                <span class="detail-label">Status</span>
                <span class="detail-value">Failed</span>
            </div>
        </div>
        <p class="redirect-message" id="closeMessage">
            This window will close automatically in <span id="countdown">3</span> seconds...
        </p>
        <button id="closeButton" class="button" style="display:none; margin-top: 16px;" onclick="window.close()">
            Close Window
        </button>
    </div>
    <script>
        var countdown = 3;
        var countdownElement = document.getElementById('countdown');
        var closeMessage = document.getElementById('closeMessage');
        var closeButton = document.getElementById('closeButton');

        var timer = setInterval(function() {
            countdown--;
            countdownElement.textContent = countdown;

            if (countdown <= 0) {
                clearInterval(timer);
                // Try to close the window
                window.close();

                // If window.close() fails (window not opened by script), show manual close button
                setTimeout(function() {
                    closeMessage.textContent = 'Please close this window manually';
                    closeButton.style.display = 'inline-block';
                }, 500);
            }
        }, 1000);
    </script>
</body>
</html>`,
		escapeHTML(message),
		escapeHTML(txnRef),
		float64(pmt.TotalAmount)/100.0,
	)
}

// generatePendingHTML creates pending page HTML
func generatePendingHTML(pmt *payment.Payment, txnRef, message string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Payment Pending</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            background: linear-gradient(135deg, #ffecd2 0%%, #fcb69f 100%%);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 20px;
        }
        .container {
            background: white;
            border-radius: 16px;
            box-shadow: 0 20px 60px rgba(0,0,0,0.3);
            max-width: 500px;
            width: 100%%;
            padding: 40px;
            text-align: center;
        }
        .icon {
            width: 80px;
            height: 80px;
            background: #FF9800;
            border-radius: 50%%;
            display: flex;
            align-items: center;
            justify-content: center;
            margin: 0 auto 24px;
            animation: pulse 2s ease-in-out infinite;
        }
        .icon svg {
            width: 48px;
            height: 48px;
            stroke: white;
            stroke-width: 3;
            stroke-linecap: round;
            stroke-linejoin: round;
            fill: none;
        }
        @keyframes pulse {
            0%%, 100%% { transform: scale(1); opacity: 1; }
            50%% { transform: scale(1.05); opacity: 0.8; }
        }
        h1 {
            color: #333;
            font-size: 28px;
            margin-bottom: 12px;
            font-weight: 600;
        }
        .message {
            color: #666;
            font-size: 16px;
            margin-bottom: 32px;
            line-height: 1.5;
        }
        .details {
            background: #f5f5f5;
            border-radius: 12px;
            padding: 24px;
            margin-bottom: 32px;
            text-align: left;
        }
        .detail-row {
            display: flex;
            justify-content: space-between;
            padding: 12px 0;
            border-bottom: 1px solid #e0e0e0;
        }
        .detail-row:last-child {
            border-bottom: none;
        }
        .detail-label {
            color: #666;
            font-size: 14px;
            font-weight: 500;
        }
        .detail-value {
            color: #333;
            font-size: 14px;
            font-weight: 600;
            text-align: right;
            max-width: 60%%;
            word-break: break-all;
        }
        .redirect-message {
            color: #999;
            font-size: 14px;
            margin-top: 24px;
        }
        .button {
            display: inline-block;
            background: #667eea;
            color: white;
            padding: 12px 32px;
            border-radius: 8px;
            text-decoration: none;
            font-weight: 600;
            font-size: 16px;
            border: none;
            cursor: pointer;
            transition: background 0.3s;
        }
        .button:hover {
            background: #5568d3;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="icon">
            <svg viewBox="0 0 24 24">
                <circle cx="12" cy="12" r="10"></circle>
                <polyline points="12 6 12 12 16 14"></polyline>
            </svg>
        </div>
        <h1>Payment Pending</h1>
        <p class="message">%s<br>We'll notify you once the payment is confirmed.</p>
        <div class="details">
            <div class="detail-row">
                <span class="detail-label">Transaction ID</span>
                <span class="detail-value">%s</span>
            </div>
            <div class="detail-row">
                <span class="detail-label">Amount</span>
                <span class="detail-value">₹%.2f</span>
            </div>
            <div class="detail-row">
                <span class="detail-label">Status</span>
                <span class="detail-value">Processing</span>
            </div>
        </div>
        <p class="redirect-message">
            You can check the status in your transaction history.
        </p>
        <button class="button" style="margin-top: 16px;" onclick="window.close()">
            Close Window
        </button>
    </div>
</body>
</html>`,
		escapeHTML(message),
		escapeHTML(txnRef),
		float64(pmt.TotalAmount)/100.0,
	)
}

// generateErrorHTML creates error page HTML
func generateErrorHTML(message string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Error</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            background: #f5f5f5;
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 20px;
        }
        .container {
            background: white;
            border-radius: 16px;
            box-shadow: 0 4px 12px rgba(0,0,0,0.1);
            max-width: 500px;
            width: 100%%;
            padding: 40px;
            text-align: center;
        }
        h1 {
            color: #333;
            font-size: 24px;
            margin-bottom: 16px;
            font-weight: 600;
        }
        p {
            color: #666;
            font-size: 16px;
            line-height: 1.5;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>Error</h1>
        <p>%s</p>
    </div>
</body>
</html>`,
		escapeHTML(message),
	)
}

// escapeHTML escapes HTML special characters
func escapeHTML(s string) string {
	s = fmt.Sprintf("%s", s) // Ensure string type
	s = replaceAll(s, "&", "&amp;")
	s = replaceAll(s, "<", "&lt;")
	s = replaceAll(s, ">", "&gt;")
	s = replaceAll(s, "\"", "&quot;")
	s = replaceAll(s, "'", "&#39;")
	return s
}

// replaceAll is a helper for string replacement
func replaceAll(s, old, new string) string {
	result := ""
	for i := 0; i < len(s); i++ {
		if i+len(old) <= len(s) && s[i:i+len(old)] == old {
			result += new
			i += len(old) - 1
		} else {
			result += string(s[i])
		}
	}
	return result
}
