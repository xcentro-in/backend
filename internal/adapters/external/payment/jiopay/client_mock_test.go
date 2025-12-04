package jiopay_test

import (
	"context"
	"net/http"
	"testing"

	"xcentro.in/internal/adapters/external/payment/jiopay"
	"xcentro.in/internal/core/payment"
)

// TestJioPayClientWithMock_RegularPayment tests regular card payment flow with mocked HTTP responses
func TestJioPayClientWithMock_RegularPayment(t *testing.T) {
	// Create mock transport
	mockTransport := jiopay.NewMockTransport()

	// Mock initiateSale endpoint
	mockTransport.AddRoute("POST", "/initiateSale", `{
		"responseCode": "R1000",
		"responseDescription": "Transaction initiated successfully",
		"merchantId": "merchant-id",
		"merchantTxnNo": "mock-transaction-123",
		"redirectURI": "https://jiopay.example.com/redirect?token=abc123",
		"showOTPCapturePage": "N",
		"tranCtx": "mock-tran-context",
		"secureHash": "mock-secure-hash",
		"oth_charge": 0.0
	}`, 200)

	// Create JioPay client with mock transport
	client := jiopay.NewJioPayClient(
		"https://api.jiopay.test",
		"https://vault.jiopay.test",
		"merchant-id",
		"api-key",
		"app-id",
		"1234567890123456", // 16-byte encryption key for AES
		false,              // isAggregatorModel
		&MockBusinessService{},
	)

	jioClient := client.(*jiopay.JioPayClient)
	jioClient.SetHTTPClient(&http.Client{Transport: mockTransport})

	// Test payment request with raw card details (for tokenization)
	ctx := context.Background()
	request := payment.GatewayPaymentRequest{
		BusinessID:    "business-123",
		TransactionID: "txn-456",
		Amount:        50000, // ₹500.00
		Currency:      payment.CurrencyINR,
		PaymentMethod: payment.PaymentMethodCard,
		PaymentType:   payment.PaymentTypeTokenization, // Use tokenization flow for new card
		Description:   "Test payment",
		CallbackURL:   "https://callback.example.com",
		CardDetails: &payment.Card{
			Number:       "4111111111111111",
			ExpiryMonth:  "12",
			ExpiryYear:   "2025",
			CVV:          "123",
			CardType:     payment.CardTypeVisa,
			CardCategory: payment.CardCategoryCredit,
		},
	}

	response, err := client.InitiatePayment(ctx, request)

	// Assertions
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if response.TransactionID != "txn-456" {
		t.Errorf("Expected TransactionID 'txn-456', got '%s'", response.TransactionID)
	}

	if response.GatewayPaymentID != "mock-transaction-123" {
		t.Errorf("Expected GatewayPaymentID 'mock-transaction-123', got '%s'", response.GatewayPaymentID)
	}

	if response.Status != payment.PaymentStatusPending {
		t.Errorf("Expected status 'pending', got '%s'", response.Status)
	}

	if response.RedirectURL != "https://jiopay.example.com/redirect?token=abc123" {
		t.Errorf("Expected RedirectURL 'https://jiopay.example.com/redirect?token=abc123', got '%s'", response.RedirectURL)
	}

	t.Logf("✅ Regular payment test passed")
	t.Logf("   Transaction ID: %s", response.TransactionID)
	t.Logf("   Gateway Payment ID: %s", response.GatewayPaymentID)
	t.Logf("   Status: %s", response.Status)
	t.Logf("   Redirect URL: %s", response.RedirectURL)
}

