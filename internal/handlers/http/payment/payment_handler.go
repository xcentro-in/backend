package payment

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/business_rules"
	"xcentro.in/internal/core/payment"
	"xcentro.in/internal/middleware"
	"xcentro.in/internal/services/payments"
)

// PaymentHandler handles payment-related HTTP requests
type PaymentHandler struct {
	paymentService       *payments.PaymentService
	paymentSourceService *payments.PaymentSourceService // Phase 10
}

// NewPaymentHandler creates a new payment handler
func NewPaymentHandler(paymentService *payments.PaymentService, paymentSourceService *payments.PaymentSourceService) *PaymentHandler {
	return &PaymentHandler{
		paymentService:       paymentService,
		paymentSourceService: paymentSourceService,
	}
}

// FetchWallet handles POST /wallet/fetch
func (h *PaymentHandler) FetchWallet(c echo.Context) error {
	fmt.Println("=== HANDLER: FetchWallet API Called ===")

	// Extract userId from JWT context (set by JWTMiddleware)
	userID, ok := c.Get("userId").(string)
	if !ok || userID == "" {
		fmt.Println("❌ Missing userId in context")
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error":   "MISSING_USER_ID",
			"message": "User ID not found in token",
		})
	}
	fmt.Printf("🔐 User ID (from JWT): %s\n", userID)

	// Extract business_id from X-Business-ID header
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		fmt.Println("❌ Missing X-Business-ID header")
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}
	fmt.Printf("🔐 Business ID (from header): %s\n", businessID)

	// Call payment service to get business wallet information
	fmt.Println("🔄 Calling Payment Service...")
	businessWallet, err := h.paymentService.GetBusinessWallet(businessID)
	if err != nil {
		fmt.Printf("❌ Wallet service error: %v\n", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "wallet_service_error",
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
		"message":  "Business wallet retrieved successfully",
		"endpoint": "POST /v1/wallet/fetch",
		"status":   "success",
		"data":     response,
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
			"error":   "invalid_request",
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

	// Call payment service to get fee rules
	fmt.Println("🔄 Calling Payment Service...")

	var rules []*business_rules.FeeRule
	var err error

	if req.PaymentMethod != "" {
		rules, err = h.paymentService.GetFeeRules(payment.PaymentMethod(req.PaymentMethod))
	} else {
		rules, err = h.paymentService.GetAllFeeRules()
	}

	if err != nil {
		fmt.Printf("❌ Fee calculator error: %v\n", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "fee_calculator_error",
			"message": "Failed to retrieve fee rules",
		})
	}

	// If amount is provided, calculate the actual fee
	var feeCalculation *business_rules.FeeCalculation
	if req.Amount > 0 && req.PaymentMethod != "" {
		fmt.Println("💰 Calculating fee for provided amount...")
		feeCalculation, err = h.paymentService.CalculateFee(
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
		"message":  "Fee rules retrieved successfully",
		"endpoint": "POST /v1/process_payment/get_fee_rules",
		"status":   "success",
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

	// Extract userId from JWT context (set by JWTMiddleware)
	userID, ok := c.Get("userId").(string)
	if !ok || userID == "" {
		fmt.Println("❌ Missing userId in context")
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error":   "MISSING_USER_ID",
			"message": "User ID not found in token",
		})
	}
	fmt.Printf("🔐 User ID (from JWT): %s\n", userID)

	// Extract business_id from X-Business-ID header
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		fmt.Println("❌ Missing X-Business-ID header")
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}
	fmt.Printf("🔐 Business ID (from header): %s\n", businessID)

	// Parse request body
	var req TokenizeRequest
	if err := c.Bind(&req); err != nil {
		fmt.Printf("❌ Error binding request: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	fmt.Printf("📥 Request: Card: %s %s\n", req.CardDetails.CardCategory, req.CardDetails.CardType)

	// Validate card details
	if err := h.validateTokenizeRequest(&req); err != nil {
		fmt.Printf("❌ Validation error: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "validation_failed",
			"message": err.Error(),
		})
	}

	// Create context with timeout for blocking tokenization
	ctx, cancel := context.WithTimeout(c.Request().Context(), 60*time.Second)
	defer cancel()

	// Convert HTTP request to core domain request
	coreRequest := payment.TokenizeRequest{
		UserID:     userID,
		BusinessID: businessID,
		CardDetails: payment.Card{
			Number:         req.CardDetails.CardNumber,
			ExpiryMonth:    req.CardDetails.ExpiryMonth,
			ExpiryYear:     req.CardDetails.ExpiryYear,
			CVV:            req.CardDetails.CVV,
			HolderName:     req.CardDetails.HolderName,
			CardType:       payment.CardType(req.CardDetails.CardType),
			CardCategory:   payment.CardCategory(req.CardDetails.CardCategory),
			LastFourDigits: extractLastFourDigits(req.CardDetails.CardNumber),
		},
	}

	// Call payment service for complete tokenization (blocking call)
	fmt.Println("🔄 Calling Payment Service for tokenization...")
	fmt.Println("⏳ This may take up to 60 seconds for JioPay 2-step process...")
	tokenizeResponse, err := h.paymentService.TokenizeCard(ctx, coreRequest)

	if err != nil {
		fmt.Printf("❌ Tokenization failed: %v\n", err)
		return h.handleTokenizationError(err)
	}

	// Store tokenized card in business wallet immediately
	fmt.Println("💾 Storing tokenized card in business wallet...")
	storedCard := &payment.StoredCard{
		CardID:         tokenizeResponse.CardID,
		BusinessID:     coreRequest.BusinessID,
		Token:          tokenizeResponse.Token, // Backend only - never exposed to client
		LastFourDigits: tokenizeResponse.LastFourDigits,
		CardType:       tokenizeResponse.CardType,
		CardCategory:   tokenizeResponse.CardCategory,
		ExpiryMonth:    tokenizeResponse.ExpiryMonth,
		ExpiryYear:     tokenizeResponse.ExpiryYear,
		IssuerBank:     "Unknown", // TODO: Add bank detection logic, or can omit as well.
		CardHolderName: coreRequest.CardDetails.HolderName,
		IsActive:       true,
		GatewayUsed:    tokenizeResponse.GatewayUsed,
		AddedAt:        time.Now(),
		Metadata: map[string]any{
			"tokenization_date": time.Now().Format("2006-01-02"),
			"gateway_used":      string(tokenizeResponse.GatewayUsed),
			"tokenization_id":   tokenizeResponse.CardID,
		},
	}

	// Add to business wallet via payment service
	if err := h.paymentService.AddPaymentSource(coreRequest.BusinessID, storedCard); err != nil {
		fmt.Printf("❌ Failed to store card in wallet: %v\n", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "storage_failed",
			"message": "Card was tokenized but failed to save. Please contact support.",
		})
	}

	// Create success response (no token exposure)
	response := map[string]any{
		"status":  "success",
		"message": "Card tokenized and saved successfully",
		"card_info": map[string]any{
			"card_id":          tokenizeResponse.CardID,
			"last_four_digits": tokenizeResponse.LastFourDigits,
			"card_type":        string(tokenizeResponse.CardType),
			"card_category":    string(tokenizeResponse.CardCategory),
			"expiry_month":     tokenizeResponse.ExpiryMonth,
			"expiry_year":      tokenizeResponse.ExpiryYear,
			"gateway_used":     string(tokenizeResponse.GatewayUsed),
		},
	}

	fmt.Printf("✅ Card tokenized and stored successfully: %s\n", tokenizeResponse.CardID)
	fmt.Printf("💳 Card Info: %s %s ending in %s\n",
		tokenizeResponse.CardCategory, tokenizeResponse.CardType, tokenizeResponse.LastFourDigits)
	fmt.Printf("🏦 Gateway Used: %s\n", tokenizeResponse.GatewayUsed)
	fmt.Println("=== HANDLER: Tokenize Complete ===")

	return c.JSON(http.StatusCreated, response)
}

