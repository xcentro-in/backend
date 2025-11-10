package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/adapters/external/payment/easebuzz"
	"xcentro.in/internal/adapters/external/payment/jiopay"
	"xcentro.in/internal/adapters/external/payment/mockpg"
	"xcentro.in/internal/core/payment"
	"xcentro.in/internal/shared/logger"
)

// CallbackHandler handles payment gateway callbacks (SURL/FURL)
// These are user-facing endpoints where gateways POST payment results
// after user completes payment. Response is HTML page for visual feedback.
type CallbackHandler struct {
	paymentRepo        payment.PaymentRepository
	jioPayGateway      payment.PaymentGateway
	easebuzzGateway    payment.PaymentGateway
	mockPGGateway      payment.PaymentGateway
	mockPGEnabled      bool        // Flag to check if mock PG is enabled
	pendingVoucherRepo interface{} // PaymentVoucherPendingRepository for fetching pending vouchers
	voucherRepo        interface{} // VoucherRepository for finalizing vouchers
	txManager          interface{} // TxManager for transaction support
	stateChangeRepo    payment.PaymentStateChangeRepository
	logger             *logger.Logger
}

// NewCallbackHandler creates a new callback handler
func NewCallbackHandler(
	paymentRepo payment.PaymentRepository,
	jioPayGateway payment.PaymentGateway,
	easebuzzGateway payment.PaymentGateway,
	mockPGGateway payment.PaymentGateway,
	mockPGEnabled bool,
	pendingVoucherRepo interface{},
	voucherRepo interface{},
	txManager interface{},
	stateChangeRepo payment.PaymentStateChangeRepository,
) *CallbackHandler {
	return &CallbackHandler{
		paymentRepo:        paymentRepo,
		jioPayGateway:      jioPayGateway,
		easebuzzGateway:    easebuzzGateway,
		mockPGGateway:      mockPGGateway,
		mockPGEnabled:      mockPGEnabled,
		pendingVoucherRepo: pendingVoucherRepo,
		voucherRepo:        voucherRepo,
		txManager:          txManager,
		stateChangeRepo:    stateChangeRepo,
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

	fmt.Printf("=== CALLBACK RECEIVED: %s ===\n", gatewayType)

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
		fmt.Printf("❌ Invalid gateway type: %s\n", gatewayType)
		return c.HTML(http.StatusBadRequest, generateErrorHTML("Invalid gateway type"))
	}
}

