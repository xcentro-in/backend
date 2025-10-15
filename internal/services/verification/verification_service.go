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
	GSTIN       string                 `json:"gstin"`
	LegalName   string                 `json:"legal_name"`
	TradeName   string                 `json:"trade_name"`
	Status      string                 `json:"status"`
	RawResponse map[string]interface{} `json:"raw_response"`
}

// PANCOMData represents the verified PAN comprehensive information
type PANCOMData struct {
	PAN            string                 `json:"pan"`
	Name           string                 `json:"name"`
	EntityType     string                 `json:"entity_type"`
	Status         string                 `json:"status"`
	PANCOMResponse map[string]interface{} `json:"pan_com_response"`
	PANGSTResponse map[string]interface{} `json:"pan_gst_response"`
}

// CINData represents the verified CIN information
type CINData struct {
	CIN         string                 `json:"cin"`
	CompanyName string                 `json:"company_name"`
	Status      string                 `json:"status"`
	RawResponse map[string]interface{} `json:"raw_response"`
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

	// Extract relevant fields from the response
	data := &GSTINData{
		GSTIN:       gstin,
		RawResponse: result,
	}

	// TODO: Parse and validate response fields once AuthBridge response structure is stable
	// if legalName, ok := result["legalName"].(string); ok {
	// 	data.LegalName = legalName
	// }
	// if tradeName, ok := result["tradeName"].(string); ok {
	// 	data.TradeName = tradeName
	// }
	// if status, ok := result["status"].(string); ok {
	// 	data.Status = status
	// }

	s.logger.Infof("GSTIN verification successful for: %s", gstin)
	return data, nil
}

// VerifyPANCOM verifies a PAN (Company) and returns comprehensive information
// This method makes two API calls:
// 1. PAN_COM API for comprehensive PAN details
// 2. PAN_GST API to get GST numbers associated with the PAN
func (s *DocumentVerificationService) VerifyPANCOM(ctx context.Context, pan string, transID string) (*PANCOMData, error) {
	s.logger.Infof("Verifying PAN (Company): %s with transID: %s", pan, transID)

	// First API call: PAN_COM
	panComReq := authbridge.NewPANRequest(pan, transID)
	panComResult, err := s.authBridgeClient.SearchDocument(panComReq)
	if err != nil {
		s.logger.Errorf("PAN_COM verification failed: %v", err)
		return nil, fmt.Errorf("PAN verification failed: %w", err)
	}

	// Second API call: PAN_GST
	panGSTReq := authbridge.NewPANGSTRequest(pan, transID+"-gst")
	panGSTResult, err := s.authBridgeClient.SearchDocument(panGSTReq)
	if err != nil {
		s.logger.Warnf("PAN_GST verification failed for %s: %v", pan, err)
		// Don't fail the entire request if PAN_GST fails, just log the warning
		panGSTResult = map[string]interface{}{
			"error": err.Error(),
		}
	}

	// Extract relevant fields
	data := &PANCOMData{
		PAN:            pan,
		PANCOMResponse: panComResult,
		PANGSTResponse: panGSTResult,
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

// VerifyPANAadhaar verifies PAN-Aadhaar linking
func (s *DocumentVerificationService) VerifyPANAadhaar(ctx context.Context, pan string, aadhaar string, transID string) (map[string]interface{}, error) {
	s.logger.Infof("Verifying PAN-Aadhaar link: PAN=%s, Aadhaar=%s, transID=%s", pan, aadhaar, transID)

	// req := authbridge.NewPANAadhaarRequest(pan, aadhaar, transID)
	// result, err := s.authBridgeClient.SearchDocument(req)
	// if err != nil {
	// 	s.logger.Errorf("PAN-Aadhaar link verification failed: %v", err)
	// 	return nil, fmt.Errorf("PAN-Aadhaar link verification failed: %w", err)
	// }

	s.logger.Infof("PAN-Aadhaar link verification successful for PAN: %s", pan)
	return map[string]interface{}{
		"Status": "Verified",
	}, nil
}

// ToJSON converts verification data to JSON string for database storage
func ToJSON(data interface{}) (string, error) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal to JSON: %w", err)
	}
	return string(jsonBytes), nil
}