// MakePayment handles POST /process_payment/make_payment
func (h *PaymentHandler) MakePayment(c echo.Context) error {
	fmt.Println("=== HANDLER: MakePayment API Called ===")

	// Extract userId from JWT context (set by JWTMiddleware)
	userID, ok := c.Get("userId").(string)
	if !ok || userID == "" {
		fmt.Println("❌ Missing userId in context")
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error":   "MISSING_USER_ID",
			"message": "User ID not found in token",
		})
	}
	fmt.Printf("🔐 User ID (from JWT): %s\n", userID)

	// Extract business_id from X-Business-ID header
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		fmt.Println("❌ Missing X-Business-ID header")
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}
	fmt.Printf("🔐 Business ID (from header): %s\n", businessID)

	// Parse request body
	var req MakePaymentRequest
	if err := c.Bind(&req); err != nil {
		fmt.Printf("❌ Error binding request: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	fmt.Printf("📥 Request: Amount: %d, Currency: %s, Payment Method: %s\n", req.Amount, req.Currency, req.PaymentMethod)

	// Validate required fields
	if err := h.validatePaymentRequest(&req); err != nil {
		fmt.Printf("❌ Validation error: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "validation_failed",
			"message": err.Error(),
		})
	}

	// Convert HTTP request to core domain request
	coreRequest := payment.PaymentRequest{
		UserID:        userID,
		BusinessID:    businessID,
		Amount:        req.Amount,
		Currency:      payment.Currency(req.Currency),
		PaymentMethod: payment.PaymentMethod(req.PaymentMethod),
		Description:   req.Description,
		CallingDevice: payment.CallingDevice(req.CallingDevice),
		VoucherIDs:    req.VoucherIDs, // Optional: Draft vouchers to finalize after payment success
		Metadata:      req.Metadata,
	}

	// Add payment method specific details
	h.populatePaymentMethodDetails(&coreRequest, &req)

	// Call payment service to process payment (creates record, calculates fees, calls orchestrator)
	fmt.Println("🔄 Calling Payment Service...")
	paymentResponse, err := h.paymentService.ProcessPayment(c.Request().Context(), coreRequest)

	if err != nil {
		fmt.Printf("❌ Payment service error: %v\n", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "payment_processing_failed",
			"message": "Failed to process payment",
		})
	}

	// Convert core response to HTTP response (only essential fields for frontend)
	response := &MakePaymentResponse{
		PaymentID:   paymentResponse.PaymentID,
		Status:      string(paymentResponse.Status),
		Amount:      paymentResponse.Amount,
		RedirectURL: paymentResponse.RedirectURL,
		Message:     paymentResponse.Message,
	}

	fmt.Printf("✅ Payment initiated successfully: %s\n", paymentResponse.PaymentID)
	fmt.Printf("🌐 Redirect URL: %s\n", paymentResponse.RedirectURL)
	fmt.Printf("🏦 Gateway Used: %s\n", paymentResponse.GatewayUsed)
	fmt.Println("=== HANDLER: MakePayment Complete ===")

	return c.JSON(http.StatusCreated, map[string]any{
		"message":  "Payment initiated successfully",
		"endpoint": "POST /v1/process_payment/make_payment",
		"status":   "success",
		"data":     response,
	})
}

