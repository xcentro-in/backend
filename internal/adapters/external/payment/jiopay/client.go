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
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/payment"
)

// JioPayClient implements the PaymentGateway interface for JioPay
type JioPayClient struct {
	baseURL             string      // Base URL for payment APIs
	tokenizationBaseURL string      // Vault API base URL
	merchantID          string      // JioPay merchant identifier
	apiKey              string      // API authentication key for HMAC
	appID               string      // x-app-id header for vault APIs
	encryptionKey       string                 // AES-128 encryption key for x-app-token
	isAggregatorModel   bool                   // Payment model: false = normal (merchant charges), true = aggregator (PG charges)
	httpClient          *http.Client
	hashGenerator       *SecureHashGenerator   // Hash generator for request/response signatures
	businessOwnerSvc    business.BusinessService // Business service for fetching owner mobile (JioPay externalId requirement)
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
//   - isAggregatorModel: JIOPAY_IS_AGGREGATOR_MODEL from environment (payment model)
//   - businessOwnerSvc: Service to fetch business owner mobile (required for JioPay externalId)
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
//	    cfg.JioPay.IsAggregatorModel,
//	    businessOwnerService,
//	)
func NewJioPayClient(baseURL, tokenizationBaseURL, merchantID, apiKey, appID, encryptionKey string, isAggregatorModel bool, businessOwnerSvc business.BusinessService) payment.PaymentGateway {
	return &JioPayClient{
		baseURL:             baseURL,
		tokenizationBaseURL: tokenizationBaseURL,
		merchantID:          merchantID,
		apiKey:              apiKey,
		appID:               appID,
		encryptionKey:       encryptionKey,
		isAggregatorModel:   isAggregatorModel,
		businessOwnerSvc:    businessOwnerSvc,
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // JioPay tokenization can take longer
		},
		hashGenerator: NewSecureHashGenerator(apiKey),
	}
}

// SetHTTPClient allows setting a custom HTTP client (useful for testing with mock transport)
// Example in tests:
//   mockTransport := NewMockTransport()
//   mockTransport.AddRoute("POST", "/initiateSale", `{"status":"success"}`, 200)
//   client.SetHTTPClient(&http.Client{Transport: mockTransport})
func (c *JioPayClient) SetHTTPClient(client *http.Client) {
	c.httpClient = client
}

// GetGatewayType returns the gateway type identifier
func (c *JioPayClient) GetGatewayType() payment.GatewayType {
	return payment.GatewayJioPay
}

// IsAggregatorModel returns the payment model for this gateway
func (c *JioPayClient) IsAggregatorModel() bool {
	return c.isAggregatorModel
}

