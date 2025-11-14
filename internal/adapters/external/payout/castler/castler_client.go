package castler

import (
	"context"
	"crypto/aes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"xcentro.in/internal/core/payout"
	"xcentro.in/internal/shared/crypto"
)

// CastlerClient implements the PayoutProvider interface for Castler
type CastlerClient struct {
	baseURL       string
	apiKey        string
	secretKey     string
	httpClient    *http.Client
	encryptionKey string // AES-256 encryption key (required)
	fixedIV       []byte // Fixed IV for AES encryption (required)
}

// NewCastlerClient creates a new Castler payout client
// All requests and responses are encrypted using AES-256-CBC
// Parameters:
//   - baseURL: Castler API base URL
//   - apiKey: API authentication key
//   - secretKey: API signing secret
//   - encryptionKey: AES-256 encryption key (32 bytes, required)
//   - encryptionIVBase64: Fixed IV in base64 format (16 bytes, required, e.g., "TkMwVjAkMFQwTDAzMFJNRQ==")
func NewCastlerClient(baseURL, apiKey, secretKey, encryptionKey, encryptionIVBase64 string) (*CastlerClient, error) {
	// Validate required parameters
	if encryptionKey == "" {
		return nil, fmt.Errorf("encryption key is required for Castler API")
	}
	if encryptionIVBase64 == "" {
		return nil, fmt.Errorf("encryption IV is required for Castler API")
	}

	// Decode fixed IV
	fixedIV, err := base64.StdEncoding.DecodeString(encryptionIVBase64)
	if err != nil {
		return nil, fmt.Errorf("invalid encryption IV (must be base64): %w", err)
	}
	if len(fixedIV) != aes.BlockSize {
		return nil, fmt.Errorf("invalid IV length: expected %d bytes, got %d bytes", aes.BlockSize, len(fixedIV))
	}

	return &CastlerClient{
		baseURL:       baseURL,
		apiKey:        apiKey,
		secretKey:     secretKey,
		encryptionKey: encryptionKey,
		fixedIV:       fixedIV,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
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

// OnboardPayeeToCastler registers a payee with Castler after penny-drop verification
// API: POST /api/v1/payee
func (c *CastlerClient) OnboardPayeeToCastler(ctx context.Context, accountHolder, email, mobile, accountNumber, ifsc, bankName string) (*CastlerPayeeOnboardResponse, error) {
	// Convert mobile string to int64 (Castler expects integer)
	mobileInt, err := strconv.ParseInt(mobile, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid mobile number: %w", err)
	}

	// Build Castler payee onboarding request
	castlerReq := CastlerPayeeOnboardRequest{
		AccountHolder: accountHolder,
		Email:         email,
		Mobile:        mobileInt,
		AccountNumber: accountNumber,
		IFSC:          ifsc,
		BankName:      bankName,
	}

	// Call Castler payee onboarding API
	var castlerResp CastlerPayeeOnboardResponse
	if err := c.post(ctx, "/api/v1/payee", castlerReq, &castlerResp); err != nil {
		return nil, fmt.Errorf("castler payee onboarding failed: %w", err)
	}

	// Check if onboarding was successful
	if !castlerResp.Success {
		errorMsg := "payee onboarding failed"
		if len(castlerResp.Errors) > 0 {
			errorMsg = castlerResp.Errors[0]
		} else if castlerResp.Message != nil {
			errorMsg = *castlerResp.Message
		}
		return nil, fmt.Errorf("payee onboarding failed: %s", errorMsg)
	}

	return &castlerResp, nil
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

// setCommonHeaders injects standard Castler API headers into HTTP requests
// This is the centralized place where all Castler API authentication headers are set
func (c *CastlerClient) setCommonHeaders(req *http.Request) {

	req.Header.Set("Content-Type", "application/json")

	// Set Castler authentication headers (required for all requests)
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("X-Client-Key", c.secretKey)
}

func (c *CastlerClient) post(ctx context.Context, endpoint string, reqBody interface{}, respBody interface{}) error {
	var bodyReader io.Reader

	// Encrypt request body (always encrypted)
	if reqBody != nil {
		// Marshal to JSON
		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}

		// Encrypt request
		encryptedData, err := crypto.EncryptForCastler(string(jsonData), c.encryptionKey, c.fixedIV)
		if err != nil {
			return fmt.Errorf("failed to encrypt request: %w", err)
		}
		bodyReader = strings.NewReader(encryptedData)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+endpoint, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Inject common headers (authentication + content-type)
	c.setCommonHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body (always encrypted, including error responses)
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Decrypt response (error responses are also encrypted)
	decryptedData, err := crypto.DecryptFromCastler(string(bodyBytes), c.encryptionKey, c.fixedIV)
	if err != nil {
		return fmt.Errorf("failed to decrypt response: %w", err)
	}

	// Check status code after decryption
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, decryptedData)
	}

	// Parse decrypted JSON into response body
	if respBody != nil {
		if err := json.Unmarshal([]byte(decryptedData), respBody); err != nil {
			return fmt.Errorf("failed to parse decrypted response: %w", err)
		}
	}

	return nil
}

func (c *CastlerClient) get(ctx context.Context, endpoint string, respBody interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Inject common headers (authentication headers)
	c.setCommonHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body (always encrypted, including error responses)
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Decrypt response (error responses are also encrypted)
	decryptedData, err := crypto.DecryptFromCastler(string(bodyBytes), c.encryptionKey, c.fixedIV)
	if err != nil {
		return fmt.Errorf("failed to decrypt response: %w", err)
	}

	// Check status code after decryption
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, decryptedData)
	}

	// Parse decrypted JSON into response body
	if respBody != nil {
		if err := json.Unmarshal([]byte(decryptedData), respBody); err != nil {
			return fmt.Errorf("failed to parse decrypted response: %w", err)
		}
	}

	return nil
}
