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
	"xcentro.in/internal/shared/logger"
)

// JioPayClient implements the PaymentGateway interface for JioPay
type JioPayClient struct {
	baseURL             string      // Base URL for payment APIs
	tokenizationBaseURL string      // Vault API base URL
	merchantID          string      // JioPay merchant identifier
	apiKey              string      // API authentication key for HMAC
	appID               string      // x-app-id header for vault APIs
	encryptionKey       string                     // AES-128 encryption key for x-app-token
	isAggregatorModel   bool                       // Payment model: false = normal (merchant charges), true = aggregator (PG charges)
	httpClient          *http.Client
	hashGenerator       *SecureHashGenerator       // Hash generator for request/response signatures
	businessOwnerSvc    business.BusinessService   // Business service for fetching owner mobile (JioPay externalId requirement)
	logger              *logger.Logger             // Logger for debugging and monitoring
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
		logger:        logger.GetDefault(),
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

	// Extract business owner mobile from metadata (required for externalId)
	ownerMobile := extractOwnerMobile(request.Metadata)
	if ownerMobile == "" {
		return nil, fmt.Errorf("business owner mobile is required for JioPay tokenization")
	}

	// Construct externalId: entityId + ownerMobile (e.g., "100001000217424" + "8412819692")
	externalID := c.merchantID + ownerMobile

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


	// Generate mandatory headers for JioPay Vault API
	xTimestampID := generateXTimestampID()
	xTraceID := generateXTraceID()
	xAppToken, err := generateXAppToken(c.appID, xTraceID, xTimestampID, c.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to generate x-app-token: %w", err)
	}


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

	// Validate response - check status and presence of vaultId
	if tokenResp.Body.Status.Status != "SUCCESS" {
		return nil, fmt.Errorf("tokenization failed: status=%s", tokenResp.Body.Status.Status)
	}
	if tokenResp.Body.VaultID == 0 {
		return nil, fmt.Errorf("tokenization failed: empty vaultId in response")
	}
	if tokenResp.Body.TokenReference == "" {
		return nil, fmt.Errorf("tokenization failed: empty tokenReference in response")
	}

	// Convert vaultId to string for storage
	vaultIDStr := fmt.Sprintf("%d", tokenResp.Body.VaultID)

	c.logger.InfoWith("JioPay card tokenization successful",
		"vault_id", vaultIDStr,
		"token_reference", tokenResp.Body.TokenReference)

	// Map to gateway response
	// IMPORTANT: Save vaultId as the primary token since it's required for cryptogram generation
	// TokenReference is kept in metadata for reference
	response := &payment.GatewayTokenizeResponse{
		Token:         vaultIDStr,                     // PRIMARY: VaultID (needed for cryptogram)
		CardID:        vaultIDStr,                     // Use vaultId as CardID
		TransactionID: request.TransactionID,
		Status:        "success",
		Message:       "Card tokenized successfully",
		Metadata: map[string]any{
			"gateway":             "jiopay",
			"tokenization_flow":   "vault_api",
			"vault_id":            tokenResp.Body.VaultID,         // Store as number
			"vault_id_str":        vaultIDStr,                     // Store as string
			"token_reference":     tokenResp.Body.TokenReference,  // Keep for reference
			"status_code":         tokenResp.StatusCode,
			"status_code_value":   tokenResp.StatusCodeValue,
			"vault_id_available":  true, // vaultId now available immediately!
		},
	}


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

	// Extract vaultId and ownerMobile from metadata
	vaultID, ok := request.Metadata["vault_id"].(string)
	if !ok || vaultID == "" {
		return nil, fmt.Errorf("vault_id is required in metadata for JioPay cryptogram generation")
	}

	ownerMobile, ok := request.Metadata["owner_mobile"].(string)
	if !ok || ownerMobile == "" {
		return nil, fmt.Errorf("owner_mobile is required in metadata for JioPay cryptogram generation")
	}

	// Construct externalId: entityId + ownerMobile (e.g., "100001000217424" + "8412819692")
	externalID := c.merchantID + ownerMobile

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


	// Generate mandatory headers for JioPay Vault API
	xTimestampID := generateXTimestampID()
	xTraceID := generateXTraceID()
	xAppToken, err := generateXAppToken(c.appID, xTraceID, xTimestampID, c.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to generate x-app-token: %w", err)
	}


	// Make HTTP POST to cryptogram API with vaultId in path
	cryptogramURL := fmt.Sprintf("%s/getCryptogram/%s", c.tokenizationBaseURL, vaultID)

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


	// Parse token expiry (MMYYYY format)
	expiresAt := time.Now().Add(10 * time.Minute) // Default 10 minutes
	if cryptoResp.Secure.TokenExpiry != "" {
		// TokenExpiry is in MMYYYY format (e.g., "062027")
		// Convert to time.Time for expiry calculation
		// For simplicity, assume 10-minute validity window
	}

	// Map to gateway response
	// IMPORTANT: Network token (secure.token) must be used as CardNo in initiateSale
	response := &payment.CryptogramResponse{
		Cryptogram:   cryptoResp.Secure.Cryptogram,
		ExpiresAt:    expiresAt,
		ValidityMins: 10, // JioPay cryptogram validity
		Algorithm:    "JioPay-Network-Token",
		Metadata: map[string]any{
			"gateway":         "jiopay",
			"cryptogram_flow": "vault_api",
			"vault_id":        cryptoResp.Card.VaultID,
			"network_token":   cryptoResp.Secure.Token,       // CRITICAL: Use this as CardNo in initiateSale
			"token_expiry":    cryptoResp.Secure.TokenExpiry,
			"card_scheme":     mapJioPayCardScheme(cryptoResp.Card.Scheme),
			"masked_number":   cryptoResp.Card.MaskedNumber,
			"token_reference": cryptoResp.Card.TokenReference, // For reference
		},
	}

	c.logger.InfoWith("JioPay cryptogram generated successfully",
		"vault_id", cryptoResp.Card.VaultID,
		"network_token_length", len(cryptoResp.Secure.Token))

	return response, nil
}

