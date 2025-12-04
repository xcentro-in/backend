package easebuzz

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"xcentro.in/internal/core/payment"
	"xcentro.in/internal/core/user"
	"xcentro.in/internal/shared/logger"
)

// EasebuzzClient implements the PaymentGateway interface for Easebuzz
type EasebuzzClient struct {
	baseURL             string
	merchantKey         string
	salt                string
	apiKey              string                 // EBZ-KEY for tokenization API
	merchantTokenID     string                 // Merchant token ID provided by Easebuzz
	tokenizationBaseURL string                 // TMS base URL for tokenization
	isAggregatorModel   bool                   // Payment model: false = normal (merchant charges), true = aggregator (PG charges)
	httpClient          *http.Client
	hashGenerator       *SecureHashGenerator   // Hash generator for request/response signatures
	userService         user.UserService       // User service for fetching customer details (required for tokenization)
	logger              *logger.Logger
}

// NewEasebuzzClient creates a new Easebuzz payment gateway client
//
// Parameters:
//   - baseURL: Easebuzz API base URL (e.g., "https://pay.easebuzz.in")
//   - merchantKey: EASEBUZZ_MERCHANT_KEY from environment
//   - salt: EASEBUZZ_SALT from environment
//   - apiKey: EASEBUZZ_API_KEY for tokenization (EBZ-KEY header)
//   - merchantTokenID: EASEBUZZ_MERCHANT_TOKEN_ID from environment (provided by Easebuzz)
//   - isAggregatorModel: EASEBUZZ_IS_AGGREGATOR_MODEL from environment (payment model)
//   - userService: UserService for fetching customer details (required for tokenization)
//
// Usage:
//
//	client := NewEasebuzzClient(
//	    cfg.Easebuzz.BaseURL,
//	    cfg.Easebuzz.MerchantKey,
//	    cfg.Easebuzz.Salt,
//	    cfg.Easebuzz.APIKey,
//	    cfg.Easebuzz.MerchantTokenID,
//	    cfg.Easebuzz.IsAggregatorModel,
//	    userService,
//	)
func NewEasebuzzClient(baseURL, merchantKey, salt, apiKey, merchantTokenID string, isAggregatorModel bool, userService user.UserService) payment.PaymentGateway {
	log := logger.GetDefault()

	paymentModel := "Normal"
	if isAggregatorModel {
		paymentModel = "Aggregator"
	}

	log.InfoWith("Payment Gateway Initialized: Easebuzz",
		"merchant_key", merchantKey,
		"base_url", baseURL,
		"payment_model", paymentModel,
		"timeout", "30s")

	return &EasebuzzClient{
		baseURL:             baseURL,
		merchantKey:         merchantKey,
		salt:                salt,
		apiKey:              apiKey,
		merchantTokenID:     merchantTokenID,
		isAggregatorModel:   isAggregatorModel,
		tokenizationBaseURL: "https://tms.easebuzz.in", // Tokenization Management System
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		hashGenerator: NewSecureHashGenerator(merchantKey, salt),
		userService:   userService,
		logger:        log,
	}
}

// GetGatewayType returns the gateway type identifier
func (c *EasebuzzClient) GetGatewayType() payment.GatewayType {
	return payment.GatewayEasebuzz
}

// IsAggregatorModel returns the payment model for this gateway
func (c *EasebuzzClient) IsAggregatorModel() bool {
	return c.isAggregatorModel
}

