package payment

import (
	"context"
	"fmt"

	"xcentro.in/internal/shared/logger"
)

// PaymentOrchestrator defines the interface for payment orchestration
// The orchestrator is responsible for gateway selection and communication only.
// All database persistence operations are handled by the service layer within transactions.
type PaymentOrchestrator interface {
	ProcessPayment(ctx context.Context, request PaymentRequest) (*PaymentResponse, error)
	TokenizeCard(ctx context.Context, request TokenizeRequest) (*TokenizeResponse, error)
	ValidatePaymentRequest(request PaymentRequest) error
	SelectGateway(request PaymentRequest) (GatewayType, error)
}

// paymentOrchestratorImpl is the standard implementation of PaymentOrchestrator
// It handles gateway selection, routing, and communication with payment gateways
type paymentOrchestratorImpl struct {
	// Gateway instances for payment processing
	jioPayGateway   PaymentGateway
	easebuzzGateway PaymentGateway
	mockPGGateway   PaymentGateway // Mock PG for testing
	mockPGEnabled   bool            // Flag to enable mock PG bypass
	// Repository dependencies - read-only access for gateway communication
	// NOTE: All database writes are handled by service layer within transactions
	paymentRepo    PaymentRepository
	storedCardRepo StoredCardRepository
	// Logger for structured logging
	logger *logger.Logger
}

// NewPaymentOrchestrator creates a new payment orchestrator
//
// The orchestrator itself is always "real" - it handles actual gateway selection and routing.
// The gateway clients (jioPayClient, easebuzzClient, mockPGClient) can be real or mock implementations.
func NewPaymentOrchestrator(
	paymentRepo PaymentRepository,
	storedCardRepo StoredCardRepository,
	jioPayClient PaymentGateway, // JioPay gateway client
	easebuzzClient PaymentGateway, // Easebuzz gateway client
	mockPGClient PaymentGateway, // Mock PG client for testing
	mockPGEnabled bool, // Enable mock PG bypass
	log *logger.Logger,
) PaymentOrchestrator {
	return &paymentOrchestratorImpl{
		jioPayGateway:   jioPayClient,
		easebuzzGateway: easebuzzClient,
		mockPGGateway:   mockPGClient,
		mockPGEnabled:   mockPGEnabled,
		paymentRepo:     paymentRepo,
		storedCardRepo:  storedCardRepo,
		logger:          log,
	}
}

