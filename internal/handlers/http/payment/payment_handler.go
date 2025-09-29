package payment

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/business_rules"
	"xcentro.in/internal/core/payment"
)

// PaymentHandler handles payment-related HTTP requests
type PaymentHandler struct {
	orchestrator    payment.PaymentOrchestrator
	feeCalculator   business_rules.FeeCalculator
	walletService   business_rules.WalletService
}

// NewPaymentHandler creates a new payment handler
func NewPaymentHandler(orchestrator payment.PaymentOrchestrator, feeCalculator business_rules.FeeCalculator, walletService business_rules.WalletService) *PaymentHandler {
	return &PaymentHandler{
		orchestrator:  orchestrator,
		feeCalculator: feeCalculator,
		walletService: walletService,
	}
}

// FetchWallet handles POST /process_payment/fetch_wallet
func (h *PaymentHandler) FetchWallet(c echo.Context) error {
	fmt.Println("=== HANDLER: FetchWallet API Called ===")

	// Parse request body
	var req FetchWalletRequest
	if err := c.Bind(&req); err != nil {
		fmt.Printf("❌ Error binding request: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error": "invalid_request",
			"message": "Invalid request body",
		})
	}

	fmt.Printf("📥 Request: Business ID: %s\n", req.BusinessID)

	// Call wallet service to get business wallet information
	fmt.Println("🔄 Calling Wallet Service...")
	businessWallet, err := h.walletService.GetBusinessWallet(req.BusinessID)
	if err != nil {
		fmt.Printf("❌ Wallet service error: %v\n", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "wallet_service_error",
			"message": "Failed to retrieve business wallet information",
		})
	}

	// Convert to response format
	response := &FetchWalletResponse{
		BusinessID:   businessWallet.BusinessID,
		StoredCards:  convertStoredCardsToResponse(businessWallet.StoredCards),
		BankAccounts: convertBankAccountsToResponse(businessWallet.BankAccounts),
		UPIDetails:   convertUPIDetailsToResponse(businessWallet.UPIDetails),
		CreditLines:  convertCreditLinesToResponse(businessWallet.CreditLines),
		LastUpdated:  businessWallet.LastUpdated.Format("2006-01-02T15:04:05Z"),
	}

	fmt.Printf("✅ Business wallet fetched successfully: %s\n", businessWallet.BusinessID)
	fmt.Printf("📊 Summary: %d cards, %d bank accounts, %d UPI details, %d credit lines\n",
		len(businessWallet.StoredCards), len(businessWallet.BankAccounts),
		len(businessWallet.UPIDetails), len(businessWallet.CreditLines))
	fmt.Println("=== HANDLER: FetchWallet Complete ===")

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Business wallet retrieved successfully",
		"endpoint": "POST /v1/process_payment/fetch_wallet",
		"status": "success",
		"data": response,
	})
}

// Helper functions to convert domain entities to response models

func convertStoredCardsToResponse(cards []*payment.StoredCard) []StoredCardResponse {
	var response []StoredCardResponse
	for _, card := range cards {
		nickname := ""
		if card.Metadata != nil {
			if n, ok := card.Metadata["nickname"].(string); ok {
				nickname = n
			}
		}

		response = append(response, StoredCardResponse{
			CardID:         card.CardID,
			LastFourDigits: card.LastFourDigits,
			CardType:       string(card.CardType),
			CardCategory:   string(card.CardCategory),
			ExpiryMonth:    card.ExpiryMonth,
			ExpiryYear:     card.ExpiryYear,
			IssuerBank:     card.IssuerBank,
			IsActive:       card.IsActive,
			Nickname:       nickname,
		})
	}
	return response
}

func convertBankAccountsToResponse(accounts []*payment.BankAccount) []BankAccountResponse {
	var response []BankAccountResponse
	for _, account := range accounts {
		nickname := ""
		if account.Metadata != nil {
			if n, ok := account.Metadata["nickname"].(string); ok {
				nickname = n
			}
		}

		response = append(response, BankAccountResponse{
			AccountID:      account.AccountID,
			LastFiveDigits: account.LastFiveDigits,
			BankName:       account.BankName,
			IFSCCode:       account.IFSCCode,
			AccountType:    string(account.AccountType),
			IsActive:       account.IsActive,
			IsVerified:     account.IsVerified,
			Nickname:       nickname,
		})
	}
	return response
}

