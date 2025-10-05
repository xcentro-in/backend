package castler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"xcentro.in/internal/core/payout"
)

// CastlerClient implements the PayoutProvider interface for Castler
type CastlerClient struct {
	baseURL    string
	apiKey     string
	secretKey  string
	httpClient *http.Client
}

// NewCastlerClient creates a new Castler payout client
func NewCastlerClient(baseURL, apiKey, secretKey string) *CastlerClient {
	return &CastlerClient{
		baseURL:   baseURL,
		apiKey:    apiKey,
		secretKey: secretKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetProviderType returns the provider identifier
func (c *CastlerClient) GetProviderType() payout.ProviderType {
	return payout.ProviderCastler
}

// SupportsTwoStepApproval returns true (Castler requires create + approve)
func (c *CastlerClient) SupportsTwoStepApproval() bool {
	return true
}

// OnboardBeneficiary registers a payee with Castler
func (c *CastlerClient) OnboardBeneficiary(ctx context.Context, req *payout.OnboardBeneficiaryRequest) (*payout.OnboardBeneficiaryResponse, error) {
	// Build Castler beneficiary request
	castlerReq := CastlerBeneficiaryRequest{
		AccountNumber:     req.AccountNumber,
		IFSCCode:          req.IFSCCode,
		AccountHolderName: req.AccountHolderName,
		BankName:          req.BankName,
		AccountType:       req.AccountType,
		Email:             req.Email,
		Phone:             req.Phone,
		GSTIN:             req.GSTIN,
		PAN:               req.PAN,
	}

	// Call Castler API
	var castlerResp CastlerBeneficiaryResponse
	if err := c.post(ctx, "/v1/beneficiaries", castlerReq, &castlerResp); err != nil {
		return nil, &payout.PayoutProviderError{
			Provider:    payout.ProviderCastler,
			ErrorCode:   payout.ErrCodeAPITimeout,
			Message:     fmt.Sprintf("Castler beneficiary onboarding failed: %v", err),
			IsRetryable: true,
		}
	}

	// Map Castler response to domain response
	status := payout.BeneficiaryStatusActive
	if castlerResp.Status != "active" {
		status = payout.BeneficiaryStatusPending
	}

	return &payout.OnboardBeneficiaryResponse{
		ProviderBeneficiaryID: castlerResp.BeneficiaryID,
		ProviderAccountID:     castlerResp.AccountID,
		Status:                status,
		Message:               castlerResp.Message,
		Metadata: map[string]interface{}{
			"castler_beneficiary_id": castlerResp.BeneficiaryID,
			"castler_account_id":     castlerResp.AccountID,
			"verification_status":    castlerResp.VerificationStatus,
		},
	}, nil
}

// GetBeneficiaryStatus checks if beneficiary is active with Castler
func (c *CastlerClient) GetBeneficiaryStatus(ctx context.Context, providerBeneficiaryID string) (*payout.BeneficiaryStatus, error) {
	endpoint := fmt.Sprintf("/v1/beneficiaries/%s", providerBeneficiaryID)

	var castlerResp CastlerBeneficiaryResponse
	if err := c.get(ctx, endpoint, &castlerResp); err != nil {
		return nil, &payout.PayoutProviderError{
			Provider:    payout.ProviderCastler,
			ErrorCode:   payout.ErrCodeBeneficiaryNotActive,
			Message:     fmt.Sprintf("Failed to get beneficiary status: %v", err),
			IsRetryable: true,
		}
	}

	status := payout.BeneficiaryStatusActive
	if castlerResp.Status != "active" {
		status = payout.BeneficiaryStatusPending
	}

	return &payout.BeneficiaryStatus{
		ProviderBeneficiaryID: castlerResp.BeneficiaryID,
		Status:                status,
		IsActive:              castlerResp.Status == "active",
		LastVerifiedAt:        &castlerResp.VerifiedAt,
		Metadata: map[string]interface{}{
			"verification_status": castlerResp.VerificationStatus,
		},
	}, nil
}

// CreateTransferRequest initiates a payout transfer with Castler
func (c *CastlerClient) CreateTransferRequest(ctx context.Context, req *payout.CreateTransferRequest) (*payout.CreateTransferResponse, error) {
	// Build Castler transfer request
	castlerReq := CastlerTransferRequest{
		BeneficiaryID: req.ProviderBeneficiaryID,
		Amount:        float64(req.Amount) / 100.0, // Convert paise to rupees
		Currency:      req.Currency,
		Purpose:       req.Purpose,
		Reference:     req.Reference,
		Narration:     req.Narration,
		TransferMode:  req.TransferMode,
	}

	// Call Castler API
	var castlerResp CastlerTransferResponse
	if err := c.post(ctx, "/v1/transfers", castlerReq, &castlerResp); err != nil {
		return nil, &payout.PayoutProviderError{
			Provider:    payout.ProviderCastler,
			ErrorCode:   payout.ErrCodeAPITimeout,
			Message:     fmt.Sprintf("Castler transfer creation failed: %v", err),
			IsRetryable: true,
		}
	}

	// Map Castler response to domain response
	status := payout.TransferStatusApproved // Castler creates in pending state
	if castlerResp.Status == "processing" {
		status = payout.TransferStatusProcessing
	}

	return &payout.CreateTransferResponse{
		ProviderTransferID:  castlerResp.TransferID,
		Status:              status,
		RequiresApproval:    true, // Castler always requires approval
		Message:             castlerResp.Message,
		EstimatedSettlement: &castlerResp.EstimatedSettlement,
		Metadata: map[string]interface{}{
			"castler_transfer_id": castlerResp.TransferID,
			"castler_status":      castlerResp.Status,
		},
	}, nil
}

// ApproveTransferRequest approves a pending transfer with Castler
func (c *CastlerClient) ApproveTransferRequest(ctx context.Context, req *payout.ApproveTransferRequest) (*payout.ApproveTransferResponse, error) {
	// Build Castler approval request
	endpoint := fmt.Sprintf("/v1/transfers/%s/approve", req.ProviderTransferID)
	castlerReq := CastlerApprovalRequest{
		Remarks: req.Remarks,
	}

	// Call Castler API
	var castlerResp CastlerTransferResponse
	if err := c.post(ctx, endpoint, castlerReq, &castlerResp); err != nil {
		return nil, &payout.PayoutProviderError{
			Provider:    payout.ProviderCastler,
			ErrorCode:   payout.ErrCodeAPITimeout,
			Message:     fmt.Sprintf("Castler transfer approval failed: %v", err),
			IsRetryable: true,
		}
	}

	// Map Castler response to domain response
	status := payout.TransferStatusProcessing
	if castlerResp.Status == "completed" {
		status = payout.TransferStatusCompleted
	}

	return &payout.ApproveTransferResponse{
		ProviderTransferID: castlerResp.TransferID,
		Status:             status,
		Message:            castlerResp.Message,
		UTR:                castlerResp.UTR,
		Metadata: map[string]interface{}{
			"castler_transfer_id": castlerResp.TransferID,
			"castler_status":      castlerResp.Status,
		},
	}, nil
}

// GetTransferStatus queries the status of a transfer
func (c *CastlerClient) GetTransferStatus(ctx context.Context, providerTransferID string) (*payout.TransferStatus, error) {
	endpoint := fmt.Sprintf("/v1/transfers/%s", providerTransferID)

	var castlerResp CastlerTransferResponse
	if err := c.get(ctx, endpoint, &castlerResp); err != nil {
		return nil, &payout.PayoutProviderError{
			Provider:    payout.ProviderCastler,
			ErrorCode:   payout.ErrCodeTransferNotFound,
			Message:     fmt.Sprintf("Failed to get transfer status: %v", err),
			IsRetryable: true,
		}
	}

	// Map Castler status to domain status
	status := payout.TransferStatusInitiated // After approval, it's initiated
	switch castlerResp.Status {
	case "processing":
		status = payout.TransferStatusProcessing
	case "completed":
		status = payout.TransferStatusCompleted
	case "failed":
		status = payout.TransferStatusFailed
	}

	return &payout.TransferStatus{
		ProviderTransferID: castlerResp.TransferID,
		Status:             status,
		UTR:                castlerResp.UTR,
		FailureReason:      castlerResp.FailureReason,
		ProcessedAt:        &castlerResp.ProcessedAt,
		SettledAt:          &castlerResp.SettledAt,
		Metadata: map[string]interface{}{
			"castler_status": castlerResp.Status,
		},
	}, nil
}

// CancelTransfer cancels a pending transfer
func (c *CastlerClient) CancelTransfer(ctx context.Context, providerTransferID string) error {
	endpoint := fmt.Sprintf("/v1/transfers/%s/cancel", providerTransferID)

	var castlerResp CastlerTransferResponse
	if err := c.post(ctx, endpoint, nil, &castlerResp); err != nil {
		return &payout.PayoutProviderError{
			Provider:    payout.ProviderCastler,
			ErrorCode:   payout.ErrCodeAPITimeout,
			Message:     fmt.Sprintf("Castler transfer cancellation failed: %v", err),
			IsRetryable: true,
		}
	}

	return nil
}

// HTTP helper methods

func (c *CastlerClient) post(ctx context.Context, endpoint string, reqBody interface{}, respBody interface{}) error {
	var bodyReader io.Reader
	if reqBody != nil {
		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+endpoint, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("X-Secret-Key", c.secretKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}

	if respBody != nil {
		if err := json.NewDecoder(resp.Body).Decode(respBody); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

func (c *CastlerClient) get(ctx context.Context, endpoint string, respBody interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("X-Secret-Key", c.secretKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}

	if err := json.NewDecoder(resp.Body).Decode(respBody); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}
