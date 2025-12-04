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
	"xcentro.in/internal/shared/logger"
)

// CastlerClient implements the PayoutProvider interface for Castler
type CastlerClient struct {
	baseURL       string
	apiKey        string
	secretKey     string
	escrowID      string // Escrow account ID for payout transfers
	httpClient    *http.Client
	encryptionKey string // AES-256 encryption key (required)
	fixedIV       []byte // Fixed IV for AES encryption (required)
	logger        *logger.Logger
}

// NewCastlerClient creates a new Castler payout client
// All requests and responses are encrypted using AES-256-CBC
// Parameters:
//   - baseURL: Castler API base URL
//   - apiKey: API authentication key
//   - secretKey: API signing secret
//   - escrowID: Escrow account ID for payout transfers
//   - encryptionKey: AES-256 encryption key (32 bytes, required)
//   - encryptionIVBase64: Fixed IV in base64 format (16 bytes, required, e.g., "TkMwVjAkMFQwTDAzMFJNRQ==")
//   - log: Logger instance for structured logging
func NewCastlerClient(baseURL, apiKey, secretKey, escrowID, encryptionKey, encryptionIVBase64 string, log *logger.Logger) (*CastlerClient, error) {
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
		escrowID:      escrowID,
		encryptionKey: encryptionKey,
		fixedIV:       fixedIV,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: log,
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
	if err := c.post(ctx, "/v1/payee", castlerReq, &castlerResp); err != nil {
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
	endpoint := fmt.Sprintf("/v1/payee/%s", providerBeneficiaryID)

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
	// Build Castler transfer request with exact API format
	castlerReq := CastlerTransferRequest{
		EscrowID:      c.escrowID,                          // Escrow account ID (from config)
		PayeeID:       req.ProviderBeneficiaryID,           // Castler payee ID
		Mode:          strings.ToLower(req.TransferMode),   // neft, rtgs, imps (Castler expects lowercase)
		Amount:        float64(req.Amount) / 100.0,         // Convert paise to rupees
		CustomerRefID: req.Reference,                       // Invoice number for tracking
		Purpose:       req.Purpose,                         // Purpose of payment
	}

	// Log Castler API request details
	c.logger.InfoWith("Castler API: Creating transfer request",
		"escrow_id", castlerReq.EscrowID,
		"payee_id", castlerReq.PayeeID,
		"mode", castlerReq.Mode,
		"amount_inr", castlerReq.Amount,
		"customer_ref_id", castlerReq.CustomerRefID,
		"purpose", castlerReq.Purpose)

	// Call Castler payout API
	var castlerResp CastlerTransferResponse
	if err := c.post(ctx, "/v1/transfer", castlerReq, &castlerResp); err != nil {
		return nil, &payout.PayoutProviderError{
			Provider:    payout.ProviderCastler,
			ErrorCode:   payout.ErrCodeAPITimeout,
			Message:     fmt.Sprintf("Castler transfer creation failed: %v", err),
			IsRetryable: true,
		}
	}

	// Check if request was successful
	if !castlerResp.Success {
		errMsg := "Unknown error"
		if castlerResp.Message != nil {
			errMsg = *castlerResp.Message
		}
		if len(castlerResp.Errors) > 0 {
			errMsg = fmt.Sprintf("%s: %v", errMsg, castlerResp.Errors)
		}
		return nil, &payout.PayoutProviderError{
			Provider:    payout.ProviderCastler,
			ErrorCode:   payout.ErrCodeAPITimeout,
			Message:     fmt.Sprintf("Castler payout failed: %s", errMsg),
			IsRetryable: true, // May be retryable depending on error
		}
	}

	// Map Castler response to domain response
	// Transfer is initiated immediately with Castler
	message := "Transfer initiated successfully"
	if castlerResp.Message != nil {
		message = *castlerResp.Message
	}

	return &payout.CreateTransferResponse{
		ProviderTransferID:  castlerResp.Result.TransferID, // Use transferId from result
		Status:              payout.TransferStatusInitiated, // Always initiated
		RequiresApproval:    false,                          // No approval workflow
		Message:             message,
		EstimatedSettlement: nil, // Settlement info comes via webhook
		Metadata: map[string]interface{}{
			"castler_transfer_id": castlerResp.Result.TransferID,
			"customer_ref_id":     req.TransferRequestID,
			"success":             castlerResp.Success,
		},
	}, nil
}

// ApproveTransferRequest approves a pending transfer with Castler
// DEPRECATED: Approval workflow removed - Castler auto-processes payouts
func (c *CastlerClient) ApproveTransferRequest(ctx context.Context, req *payout.ApproveTransferRequest) (*payout.ApproveTransferResponse, error) {
	return nil, &payout.PayoutProviderError{
		Provider:    payout.ProviderCastler,
		ErrorCode:   payout.ErrCodeAPITimeout,
		Message:     "Approval not supported - Castler auto-processes payouts after initiation",
		IsRetryable: false,
	}
}

// GetTransferStatus queries the status of a transfer
// NOTE: Status updates come via webhooks - this is for manual debugging only
func (c *CastlerClient) GetTransferStatus(ctx context.Context, providerTransferID string) (*payout.TransferStatus, error) {
	// TODO: Implement if needed for debugging/manual status checks
	// Would need to update to use new Castler response format
	return nil, &payout.PayoutProviderError{
		Provider:    payout.ProviderCastler,
		ErrorCode:   payout.ErrCodeTransferNotFound,
		Message:     "Manual status check not implemented - use settlement webhooks",
		IsRetryable: false,
	}
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

	// Debug: Show what headers we're sending (mask credentials)
	fmt.Printf("🔐 Castler Auth Headers:\n")
	fmt.Printf("   X-API-Key: %s...%s (len=%d)\n", maskCredential(c.apiKey, 4), maskCredentialEnd(c.apiKey, 4), len(c.apiKey))
	fmt.Printf("   X-Client-Key: %s...%s (len=%d)\n", maskCredential(c.secretKey, 4), maskCredentialEnd(c.secretKey, 4), len(c.secretKey))
}

// maskCredential shows first N chars, rest as asterisks
func maskCredential(s string, showFirst int) string {
	if len(s) <= showFirst {
		return s
	}
	return s[:showFirst] + "****"
}

// maskCredentialEnd shows last N chars
func maskCredentialEnd(s string, showLast int) string {
	if len(s) <= showLast {
		return ""
	}
	return s[len(s)-showLast:]
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

		// Log request JSON before encryption
		c.logger.InfoWith("Castler API: Request body (before encryption)", "json", string(jsonData))

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

	// Log raw response for debugging
	rawResponse := string(bodyBytes)
	if len(rawResponse) > 200 {
		c.logger.InfoWith("Castler API: Raw response (encrypted)",
			"response_truncated", rawResponse[:200],
			"http_status", resp.StatusCode)
	} else {
		c.logger.InfoWith("Castler API: Raw response (encrypted)",
			"response", rawResponse,
			"http_status", resp.StatusCode)
	}

	// Decrypt response (error responses are also encrypted)
	decryptedData, err := crypto.DecryptFromCastler(string(bodyBytes), c.encryptionKey, c.fixedIV)
	if err != nil {
		c.logger.ErrorWith("Castler API: Decryption failed", "error", err, "response_body", rawResponse)
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

	// Log raw response for debugging
	rawResponse := string(bodyBytes)
	if len(rawResponse) > 200 {
		c.logger.InfoWith("Castler API: Raw response (encrypted)",
			"response_truncated", rawResponse[:200],
			"http_status", resp.StatusCode)
	} else {
		c.logger.InfoWith("Castler API: Raw response (encrypted)",
			"response", rawResponse,
			"http_status", resp.StatusCode)
	}

	// Decrypt response (error responses are also encrypted)
	decryptedData, err := crypto.DecryptFromCastler(string(bodyBytes), c.encryptionKey, c.fixedIV)
	if err != nil {
		c.logger.ErrorWith("Castler API: Decryption failed", "error", err, "response_body", rawResponse)
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