// AddPaymentSource handles POST /wallet/add_source
func (h *PaymentHandler) AddPaymentSource(c echo.Context) error {
	fmt.Println("=== HANDLER: AddPaymentSource API Called ===")

	// Parse request body
	var req AddPaymentSourceRequest
	if err := c.Bind(&req); err != nil {
		fmt.Printf("❌ Error binding request: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	// Validate business_id
	if req.BusinessID == "" {
		fmt.Println("❌ business_id is missing")
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "missing_business_id",
			"message": "business_id is required in request body",
		})
	}

	businessID := req.BusinessID
	fmt.Printf("📥 Request: Business ID: %s, Source Type: %s\n", businessID, req.SourceType)

	// Validate request
	if err := h.validateAddPaymentSourceRequest(&req); err != nil {
		fmt.Printf("❌ Validation error: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "validation_failed",
			"message": err.Error(),
		})
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(c.Request().Context(), 30*time.Second)
	defer cancel()

	// Route to appropriate handler based on source type
	switch req.SourceType {
	case "bank_account":
		return h.handleBankAccountAddition(ctx, c, businessID, &req)
	case "upi":
		return h.handleUPIAccountAddition(ctx, c, businessID, &req)
	default:
		fmt.Printf("❌ Unsupported source type: %s\n", req.SourceType)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "unsupported_source_type",
			"message": fmt.Sprintf("Source type '%s' is not supported", req.SourceType),
		})
	}
}