// ProcessPayment processes a payment request with detailed logging
func (o *paymentOrchestratorImpl) ProcessPayment(ctx context.Context, request PaymentRequest) (*PaymentResponse, error) {
	o.logger.InfoWith("Orchestrator: ProcessPayment called",
		"business_id", request.BusinessID,
		"amount", request.Amount,
		"currency", request.Currency,
		"payment_method", request.PaymentMethod)
	o.logger.DebugWith("Payment request details", "description", request.Description)

	// Handle token-based payments differently
	if request.PaymentMethod == PaymentMethodTokenCard {
		return o.ProcessTokenizedPayment(ctx, request)
	}

	// Validate request
	if err := o.ValidatePaymentRequest(request); err != nil {
		o.logger.ErrorWith("Payment request validation failed", "error", err)
		return nil, err
	}
	o.logger.DebugWith("Request validation passed")

	// Select gateway
	gateway, err := o.SelectGateway(request)
	if err != nil {
		o.logger.ErrorWith("Gateway selection failed", "error", err)
		return nil, err
	}
	o.logger.InfoWith("Gateway selected", "gateway", gateway)

	// Use payment_id provided by service layer (already exists in DB)
	paymentID := request.PaymentID
	if paymentID == "" {
		o.logger.ErrorWith("Payment ID missing from request")
		return nil, fmt.Errorf("payment_id is required (should be set by service layer)")
	}
	o.logger.DebugWith("Using payment ID from service", "payment_id", paymentID)

	// Mock fee calculation
	feeAmount := o.calculateMockFee(request.Amount, request.PaymentMethod)
	o.logger.DebugWith("Fee calculated", "fee_amount", feeAmount, "currency", request.Currency)

	// NOTE: Fee calculation and gateway selection info will be returned in response
	// Service layer will persist this information within a transaction

	// Get selected gateway instance
	selectedGateway := o.getGatewayInstance(gateway)
	if selectedGateway == nil {
		return nil, fmt.Errorf("gateway instance not found: %s", gateway)
	}

	// Prepare gateway payment request
	// Note: CallbackURL comes from request (set by payment service based on calling device)
	// We don't fetch from DB because payment is in uncommitted transaction
	gatewayRequest := GatewayPaymentRequest{
		BusinessID:        request.BusinessID,
		TransactionID:     paymentID, // Use payment UUID as gateway transaction ID
		Amount:            request.Amount + feeAmount, // Include fee in total
		Currency:          request.Currency,
		PaymentMethod:     request.PaymentMethod,
		Description:       request.Description,
		CallbackURL:       request.CallbackURL, // From request, not DB
		CardDetails:       request.CardDetails,
		NetBankingDetails: request.NetBankingDetails,
		UPIDetails:        request.UPIDetails,
		WalletDetails:     request.WalletDetails,
		Metadata:          request.Metadata,
	}

	// Call actual gateway
	o.logger.InfoWith("Calling gateway for payment initiation", "gateway", gateway)
	gatewayResponse, err := selectedGateway.InitiatePayment(ctx, gatewayRequest)

	if err != nil {
		o.logger.ErrorWith("Gateway payment initiation failed", "gateway", gateway, "error", err)
		// Return error - service layer will handle transaction rollback
		return nil, fmt.Errorf("gateway payment initiation failed: %w", err)
	}

	o.logger.InfoWith("Gateway call successful",
		"gateway", gateway,
		"gateway_payment_id", gatewayResponse.GatewayPaymentID)

	// Create final response with all information service layer needs
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

	// NOTE: Service layer will:
	// 1. Update payment record with gateway response (gateway_payment_id, redirect_url, status)
	// 2. Update payment with fee details
	// 3. Finalize vouchers if voucher_ids provided
	// All within a single database transaction

	o.logger.InfoWith("Payment processing initiated",
		"payment_id", paymentID,
		"gateway", gateway,
		"redirect_url", response.RedirectURL)
	o.logger.DebugWith("Orchestrator: ProcessPayment completed successfully")

	return response, nil
}

