package jiopay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"xcentro.in/internal/core/payment"
)

// JioPayClient implements the PaymentGateway interface for JioPay
type JioPayClient struct {
	baseURL             string // Base URL for payment APIs
	tokenizationBaseURL string // Vault API base URL
	merchantID          string // JioPay merchant identifier
	apiKey              string // API authentication key for HMAC
	appID               string // x-app-id header for vault APIs
	encryptionKey       string // AES-128 encryption key for x-app-token
	httpClient          *http.Client
	hashGenerator       *SecureHashGenerator // Hash generator for request/response signatures
}

// NewJioPayClient creates a new JioPay payment gateway client
//
// Parameters:
//   - baseURL: JioPay API base URL (e.g., "https://api.jiopay.com")
//   - tokenizationBaseURL: Vault API base URL (e.g., "https://uat.jiopay.co.in/qatsp/api/vault")
//   - merchantID: JIOPAY_MERCHANT_ID from environment
//   - apiKey: JIOPAY_API_KEY from environment (for HMAC-SHA256)
//   - appID: JIOPAY_APP_ID from environment (for x-app-id header)
//   - encryptionKey: JIOPAY_ENCRYPTION_KEY from environment (16 bytes for AES-128)
//
// Usage:
//
//	client := NewJioPayClient(
//	    cfg.JioPay.BaseURL,
//	    cfg.JioPay.TokenizationBaseURL,
//	    cfg.JioPay.MerchantID,
//	    cfg.JioPay.APIKey,
//	    cfg.JioPay.AppID,
//	    cfg.JioPay.EncryptionKey,
//	)
func NewJioPayClient(baseURL, tokenizationBaseURL, merchantID, apiKey, appID, encryptionKey string) payment.PaymentGateway {
	return &JioPayClient{
		baseURL:             baseURL,
		tokenizationBaseURL: tokenizationBaseURL,
		merchantID:          merchantID,
		apiKey:              apiKey,
		appID:               appID,
		encryptionKey:       encryptionKey,
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // JioPay tokenization can take longer
		},
		hashGenerator: NewSecureHashGenerator(apiKey),
	}
}

// GetGatewayType returns the gateway type identifier
func (c *JioPayClient) GetGatewayType() payment.GatewayType {
	return payment.GatewayJioPay
}

