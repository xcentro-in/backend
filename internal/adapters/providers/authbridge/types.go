package authbridge

import (
	"encoding/json"
	"fmt"
)

// DocumentType represents the document type constants
type DocumentType int

// Document type constants from TruthScreen API
const (
	PAN     DocumentType = 2
	PAN_COM DocumentType = 523
	GSTIN   DocumentType = 23
	CIN     DocumentType = 15
	AADHAAR DocumentType = 472 // Used for both LINK and STATUS actions
)

// DocumentInfo holds information about each document type including its endpoint
type DocumentInfo struct {
	Type     DocumentType
	Name     string
	Endpoint string
}

// DocumentRegistry maps each document type to its endpoint and metadata
var DocumentRegistry = map[DocumentType]DocumentInfo{
	PAN: {
		Type:     PAN,
		Name:     "PAN",
		Endpoint: "https://www.truthscreen.com/v1/apicall/nid/idsearch",
	},
	PAN_COM: {
		Type:     PAN_COM,
		Name:     "PAN_COM",
		Endpoint: "https://www.truthscreen.com/v1/apicall/nid/panComprehensive",
	},
	GSTIN: {
		Type:     GSTIN,
		Name:     "GSTIN",
		Endpoint: "https://www.truthscreen.com/api/v2.2/idsearch",
	},
	CIN: {
		Type:     CIN,
		Name:     "CIN",
		Endpoint: "https://www.truthscreen.com/api/v2.2/idsearch",
	},
	AADHAAR: {
		Type:     AADHAAR,
		Name:     "AADHAAR",
		Endpoint: "https://www.truthscreen.com/api/v1.0/eaadhaardigilocker/",
	},
}

// DocumentRequest is an interface that all document-specific requests must implement
type DocumentRequest interface {
	GetDocType() DocumentType
	Validate() error
	ToJSON() (string, error)
}

// PANRequest represents the request structure for PAN document search
type PANRequest struct {
	DocType   DocumentType `json:"docType"`
	DocNumber string       `json:"docNumber"`
	TransID   string       `json:"transID,omitempty"`
}

// NewPANRequest creates a new PANRequest with default DocType
func NewPANRequest(docNumber, transID string) *PANRequest {
	return &PANRequest{
		DocType:   PAN,
		DocNumber: docNumber,
		TransID:   transID,
	}
}

func (r PANRequest) GetDocType() DocumentType {
	return r.DocType
}

func (r PANRequest) Validate() error {
	if r.DocNumber == "" {
		return fmt.Errorf("docNumber is required for PAN search")
	}
	if len(r.DocNumber) != 10 {
		return fmt.Errorf("PAN number must be 10 characters long")
	}
	return nil
}

func (r PANRequest) ToJSON() (string, error) {
	jsonBytes, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("failed to marshal PAN request: %v", err)
	}
	return string(jsonBytes), nil
}

// GSTINRequest represents the request structure for GSTIN document search
type GSTINRequest struct {
	DocType   DocumentType `json:"docType"`
	DocNumber string       `json:"docNumber"`
	TransID   string       `json:"transID,omitempty"`
	IsAsync   int          `json:"isAsync,omitempty"`
	// GSTIN might have additional fields
	StateCode string `json:"stateCode,omitempty"`
}

// NewGSTINRequest creates a new GSTINRequest with default DocType
func NewGSTINRequest(docNumber, transID string, isAsync int) *GSTINRequest {
	return &GSTINRequest{
		DocType:   GSTIN,
		DocNumber: docNumber,
		TransID:   transID,
		IsAsync:   isAsync,
	}
}

func (r GSTINRequest) GetDocType() DocumentType {
	return r.DocType
}

func (r GSTINRequest) Validate() error {
	if r.DocNumber == "" {
		return fmt.Errorf("docNumber is required for GSTIN search")
	}
	if len(r.DocNumber) != 15 {
		return fmt.Errorf("GSTIN number must be 15 characters long")
	}
	return nil
}

