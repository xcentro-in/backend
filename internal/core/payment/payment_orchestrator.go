package payment

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// PaymentOrchestrator defines the interface for payment orchestration
type PaymentOrchestrator interface {
	ProcessPayment(ctx context.Context, request PaymentRequest) (*PaymentResponse, error)
	TokenizeCard(ctx context.Context, request TokenizeRequest) (*TokenizeResponse, error)
	ValidatePaymentRequest(request PaymentRequest) error
	SelectGateway(request PaymentRequest) (GatewayType, error)
}

// MockPaymentOrchestrator provides a mock implementation with detailed logging
type MockPaymentOrchestrator struct {
	// No dependencies yet - just print-based implementation
}

// NewMockPaymentOrchestrator creates a new mock payment orchestrator
func NewMockPaymentOrchestrator() PaymentOrchestrator {
	return &MockPaymentOrchestrator{}
}

// ProcessPayment processes a payment request with detailed logging
func (o *MockPaymentOrchestrator) ProcessPayment(ctx context.Context, request PaymentRequest) (*PaymentResponse, error) {
	fmt.Println("=== ORCHESTRATOR: ProcessPayment Called ===")
	fmt.Printf("User ID: %s\n", request.UserID)
	fmt.Printf("Amount: %d %s\n", request.Amount, request.Currency)
	fmt.Printf("Payment Method: %s\n", request.PaymentMethod)
	fmt.Printf("Description: %s\n", request.Description)

	// Validate request
	if err := o.ValidatePaymentRequest(request); err != nil {
		fmt.Printf("❌ Validation failed: %v\n", err)
		return nil, err
	}
	fmt.Println("✅ Request validation passed")

	// Select gateway
	gateway, err := o.SelectGateway(request)
	if err != nil {
		fmt.Printf("❌ Gateway selection failed: %v\n", err)
		return nil, err
	}
	fmt.Printf("🎯 Selected Gateway: %s\n", gateway)

	// Mock fee calculation
	feeAmount := o.calculateMockFee(request.Amount, request.PaymentMethod)
	fmt.Printf("💰 Calculated Fee: %d %s\n", feeAmount, request.Currency)

	// Generate mock payment ID
	paymentID := uuid.New().String()
	fmt.Printf("🆔 Generated Payment ID: %s\n", paymentID)

	// Mock gateway payment ID
	gatewayPaymentID := fmt.Sprintf("%s_%s", gateway, paymentID[:8])
	fmt.Printf("🏦 Gateway Payment ID: %s\n", gatewayPaymentID)

	// Create mock response
	response := &PaymentResponse{
		PaymentID:        paymentID,
		Status:           PaymentStatusPending,
		Amount:           request.Amount,
		FeeAmount:        feeAmount,
		TotalAmount:      request.Amount + feeAmount,
		Currency:         request.Currency,
		PaymentMethod:    request.PaymentMethod,
		GatewayUsed:      gateway,
		GatewayPaymentID: gatewayPaymentID,
		RedirectURL:      fmt.Sprintf("https://mock-%s.com/payment/%s", gateway, gatewayPaymentID),
		Message:          "Payment initiated successfully",
		CreatedAt:        time.Now(),
		Metadata:         request.Metadata,
	}

	fmt.Printf("🚀 Payment processing initiated with %s gateway\n", gateway)
	fmt.Printf("💳 Redirect URL: %s\n", response.RedirectURL)
	fmt.Println("=== ORCHESTRATOR: ProcessPayment Complete ===")

	return response, nil
}

// TokenizeCard tokenizes a card with detailed logging
func (o *MockPaymentOrchestrator) TokenizeCard(ctx context.Context, request TokenizeRequest) (*TokenizeResponse, error) {
	fmt.Println("=== ORCHESTRATOR: TokenizeCard Called ===")
	fmt.Printf("User ID: %s\n", request.UserID)
	fmt.Printf("Card Type: %s\n", request.CardDetails.CardType)
	fmt.Printf("Card Category: %s\n", request.CardDetails.CardCategory)
	fmt.Printf("Last Four Digits: %s\n", request.CardDetails.LastFourDigits)

	// Select gateway for tokenization (same logic as payment)
	mockPaymentRequest := PaymentRequest{
		PaymentMethod: PaymentMethodCard,
		CardDetails:   &request.CardDetails,
	}
	gateway, err := o.SelectGateway(mockPaymentRequest)
	if err != nil {
		fmt.Printf("❌ Gateway selection for tokenization failed: %v\n", err)
		return nil, err
	}
	fmt.Printf("🎯 Selected Gateway for Tokenization: %s\n", gateway)

	// Generate mock token
	token := fmt.Sprintf("tok_%s_%s_%s", gateway, request.CardDetails.CardType, uuid.New().String()[:8])
	cardID := uuid.New().String()

	fmt.Printf("🔐 Generated Token: %s\n", token)
	fmt.Printf("🆔 Generated Card ID: %s\n", cardID)

	response := &TokenizeResponse{
		Token:          token,
		CardID:         cardID,
		LastFourDigits: request.CardDetails.LastFourDigits,
		CardType:       request.CardDetails.CardType,
		CardCategory:   request.CardDetails.CardCategory,
		ExpiryMonth:    request.CardDetails.ExpiryMonth,
		ExpiryYear:     request.CardDetails.ExpiryYear,
		GatewayUsed:    gateway,
		Status:         "success",
		Message:        "Card tokenized successfully",
		CreatedAt:      time.Now(),
	}

	fmt.Printf("✅ Card tokenization successful with %s\n", gateway)
	fmt.Println("=== ORCHESTRATOR: TokenizeCard Complete ===")

	return response, nil
}