func convertUPIDetailsToResponse(upis []*payment.UPIDetail) []UPIDetailResponse {
	var response []UPIDetailResponse
	for _, upi := range upis {
		nickname := ""
		if upi.Metadata != nil {
			if n, ok := upi.Metadata["nickname"].(string); ok {
				nickname = n
			}
		}

		response = append(response, UPIDetailResponse{
			UPIID:      upi.UPIID,
			UPIAddress: upi.UPIAddress,
			Provider:   string(upi.Provider),
			IsActive:   upi.IsActive,
			IsVerified: upi.IsVerified,
			Nickname:   nickname,
		})
	}
	return response
}

func convertCreditLinesToResponse(credits []*payment.CreditLine) []CreditLineResponse {
	var response []CreditLineResponse
	for _, credit := range credits {
		nickname := ""
		if credit.Metadata != nil {
			if n, ok := credit.Metadata["nickname"].(string); ok {
				nickname = n
			}
		}

		response = append(response, CreditLineResponse{
			CreditLineID:   credit.CreditLineID,
			Provider:       credit.Provider,
			AvailableLimit: float64(credit.AvailableLimit) / 100, // Convert to rupees
			TotalLimit:     float64(credit.CreditLimit) / 100,
			Currency:       string(credit.Currency),
			Status:         string(credit.Status),
			IsActive:       credit.IsActive,
			Nickname:       nickname,
		})
	}
	return response
}

// GetFeeRules handles POST /process_payment/get_fee_rules
func (h *PaymentHandler) GetFeeRules(c echo.Context) error {
	fmt.Println("=== HANDLER: GetFeeRules API Called ===")

	// Parse request body
	var req GetFeeRulesRequest
	if err := c.Bind(&req); err != nil {
		fmt.Printf("❌ Error binding request: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error": "invalid_request",
			"message": "Invalid request body",
		})
	}

	fmt.Printf("📥 Request: Payment Method: %s", req.PaymentMethod)
	if req.CardType != "" {
		fmt.Printf(", Card: %s %s", req.CardCategory, req.CardType)
	}
	if req.Amount > 0 {
		fmt.Printf(", Amount: %d", req.Amount)
	}
	fmt.Println()

	// Call fee calculator to get fee rules
	fmt.Println("🔄 Calling Fee Calculator...")

	var rules []*business_rules.FeeRule
	var err error

	if req.PaymentMethod != "" {
		rules, err = h.feeCalculator.GetFeeRules(payment.PaymentMethod(req.PaymentMethod))
	} else {
		rules, err = h.feeCalculator.GetAllFeeRules()
	}

	if err != nil {
		fmt.Printf("❌ Fee calculator error: %v\n", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "fee_calculator_error",
			"message": "Failed to retrieve fee rules",
		})
	}

	// If amount is provided, calculate the actual fee
	var feeCalculation *business_rules.FeeCalculation
	if req.Amount > 0 && req.PaymentMethod != "" {
		fmt.Println("💰 Calculating fee for provided amount...")
		feeCalculation, err = h.feeCalculator.CalculateFee(
			req.Amount,
			payment.PaymentMethod(req.PaymentMethod),
			payment.CardType(req.CardType),
			payment.CardCategory(req.CardCategory),
		)
		if err != nil {
			fmt.Printf("❌ Fee calculation error: %v\n", err)
		}
	}

	// Convert rules to response format
	var responseRules []GetFeeRulesResponse
	for _, rule := range rules {
		responseRule := GetFeeRulesResponse{
			PaymentMethod: string(rule.PaymentMethod),
			FeePercentage: rule.PercentageFee,
			FixedFee:      float64(rule.FixedFee) / 100, // Convert to rupees
			MinFee:        float64(rule.MinFee) / 100,
			MaxFee:        float64(rule.MaxFee) / 100,
			Currency:      string(rule.Currency),
		}
		responseRules = append(responseRules, responseRule)
	}

	response := map[string]any{
		"message": "Fee rules retrieved successfully",
		"endpoint": "POST /v1/process_payment/get_fee_rules",
		"status": "success",
		"data": map[string]any{
			"rules": responseRules,
		},
	}

	// Add fee calculation if available
	if feeCalculation != nil {
		response["data"].(map[string]any)["calculation"] = map[string]any{
			"base_amount":  feeCalculation.BaseAmount,
			"fee_amount":   feeCalculation.FeeAmount,
			"total_amount": feeCalculation.TotalAmount,
			"currency":     string(feeCalculation.Currency),
			"breakdown":    feeCalculation.Breakdown,
		}
		fmt.Printf("💰 Fee calculated: %d + %d = %d %s\n",
			feeCalculation.BaseAmount, feeCalculation.FeeAmount,
			feeCalculation.TotalAmount, feeCalculation.Currency)
	}

	fmt.Printf("✅ Retrieved %d fee rules\n", len(rules))
	fmt.Println("=== HANDLER: GetFeeRules Complete ===")

	return c.JSON(http.StatusOK, response)
}