// TokenizeCard tokenizes a card with detailed logging
func (o *paymentOrchestratorImpl) TokenizeCard(ctx context.Context, request TokenizeRequest) (*TokenizeResponse, error) {
	o.logger.InfoWith("Orchestrator: TokenizeCard called",
		"business_id", request.BusinessID,
		"card_type", request.CardDetails.CardType,
		"card_category", request.CardDetails.CardCategory)
	o.logger.DebugWith("Card details", "last_four_digits", request.CardDetails.LastFourDigits)

	// Select gateway for tokenization (same logic as payment)
	mockPaymentRequest := PaymentRequest{
		PaymentMethod: PaymentMethodCard,
		CardDetails:   &request.CardDetails,
	}
	gateway, err := o.SelectGateway(mockPaymentRequest)
	if err != nil {
		o.logger.ErrorWith("Gateway selection for tokenization failed", "error", err)
		return nil, err
	}
	o.logger.InfoWith("Gateway selected for tokenization", "gateway", gateway)

	// Use transaction ID from service layer (payment UUID for ₹1 tokenization payment)
	if request.TransactionID == "" {
		o.logger.ErrorWith("Transaction ID missing from tokenization request")
		return nil, fmt.Errorf("transaction_id is required (should be set by service layer as payment UUID)")
	}
	o.logger.DebugWith("Using transaction ID from service", "transaction_id", request.TransactionID)

	// Get selected gateway instance
	selectedGateway := o.getGatewayInstance(gateway)
	if selectedGateway == nil {
		return nil, fmt.Errorf("gateway instance not found for tokenization: %s", gateway)
	}

	// Prepare gateway tokenization request
	gatewayRequest := GatewayTokenizeRequest{
		BusinessID:    request.BusinessID,
		TransactionID: request.TransactionID, // Payment UUID from service layer
		CardDetails:   request.CardDetails,
		CallbackURL:   "https://mock-callback.com/tokenize", // TODO: Make configurable
		Metadata: map[string]any{
			"tokenization_request": true,
			"business_id":          request.BusinessID,
		},
	}

	// For Easebuzz, amount is implicit (₹1 built into tokenization API)
	if gateway == GatewayEasebuzz {
		o.logger.DebugWith("Easebuzz tokenization", "note", "₹1 amount implicit in API")
	}

	// Call actual gateway for tokenization
	o.logger.InfoWith("Calling gateway for card tokenization", "gateway", gateway)
	gatewayResponse, err := selectedGateway.TokenizeCard(ctx, gatewayRequest)
	if err != nil {
		o.logger.ErrorWith("Gateway tokenization failed", "gateway", gateway, "error", err)
		return nil, fmt.Errorf("gateway tokenization failed: %w", err)
	}
	o.logger.InfoWith("Gateway tokenization successful", "gateway", gateway)

	// NOTE: Service layer will persist the tokenized card to database
	// within a transaction. Including card details in response for service to use.
	o.logger.DebugWith("Card tokenization successful - awaiting service layer persistence")

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

	o.logger.InfoWith("Card tokenization completed",
		"gateway", gateway,
		"card_id", response.CardID)
	o.logger.DebugWith("Orchestrator: TokenizeCard completed successfully")

	return response, nil
}