// handleBankAccountAddition handles bank account verification and addition
func (h *PaymentHandler) handleBankAccountAddition(ctx context.Context, c echo.Context, businessID string, req *AddPaymentSourceRequest) error {
	fmt.Println("🏦 Processing bank account addition...")

	// Convert HTTP request to core domain request
	bankAccount := &payment.BankAccount{
		AccountNumber:     req.BankAccountDetails.AccountNumber,
		IFSCCode:          req.BankAccountDetails.IFSCCode,
		AccountHolderName: req.BankAccountDetails.AccountHolderName,
		BankName:          req.BankAccountDetails.BankName,
		BranchName:        req.BankAccountDetails.BranchName,
		AccountType:       payment.BankAccountType(req.BankAccountDetails.AccountType),
	}

	fmt.Println("🔄 Calling Payment Service for bank account verification and addition...")

	// Call payment service to add and verify bank account
	err := h.paymentService.AddPaymentSource(businessID, bankAccount)

	if err != nil {
		fmt.Printf("❌ Bank account addition failed: %v\n", err)

		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "internal_error",
			"message": fmt.Sprintf("Failed to add bank account: %v", err),
		})
	}

	// Create success response
	response := AddPaymentSourceResponse{
		Success:            true,
		Message:            "Bank account verified and added successfully",
		SourceID:           bankAccount.GetSourceID(),
		SourceType:         "bank_account",
		VerificationStatus: "verified",
		AccountDetails: &VerifiedAccountDetails{
			MaskedAccountNumber: h.maskAccountNumber(req.BankAccountDetails.AccountNumber),
			VerifiedName:        req.BankAccountDetails.AccountHolderName, // TODO: Use verified name from response
			BankName:            req.BankAccountDetails.BankName,
			BranchName:          req.BankAccountDetails.BranchName,
			IFSCCode:            req.BankAccountDetails.IFSCCode,
			AccountType:         req.BankAccountDetails.AccountType,
			IsActive:            true,
		},
	}

	fmt.Printf("✅ Bank account added successfully with ID: %s\n", bankAccount.GetSourceID())
	fmt.Println("=== HANDLER: AddPaymentSource Complete ===")

	return c.JSON(http.StatusCreated, map[string]any{
		"message":  "Payment source added successfully",
		"endpoint": "POST /v1/wallet/add_source",
		"status":   "success",
		"data":     response,
	})
}

// handleUPIAccountAddition handles UPI account verification and addition
func (h *PaymentHandler) handleUPIAccountAddition(ctx context.Context, c echo.Context, businessID string, req *AddPaymentSourceRequest) error {
	fmt.Println("📱 Processing UPI account addition...")

	// TODO: Implement UPI verification logic when UPI service is available
	return c.JSON(http.StatusNotImplemented, map[string]any{
		"error":   "not_implemented",
		"message": "UPI account addition is not yet implemented",
	})
}

// Helper functions for AddPaymentSource handler