// TokenizeCard implements Easebuzz-specific tokenization flow
// Easebuzz Flow: Encrypt full payload → POST to TMS API
//
// API: POST https://tms.easebuzz.in/v1/create_token
// Header: EBZ-KEY (API key)
// Body: {"enc_data": "base64_encrypted_payload"}
func (c *EasebuzzClient) TokenizeCard(ctx context.Context, request payment.GatewayTokenizeRequest) (*payment.GatewayTokenizeResponse, error) {
	fmt.Println("=== EASEBUZZ GATEWAY: Tokenization (TMS API) ===")
	fmt.Printf("📥 Business ID: %s\n", request.BusinessID)
	fmt.Printf("📥 Transaction ID: %s\n", request.TransactionID)
	fmt.Printf("💳 Card: %s %s ending in %s\n",
		request.CardDetails.CardCategory, request.CardDetails.CardType, request.CardDetails.LastFourDigits)

	// Fetch customer details from database (REQUIRED for Easebuzz tokenization)
	// UserID must be provided in metadata
	userID, ok := request.Metadata["user_id"].(string)
	if !ok || userID == "" {
		return nil, fmt.Errorf("user_id is required in metadata for Easebuzz tokenization")
	}

	// Fetch user from database
	if c.userService == nil {
		return nil, fmt.Errorf("user service not configured for Easebuzz")
	}

	userObj, err := c.userService.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user details: %w", err)
	}

	// Validate required fields
	if userObj.Email == "" {
		return nil, fmt.Errorf("user email is required for Easebuzz tokenization")
	}
	if userObj.Mobile == "" {
		return nil, fmt.Errorf("user mobile is required for Easebuzz tokenization")
	}

	// Extract customer details from user object
	customerName := userObj.Name
	if customerName == "" {
		customerName = "Customer" // Fallback only if name is not set
	}
	email := userObj.Email
	mobile := sanitizePhone(userObj.Mobile)

	// Use business ID as customer unique ID (for business-scoped tokenization)
	customerUniqueID := request.BusinessID

	// Use merchant token ID from Easebuzz configuration (provided by Easebuzz)
	merchantTokenID := c.merchantTokenID

	// Determine card type (dc/cc) and brand
	cardType := "cc" // Default credit
	if request.CardDetails.CardCategory == payment.CardCategoryDebit {
		cardType = "dc"
	}
	cardBrand := string(request.CardDetails.CardType) // "visa", "mastercard", etc.

	// Derive s_key and s_iv (same as payment encryption)
	sKey := deriveAES256Key(c.merchantKey)
	sIV := deriveFixedIV(c.salt)

	// Build tokenization request payload
	tokenReq := TokenizationRequest{
		CardNum:          request.CardDetails.Number, // Field is "Number" not "CardNumber"
		ExpiryMonth:      request.CardDetails.ExpiryMonth,
		ExpiryYear:       request.CardDetails.ExpiryYear,
		CVV:              request.CardDetails.CVV,
		NameOnCard:       request.CardDetails.HolderName, // Field is "HolderName" not "CardHolderName"
		CardType:         cardType,
		CardBrand:        cardBrand,
		Name:             customerName,
		Email:            email,
		Mobile:           mobile,
		CustomerUniqueID: customerUniqueID,
		MerchantTokenID:  merchantTokenID,
		TID:              request.TransactionID,
		MerchantKey:      c.merchantKey,
		Salt:             c.salt,
		SKey:             base64.StdEncoding.EncodeToString(sKey),
		SIV:              base64.StdEncoding.EncodeToString(sIV),
		Amount:           "1.00", // Tokenization amount
	}

	// Marshal to JSON
	jsonPayload, err := json.Marshal(tokenReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tokenization request: %w", err)
	}

	logger.Debug(string(jsonPayload))

	fmt.Printf("📝 Payload size: %d bytes\n", len(jsonPayload))

	// Encrypt full payload
	encData, err := EncryptFullPayload(string(jsonPayload), sKey, sIV)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt payload: %w", err)
	}

	fmt.Printf("🔐 Encrypted payload: %s...\n", encData[:50])

	// Build API request
	apiReq := TokenizationAPIRequest{
		EncData: encData,
	}

	reqBody, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal API request: %w", err)
	}

	// Make HTTP POST to tokenization API
	req, err := http.NewRequestWithContext(ctx, "POST", c.tokenizationBaseURL+"/v1/create_token", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("EBZ-KEY", c.apiKey)

	fmt.Println("🚀 Calling Easebuzz TMS API...")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var tokenResp TokenizationResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse tokenization response: %w", err)
	}

	if tokenResp.Status != 1 {
		return nil, fmt.Errorf("tokenization failed: %s", tokenResp.Error)
	}

	fmt.Printf("✅ Token created: %s\n", tokenResp.Data.Token[:10]+"...")

	// Map to gateway response
	response := &payment.GatewayTokenizeResponse{
		Token:         tokenResp.Data.Token,
		CardID:        tokenResp.Data.CardID,
		TransactionID: request.TransactionID,
		Status:        "success",
		Message:       tokenResp.Data.Message,
		Metadata: map[string]any{
			"gateway":           "easebuzz",
			"tokenization_flow": "tms_api",
			"card_brand":        tokenResp.Data.CardBrand,
			"card_type":         tokenResp.Data.CardType,
			"last_four":         tokenResp.Data.LastFour,
		},
	}

	fmt.Println("=== EASEBUZZ GATEWAY: Tokenization Complete ===")

	return response, nil
}

