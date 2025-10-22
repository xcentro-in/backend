package castler

import (
	"context"
	"fmt"
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
// API: POST /api/v1/payee/validation
func (s *CastlerBankVerificationService) VerifyBankAccount(ctx context.Context, req business_rules.BankVerificationRequest) (*business_rules.BankVerificationResponse, error) {
	// Extract email and phone from metadata (passed from payee onboarding)
	email := ""
	phone := ""
	if req.Metadata != nil {
		if e, ok := req.Metadata["email"].(string); ok {
			email = e
		}
		if p, ok := req.Metadata["phone"].(string); ok {
			phone = p
		}
	}

	// Build Castler penny-drop request
	castlerReq := CastlerPennyDropRequest{
		Payees: []CastlerPayeeValidation{
			{
				AccountHolder: req.AccountHolderName,
				Email:         email,
				Mobile:        phone,
				AccountNumber: req.AccountNumber,
				IFSC:          req.IFSCCode,
				BankName:      req.BankName,
				BranchName:    req.BranchName,
			},
		},
	}

	// Call Castler penny-drop API
	var castlerResp CastlerPennyDropResponse
	if err := s.client.post(ctx, "/api/v1/payee/validation", castlerReq, &castlerResp); err != nil {
		return nil, fmt.Errorf("castler penny-drop API failed: %w", err)
	}

	// Check if verification was successful
	if !castlerResp.Success {
		errorMsg := "penny-drop verification failed"
		if len(castlerResp.Errors) > 0 {
			errorMsg = castlerResp.Errors[0]
		} else if castlerResp.Message != nil {
			errorMsg = *castlerResp.Message
		}

		return &business_rules.BankVerificationResponse{
			RequestID: req.RequestID,
			Status:    business_rules.BankVerificationStatusFailed,
			Provider:  business_rules.BankVerificationProviderCastler,
			Metadata: map[string]any{
				"error":  errorMsg,
				"errors": castlerResp.Errors,
			},
		}, fmt.Errorf("penny-drop failed: %s", errorMsg)
	}

	// Success: Return verified bank details
	return &business_rules.BankVerificationResponse{
		RequestID:            req.RequestID,
		Status:               business_rules.BankVerificationStatusSuccess,
		VerifiedAccountName:  req.AccountHolderName, // Castler validates the account
		BankName:             req.BankName,
		BranchName:           req.BranchName,
		IFSCCode:             req.IFSCCode,
		AccountNumber:        req.AccountNumber,
		AccountType:          req.AccountType,
		IsAccountActive:      true,
		VerificationAmount:   100, // ₹1.00 in paise (standard penny drop amount)
		TransactionReference: castlerResp.Result,
		VerifiedAt:           time.Now(),
		Provider:             business_rules.BankVerificationProviderCastler,
		Metadata: map[string]any{
			"provider":        "castler",
			"validation_id":   castlerResp.Result,
			"message":         castlerResp.Message,
			"total_elements":  castlerResp.TotalElements,
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