// ValidatePaymentRequest validates a payment request
func (o *MockPaymentOrchestrator) ValidatePaymentRequest(request PaymentRequest) error {
	fmt.Println("--- Validating Payment Request ---")

	if request.UserID == "" {
		return fmt.Errorf("user_id is required")
	}
	fmt.Println("✓ User ID is valid")

	if request.Amount <= 0 {
		return fmt.Errorf("amount must be greater than 0")
	}
	fmt.Printf("✓ Amount is valid: %d\n", request.Amount)

	if request.Currency == "" {
		return fmt.Errorf("currency is required")
	}
	fmt.Printf("✓ Currency is valid: %s\n", request.Currency)

	if request.PaymentMethod == "" {
		return fmt.Errorf("payment_method is required")
	}
	fmt.Printf("✓ Payment method is valid: %s\n", request.PaymentMethod)

	// Validate payment method specific details
	switch request.PaymentMethod {
	case PaymentMethodCard:
		if request.CardDetails == nil {
			return fmt.Errorf("card_details are required for card payments")
		}
		fmt.Println("✓ Card details provided for card payment")
	case PaymentMethodNetBanking:
		if request.NetBankingDetails == nil {
			return fmt.Errorf("net_banking_details are required for net banking payments")
		}
		fmt.Println("✓ Net banking details provided")
	case PaymentMethodUPI:
		if request.UPIDetails == nil {
			return fmt.Errorf("upi_details are required for UPI payments")
		}
		fmt.Println("✓ UPI details provided")
	case PaymentMethodWallet:
		if request.WalletDetails == nil {
			return fmt.Errorf("wallet_details are required for wallet payments")
		}
		fmt.Println("✓ Wallet details provided")
	}

	fmt.Println("--- Validation Complete: All checks passed ---")
	return nil
}

// SelectGateway selects the appropriate gateway based on payment method and card details
func (o *MockPaymentOrchestrator) SelectGateway(request PaymentRequest) (GatewayType, error) {
	fmt.Println("--- Gateway Selection Logic ---")

	switch request.PaymentMethod {
	case PaymentMethodNetBanking:
		fmt.Println("🏦 Net Banking detected")
		fmt.Println("📍 Route Rule: All Net Banking → JioPay")
		return GatewayJioPay, nil

	case PaymentMethodUPI:
		fmt.Println("📱 UPI detected")
		fmt.Println("📍 Route Rule: All UPI → JioPay (default)")
		return GatewayJioPay, nil

	case PaymentMethodWallet:
		fmt.Println("👛 Wallet detected")
		fmt.Println("📍 Route Rule: All Wallet → JioPay (default)")
		return GatewayJioPay, nil

	case PaymentMethodCard:
		if request.CardDetails == nil {
			return "", fmt.Errorf("card details are required for card payments")
		}

		cardType := request.CardDetails.CardType
		cardCategory := request.CardDetails.CardCategory

		fmt.Printf("💳 Card detected: %s %s\n", cardCategory, cardType)

		// JioPay routing rules
		if cardCategory == CardCategoryCredit {
			fmt.Println("💎 Credit Card detected")
			switch cardType {
			case CardTypeVisa, CardTypeMastercard, CardTypeRupay:
				fmt.Printf("📍 Route Rule: %s Credit → JioPay\n", cardType)
				return GatewayJioPay, nil
			}
		}

		if cardCategory == CardCategoryCorporate {
			fmt.Println("🏢 Corporate Card detected")
			fmt.Println("📍 Route Rule: All Corporate Cards → JioPay")
			return GatewayJioPay, nil
		}

		// Easebuzz routing rules
		if cardType == CardTypeAmex || cardType == CardTypeDiners {
			fmt.Printf("🎯 %s Card detected\n", cardType)
			fmt.Printf("📍 Route Rule: %s Cards → Easebuzz\n", cardType)
			return GatewayEasebuzz, nil
		}

		if cardCategory == CardCategoryDebit && (cardType == CardTypeVisa || cardType == CardTypeMastercard) {
			fmt.Printf("💳 %s Debit Card detected\n", cardType)
			fmt.Printf("📍 Route Rule: %s Debit → Easebuzz\n", cardType)
			return GatewayEasebuzz, nil
		}

		// Default fallback
		fmt.Printf("⚠️  Unmatched card combination: %s %s\n", cardCategory, cardType)
		fmt.Println("📍 Route Rule: Fallback → JioPay")
		return GatewayJioPay, nil

	default:
		return "", fmt.Errorf("unsupported payment method: %s", request.PaymentMethod)
	}
}

// calculateMockFee calculates a mock fee based on payment method
func (o *MockPaymentOrchestrator) calculateMockFee(amount int64, method PaymentMethod) int64 {
	switch method {
	case PaymentMethodCard:
		return amount * 25 / 1000 // 2.5%
	case PaymentMethodNetBanking:
		return amount * 15 / 1000 // 1.5%
	case PaymentMethodUPI:
		return 0 // 0% fee
	case PaymentMethodWallet:
		return amount * 10 / 1000 // 1.0%
	default:
		return amount * 20 / 1000 // 2.0% default
	}
}