// InitiatePayment implements Easebuzz payment initiation
// This is the main entry point for all Easebuzz payments
//
// Flow:
//  1. Call initiatePaymentAPI() to get access_key
//  2. Based on payment method, call appropriate API:
//     - Token card: processTokenPayment() with encryption
//     - Net banking/UPI: TODO
func (c *EasebuzzClient) InitiatePayment(ctx context.Context, req payment.GatewayPaymentRequest) (*payment.GatewayPaymentResponse, error) {
	fmt.Println("=== EASEBUZZ GATEWAY: InitiatePayment Called ===")
	fmt.Printf("📥 Business ID: %s\n", req.BusinessID)
	fmt.Printf("📥 Transaction ID: %s\n", req.TransactionID)
	fmt.Printf("💰 Amount: %d %s\n", req.Amount, req.Currency)
	fmt.Printf("🔀 Payment Method: %s\n", req.PaymentMethod)

	// STEP 1: Get access_key from Easebuzz Initiate Payment API
	accessKey, err := c.initiatePaymentAPI(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("initiate payment failed: %w", err)
	}

	fmt.Printf("✅ Access key obtained: %s...\n", accessKey[:10])

	// STEP 2: Process based on payment method
	switch req.PaymentMethod {
	case payment.PaymentMethodCard:
		if req.Token != "" {
			// Token-based payment with ENCRYPTION
			// Easebuzz-specific: Always generate cryptogram internally
			// Cryptogram is a PG-specific implementation detail, not exposed to service layer
			fmt.Println("💳 Processing token-based card payment with encryption...")

			// Generate cryptogram internally
			cryptogramReq := payment.CryptogramRequest{
				Token:         req.Token,
				Amount:        req.Amount,
				Currency:      req.Currency,
				TransactionID: req.TransactionID,
				BusinessID:    req.BusinessID,
				Metadata:      req.Metadata,
			}

			cryptogramResp, err := c.GenerateCryptogram(ctx, cryptogramReq)
			if err != nil {
				return nil, fmt.Errorf("failed to generate cryptogram: %w", err)
			}

			// Set cryptogram in request
			req.Cryptogram = cryptogramResp.Cryptogram

			// Chain ALL cryptogram metadata to request for processTokenPayment
			// These values are REQUIRED by initiate_seamless_payment API
			if req.Metadata == nil {
				req.Metadata = make(map[string]any)
			}
			req.Metadata["network_token"] = cryptogramResp.Metadata["network_token"]
			req.Metadata["token_expiry"] = cryptogramResp.Metadata["token_expiry"]
			req.Metadata["token_requester_id"] = cryptogramResp.Metadata["token_requester_id"]
			req.Metadata["easebuzz_token_id"] = cryptogramResp.Metadata["easebuzz_token_id"]

			return c.processTokenPayment(ctx, accessKey, req)
		} else {
			// Direct card payment (not implemented yet)
			return nil, fmt.Errorf("direct card payment not supported yet")
		}

	case payment.PaymentMethodNetBanking:
		// TODO: Implement net banking flow
		return nil, fmt.Errorf("net banking not implemented yet")

	case payment.PaymentMethodUPI:
		// TODO: Implement UPI flow
		return nil, fmt.Errorf("UPI not implemented yet")

	default:
		return nil, fmt.Errorf("unsupported payment method for Easebuzz: %s", req.PaymentMethod)
	}
}

