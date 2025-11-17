# Testing JioPay Client with Mocked API Responses

This guide explains how to test JioPay payment flows without hitting the actual JioPay API.

## Overview

Instead of creating a separate mock client, we use Go's standard **HTTP RoundTripper pattern** to intercept and mock HTTP responses at the transport layer. This approach:

- ✅ Tests the actual client code (not a mock)
- ✅ Mocks only the HTTP layer
- ✅ Allows testing all JioPay flows (payment, tokenization, cryptogram)
- ✅ No code changes needed in production

## Quick Start

### 1. Basic Test Setup

```go
package mypackage_test

import (
	"context"
	"net/http"
	"testing"

	"xcentro.in/internal/adapters/external/payment/jiopay"
	"xcentro.in/internal/core/payment"
)

func TestJioPayPayment(t *testing.T) {
	// Step 1: Create mock transport
	mockTransport := jiopay.NewMockTransport()

	// Step 2: Add mock responses
	mockTransport.AddRoute("POST", "/initiateSale", `{
		"txnId": "test-123",
		"status": "success",
		"redirectUrl": "https://jiopay.com/pay?token=abc"
	}`, 200)

	// Step 3: Create real JioPay client
	client := jiopay.NewJioPayClient(
		"https://api.jiopay.test",
		"https://vault.jiopay.test",
		"merchant-id",
		"api-key",
		"app-id",
		"encryptionkey16",
		mockBusinessService,
	)

	// Step 4: Inject mock HTTP client
	jioClient := client.(*jiopay.JioPayClient)
	jioClient.SetHTTPClient(&http.Client{Transport: mockTransport})

	// Step 5: Test the flow - uses mocked responses
	response, err := client.InitiatePayment(ctx, request)

	// Step 6: Assertions
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if response.TransactionID != "test-123" {
		t.Errorf("Expected txn test-123, got %s", response.TransactionID)
	}
}
```

## Testing Different Flows

### Flow 1: Regular Card Payment

```go
func TestRegularCardPayment(t *testing.T) {
	mockTransport := jiopay.NewMockTransport()

	// Mock initiateSale endpoint
	mockTransport.AddRoute("POST", "/initiateSale", `{
		"txnId": "card-payment-123",
		"status": "success",
		"redirectUrl": "https://jiopay.com/redirect",
		"message": "Payment initiated"
	}`, 200)

	// Create client and inject mock
	client := setupMockedClient(mockTransport)

	// Test payment request
	request := payment.GatewayPaymentRequest{
		BusinessID:    "biz-123",
		TransactionID: "txn-456",
		Amount:        50000, // ₹500
		Currency:      payment.CurrencyINR,
		PaymentMethod: payment.PaymentMethodCard,
		CardDetails: &payment.Card{
			CardNumber:   "4111111111111111",
			ExpiryMonth:  "12",
			ExpiryYear:   "2025",
			CVV:          "123",
			CardType:     payment.CardTypeCredit,
			CardCategory: payment.CardCategoryVisa,
		},
	}

	response, err := client.InitiatePayment(context.Background(), request)

	// Assertions
	assert.NoError(t, err)
	assert.Equal(t, "card-payment-123", response.TransactionID)
	assert.Equal(t, "success", response.Status)
}
```

### Flow 2: Tokenized Payment (3-Step Flow)

For tokenized payments, JioPay requires:
1. Fetch card by last 4 digits → get vaultId
2. Generate cryptogram using vaultId
3. Initiate payment with cryptogram

