package verification

import (
	"context"
	"encoding/json"
	"fmt"

	"xcentro.in/internal/adapters/providers/authbridge"
	"xcentro.in/internal/shared/logger"
)

// DocumentVerificationService provides a clean interface for document verification
type DocumentVerificationService struct {
	authBridgeClient *authbridge.AuthBridgeClient
	logger           *logger.Logger
}

// NewDocumentVerificationService creates a new verification service
func NewDocumentVerificationService() (*DocumentVerificationService, error) {
	client, err := authbridge.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create AuthBridge client: %w", err)
	}

	return &DocumentVerificationService{
		authBridgeClient: client,
		logger:           logger.GetDefault(),
	}, nil
}

// GSTINData represents the verified GSTIN information
type GSTINData struct {
	GSTIN          string                 `json:"gstin"`
	LegalName      string                 `json:"legal_name"`
	TradeName      string                 `json:"trade_name"`
	ProprietorName []string               `json:"proprietor_name"`
	Status         string                 `json:"status"`
	RawResponse    map[string]interface{} `json:"raw_response"`
}

// PANCOMData represents the verified PAN comprehensive information
type PANCOMData struct {
	PAN            string                 `json:"pan"`
	Name           string                 `json:"name"`
	EntityType     string                 `json:"entity_type"`
	Status         string                 `json:"status"`
	PANCOMResponse map[string]interface{} `json:"pan_com_response"`
}

// CINData represents the verified CIN information
type CINData struct {
	CIN         string                 `json:"cin"`
	CompanyName string                 `json:"company_name"`
	Status      string                 `json:"status"`
	RawResponse map[string]interface{} `json:"raw_response"`
}

// BankVerificationData represents the verified bank account information
type BankVerificationData struct {
	AccountNumber     string                 `json:"account_number"`
	IFSC              string                 `json:"ifsc"`
	AccountHolderName string                 `json:"account_holder_name"`
	AccountType       string                 `json:"account_type"`
	Status            string                 `json:"status"`
	TransactionRef    string                 `json:"transaction_ref"`
	RRN               string                 `json:"rrn"`
	VerifiedAt        string                 `json:"verified_at"`
	RawResponse       map[string]interface{} `json:"raw_response"`
}

// VerifyGSTIN verifies a GSTIN and returns the business information
func (s *DocumentVerificationService) VerifyGSTIN(ctx context.Context, gstin string, transID string) (*GSTINData, error) {
	s.logger.Infof("Verifying GSTIN: %s with transID: %s", gstin, transID)

	req := authbridge.NewGSTINRequest(gstin, transID)
	result, err := s.authBridgeClient.SearchDocument(req)
	if err != nil {
		s.logger.Errorf("GSTIN verification failed: %v", err)
		return nil, fmt.Errorf("GSTIN verification failed: %w", err)
	}

	// Check status code first - AuthBridge returns status: 1 for success
	status, ok := result["status"].(float64)
	if !ok {
		s.logger.Errorf("GSTIN verification failed: invalid response format, missing status field")
		return nil, fmt.Errorf("GSTIN verification failed: invalid response format")
	}

	if status != 1 {
		// Extract error message if available
		var errorMsg string
		if msg, ok := result["msg"].(string); ok {
			errorMsg = msg
		} else {
			errorMsg = "unknown error"
		}
		s.logger.Errorf("GSTIN verification failed for %s: status=%v, message=%s", gstin, status, errorMsg)
		return nil, fmt.Errorf("GSTIN verification failed: %s (status code: %.0f)", errorMsg, status)
	}

	// Extract relevant fields from the response
	data := &GSTINData{
		GSTIN:       gstin,
		RawResponse: result,
	}

	if msgData, ok := result["msg"].(map[string]interface{}); ok {
		if legalName, ok := msgData["Legal Name of Business"].(string); ok {
			data.LegalName = legalName
		}
		if tradeName, ok := msgData["Trade Name"].(string); ok {
			data.TradeName = tradeName
		}
		// Parse proprietor_name array
		if proprietorNameRaw, ok := msgData["proprietor_name"].([]interface{}); ok {
			proprietorNames := make([]string, 0, len(proprietorNameRaw))
			for _, name := range proprietorNameRaw {
				if nameStr, ok := name.(string); ok {
					proprietorNames = append(proprietorNames, nameStr)
				}
			}
			data.ProprietorName = proprietorNames
		}
	}
	// if status, ok := result["status"].(string); ok {
	// 	data.Status = status
	// }

	s.logger.Infof("GSTIN verification successful for: %s (proprietors: %v)", gstin, data.ProprietorName)
	return data, nil
}