// validateAddPaymentSourceRequest validates the add payment source request
func (h *PaymentHandler) validateAddPaymentSourceRequest(req *AddPaymentSourceRequest) error {
	// Validate source type
	if req.SourceType == "" {
		return fmt.Errorf("source_type is required")
	}

	// Validate source-specific details
	switch req.SourceType {
	case "bank_account":
		if req.BankAccountDetails == nil {
			return fmt.Errorf("bank_account_details is required for bank_account source type")
		}
		return h.validateBankAccountDetails(req.BankAccountDetails)
	case "upi":
		if req.UPIAccountDetails == nil {
			return fmt.Errorf("upi_account_details is required for upi source type")
		}
		return h.validateUPIAccountDetails(req.UPIAccountDetails)
	default:
		return fmt.Errorf("unsupported source_type: %s", req.SourceType)
	}
}

// validateBankAccountDetails validates bank account details
func (h *PaymentHandler) validateBankAccountDetails(details *BankAccountDetails) error {
	if details.AccountNumber == "" {
		return fmt.Errorf("account_number is required")
	}
	if details.IFSCCode == "" {
		return fmt.Errorf("ifsc_code is required")
	}
	if details.AccountHolderName == "" {
		return fmt.Errorf("account_holder_name is required")
	}
	if details.BankName == "" {
		return fmt.Errorf("bank_name is required")
	}
	if details.AccountType == "" {
		return fmt.Errorf("account_type is required")
	}
	return nil
}

// validateUPIAccountDetails validates UPI account details
func (h *PaymentHandler) validateUPIAccountDetails(details *UPIAccountDetails) error {
	if details.UPIAddress == "" {
		return fmt.Errorf("upi_address is required")
	}
	return nil
}