// GenerateCryptogram generates Easebuzz-specific cryptogram
//
// API: POST https://tms.easebuzz.in/v1/get_cryptogram
// Header: EBZ-KEY (API key)
// Body: {"enc_data": "encrypted_payload"}
func (c *EasebuzzClient) GenerateCryptogram(ctx context.Context, request payment.CryptogramRequest) (*payment.CryptogramResponse, error) {
	fmt.Println("=== EASEBUZZ GATEWAY: GenerateCryptogram (TMS API) ===")
	fmt.Printf("🔐 Token: %s\n", request.Token[:10]+"...")
	fmt.Printf("💰 Amount: %d %s\n", request.Amount, request.Currency)

	// Generate merchant token ID (or extract from metadata if stored)
	merchantTokenID := fmt.Sprintf("mtoken_%s_%d", request.BusinessID[:8], time.Now().Unix())
	if mtid, ok := request.Metadata["merchant_token_id"].(string); ok && mtid != "" {
		merchantTokenID = mtid
	}

	// Derive s_key and s_iv (same as tokenization encryption)
	sKey := deriveAES256Key(c.merchantKey)
	sIV := deriveFixedIV(c.salt)

	// Build cryptogram request payload
	cryptoReq := CryptogramRequest{
		MerchantTokenID: merchantTokenID,
		EasebuzzTokenID: request.Token, // This is the easebuzz token from stored card
	}

	// Marshal to JSON
	jsonPayload, err := json.Marshal(cryptoReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cryptogram request: %w", err)
	}

	fmt.Printf("📝 Payload size: %d bytes\n", len(jsonPayload))

	// Encrypt full payload
	encData, err := EncryptFullPayload(string(jsonPayload), sKey, sIV)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt payload: %w", err)
	}

	fmt.Printf("🔐 Encrypted payload: %s...\n", encData[:50])

	// Build API request
	apiReq := CryptogramAPIRequest{
		EncData: encData,
	}

	reqBody, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal API request: %w", err)
	}

	// Make HTTP POST to cryptogram API
	req, err := http.NewRequestWithContext(ctx, "POST", c.tokenizationBaseURL+"/v1/get_cryptogram", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("EBZ-KEY", c.apiKey)

	fmt.Println("🚀 Calling Easebuzz TMS API for cryptogram...")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var cryptoResp CryptogramResponse
	if err := json.Unmarshal(body, &cryptoResp); err != nil {
		return nil, fmt.Errorf("failed to parse cryptogram response: %w", err)
	}

	if !cryptoResp.Status {
		return nil, fmt.Errorf("cryptogram generation failed: %s", cryptoResp.Error)
	}

	fmt.Printf("✅ Cryptogram generated: %s...\n", cryptoResp.Data.TokenDetails.Cryptogram[:20])

	// Format token expiry as MM/YYYY (required by initiate_seamless_payment API)
	tokenExpiry := fmt.Sprintf("%s/%s",
		cryptoResp.Data.TokenDetails.ExpirationMonth,
		cryptoResp.Data.TokenDetails.ExpirationYear)

	// Cryptogram validity: 15 minutes (Easebuzz standard)
	expiresAt := time.Now().Add(15 * time.Minute)

	// Map to gateway response with ALL required fields for payment
	response := &payment.CryptogramResponse{
		Cryptogram:   cryptoResp.Data.TokenDetails.Cryptogram,
		ExpiresAt:    expiresAt,
		ValidityMins: 15,
		Algorithm:    "Easebuzz-AES256-CBC",
		Metadata: map[string]any{
			"gateway":             "easebuzz",
			"cryptogram_flow":     "tms_api",
			"easebuzz_token_id":   cryptoResp.Data.EasebuzzTokenID,
			"merchant_token_id":   cryptoResp.Data.MerchantTokenID,
			"network_token":       cryptoResp.Data.TokenDetails.Number,       // CRITICAL: Use this as card_token in payment
			"token_expiry":        tokenExpiry,                               // CRITICAL: MM/YYYY format for token_expiry_date
			"token_requester_id":  cryptoResp.Data.TokenDetails.RequestorID, // CRITICAL: TRID for payment
			"expiration_month":    cryptoResp.Data.TokenDetails.ExpirationMonth,
			"expiration_year":     cryptoResp.Data.TokenDetails.ExpirationYear,
		},
	}

	fmt.Println("=== EASEBUZZ GATEWAY: GenerateCryptogram Complete ===")

	return response, nil
}

