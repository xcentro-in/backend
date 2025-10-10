package easebuzz

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"xcentro.in/internal/core/payment"
)

// EasebuzzClient implements the PaymentGateway interface for Easebuzz
type EasebuzzClient struct {
	baseURL     string
	merchantKey string
	salt        string
	httpClient  *http.Client
}

// NewEasebuzzClient creates a new Easebuzz payment gateway client
//
// Parameters:
//   - baseURL: Easebuzz API base URL (e.g., "https://pay.easebuzz.in")
//   - merchantKey: EASEBUZZ_MERCHANT_KEY from environment
//   - salt: EASEBUZZ_SALT from environment
//
// Usage:
//
//	client := NewEasebuzzClient(
//	    cfg.Easebuzz.BaseURL,
//	    cfg.Easebuzz.MerchantKey,
//	    cfg.Easebuzz.Salt,
//	)
func NewEasebuzzClient(baseURL, merchantKey, salt string) payment.PaymentGateway {
	return &EasebuzzClient{
		baseURL:     baseURL,
		merchantKey: merchantKey,
		salt:        salt,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetGatewayType returns the gateway type identifier
func (c *EasebuzzClient) GetGatewayType() payment.GatewayType {
	return payment.GatewayEasebuzz
}

// TokenizeCard implements Easebuzz-specific tokenization flow
// Easebuzz Flow: Direct tokenize API with implicit ₹1 → credit + tokenize
//
// TODO: Implement actual Easebuzz tokenization API
// For now, returns mock response similar to the old mock gateway
func (c *EasebuzzClient) TokenizeCard(ctx context.Context, request payment.GatewayTokenizeRequest) (*payment.GatewayTokenizeResponse, error) {
	fmt.Println("=== EASEBUZZ GATEWAY: Direct Tokenization ===")
	fmt.Printf("📥 Business ID: %s\n", request.BusinessID)
	fmt.Printf("📥 Transaction ID: %s\n", request.TransactionID)
	fmt.Printf("💳 Card: %s %s ending in %s\n",
		request.CardDetails.CardCategory, request.CardDetails.CardType, request.CardDetails.LastFourDigits)

	// TODO: Implement actual Easebuzz tokenization API call
	// For now, return mock response
	fmt.Println("⚠️  WARNING: Using mock tokenization (implement real API)")

	response := &payment.GatewayTokenizeResponse{
		Token:         fmt.Sprintf("eb_tok_%s_%d", request.CardDetails.CardType, time.Now().Unix()),
		CardID:        fmt.Sprintf("card_%d", time.Now().Unix()),
		TransactionID: request.TransactionID,
		Status:        "success",
		Message:       "Card tokenized successfully with Easebuzz (MOCK)",
		Metadata: map[string]any{
			"gateway":           "easebuzz",
			"tokenization_flow": "direct",
			"mock_data":         true,
		},
	}

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
		if req.Token != "" && req.Cryptogram != "" {
			// Token-based payment with ENCRYPTION
			fmt.Println("💳 Processing token-based card payment with encryption...")
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
// TODO: Implement actual Easebuzz cryptogram generation
// For now, returns mock response
func (c *EasebuzzClient) GenerateCryptogram(ctx context.Context, request payment.CryptogramRequest) (*payment.CryptogramResponse, error) {
	fmt.Println("=== EASEBUZZ GATEWAY: GenerateCryptogram Called ===")
	fmt.Printf("🔐 Token: %s\n", request.Token)
	fmt.Printf("💰 Amount: %d %s\n", request.Amount, request.Currency)

	// TODO: Implement actual Easebuzz cryptogram generation
	fmt.Println("⚠️  WARNING: Using mock cryptogram (implement real API)")

	response := &payment.CryptogramResponse{
		Cryptogram:   fmt.Sprintf("eb_crypto_%s_%d", request.Token[:8], time.Now().Unix()),
		ExpiresAt:    time.Now().Add(15 * time.Minute),
		ValidityMins: 15,
		Algorithm:    "Easebuzz-HMAC-SHA256",
		Metadata: map[string]any{
			"gateway":   "easebuzz",
			"mock_data": true,
		},
	}

	return response, nil
}

// ValidateWebhook validates Easebuzz webhook signatures
//
// TODO: Implement actual Easebuzz webhook signature validation
func (c *EasebuzzClient) ValidateWebhook(payload []byte, signature string) error {
	fmt.Println("=== EASEBUZZ GATEWAY: ValidateWebhook Called ===")
	fmt.Printf("📝 Payload length: %d bytes\n", len(payload))
	fmt.Printf("🔏 Signature: %s\n", signature)

	// TODO: Implement actual Easebuzz webhook signature validation
	// Use generateReverseHash() to verify webhook authenticity
	fmt.Println("⚠️  WARNING: Webhook validation not implemented (implement reverse hash verification)")

	return nil
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

	// Build hash sequence (with empty UDFs as per Easebuzz requirements)
	// Format: key|txnid|amount|productinfo|firstname|email|||||||||||salt
	amountRupees := fmt.Sprintf("%.2f", float64(req.Amount)/100.0)
	hashSequence := fmt.Sprintf("%s|%s|%s|%s|%s|%s|||||||||||%s",
		c.merchantKey,
		req.TransactionID,
		amountRupees,
		req.Description,
		firstname,
		email,
		c.salt,
	)
	hash := generateHash(hashSequence)

	// Build form data
	formData := url.Values{
		"key":          {c.merchantKey},
		"txnid":        {req.TransactionID},
		"amount":       {amountRupees},
		"productinfo":  {req.Description},
		"firstname":    {firstname},
		"phone":        {phone},
		"email":        {email},
		"surl":         {req.CallbackURL + "/success"},
		"furl":         {req.CallbackURL + "/failure"},
		"hash":         {hash},
		"request_flow": {"SEAMLESS"}, // Critical for seamless flow
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

	// ENCRYPT sensitive fields using Easebuzz-specific encryption
	encryptedToken, err := EncryptCardField(req.Token, c.merchantKey, c.salt)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt token: %w", err)
	}
	fmt.Printf("✅ Token encrypted: %s...\n", encryptedToken[:20])

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
		"card_token":         {encryptedToken},      // ENCRYPTED
		"cryptogram":         {encryptedCrypto},     // ENCRYPTED
		"token_expiry_date":  {encryptedExpiry},     // ENCRYPTED
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
			"gateway":         "easebuzz",
			"easepayid":       paymentResp.Data.EasepayID,
			"bank_ref_num":    paymentResp.Data.BankRefNum,
			"payment_source":  paymentResp.Data.PaymentSource,
			"card_category":   paymentResp.Data.CardCategory,
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
