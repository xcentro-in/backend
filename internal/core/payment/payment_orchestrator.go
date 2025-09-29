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
	// Gateway instances for payment processing
	jioPayGateway   PaymentGateway
	easebuzzGateway PaymentGateway
}

// NewMockPaymentOrchestrator creates a new mock payment orchestrator
func NewMockPaymentOrchestrator() PaymentOrchestrator {
	return &MockPaymentOrchestrator{
		jioPayGateway:   NewMockJioPayGateway(),
		easebuzzGateway: NewMockEasebuzzGateway(),
	}
}

// ProcessPayment processes a payment request with detailed logging
func (o *MockPaymentOrchestrator) ProcessPayment(ctx context.Context, request PaymentRequest) (*PaymentResponse, error) {
	fmt.Println("=== ORCHESTRATOR: ProcessPayment Called ===")
	fmt.Printf("Business ID: %s\n", request.BusinessID)
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

	// Generate transaction IDs
	transactionID := fmt.Sprintf("txn_%s_%s", time.Now().Format("20060102"), uuid.New().String()[:8])
	paymentID := uuid.New().String()
	fmt.Printf("🆔 Generated Transaction ID: %s\n", transactionID)
	fmt.Printf("🆔 Generated Payment ID: %s\n", paymentID)

	// Get selected gateway instance
	selectedGateway := o.getGatewayInstance(gateway)
	if selectedGateway == nil {
		return nil, fmt.Errorf("gateway instance not found: %s", gateway)
	}

	// Prepare gateway payment request
	gatewayRequest := GatewayPaymentRequest{
		BusinessID:      request.BusinessID,
		TransactionID:   transactionID,
		Amount:          request.Amount + feeAmount, // Include fee in total
		Currency:        request.Currency,
		PaymentMethod:   request.PaymentMethod,
		Description:     request.Description,
		CallbackURL:     request.CallbackURL,
		CardDetails:     request.CardDetails,
		NetBankingDetails: request.NetBankingDetails,
		UPIDetails:      request.UPIDetails,
		WalletDetails:   request.WalletDetails,
		Metadata:        request.Metadata,
	}

	// Call actual gateway
	fmt.Printf("🚀 Calling %s gateway for payment initiation...\n", gateway)
	gatewayResponse, err := selectedGateway.InitiatePayment(ctx, gatewayRequest)
	if err != nil {
		fmt.Printf("❌ Gateway call failed: %v\n", err)
		return nil, fmt.Errorf("gateway payment initiation failed: %w", err)
	}

	// Create final response
	response := &PaymentResponse{
		PaymentID:        paymentID,
		Status:           gatewayResponse.Status,
		Amount:           request.Amount,
		FeeAmount:        feeAmount,
		TotalAmount:      request.Amount + feeAmount,
		Currency:         request.Currency,
		PaymentMethod:    request.PaymentMethod,
		GatewayUsed:      gateway,
		GatewayPaymentID: gatewayResponse.GatewayPaymentID,
		RedirectURL:      gatewayResponse.RedirectURL,
		Message:          gatewayResponse.Message,
		CreatedAt:        gatewayResponse.CreatedAt,
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
	fmt.Printf("Business ID: %s\n", request.BusinessID)
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

	// Generate transaction ID for tokenization
	transactionID := fmt.Sprintf("tok_%s_%s", time.Now().Format("20060102"), uuid.New().String()[:8])
	fmt.Printf("🆔 Generated Tokenization Transaction ID: %s\n", transactionID)

	// Get selected gateway instance
	selectedGateway := o.getGatewayInstance(gateway)
	if selectedGateway == nil {
		return nil, fmt.Errorf("gateway instance not found for tokenization: %s", gateway)
	}

	// Prepare gateway tokenization request
	gatewayRequest := GatewayTokenizeRequest{
		BusinessID:    request.BusinessID,
		TransactionID: transactionID,
		CardDetails:   request.CardDetails,
		CallbackURL:   "https://mock-callback.com/tokenize", // TODO: Make configurable
		Metadata: map[string]any{
			"tokenization_request": true,
			"business_id":         request.BusinessID,
		},
	}

	// For Easebuzz, add amount requirement
	if gateway == GatewayEasebuzz {
		gatewayRequest.Amount = 100 // Minimum amount for Easebuzz tokenization (₹1)
		gatewayRequest.Currency = CurrencyINR
		fmt.Printf("💰 Adding amount for Easebuzz tokenization: %d %s\n", gatewayRequest.Amount, gatewayRequest.Currency)
	}

	// Call actual gateway for tokenization
	fmt.Printf("🚀 Calling %s gateway for tokenization...\n", gateway)
	gatewayResponse, err := selectedGateway.TokenizeCard(ctx, gatewayRequest)
	if err != nil {
		fmt.Printf("❌ Gateway tokenization failed: %v\n", err)
		return nil, fmt.Errorf("gateway tokenization failed: %w", err)
	}

	// Create final response
	response := &TokenizeResponse{
		Token:          gatewayResponse.Token,
		CardID:         gatewayResponse.CardID,
		LastFourDigits: request.CardDetails.LastFourDigits,
		CardType:       request.CardDetails.CardType,
		CardCategory:   request.CardDetails.CardCategory,
		ExpiryMonth:    request.CardDetails.ExpiryMonth,
		ExpiryYear:     request.CardDetails.ExpiryYear,
		GatewayUsed:    gateway,
		Status:         gatewayResponse.Status,
		Message:        gatewayResponse.Message,
		CreatedAt:      gatewayResponse.ExpiresAt.Add(-15 * time.Minute), // Mock created time
	}

	fmt.Printf("✅ Card tokenization successful with %s\n", gateway)
	fmt.Println("=== ORCHESTRATOR: TokenizeCard Complete ===")

	return response, nil
}

// ValidatePaymentRequest validates a payment request
func (o *MockPaymentOrchestrator) ValidatePaymentRequest(request PaymentRequest) error {
	fmt.Println("--- Validating Payment Request ---")

	if request.BusinessID == "" {
		return fmt.Errorf("business_id is required")
	}
	fmt.Println("✓ Business ID is valid")

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

// getGatewayInstance returns the appropriate gateway instance
func (o *MockPaymentOrchestrator) getGatewayInstance(gatewayType GatewayType) PaymentGateway {
	switch gatewayType {
	case GatewayJioPay:
		return o.jioPayGateway
	case GatewayEasebuzz:
		return o.easebuzzGateway
	default:
		return nil
	}
}