// ValidateWebhook validates Easebuzz callback/webhook signatures
//
// Easebuzz uses SHA-512 reverse hash for signature verification.
// The signature (hash) is calculated from response fields in reverse order.
//
// Verification steps:
//  1. Parse the payload to extract all fields
//  2. Build reverse hash sequence: salt|status|udf10|...|udf1|email|firstname|productinfo|amount|txnid|key
//  3. Calculate SHA-512 hash
//  4. Compare with provided signature using constant-time comparison
//
// This method is used for both:
//   - Callback (SURL/FURL): Synchronous response after payment
//   - Webhook: Asynchronous settlement notification
func (c *EasebuzzClient) ValidateWebhook(payload []byte, signature string) error {
	fmt.Println("=== EASEBUZZ GATEWAY: ValidateWebhook Called ===")
	fmt.Printf("📝 Payload length: %d bytes\n", len(payload))

	// Parse payload to CallbackPayload struct
	var callback CallbackPayload
	if err := json.Unmarshal(payload, &callback); err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	// Delegate to hash generator for validation
	return c.hashGenerator.ValidateFromResponse(callback)
}

// initiatePaymentAPI calls Easebuzz Initiate Payment API to get access_key
// This is Step 1 of the Easebuzz payment flow
//
// API: POST /payment/initiateLink
// Returns: access_key for step 2
func (c *EasebuzzClient) initiatePaymentAPI(ctx context.Context, req payment.GatewayPaymentRequest) (string, error) {
	// Extract customer details from metadata
	firstname := extractFirstName(req)
	email := extractEmail(req)
	phone := extractPhone(req)

	// Format amount to rupees with 2 decimals
	amountRupees := fmt.Sprintf("%.2f", float64(req.Amount)/100.0)

	// Extract UDF values from metadata (if present)
	// UDF fields are optional and will be empty strings if not provided
	udf1 := extractUDF(req.Metadata, "udf1")
	udf2 := extractUDF(req.Metadata, "udf2")
	udf3 := extractUDF(req.Metadata, "udf3")
	udf4 := extractUDF(req.Metadata, "udf4")
	udf5 := extractUDF(req.Metadata, "udf5")
	udf6 := extractUDF(req.Metadata, "udf6")
	udf7 := extractUDF(req.Metadata, "udf7")
	udf8 := extractUDF(req.Metadata, "udf8")
	udf9 := extractUDF(req.Metadata, "udf9")
	udf10 := extractUDF(req.Metadata, "udf10")

	// Build hash parameters
	hashParams := RequestHashParams{
		TxnID:       req.TransactionID,
		Amount:      amountRupees,
		ProductInfo: req.Description,
		FirstName:   firstname,
		Email:       email,
		UDF1:        udf1,
		UDF2:        udf2,
		UDF3:        udf3,
		UDF4:        udf4,
		UDF5:        udf5,
		UDF6:        udf6,
		UDF7:        udf7,
		UDF8:        udf8,
		UDF9:        udf9,
		UDF10:       udf10,
	}

	// Generate hash using hash generator
	hash := c.hashGenerator.GenerateForRequest(hashParams)

	// Build callback URLs with gateway type parameter
	// Format: https://api.xcentro.com/api/v1/process_payment/callback?type=easebuzz
	surl := buildCallbackURL(req.CallbackURL, "easebuzz")
	furl := buildCallbackURL(req.CallbackURL, "easebuzz")

	fmt.Printf("🔗 Callback URLs - SURL: %s, FURL: %s\n", surl, furl)

	// Build form data
	formData := url.Values{
		"key":          {c.merchantKey},
		"txnid":        {req.TransactionID},
		"amount":       {amountRupees},
		"productinfo":  {req.Description},
		"firstname":    {firstname},
		"phone":        {phone},
		"email":        {email},
		"surl":         {surl},
		"furl":         {furl},
		"hash":         {hash},
		"request_flow": {"SEAMLESS"}, // Critical for seamless flow
	}

	// Add UDF fields to form data if they have values
	if udf1 != "" {
		formData.Add("udf1", udf1)
	}
	if udf2 != "" {
		formData.Add("udf2", udf2)
	}
	if udf3 != "" {
		formData.Add("udf3", udf3)
	}
	if udf4 != "" {
		formData.Add("udf4", udf4)
	}
	if udf5 != "" {
		formData.Add("udf5", udf5)
	}
	if udf6 != "" {
		formData.Add("udf6", udf6)
	}
	if udf7 != "" {
		formData.Add("udf7", udf7)
	}
	if udf8 != "" {
		formData.Add("udf8", udf8)
	}
	if udf9 != "" {
		formData.Add("udf9", udf9)
	}
	if udf10 != "" {
		formData.Add("udf10", udf10)
	}

	// Make HTTP POST request
	resp, err := c.postFormData(ctx, "/payment/initiateLink", formData)
	if err != nil {
		return "", fmt.Errorf("initiate payment API call failed: %w", err)
	}

	// Parse response
	var initResp InitiatePaymentResponse
	if err := json.Unmarshal(resp, &initResp); err != nil {
		return "", fmt.Errorf("failed to parse initiate payment response: %w", err)
	}

	if initResp.Status != 1 {
		return "", fmt.Errorf("initiate payment failed: %s", initResp.Message)
	}

	return initResp.Data.AccessKey, nil
}

