package mockpg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"xcentro.in/internal/core/payment"
)

// MockPGClient implements the PaymentGateway interface for testing purposes
// This gateway simulates real payment gateway behavior without making external API calls
type MockPGClient struct {
	merchantID string
	apiKey     string
	// Configuration for simulating different scenarios
	simulateSuccess bool          // If true, always return success; if false, simulate failures
	delayMs         int           // Artificial delay in milliseconds to simulate network latency
	webhookDelay    time.Duration // Delay before sending webhook (default: 2 minutes)
}

// NewMockPGClient creates a new mock payment gateway client
//
// Parameters:
//   - merchantID: Mock merchant identifier (from env)
//   - apiKey: Mock API key (from env)
//   - simulateSuccess: If true, all operations succeed; if false, simulate realistic failures
//
// Usage:
//
//	client := NewMockPGClient(
//	    cfg.MockPG.MerchantID,
//	    cfg.MockPG.APIKey,
//	    true, // Always succeed for testing
//	)
func NewMockPGClient(merchantID, apiKey string, simulateSuccess bool) payment.PaymentGateway {
	return &MockPGClient{
		merchantID:      merchantID,
		apiKey:          apiKey,
		simulateSuccess: simulateSuccess,
		delayMs:         100,           // Simulate 100ms network delay
		webhookDelay:    2 * time.Minute, // Send webhook after 2 minutes
	}
}

// GetGatewayType returns the gateway type identifier
func (c *MockPGClient) GetGatewayType() payment.GatewayType {
	return payment.GatewayMockPG
}

// TokenizeCard implements mock card tokenization flow
// Simulates the tokenization process without making external API calls
func (c *MockPGClient) TokenizeCard(ctx context.Context, request payment.GatewayTokenizeRequest) (*payment.GatewayTokenizeResponse, error) {
	fmt.Println("=== MOCK PG GATEWAY: Tokenization ===")
	fmt.Printf("📥 Business ID: %s\n", request.BusinessID)
	fmt.Printf("📥 Transaction ID: %s\n", request.TransactionID)
	fmt.Printf("💳 Card: %s %s ending in %s\n",
		request.CardDetails.CardCategory, request.CardDetails.CardType, request.CardDetails.LastFourDigits)

	// Simulate network delay
	c.simulateDelay(ctx)

	// Check if we should simulate failure
	if !c.simulateSuccess {
		// Simulate 10% failure rate for tokenization
		if time.Now().UnixNano()%10 == 0 {
			fmt.Println("❌ Mock PG: Simulating tokenization failure")
			return nil, fmt.Errorf("mock tokenization failed: issuer bank unavailable")
		}
	}

	// Generate mock token and card ID
	mockToken := fmt.Sprintf("MOCK_TOKEN_%s", uuid.Must(uuid.NewV7()).String())
	mockCardID := fmt.Sprintf("MOCK_CARD_%s", uuid.Must(uuid.NewV7()).String()[:16])
	mockVaultID := fmt.Sprintf("VAULT_%s", uuid.Must(uuid.NewV7()).String()[:12])

	// Build masked card number
	maskedNumber := fmt.Sprintf("XXXX-XXXX-XXXX-%s", request.CardDetails.LastFourDigits)

	fmt.Printf("✅ Token created: CardID %s\n", mockCardID)
	fmt.Printf("🔐 Token: %s...\n", mockToken[:20])

	// Create response mimicking real gateway behavior
	response := &payment.GatewayTokenizeResponse{
		Token:         mockToken,
		CardID:        mockCardID,
		TransactionID: request.TransactionID,
		Status:        "success",
		Message:       "Card tokenized successfully via Mock PG",
		Metadata: map[string]any{
			"gateway":           "mockpg",
			"tokenization_flow": "mock_api",
			"vault_id":          mockVaultID,
			"masked_number":     maskedNumber,
			"bank_name":         getMockBankName(request.CardDetails.CardType),
			"card_scheme":       string(request.CardDetails.CardType),
			"card_type":         string(request.CardDetails.CardCategory),
			"is_domestic":       true,
			"cvv_less":          false,
			"mock_simulation":   true,
		},
	}

	fmt.Println("=== MOCK PG GATEWAY: Tokenization Complete ===")
	return response, nil
}