// maskAccountNumber masks account number for display
func (h *PaymentHandler) maskAccountNumber(accountNumber string) string {
	if len(accountNumber) <= 4 {
		return "****"
	}
	return "****" + accountNumber[len(accountNumber)-4:]
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
		"card": true, "token_card": true, "net_banking": true, "upi": true, "wallet": true,
	}
	if !validMethods[req.PaymentMethod] {
		return fmt.Errorf("invalid payment method: %s", req.PaymentMethod)
	}

	// Set default calling_device to "web" if not provided
	if req.CallingDevice == "" {
		req.CallingDevice = "web"
	}

	// Validate calling_device
	validDevices := map[string]bool{
		"web": true, "mobile": true, "pos": true,
	}
	if !validDevices[req.CallingDevice] {
		return fmt.Errorf("invalid calling_device: %s. Valid values: web, mobile, pos", req.CallingDevice)
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
	case "token_card":
		if req.TokenDetails == nil {
			return fmt.Errorf("token_details are required for token card payments")
		}
		if req.TokenDetails.CardID == "" {
			return fmt.Errorf("card_id is required for token card payments")
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
	case "token_card":
		if httpReq.TokenDetails != nil {
			coreReq.TokenCardDetails = &payment.TokenCardDetails{
				CardID: httpReq.TokenDetails.CardID,
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

// Helper functions for Tokenize handler

// validateTokenizeRequest validates the tokenization request
func (h *PaymentHandler) validateTokenizeRequest(req *TokenizeRequest) error {
	if req.CardDetails == nil {
		return fmt.Errorf("card_details are required")
	}

	return h.validateCardDetails(req.CardDetails)
}

// extractLastFourDigits safely extracts the last 4 digits from card number
func extractLastFourDigits(cardNumber string) string {
	// Remove any spaces or special characters
	cleaned := strings.ReplaceAll(cardNumber, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")

	if len(cleaned) >= 4 {
		return cleaned[len(cleaned)-4:]
	}
	return cleaned
}

// handleTokenizationError provides user-friendly error responses
func (h *PaymentHandler) handleTokenizationError(err error) error {
	errorMessage := err.Error()

	// Categorize errors for better user experience
	switch {
	case strings.Contains(errorMessage, "transaction_declined"):
		return echo.NewHTTPError(http.StatusBadRequest, map[string]any{
			"error":   "card_declined",
			"message": "Card was declined. Please check your card details and try again.",
		})
	case strings.Contains(errorMessage, "invalid_card"):
		return echo.NewHTTPError(http.StatusBadRequest, map[string]any{
			"error":   "invalid_card",
			"message": "Invalid card details. Please check and try again.",
		})
	case strings.Contains(errorMessage, "timeout"):
		return echo.NewHTTPError(http.StatusRequestTimeout, map[string]any{
			"error":   "timeout",
			"message": "Tokenization timed out. Please try again.",
		})
	case strings.Contains(errorMessage, "gateway"):
		return echo.NewHTTPError(http.StatusServiceUnavailable, map[string]any{
			"error":   "gateway_error",
			"message": "Payment gateway temporarily unavailable. Please try again later.",
		})
	default:
		return echo.NewHTTPError(http.StatusInternalServerError, map[string]any{
			"error":   "tokenization_failed",
			"message": "Unable to save card. Please try again.",
		})
	}
}

// GetPaymentSourcesWithFees handles POST /process_payment/get_payment_sources_with_fees (Phase 10)
func (h *PaymentHandler) GetPaymentSourcesWithFees(c echo.Context) error {
	fmt.Println("=== HANDLER: GetPaymentSourcesWithFees API Called (Phase 10) ===")

	// Parse request body
	var req GetPaymentSourcesWithFeesRequest
	if err := c.Bind(&req); err != nil {
		fmt.Printf("❌ Error binding request: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	// Validate business_id
	if req.BusinessID == "" {
		fmt.Println("❌ business_id is missing")
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "missing_business_id",
			"message": "business_id is required in request body",
		})
	}

	// Round base amount to nearest integer (paise)
	baseAmountInt := int64(req.BaseAmount + 0.5) // Round to nearest int64

	fmt.Printf("📥 Request: Business ID: %s, Context: %s, Amount: %.2f (rounded to %d paise)\n",
		req.BusinessID, req.PaymentContext, req.BaseAmount, baseAmountInt)

	// Validate payment context
	paymentContext := payment.PaymentContext(req.PaymentContext)
	if !paymentContext.IsValid() {
		fmt.Printf("❌ Invalid payment context: %s\n", req.PaymentContext)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_payment_context",
			"message": fmt.Sprintf("Invalid payment context: %s. Valid contexts: vendor_payment, gst_payment, bill_payment, payroll", req.PaymentContext),
		})
	}

	// Validate base amount
	if baseAmountInt <= 0 {
		fmt.Printf("❌ Invalid base amount: %d\n", baseAmountInt)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_amount",
			"message": "Base amount must be greater than 0",
		})
	}

	// Call payment source service
	fmt.Println("🔄 Calling Payment Source Service...")
	ctx, cancel := context.WithTimeout(c.Request().Context(), 10*time.Second)
	defer cancel()

	response, err := h.paymentSourceService.GetPaymentSourcesWithFees(
		ctx,
		req.BusinessID,
		paymentContext,
		baseAmountInt,
	)
	if err != nil {
		fmt.Printf("❌ Payment source service error: %v\n", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "service_error",
			"message": "Failed to retrieve payment sources with fees",
		})
	}

	// Convert to API response format
	apiResponse := convertPaymentSourcesResponse(response)

	fmt.Printf("✅ Payment sources fetched successfully\n")
	fmt.Printf("📊 Total sources: %d\n", len(response.Sources))
	fmt.Println("=== HANDLER: GetPaymentSourcesWithFees Complete ===")

	return c.JSON(http.StatusOK, map[string]any{
		"message":  "Payment sources with fees retrieved successfully",
		"endpoint": "POST /v1/process_payment/get_payment_sources_with_fees",
		"status":   "success",
		"data":     apiResponse,
	})
}

