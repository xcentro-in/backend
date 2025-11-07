package mockpg

import (
	"context"
	"fmt"
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
	simulateSuccess bool // If true, always return success; if false, simulate failures
	delayMs         int  // Artificial delay in milliseconds to simulate network latency
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
		delayMs:         100, // Simulate 100ms network delay
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
func (c *MockPGClient) InitiatePayment(ctx context.Context, request payment.GatewayPaymentRequest) (*payment.GatewayPaymentResponse, error) {
	fmt.Println("=== MOCK PG GATEWAY: InitiatePayment Called ===")
	fmt.Printf("📥 Business ID: %s\n", request.BusinessID)
	fmt.Printf("📥 Transaction ID: %s\n", request.TransactionID)
	fmt.Printf("💰 Amount: %d %s\n", request.Amount, request.Currency)
	fmt.Printf("🔀 Payment Method: %s\n", request.PaymentMethod)

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

	// Generate mock redirect URL (simulates 3DS/OTP page)
	redirectURL := fmt.Sprintf("https://mockpg.example.com/auth?txn=%s&merchant=%s",
		request.TransactionID, c.merchantID)

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
			fmt.Printf("📱 UPI ID: %s\n", request.UPIDetails.UPIID)
		}
	case payment.PaymentMethodWallet:
		fmt.Println("👛 Processing wallet payment...")
		if request.WalletDetails != nil {
			fmt.Printf("👛 Wallet: %s\n", request.WalletDetails.WalletProvider)
		}
	}

	fmt.Printf("✅ Payment initiated successfully\n")
	fmt.Printf("🌐 Redirect URL: %s\n", redirectURL)
	fmt.Printf("🔑 Gateway Payment ID: %s\n", mockGatewayPaymentID)

	// Create response
	response := &payment.GatewayPaymentResponse{
		PaymentID:        request.TransactionID,
		GatewayPaymentID: mockGatewayPaymentID,
		TransactionID:    request.TransactionID,
		Status:           status,
		Message:          "Payment initiated successfully via Mock PG",
		RedirectURL:      redirectURL,
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
		},
	}

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
