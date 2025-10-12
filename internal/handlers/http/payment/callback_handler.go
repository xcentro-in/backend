package payment

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/adapters/external/payment/easebuzz"
	"xcentro.in/internal/adapters/external/payment/jiopay"
	"xcentro.in/internal/core/payment"
)

// CallbackHandler handles payment gateway callbacks (SURL/FURL)
// These are user-facing endpoints where gateways POST payment results
// after user completes payment. Response is HTML page for visual feedback.
type CallbackHandler struct {
	paymentRepo     payment.PaymentRepository
	jioPayGateway   payment.PaymentGateway
	easebuzzGateway payment.PaymentGateway
}

// NewCallbackHandler creates a new callback handler
func NewCallbackHandler(
	paymentRepo payment.PaymentRepository,
	jioPayGateway payment.PaymentGateway,
	easebuzzGateway payment.PaymentGateway,
) *CallbackHandler {
	return &CallbackHandler{
		paymentRepo:     paymentRepo,
		jioPayGateway:   jioPayGateway,
		easebuzzGateway: easebuzzGateway,
	}
}

// HandleCallback handles payment gateway callbacks (SURL/FURL)
// Route: POST /api/v1/process_payment/callback?type=jiopay
// Route: POST /api/v1/process_payment/callback?type=easebuzz
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
			"gateway":              "easebuzz",
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
        <p class="redirect-message">
            Redirecting to dashboard<span class="spinner"></span>
        </p>
    </div>
    <script>
        setTimeout(function() {
            window.location.href = '/dashboard';
        }, 3000);
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
        <a href="/payments/retry" class="button">Try Again</a>
    </div>
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