// TestJioPayClientWithMock_TokenizedPayment tests tokenized payment flow (2-step: cryptogram → payment)
// NEW: No need to fetch-card anymore since vaultId is stored directly as token!
func TestJioPayClientWithMock_TokenizedPayment(t *testing.T) {
	mockTransport := jiopay.NewMockTransport()

	// Step 1: Mock getCryptogram endpoint (using vaultId from stored token)
	// NEW: Network token is now a string, matching actual JioPay response
	mockTransport.AddRoute("POST", "/getCryptogram/30518336", `{
		"status": {
			"status": "SUCCESS"
		},
		"card": {
			"type": 130,
			"expiryMm": "XX",
			"expiryYy": "XXXX",
			"name": "XXXXXXXXXXXXXX",
			"scheme": 104,
			"bankId": "NA",
			"remark": "CustId1000010002174238412819692",
			"vaultId": "30518336",
			"maskedNumber": "XXXXXXXXXXXX0055",
			"product": "MPL",
			"countryCode": "91",
			"tokenReference": "DNITHE3176415083784437844",
			"bankName": "NA",
			"par": "V20251126176415083784495166341",
			"authorizationState": "98765432109876543212",
			"cvvLess": 1,
			"cardReference": "91",
			"consent": {
				"id": "123",
				"timestamp": "2025-09-25T18:15:30.0002"
			},
			"productType": "Normal",
			"isExpired": false,
			"isDomestic": true
		},
		"secure": {
			"token": "4489682380079084",
			"tokenExpiry": "122027",
			"cryptogram": "I6SCOIN6QhVdmqiuYRp2kQdeOr="
		}
	}`, 200)

	// Step 2: Mock initiateSale endpoint
	mockTransport.AddRoute("POST", "/initiateSale", `{
		"responseCode": "R1000",
		"responseDescription": "Transaction initiated successfully",
		"merchantId": "merchant-id",
		"merchantTxnNo": "txn-tokenized",
		"redirectURI": "https://jiopay.example.com/redirect?token=xyz",
		"showOTPCapturePage": "N",
		"tranCtx": "mock-tran-context",
		"secureHash": "mock-secure-hash",
		"oth_charge": 0.0
	}`, 200)

	// Create client with mock transport
	client := jiopay.NewJioPayClient(
		"https://api.jiopay.test",
		"https://vault.jiopay.test",
		"merchant-id",
		"api-key",
		"app-id",
		"1234567890123456",
		false, // isAggregatorModel
		&MockBusinessService{},
	)

	jioClient := client.(*jiopay.JioPayClient)
	jioClient.SetHTTPClient(&http.Client{Transport: mockTransport})

	// Test tokenized payment
	ctx := context.Background()
	request := payment.GatewayPaymentRequest{
		BusinessID:    "business-123",
		TransactionID: "txn-tokenized",
		Amount:        100000, // ₹1000
		Currency:      payment.CurrencyINR,
		PaymentMethod: payment.PaymentMethodCard,
		Token:         "30518336", // NEW: VaultId directly from stored_cards.token
		Metadata:      map[string]any{
			// No need for stored_card metadata - vaultId used directly!
		},
	}

	response, err := client.InitiatePayment(ctx, request)

	// Assertions
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if response.TransactionID != "txn-tokenized" {
		t.Errorf("Expected TransactionID 'txn-tokenized', got '%s'", response.TransactionID)
	}

	if response.GatewayPaymentID != "txn-tokenized" {
		t.Errorf("Expected GatewayPaymentID 'txn-tokenized', got '%s'", response.GatewayPaymentID)
	}

	if response.Status != payment.PaymentStatusPending {
		t.Errorf("Expected status 'pending', got '%s'", response.Status)
	}

	if response.RedirectURL != "https://jiopay.example.com/redirect?token=xyz" {
		t.Errorf("Expected RedirectURL 'https://jiopay.example.com/redirect?token=xyz', got '%s'", response.RedirectURL)
	}

	t.Logf("✅ Tokenized payment test passed (2-step flow: cryptogram → payment)")
	t.Logf("   Transaction ID: %s", response.TransactionID)
	t.Logf("   Gateway Payment ID: %s", response.GatewayPaymentID)
	t.Logf("   Status: %s", response.Status)
	t.Logf("   Redirect URL: %s", response.RedirectURL)
	t.Logf("   ✅ No fetch-card needed - vaultId used directly as token!")
}

// TestJioPayClientWithMock_PaymentError tests error handling with mocked error responses
func TestJioPayClientWithMock_PaymentError(t *testing.T) {
	mockTransport := jiopay.NewMockTransport()

	// Mock error response
	mockTransport.AddRoute("POST", "/initiateSale", `{
		"responseCode": "E1001",
		"responseDescription": "Invalid payment details",
		"merchantId": "merchant-id",
		"merchantTxnNo": "",
		"redirectURI": "",
		"showOTPCapturePage": "N",
		"tranCtx": "",
		"secureHash": "",
		"oth_charge": 0.0
	}`, 400)

	client := jiopay.NewJioPayClient(
		"https://api.jiopay.test",
		"https://vault.jiopay.test",
		"merchant-id",
		"api-key",
		"app-id",
		"1234567890123456",
		false, // isAggregatorModel
		&MockBusinessService{},
	)

	jioClient := client.(*jiopay.JioPayClient)
	jioClient.SetHTTPClient(&http.Client{Transport: mockTransport})

	ctx := context.Background()
	request := payment.GatewayPaymentRequest{
		BusinessID:    "business-123",
		TransactionID: "txn-error",
		Amount:        50000,
		Currency:      payment.CurrencyINR,
		PaymentMethod: payment.PaymentMethodCard,
		PaymentType:   payment.PaymentTypeTokenization, // Use tokenization flow
		CardDetails: &payment.Card{
			Number:       "4111111111111111",
			ExpiryMonth:  "12",
			ExpiryYear:   "2025",
			CVV:          "999",
			CardType:     payment.CardTypeVisa,
			CardCategory: payment.CardCategoryCredit,
		},
	}

	_, err := client.InitiatePayment(ctx, request)

	// Assert error occurred
	if err == nil {
		t.Fatal("Expected error but got nil")
	}

	t.Logf("✅ Error handling test passed")
	t.Logf("   Error message: %v", err)
}