// ProcessTokenizedPayment handles payments using tokenized cards
func (o *paymentOrchestratorImpl) ProcessTokenizedPayment(ctx context.Context, request PaymentRequest) (*PaymentResponse, error) {
	o.logger.InfoWith("Orchestrator: ProcessTokenizedPayment called")

	// Validate token card details
	if request.TokenCardDetails == nil {
		o.logger.ErrorWith("Token card details are nil")
		return nil, fmt.Errorf("token_card_details are required for token card payments")
	}

	if request.TokenCardDetails.CardID == "" {
		o.logger.ErrorWith("Card ID is empty in token card details")
		return nil, fmt.Errorf("card_id is required for token card payments")
	}

	o.logger.DebugWith("Token card payment details", "card_id", request.TokenCardDetails.CardID)

	// Use payment ID from service layer (already created in DB)
	paymentID := request.PaymentID
	if paymentID == "" {
		o.logger.ErrorWith("Payment ID missing from request")
		return nil, fmt.Errorf("payment_id is required (should be set by service layer)")
	}
	o.logger.DebugWith("Using payment ID from service", "payment_id", paymentID)

	// Step 1: Retrieve stored card by card_id using wallet service
	o.logger.DebugWith("Looking up stored card by ID", "card_id", request.TokenCardDetails.CardID)
	// Get business wallet to find the stored card
	// This is a simplified approach - in production, you might have a direct method
	storedCard, err := o.getStoredCardByID(request.BusinessID, request.TokenCardDetails.CardID)

	if err != nil {
		o.logger.ErrorWith("Failed to find stored card", "card_id", request.TokenCardDetails.CardID, "error", err)
		return nil, fmt.Errorf("invalid card_id or card not found: %w", err)
	}

	// NOTE: Service layer will link payment to payment source within transaction

	if !storedCard.IsActive {
		o.logger.ErrorWith("Card is inactive", "card_id", storedCard.CardID)
		return nil, fmt.Errorf("card is inactive")
	}

	o.logger.InfoWith("Found stored card",
		"card_category", storedCard.CardCategory,
		"card_type", storedCard.CardType,
		"last_four_digits", storedCard.LastFourDigits,
		"gateway_used", storedCard.GatewayUsed)
	o.logger.DebugWith("Card token available", "token_prefix", storedCard.Token[:10])

	// Step 2: Get the specific gateway instance
	// If mock PG is enabled, bypass the normal gateway and use mock PG instead
	var selectedGatewayType GatewayType
	if o.mockPGEnabled {
		selectedGatewayType = GatewayMockPG
		o.logger.InfoWith("Mock PG enabled: bypassing tokenized card gateway routing",
			"original_gateway", storedCard.GatewayUsed,
			"override_gateway", GatewayMockPG)
	} else {
		selectedGatewayType = storedCard.GatewayUsed
	}

	selectedGateway := o.getGatewayInstance(selectedGatewayType)
	if selectedGateway == nil {
		return nil, fmt.Errorf("gateway instance not found: %s", selectedGatewayType)
	}

	// Step 3: Generate fresh cryptogram for this payment
	o.logger.DebugWith("Generating cryptogram from gateway", "gateway", storedCard.GatewayUsed)
	cryptogramRequest := CryptogramRequest{
		Token:         storedCard.Token,
		Amount:        request.Amount,
		Currency:      request.Currency,
		TransactionID: paymentID, // Use payment UUID as transaction ID
		BusinessID:    request.BusinessID,
		Metadata: map[string]any{
			"payment_type": "tokenized",
			"card_id":      storedCard.CardID,
		},
	}

	cryptogramResponse, err := selectedGateway.GenerateCryptogram(ctx, cryptogramRequest)
	if err != nil {
		o.logger.ErrorWith("Cryptogram generation failed", "gateway", storedCard.GatewayUsed, "error", err)
		return nil, fmt.Errorf("failed to generate cryptogram: %w", err)
	}

	o.logger.InfoWith("Cryptogram generated successfully",
		"cryptogram_prefix", cryptogramResponse.Cryptogram[:10],
		"expires_at", cryptogramResponse.ExpiresAt.Format("15:04:05"))

	// Step 4: Calculate fees (same as regular payments)
	feeAmount := o.calculateMockFee(request.Amount, PaymentMethodCard) // Use card method for fee calc
	o.logger.DebugWith("Fee calculated for tokenized payment", "fee_amount", feeAmount, "currency", request.Currency)

	// Step 5: Prepare gateway payment request with token + cryptogram
	// Note: CallbackURL comes from request (set by payment service based on calling device)
	// We don't fetch from DB because payment is in uncommitted transaction
	gatewayRequest := GatewayPaymentRequest{
		BusinessID:    request.BusinessID,
		TransactionID: cryptogramRequest.TransactionID,
		Amount:        request.Amount + feeAmount,
		Currency:      request.Currency,
		PaymentMethod: PaymentMethodCard, // Gateway sees this as card payment
		Description:   request.Description,
		CallbackURL:   request.CallbackURL, // From request, not DB
		Token:         storedCard.Token,
		Cryptogram:    cryptogramResponse.Cryptogram,
		Metadata: map[string]any{
			"payment_type":      "tokenized",
			"card_id":           storedCard.CardID,
			"original_metadata": request.Metadata,
		},
	}

	// Step 7: Process payment with specific gateway
	o.logger.InfoWith("Processing tokenized payment with gateway", "gateway", selectedGatewayType)
	gatewayResponse, err := selectedGateway.InitiatePayment(ctx, gatewayRequest)

	if err != nil {
		o.logger.ErrorWith("Tokenized gateway payment failed", "gateway", selectedGatewayType, "error", err)
		// Return error - service layer will handle transaction rollback
		return nil, fmt.Errorf("gateway payment initiation failed: %w", err)
	}

	o.logger.InfoWith("Tokenized payment initiated successfully", "gateway", selectedGatewayType)
	// NOTE: Service layer will update card last_used_at timestamp within transaction

	// Step 7: Create response metadata
	metadata := map[string]any{
		"payment_type":      "tokenized",
		"card_id":           storedCard.CardID,
		"card_last_four":    storedCard.LastFourDigits,
		"gateway_used":      string(selectedGatewayType),
		"original_metadata": request.Metadata,
	}

	// Only add mock-related fields when mock PG is enabled
	if o.mockPGEnabled {
		metadata["original_gateway"] = string(storedCard.GatewayUsed) // Show which gateway the card was originally tokenized with
		metadata["mockpg_override"] = true                             // Indicate this payment used mock PG instead of real gateway
	}

	response := &PaymentResponse{
		PaymentID:        paymentID,
		Status:           gatewayResponse.Status,
		Amount:           request.Amount,
		FeeAmount:        feeAmount,
		TotalAmount:      request.Amount + feeAmount,
		Currency:         request.Currency,
		PaymentMethod:    PaymentMethodTokenCard, // Return the original method
		GatewayUsed:      selectedGatewayType,
		GatewayPaymentID: gatewayResponse.GatewayPaymentID,
		RedirectURL:      gatewayResponse.RedirectURL,
		Message:          fmt.Sprintf("Tokenized payment initiated with %s", selectedGatewayType),
		CreatedAt:        gatewayResponse.CreatedAt,
		Metadata:         metadata,
	}

	o.logger.InfoWith("Tokenized payment response prepared",
		"payment_id", paymentID,
		"card_type", storedCard.CardType,
		"last_four_digits", storedCard.LastFourDigits,
		"redirect_url", response.RedirectURL)
	o.logger.DebugWith("Orchestrator: ProcessTokenizedPayment completed successfully")

	return response, nil
}