// VerifyPANCOM verifies a PAN (Company) and returns comprehensive information
func (s *DocumentVerificationService) VerifyPANCOM(ctx context.Context, pan string, transID string) (*PANCOMData, error) {
	s.logger.Infof("Verifying PAN (Company): %s with transID: %s", pan, transID)

	// PAN_COM API call for comprehensive PAN details
	panComReq := authbridge.NewPANCOMRequest(pan, transID)
	panComResult, err := s.authBridgeClient.SearchDocument(panComReq)
	if err != nil {
		s.logger.Errorf("PAN_COM verification failed: %v", err)
		return nil, fmt.Errorf("PAN verification failed: %w", err)
	}

	// Check status code for PAN_COM
	if status, ok := panComResult["status"].(float64); ok && status != 1 {
		var errorMsg string
		if msg, ok := panComResult["msg"].(string); ok {
			errorMsg = msg
		} else {
			errorMsg = "unknown error"
		}
		s.logger.Errorf("PAN_COM verification failed for %s: status=%v, message=%s", pan, status, errorMsg)
		return nil, fmt.Errorf("PAN verification failed: %s (status code: %.0f)", errorMsg, status)
	}

	// Extract relevant fields
	data := &PANCOMData{
		PAN:            pan,
		PANCOMResponse: panComResult,
	}

	// TODO: Parse and validate response fields once AuthBridge response structure is stable
	// if name, ok := panComResult["name"].(string); ok {
	// 	data.Name = name
	// }
	// if entityType, ok := panComResult["entityType"].(string); ok {
	// 	data.EntityType = entityType
	// }
	// if status, ok := panComResult["status"].(string); ok {
	// 	data.Status = status
	// }

	s.logger.Infof("PAN_COM verification successful for: %s", pan)
	return data, nil
}

// VerifyCIN verifies a CIN and returns company information
func (s *DocumentVerificationService) VerifyCIN(ctx context.Context, cin string, transID string) (*CINData, error) {
	s.logger.Infof("Verifying CIN: %s with transID: %s", cin, transID)

	req := authbridge.NewCINRequest(cin, transID)
	result, err := s.authBridgeClient.SearchDocument(req)
	if err != nil {
		s.logger.Errorf("CIN verification failed: %v", err)
		return nil, fmt.Errorf("CIN verification failed: %w", err)
	}

	// Check status code first - AuthBridge returns status: 1 for success
	status, ok := result["status"].(float64)
	if !ok {
		s.logger.Errorf("CIN verification failed: invalid response format, missing status field")
		return nil, fmt.Errorf("CIN verification failed: invalid response format")
	}

	if status != 1 {
		// Extract error message if available
		var errorMsg string
		if msg, ok := result["msg"].(string); ok {
			errorMsg = msg
		} else {
			errorMsg = "unknown error"
		}
		s.logger.Errorf("CIN verification failed for %s: status=%v, message=%s", cin, status, errorMsg)
		return nil, fmt.Errorf("CIN verification failed: %s (status code: %.0f)", errorMsg, status)
	}

	// Extract relevant fields
	data := &CINData{
		CIN:         cin,
		RawResponse: result,
	}

	// TODO: Parse and validate response fields once AuthBridge response structure is stable
	// if companyName, ok := result["companyName"].(string); ok {
	// 	data.CompanyName = companyName
	// }
	// if status, ok := result["status"].(string); ok {
	// 	data.Status = status
	// }

	s.logger.Infof("CIN verification successful for: %s", cin)
	return data, nil
}