// TestJioPayClientWithMock_CardTokenization tests card tokenization flow with mocked HTTP response
func TestJioPayClientWithMock_CardTokenization(t *testing.T) {
	mockTransport := jiopay.NewMockTransport()

	// Mock /tokenized-card endpoint with NEW JioPay API response format (2025)
	// IMPORTANT: New format has nested structure with vaultId immediately available!
	mockTransport.AddRoute("POST", "/tokenized-card", `{
		"headers": {},
		"body": {
			"status": {
				"status": "SUCCESS"
			},
			"vaultId": 30518336,
			"tokenReference": "DNITHE3176415083784437844"
		},
		"statusCode": "OK",
		"statusCodeValue": 200
	}`, 200)

	// Create JioPay client with mock transport
	client := jiopay.NewJioPayClient(
		"https://api.jiopay.test",
		"https://vault.jiopay.test",
		"merchant-id",
		"api-key",
		"app-id",
		"1234567890123456",
		false, // isAggregatorModel
		&MockBusinessService{},
	)

	jioClient := client.(*jiopay.JioPayClient)
	jioClient.SetHTTPClient(&http.Client{Transport: mockTransport})

	// Test tokenization request
	ctx := context.Background()
	request := payment.GatewayTokenizeRequest{
		BusinessID:    "business-123",
		TransactionID: "tokenize-txn-001",
		CardDetails: payment.Card{
			Number:         "4111111111115678",
			ExpiryMonth:    "12",
			ExpiryYear:     "2026",
			CVV:            "456",
			CardType:       payment.CardTypeVisa,
			CardCategory:   payment.CardCategoryCredit,
			HolderName:     "John Doe",
			LastFourDigits: "5678",
		},
		Metadata: map[string]any{
			"owner_mobile":   "9876543210",
			"customer_email": "customer@example.com",
			"customer_name":  "Customer",
		},
	}

	response, err := client.TokenizeCard(ctx, request)

	// Assertions
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// NEW: Token should be vaultId (as string), not tokenReference
	if response.Token != "30518336" {
		t.Errorf("Expected Token '30518336' (vaultId), got '%s'", response.Token)
	}

	// NEW: CardID should also be vaultId (same as token)
	if response.CardID != "30518336" {
		t.Errorf("Expected CardID '30518336' (vaultId), got '%s'", response.CardID)
	}

	if response.TransactionID != "tokenize-txn-001" {
		t.Errorf("Expected TransactionID 'tokenize-txn-001', got '%s'", response.TransactionID)
	}

	if response.Status != "success" {
		t.Errorf("Expected Status 'success', got '%s'", response.Status)
	}

	// Verify metadata contains JioPay-specific fields (new response format)
	if response.Metadata == nil {
		t.Fatal("Expected Metadata to be non-nil")
	}

	if gateway, ok := response.Metadata["gateway"].(string); !ok || gateway != "jiopay" {
		t.Errorf("Expected gateway 'jiopay', got '%v'", response.Metadata["gateway"])
	}

	// NEW: Check vaultId (as number)
	if vaultID, ok := response.Metadata["vault_id"].(int64); !ok || vaultID != 30518336 {
		t.Errorf("Expected vault_id 30518336, got '%v'", response.Metadata["vault_id"])
	}

	// NEW: Check vaultId (as string)
	if vaultIDStr, ok := response.Metadata["vault_id_str"].(string); !ok || vaultIDStr != "30518336" {
		t.Errorf("Expected vault_id_str '30518336', got '%v'", response.Metadata["vault_id_str"])
	}

	// NEW: Check tokenReference (now in metadata, not as primary token)
	if tokenRef, ok := response.Metadata["token_reference"].(string); !ok || tokenRef != "DNITHE3176415083784437844" {
		t.Errorf("Expected token_reference 'DNITHE3176415083784437844', got '%v'", response.Metadata["token_reference"])
	}

	// NEW: Check vault_id_available flag (should be true - vaultId available immediately!)
	if vaultAvailable, ok := response.Metadata["vault_id_available"].(bool); !ok || !vaultAvailable {
		t.Errorf("Expected vault_id_available 'true', got '%v'", response.Metadata["vault_id_available"])
	}

	// NEW: Check status code fields
	if statusCode, ok := response.Metadata["status_code"].(string); !ok || statusCode != "OK" {
		t.Errorf("Expected status_code 'OK', got '%v'", response.Metadata["status_code"])
	}

	if statusCodeValue, ok := response.Metadata["status_code_value"].(int); !ok || statusCodeValue != 200 {
		t.Errorf("Expected status_code_value 200, got '%v'", response.Metadata["status_code_value"])
	}

	t.Logf("✅ Card tokenization test passed")
	t.Logf("   Transaction ID: %s", response.TransactionID)
	t.Logf("   Token (vaultId): %s", response.Token)
	t.Logf("   Card ID (vaultId): %s", response.CardID)
	t.Logf("   Status: %s", response.Status)
	t.Logf("   VaultId (int64): %v", response.Metadata["vault_id"])
	t.Logf("   TokenReference: %s", response.Metadata["token_reference"])
	t.Logf("   ✅ VaultId is now available immediately - no need to call /fetch-card!")
}
