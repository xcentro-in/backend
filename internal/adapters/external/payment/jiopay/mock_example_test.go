package jiopay_test

import (
	"context"
	"fmt"
	"net/http"

	"xcentro.in/internal/adapters/external/payment/jiopay"
	"xcentro.in/internal/core/payment"
)

// Example: How to test JioPay client with mocked HTTP responses
// This is a demonstration - adapt for your actual test needs
func ExampleMockTransport_usage() {
	// Step 1: Create mock transport
	mockTransport := jiopay.NewMockTransport()

	// Step 2: Add mock responses for JioPay API endpoints
	mockTransport.AddRoute("POST", "/initiateSale", `{
		"txnId": "mock-transaction-123",
		"status": "success",
		"redirectUrl": "https://jiopay.example.com/redirect?token=abc123",
		"message": "Transaction initiated successfully"
	}`, 200)

	// Step 3: Create JioPay client (with mock business service)
	// Note: In real tests, pass your actual mock or test business service
	client := jiopay.NewJioPayClient(
		"https://api.jiopay.test",
		"https://vault.jiopay.test",
		"merchant-id",
		"api-key",
		"app-id",
		"encryptionkey16",
		nil, // businessService - use your mock here
	)

	// Step 4: Inject mock HTTP client
	jioClient := client.(*jiopay.JioPayClient)
	jioClient.SetHTTPClient(&http.Client{
		Transport: mockTransport,
	})

	// Step 5: Now any API calls will use mocked responses
	ctx := context.Background()
	request := payment.GatewayPaymentRequest{
		BusinessID:    "business-123",
		TransactionID: "txn-456",
		Amount:        50000, // ₹500.00
		Currency:      payment.CurrencyINR,
		PaymentMethod: payment.PaymentMethodCard,
		Description:   "Test payment",
		CallbackURL:   "https://callback.example.com",
		// ... other fields
	}

	response, err := client.InitiatePayment(ctx, request)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Transaction ID: %s\n", response.TransactionID)
	fmt.Printf("Status: %s\n", response.Status)
	fmt.Printf("Redirect URL: %s\n", response.RedirectURL)

	// Output will use mocked response:
	// Transaction ID: mock-transaction-123
	// Status: success
	// Redirect URL: https://jiopay.example.com/redirect?token=abc123
}

// Example: Testing tokenized payment flow with multiple API calls
func ExampleMockTransport_tokenizedPayment() {
	mockTransport := jiopay.NewMockTransport()

	// Mock all 3 endpoints for tokenized payment
	// 1. Fetch card (get vaultId)
	mockTransport.AddRoute("POST", "/fetch-card", `{
		"status": "success",
		"data": {
			"cards": [{
				"vaultId": "vault-123",
				"maskedNumber": "XXXXXXXXXXXX1234",
				"status": "active"
			}]
		}
	}`, 200)

	// 2. Generate cryptogram
	mockTransport.AddRoute("POST", "/getCryptogram/vault-123", `{
		"status": "success",
		"cryptogram": "mock-cryptogram-xyz",
		"expiresAt": "2025-11-17T18:00:00Z"
	}`, 200)

	// 3. Initiate payment
	mockTransport.AddRoute("POST", "/initiateSale", `{
		"txnId": "tokenized-txn-789",
		"status": "success",
		"redirectUrl": "https://jiopay.example.com/redirect"
	}`, 200)

	// Create client and inject mock
	client := jiopay.NewJioPayClient(
		"https://api.jiopay.test",
		"https://vault.jiopay.test",
		"merchant-id",
		"api-key",
		"app-id",
		"encryptionkey16",
		nil, // Use your mock business service
	)

	jioClient := client.(*jiopay.JioPayClient)
	jioClient.SetHTTPClient(&http.Client{Transport: mockTransport})

	// Test tokenized payment - all 3 API calls will use mocks
	ctx := context.Background()
	request := payment.GatewayPaymentRequest{
		BusinessID:    "business-123",
		TransactionID: "txn-tokenized",
		Amount:        100000,
		Currency:      payment.CurrencyINR,
		PaymentMethod: payment.PaymentMethodTokenCard,
		Token:         "stored-token",
		Metadata: map[string]any{
			"stored_card": map[string]any{
				"last_four_digits": "1234",
			},
		},
	}

	response, _ := client.InitiatePayment(ctx, request)
	fmt.Printf("Tokenized Payment ID: %s\n", response.TransactionID)

	// Output:
	// Tokenized Payment ID: tokenized-txn-789
}