// GenerateCryptogram generates a mock cryptogram for secure payments
func (c *MockPGClient) GenerateCryptogram(ctx context.Context, request payment.CryptogramRequest) (*payment.CryptogramResponse, error) {
	fmt.Println("=== MOCK PG GATEWAY: GenerateCryptogram ===")
	fmt.Printf("🔐 Token: %s...\n", request.Token[:20])
	fmt.Printf("💰 Amount: %d %s\n", request.Amount, request.Currency)

	// Simulate network delay
	c.simulateDelay(ctx)

	// Check if we should simulate failure
	if !c.simulateSuccess {
		// Simulate 5% failure rate for cryptogram generation
		if time.Now().UnixNano()%20 == 0 {
			fmt.Println("❌ Mock PG: Simulating cryptogram generation failure")
			return nil, fmt.Errorf("mock cryptogram generation failed: token expired")
		}
	}

	// Generate mock cryptogram (random UUID-based string)
	mockCryptogram := fmt.Sprintf("MOCK_CRYPTO_%s_%d",
		uuid.Must(uuid.NewV7()).String()[:24],
		time.Now().UnixNano())

	// Mock token (network token as numeric)
	mockNetworkToken := time.Now().UnixNano()

	// Cryptogram expiry (10 minutes from now)
	expiresAt := time.Now().Add(10 * time.Minute)

	fmt.Printf("✅ Cryptogram generated: %s...\n", mockCryptogram[:30])
	fmt.Printf("⏰ Expires at: %s\n", expiresAt.Format("15:04:05"))

	// Create response
	response := &payment.CryptogramResponse{
		Cryptogram:   mockCryptogram,
		ExpiresAt:    expiresAt,
		ValidityMins: 10,
		Algorithm:    "MockPG-Network-Token",
		Metadata: map[string]any{
			"gateway":         "mockpg",
			"cryptogram_flow": "mock_api",
			"network_token":   fmt.Sprintf("%d", mockNetworkToken),
			"token_expiry":    expiresAt.Format("012006"), // MMYYYY format
			"mock_simulation": true,
		},
	}

	fmt.Println("=== MOCK PG GATEWAY: GenerateCryptogram Complete ===")
	return response, nil
}