// InitiatePayment implements JioPay payment initiation
// API: POST https://uat.jiopay.co.in/pg/api/v2/initiateSale
// Supports:
// 1. Token-based card payments (with cryptogram)
// 2. Raw card payments for tokenization (with saveCardIndicator)
func (c *JioPayClient) InitiatePayment(ctx context.Context, request payment.GatewayPaymentRequest) (*payment.GatewayPaymentResponse, error) {

	// Only support card payments for now
	if request.PaymentMethod != payment.PaymentMethodCard {
		return nil, fmt.Errorf("unsupported payment method: %s (only CARD supported for now)", request.PaymentMethod)
	}

	// Determine if this is tokenization or token-based payment
	isTokenization := request.PaymentType == payment.PaymentTypeTokenization
	isTokenBased := request.Token != "" // Token-based payment uses stored card

	if isTokenization {
		// For tokenization, we need raw card details
		if request.CardDetails == nil {
			return nil, fmt.Errorf("card details are required for tokenization payments")
		}
	} else if isTokenBased {
		// JioPay-specific: Always generate cryptogram internally
		// This encapsulates JioPay's complex cryptogram generation flow
		// Cryptogram is a PG-specific implementation detail, not exposed to service layer

		// Step 1: Get owner mobile using business service
		if c.businessOwnerSvc == nil {
			return nil, fmt.Errorf("business service not configured for JioPay")
		}

		ownerMobile, err := c.businessOwnerSvc.GetOwnerMobile(ctx, request.BusinessID)
		if err != nil {
			return nil, fmt.Errorf("failed to get owner mobile: %w", err)
		}

		// Step 2: Use request.Token directly as vaultId (since tokenization now returns vaultId)
		// Previously, we had to call FetchCardByLast4Digits to get vaultId
		// Now, the vaultId is stored directly in stored_cards.token column
		vaultID := request.Token

		// Step 3: Generate cryptogram internally
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

		// Extract cryptogram and network token from response
		request.Cryptogram = cryptogramResp.Cryptogram

		// CRITICAL: Extract network token from metadata for use in initiateSale
		networkToken, ok := cryptogramResp.Metadata["network_token"].(string)
		if !ok || networkToken == "" {
			return nil, fmt.Errorf("network_token not found in cryptogram response")
		}

		// Store network token in request metadata for use in sale request
		if request.Metadata == nil {
			request.Metadata = make(map[string]any)
		}
		request.Metadata["network_token"] = networkToken
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
		// TOKENIZATION FLOW: Use raw card details
		saleReq.CardNo = request.CardDetails.Number // PCI DSS: Sent directly to gateway, never stored
		saleReq.CardExpiry = cardExpiry             // YYYYMM format
		saleReq.NameOnCard = nameOnCard
		saleReq.CVV = request.CardDetails.CVV                                          // PCI DSS: Transient only, never stored
		saleReq.PaymentOptionCodes = determinePaymentOptionCode(request.CardDetails.CardCategory) // "CC" for credit, "DC" for debit
	} else {
		// TOKEN-BASED FLOW: Use network token + cryptogram

		// CRITICAL: Use network token (from cryptogram response) as CardNo, NOT vaultId
		// vaultId is stored in request.Token, but we need the network token for payment
		networkToken, ok := request.Metadata["network_token"].(string)
		if !ok || networkToken == "" {
			return nil, fmt.Errorf("network_token not found in request metadata - cryptogram may not have been generated")
		}

		saleReq.CardNo = networkToken                // Network token from cryptogram (NOT vaultId)
		saleReq.CardExpiry = cardExpiry              // YYYYMM format
		saleReq.NameOnCard = nameOnCard              // Name on card
		saleReq.TokenCryptogram = request.Cryptogram // Cryptogram from getCryptogram
		saleReq.TokenRefID = tokenRefID              // Token reference ID
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


	// Make HTTP POST to initiateSale API
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v2/initiateSale", bytes.NewReader(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")


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
	var saleResp InitiateSaleResponse
	if err := json.Unmarshal(body, &saleResp); err != nil {
		return nil, fmt.Errorf("failed to parse initiateSale response: %w", err)
	}

	// Check response code
	if saleResp.ResponseCode != "R1000" {
		return nil, fmt.Errorf("payment initiation failed: %s - %s",
			saleResp.ResponseCode, saleResp.ResponseDescription)
	}

	// Build complete redirect URL with tranCtx parameter
	// Format: https://uat.jiopay.co.in/tsp/pg/api/v2/authRedirect?tranCtx=R1ed14028-...
	redirectURL := fmt.Sprintf("%s?tranCtx=%s", saleResp.RedirectURI, saleResp.TranCtx)

	// Map to gateway response
	response := &payment.GatewayPaymentResponse{
		PaymentID:        request.TransactionID,
		GatewayPaymentID: "", // Gateway txnID not available yet, will be set in callback
		TransactionID:    request.TransactionID,
		Status:           payment.PaymentStatusPending, // Payment is pending 3DS/OTP
		Message:          "Payment initiated successfully",
		RedirectURL:      redirectURL, // Complete URL with tranCtx parameter
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

	c.logger.InfoWith("JioPay payment initiated successfully",
		"payment_id", request.TransactionID,
		"redirect_url", redirectURL,
		"response_code", saleResp.ResponseCode)

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

	// Construct externalId: entityId + ownerMobile (e.g., "100001000217424" + "8412819692")
	externalID := c.merchantID + ownerMobile

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


	// Generate mandatory headers for JioPay Vault API
	xTimestampID := generateXTimestampID()
	xTraceID := generateXTraceID()
	xAppToken, err := generateXAppToken(c.appID, xTraceID, xTimestampID, c.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to generate x-app-token: %w", err)
	}


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


	// Match by last 4 digits
	// IMPORTANT: User specified "match the last four digit forthis particular business"
	// We're doing the business scoping at the service layer (by passing businessID context)
	for _, card := range fetchResp.Card {
		// Extract last 4 digits from masked number (e.g., "XXXXXXXXXXXX0055" -> "0055")
		if len(card.MaskedNumber) < 4 {
			continue // Skip invalid masked numbers
		}
		cardLast4 := card.MaskedNumber[len(card.MaskedNumber)-4:]


		// Match by last 4 digits AND ensure card is ACTIVE
		if cardLast4 == last4Digits && card.Status == "ACTIVE" {
			return card.VaultID, nil
		}
	}

	// No matching card found
	return "", fmt.Errorf("no active card found with last 4 digits: %s for business: %s", last4Digits, businessID)
}
