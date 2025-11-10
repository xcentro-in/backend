package payment

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/payment"
	"xcentro.in/internal/shared/logger"
)

// Mock3DSHandler handles the mock 3DS/OTP page for Mock PG testing
// This simulates the bank's authentication page where users would normally enter OTP
type Mock3DSHandler struct {
	paymentRepo payment.PaymentRepository
	logger      *logger.Logger
	serverURL   string // Base server URL for constructing callback URLs
}

// NewMock3DSHandler creates a new mock 3DS handler
func NewMock3DSHandler(paymentRepo payment.PaymentRepository, serverURL string) *Mock3DSHandler {
	return &Mock3DSHandler{
		paymentRepo: paymentRepo,
		logger:      logger.GetDefault(),
		serverURL:   serverURL,
	}
}

// HandleMock3DSPage serves the mock 3DS authentication page
// Route: GET /api/v1/process_payment/mock_3ds?transaction_id=<id>&gateway_payment_id=<id>&amount=<amt>
//
// This page simulates a bank's OTP/3DS authentication page
// User can click "Success" or "Failure" buttons to simulate payment outcome
func (h *Mock3DSHandler) HandleMock3DSPage(c echo.Context) error {
	ctx := c.Request().Context()

	// Get parameters from query string
	transactionID := c.QueryParam("transaction_id")
	gatewayPaymentID := c.QueryParam("gateway_payment_id")
	amountStr := c.QueryParam("amount")

	if transactionID == "" || gatewayPaymentID == "" || amountStr == "" {
		h.logger.ErrorWith("Missing required parameters in mock 3DS request",
			"transaction_id", transactionID,
			"gateway_payment_id", gatewayPaymentID,
			"amount", amountStr)
		return c.HTML(http.StatusBadRequest, generateMock3DSErrorHTML("Invalid request parameters"))
	}

	// Fetch payment from database to get details
	pmt, err := h.paymentRepo.GetPaymentByID(ctx, transactionID)
	if err != nil {
		h.logger.ErrorWith("Failed to fetch payment in mock 3DS", "error", err, "transaction_id", transactionID)
		return c.HTML(http.StatusNotFound, generateMock3DSErrorHTML("Payment not found"))
	}

	if pmt == nil {
		h.logger.ErrorWith("Payment not found in mock 3DS", "transaction_id", transactionID)
		return c.HTML(http.StatusNotFound, generateMock3DSErrorHTML("Payment not found"))
	}

	// Calculate amount in rupees
	amountINR := float64(pmt.Amount) / 100.0

	// Construct callback URL
	callbackURL := fmt.Sprintf("%s/api/v1/process_payment/callback?type=mockpg", h.serverURL)

	h.logger.InfoWith("Serving mock 3DS page",
		"transaction_id", transactionID,
		"gateway_payment_id", gatewayPaymentID,
		"amount_inr", amountINR)

	// Generate and return the mock 3DS HTML page
	html := generateMock3DSPageHTML(transactionID, gatewayPaymentID, amountINR, callbackURL)
	return c.HTML(http.StatusOK, html)
}