// InitiatePayment implements mock payment initiation
// Supports all payment methods: card (tokenized & direct), net banking, UPI, wallet
// Also handles tokenization payments (payment_type="tokenization")
func (c *MockPGClient) InitiatePayment(ctx context.Context, request payment.GatewayPaymentRequest) (*payment.GatewayPaymentResponse, error) {
	fmt.Println("=== MOCK PG GATEWAY: InitiatePayment Called ===")
	fmt.Printf("📥 Business ID: %s\n", request.BusinessID)
	fmt.Printf("📥 Transaction ID: %s\n", request.TransactionID)
	fmt.Printf("💰 Amount: %d %s\n", request.Amount, request.Currency)
	fmt.Printf("🔀 Payment Method: %s\n", request.PaymentMethod)
	fmt.Printf("🔀 Payment Type: %s\n", request.PaymentType)

	// Check if this is a tokenization payment
	isTokenization := request.PaymentType == payment.PaymentTypeTokenization
	if isTokenization {
		fmt.Println("🔑 TOKENIZATION PAYMENT: Will store card details for post-payment tokenization")
	}

	// Simulate network delay
	c.simulateDelay(ctx)

	// Check if we should simulate failure
	if !c.simulateSuccess {
		// Simulate 15% failure rate for payment initiation
		if time.Now().UnixNano()%7 == 0 {
			fmt.Println("❌ Mock PG: Simulating payment initiation failure")
			return nil, fmt.Errorf("mock payment failed: insufficient funds")
		}
	}

	// Generate mock gateway payment ID
	mockGatewayPaymentID := fmt.Sprintf("MOCKPG_%s", uuid.Must(uuid.NewV7()).String())

	// Construct mock 3DS page URL (simulates bank's OTP/authentication page)
	// Extract base URL from callback URL and construct mock 3DS URL
	// Callback URL format: http://localhost:8888/v1/process_payment/callback_page?transaction_id=xxx&source=web
	// Mock 3DS URL format: http://localhost:8888/v1/process_payment/mock_3ds?transaction_id=xxx&gateway_payment_id=xxx&amount=xxx
	baseURL := extractBaseURL(request.CallbackURL)
	redirectURL := fmt.Sprintf("%s/v1/process_payment/mock_3ds?transaction_id=%s&gateway_payment_id=%s&amount=%d",
		baseURL,
		request.TransactionID,
		mockGatewayPaymentID,
		request.Amount,
	)

	// Determine payment status based on payment method
	status := payment.PaymentStatusPending // Most payments require user action

	// Log payment method specific details
	switch request.PaymentMethod {
	case payment.PaymentMethodCard:
		if request.Token != "" && request.Cryptogram != "" {
			fmt.Println("💳 Processing tokenized card payment...")
			fmt.Printf("🎫 Token: %s...\n", request.Token[:20])
			fmt.Printf("🔐 Cryptogram: %s...\n", request.Cryptogram[:30])
		} else if request.CardDetails != nil {
			fmt.Println("💳 Processing direct card payment...")
			fmt.Printf("💳 Card ending in: %s\n", request.CardDetails.LastFourDigits)
		}
	case payment.PaymentMethodNetBanking:
		fmt.Println("🏦 Processing net banking payment...")
		if request.NetBankingDetails != nil {
			fmt.Printf("🏦 Bank: %s\n", request.NetBankingDetails.BankCode)
		}
	case payment.PaymentMethodUPI:
		fmt.Println("📱 Processing UPI payment...")
		if request.UPIDetails != nil {
			fmt.Printf("📱 VPA: %s\n", request.UPIDetails.VPA)
		}
	case payment.PaymentMethodWallet:
		fmt.Println("👛 Processing wallet payment...")
		if request.WalletDetails != nil {
			fmt.Printf("👛 Wallet ID: %s\n", request.WalletDetails.WalletID)
		}
	}

	fmt.Printf("✅ Payment request accepted (PENDING - awaiting user authentication)\n")
	fmt.Printf("🌐 Redirect URL (Mock 3DS): %s\n", redirectURL)
	fmt.Printf("🔑 Gateway Payment ID: %s\n", mockGatewayPaymentID)

	// Note: Webhook is NOT scheduled automatically
	// The mock 3DS page will trigger the callback when user clicks Success/Failure buttons
	fmt.Printf("📌 User will be redirected to mock 3DS page for authentication\n")
	fmt.Printf("📌 Callback will be triggered when user completes authentication\n")

	// Create response - returns PENDING status (payment not complete yet)
	// Real payment gateways return PENDING here and send final status via webhook
	response := &payment.GatewayPaymentResponse{
		PaymentID:        request.TransactionID,
		GatewayPaymentID: mockGatewayPaymentID,
		TransactionID:    request.TransactionID,
		Status:           status, // PENDING - waiting for user to complete OTP/3DS
		Message:          "Payment initiated - redirect user for authentication",
		RedirectURL:      redirectURL, // User should be redirected here for OTP/3DS
		Amount:           request.Amount,
		Currency:         request.Currency,
		CreatedAt:        time.Now(),
		Metadata: map[string]any{
			"gateway":          "mockpg",
			"mock_simulation":  true,
			"payment_method":   string(request.PaymentMethod),
			"tokenized":        request.Token != "",
			"merchant_id":      c.merchantID,
			"simulate_success": c.simulateSuccess,
			"mock_3ds_url":     redirectURL,
		},
	}

	// For tokenization payments, add card details to metadata for later tokenization
	// Mock PG is an "immature" gateway - needs to call tokenize API after payment success
	if isTokenization && request.CardDetails != nil {
		fmt.Println("🔑 Storing card details in metadata for tokenization after payment success")
		response.Metadata["_tokenization_card_details"] = map[string]any{
			"card_number":    request.CardDetails.Number,
			"expiry_month":   request.CardDetails.ExpiryMonth,
			"expiry_year":    request.CardDetails.ExpiryYear,
			"holder_name":    request.CardDetails.HolderName,
			"card_type":      string(request.CardDetails.CardType),
			"card_category":  string(request.CardDetails.CardCategory),
			"last_four":      request.CardDetails.LastFourDigits,
		}
		response.Metadata["_requires_tokenization"] = true
		fmt.Println("✅ Card details stored securely in metadata")
	}

	fmt.Printf("📌 Note: User should open the redirect URL to complete authentication\n")
	fmt.Println("=== MOCK PG GATEWAY: InitiatePayment Complete ===")
	return response, nil
}

// ValidateWebhook validates mock webhook signatures
// For mock gateway, we do basic validation without actual signature verification
func (c *MockPGClient) ValidateWebhook(payload []byte, signature string) error {
	fmt.Println("=== MOCK PG GATEWAY: ValidateWebhook Called ===")
	fmt.Printf("📥 Payload size: %d bytes\n", len(payload))
	fmt.Printf("🔐 Signature: %s\n", signature)

	// Mock validation - always succeeds in simulation mode
	if c.simulateSuccess {
		fmt.Println("✅ Webhook validation successful (mock mode)")
		return nil
	}

	// In failure simulation mode, validate signature format
	if signature == "" {
		fmt.Println("❌ Webhook validation failed: missing signature")
		return fmt.Errorf("webhook signature is required")
	}

	if len(signature) < 32 {
		fmt.Println("❌ Webhook validation failed: invalid signature format")
		return fmt.Errorf("invalid webhook signature format")
	}

	fmt.Println("✅ Webhook validation successful")
	return nil
}

// === HELPER FUNCTIONS ===

// simulateDelay adds artificial delay to simulate network latency
func (c *MockPGClient) simulateDelay(ctx context.Context) {
	if c.delayMs > 0 {
		select {
		case <-time.After(time.Duration(c.delayMs) * time.Millisecond):
		case <-ctx.Done():
			return
		}
	}
}