func (r GSTINRequest) ToJSON() (string, error) {
	jsonBytes, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("failed to marshal GSTIN request: %v", err)
	}
	return string(jsonBytes), nil
}

// CINRequest represents the request structure for CIN document search
type CINRequest struct {
	DocType   DocumentType `json:"docType"`
	DocNumber string       `json:"docNumber"`
	TransID   string       `json:"transID,omitempty"`
	IsAsync   int          `json:"isAsync,omitempty"`
	// CIN might have additional fields
	CompanyName string `json:"companyName,omitempty"`
}

// NewCINRequest creates a new CINRequest with default DocType
func NewCINRequest(docNumber, transID string, isAsync int) *CINRequest {
	return &CINRequest{
		DocType:   CIN,
		DocNumber: docNumber,
		TransID:   transID,
		IsAsync:   isAsync,
	}
}

func (r CINRequest) GetDocType() DocumentType {
	return r.DocType
}

func (r CINRequest) Validate() error {
	if r.DocNumber == "" {
		return fmt.Errorf("docNumber is required for CIN search")
	}
	return nil
}

func (r CINRequest) ToJSON() (string, error) {
	jsonBytes, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("failed to marshal CIN request: %v", err)
	}
	return string(jsonBytes), nil
}

// PANCOMRequest represents the request structure for Company PAN document search
type PANCOMRequest struct {
	DocType   DocumentType `json:"docType"`
	DocNumber string       `json:"PanNumber"`
	TransID   string       `json:"transID,omitempty"`
}

// NewPANCOMRequest creates a new PANCOMRequest with default DocType
func NewPANCOMRequest(docNumber, transID string) *PANCOMRequest {
	return &PANCOMRequest{
		DocType:   PAN_COM,
		DocNumber: docNumber,
		TransID:   transID,
	}
}

func (r PANCOMRequest) GetDocType() DocumentType {
	return r.DocType
}

func (r PANCOMRequest) Validate() error {
	if r.DocNumber == "" {
		return fmt.Errorf("docNumber is required for Company PAN search")
	}
	if len(r.DocNumber) != 10 {
		return fmt.Errorf("company PAN number must be 10 characters long")
	}
	return nil
}

func (r PANCOMRequest) ToJSON() (string, error) {
	jsonBytes, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Company PAN request: %v", err)
	}
	return string(jsonBytes), nil
}

// AadhaarLinkRequest represents the request structure for Aadhaar link generation
type AadhaarLinkRequest struct {
	DocType     DocumentType `json:"doc_type"`
	TransID     string       `json:"trans_id"`
	Action      string       `json:"action"`
	CallbackURL string       `json:"callback_url"`
	RedirectURL string       `json:"redirect_url"`
}

// NewAadhaarLinkRequest creates a new AadhaarLinkRequest with default DocType and action
func NewAadhaarLinkRequest(transID, callbackURL, redirectURL string) *AadhaarLinkRequest {
	// transID is required for creating a valid Aadhaar link request.
	// callbackURL and redirectURL are optional — only set them if provided.
	if transID == "" {
		return nil
	}

	req := &AadhaarLinkRequest{
		DocType: AADHAAR,
		TransID: transID,
		Action:  "LINK",
	}

	if callbackURL != "" {
		req.CallbackURL = callbackURL
	}
	if redirectURL != "" {
		req.RedirectURL = redirectURL
	}

	return req
}

func (r AadhaarLinkRequest) GetDocType() DocumentType {
	return r.DocType
}

func (r AadhaarLinkRequest) Validate() error {
	if r.TransID == "" {
		return fmt.Errorf("trans_id is required for Aadhaar link generation")
	}
	// if r.CallbackURL == "" {
	// 	return fmt.Errorf("callback_url is required for Aadhaar link generation")
	// }
	// if r.RedirectURL == "" {
	// 	return fmt.Errorf("redirect_url is required for Aadhaar link generation")
	// }
	if r.Action != "LINK" {
		return fmt.Errorf("action must be 'LINK' for Aadhaar link generation")
	}
	return nil
}