// handleJioPayCallback processes JioPay callback (SURL/FURL)
func (h *CallbackHandler) handleJioPayCallback(c echo.Context) error {
	ctx := c.Request().Context()

	fmt.Println("=== JIOPAY CALLBACK: Processing ===")

	// Step 1: Read request body
	bodyBytes, err := io.ReadAll(c.Request().Body)
	if err != nil {
		fmt.Printf("❌ Failed to read body: %v\n", err)
		return c.HTML(http.StatusBadRequest, generateErrorHTML("Invalid request"))
	}

	fmt.Printf("📥 Callback payload size: %d bytes\n", len(bodyBytes))

	// Step 2: Parse callback payload
	var callback jiopay.CallbackPayload
	if err := json.Unmarshal(bodyBytes, &callback); err != nil {
		fmt.Printf("❌ Failed to parse payload: %v\n", err)
		return c.HTML(http.StatusBadRequest, generateErrorHTML("Invalid payload format"))
	}

	fmt.Printf("📋 MerchantTxnNo: %s\n", callback.MerchantTxnNo)
	fmt.Printf("📋 TxnStatus: %s\n", callback.TxnStatus)
	fmt.Printf("📋 ResponseCode: %s\n", callback.ResponseCode)
	fmt.Printf("📋 JioPay TxnID: %s\n", callback.TxnID)

	// Step 3: Verify signature (security validation)
	if err := h.jioPayGateway.ValidateWebhook(bodyBytes, callback.SecureHash); err != nil {
		fmt.Printf("❌ Signature verification failed: %v\n", err)
		return c.HTML(http.StatusUnauthorized, generateErrorHTML("Security verification failed"))
	}

	fmt.Println("✅ Signature verified")

	// Step 4: Query payment by merchantTxnNo (our transaction ID)
	pmt, err := h.paymentRepo.GetPaymentByGatewayPaymentID(ctx, callback.MerchantTxnNo)
	if err != nil || pmt == nil {
		fmt.Printf("❌ Payment not found: %s, error: %v\n", callback.MerchantTxnNo, err)
		return c.HTML(http.StatusNotFound, generateErrorHTML("Payment not found"))
	}

	fmt.Printf("✅ Payment found: %s (current status: %s)\n", pmt.ID, pmt.Status)

	// Step 5: Map callback status to internal status
	newStatus := mapCallbackStatusToPaymentStatus(
		jiopay.MapJioPayCallbackStatus(callback.ResponseCode, callback.TxnStatus),
	)

	fmt.Printf("📊 Status mapping: %s + %s → %s\n",
		callback.ResponseCode, callback.TxnStatus, newStatus)

	// Step 6: Update payment (only if not already in terminal state)
	if !isTerminalStatus(pmt.Status) {
		fmt.Println("🔄 Updating payment status...")

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
			fmt.Printf("❌ Failed to update payment: %v\n", err)
			// Don't fail callback - still show page to user
		} else {
			fmt.Printf("✅ Payment status updated to: %s\n", newStatus)
		}

		// Update gateway payment ID if not set
		if pmt.GatewayPaymentID == "" || pmt.GatewayPaymentID == callback.MerchantTxnNo {
			if err := h.paymentRepo.UpdateGatewayPaymentID(ctx, pmt.ID, callback.TxnID, ""); err != nil {
				fmt.Printf("⚠️  Failed to update gateway payment ID: %v\n", err)
			} else {
				fmt.Printf("✅ Gateway payment ID updated to: %s\n", callback.TxnID)
			}
		}
	} else {
		fmt.Printf("⚠️  Payment already in terminal state: %s (skipping update)\n", pmt.Status)
	}

	// Step 7: Return HTML response to user
	fmt.Println("=== JIOPAY CALLBACK: Complete ===")

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

	fmt.Println("=== EASEBUZZ CALLBACK: Processing ===")

	// Step 1: Read request body
	bodyBytes, err := io.ReadAll(c.Request().Body)
	if err != nil {
		fmt.Printf("❌ Failed to read body: %v\n", err)
		return c.HTML(http.StatusBadRequest, generateErrorHTML("Invalid request"))
	}

	fmt.Printf("📥 Callback payload size: %d bytes\n", len(bodyBytes))

	// Step 2: Parse callback payload
	var callback easebuzz.CallbackPayload
	if err := json.Unmarshal(bodyBytes, &callback); err != nil {
		fmt.Printf("❌ Failed to parse payload: %v\n", err)
		return c.HTML(http.StatusBadRequest, generateErrorHTML("Invalid payload format"))
	}

	fmt.Printf("📋 TxnID: %s\n", callback.TxnID)
	fmt.Printf("📋 Status: %s\n", callback.Status)
	fmt.Printf("📋 Amount: %s\n", callback.Amount)
	fmt.Printf("📋 Easepay ID: %s\n", callback.EasepayID)

	// Step 3: Verify signature (security validation)
	if err := h.easebuzzGateway.ValidateWebhook(bodyBytes, callback.Hash); err != nil {
		fmt.Printf("❌ Signature verification failed: %v\n", err)
		return c.HTML(http.StatusUnauthorized, generateErrorHTML("Security verification failed"))
	}

	fmt.Println("✅ Signature verified")

	// Step 4: Query payment by txnid (our transaction ID)
	pmt, err := h.paymentRepo.GetPaymentByGatewayPaymentID(ctx, callback.TxnID)
	if err != nil || pmt == nil {
		fmt.Printf("❌ Payment not found: %s, error: %v\n", callback.TxnID, err)
		return c.HTML(http.StatusNotFound, generateErrorHTML("Payment not found"))
	}

	fmt.Printf("✅ Payment found: %s (current status: %s)\n", pmt.ID, pmt.Status)

	// Step 5: Map callback status to internal status
	newStatus := mapCallbackStatusToPaymentStatus(
		easebuzz.MapEasebuzzCallbackStatus(callback.Status),
	)

	fmt.Printf("📊 Status mapping: %s → %s\n", callback.Status, newStatus)

	// Step 6: Update payment (only if not already in terminal state)
	if !isTerminalStatus(pmt.Status) {
		fmt.Println("🔄 Updating payment status...")

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
			fmt.Printf("❌ Failed to update payment: %v\n", err)
			// Don't fail callback - still show page to user
		} else {
			fmt.Printf("✅ Payment status updated to: %s\n", newStatus)
		}

		// Update gateway payment ID if not set or if it's still our txnid
		if pmt.GatewayPaymentID == "" || pmt.GatewayPaymentID == callback.TxnID {
			if err := h.paymentRepo.UpdateGatewayPaymentID(ctx, pmt.ID, callback.EasepayID, ""); err != nil {
				fmt.Printf("⚠️  Failed to update gateway payment ID: %v\n", err)
			} else {
				fmt.Printf("✅ Gateway payment ID updated to: %s\n", callback.EasepayID)
			}
		}
	} else {
		fmt.Printf("⚠️  Payment already in terminal state: %s (skipping update)\n", pmt.Status)
	}

	// Step 7: Return HTML response to user
	fmt.Println("=== EASEBUZZ CALLBACK: Complete ===")

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
	if callback.Status == "success" {
		newStatus = payment.PaymentStatusSuccess
	} else {
		newStatus = payment.PaymentStatusFailed
	}

	fmt.Printf("📊 Status mapping: %s → %s\n", callback.Status, newStatus)

	// Step 6: Update payment (only if not already in terminal state)
	if !isTerminalStatus(pmt.Status) {
		fmt.Println("🔄 Updating payment status...")

		metadata := map[string]any{
			"callback_received_at":      time.Now().Format(time.RFC3339),
			"callback_type":             "payment_status",
			"gateway":                   "mockpg",
			"mock_gateway_payment_id":   callback.GatewayPaymentID,
			"payment_method":            callback.PaymentMethod,
			"processed_at":              callback.ProcessedAt,
			"merchant_id":               callback.MerchantID,
			"mock_simulation":           true,
		}

		if err := h.paymentRepo.UpdatePaymentStatus(ctx, pmt.ID, newStatus, metadata); err != nil {
			fmt.Printf("❌ Failed to update payment: %v\n", err)
			// Don't fail callback - still show page to user
		} else {
			fmt.Printf("✅ Payment status updated to: %s\n", newStatus)

			// Step 6.1: Record payment state change in audit trail
			previousState := string(pmt.Status)
			newStateStr := string(newStatus)
			var action payment.PaymentAction
			if newStatus == payment.PaymentStatusSuccess {
				action = payment.PaymentActionSuccess
			} else if newStatus == payment.PaymentStatusFailed {
				action = payment.PaymentActionFailed
			} else {
				action = payment.PaymentActionProcessing
			}

			if err := h.recordPaymentStateChange(ctx, pmt.ID, action, previousState, newStateStr, "mockpg", metadata); err != nil {
				fmt.Printf("⚠️  Failed to record state change: %v\n", err)
				// Don't fail the callback - payment update is still successful
			} else {
				fmt.Println("✅ Payment state change recorded")
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

		// Step 6.5: Finalize vouchers if payment succeeded
		if newStatus == payment.PaymentStatusSuccess {
			fmt.Println("💰 Payment succeeded, finalizing vouchers...")
			if err := h.finalizeVouchersOnSuccess(ctx, pmt.ID, "mockpg"); err != nil {
				fmt.Printf("⚠️  Failed to finalize vouchers: %v\n", err)
				// Don't fail the callback - payment is still successful
				// Vouchers can be finalized manually or by a recovery job
			} else {
				fmt.Println("✅ Vouchers finalized successfully")
			}
		}
	} else {
		fmt.Printf("⚠️  Payment already in terminal state: %s (skipping update)\n", pmt.Status)
	}

	// Step 7: Return HTML response to user
	fmt.Println("=== MOCK PG CALLBACK: Complete ===")

	switch newStatus {
	case payment.PaymentStatusSuccess:
		return c.HTML(http.StatusOK, generateSuccessHTML(pmt, callback.TransactionID, "Payment completed successfully"))
	case payment.PaymentStatusPending:
		return c.HTML(http.StatusOK, generatePendingHTML(pmt, callback.TransactionID, "Payment is being processed"))
	default:
		return c.HTML(http.StatusOK, generateFailureHTML(pmt, callback.TransactionID, "Payment failed"))
	}
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
	case "success":
		return payment.PaymentStatusSuccess
	case "pending":
		return payment.PaymentStatusPending
	case "failed":
		return payment.PaymentStatusFailed
	default:
		return payment.PaymentStatusFailed
	}
}

