package jiopay_test

import (
	"context"
	"fmt"
	"net/http"

	"xcentro.in/internal/adapters/external/payment/jiopay"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/payment"
	"xcentro.in/internal/core/shared"
)

// MockBusinessService implements minimal business.BusinessService for testing
type MockBusinessService struct{}

// GetOwnerMobile returns a test mobile number
func (m *MockBusinessService) GetOwnerMobile(ctx context.Context, businessID string) (string, error) {
	return "9876543210", nil
}

// Stub implementations for other BusinessService methods (not used by JioPay client)
func (m *MockBusinessService) UpdateBusinessDeedDocumentKey(ctx context.Context, bid string, key string) error {
	return nil
}
func (m *MockBusinessService) GetKYBDocumentUploadURL(ctx context.Context, userRoles []*business.BusinessUserRelation) (string, error) {
	return "", nil
}
func (m *MockBusinessService) ConfirmKYBDocumentUpload(ctx context.Context, userRoles []*business.BusinessUserRelation) (string, error) {
	return "", nil
}
func (m *MockBusinessService) InitiateOnboarding(ctx context.Context, uid string, name string, gstin string) (*business.OnboardingInitResponse, error) {
	return nil, nil
}
func (m *MockBusinessService) CompleteKYB(ctx context.Context, uid string, userRoles []*business.BusinessUserRelation, req *business.CompleteKYBRequest) (*business.CompleteKYBResponse, error) {
	return nil, nil
}
func (m *MockBusinessService) GetOnboardingStatus(ctx context.Context, uid string, userRoles []*business.BusinessUserRelation) (*business.OnboardingStatusResponse, error) {
	return nil, nil
}
func (m *MockBusinessService) GetBusinessDetails(ctx context.Context, bid string, userRole shared.UserRole) (*business.BusinessDetailsResponse, error) {
	return nil, nil
}
func (m *MockBusinessService) GetBusinessKYBData(ctx context.Context, bid string) (*business.BusinessKYBData, error) {
	return nil, nil
}
func (m *MockBusinessService) GetBusiness(ctx context.Context, bid string) (*business.Business, error) {
	return nil, nil
}
func (m *MockBusinessService) GetBusinessOnboardingFlags(ctx context.Context, bid string) (*business.BusinessOnboardingStatusDTO, error) {
	return nil, nil
}
func (m *MockBusinessService) UpdateOnboardingStatus(ctx context.Context, bid string, flagName string, value bool) error {
	return nil
}
func (m *MockBusinessService) InviteUserToBusiness(ctx context.Context, bid string, email string, name string, role shared.UserRole, requesterRole shared.UserRole) (*business.InviteUserResponse, error) {
	return nil, nil
}
func (m *MockBusinessService) AddUserToBusiness(ctx context.Context, relation *business.BusinessUserRelation) error {
	return nil
}
func (m *MockBusinessService) GetBusinessUsers(ctx context.Context, bid string, requesterRole shared.UserRole) ([]*business.BusinessUserDTO, error) {
	return nil, nil
}
func (m *MockBusinessService) UpdateUserRoleInBusiness(ctx context.Context, bid string, targetUserID string, requesterRole shared.UserRole, newRole shared.UserRole) error {
	return nil
}
func (m *MockBusinessService) RemoveUserFromBusiness(ctx context.Context, bid string, targetUserID string, requesterRole shared.UserRole) error {
	return nil
}
func (m *MockBusinessService) ListBusinessesPaginated(ctx context.Context, page int, limit int, kybStatus string) ([]*business.OpsBusinessListItem, int, error) {
	return nil, 0, nil
}
func (m *MockBusinessService) SearchBusinesses(ctx context.Context, searchTerm string, kybStatus string) ([]*business.OpsBusinessSearchResult, error) {
	return nil, nil
}
func (m *MockBusinessService) GetBusinessDetailsForOps(ctx context.Context, bid string) (*business.OpsBusinessDetails, error) {
	return nil, nil
}
func (m *MockBusinessService) ReloadBusinessData(ctx context.Context, bid string, dataType string) error {
	return nil
}
func (m *MockBusinessService) RejectKYB(ctx context.Context, bid string, notes string) error {
	return nil
}
func (m *MockBusinessService) VerifyKYB(ctx context.Context, bid string, notes string) error {
	return nil
}
func (m *MockBusinessService) OpsUpdateUserRole(ctx context.Context, bid string, userID string, role shared.UserRole) error {
	return nil
}

// Example: How to test JioPay client with mocked HTTP responses
// This is a demonstration - adapt for your actual test needs
func ExampleMockTransport_usage() {
	// Step 1: Create mock transport
	mockTransport := jiopay.NewMockTransport()

	// Step 2: Add mock responses for JioPay API endpoints
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

	// Step 3: Create JioPay client (with mock business service)
	// Note: In real tests, pass your actual mock or test business service
	client := jiopay.NewJioPayClient(
		"https://api.jiopay.test",
		"https://vault.jiopay.test",
		"merchant-id",
		"api-key",
		"app-id",
		"1234567890123456",      // 16-byte encryption key for AES
		false,                   // isAggregatorModel
		&MockBusinessService{},  // businessService - using mock
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

	fmt.Printf("\n=== Example Output ===\n")
	fmt.Printf("Transaction ID: %s\n", response.TransactionID)
	fmt.Printf("Gateway Payment ID: %s\n", response.GatewayPaymentID)
	fmt.Printf("Status: %s\n", response.Status)
	fmt.Printf("Redirect URL: %s\n", response.RedirectURL)

	// NOTE: This is a runnable example demonstrating JioPay mocking
	// Expected values with mocked response:
	//   Transaction ID: txn-456
	//   Gateway Payment ID: mock-transaction-123
	//   Status: pending
	//   Redirect URL: https://jiopay.example.com/redirect?token=abc123
}

// Example: Testing tokenized payment flow with multiple API calls
func ExampleMockTransport_tokenizedPayment() {
	mockTransport := jiopay.NewMockTransport()

	// Mock all 3 endpoints for tokenized payment
	// 1. Fetch card (get vaultId)
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

	// 2. Generate cryptogram
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

	// 3. Initiate payment
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

	// Create client and inject mock
	client := jiopay.NewJioPayClient(
		"https://api.jiopay.test",
		"https://vault.jiopay.test",
		"merchant-id",
		"api-key",
		"app-id",
		"1234567890123456",      // 16-byte encryption key for AES
		false,                   // isAggregatorModel
		&MockBusinessService{},  // Using mock business service
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
		PaymentMethod: payment.PaymentMethodCard, // Use CARD method
		Token:         "stored-token",             // Presence of token indicates tokenized payment
		Metadata: map[string]any{
			"stored_card": map[string]any{
				"last_four_digits": "1234",
			},
		},
	}

	response, err := client.InitiatePayment(ctx, request)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("\n=== Example Output ===\n")
	fmt.Printf("Transaction ID: %s\n", response.TransactionID)
	fmt.Printf("Gateway Payment ID: %s\n", response.GatewayPaymentID)
	fmt.Printf("Status: %s\n", response.Status)
	fmt.Printf("Redirect URL: %s\n", response.RedirectURL)

	// NOTE: This is a runnable example demonstrating JioPay tokenized payment flow
	// This example shows the complete 3-step flow: fetch card → generate cryptogram → initiate payment
	// Expected values with mocked responses:
	//   Transaction ID: txn-tokenized
	//   Gateway Payment ID: txn-tokenized
	//   Status: pending
	//   Redirect URL: https://jiopay.example.com/redirect?token=xyz
}