// Tokenize handles POST /process_payment/tokenize
func (h *PaymentHandler) Tokenize(c echo.Context) error {
	fmt.Println("=== HANDLER: Tokenize API Called ===")

	// Parse request body
	var req TokenizeRequest
	if err := c.Bind(&req); err != nil {
		fmt.Printf("❌ Error binding request: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error": "invalid_request",
			"message": "Invalid request body",
		})
	}

	fmt.Printf("📥 Request: User ID: %s, Card: %s %s\n", req.UserID, req.CardDetails.CardCategory, req.CardDetails.CardType)

	// Convert HTTP request to core domain request
	coreRequest := payment.TokenizeRequest{
		BusinessID: req.UserID, // TODO: This should be BusinessID, not UserID
		CardDetails: payment.Card{
			Number:         req.CardDetails.CardNumber,
			ExpiryMonth:    req.CardDetails.ExpiryMonth,
			ExpiryYear:     req.CardDetails.ExpiryYear,
			CVV:            req.CardDetails.CVV,
			HolderName:     req.CardDetails.HolderName,
			CardType:       payment.CardType(req.CardDetails.CardType),
			CardCategory:   payment.CardCategory(req.CardDetails.CardCategory),
			LastFourDigits: req.CardDetails.LastFourDigits,
		},
	}

	// Call orchestrator to tokenize card
	fmt.Println("🔄 Calling Payment Orchestrator for tokenization...")
	tokenizeResponse, err := h.orchestrator.TokenizeCard(c.Request().Context(), coreRequest)
	if err != nil {
		fmt.Printf("❌ Orchestrator error: %v\n", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "tokenization_failed",
			"message": "Failed to tokenize card",
		})
	}

	// Convert core response to HTTP response
	response := &TokenizeResponse{
		Token:          tokenizeResponse.Token,
		CardID:         tokenizeResponse.CardID,
		LastFourDigits: tokenizeResponse.LastFourDigits,
		CardType:       string(tokenizeResponse.CardType),
		CardCategory:   string(tokenizeResponse.CardCategory),
		ExpiryMonth:    tokenizeResponse.ExpiryMonth,
		ExpiryYear:     tokenizeResponse.ExpiryYear,
		Status:         tokenizeResponse.Status,
		Message:        tokenizeResponse.Message,
	}

	fmt.Printf("✅ Card tokenized successfully: %s\n", tokenizeResponse.Token)
	fmt.Println("=== HANDLER: Tokenize Complete ===")

	return c.JSON(http.StatusCreated, map[string]any{
		"message": "Card tokenized successfully",
		"endpoint": "POST /v1/process_payment/tokenize",
		"status": "success",
		"data": response,
	})
}