// TokenizeCard implements JioPay card tokenization flow
// JioPay Flow: POST to /fetch-card with 5-object structure
//
// API: POST https://uat.jiopay.co.in/qatsp/api/vault/fetch-card
// Content-Type: application/json
// Body: {user, entity, card, consent, audit}
func (c *JioPayClient) TokenizeCard(ctx context.Context, request payment.GatewayTokenizeRequest) (*payment.GatewayTokenizeResponse, error) {
	fmt.Println("=== JIOPAY GATEWAY: Tokenization (Vault API) ===")
	fmt.Printf("📥 Business ID: %s\n", request.BusinessID)
	fmt.Printf("📥 Transaction ID: %s\n", request.TransactionID)
	fmt.Printf("💳 Card: %s %s ending in %s\n",
		request.CardDetails.CardCategory, request.CardDetails.CardType, request.CardDetails.LastFourDigits)

	// Build user object
	userObj := UserObject{
		ExternalID: request.BusinessID, // Use business ID as external customer ID
		Email:      extractCustomerEmail(request.Metadata),
		Name:       extractCustomerName(request.Metadata),
		Mobile:     buildMobileObject(request.Metadata),
	}

	// Build entity object
	entityObj := EntityObject{
		EntityID: c.merchantID,
	}

	// Build card object
	cardObj := CardObject{
		Number:   request.CardDetails.Number,
		ExpiryMm: request.CardDetails.ExpiryMonth,
		ExpiryYy: formatExpiryYear(request.CardDetails.ExpiryYear),
		Name:     request.CardDetails.HolderName,
		Remark:   "Card tokenization request",
	}

	// Add Authentication Reference Id for RuPay cards
	if request.CardDetails.CardType == payment.CardTypeRupay {
		// For RuPay, this should come from prior authentication
		// For now, generate a placeholder
		cardObj.AuthenticationReferenceID = fmt.Sprintf("AUTHREF_%s", uuid.New().String()[:10])
		fmt.Println("📝 RuPay card detected: Added Authentication Reference Id")
	}

	// Build consent and audit objects
	consentObj := buildConsentObject()
	auditObj := buildAuditObject()

	// Build full tokenization request
	tokenReq := TokenizationRequest{
		User:    userObj,
		Entity:  entityObj,
		Card:    cardObj,
		Consent: consentObj,
		Audit:   auditObj,
	}

	// Marshal to JSON
	jsonPayload, err := json.Marshal(tokenReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tokenization request: %w", err)
	}

	fmt.Printf("📝 Payload size: %d bytes\n", len(jsonPayload))

	// Generate mandatory headers for JioPay Vault API
	xTimestampID := generateXTimestampID()
	xTraceID := generateXTraceID()
	xAppToken, err := generateXAppToken(c.appID, xTraceID, xTimestampID, c.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to generate x-app-token: %w", err)
	}

	fmt.Printf("📋 x-timestamp-id: %s\n", xTimestampID)
	fmt.Printf("📋 x-trace-id: %s\n", xTraceID)
	fmt.Printf("🔐 x-app-token: %s...\n", xAppToken[:20])

	// Make HTTP POST to tokenization API
	req, err := http.NewRequestWithContext(ctx, "POST", c.tokenizationBaseURL+"/fetch-card", bytes.NewReader(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set mandatory headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-app-id", c.appID)
	req.Header.Set("x-timestamp-id", xTimestampID)
	req.Header.Set("x-trace-id", xTraceID)
	req.Header.Set("x-app-token", xAppToken)

	fmt.Println("🚀 Calling JioPay Vault API...")

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

	if tokenResp.Status.Status != "SUCCESS" {
		return nil, fmt.Errorf("tokenization failed: %s", tokenResp.Status.Status)
	}

	fmt.Printf("✅ Token created: VaultID %s\n", tokenResp.Card.VaultID)

	// Map to gateway response
	response := &payment.GatewayTokenizeResponse{
		Token:         tokenResp.Card.TokenReference, // Use token reference as token
		CardID:        tokenResp.Card.VaultID,        // Use vault ID as card ID
		TransactionID: request.TransactionID,
		Status:        "success",
		Message:       tokenResp.Card.Remark,
		Metadata: map[string]any{
			"gateway":           "jiopay",
			"tokenization_flow": "vault_api",
			"vault_id":          tokenResp.Card.VaultID,
			"masked_number":     tokenResp.Card.MaskedNumber,
			"bank_name":         tokenResp.Card.BankName,
			"card_scheme":       mapJioPayCardScheme(tokenResp.Card.Scheme),
			"card_type":         mapJioPayCardType(tokenResp.Card.Type),
			"is_domestic":       tokenResp.Card.IsDomestic,
			"cvv_less":          tokenResp.Card.CVVLess,
			"par":               tokenResp.Card.PAR,
		},
	}

	fmt.Println("=== JIOPAY GATEWAY: Tokenization Complete ===")

	return response, nil
}

// GenerateCryptogram generates JioPay-specific cryptogram
//
// API: POST https://uat.jiopay.co.in/qatsp/api/vault/getCryptogram
// Content-Type: application/json
// Body: {user, entity, amount}
func (c *JioPayClient) GenerateCryptogram(ctx context.Context, request payment.CryptogramRequest) (*payment.CryptogramResponse, error) {
	fmt.Println("=== JIOPAY GATEWAY: GenerateCryptogram (Vault API) ===")
	fmt.Printf("🔐 Token: %s\n", request.Token[:10]+"...")
	fmt.Printf("💰 Amount: %d %s\n", request.Amount, request.Currency)

	// Build cryptogram request
	cryptoReq := CryptogramRequest{
		User: UserObject{
			ExternalID: request.BusinessID,
		},
		Entity: EntityObject{
			EntityID: c.merchantID,
		},
		Amount: AmountObject{
			NetAmount: convertAmountToRupees(request.Amount), // Convert paise to rupees
		},
	}

	// Marshal to JSON
	jsonPayload, err := json.Marshal(cryptoReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cryptogram request: %w", err)
	}

	fmt.Printf("📝 Payload size: %d bytes\n", len(jsonPayload))

	// Generate mandatory headers for JioPay Vault API
	xTimestampID := generateXTimestampID()
	xTraceID := generateXTraceID()
	xAppToken, err := generateXAppToken(c.appID, xTraceID, xTimestampID, c.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to generate x-app-token: %w", err)
	}

	fmt.Printf("📋 x-timestamp-id: %s\n", xTimestampID)
	fmt.Printf("📋 x-trace-id: %s\n", xTraceID)
	fmt.Printf("🔐 x-app-token: %s...\n", xAppToken[:20])

	// Make HTTP POST to cryptogram API
	req, err := http.NewRequestWithContext(ctx, "POST", c.tokenizationBaseURL+"/getCryptogram", bytes.NewReader(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set mandatory headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-app-id", c.appID)
	req.Header.Set("x-timestamp-id", xTimestampID)
	req.Header.Set("x-trace-id", xTraceID)
	req.Header.Set("x-app-token", xAppToken)

	fmt.Println("🚀 Calling JioPay Vault API for cryptogram...")

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

	if cryptoResp.Status.Status != "SUCCESS" {
		return nil, fmt.Errorf("cryptogram generation failed: %s", cryptoResp.Status.Status)
	}

	fmt.Printf("✅ Cryptogram generated: %s...\n", cryptoResp.Secure.Cryptogram[:20])

	// Parse token expiry (MMYYYY format)
	expiresAt := time.Now().Add(10 * time.Minute) // Default 10 minutes
	if cryptoResp.Secure.TokenExpiry != "" {
		// TokenExpiry is in MMYYYY format (e.g., "062027")
		// Convert to time.Time for expiry calculation
		// For simplicity, assume 10-minute validity window
		fmt.Printf("📅 Token expiry: %s\n", cryptoResp.Secure.TokenExpiry)
	}

	// Map to gateway response
	response := &payment.CryptogramResponse{
		Cryptogram:   cryptoResp.Secure.Cryptogram,
		ExpiresAt:    expiresAt,
		ValidityMins: 10, // JioPay cryptogram validity
		Algorithm:    "JioPay-Network-Token",
		Metadata: map[string]any{
			"gateway":         "jiopay",
			"cryptogram_flow": "vault_api",
			"vault_id":        cryptoResp.Card.VaultID,
			"token":           fmt.Sprintf("%d", cryptoResp.Secure.Token),
			"token_expiry":    cryptoResp.Secure.TokenExpiry,
			"card_scheme":     mapJioPayCardScheme(cryptoResp.Card.Scheme),
			"masked_number":   cryptoResp.Card.MaskedNumber,
		},
	}

	fmt.Println("=== JIOPAY GATEWAY: GenerateCryptogram Complete ===")

	return response, nil
}

// InitiatePayment implements JioPay payment initiation
// API: POST https://uat.jiopay.co.in/pg/api/v2/initiateSale
// Supports token-based card payments with cryptogram
func (c *JioPayClient) InitiatePayment(ctx context.Context, request payment.GatewayPaymentRequest) (*payment.GatewayPaymentResponse, error) {
	fmt.Println("=== JIOPAY GATEWAY: InitiatePayment Called ===")
	fmt.Printf("📥 Business ID: %s\n", request.BusinessID)
	fmt.Printf("📥 Transaction ID: %s\n", request.TransactionID)
	fmt.Printf("💰 Amount: %d %s\n", request.Amount, request.Currency)
	fmt.Printf("🔀 Payment Method: %s\n", request.PaymentMethod)

	// Only support card payments for now (token-based)
	if request.PaymentMethod != payment.PaymentMethodCard {
		return nil, fmt.Errorf("unsupported payment method: %s (only CARD supported for now)", request.PaymentMethod)
	}

	// Verify token and cryptogram are provided
	if request.Token == "" || request.Cryptogram == "" {
		return nil, fmt.Errorf("token and cryptogram are required for token-based card payments")
	}

	fmt.Println("💳 Processing token-based card payment...")
	fmt.Printf("🎫 Token: %s...\n", request.Token[:10])
	fmt.Printf("🔐 Cryptogram: %s...\n", request.Cryptogram[:20])

	// Extract customer details from metadata
	customerEmail := extractCustomerEmail(request.Metadata)
	customerName := extractCustomerName(request.Metadata)
	customerPhone, _ := extractCustomerPhone(request.Metadata)

	// Format amount to rupees with decimals
	amountStr := formatAmountWithDecimals(request.Amount)

	// Format transaction date
	txnDate := formatJioPayTxnDate(time.Now())

	// Extract token reference ID from metadata (should be stored from tokenization)
	tokenRefID := ""
	if refID, ok := request.Metadata["token_reference"].(string); ok && refID != "" {
		tokenRefID = refID
	} else {
		// If not in metadata, use token itself as reference
		tokenRefID = request.Token
	}

	// Format card expiry to YYYYMM
	cardExpiry := ""
	if request.CardDetails != nil {
		cardExpiry = formatJioPayCardExpiry(request.CardDetails.ExpiryMonth, request.CardDetails.ExpiryYear)
	}

	// Get name on card
	nameOnCard := customerName
	if request.CardDetails != nil && request.CardDetails.HolderName != "" {
		nameOnCard = request.CardDetails.HolderName
	}

	// Build callback URL with gateway type parameter
	// Format: https://api.xcentro.com/api/v1/process_payment/callback?type=jiopay
	callbackURL := buildCallbackURL(request.CallbackURL, "jiopay")
	fmt.Printf("🔗 Callback URL: %s\n", callbackURL)

	// Build initiateSale request (without secureHash first)
	saleReq := InitiateSaleRequest{
		// Mandatory fields
		MerchantID:      c.merchantID,
		MerchantTxnNo:   request.TransactionID,
		Amount:          amountStr,
		CurrencyCode:    "356", // INR
		PayType:         "1",   // Direct/Merchant checkout
		CustomerEmailID: customerEmail,
		TransactionType: "SALE",
		ReturnURL:       callbackURL,
		TxnDate:         txnDate,

		// Payment method
		PaymentMode: "CARD",

		// Token card details
		CardNo:          request.Token,      // Network token
		CardExpiry:      cardExpiry,         // YYYYMM format
		NameOnCard:      nameOnCard,         // Name on card
		TokenCryptogram: request.Cryptogram, // Cryptogram from getCryptogram
		TokenRefID:      tokenRefID,         // Token reference ID

		// Optional customer details
		CustomerName:     customerName,
		CustomerMobileNo: fmt.Sprintf("%d", customerPhone),

		// Optional transaction details
		OrderDesc: request.Description,

		// Security - will be calculated below
		SecureHash: "", // Set after hash generation
	}

	// Generate secure hash for outgoing request
	// Use hash generator to calculate HMAC-SHA256 from all request fields
	fieldMap := buildInitiateSaleFieldMap(&saleReq)
	secureHash := c.hashGenerator.GenerateForRequest(fieldMap)
	saleReq.SecureHash = secureHash

	// Marshal to JSON
	jsonPayload, err := json.Marshal(saleReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal initiateSale request: %w", err)
	}

	fmt.Printf("📝 Request payload size: %d bytes\n", len(jsonPayload))

	// Make HTTP POST to initiateSale API
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/pg/api/v2/initiateSale", bytes.NewReader(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	fmt.Println("🚀 Calling JioPay initiateSale API...")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	fmt.Printf("📥 Response status: %d\n", resp.StatusCode)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var saleResp InitiateSaleResponse
	if err := json.Unmarshal(body, &saleResp); err != nil {
		return nil, fmt.Errorf("failed to parse initiateSale response: %w", err)
	}

	// Check response code
	if saleResp.ResponseCode != "R1000" {
		return nil, fmt.Errorf("payment initiation failed: %s - %s",
			saleResp.ResponseCode, saleResp.ResponseDescription)
	}

	fmt.Printf("✅ Payment initiated successfully\n")
	fmt.Printf("🌐 Redirect URI: %s\n", saleResp.RedirectURI)
	fmt.Printf("📋 Show OTP Capture: %s\n", saleResp.ShowOTPCapturePage)
	fmt.Printf("🔑 Transaction Context: %s\n", saleResp.TranCtx)

	// Map to gateway response
	response := &payment.GatewayPaymentResponse{
		PaymentID:        request.TransactionID,
		GatewayPaymentID: saleResp.MerchantTxnNo, // Use merchant txn no as gateway payment ID
		TransactionID:    request.TransactionID,
		Status:           payment.PaymentStatusPending, // Payment is pending 3DS/OTP
		Message:          "Payment initiated successfully",
		RedirectURL:      saleResp.RedirectURI,
		Amount:           request.Amount,
		Currency:         request.Currency,
		CreatedAt:        time.Now(),
		Metadata: map[string]any{
			"gateway":          "jiopay",
			"response_code":    saleResp.ResponseCode,
			"show_otp_capture": saleResp.ShowOTPCapturePage,
			"tran_ctx":         saleResp.TranCtx,
			"secure_hash":      saleResp.SecureHash,
			"oth_charge":       saleResp.OthCharge,
			"payment_method":   "card",
			"tokenized":        true,
		},
	}

	// Add OTP URIs if present
	if saleResp.GenerateOTPURI != nil {
		response.Metadata["generate_otp_uri"] = *saleResp.GenerateOTPURI
	}
	if saleResp.VerifyOTPURI != nil {
		response.Metadata["verify_otp_uri"] = *saleResp.VerifyOTPURI
	}
	if saleResp.AuthorizeURI != nil {
		response.Metadata["authorize_uri"] = *saleResp.AuthorizeURI
	}

	fmt.Println("=== JIOPAY GATEWAY: InitiatePayment Complete ===")

	return response, nil
}

// ValidateWebhook validates JioPay callback/webhook signatures
//
// JioPay uses HMAC-SHA256 for signature verification (same algorithm as payment initiation).
// The signature (secureHash) is calculated from all fields in the response (excluding secureHash itself).
//
// This method is used for both:
//   - Callback (SURL/FURL): Synchronous response after payment
//   - Webhook: Asynchronous settlement notification
func (c *JioPayClient) ValidateWebhook(payload []byte, signature string) error {
	fmt.Println("=== JIOPAY GATEWAY: ValidateWebhook Called ===")

	// Delegate to hash generator for validation
	return c.hashGenerator.ValidateFromResponse(payload, signature)
}