// VerifyBankAccount verifies a bank account via penny drop and returns account details
func (s *DocumentVerificationService) VerifyBankAccount(ctx context.Context, accountNumber string, ifsc string, transID string) (*BankVerificationData, error) {
	s.logger.Infof("Verifying bank account: IFSC=%s, transID=%s (account masked for security)", ifsc, transID)

	// Create bank verification request
	req := authbridge.NewBankVerificationRequest(transID, accountNumber, ifsc)
	result, err := s.authBridgeClient.SearchDocument(req)
	if err != nil {
		s.logger.Errorf("Bank account verification failed: %v", err)
		return nil, fmt.Errorf("bank account verification failed: %w", err)
	}

	s.logger.Debugf("Bank verification raw response: %+v", result)

	// Check top-level status code first - AuthBridge returns status: 1 for success
	status, ok := result["status"].(float64)
	if !ok {
		s.logger.Errorf("Bank verification failed: invalid response format, missing status field")
		return nil, fmt.Errorf("bank verification failed: invalid response format")
	}

	if status != 1 {
		// Extract error message if available
		var errorMsg string
		if msg, ok := result["msg"].(string); ok {
			errorMsg = msg
		} else {
			errorMsg = "unknown error"
		}
		s.logger.Errorf("Bank verification failed: status=%v, message=%s (IFSC=%s, transID=%s)", status, errorMsg, ifsc, transID)
		return nil, fmt.Errorf("bank verification failed: %s (status code: %.0f)", errorMsg, status)
	}

	// Parse nested response structure
	// AuthBridge returns: { "msg": {...}, "status": 1, "transId": "...", "tsTransId": "..." }
	msgData, ok := result["msg"].(map[string]interface{})
	if !ok {
		s.logger.Errorf("Invalid bank verification response format: missing 'msg' field")
		return nil, fmt.Errorf("invalid bank verification response format: missing 'msg' field")
	}

	// Extract verification status from nested msg object
	verificationStatus, _ := msgData["status"].(string)

	// Fail fast: Return error if verification did not succeed
	if verificationStatus != "SUCCESS" {
		s.logger.Warnf("Bank verification failed: status=%s", verificationStatus)
		return nil, fmt.Errorf("bank verification failed: account could not be verified (status: %s)", verificationStatus)
	}

	// Extract all relevant fields from msg object (with safe type assertions)
	accountHolderName, _ := msgData["beneficiaryName"].(string)
	accountType, _ := msgData["accountType"].(string)
	rrn, _ := msgData["rrn"].(string)
	transDateTime, _ := msgData["transDateTime"].(string)

	// Extract top-level fields
	tsTransID, _ := result["tsTransId"].(string)

	// Build structured response
	data := &BankVerificationData{
		AccountNumber:     accountNumber,
		IFSC:              ifsc,
		AccountHolderName: accountHolderName,
		AccountType:       accountType,
		Status:            verificationStatus,
		TransactionRef:    tsTransID,
		RRN:               rrn,
		VerifiedAt:        transDateTime,
		RawResponse:       result,
	}

	s.logger.Infof("Bank verification successful: account_holder=%s, account_type=%s", accountHolderName, accountType)
	return data, nil
}

// VerifyPANAadhaar verifies PAN-Aadhaar linking
func (s *DocumentVerificationService) VerifyPANAadhaar(ctx context.Context, pan string, aadhaar string, transID string) (map[string]interface{}, error) {
	s.logger.Infof("Verifying PAN-Aadhaar link: PAN=%s, Aadhaar=%s, transID=%s", pan, aadhaar, transID)

	req := authbridge.NewPANAadhaarRequest(pan, aadhaar, transID)
	result, err := s.authBridgeClient.SearchDocument(req)
	if err != nil {
		s.logger.Errorf("PAN-Aadhaar link verification failed: %v", err)
		return nil, fmt.Errorf("PAN-Aadhaar link verification failed: %w", err)
	}

	// Check status code first - AuthBridge returns status: 1 for success
	status, ok := result["status"].(float64)
	if !ok {
		s.logger.Errorf("PAN-Aadhaar verification failed: invalid response format, missing status field")
		return nil, fmt.Errorf("PAN-Aadhaar verification failed: invalid response format")
	}

	if status != 1 {
		// Extract error message if available
		var errorMsg string
		if msg, ok := result["msg"].(string); ok {
			errorMsg = msg
		} else {
			errorMsg = "unknown error"
		}
		s.logger.Errorf("PAN-Aadhaar verification failed for PAN %s: status=%v, message=%s", pan, status, errorMsg)
		return nil, fmt.Errorf("PAN-Aadhaar verification failed: %s (status code: %.0f)", errorMsg, status)
	}

	// COMPLIANCE: Remove Aadhaar number from response before storing in database
	// Navigate to msg.pan_aadhaar_linking and remove the aadhaar field
	if msgData, ok := result["msg"].(map[string]interface{}); ok {
		if panAadhaarData, ok := msgData["pan_aadhaar_linking"].(map[string]interface{}); ok {
			// Remove the aadhaar field for compliance
			delete(panAadhaarData, "aadhaar")
			s.logger.Info("Removed Aadhaar number from response for compliance")
		}
	}

	s.logger.Infof("PAN-Aadhaar link verification successful for PAN: %s", pan)
	return result, nil
}

// ToJSON converts verification data to JSON string for database storage
func ToJSON(data interface{}) (string, error) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal to JSON: %w", err)
	}
	return string(jsonBytes), nil
}