// MakePayment handles POST /process_payment/make_payment
func (h *PaymentHandler) MakePayment(c echo.Context) error {
	fmt.Println("=== HANDLER: MakePayment API Called ===")

	// Parse request body
	var req MakePaymentRequest
	if err := c.Bind(&req); err != nil {
		fmt.Printf("❌ Error binding request: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error": "invalid_request",
			"message": "Invalid request body",
		})
	}

	fmt.Printf("📥 Request: Amount: %d, Currency: %s, Payment Method: %s\n", req.Amount, req.Currency, req.PaymentMethod)

	// Validate required fields
	if err := h.validatePaymentRequest(&req); err != nil {
		fmt.Printf("❌ Validation error: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error": "validation_failed",
			"message": err.Error(),
		})
	}

	// Calculate fees using business rules
	// TODO: don't think we really need to do this here, as the orchestrator should handle it
	// Jiopay doens't need fee calc
	// easebuzz might need as there is no reseller model
	// no idea how their reseller model is going to work
	fmt.Println("💰 Calculating payment fees...")
	feeCalculation, err := h.feeCalculator.CalculateFee(
		req.Amount,
		payment.PaymentMethod(req.PaymentMethod),
		payment.CardType(getCardTypeFromRequest(&req)),
		payment.CardCategory(getCardCategoryFromRequest(&req)),
	)
	if err != nil {
		fmt.Printf("❌ Fee calculation error: %v\n", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "fee_calculation_failed",
			"message": "Failed to calculate payment fees",
		})
	}

	fmt.Printf("💰 Fee calculated: Base: %d, Fee: %d, Total: %d %s\n",
		feeCalculation.BaseAmount, feeCalculation.FeeAmount,
		feeCalculation.TotalAmount, feeCalculation.Currency)

	// Convert HTTP request to core domain request
	coreRequest := payment.PaymentRequest{
		BusinessID:    "business-123", // TODO: Extract from request context or JWT token
		Amount:        req.Amount,
		Currency:      payment.Currency(req.Currency),
		PaymentMethod: payment.PaymentMethod(req.PaymentMethod),
		Description:   req.Description,
		CallbackURL:   req.CallbackURL,
		Metadata:      req.Metadata,
	}

	// Add payment method specific details
	h.populatePaymentMethodDetails(&coreRequest, &req)

	// Call orchestrator to process payment
	fmt.Println("🔄 Calling Payment Orchestrator...")
	paymentResponse, err := h.orchestrator.ProcessPayment(c.Request().Context(), coreRequest)
	if err != nil {
		fmt.Printf("❌ Orchestrator error: %v\n", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "payment_processing_failed",
			"message": "Failed to process payment",
		})
	}

	// Convert core response to HTTP response
	response := &MakePaymentResponse{
		PaymentID:        paymentResponse.PaymentID,
		Status:           string(paymentResponse.Status),
		Amount:           paymentResponse.Amount,
		Currency:         string(paymentResponse.Currency),
		PaymentMethod:    string(paymentResponse.PaymentMethod),
		GatewayUsed:      string(paymentResponse.GatewayUsed),
		GatewayPaymentID: paymentResponse.GatewayPaymentID,
		RedirectURL:      paymentResponse.RedirectURL,
		Message:          paymentResponse.Message,
		CreatedAt:        paymentResponse.CreatedAt.Format("2006-01-02T15:04:05Z"),
		Metadata:         paymentResponse.Metadata,
	}

	fmt.Printf("✅ Payment initiated successfully: %s\n", paymentResponse.PaymentID)
	fmt.Printf("🌐 Redirect URL: %s\n", paymentResponse.RedirectURL)
	fmt.Printf("🏦 Gateway Used: %s\n", paymentResponse.GatewayUsed)
	fmt.Println("=== HANDLER: MakePayment Complete ===")

	return c.JSON(http.StatusCreated, map[string]any{
		"message": "Payment initiated successfully",
		"endpoint": "POST /v1/process_payment/make_payment",
		"status": "success",
		"data": response,
	})
}

// Helper functions for MakePayment handler

// validatePaymentRequest validates the payment request
func (h *PaymentHandler) validatePaymentRequest(req *MakePaymentRequest) error {
	// Validate amount
	if req.Amount <= 0 {
		return fmt.Errorf("amount must be greater than 0")
	}

	// Validate currency
	if req.Currency != "INR" && req.Currency != "USD" {
		return fmt.Errorf("currency must be INR or USD")
	}

	// Validate payment method
	validMethods := map[string]bool{
		"card": true, "netbanking": true, "upi": true, "wallet": true,
	}
	if !validMethods[req.PaymentMethod] {
		return fmt.Errorf("invalid payment method: %s", req.PaymentMethod)
	}

	// Validate callback URL
	if req.CallbackURL == "" {
		return fmt.Errorf("callback_url is required")
	}

	// Payment method specific validation
	switch req.PaymentMethod {
	case "card":
		if req.CardDetails == nil {
			return fmt.Errorf("card_details are required for card payments")
		}
		if err := h.validateCardDetails(req.CardDetails); err != nil {
			return fmt.Errorf("card validation failed: %v", err)
		}
	case "netbanking":
		if req.NetBanking == nil {
			return fmt.Errorf("net_banking details are required for netbanking payments")
		}
		if req.NetBanking.BankCode == "" {
			return fmt.Errorf("bank_code is required for netbanking")
		}
	case "upi":
		if req.UPI == nil {
			return fmt.Errorf("upi details are required for UPI payments")
		}
		if req.UPI.VPA == "" {
			return fmt.Errorf("vpa is required for UPI payments")
		}
	case "wallet":
		if req.Wallet == nil {
			return fmt.Errorf("wallet details are required for wallet payments")
		}
		if req.Wallet.WalletType == "" {
			return fmt.Errorf("wallet_type is required for wallet payments")
		}
	}

	return nil
}