// getStoredCardByID retrieves a stored card by its ID from the database
func (o *paymentOrchestratorImpl) getStoredCardByID(businessID, cardID string) (*StoredCard, error) {
	o.logger.DebugWith("Searching for stored card in database",
		"business_id", businessID,
		"card_id", cardID)

	// Query stored card from database
	ctx := context.Background()
	dbCard, err := o.storedCardRepo.GetStoredCardByID(ctx, cardID)

	if err != nil {
		o.logger.ErrorWith("Failed to retrieve stored card from database",
			"card_id", cardID,
			"error", err)
		return nil, fmt.Errorf("failed to retrieve card: %w", err)
	}

	if dbCard == nil {
		o.logger.ErrorWith("Stored card not found in database", "card_id", cardID)
		return nil, fmt.Errorf("card not found: %s", cardID)
	}

	// Security check: verify card belongs to the requesting business
	if dbCard.BusinessID != businessID {
		o.logger.ErrorWith("Card does not belong to business",
			"card_id", cardID,
			"card_business_id", dbCard.BusinessID,
			"requested_business_id", businessID)
		return nil, fmt.Errorf("card not found: %s", cardID) // Don't expose business mismatch
	}

	// Check if card is active
	if !dbCard.IsActive {
		o.logger.ErrorWith("Stored card is inactive",
			"card_id", cardID,
			"business_id", businessID)
		return nil, fmt.Errorf("card is inactive")
	}

	o.logger.InfoWith("Found stored card in database",
		"card_id", cardID,
		"card_type", dbCard.CardType,
		"last_four_digits", dbCard.LastFourDigits,
		"gateway_used", dbCard.GatewayUsed)

	return dbCard, nil
}