// generateMock3DSPageHTML creates the HTML for the mock 3DS page
// This simulates a bank's OTP authentication page with success/failure buttons
func generateMock3DSPageHTML(transactionID, gatewayPaymentID string, amountINR float64, callbackURL string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Mock Bank - OTP Authentication</title>
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
        }
        .header {
            text-align: center;
            margin-bottom: 32px;
        }
        .bank-logo {
            width: 80px;
            height: 80px;
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            border-radius: 50%%;
            display: flex;
            align-items: center;
            justify-content: center;
            margin: 0 auto 16px;
            font-size: 32px;
            color: white;
            font-weight: bold;
        }
        h1 {
            color: #333;
            font-size: 24px;
            margin-bottom: 8px;
            font-weight: 600;
        }
        .subtitle {
            color: #666;
            font-size: 14px;
        }
        .payment-details {
            background: #f8f9fa;
            border-radius: 12px;
            padding: 24px;
            margin-bottom: 32px;
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
        .amount-highlight {
            font-size: 28px;
            color: #667eea;
            font-weight: bold;
            text-align: center;
            padding: 24px 0;
            border-bottom: 2px solid #e0e0e0;
            margin-bottom: 24px;
        }
        .otp-section {
            text-align: center;
            margin-bottom: 32px;
        }
        .otp-label {
            color: #666;
            font-size: 14px;
            margin-bottom: 12px;
        }
        .otp-input {
            width: 100%%;
            padding: 16px;
            font-size: 24px;
            text-align: center;
            border: 2px solid #e0e0e0;
            border-radius: 8px;
            letter-spacing: 8px;
            font-weight: bold;
            margin-bottom: 8px;
        }
        .otp-input:focus {
            outline: none;
            border-color: #667eea;
        }
        .otp-hint {
            color: #999;
            font-size: 12px;
        }
        .button-group {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 16px;
            margin-top: 24px;
        }
        .button {
            padding: 16px 24px;
            border-radius: 8px;
            border: none;
            font-weight: 600;
            font-size: 16px;
            cursor: pointer;
            transition: all 0.3s;
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 8px;
        }
        .button:disabled {
            opacity: 0.5;
            cursor: not-allowed;
        }
        .button-success {
            background: #4CAF50;
            color: white;
        }
        .button-success:hover:not(:disabled) {
            background: #45a049;
            transform: translateY(-2px);
            box-shadow: 0 4px 12px rgba(76, 175, 80, 0.3);
        }
        .button-failure {
            background: #f44336;
            color: white;
        }
        .button-failure:hover:not(:disabled) {
            background: #da190b;
            transform: translateY(-2px);
            box-shadow: 0 4px 12px rgba(244, 67, 54, 0.3);
        }
        .spinner {
            display: inline-block;
            width: 16px;
            height: 16px;
            border: 2px solid rgba(255,255,255,0.3);
            border-top-color: white;
            border-radius: 50%%;
            animation: spin 0.8s linear infinite;
        }
        @keyframes spin {
            to { transform: rotate(360deg); }
        }
        .mock-badge {
            background: #FFA726;
            color: white;
            padding: 8px 16px;
            border-radius: 20px;
            font-size: 12px;
            font-weight: bold;
            display: inline-block;
            margin-bottom: 16px;
        }
        .info-box {
            background: #e3f2fd;
            border-left: 4px solid #2196F3;
            padding: 16px;
            border-radius: 8px;
            margin-top: 24px;
        }
        .info-box p {
            color: #1565C0;
            font-size: 14px;
            line-height: 1.5;
            margin: 0;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <div class="bank-logo">MB</div>
            <div class="mock-badge">🧪 TEST MODE</div>
            <h1>Payment Authentication</h1>
            <p class="subtitle">Mock Bank - Test Environment</p>
        </div>

        <div class="amount-highlight">
            ₹%.2f
        </div>

        <div class="payment-details">
            <div class="detail-row">
                <span class="detail-label">Transaction ID</span>
                <span class="detail-value">%s</span>
            </div>
            <div class="detail-row">
                <span class="detail-label">Gateway Payment ID</span>
                <span class="detail-value">%s</span>
            </div>
            <div class="detail-row">
                <span class="detail-label">Merchant</span>
                <span class="detail-value">XCentro</span>
            </div>
        </div>

        <div class="otp-section">
            <p class="otp-label">Enter OTP sent to your registered mobile</p>
            <input
                type="text"
                id="otpInput"
                class="otp-input"
                placeholder="123456"
                maxlength="6"
                readonly
            />
            <p class="otp-hint">Mock OTP: Any 6 digits will work</p>
        </div>

        <div class="button-group">
            <button id="successBtn" class="button button-success" onclick="handlePayment('success')">
                <span id="successText">✓ Simulate Success</span>
                <span id="successSpinner" class="spinner" style="display:none;"></span>
            </button>
            <button id="failureBtn" class="button button-failure" onclick="handlePayment('failure')">
                <span id="failureText">✗ Simulate Failure</span>
                <span id="failureSpinner" class="spinner" style="display:none;"></span>
            </button>
        </div>

        <div class="info-box">
            <p>
                <strong>🧪 Test Mode:</strong> This is a simulated bank page for testing.
                Click "Simulate Success" to complete the payment successfully, or "Simulate Failure" to test error handling.
            </p>
        </div>
    </div>

    <script>
        const transactionID = '%s';
        const gatewayPaymentID = '%s';
        const callbackURL = '%s';
        const otpInput = document.getElementById('otpInput');
        const successBtn = document.getElementById('successBtn');
        const failureBtn = document.getElementById('failureBtn');

        // Auto-fill mock OTP for convenience
        otpInput.value = '123456';

        // Handle payment simulation (success or failure)
        async function handlePayment(outcome) {
            console.log('Mock 3DS: Simulating payment outcome:', outcome);

            // Disable buttons and show spinner
            const isSuccess = outcome === 'success';
            const btn = isSuccess ? successBtn : failureBtn;
            const text = document.getElementById(isSuccess ? 'successText' : 'failureText');
            const spinner = document.getElementById(isSuccess ? 'successSpinner' : 'failureSpinner');

            successBtn.disabled = true;
            failureBtn.disabled = true;
            text.style.display = 'none';
            spinner.style.display = 'inline-block';

            // Prepare webhook payload (matches MockPGWebhookPayload structure)
            const webhookPayload = {
                merchant_id: 'MOCK_MERCHANT_123',
                transaction_id: transactionID,
                gateway_payment_id: gatewayPaymentID,
                status: outcome,
                amount: %d, // amount in paise
                currency: 'INR',
                payment_method: 'card',
                processed_at: new Date().toISOString(),
                signature: 'MOCK_SIG_' + Math.random().toString(36).substring(7)
            };

            console.log('Sending webhook to callback URL:', callbackURL);
            console.log('Webhook payload:', webhookPayload);

            try {
                // Send webhook to callback endpoint
                const response = await fetch(callbackURL, {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                    },
                    body: JSON.stringify(webhookPayload)
                });

                console.log('Callback response status:', response.status);
                console.log('Callback response content-type:', response.headers.get('content-type'));

                // Check if response is HTML (callback always returns HTML pages)
                const contentType = response.headers.get('content-type');
                if (contentType && contentType.includes('text/html')) {
                    const html = await response.text();
                    console.log('Rendering HTML response page (status:', response.status, ')');

                    // Replace current page with callback response (success/failure/error page)
                    document.open();
                    document.write(html);
                    document.close();
                } else {
                    console.error('Unexpected response type:', contentType);
                    showError('Payment processing failed. Please try again.');
                }
            } catch (error) {
                console.error('Error sending callback:', error);
                showError('Network error. Please check your connection and try again.');
            }
        }

        function showError(message) {
            // Re-enable buttons
            successBtn.disabled = false;
            failureBtn.disabled = false;
            document.getElementById('successText').style.display = 'inline';
            document.getElementById('failureText').style.display = 'inline';
            document.getElementById('successSpinner').style.display = 'none';
            document.getElementById('failureSpinner').style.display = 'none';

            alert(message);
        }

        // Simulate OTP input validation
        otpInput.addEventListener('input', function() {
            this.value = this.value.replace(/[^0-9]/g, '').substring(0, 6);
        });
    </script>
</body>
</html>`,
		amountINR,
		escapeHTML(transactionID),
		escapeHTML(gatewayPaymentID),
		escapeHTML(transactionID),
		escapeHTML(gatewayPaymentID),
		escapeHTML(callbackURL),
		int64(amountINR*100), // Convert back to paise for webhook
	)
}

// generateMock3DSErrorHTML creates error page HTML for mock 3DS errors
func generateMock3DSErrorHTML(message string) string {
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
        .icon {
            width: 80px;
            height: 80px;
            background: #f44336;
            border-radius: 50%%;
            display: flex;
            align-items: center;
            justify-content: center;
            margin: 0 auto 24px;
            font-size: 40px;
            color: white;
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
        .button {
            margin-top: 24px;
            padding: 12px 32px;
            background: #667eea;
            color: white;
            border: none;
            border-radius: 8px;
            font-weight: 600;
            font-size: 16px;
            cursor: pointer;
        }
        .button:hover {
            background: #5568d3;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="icon">✗</div>
        <h1>Error</h1>
        <p>%s</p>
        <button class="button" onclick="window.close()">Close Window</button>
    </div>
</body>
</html>`,
		escapeHTML(message),
	)
}