```go
func TestTokenizedPayment(t *testing.T) {
	mockTransport := jiopay.NewMockTransport()

	// Step 1: Mock /fetch-card endpoint
	mockTransport.AddRoute("POST", "/fetch-card", `{
		"status": "success",
		"data": {
			"cards": [
				{
					"vaultId": "vault-abc-123",
					"maskedNumber": "XXXXXXXXXXXX1234",
					"status": "active",
					"cardType": "CREDIT",
					"cardBrand": "VISA"
				}
			]
		}
	}`, 200)

	// Step 2: Mock /getCryptogram/{vaultId} endpoint
	mockTransport.AddRoute("POST", "/getCryptogram/vault-abc-123", `{
		"status": "success",
		"cryptogram": "crypto-xyz-789",
		"expiresAt": "2025-11-17T18:00:00Z"
	}`, 200)

	// Step 3: Mock /initiateSale endpoint
	mockTransport.AddRoute("POST", "/initiateSale", `{
		"txnId": "tokenized-txn-999",
		"status": "success",
		"redirectUrl": "https://jiopay.com/redirect?token=xyz"
	}`, 200)

	// Create client and inject mock
	client := setupMockedClient(mockTransport)

	// Test tokenized payment
	request := payment.GatewayPaymentRequest{
		BusinessID:    "biz-123",
		TransactionID: "txn-tokenized",
		Amount:        100000, // ₹1000
		Currency:      payment.CurrencyINR,
		PaymentMethod: payment.PaymentMethodTokenCard,
		Token:         "stored-token-123",
		Metadata: map[string]any{
			"stored_card": map[string]any{
				"last_four_digits": "1234",
				"card_type":        "credit",
				"card_category":    "visa",
			},
		},
	}

	response, err := client.InitiatePayment(context.Background(), request)

	// Assertions
	assert.NoError(t, err)
	assert.Equal(t, "tokenized-txn-999", response.TransactionID)
}
```

### Flow 3: Card Tokenization

```go
func TestCardTokenization(t *testing.T) {
	mockTransport := jiopay.NewMockTransport()

	// Mock /fetch-card endpoint (tokenization)
	mockTransport.AddRoute("POST", "/fetch-card", `{
		"status": "success",
		"data": {
			"tokenReference": "token-ref-new-456",
			"vaultId": "vault-new-789",
			"maskedNumber": "XXXXXXXXXXXX9876",
			"status": "active"
		}
	}`, 200)

	client := setupMockedClient(mockTransport)

	// Test tokenization
	request := payment.GatewayTokenizeRequest{
		BusinessID:    "biz-123",
		TransactionID: "tokenize-txn-001",
		CardDetails: payment.Card{
			CardNumber:     "4111111111119876",
			ExpiryMonth:    "06",
			ExpiryYear:     "2026",
			CVV:            "456",
			CardType:       payment.CardTypeCredit,
			CardCategory:   payment.CardCategoryVisa,
			LastFourDigits: "9876",
		},
		Metadata: map[string]any{
			"owner_mobile": "9876543210",
		},
	}

	response, err := client.TokenizeCard(context.Background(), request)

	// Assertions
	assert.NoError(t, err)
	assert.Equal(t, "token-ref-new-456", response.Token)
	assert.Equal(t, "success", response.Status)
}
```

## Testing Error Scenarios

### Test API Errors

```go
func TestPaymentAPIError(t *testing.T) {
	mockTransport := jiopay.NewMockTransport()

	// Mock error response
	mockTransport.AddRoute("POST", "/initiateSale", `{
		"status": "failed",
		"errorCode": "INSUFFICIENT_BALANCE",
		"message": "Insufficient balance"
	}`, 400)

	client := setupMockedClient(mockTransport)

	response, err := client.InitiatePayment(ctx, request)

	// Assert error is handled correctly
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "INSUFFICIENT_BALANCE")
}
```

### Test Network Timeout

```go
func TestPaymentTimeout(t *testing.T) {
	mockTransport := jiopay.NewMockTransport()

	// Don't add any routes - will timeout or return 404

	client := setupMockedClient(mockTransport)

	// Set short timeout
	jioClient := client.(*jiopay.JioPayClient)
	jioClient.SetHTTPClient(&http.Client{
		Transport: mockTransport,
		Timeout:   100 * time.Millisecond,
	})

	_, err := client.InitiatePayment(ctx, request)

	assert.Error(t, err)
}
```

## Helper Functions

Create reusable helper functions for cleaner tests:

```go
// setupMockedClient creates a JioPay client with mock transport
func setupMockedClient(mockTransport *jiopay.MockTransport) payment.PaymentGateway {
	client := jiopay.NewJioPayClient(
		"https://api.jiopay.test",
		"https://vault.jiopay.test",
		"test-merchant",
		"test-api-key",
		"test-app-id",
		"testkey16bytes!",
		&MockBusinessService{OwnerMobile: "9876543210"},
	)

	jioClient := client.(*jiopay.JioPayClient)
	jioClient.SetHTTPClient(&http.Client{Transport: mockTransport})

	return client
}

// MockBusinessService for testing
type MockBusinessService struct {
	OwnerMobile string
}

func (m *MockBusinessService) GetOwnerMobile(ctx context.Context, businessID string) (string, error) {
	return m.OwnerMobile, nil
}
```

## Integration Tests vs Unit Tests

### Unit Tests (This Approach)
- Mock HTTP responses
- Test client logic
- Fast execution
- No external dependencies

```go
func TestJioPayClient_UnitTest(t *testing.T) {
	mockTransport := jiopay.NewMockTransport()
	mockTransport.AddRoute("POST", "/initiateSale", mockResponse, 200)
	// ... test with mocked responses
}
```

### Integration Tests (Optional)
- Use real JioPay sandbox
- Test actual API integration
- Slower execution
- Requires sandbox credentials

```go
func TestJioPayClient_IntegrationTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Use real JioPay sandbox client
	client := jiopay.NewJioPayClient(
		os.Getenv("JIOPAY_SANDBOX_URL"),
		os.Getenv("JIOPAY_SANDBOX_VAULT_URL"),
		// ... real credentials
	)

	// No mock - hits real API
	response, err := client.InitiatePayment(ctx, request)
	// ... assertions
}
```

## Running Tests

```bash
# Run all tests
go test ./internal/adapters/external/payment/jiopay/...

# Run with verbose output
go test -v ./internal/adapters/external/payment/jiopay/...

# Run specific test
go test -v -run TestTokenizedPayment ./internal/adapters/external/payment/jiopay/

# Run with coverage
go test -cover ./internal/adapters/external/payment/jiopay/...
```

## Best Practices

1. **Mock all external calls**: Ensure every HTTP endpoint is mocked
2. **Test happy paths**: Test successful flows first
3. **Test error paths**: Test API errors, validation errors, timeouts
4. **Use table-driven tests**: For testing multiple scenarios
5. **Keep mocks realistic**: Use actual JioPay response formats
6. **Test edge cases**: Empty responses, malformed JSON, etc.

## Example: Table-Driven Test

```go
func TestInitiatePayment_Scenarios(t *testing.T) {
	tests := []struct {
		name           string
		mockResponse   string
		mockStatusCode int
		wantError      bool
		wantTxnID      string
	}{
		{
			name: "successful payment",
			mockResponse: `{"txnId":"success-123","status":"success"}`,
			mockStatusCode: 200,
			wantError: false,
			wantTxnID: "success-123",
		},
		{
			name: "insufficient balance",
			mockResponse: `{"status":"failed","errorCode":"INSUFFICIENT_BALANCE"}`,
			mockStatusCode: 400,
			wantError: true,
		},
		{
			name: "invalid card",
			mockResponse: `{"status":"failed","errorCode":"INVALID_CARD"}`,
			mockStatusCode: 400,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTransport := jiopay.NewMockTransport()
			mockTransport.AddRoute("POST", "/initiateSale", tt.mockResponse, tt.mockStatusCode)

			client := setupMockedClient(mockTransport)
			response, err := client.InitiatePayment(ctx, request)

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantTxnID, response.TransactionID)
			}
		})
	}
}
```

## Advantages of This Approach

✅ **No separate mock client** - Test the real implementation
✅ **HTTP-layer mocking** - Clean separation of concerns
✅ **Flexible** - Easy to add new mock responses
✅ **Standard Go pattern** - Uses http.RoundTripper interface
✅ **Comprehensive testing** - Test all code paths
✅ **Fast** - No network calls
✅ **Deterministic** - Predictable test results

## Troubleshooting

### Mock not matching request
- Check URL path matches exactly
- Verify HTTP method (GET/POST/PUT)
- Enable debug logging to see actual requests

### Response parsing errors
- Ensure mock response is valid JSON
- Match actual JioPay response format
- Check field names and types

### Client not using mock
- Verify `SetHTTPClient()` is called after client creation
- Ensure using `&http.Client{Transport: mockTransport}`