// ValidatePaymentRequest validates a payment request
func (o *paymentOrchestratorImpl) ValidatePaymentRequest(request PaymentRequest) error {
	o.logger.DebugWith("Validating payment request")

	if request.BusinessID == "" {
		o.logger.ErrorWith("Validation failed: business_id is required")
		return fmt.Errorf("business_id is required")
	}
	o.logger.DebugWith("Business ID is valid", "business_id", request.BusinessID)

	if request.Amount <= 0 {
		o.logger.ErrorWith("Validation failed: invalid amount", "amount", request.Amount)
		return fmt.Errorf("amount must be greater than 0")
	}
	o.logger.DebugWith("Amount is valid", "amount", request.Amount)

	if request.Currency == "" {
		o.logger.ErrorWith("Validation failed: currency is required")
		return fmt.Errorf("currency is required")
	}
	o.logger.DebugWith("Currency is valid", "currency", request.Currency)

	if request.PaymentMethod == "" {
		o.logger.ErrorWith("Validation failed: payment_method is required")
		return fmt.Errorf("payment_method is required")
	}
	o.logger.DebugWith("Payment method is valid", "payment_method", request.PaymentMethod)

	// Validate payment method specific details
	switch request.PaymentMethod {
	case PaymentMethodCard:
		if request.CardDetails == nil {
			o.logger.ErrorWith("Validation failed: card_details required for card payment")
			return fmt.Errorf("card_details are required for card payments")
		}
		o.logger.DebugWith("Card details provided")
	case PaymentMethodNetBanking:
		if request.NetBankingDetails == nil {
			o.logger.ErrorWith("Validation failed: net_banking_details required")
			return fmt.Errorf("net_banking_details are required for net banking payments")
		}
		o.logger.DebugWith("Net banking details provided")
	case PaymentMethodUPI:
		if request.UPIDetails == nil {
			o.logger.ErrorWith("Validation failed: upi_details required")
			return fmt.Errorf("upi_details are required for UPI payments")
		}
		o.logger.DebugWith("UPI details provided")
	case PaymentMethodWallet:
		if request.WalletDetails == nil {
			o.logger.ErrorWith("Validation failed: wallet_details required")
			return fmt.Errorf("wallet_details are required for wallet payments")
		}
		o.logger.DebugWith("Wallet details provided")
	}

	o.logger.DebugWith("Payment request validation completed successfully")
	return nil
}

// SelectGateway selects the appropriate gateway based on payment method and card details
func (o *paymentOrchestratorImpl) SelectGateway(request PaymentRequest) (GatewayType, error) {
	o.logger.DebugWith("Selecting gateway", "payment_method", request.PaymentMethod)

	// Check if mock PG is enabled - bypass normal gateway selection
	if o.mockPGEnabled {
		o.logger.InfoWith("Mock PG enabled: bypassing normal gateway selection", "rule", "mockpg_bypass")
		return GatewayMockPG, nil
	}

	switch request.PaymentMethod {
	case PaymentMethodNetBanking:
		o.logger.InfoWith("Gateway selected: NetBanking → JioPay", "rule", "all_netbanking_jiopay")
		return GatewayJioPay, nil

	case PaymentMethodUPI:
		o.logger.InfoWith("Gateway selected: UPI → JioPay", "rule", "all_upi_jiopay")
		return GatewayJioPay, nil

	case PaymentMethodWallet:
		o.logger.InfoWith("Gateway selected: Wallet → JioPay", "rule", "all_wallet_jiopay")
		return GatewayJioPay, nil

	case PaymentMethodCard:
		return o.selectGatewayForCard(request.CardDetails, request.Amount)

	default:
		o.logger.ErrorWith("Unsupported payment method", "payment_method", request.PaymentMethod)
		return "", fmt.Errorf("unsupported payment method: %s", request.PaymentMethod)
	}
}