// getMockBankName returns a mock bank name based on card type
func getMockBankName(cardType payment.CardType) string {
	switch cardType {
	case payment.CardTypeVisa:
		return "Mock Visa Bank"
	case payment.CardTypeMastercard:
		return "Mock Mastercard Bank"
	case payment.CardTypeRupay:
		return "Mock RuPay Bank"
	case payment.CardTypeAmex:
		return "Mock American Express"
	case payment.CardTypeDiners:
		return "Mock Diners Club"
	default:
		return "Mock Bank"
	}
}

// extractBaseURL extracts the base URL from a callback URL
// Example: extractBaseURL("http://localhost:8888/v1/process_payment/callback_page?transaction_id=xxx&source=web")
// Returns: "http://localhost:8888"
func extractBaseURL(callbackURL string) string {
	// Find the position of the first slash after "://"
	// This handles both http:// and https://
	schemeEnd := 0
	if idx := findSubstring(callbackURL, "://"); idx >= 0 {
		schemeEnd = idx + 3
	}

	// Find the first slash after the scheme (this marks the end of host:port)
	pathStart := schemeEnd
	for i := schemeEnd; i < len(callbackURL); i++ {
		if callbackURL[i] == '/' {
			pathStart = i
			break
		}
	}

	// Return everything up to (but not including) the first path slash
	if pathStart > schemeEnd {
		return callbackURL[:pathStart]
	}

	// Fallback: return the whole URL if no path found
	return callbackURL
}

// findSubstring finds the index of a substring in a string
func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// === WEBHOOK SIMULATION ===

// MockPGWebhookPayload represents the webhook payload sent by Mock PG
// This simulates the payment completion notification sent after user completes OTP/3DS
type MockPGWebhookPayload struct {
	MerchantID       string `json:"merchant_id"`        // Mock merchant ID
	TransactionID    string `json:"transaction_id"`     // Our payment ID
	GatewayPaymentID string `json:"gateway_payment_id"` // Mock gateway payment ID
	Status           string `json:"status"`             // "success" or "failed"
	Amount           int64  `json:"amount"`             // Amount in smallest unit
	Currency         string `json:"currency"`           // Currency code
	PaymentMethod    string `json:"payment_method"`     // Payment method used
	ProcessedAt      string `json:"processed_at"`       // ISO 8601 timestamp
	Signature        string `json:"signature"`          // Mock signature
}

// sendWebhookAfterDelay simulates the async webhook sent by payment gateways
// This runs in a background goroutine and sends the webhook after the configured delay
func (c *MockPGClient) sendWebhookAfterDelay(callbackURL, transactionID, gatewayPaymentID string, amount int64, currency, paymentMethod string) {
	go func() {
		fmt.Printf("⏰ Mock PG: Webhook scheduled for transaction %s (will fire in %v)\n", transactionID, c.webhookDelay)

		// Wait for the configured delay
		time.Sleep(c.webhookDelay)

		// Determine webhook status based on simulateSuccess
		webhookStatus := "success"
		if !c.simulateSuccess {
			// 10% failure rate for webhook
			if time.Now().UnixNano()%10 == 0 {
				webhookStatus = "failed"
			}
		}

		// Build webhook payload
		payload := MockPGWebhookPayload{
			MerchantID:       c.merchantID,
			TransactionID:    transactionID,
			GatewayPaymentID: gatewayPaymentID,
			Status:           webhookStatus,
			Amount:           amount,
			Currency:         currency,
			PaymentMethod:    paymentMethod,
			ProcessedAt:      time.Now().UTC().Format(time.RFC3339),
			Signature:        fmt.Sprintf("MOCK_SIG_%s", uuid.Must(uuid.NewV7()).String()[:16]),
		}

		// Marshal to JSON
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			fmt.Printf("❌ Mock PG: Failed to marshal webhook payload: %v\n", err)
			return
		}

		fmt.Printf("📤 Mock PG: Sending webhook to %s\n", callbackURL)
		fmt.Printf("📦 Webhook payload: %s\n", string(payloadBytes))

		// Send POST request to callback URL
		client := &http.Client{
			Timeout: 30 * time.Second,
		}

		resp, err := client.Post(callbackURL, "application/json", bytes.NewReader(payloadBytes))
		if err != nil {
			fmt.Printf("❌ Mock PG: Failed to send webhook: %v\n", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			fmt.Printf("✅ Mock PG: Webhook delivered successfully (status: %d, payment: %s)\n", resp.StatusCode, webhookStatus)
		} else {
			fmt.Printf("⚠️  Mock PG: Webhook delivery got status %d\n", resp.StatusCode)
		}
	}()
}
