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

// TestJioPayClientWithMock_TokenizedPayment tests tokenized payment flow (3-step: fetch card → cryptogram → payment)
func TestJioPayClientWithMock_TokenizedPayment(t *testing.T) {
	mockTransport := jiopay.NewMockTransport()

	// Step 1: Mock fetch card endpoint
	mockTransport.AddRoute("POST", "/fetch-card", `{
		"status": {
			"status": "SUCCESS"
		},
		"card": [{
			"vaultId": "vault-123",
			"maskedNumber": "XXXXXXXXXXXX1234",
			"type": 1,
			"expiryMm": "12",
			"expiryYy": "25",
			"name": "Test Card",
			"scheme": 104,
			"bankId": "BANK123",
			"remark": "test-remark",
			"product": "CREDIT",
			"countryCode": "IN",
			"tokenReference": "token-ref-123",
			"bankName": "Test Bank",
			"authorizationState": "ACTIVE",
			"status": "ACTIVE"
		}]
	}`, 200)

	// Step 2: Mock getCryptogram endpoint
	mockTransport.AddRoute("POST", "/getCryptogram/vault-123", `{
		"status": {
			"status": "SUCCESS"
		},
		"card": {
			"type": 1,
			"expiryMm": "12",
			"expiryYy": "25",
			"name": "Test Card",
			"scheme": 104,
			"remark": "test-remark",
			"vaultId": "vault-123",
			"maskedNumber": "XXXXXXXXXXXX1234",
			"status": "ACTIVE",
			"isExpired": false,
			"isDomestic": true,
			"cvvLess": 0
		},
		"secure": {
			"token": 1234567890123456,
			"tokenExpiry": "122025",
			"cryptogram": "ABCDEF1234567890CRYPTOGRAM1234567890ABCDEF"
		}
	}`, 200)

	// Step 3: Mock initiateSale endpoint
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
		Token:         "stored-token", // Token presence triggers tokenized flow
		Metadata: map[string]any{
			"stored_card": map[string]any{
				"last_four_digits": "1234",
				"card_type":        "credit",
				"card_category":    "visa",
			},
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

	t.Logf("✅ Tokenized payment test passed (3-step flow complete)")
	t.Logf("   Transaction ID: %s", response.TransactionID)
	t.Logf("   Gateway Payment ID: %s", response.GatewayPaymentID)
	t.Logf("   Status: %s", response.Status)
	t.Logf("   Redirect URL: %s", response.RedirectURL)
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

	// Mock /tokenized-card endpoint with ACTUAL JioPay API response format
	// IMPORTANT: This is the REAL format - flat structure, NO nested objects!
	mockTransport.AddRoute("POST", "/tokenized-card", `{
		"responseId": "a92004c2-6d6d-4a3f-aee2-59af50b4b806",
		"responseHost": null,
		"tokenReference": "token-ref-new-card-456",
		"panLastFourDigits": "5678",
		"cardReference": "V0010013019009357267382613440",
		"bankName": "Test Bank Limited",
		"bankId": "BANK123",
		"par": "PAR123456789ABC",
		"expiry": "122026",
		"tokenRequestorId": "40000000082"
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

	if response.Token != "token-ref-new-card-456" {
		t.Errorf("Expected Token 'token-ref-new-card-456', got '%s'", response.Token)
	}

	// CardID now uses cardReference (vaultId not available from /tokenized-card)
	if response.CardID != "V0010013019009357267382613440" {
		t.Errorf("Expected CardID 'V0010013019009357267382613440', got '%s'", response.CardID)
	}

	if response.TransactionID != "tokenize-txn-001" {
		t.Errorf("Expected TransactionID 'tokenize-txn-001', got '%s'", response.TransactionID)
	}

	if response.Status != "success" {
		t.Errorf("Expected Status 'success', got '%s'", response.Status)
	}

	// Verify metadata contains JioPay-specific fields (actual response format)
	if response.Metadata == nil {
		t.Fatal("Expected Metadata to be non-nil")
	}

	if gateway, ok := response.Metadata["gateway"].(string); !ok || gateway != "jiopay" {
		t.Errorf("Expected gateway 'jiopay', got '%v'", response.Metadata["gateway"])
	}

	// Check tokenReference
	if tokenRef, ok := response.Metadata["token_reference"].(string); !ok || tokenRef != "token-ref-new-card-456" {
		t.Errorf("Expected token_reference 'token-ref-new-card-456', got '%v'", response.Metadata["token_reference"])
	}

	// Check cardReference (used as CardID placeholder)
	if cardRef, ok := response.Metadata["card_reference"].(string); !ok || cardRef != "V0010013019009357267382613440" {
		t.Errorf("Expected card_reference 'V0010013019009357267382613440', got '%v'", response.Metadata["card_reference"])
	}

	// Check panLastFourDigits
	if panLast4, ok := response.Metadata["pan_last_4_digits"].(string); !ok || panLast4 != "5678" {
		t.Errorf("Expected pan_last_4_digits '5678', got '%v'", response.Metadata["pan_last_4_digits"])
	}

	if bankName, ok := response.Metadata["bank_name"].(string); !ok || bankName != "Test Bank Limited" {
		t.Errorf("Expected bank_name 'Test Bank Limited', got '%v'", response.Metadata["bank_name"])
	}

	if par, ok := response.Metadata["par"].(string); !ok || par != "PAR123456789ABC" {
		t.Errorf("Expected par 'PAR123456789ABC', got '%v'", response.Metadata["par"])
	}

	// Check vault_id_available flag (should be false - vaultId comes from /fetch-card later)
	if vaultAvailable, ok := response.Metadata["vault_id_available"].(bool); !ok || vaultAvailable {
		t.Errorf("Expected vault_id_available 'false', got '%v'", response.Metadata["vault_id_available"])
	}

	t.Logf("✅ Card tokenization test passed")
	t.Logf("   Transaction ID: %s", response.TransactionID)
	t.Logf("   Token: %s", response.Token)
	t.Logf("   Card ID (cardReference): %s", response.CardID)
	t.Logf("   Status: %s", response.Status)
	t.Logf("   PAN Last 4 Digits: %s", response.Metadata["pan_last_4_digits"])
	t.Logf("   Bank Name: %s", response.Metadata["bank_name"])
	t.Logf("   PAR: %s", response.Metadata["par"])
	t.Logf("   ⚠️  VaultId will be obtained later via /fetch-card")
}
