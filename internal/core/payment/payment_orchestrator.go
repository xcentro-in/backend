package payment

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// PaymentOrchestrator defines the interface for payment orchestration
type PaymentOrchestrator interface {
	ProcessPayment(ctx context.Context, request PaymentRequest) (*PaymentResponse, error)
	TokenizeCard(ctx context.Context, request TokenizeRequest) (*TokenizeResponse, error)
	ValidatePaymentRequest(request PaymentRequest) error
	SelectGateway(request PaymentRequest) (GatewayType, error)
	// Voucher service dependency injection (Phase 6)
	SetVoucherService(voucherService interface{})
}

// MockPaymentOrchestrator provides a mock implementation with detailed logging
type MockPaymentOrchestrator struct {
	// Gateway instances for payment processing
	jioPayGateway   PaymentGateway
	easebuzzGateway PaymentGateway
	// Simple storage for tokenized cards (for Phase 3C mock)
	// TODO-DB-4.1-CRITICAL: Remove in-memory storage
	// - Delete this map: storedCards map[string][]*StoredCard
	// - All card storage should go through WalletService -> Repository
	// - Orchestrator should be stateless
	storedCards map[string][]*StoredCard // businessID -> cards
	// Repository dependencies (Phase 4.2)
	paymentRepo     PaymentRepository
	storedCardRepo  StoredCardRepository
	// Voucher service for finalization (Phase 6)
	voucherService  interface{} // Will be voucher.VoucherService, using interface{} to avoid circular import
}

// NewMockPaymentOrchestrator creates a new mock payment orchestrator
// TODO: Rename to NewPaymentOrchestrator when moving to production
func NewMockPaymentOrchestrator(
	paymentRepo PaymentRepository,
	storedCardRepo StoredCardRepository,
	jioPayClient PaymentGateway,   // Real JioPay client injected
	easebuzzClient PaymentGateway, // Real Easebuzz client injected
) PaymentOrchestrator {
	return &MockPaymentOrchestrator{
		jioPayGateway:   jioPayClient,       // ✅ Real JioPay client
		easebuzzGateway: easebuzzClient,     // ✅ Real Easebuzz client
		storedCards:     make(map[string][]*StoredCard),
		paymentRepo:     paymentRepo,
		storedCardRepo:  storedCardRepo,
	}
}