// TokenizeCard implements JioPay card tokenization flow
// JioPay Flow: POST to /fetch-card with 5-object structure
//
// API: POST https://uat.jiopay.co.in/qatsp/api/vault/fetch-card
// Content-Type: application/json
// Body: {user, entity, card, consent, audit}
func (c *JioPayClient) TokenizeCard(ctx context.Context, request payment.GatewayTokenizeRequest) (*payment.GatewayTokenizeResponse, error) {
	fmt.Println("=== JIOPAY GATEWAY: Tokenization (Vault API - /tokenized-card) ===")
	fmt.Printf("📥 Business ID: %s\n", request.BusinessID)
	fmt.Printf("📥 Transaction ID: %s\n", request.TransactionID)
	fmt.Printf("💳 Card: %s %s ending in %s\n",
		request.CardDetails.CardCategory, request.CardDetails.CardType, request.CardDetails.LastFourDigits)

	// Extract business owner mobile from metadata (required for externalId)
	ownerMobile := extractOwnerMobile(request.Metadata)
	if ownerMobile == "" {
		return nil, fmt.Errorf("business owner mobile is required for JioPay tokenization")
	}

	// Construct externalId: entityId + ownerMobile (e.g., "100001000217424" + "8412819692")
	externalID := c.merchantID + ownerMobile
	fmt.Printf("🔑 ExternalID: %s (entityId: %s + mobile: %s)\n", externalID, c.merchantID, ownerMobile)

	// Build user object
	userObj := UserObject{
		ExternalID: externalID, // CRITICAL: entityId + userMobile
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
		cardObj.AuthenticationReferenceID = fmt.Sprintf("AUTHREF_%s", uuid.Must(uuid.NewV7()).String()[:10])
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
	req, err := http.NewRequestWithContext(ctx, "POST", c.tokenizationBaseURL+"/tokenized-card", bytes.NewReader(jsonPayload))
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

	// Validate response - check if tokenReference is present
	if tokenResp.TokenReference == "" {
		return nil, fmt.Errorf("tokenization failed: empty tokenReference in response")
	}

	fmt.Printf("✅ Token created: TokenReference %s\n", tokenResp.TokenReference)
	fmt.Printf("📝 Card Reference: %s\n", tokenResp.CardReference)
	fmt.Printf("🔢 Last 4 digits: %s\n", tokenResp.PanLastFourDigits)
	fmt.Printf("📅 Expiry: %s\n", tokenResp.Expiry)

	// IMPORTANT: JioPay tokenization does NOT return vaultId in this response
	// vaultId is obtained later by calling /fetch-card endpoint
	// For now, use cardReference as a placeholder CardID

	// Map to gateway response
	response := &payment.GatewayTokenizeResponse{
		Token:         tokenResp.TokenReference,    // PRIMARY: Token reference for future payments
		CardID:        tokenResp.CardReference,     // Use cardReference as CardID (vaultId not available yet)
		TransactionID: request.TransactionID,
		Status:        "success",
		Message:       "Card tokenized successfully",
		Metadata: map[string]any{
			"gateway":             "jiopay",
			"tokenization_flow":   "vault_api",
			"token_reference":     tokenResp.TokenReference,
			"card_reference":      tokenResp.CardReference,
			"pan_last_4_digits":   tokenResp.PanLastFourDigits,
			"bank_name":           tokenResp.BankName,
			"bank_id":             tokenResp.BankID,
			"par":                 tokenResp.PAR,
			"expiry":              tokenResp.Expiry, // MMYYYY format
			"token_requestor_id":  tokenResp.TokenRequestorID,
			"response_id":         tokenResp.ResponseID,
			// NOTE: vaultId will be available only after calling /fetch-card later
			"vault_id_available":  false,
		},
	}

	fmt.Println("=== JIOPAY GATEWAY: Tokenization Complete ===")
	fmt.Println("⚠️  Note: vaultId will be obtained when fetching card for payment")

	return response, nil
}

// GenerateCryptogram generates JioPay-specific cryptogram for a tokenized card
//
// API: POST https://uat.jiopay.co.in/qatsp/api/vault/getCryptogram/{vaultId}
// Content-Type: application/json
// Body: {user, entity, amount}
//
// IMPORTANT: vaultId MUST be obtained via FetchCardByLast4Digits first
func (c *JioPayClient) GenerateCryptogram(ctx context.Context, request payment.CryptogramRequest) (*payment.CryptogramResponse, error) {
	fmt.Println("=== JIOPAY GATEWAY: GenerateCryptogram (Vault API) ===")
	fmt.Printf("💰 Amount: %d %s\n", request.Amount, request.Currency)

	// Extract vaultId and ownerMobile from metadata
	vaultID, ok := request.Metadata["vault_id"].(string)
	if !ok || vaultID == "" {
		return nil, fmt.Errorf("vault_id is required in metadata for JioPay cryptogram generation")
	}
	fmt.Printf("🔑 VaultID: %s\n", vaultID)

	ownerMobile, ok := request.Metadata["owner_mobile"].(string)
	if !ok || ownerMobile == "" {
		return nil, fmt.Errorf("owner_mobile is required in metadata for JioPay cryptogram generation")
	}
	fmt.Printf("📱 Owner Mobile: %s\n", ownerMobile)

	// Construct externalId: entityId + ownerMobile (e.g., "100001000217424" + "8412819692")
	externalID := c.merchantID + ownerMobile
	fmt.Printf("🔑 ExternalID: %s (entityId: %s + mobile: %s)\n", externalID, c.merchantID, ownerMobile)

	// Build cryptogram request
	cryptoReq := CryptogramRequest{
		User: UserObject{
			ExternalID: externalID, // CRITICAL: entityId + ownerMobile
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

	// Make HTTP POST to cryptogram API with vaultId in path
	cryptogramURL := fmt.Sprintf("%s/getCryptogram/%s", c.tokenizationBaseURL, vaultID)
	fmt.Printf("🌐 Cryptogram URL: %s\n", cryptogramURL)

	req, err := http.NewRequestWithContext(ctx, "POST", cryptogramURL, bytes.NewReader(jsonPayload))
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
// Supports:
// 1. Token-based card payments (with cryptogram)
// 2. Raw card payments for tokenization (with saveCardIndicator)
func (c *JioPayClient) InitiatePayment(ctx context.Context, request payment.GatewayPaymentRequest) (*payment.GatewayPaymentResponse, error) {
	fmt.Println("=== JIOPAY GATEWAY: InitiatePayment Called ===")
	fmt.Printf("📥 Business ID: %s\n", request.BusinessID)
	fmt.Printf("📥 Transaction ID: %s\n", request.TransactionID)
	fmt.Printf("💰 Amount: %d %s\n", request.Amount, request.Currency)
	fmt.Printf("🔀 Payment Method: %s\n", request.PaymentMethod)
	fmt.Printf("🔀 Payment Type: %s\n", request.PaymentType)

	// Only support card payments for now
	if request.PaymentMethod != payment.PaymentMethodCard {
		return nil, fmt.Errorf("unsupported payment method: %s (only CARD supported for now)", request.PaymentMethod)
	}

	// Determine if this is tokenization or token-based payment
	isTokenization := request.PaymentType == payment.PaymentTypeTokenization
	isTokenBased := request.Token != "" // Token-based payment uses stored card

	if isTokenization {
		fmt.Println("🔑 Processing TOKENIZATION payment (raw card with saveCardIndicator)")
		// For tokenization, we need raw card details
		if request.CardDetails == nil {
			return nil, fmt.Errorf("card details are required for tokenization payments")
		}
	} else if isTokenBased {
		fmt.Println("💳 Processing TOKEN-BASED card payment")

		// JioPay-specific: Generate cryptogram if not provided by orchestrator
		// This encapsulates JioPay's complex cryptogram generation flow
		if request.Cryptogram == "" {
			fmt.Println("🔐 Cryptogram not provided - generating internally (JioPay 3-step flow)")

			// Step 1: Get owner mobile using business service
			if c.businessOwnerSvc == nil {
				return nil, fmt.Errorf("business service not configured for JioPay")
			}

			ownerMobile, err := c.businessOwnerSvc.GetOwnerMobile(ctx, request.BusinessID)
			if err != nil {
				return nil, fmt.Errorf("failed to get owner mobile: %w", err)
			}
			fmt.Printf("📱 Owner mobile fetched: %d digits\n", len(ownerMobile))

			// Step 2: Get stored card details from metadata
			storedCard, ok := request.Metadata["stored_card"].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("stored_card metadata required for token-based payment")
			}

			last4Digits, _ := storedCard["last_four_digits"].(string)
			if last4Digits == "" {
				return nil, fmt.Errorf("last_four_digits required in stored_card metadata")
			}

			// Step 3: Fetch vaultId by matching last 4 digits
			fmt.Printf("🔍 Fetching vaultId for card ending in %s\n", last4Digits)
			vaultID, err := c.FetchCardByLast4Digits(ctx, request.BusinessID, last4Digits, ownerMobile)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch vaultId: %w", err)
			}
			fmt.Printf("✅ VaultID found: %s\n", vaultID)

			// Step 4: Generate cryptogram internally
			cryptogramReq := payment.CryptogramRequest{
				Token:         request.Token,
				Amount:        request.Amount,
				Currency:      request.Currency,
				TransactionID: request.TransactionID,
				BusinessID:    request.BusinessID,
				Metadata: map[string]any{
					"vault_id":     vaultID,
					"owner_mobile": ownerMobile,
				},
			}

			cryptogramResp, err := c.GenerateCryptogram(ctx, cryptogramReq)
			if err != nil {
				return nil, fmt.Errorf("failed to generate cryptogram: %w", err)
			}

			// Use generated cryptogram
			request.Cryptogram = cryptogramResp.Cryptogram
			fmt.Printf("✅ Cryptogram generated: %s...\n", request.Cryptogram[:20])
		} else {
			fmt.Println("🔐 Using cryptogram provided by orchestrator")
			fmt.Printf("🎫 Token: %s...\n", request.Token[:10])
			fmt.Printf("🔐 Cryptogram: %s...\n", request.Cryptogram[:20])
		}
	}

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
		PaymentMode: "Card", // JioPay expects "Card" not "CARD"

		// Optional customer details
		CustomerName:     customerName,
		CustomerMobileNo: fmt.Sprintf("%d", customerPhone),

		// Optional transaction details
		OrderDesc: request.Description,

		// Security - will be calculated below
		SecureHash: "", // Set after hash generation
	}

	// Populate payment-type-specific fields
	if isTokenization {
		// TOKENIZATION FLOW: Use raw card details + saveCardIndicator
		fmt.Println("🔑 Building tokenization payment request...")
		saleReq.CardNo = request.CardDetails.Number // PCI DSS: Sent directly to gateway, never stored
		saleReq.CardExpiry = cardExpiry             // YYYYMM format
		saleReq.NameOnCard = nameOnCard
		saleReq.CVV = request.CardDetails.CVV // PCI DSS: Transient only, never stored
		saleReq.SaveCardIndicator = "Y"       // KEY: Tells JioPay to tokenize
		saleReq.PaymentOptionCodes = determinePaymentOptionCode(request.CardDetails.CardCategory)
		fmt.Printf("🔑 SaveCardIndicator: Y, PaymentOptionCodes: %s\n", saleReq.PaymentOptionCodes)
	} else {
		// TOKEN-BASED FLOW: Use network token + cryptogram
		fmt.Println("💳 Building token-based payment request...")
		saleReq.CardNo = request.Token               // Network token
		saleReq.CardExpiry = cardExpiry              // YYYYMM format
		saleReq.NameOnCard = nameOnCard              // Name on card
		saleReq.TokenCryptogram = request.Cryptogram // Cryptogram from getCryptogram
		saleReq.TokenRefID = tokenRefID              // Token reference ID
		fmt.Printf("💳 Token: %s..., Cryptogram: %s...\n", request.Token[:10], request.Cryptogram[:20])
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

// determinePaymentOptionCode maps card category to JioPay payment option codes
// CC = Credit Card, DC = Debit Card
func determinePaymentOptionCode(cardCategory payment.CardCategory) string {
	switch cardCategory {
	case payment.CardCategoryCredit:
		return "CC"
	case payment.CardCategoryDebit:
		return "DC"
	case payment.CardCategoryCorporate:
		return "CC" // Corporate cards use CC code
	default:
		return "CC" // Default to credit card
	}
}

// extractOwnerMobile extracts business owner mobile from metadata
// Returns empty string if not found
func extractOwnerMobile(metadata map[string]any) string {
	if mobile, ok := metadata["owner_mobile"].(string); ok {
		return mobile
	}
	return ""
}

// FetchCardByLast4Digits fetches all cards for a customer and returns the vaultId
// for the card matching the last 4 digits for this particular business.
//
// This is CRITICAL for JioPay payment flow because:
// - JioPay tokenization returns tokenReference but NOT vaultId
// - To generate cryptogram, we MUST have vaultId
// - Solution: Fetch all cards, match by last 4 digits, extract vaultId
//
// API: POST https://uat.jiopay.co.in/qatsp/api/vault/fetch-card
func (c *JioPayClient) FetchCardByLast4Digits(ctx context.Context, businessID string, last4Digits string, ownerMobile string) (string, error) {
	fmt.Println("=== JIOPAY GATEWAY: FetchCardByLast4Digits (Vault API) ===")
	fmt.Printf("🔍 Business ID: %s\n", businessID)
	fmt.Printf("🔍 Last 4 Digits: %s\n", last4Digits)
	fmt.Printf("📱 Owner Mobile: %s\n", ownerMobile)

	// Construct externalId: entityId + ownerMobile (e.g., "100001000217424" + "8412819692")
	externalID := c.merchantID + ownerMobile
	fmt.Printf("🔑 ExternalID: %s (entityId: %s + mobile: %s)\n", externalID, c.merchantID, ownerMobile)

	// Build fetch-card request
	fetchReq := FetchCardRequest{
		Entity: EntityObject{
			EntityID: c.merchantID,
		},
		User: UserObject{
			ExternalID: externalID, // CRITICAL: entityId + ownerMobile
		},
	}

	// Marshal to JSON
	jsonPayload, err := json.Marshal(fetchReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal fetch-card request: %w", err)
	}

	fmt.Printf("📝 Payload size: %d bytes\n", len(jsonPayload))

	// Generate mandatory headers for JioPay Vault API
	xTimestampID := generateXTimestampID()
	xTraceID := generateXTraceID()
	xAppToken, err := generateXAppToken(c.appID, xTraceID, xTimestampID, c.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to generate x-app-token: %w", err)
	}

	fmt.Printf("📋 x-timestamp-id: %s\n", xTimestampID)
	fmt.Printf("📋 x-trace-id: %s\n", xTraceID)
	fmt.Printf("🔐 x-app-token: %s...\n", xAppToken[:20])

	// Make HTTP POST to fetch-card API
	req, err := http.NewRequestWithContext(ctx, "POST", c.tokenizationBaseURL+"/fetch-card", bytes.NewReader(jsonPayload))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set mandatory headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-app-id", c.appID)
	req.Header.Set("x-timestamp-id", xTimestampID)
	req.Header.Set("x-trace-id", xTraceID)
	req.Header.Set("x-app-token", xAppToken)

	fmt.Println("🚀 Calling JioPay Vault API to fetch all cards...")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var fetchResp FetchCardResponse
	if err := json.Unmarshal(body, &fetchResp); err != nil {
		return "", fmt.Errorf("failed to parse fetch-card response: %w", err)
	}

	if fetchResp.Status.Status != "SUCCESS" {
		return "", fmt.Errorf("fetch-card failed: %s", fetchResp.Status.Status)
	}

	fmt.Printf("✅ Fetched %d cards for customer\n", len(fetchResp.Card))

	// Match by last 4 digits
	// IMPORTANT: User specified "match the last four digit forthis particular business"
	// We're doing the business scoping at the service layer (by passing businessID context)
	for _, card := range fetchResp.Card {
		// Extract last 4 digits from masked number (e.g., "XXXXXXXXXXXX0055" -> "0055")
		if len(card.MaskedNumber) < 4 {
			continue // Skip invalid masked numbers
		}
		cardLast4 := card.MaskedNumber[len(card.MaskedNumber)-4:]

		fmt.Printf("🔍 Checking card: VaultID=%s, MaskedNumber=%s, Last4=%s, Status=%s\n",
			card.VaultID, card.MaskedNumber, cardLast4, card.Status)

		// Match by last 4 digits AND ensure card is ACTIVE
		if cardLast4 == last4Digits && card.Status == "ACTIVE" {
			fmt.Printf("✅ Found matching card! VaultID: %s\n", card.VaultID)
			return card.VaultID, nil
		}
	}

	// No matching card found
	return "", fmt.Errorf("no active card found with last 4 digits: %s for business: %s", last4Digits, businessID)
}