// processTokenPayment processes token-based payment with encrypted card fields
// This is Step 2 of the Easebuzz payment flow (after getting access_key)
//
// API: POST /initiate_seamless_payment/
// Encrypts: card_token, cryptogram, token_expiry_date, card_cvv (if needed)
func (c *EasebuzzClient) processTokenPayment(
	ctx context.Context,
	accessKey string,
	req payment.GatewayPaymentRequest,
) (*payment.GatewayPaymentResponse, error) {
	fmt.Println("=== EASEBUZZ: Processing Token Payment with Encryption ===")

	// Extract network token from metadata (from cryptogram response)
	// This is the token_details.number field from getCryptogram API
	networkToken, ok := req.Metadata["network_token"].(string)
	if !ok || networkToken == "" {
		return nil, fmt.Errorf("network_token not found in metadata - ensure GenerateCryptogram was called")
	}

	// ENCRYPT network token (NOT the stored vault token!)
	// The API requires the network token from cryptogram response as card_token
	encryptedToken, err := EncryptCardField(networkToken, c.merchantKey, c.salt)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt network token: %w", err)
	}
	fmt.Printf("✅ Network token encrypted: %s...\n", encryptedToken[:20])

	encryptedCrypto, err := EncryptCardField(req.Cryptogram, c.merchantKey, c.salt)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt cryptogram: %w", err)
	}
	fmt.Printf("✅ Cryptogram encrypted: %s...\n", encryptedCrypto[:20])

	// Extract and encrypt token expiry
	tokenExpiry, err := extractTokenExpiry(req)
	if err != nil {
		return nil, fmt.Errorf("failed to extract token_expiry: %w", err)
	}
	encryptedExpiry, err := EncryptCardField(tokenExpiry, c.merchantKey, c.salt)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt token_expiry: %w", err)
	}
	fmt.Printf("✅ Token expiry encrypted: %s...\n", encryptedExpiry[:20])

	// Extract token requester ID
	tokenRequesterID, err := extractTokenRequesterID(req)
	if err != nil {
		return nil, fmt.Errorf("failed to extract token_requester_id: %w", err)
	}

	// Determine payment mode (CC or DC)
	paymentMode := determineCardPaymentMode(req.CardDetails.CardCategory)

	// Build form data with ENCRYPTED fields
	formData := url.Values{
		"access_key":         {accessKey},
		"payment_mode":       {string(paymentMode)},
		"card_token":         {encryptedToken},  // ENCRYPTED
		"cryptogram":         {encryptedCrypto}, // ENCRYPTED
		"token_expiry_date":  {encryptedExpiry}, // ENCRYPTED
		"token_requester_id": {tokenRequesterID},
		"card_last_4_digits": {req.CardDetails.LastFourDigits},
	}

	// Conditionally add CVV (required for RuPay cards)
	if req.CardDetails.CardType == payment.CardTypeRupay && req.CardDetails.CVV != "" {
		encryptedCVV, err := EncryptCardField(req.CardDetails.CVV, c.merchantKey, c.salt)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt CVV: %w", err)
		}
		formData.Add("card_cvv", encryptedCVV) // ENCRYPTED
		fmt.Println("✅ CVV encrypted (required for RuPay)")
	}

	// Make HTTP POST request to token payment API
	resp, err := c.postFormData(ctx, "/initiate_seamless_payment/", formData)
	if err != nil {
		return nil, fmt.Errorf("token payment API call failed: %w", err)
	}

	// Parse response
	var paymentResp TokenPaymentResponse
	if err := json.Unmarshal(resp, &paymentResp); err != nil {
		return nil, fmt.Errorf("failed to parse token payment response: %w", err)
	}

	if paymentResp.Status != 1 {
		return nil, fmt.Errorf("token payment failed: %s - %s",
			paymentResp.Data.Error, paymentResp.Data.ErrorMessage)
	}

	// Map to generic gateway response
	return &payment.GatewayPaymentResponse{
		PaymentID:        req.TransactionID,
		GatewayPaymentID: paymentResp.Data.EasepayID,
		TransactionID:    paymentResp.Data.TransactionID,
		Status:           mapEasebuzzStatus(paymentResp.Data.Status),
		Message:          paymentResp.Data.Message,
		RedirectURL:      paymentResp.Data.RedirectURL,
		Amount:           req.Amount,
		Currency:         req.Currency,
		CreatedAt:        time.Now(),
		Metadata: map[string]any{
			"gateway":        "easebuzz",
			"easepayid":      paymentResp.Data.EasepayID,
			"bank_ref_num":   paymentResp.Data.BankRefNum,
			"payment_source": paymentResp.Data.PaymentSource,
			"card_category":  paymentResp.Data.CardCategory,
		},
	}, nil
}

// postFormData makes a POST request with form-encoded data
func (c *EasebuzzClient) postFormData(ctx context.Context, endpoint string, formData url.Values) ([]byte, error) {
	fullURL := c.baseURL + endpoint

	req, err := http.NewRequestWithContext(ctx, "POST", fullURL, bytes.NewBufferString(formData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// mapEasebuzzStatus maps Easebuzz status to generic payment status
func mapEasebuzzStatus(easebuzzStatus string) payment.PaymentStatus {
	switch easebuzzStatus {
	case "success":
		return payment.PaymentStatusSuccess
	case "failure":
		return payment.PaymentStatusFailed
	case "pending":
		return payment.PaymentStatusPending
	case "dropped", "userCancelled":
		return payment.PaymentStatusCancelled
	default:
		return payment.PaymentStatusPending
	}
}