// ProcessPayment processes a payment request with detailed logging
func (o *MockPaymentOrchestrator) ProcessPayment(ctx context.Context, request PaymentRequest) (*PaymentResponse, error) {
	fmt.Println("=== ORCHESTRATOR: ProcessPayment Called ===")
	fmt.Printf("Business ID: %s\n", request.BusinessID)
	fmt.Printf("Amount: %d %s\n", request.Amount, request.Currency)
	fmt.Printf("Payment Method: %s\n", request.PaymentMethod)
	fmt.Printf("Description: %s\n", request.Description)

	// Handle token-based payments differently
	if request.PaymentMethod == PaymentMethodTokenCard {
		return o.ProcessTokenizedPayment(ctx, request)
	}

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

	// Use payment_id provided by service layer (already exists in DB)
	paymentID := request.PaymentID
	if paymentID == "" {
		return nil, fmt.Errorf("payment_id is required (should be set by service layer)")
	}
	fmt.Printf("🆔 Using Payment ID from service: %s\n", paymentID)

	// Generate transaction ID for gateway
	transactionID := fmt.Sprintf("txn_%s_%s", time.Now().Format("20060102"), uuid.New().String()[:8])
	fmt.Printf("🆔 Generated Transaction ID: %s\n", transactionID)

	// TODO-DB-4.1-CRITICAL: Update payment record with selected gateway
	// - Call paymentRepo.UpdatePaymentGateway(ctx, paymentID, gateway)
	// - This records which gateway was selected for routing
	_ = o.paymentRepo.UpdatePaymentGateway(ctx, paymentID, gateway)

	// Mock fee calculation
	feeAmount := o.calculateMockFee(request.Amount, request.PaymentMethod)
	fmt.Printf("💰 Calculated Fee: %d %s\n", feeAmount, request.Currency)

	// TODO-DB-4.1-CRITICAL: Update payment with fee breakdown
	// - Call paymentRepo.UpdatePaymentFees(ctx, paymentID, feeAmount, request.Amount + feeAmount)
	// - Store fee_amount and total_amount for accounting
	_ = o.paymentRepo.UpdatePaymentFees(ctx, paymentID, feeAmount, request.Amount+feeAmount)

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

	// TODO-DB-4.1-CRITICAL: Update payment with gateway response
	// - Call paymentRepo.UpdateGatewayPaymentID(ctx, paymentID, gatewayResponse.GatewayPaymentID, gatewayResponse.RedirectURL)
	// - Store gateway's transaction ID for webhook matching
	// - Store redirect URL if present (for 3DS/redirect flows)

	if err != nil {
		// TODO-DB-4.1-CRITICAL: Update payment status to failed
		// - Call paymentRepo.UpdatePaymentStatus(ctx, paymentID, PaymentStatusFailed, err.Error())
		// - Store failure reason in metadata for debugging
		failureMetadata := map[string]any{"error": err.Error()}
		_ = o.paymentRepo.UpdatePaymentStatus(ctx, paymentID, PaymentStatusFailed, failureMetadata)
		fmt.Printf("❌ Gateway call failed: %v\n", err)
		return nil, fmt.Errorf("gateway payment initiation failed: %w", err)
	}

	_ = o.paymentRepo.UpdateGatewayPaymentID(ctx, paymentID, gatewayResponse.GatewayPaymentID, gatewayResponse.RedirectURL)

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

	// TODO-DB-4.1-CRITICAL: Update payment status to pending
	// - Call paymentRepo.UpdatePaymentStatus(ctx, paymentID, PaymentStatusPending, "")
	// - Payment is now pending gateway/bank processing
	_ = o.paymentRepo.UpdatePaymentStatus(ctx, paymentID, PaymentStatusPending, nil)

	// PHASE 6: Finalize vouchers after payment initiation succeeds
	if len(request.VoucherIDs) > 0 && o.voucherService != nil {
		fmt.Printf("📋 Finalizing %d voucher(s) linked to payment...\n", len(request.VoucherIDs))

		finalizedVouchers, voucherErr := o.finalizeVouchers(ctx, paymentID, request.VoucherIDs)

		if voucherErr != nil {
			// Log error but don't fail payment (payment already succeeded)
			fmt.Printf("⚠️  WARNING: Payment succeeded but voucher finalization had issues: %v\n", voucherErr)
			response.VoucherStatus = "failed"
			if response.Metadata == nil {
				response.Metadata = make(map[string]any)
			}
			response.Metadata["voucher_error"] = voucherErr.Error()
			response.VouchersFinalized = finalizedVouchers // Partial success list
		} else {
			fmt.Printf("✅ All %d voucher(s) finalized successfully\n", len(finalizedVouchers))
			response.VoucherStatus = "finalized"
			response.VouchersFinalized = finalizedVouchers
		}
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

	// For Easebuzz, amount is implicit (₹1 built into tokenization API)
	if gateway == GatewayEasebuzz {
		fmt.Printf("💰 Easebuzz tokenization: ₹1 amount is implicit in API\n")
	}

	// Call actual gateway for tokenization
	fmt.Printf("🚀 Calling %s gateway for tokenization...\n", gateway)
	gatewayResponse, err := selectedGateway.TokenizeCard(ctx, gatewayRequest)
	if err != nil {
		fmt.Printf("❌ Gateway tokenization failed: %v\n", err)
		return nil, fmt.Errorf("gateway tokenization failed: %w", err)
	}
	fmt.Print("✅ Gateway tokenization successful\n")

	// Store tokenized card in database
	fmt.Println("💾 Storing tokenized card in database...")
	storedCard := &StoredCard{
		CardID:         gatewayResponse.CardID,
		BusinessID:     request.BusinessID,
		Token:          gatewayResponse.Token,
		LastFourDigits: request.CardDetails.LastFourDigits,
		CardType:       request.CardDetails.CardType,
		CardCategory:   request.CardDetails.CardCategory,
		ExpiryMonth:    request.CardDetails.ExpiryMonth,
		ExpiryYear:     request.CardDetails.ExpiryYear,
		IssuerBank:     "", // Issuer bank not available in request, will be populated by gateway metadata if available
		CardHolderName: request.CardDetails.HolderName,
		IsActive:       true,
		GatewayUsed:    gateway,
		AddedAt:        Now(),
		LastUsedAt:     nil,
		Metadata:       gatewayResponse.Metadata,
	}

	// Persist card to database via repository
	err = o.storedCardRepo.CreateStoredCard(ctx, storedCard)
	if err != nil {
		fmt.Printf("❌ Failed to store card in database: %v\n", err)
		// Don't fail the tokenization response, but log the error
		// The gateway has already tokenized the card successfully
		fmt.Printf("⚠️  WARNING: Card tokenized by gateway but failed to save in database\n")
	} else {
		fmt.Printf("✅ Card stored successfully in database: %s\n", storedCard.CardID)
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
		// CreatedAt:      gatewayResponse.ExpiresAt.Add(-15 * time.Minute), // Mock created time
	}

	fmt.Printf("✅ Card tokenization successful with %s\n", gateway)
	fmt.Println("=== ORCHESTRATOR: TokenizeCard Complete ===")

	return response, nil
}

// ProcessTokenizedPayment handles payments using tokenized cards
func (o *MockPaymentOrchestrator) ProcessTokenizedPayment(ctx context.Context, request PaymentRequest) (*PaymentResponse, error) {
	fmt.Println("=== ORCHESTRATOR: ProcessTokenizedPayment Called ===")

	// Validate token card details
	if request.TokenCardDetails == nil {
		fmt.Printf("❌ Token card details are nil\n")
		return nil, fmt.Errorf("token_card_details are required for token card payments")
	}

	if request.TokenCardDetails.CardID == "" {
		fmt.Printf("❌ Card ID is empty\n")
		return nil, fmt.Errorf("card_id is required for token card payments")
	}

	fmt.Printf("📥 Card ID: %s\n", request.TokenCardDetails.CardID)

	// TODO-CRITICAL: Generate payment ID (should come from handler in Phase 4.4)
	paymentID := uuid.New().String()
	fmt.Printf("🆔 Generated Payment ID: %s\n", paymentID)

	// Step 1: Retrieve stored card by card_id using wallet service
	fmt.Println("🔍 Looking up stored card by ID...")
	// Get business wallet to find the stored card
	// This is a simplified approach - in production, you might have a direct method
	storedCard, err := o.getStoredCardByID(request.BusinessID, request.TokenCardDetails.CardID)

	// TODO-DB-4.1-CRITICAL: Query stored card from database
	// - Already has TODO at line 340+ in getStoredCardByID()
	// - This method should call storedCardRepo.GetStoredCardByID()

	if err != nil {
		fmt.Printf("❌ Failed to find stored card: %v\n", err)
		return nil, fmt.Errorf("invalid card_id or card not found: %w", err)
	}

	// TODO-DB-4.1-CRITICAL: Link payment to payment source
	// - Call paymentRepo.UpdatePaymentSource(ctx, paymentID, storedCard.CardID, PaymentSourceCard)
	// - Records which stored card was used for this payment
	_ = o.paymentRepo.UpdatePaymentSource(ctx, paymentID, storedCard.CardID, PaymentSourceCard)

	if !storedCard.IsActive {
		fmt.Printf("❌ Card is inactive: %s\n", storedCard.CardID)
		return nil, fmt.Errorf("card is inactive")
	}

	fmt.Printf("✅ Found stored card: %s %s ending in %s\n",
		storedCard.CardCategory, storedCard.CardType, storedCard.LastFourDigits)
	fmt.Printf("🏦 Gateway Used: %s\n", storedCard.GatewayUsed)
	fmt.Printf("🎫 Token Available: %s\n", storedCard.Token[:10]+"...")

	// Step 2: Get the specific gateway instance that was used for tokenization
	selectedGateway := o.getGatewayInstance(storedCard.GatewayUsed)
	if selectedGateway == nil {
		return nil, fmt.Errorf("gateway instance not found: %s", storedCard.GatewayUsed)
	}

	// Step 3: Generate fresh cryptogram for this payment
	fmt.Println("🔐 Generating fresh cryptogram from gateway...")
	cryptogramRequest := CryptogramRequest{
		Token:         storedCard.Token,
		Amount:        request.Amount,
		Currency:      request.Currency,
		TransactionID: fmt.Sprintf("txn_%s_%s", time.Now().Format("20060102"), uuid.New().String()[:8]),
		BusinessID:    request.BusinessID,
		Metadata: map[string]any{
			"payment_type": "tokenized",
			"card_id":      storedCard.CardID,
		},
	}

	cryptogramResponse, err := selectedGateway.GenerateCryptogram(ctx, cryptogramRequest)
	if err != nil {
		fmt.Printf("❌ Cryptogram generation failed: %v\n", err)
		return nil, fmt.Errorf("failed to generate cryptogram: %w", err)
	}

	fmt.Printf("✅ Cryptogram generated: %s\n", cryptogramResponse.Cryptogram[:10]+"...")
	fmt.Printf("⏰ Valid until: %s\n", cryptogramResponse.ExpiresAt.Format("15:04:05"))

	// Step 4: Calculate fees (same as regular payments)
	feeAmount := o.calculateMockFee(request.Amount, PaymentMethodCard) // Use card method for fee calc
	fmt.Printf("💰 Calculated Fee: %d %s\n", feeAmount, request.Currency)

	// Step 5: Prepare gateway payment request with token + cryptogram
	gatewayRequest := GatewayPaymentRequest{
		BusinessID:    request.BusinessID,
		TransactionID: cryptogramRequest.TransactionID,
		Amount:        request.Amount + feeAmount,
		Currency:      request.Currency,
		PaymentMethod: PaymentMethodCard, // Gateway sees this as card payment
		Description:   request.Description,
		CallbackURL:   request.CallbackURL,
		Token:         storedCard.Token,
		Cryptogram:    cryptogramResponse.Cryptogram,
		Metadata: map[string]any{
			"payment_type": "tokenized",
			"card_id":      storedCard.CardID,
			"original_metadata": request.Metadata,
		},
	}

	// Step 6: Process payment with specific gateway
	fmt.Printf("🚀 Processing payment with %s gateway (tokenized)...\n", storedCard.GatewayUsed)
	gatewayResponse, err := selectedGateway.InitiatePayment(ctx, gatewayRequest)

	// TODO-DB-4.1-CRITICAL: Update card last_used_at timestamp
	// - Call storedCardRepo.UpdateLastUsedAt(ctx, storedCard.CardID)
	// - Track card usage patterns for analytics
	_ = o.storedCardRepo.UpdateLastUsedAt(ctx, storedCard.CardID, time.Now())

	if err != nil {
		fmt.Printf("❌ Gateway payment failed: %v\n", err)
		return nil, fmt.Errorf("gateway payment initiation failed: %w", err)
	}

	// Step 7: Create response
	response := &PaymentResponse{
		PaymentID:        paymentID,
		Status:           gatewayResponse.Status,
		Amount:           request.Amount,
		FeeAmount:        feeAmount,
		TotalAmount:      request.Amount + feeAmount,
		Currency:         request.Currency,
		PaymentMethod:    PaymentMethodTokenCard, // Return the original method
		GatewayUsed:      storedCard.GatewayUsed,
		GatewayPaymentID: gatewayResponse.GatewayPaymentID,
		RedirectURL:      gatewayResponse.RedirectURL,
		Message:          fmt.Sprintf("Tokenized payment initiated with %s", storedCard.GatewayUsed),
		CreatedAt:        gatewayResponse.CreatedAt,
		Metadata: map[string]any{
			"payment_type":    "tokenized",
			"card_id":         storedCard.CardID,
			"card_last_four":  storedCard.LastFourDigits,
			"gateway_used":    string(storedCard.GatewayUsed),
			"original_metadata": request.Metadata,
		},
	}

	fmt.Printf("✅ Tokenized payment initiated: %s\n", paymentID)
	fmt.Printf("💳 Card: %s ending in %s\n", storedCard.CardType, storedCard.LastFourDigits)
	fmt.Printf("🌐 Redirect URL: %s\n", response.RedirectURL)
	fmt.Println("=== ORCHESTRATOR: ProcessTokenizedPayment Complete ===")

	return response, nil
}

// getStoredCardByID retrieves a stored card by its ID from stored cards
func (o *MockPaymentOrchestrator) getStoredCardByID(businessID, cardID string) (*StoredCard, error) {
	fmt.Printf("🔍 Searching for card %s in business wallet...\n", cardID)

	// For Phase 3C, we'll create some mock stored cards to simulate the functionality
	// In production, this would be a database lookup
	// TODO-DB-4.1-CRITICAL: Query stored card from database
	// - Replace mock data generation and in-memory lookup
	// - Call storedCardRepo.GetStoredCardByID(ctx, cardID)
	// - Return error if card not found or not active
	// - Verify card belongs to requesting business_id for security
	ctx := context.Background()
	dbCard, _ := o.storedCardRepo.GetStoredCardByID(ctx, cardID)
	if dbCard != nil && dbCard.BusinessID == businessID && dbCard.IsActive {
		fmt.Printf("✅ Found card in database: %s %s\n", dbCard.CardType, dbCard.LastFourDigits)
		return dbCard, nil
	}
	mockStoredCards := o.generateMockStoredCards(businessID)

	// Also check any real stored cards from tokenization process
	if storedCards, exists := o.storedCards[businessID]; exists {
		mockStoredCards = append(mockStoredCards, storedCards...)
	}

	// Search through stored cards to find the matching card_id
	for _, card := range mockStoredCards {
		if card.CardID == cardID {
			fmt.Printf("✅ Found card: %s %s\n", card.CardType, card.LastFourDigits)
			return card, nil
		}
	}

	return nil, fmt.Errorf("card not found: %s", cardID)
}

// generateMockStoredCards creates mock stored cards for testing
func (o *MockPaymentOrchestrator) generateMockStoredCards(businessID string) []*StoredCard {
	return []*StoredCard{
		{
			CardID:         fmt.Sprintf("card_%s_001", businessID[:8]),
			BusinessID:     businessID,
			Token:          fmt.Sprintf("jio_tok_visa_%s", businessID[:8]),
			LastFourDigits: "4532",
			CardType:       CardTypeVisa,
			CardCategory:   CardCategoryCredit,
			ExpiryMonth:    "12",
			ExpiryYear:     "25",
			IssuerBank:     "HDFC Bank",
			CardHolderName: "Business Account",
			IsActive:       true,
			GatewayUsed:    GatewayJioPay,
			AddedAt:        Now().AddDate(0, -2, 0),
			Metadata: map[string]any{
				"nickname": "Primary Card",
				"mock_data": true,
			},
		},
		{
			CardID:         fmt.Sprintf("card_%s_002", businessID[:8]),
			BusinessID:     businessID,
			Token:          fmt.Sprintf("eb_tok_master_%s", businessID[:8]),
			LastFourDigits: "8765",
			CardType:       CardTypeMastercard,
			CardCategory:   CardCategoryDebit,
			ExpiryMonth:    "08",
			ExpiryYear:     "26",
			IssuerBank:     "ICICI Bank",
			CardHolderName: "Business Account",
			IsActive:       true,
			GatewayUsed:    GatewayEasebuzz,
			AddedAt:        Now().AddDate(0, -1, -15),
			Metadata: map[string]any{
				"nickname": "Backup Card",
				"mock_data": true,
			},
		},
	}
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
	fmt.Println("--- Enhanced Gateway Selection Logic ---")

	switch request.PaymentMethod {
	case PaymentMethodNetBanking:
		fmt.Println("🏦 NetBanking detected")
		fmt.Println("📍 Rule: All NetBanking → JioPay")
		return GatewayJioPay, nil

	case PaymentMethodUPI:
		fmt.Println("📱 UPI detected")
		fmt.Println("📍 Rule: All UPI → JioPay")
		return GatewayJioPay, nil

	case PaymentMethodWallet:
		fmt.Println("👛 Wallet detected")
		fmt.Println("📍 Rule: All Wallet → JioPay")
		return GatewayJioPay, nil

	case PaymentMethodCard:
		return o.selectGatewayForCard(request.CardDetails, request.Amount)

	default:
		return "", fmt.Errorf("unsupported payment method: %s", request.PaymentMethod)
	}
}

// selectGatewayForCard implements the exact card routing business rules
func (o *MockPaymentOrchestrator) selectGatewayForCard(card *Card, amount int64) (GatewayType, error) {
	if card == nil {
		return "", fmt.Errorf("card details are required for card payments")
	}

	fmt.Printf("💳 Card: %s %s, Amount: ₹%.2f\n",
		card.CardCategory, card.CardType, float64(amount)/100)

	// Corporate Credit Cards - All → JioPay
	if card.CardCategory == CardCategoryCorporate {
		fmt.Println("🏢 Corporate Card detected")
		fmt.Printf("📍 Rule: All Corporate CC (%s) → JioPay\n", card.CardType)
		return GatewayJioPay, nil
	}

	// Credit Cards
	if card.CardCategory == CardCategoryCredit {
		fmt.Println("💎 Credit Card detected")
		switch card.CardType {
		case CardTypeVisa, CardTypeMastercard:
			fmt.Printf("📍 Rule: %s Credit → JioPay\n", card.CardType)
			return GatewayJioPay, nil
		case CardTypeRupay:
			fmt.Println("📍 Rule: RuPay Credit → JioPay")
			return GatewayJioPay, nil
		case CardTypeDiners, CardTypeAmex:
			fmt.Printf("📍 Rule: %s Credit → Easebuzz\n", card.CardType)
			return GatewayEasebuzz, nil
		default:
			return "", fmt.Errorf("unsupported credit card type: %s", card.CardType)
		}
	}

	// Debit Cards
	if card.CardCategory == CardCategoryDebit {
		fmt.Println("💳 Debit Card detected")
		switch card.CardType {
		case CardTypeVisa, CardTypeMastercard:
			threshold := int64(200000) // ₹2000 in paise
			if amount < threshold {
				fmt.Printf("📍 Rule: %s Debit < ₹2000 → Easebuzz\n", card.CardType)
				return GatewayEasebuzz, nil
			} else {
				fmt.Printf("📍 Rule: %s Debit >= ₹2000 → JioPay\n", card.CardType)
				return GatewayJioPay, nil
			}
		case CardTypeRupay:
			fmt.Println("📍 Rule: RuPay Debit (any amount) → JioPay")
			return GatewayJioPay, nil
		case CardTypeDiners, CardTypeAmex:
			// Note: Debit cards for Amex/Diners are rare, but handling for completeness
			fmt.Printf("📍 Rule: %s Debit → Easebuzz\n", card.CardType)
			return GatewayEasebuzz, nil
		default:
			return "", fmt.Errorf("unsupported debit card type: %s", card.CardType)
		}
	}

	// Prepaid cards (if any) - fallback to JioPay
	if card.CardCategory == CardCategoryCredit { // Assuming prepaid would be similar category
		fmt.Println("💰 Prepaid Card detected")
		fmt.Println("📍 Rule: Prepaid Cards → JioPay (fallback)")
		return GatewayJioPay, nil
	}

	return "", fmt.Errorf("unsupported card combination: %s %s", card.CardCategory, card.CardType)
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

// SetVoucherService injects the voucher service dependency (Phase 6)
func (o *MockPaymentOrchestrator) SetVoucherService(voucherService interface{}) {
	o.voucherService = voucherService
	fmt.Println("✅ Voucher service injected into payment orchestrator")
}

// finalizeVouchers finalizes vouchers after payment success (Phase 6)
func (o *MockPaymentOrchestrator) finalizeVouchers(ctx context.Context, paymentID string, voucherIDs []string) ([]string, error) {
	if o.voucherService == nil {
		return nil, fmt.Errorf("voucher service not initialized")
	}

	// Type assert to voucher service interface
	// Using reflection to avoid circular import
	type voucherServiceInterface interface {
		FinalizeVoucherByID(ctx context.Context, voucherID string, paymentID string) error
	}

	voucherSvc, ok := o.voucherService.(voucherServiceInterface)
	if !ok {
		return nil, fmt.Errorf("invalid voucher service type")
	}

	finalizedVouchers := make([]string, 0, len(voucherIDs))
	var finalizeErrors []string

	for _, voucherID := range voucherIDs {
		fmt.Printf("📄 Finalizing voucher: %s\n", voucherID)

		err := voucherSvc.FinalizeVoucherByID(ctx, voucherID, paymentID)
		if err != nil {
			errMsg := fmt.Sprintf("voucher %s: %v", voucherID, err)
			finalizeErrors = append(finalizeErrors, errMsg)
			fmt.Printf("❌ Failed to finalize voucher %s: %v\n", voucherID, err)
		} else {
			finalizedVouchers = append(finalizedVouchers, voucherID)
			fmt.Printf("✅ Voucher %s finalized\n", voucherID)
		}
	}

	// Return partial success
	if len(finalizeErrors) > 0 {
		return finalizedVouchers, fmt.Errorf("finalization errors: %s", strings.Join(finalizeErrors, "; "))
	}

	return finalizedVouchers, nil
}