// finalizeVouchersOnSuccess finalizes vouchers when payment succeeds
// This is called from callback handlers after payment status is updated to SUCCESS
// It fetches pending vouchers, finalizes them, and cleans up pending records - all in a transaction
func (h *CallbackHandler) finalizeVouchersOnSuccess(ctx context.Context, paymentID string, gatewayName string) error {
	if h.pendingVoucherRepo == nil || h.voucherRepo == nil || h.txManager == nil {
		h.logger.WarnWith("Voucher finalization dependencies not available, skipping", "payment_id", paymentID)
		return nil // Not an error, just skip if dependencies aren't wired
	}

	h.logger.InfoWith("Checking for pending vouchers to finalize", "payment_id", paymentID)

	// Step 1: Fetch pending vouchers for this payment
	type pendingVoucherFetcher interface {
		GetPendingVouchers(ctx context.Context, paymentID uuid.UUID) ([]uuid.UUID, error)
	}

	fetcher, ok := h.pendingVoucherRepo.(pendingVoucherFetcher)
	if !ok {
		h.logger.ErrorWith("Pending voucher repository does not implement GetPendingVouchers")
		return fmt.Errorf("pending voucher repository missing GetPendingVouchers method")
	}

	paymentUUID, err := uuid.Parse(paymentID)
	if err != nil {
		h.logger.ErrorWith("Invalid payment ID", "payment_id", paymentID, "error", err)
		return fmt.Errorf("invalid payment ID: %w", err)
	}

	voucherIDs, err := fetcher.GetPendingVouchers(ctx, paymentUUID)
	if err != nil {
		h.logger.ErrorWith("Failed to fetch pending vouchers", "error", err)
		return fmt.Errorf("failed to fetch pending vouchers: %w", err)
	}

	if len(voucherIDs) == 0 {
		h.logger.InfoWith("No pending vouchers found for payment", "payment_id", paymentID)
		return nil // No vouchers to finalize
	}

	h.logger.InfoWith("Found pending vouchers, starting finalization", "payment_id", paymentID, "voucher_count", len(voucherIDs))

	// Step 2: Finalize vouchers in a transaction
	type txManagerInterface interface {
		WithTx(ctx context.Context, fn func(ctx context.Context, tx interface{}) error) error
	}

	txMgr, ok := h.txManager.(txManagerInterface)
	if !ok {
		h.logger.ErrorWith("TxManager does not implement WithTx method")
		return fmt.Errorf("txManager missing WithTx method")
	}

	err = txMgr.WithTx(ctx, func(txCtx context.Context, tx interface{}) error {
		// Create transactional voucher repository
		var txVoucherRepo interface{}
		repoValue := reflect.ValueOf(h.voucherRepo)
		withTxMethod := repoValue.MethodByName("WithTx")

		if withTxMethod.IsValid() {
			txValue := reflect.ValueOf(tx)
			result := withTxMethod.Call([]reflect.Value{txValue})
			if len(result) > 0 {
				txVoucherRepo = result[0].Interface()
				h.logger.DebugWith("Created transactional voucher repository")
			} else {
				return fmt.Errorf("WithTx returned no value")
			}
		} else {
			h.logger.WarnWith("Voucher repository does not have WithTx, using non-transactional")
			txVoucherRepo = h.voucherRepo
		}

		// Finalize each voucher
		type voucherFinalizer interface {
			FinalizeVoucher(ctx context.Context, voucherID uuid.UUID, paymentID uuid.UUID) error
		}

		finalizer, ok := txVoucherRepo.(voucherFinalizer)
		if !ok {
			h.logger.ErrorWith("Voucher repository does not implement FinalizeVoucher")
			return fmt.Errorf("voucher repository missing FinalizeVoucher method")
		}

		for _, voucherID := range voucherIDs {
			h.logger.DebugWith("Finalizing voucher", "voucher_id", voucherID.String(), "payment_id", paymentID)
			if err := finalizer.FinalizeVoucher(txCtx, voucherID, paymentUUID); err != nil {
				h.logger.ErrorWith("Failed to finalize voucher", "voucher_id", voucherID.String(), "error", err)
				return fmt.Errorf("failed to finalize voucher %s: %w", voucherID.String(), err)
			}
		}

		h.logger.InfoWith("All vouchers finalized successfully", "count", len(voucherIDs))

		// Create transactional pending voucher repository and delete records
		var txPendingRepo interface{}
		pendingRepoValue := reflect.ValueOf(h.pendingVoucherRepo)
		pendingWithTxMethod := pendingRepoValue.MethodByName("WithTx")

		if pendingWithTxMethod.IsValid() {
			txValue := reflect.ValueOf(tx)
			result := pendingWithTxMethod.Call([]reflect.Value{txValue})
			if len(result) > 0 {
				txPendingRepo = result[0].Interface()
				h.logger.DebugWith("Created transactional pending voucher repository")
			} else {
				return fmt.Errorf("pending repo WithTx returned no value")
			}
		} else {
			h.logger.WarnWith("Pending voucher repository does not have WithTx, using non-transactional")
			txPendingRepo = h.pendingVoucherRepo
		}

		// Delete pending records
		type pendingVoucherDeleter interface {
			DeletePendingVouchers(ctx context.Context, paymentID uuid.UUID) error
		}

		deleter, ok := txPendingRepo.(pendingVoucherDeleter)
		if !ok {
			h.logger.ErrorWith("Pending voucher repository does not implement DeletePendingVouchers")
			return fmt.Errorf("pending voucher repository missing DeletePendingVouchers method")
		}

		h.logger.DebugWith("Deleting pending voucher records", "payment_id", paymentID)
		if err := deleter.DeletePendingVouchers(txCtx, paymentUUID); err != nil {
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

// recordPaymentStateChange records a state change in the audit trail
func (h *CallbackHandler) recordPaymentStateChange(
	ctx context.Context,
	paymentID string,
	action payment.PaymentAction,
	previousState string,
	newState string,
	gatewayName string,
	metadata map[string]any,
) error {
	// Parse payment ID
	paymentUUID, err := uuid.Parse(paymentID)
	if err != nil {
		h.logger.ErrorWith("Failed to parse payment ID", "payment_id", paymentID, "error", err)
		return fmt.Errorf("invalid payment ID: %w", err)
	}

	// Create state change request
	req := &payment.CreatePaymentStateChangeRequest{
		PaymentID:       paymentUUID,
		Action:          action,
		ActionTimestamp: time.Now(),
		ActorType:       payment.ActorTypeWebhook,
		ActorName:       fmt.Sprintf("%s_callback", gatewayName),
		PreviousState:   previousState,
		NewState:        newState,
		Provider:        gatewayName,
		Metadata:        metadata,
		Remarks:         fmt.Sprintf("Payment status updated via %s callback", gatewayName),
	}

	// Record state change
	stateChange, err := h.stateChangeRepo.Create(ctx, req)
	if err != nil {
		h.logger.ErrorWith("Failed to record state change", "payment_id", paymentID, "error", err)
		return fmt.Errorf("failed to record state change: %w", err)
	}

	h.logger.InfoWith("Payment state change recorded",
		"payment_id", paymentID,
		"state_change_id", stateChange.StateChangeID,
		"action", action,
		"previous_state", previousState,
		"new_state", newState,
	)

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
		float64(pmt.Amount)/100.0,
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
		float64(pmt.Amount)/100.0,
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
		float64(pmt.Amount)/100.0,
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