// validateCardDetails validates card-specific details
func (h *PaymentHandler) validateCardDetails(card *CardDetails) error {
	if card.CardNumber == "" {
		return fmt.Errorf("card_number is required")
	}
	if card.ExpiryMonth == "" {
		return fmt.Errorf("expiry_month is required")
	}
	if card.ExpiryYear == "" {
		return fmt.Errorf("expiry_year is required")
	}
	if card.CVV == "" {
		return fmt.Errorf("cvv is required")
	}
	if card.HolderName == "" {
		return fmt.Errorf("holder_name is required")
	}

	// Validate card type and category
	validCardTypes := map[string]bool{
		"visa": true, "mastercard": true, "rupay": true, "amex": true, "diners": true,
	}
	if !validCardTypes[card.CardType] {
		return fmt.Errorf("invalid card_type: %s", card.CardType)
	}

	validCardCategories := map[string]bool{
		"credit": true, "debit": true, "corporate": true,
	}
	if !validCardCategories[card.CardCategory] {
		return fmt.Errorf("invalid card_category: %s", card.CardCategory)
	}

	return nil
}

// getCardTypeFromRequest extracts card type from request
func getCardTypeFromRequest(req *MakePaymentRequest) string {
	if req.CardDetails != nil {
		return req.CardDetails.CardType
	}
	return ""
}

// getCardCategoryFromRequest extracts card category from request
func getCardCategoryFromRequest(req *MakePaymentRequest) string {
	if req.CardDetails != nil {
		return req.CardDetails.CardCategory
	}
	return ""
}

// populatePaymentMethodDetails adds payment method specific details to core request
func (h *PaymentHandler) populatePaymentMethodDetails(coreReq *payment.PaymentRequest, httpReq *MakePaymentRequest) {
	switch httpReq.PaymentMethod {
	case "card":
		if httpReq.CardDetails != nil {
			coreReq.CardDetails = &payment.Card{
				Number:         httpReq.CardDetails.CardNumber,
				ExpiryMonth:    httpReq.CardDetails.ExpiryMonth,
				ExpiryYear:     httpReq.CardDetails.ExpiryYear,
				CVV:            httpReq.CardDetails.CVV,
				HolderName:     httpReq.CardDetails.HolderName,
				CardType:       payment.CardType(httpReq.CardDetails.CardType),
				CardCategory:   payment.CardCategory(httpReq.CardDetails.CardCategory),
				LastFourDigits: httpReq.CardDetails.LastFourDigits,
			}
		}
	case "netbanking":
		if httpReq.NetBanking != nil {
			coreReq.NetBankingDetails = &payment.NetBanking{
				BankCode: httpReq.NetBanking.BankCode,
				BankName: httpReq.NetBanking.BankName,
			}
		}
	case "upi":
		if httpReq.UPI != nil {
			coreReq.UPIDetails = &payment.UPI{
				VPA: httpReq.UPI.VPA,
			}
		}
	case "wallet":
		if httpReq.Wallet != nil {
			coreReq.WalletDetails = &payment.Wallet{
				WalletID: httpReq.Wallet.WalletID,
				Currency: payment.Currency(httpReq.Currency),
			}
		}
	}
}

// RegisterPaymentRoutes registers all payment routes
func RegisterPaymentRoutes(group *echo.Group, orchestrator payment.PaymentOrchestrator, feeCalculator business_rules.FeeCalculator, walletService business_rules.WalletService) {
	handler := NewPaymentHandler(orchestrator, feeCalculator, walletService)

	// Core payment operations
	group.POST("/fetch_wallet", handler.FetchWallet)
	group.POST("/get_fee_rules", handler.GetFeeRules)
	group.POST("/tokenize", handler.Tokenize)
	group.POST("/make_payment", handler.MakePayment)

	// Webhook endpoints (Phase 3C)
	webhookGroup := group.Group("/webhooks")
	RegisterWebhookRoutes(webhookGroup, orchestrator)
}