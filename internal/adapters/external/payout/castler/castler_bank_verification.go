package castler

import (
	"context"
	"time"

	"xcentro.in/internal/core/business_rules"
)

// CastlerBankVerificationService implements BankVerificationService using Castler's penny drop API
// TODO: Implement actual Castler penny drop API integration
type CastlerBankVerificationService struct {
	client *CastlerClient
}

// NewCastlerBankVerificationService creates a new Castler bank verification service
func NewCastlerBankVerificationService(client *CastlerClient) business_rules.BankVerificationService {
	return &CastlerBankVerificationService{
		client: client,
	}
}

// GetProviderType returns the provider identifier
func (s *CastlerBankVerificationService) GetProviderType() business_rules.BankVerificationProvider {
	return business_rules.BankVerificationProviderCastler
}

// VerifyBankAccount performs penny drop verification using Castler's API
func (s *CastlerBankVerificationService) VerifyBankAccount(ctx context.Context, req business_rules.BankVerificationRequest) (*business_rules.BankVerificationResponse, error) {
	// TODO: Implement Castler penny drop API call
	// API endpoint: POST /v1/bank-verification/penny-drop
	// Request body: {
	//   "account_number": req.AccountNumber,
	//   "ifsc_code": req.IFSCCode,
	//   "account_holder_name": req.AccountHolderName
	// }
	// Response: {
	//   "verification_id": "string",
	//   "status": "success" | "failed",
	//   "account_holder_name": "string",
	//   "name_match": true | false,
	//   "penny_amount": 1.00
	// }

	// Placeholder: Simulate API delay
	time.Sleep(2 * time.Second)

	// Placeholder: Return success response
	// In production, this will call s.client.post() to Castler's penny drop endpoint
	return &business_rules.BankVerificationResponse{
		RequestID:            req.RequestID,
		Status:               business_rules.BankVerificationStatusSuccess,
		VerifiedAccountName:  req.AccountHolderName,
		BankName:             req.BankName,
		BranchName:           req.BranchName,
		IFSCCode:             req.IFSCCode,
		AccountNumber:        req.AccountNumber,
		AccountType:          req.AccountType,
		IsAccountActive:      true,
		VerificationAmount:   100, // ₹1.00 in paise
		TransactionReference: "CASTLER_TXN_" + req.AccountNumber[len(req.AccountNumber)-6:],
		VerifiedAt:           time.Now(),
		Provider:             business_rules.BankVerificationProviderCastler,
		Metadata: map[string]any{
			"provider":        "castler",
			"verification_id": "CASTLER_VERIFY_" + req.AccountNumber[len(req.AccountNumber)-6:],
			"note":            "Placeholder implementation - TODO: Integrate Castler penny drop API",
		},
	}, nil
}

// GetVerificationStatus retrieves the status of a penny drop verification
func (s *CastlerBankVerificationService) GetVerificationStatus(ctx context.Context, verificationID string) (*business_rules.BankVerificationResponse, error) {
	// TODO: Implement Castler verification status API call
	// API endpoint: GET /v1/bank-verification/{verification_id}

	// Placeholder: Return success status
	return &business_rules.BankVerificationResponse{
		RequestID: verificationID,
		Status:    business_rules.BankVerificationStatusSuccess,
		Provider:  business_rules.BankVerificationProviderCastler,
		Metadata: map[string]any{
			"note": "Placeholder implementation - TODO: Integrate Castler verification status API",
		},
	}, nil
}