func (r AadhaarLinkRequest) ToJSON() (string, error) {
	jsonBytes, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Aadhaar link request: %v", err)
	}
	return string(jsonBytes), nil
}

// AadhaarStatusRequest represents the request structure for Aadhaar status checking
type AadhaarStatusRequest struct {
	DocType   DocumentType `json:"doc_type"`
	TSTransID string       `json:"ts_trans_id"`
	Action    string       `json:"action"`
}

// NewAadhaarStatusRequest creates a new AadhaarStatusRequest with default DocType and action
func NewAadhaarStatusRequest(tsTransID string) *AadhaarStatusRequest {
	return &AadhaarStatusRequest{
		DocType:   AADHAAR,
		TSTransID: tsTransID,
		Action:    "STATUS",
	}
}

func (r AadhaarStatusRequest) GetDocType() DocumentType {
	return r.DocType
}

func (r AadhaarStatusRequest) Validate() error {
	if r.TSTransID == "" {
		return fmt.Errorf("ts_trans_id is required for Aadhaar status check")
	}
	if r.Action != "STATUS" {
		return fmt.Errorf("action must be 'STATUS' for Aadhaar status check")
	}
	return nil
}

func (r AadhaarStatusRequest) ToJSON() (string, error) {
	jsonBytes, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Aadhaar status request: %v", err)
	}
	return string(jsonBytes), nil
}

// APIRequest represents the encrypted request body sent to API
type APIRequest struct {
	RequestData string `json:"requestData"`
}

// APIResponse represents the response from TruthScreen API
type APIResponse struct {
	ResponseData string `json:"responseData"`
}

// Config holds the client configuration
type Config struct {
	Username string
	Password string
}

// GetEndpoint returns the endpoint for a specific document type
func GetEndpoint(docType DocumentType) string {
	if info, exists := DocumentRegistry[docType]; exists {
		return info.Endpoint
	}
	// Fallback to default endpoint
	return "https://www.truthscreen.com/api/v2.2/idsearch"
}

// GetDocumentName returns the human-readable name for a document type
func GetDocumentName(docType DocumentType) string {
	if info, exists := DocumentRegistry[docType]; exists {
		return info.Name
	}
	return "Unknown"
}

// IsValidDocumentType checks if a document type is supported
func IsValidDocumentType(docType DocumentType) bool {
	_, exists := DocumentRegistry[docType]
	return exists
}

// CreateDocumentRequest creates the appropriate request type based on document type
// For Aadhaar operations, use the specific constructor functions instead
func CreateDocumentRequest(docType DocumentType, docNumber, transID string, isAsync int) (DocumentRequest, error) {
	if !IsValidDocumentType(docType) {
		return nil, fmt.Errorf("unsupported document type: %d", docType)
	}

	switch docType {
	case PAN:
		return NewPANRequest(docNumber, transID), nil
	case GSTIN:
		return NewGSTINRequest(docNumber, transID, isAsync), nil
	case CIN:
		return NewCINRequest(docNumber, transID, isAsync), nil
	case PAN_COM:
		return NewPANCOMRequest(docNumber, transID), nil
	case AADHAAR:
		// For Aadhaar, users should use specific constructors since it requires different parameters
		return nil, fmt.Errorf("for Aadhaar operations, use NewAadhaarLinkRequest or NewAadhaarStatusRequest directly")
	default:
		return nil, fmt.Errorf("unsupported document type: %d", docType)
	}
}

// ValidateAndCreateRequest creates and validates a document request in one step
// For Aadhaar operations, use the specific constructor functions instead
func ValidateAndCreateRequest(docType DocumentType, docNumber, transID string, isAsync int) (DocumentRequest, error) {
	req, err := CreateDocumentRequest(docType, docNumber, transID, isAsync)
	if err != nil {
		return nil, err
	}

	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %v", err)
	}

	return req, nil
}