// convertPaymentSourcesResponse converts service response to API response
func convertPaymentSourcesResponse(serviceResp *payments.GetPaymentSourcesResponse) *GetPaymentSourcesWithFeesResponse {
	apiSources := make([]PaymentSourceWithFeesResponse, 0, len(serviceResp.Sources))

	for _, source := range serviceResp.Sources {
		apiSource := PaymentSourceWithFeesResponse{
			SourceID:          source.SourceID,
			SourceType:        string(source.SourceType),
			PaymentMethod:     string(source.PaymentMethod),
			DisplayName:       source.DisplayName,
			LastFourDigits:    source.LastFourDigits,
			LastFiveDigits:    source.LastFiveDigits,
			BankName:          source.BankName,
			IFSCCode:          source.IFSCCode,
			UPIAddress:        source.UPIAddress,
			IsAllowed:         source.IsAllowed,
			RestrictionReason: source.RestrictionReason,
		}

		// Convert card type/category pointers
		if source.CardType != nil {
			cardType := string(*source.CardType)
			apiSource.CardType = &cardType
		}
		if source.CardCategory != nil {
			cardCategory := string(*source.CardCategory)
			apiSource.CardCategory = &cardCategory
		}

		// Convert fee breakdown if present
		if source.FeeBreakdown != nil {
			apiSource.FeeBreakdown = &FeeBreakdownResponse{
				BaseAmount:            source.FeeBreakdown.BaseAmount,
				ConvenienceFee:        source.FeeBreakdown.ConvenienceFee,
				ConvenienceFeeGST:     source.FeeBreakdown.ConvenienceFeeGST,
				TotalFee:              source.FeeBreakdown.TotalFee,
				TotalAmount:           source.FeeBreakdown.TotalAmount,
				ConvenienceFeePercent: source.FeeBreakdown.ConvenienceFeePercent,
				GSTPercent:            source.FeeBreakdown.GSTPercent,
			}
		}

		apiSources = append(apiSources, apiSource)
	}

	return &GetPaymentSourcesWithFeesResponse{
		BusinessID:     serviceResp.BusinessID,
		PaymentContext: string(serviceResp.PaymentContext),
		BaseAmount:     serviceResp.BaseAmount,
		Sources:        apiSources,
	}
}

// RegisterPaymentRoutes registers all payment processing routes
func RegisterPaymentRoutes(group *echo.Group, paymentService *payments.PaymentService, paymentSourceService *payments.PaymentSourceService, authService auth.AuthService) {
	handler := NewPaymentHandler(paymentService, paymentSourceService)

	// Apply JWT middleware to all payment routes
	group.Use(middleware.JWTMiddleware(authService))

	// Core payment operations
	group.POST("/get_fee_rules", handler.GetFeeRules)
	group.POST("/tokenize", handler.Tokenize)
	group.POST("/make_payment", handler.MakePayment)

	// Phase 10: Payment source selection with context-aware fees
	group.POST("/get_payment_sources_with_fees", handler.GetPaymentSourcesWithFees)

	// Payment list for dashboard/history
	group.GET("/list", handler.GetPaymentList)
}

// RegisterWalletRoutes registers all wallet management routes
func RegisterWalletRoutes(group *echo.Group, paymentService *payments.PaymentService, paymentSourceService *payments.PaymentSourceService, authService auth.AuthService) {
	handler := NewPaymentHandler(paymentService, paymentSourceService)

	// Apply JWT middleware to all wallet routes
	group.Use(middleware.JWTMiddleware(authService))

	// Wallet operations
	group.POST("/fetch", handler.FetchWallet)
	group.POST("/add_source", handler.AddPaymentSource)
}

// GetPaymentList handles GET /payments
// Retrieves paginated list of payments for a business with voucher/bill aggregation
func (h *PaymentHandler) GetPaymentList(c echo.Context) error {
	// Extract business_id from X-Business-ID header
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}

	// Extract userId from JWT context for authorization
	userID, ok := c.Get("userId").(string)
	if !ok || userID == "" {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error":   "MISSING_USER_ID",
			"message": "User ID not found in token",
		})
	}

	// Parse pagination parameters
	var req struct {
		Limit  int `query:"limit"`
		Offset int `query:"offset"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "INVALID_REQUEST",
			"message": "Invalid query parameters",
		})
	}

	// Call payment service (validation happens in service layer)
	response, err := h.paymentService.GetPaymentList(c.Request().Context(), businessID, req.Limit, req.Offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "SERVICE_ERROR",
			"message": "Failed to retrieve payment list",
		})
	}

	return c.JSON(http.StatusOK, response)
}
