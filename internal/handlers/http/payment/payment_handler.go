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
		UserID: req.UserID,
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
	fmt.Println("=== MAKE_PAYMENT API CALLED ===")

	// Print request body
	var body map[string]any
	if err := c.Bind(&body); err != nil {
		fmt.Printf("Error binding request body: %v\n", err)
	} else {
		fmt.Printf("Request Body: %+v\n", body)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"message": "make_payment endpoint called",
		"endpoint": "POST /v1/process_payment/make_payment",
		"status": "success",
		"received_data": body,
	})
}

// RegisterPaymentRoutes registers all payment routes
func RegisterPaymentRoutes(group *echo.Group, orchestrator payment.PaymentOrchestrator, feeCalculator business_rules.FeeCalculator, walletService business_rules.WalletService) {
	handler := NewPaymentHandler(orchestrator, feeCalculator, walletService)

	group.POST("/fetch_wallet", handler.FetchWallet)
	group.POST("/get_fee_rules", handler.GetFeeRules)
	group.POST("/tokenize", handler.Tokenize)
	group.POST("/make_payment", handler.MakePayment)
}