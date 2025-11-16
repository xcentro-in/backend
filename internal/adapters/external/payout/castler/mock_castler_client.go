package castler

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"xcentro.in/internal/core/payout"
	"xcentro.in/internal/shared/logger"
)

// MockCastlerClient implements PayoutProvider interface with mock responses
// Use this for testing and development without making actual API calls to Castler
type MockCastlerClient struct {
	// Add any state tracking if needed for testing
}

// NewMockCastlerClient creates a new mock Castler client
func NewMockCastlerClient() *MockCastlerClient {
	logger.Info("🎭 Mock Castler Client initialized - No real API calls will be made")
	return &MockCastlerClient{}
}

// GetProviderType returns the provider identifier
func (m *MockCastlerClient) GetProviderType() payout.ProviderType {
	return payout.ProviderCastler
}

// SupportsTwoStepApproval returns true (Castler requires create + approve)
func (m *MockCastlerClient) SupportsTwoStepApproval() bool {
	return true
}

// OnboardBeneficiary mocks beneficiary registration
func (m *MockCastlerClient) OnboardBeneficiary(ctx context.Context, req *payout.OnboardBeneficiaryRequest) (*payout.OnboardBeneficiaryResponse, error) {
	logger.Info(fmt.Sprintf("🎭 MOCK: OnboardBeneficiary called for payee_id=%s, account=%s", req.PayeeID, req.AccountNumber))

	// Generate mock Castler IDs
	mockBeneficiaryID := fmt.Sprintf("MOCK_BEN_%s", uuid.New().String()[:8])
	mockAccountID := fmt.Sprintf("MOCK_ACC_%s", uuid.New().String()[:8])

	return &payout.OnboardBeneficiaryResponse{
		ProviderBeneficiaryID: mockBeneficiaryID,
		ProviderAccountID:     mockAccountID,
		Status:                payout.BeneficiaryStatusActive,
		Message:               "Mock beneficiary onboarded successfully",
		Metadata: map[string]interface{}{
			"mock":                   true,
			"castler_beneficiary_id": mockBeneficiaryID,
			"castler_account_id":     mockAccountID,
			"verification_status":    "verified",
		},
	}, nil
}

// GetBeneficiaryStatus mocks beneficiary status check
func (m *MockCastlerClient) GetBeneficiaryStatus(ctx context.Context, providerBeneficiaryID string) (*payout.BeneficiaryStatus, error) {
	logger.Info(fmt.Sprintf("🎭 MOCK: GetBeneficiaryStatus called for beneficiary_id=%s", providerBeneficiaryID))

	now := time.Now().Format(time.RFC3339)

	return &payout.BeneficiaryStatus{
		ProviderBeneficiaryID: providerBeneficiaryID,
		Status:                payout.BeneficiaryStatusActive,
		IsActive:              true,
		LastVerifiedAt:        &now,
		Metadata: map[string]interface{}{
			"mock":                true,
			"verification_status": "verified",
		},
	}, nil
}

// CreateTransferRequest mocks transfer creation
func (m *MockCastlerClient) CreateTransferRequest(ctx context.Context, req *payout.CreateTransferRequest) (*payout.CreateTransferResponse, error) {
	logger.Info(fmt.Sprintf("🎭 MOCK: CreateTransferRequest called for voucher_id=%s, amount=%d, beneficiary=%s",
		req.VoucherID, req.Amount, req.ProviderBeneficiaryID))

	// Generate mock transfer ID
	mockTransferID := fmt.Sprintf("MOCK_TXN_%s", uuid.New().String()[:8])
	estimatedSettlement := time.Now().Add(24 * time.Hour).Format(time.RFC3339)

	return &payout.CreateTransferResponse{
		ProviderTransferID:  mockTransferID,
		Status:              payout.TransferStatusInitiated,
		RequiresApproval:    false,
		Message:             "Mock transfer created and initiated successfully",
		EstimatedSettlement: &estimatedSettlement,
		Metadata: map[string]interface{}{
			"mock":                true,
			"castler_transfer_id": mockTransferID,
			"castler_status":      "pending",
		},
	}, nil
}

// ApproveTransferRequest mocks transfer approval
func (m *MockCastlerClient) ApproveTransferRequest(ctx context.Context, req *payout.ApproveTransferRequest) (*payout.ApproveTransferResponse, error) {
	logger.Info(fmt.Sprintf("🎭 MOCK: ApproveTransferRequest called for transfer_id=%s, approved_by=%s",
		req.ProviderTransferID, req.ApprovedBy))

	// Generate mock UTR
	mockUTR := fmt.Sprintf("MOCK%d", time.Now().Unix())

	return &payout.ApproveTransferResponse{
		ProviderTransferID: req.ProviderTransferID,
		Status:             payout.TransferStatusProcessing,
		Message:            "Mock transfer approved and processing",
		UTR:                mockUTR,
		Metadata: map[string]interface{}{
			"mock":                true,
			"castler_transfer_id": req.ProviderTransferID,
			"castler_status":      "processing",
		},
	}, nil
}

// GetTransferStatus mocks transfer status query
func (m *MockCastlerClient) GetTransferStatus(ctx context.Context, providerTransferID string) (*payout.TransferStatus, error) {
	logger.Info(fmt.Sprintf("🎭 MOCK: GetTransferStatus called for transfer_id=%s", providerTransferID))

	// Generate mock UTR if not already set
	mockUTR := fmt.Sprintf("MOCK%d", time.Now().Unix())
	processedAt := time.Now().Format(time.RFC3339)
	settledAt := time.Now().Add(2 * time.Hour).Format(time.RFC3339)

	return &payout.TransferStatus{
		ProviderTransferID: providerTransferID,
		Status:             payout.TransferStatusCompleted,
		UTR:                mockUTR,
		FailureReason:      "",
		ProcessedAt:        &processedAt,
		SettledAt:          &settledAt,
		Metadata: map[string]interface{}{
			"mock":           true,
			"castler_status": "completed",
		},
	}, nil
}

// OnboardPayeeToCastler mocks payee onboarding to Castler
func (m *MockCastlerClient) OnboardPayeeToCastler(ctx context.Context, accountHolder, email, mobile, accountNumber, ifsc, bankName string) (*CastlerPayeeOnboardResponse, error) {
	logger.Info(fmt.Sprintf("🎭 MOCK: OnboardPayeeToCastler called for account=%s, holder=%s", accountNumber, accountHolder))

	// Generate mock payee ID
	mockPayeeID := fmt.Sprintf("MOCK_PAYEE_%s", uuid.New().String()[:8])
	message := "Mock payee onboarded successfully"

	return &CastlerPayeeOnboardResponse{
		Result:        mockPayeeID,
		Message:       &message,
		Errors:        []string{},
		Success:       true,
		TotalElements: 1,
	}, nil
}

// CancelTransfer mocks transfer cancellation
func (m *MockCastlerClient) CancelTransfer(ctx context.Context, providerTransferID string) error {
	logger.Info(fmt.Sprintf("🎭 MOCK: CancelTransfer called for transfer_id=%s", providerTransferID))
	return nil
}