// selectGatewayForCard implements the exact card routing business rules
func (o *paymentOrchestratorImpl) selectGatewayForCard(card *Card, amount int64) (GatewayType, error) {
	if card == nil {
		o.logger.ErrorWith("Card details missing for card payment")
		return "", fmt.Errorf("card details are required for card payments")
	}

	o.logger.DebugWith("Card gateway selection",
		"card_category", card.CardCategory,
		"card_type", card.CardType,
		"amount_inr", float64(amount)/100)

	// Corporate Credit Cards - All → JioPay
	if card.CardCategory == CardCategoryCorporate {
		o.logger.InfoWith("Gateway selected: Corporate Card → JioPay",
			"card_type", card.CardType,
			"rule", "corporate_jiopay")
		return GatewayJioPay, nil
	}

	// Credit Cards
	if card.CardCategory == CardCategoryCredit {
		o.logger.DebugWith("Credit card detected", "card_type", card.CardType)
		switch card.CardType {
		case CardTypeVisa, CardTypeMastercard:
			o.logger.InfoWith("Gateway selected: Credit Card → JioPay",
				"card_type", card.CardType,
				"rule", "visa_mc_credit_jiopay")
			return GatewayJioPay, nil
		case CardTypeRupay:
			o.logger.InfoWith("Gateway selected: RuPay Credit → JioPay",
				"rule", "rupay_credit_jiopay")
			return GatewayJioPay, nil
		case CardTypeDiners, CardTypeAmex:
			o.logger.InfoWith("Gateway selected: Amex/Diners Credit → Easebuzz",
				"card_type", card.CardType,
				"rule", "amex_diners_credit_easebuzz")
			return GatewayEasebuzz, nil
		default:
			o.logger.ErrorWith("Unsupported credit card type", "card_type", card.CardType)
			return "", fmt.Errorf("unsupported credit card type: %s", card.CardType)
		}
	}

	// Debit Cards
	if card.CardCategory == CardCategoryDebit {
		o.logger.DebugWith("Debit card detected", "card_type", card.CardType)
		switch card.CardType {
		case CardTypeVisa, CardTypeMastercard:
			threshold := int64(200000) // ₹2000 in paise
			if amount < threshold {
				o.logger.InfoWith("Gateway selected: Debit < ₹2000 → Easebuzz",
					"card_type", card.CardType,
					"amount_inr", float64(amount)/100,
					"rule", "debit_low_amount_easebuzz")
				return GatewayEasebuzz, nil
			} else {
				o.logger.InfoWith("Gateway selected: Debit >= ₹2000 → JioPay",
					"card_type", card.CardType,
					"amount_inr", float64(amount)/100,
					"rule", "debit_high_amount_jiopay")
				return GatewayJioPay, nil
			}
		case CardTypeRupay:
			o.logger.InfoWith("Gateway selected: RuPay Debit → JioPay",
				"rule", "rupay_debit_jiopay")
			return GatewayJioPay, nil
		case CardTypeDiners, CardTypeAmex:
			// Note: Debit cards for Amex/Diners are rare, but handling for completeness
			o.logger.InfoWith("Gateway selected: Amex/Diners Debit → Easebuzz",
				"card_type", card.CardType,
				"rule", "amex_diners_debit_easebuzz")
			return GatewayEasebuzz, nil
		default:
			o.logger.ErrorWith("Unsupported debit card type", "card_type", card.CardType)
			return "", fmt.Errorf("unsupported debit card type: %s", card.CardType)
		}
	}

	// Prepaid cards (if any) - fallback to JioPay
	if card.CardCategory == CardCategoryCredit { // Assuming prepaid would be similar category
		o.logger.InfoWith("Gateway selected: Prepaid Card → JioPay (fallback)",
			"rule", "prepaid_jiopay_fallback")
		return GatewayJioPay, nil
	}

	o.logger.ErrorWith("Unsupported card combination",
		"card_category", card.CardCategory,
		"card_type", card.CardType)
	return "", fmt.Errorf("unsupported card combination: %s %s", card.CardCategory, card.CardType)
}

// calculateMockFee calculates a mock fee based on payment method
func (o *paymentOrchestratorImpl) calculateMockFee(amount int64, method PaymentMethod) int64 {
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
func (o *paymentOrchestratorImpl) getGatewayInstance(gatewayType GatewayType) PaymentGateway {
	switch gatewayType {
	case GatewayJioPay:
		return o.jioPayGateway
	case GatewayEasebuzz:
		return o.easebuzzGateway
	case GatewayMockPG:
		return o.mockPGGateway
	default:
		return nil
	}
}

