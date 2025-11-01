package authbridge

import (
	"encoding/json"
	"fmt"
)

// DocumentType represents the document type constants
type DocumentType int

// Document type constants from TruthScreen API
const (
	PAN         DocumentType = 2
	PAN_COM     DocumentType = 523
	PAN_GST     DocumentType = 455
	PAN_AADHAAR DocumentType = 544
	GSTIN       DocumentType = 23
	CIN         DocumentType = 15
	AADHAAR     DocumentType = 472 // Used for both LINK and STATUS actions
	BANK_VERIFY DocumentType = 92
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
	PAN_GST: {
		Type:     PAN_GST,
		Name:     "PAN_GST",
		Endpoint: "https://www.truthscreen.com/v1/apicall/business/pantogst",
	},
	PAN_AADHAAR: {
		Type:     PAN_AADHAAR,
		Name:     "PAN_AADHAAR",
		Endpoint: "https://www.truthscreen.com/v1/apicall/employment/pan_aadhaar_linking",
	},
	GSTIN: {
		Type:     GSTIN,
		Name:     "GSTIN",
		Endpoint: "https://www.truthscreen.com/v1/apicall/business/gstin",
	},
	CIN: {
		Type:     CIN,
		Name:     "CIN",
		Endpoint: "https://www.truthscreen.com/api/v2.2/idsearch",
	},
	AADHAAR: {
		Type:     AADHAAR,
		Name:     "AADHAAR",
		Endpoint: "https://www.truthscreen.com/api/v1.0/eaadhaardigilocker",
	},
	BANK_VERIFY: {
		Type:     BANK_VERIFY,
		Name:     "BANK_VERIFY",
		Endpoint: "https://www.truthscreen.com/v1/apicall/bank/bankAccountVerification",
	},
}

// DocumentRequest is an interface that all document-specific requests must implement
type DocumentRequest interface {
	GetDocType() DocumentType
	Validate() error
	ToJSON() (string, error)
}

// =============================================================================
// PAN Document Types
// =============================================================================

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
		return fmt.Errorf("docNumber is required")
	}
	if r.TransID == "" {
		return fmt.Errorf("transID is required")
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

// =============================================================================
// CIN Document Types
// =============================================================================

// CINRequest represents the request structure for CIN document search
type CINRequest struct {
	DocType   DocumentType `json:"docType"`
	DocNumber string       `json:"docNumber"`
	TransID   string       `json:"transID,omitempty"`
}

// NewCINRequest creates a new CINRequest with default DocType
func NewCINRequest(docNumber, transID string) *CINRequest {
	return &CINRequest{
		DocType:   CIN,
		DocNumber: docNumber,
		TransID:   transID,
	}
}

func (r CINRequest) GetDocType() DocumentType {
	return r.DocType
}

func (r CINRequest) Validate() error {
	if r.DocNumber == "" {
		return fmt.Errorf("docNumber is required")
	}
	if r.TransID == "" {
		return fmt.Errorf("transID is required")
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
		return fmt.Errorf("docNumber is required")
	}
	if r.TransID == "" {
		return fmt.Errorf("transID is required")
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

// PANGSTRequest represents the request structure for PAN to GST document search
type PANGSTRequest struct {
	DocType   DocumentType `json:"doc_type"`
	DocNumber string       `json:"doc_number"`
	TransID   string       `json:"trans_id,omitempty"`
}

// NewPANGSTRequest creates a new PANGSTRequest with default DocType
func NewPANGSTRequest(docNumber, transID string) *PANGSTRequest {
	return &PANGSTRequest{
		DocType:   PAN_GST,
		DocNumber: docNumber,
		TransID:   transID,
	}
}

func (r PANGSTRequest) GetDocType() DocumentType {
	return r.DocType
}

func (r PANGSTRequest) Validate() error {
	if r.DocNumber == "" {
		return fmt.Errorf("docNumber is required")
	}
	if r.TransID == "" {
		return fmt.Errorf("transID is required")
	}
	return nil
}

func (r PANGSTRequest) ToJSON() (string, error) {
	jsonBytes, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("failed to marshal PAN to GST request: %v", err)
	}
	return string(jsonBytes), nil
}

// =============================================================================
// GSTIN Document Types
// =============================================================================

// GSTINRequest represents the request structure for GSTIN document search
type GSTINRequest struct {
	DocType   DocumentType `json:"docType"`
	DocNumber string       `json:"docNumber"`
	TransID   string       `json:"transID,omitempty"`
}

// NewGSTINRequest creates a new GSTINRequest with default DocType
func NewGSTINRequest(docNumber, transID string) *GSTINRequest {
	return &GSTINRequest{
		DocType:   GSTIN,
		DocNumber: docNumber,
		TransID:   transID,
	}
}

func (r GSTINRequest) GetDocType() DocumentType {
	return r.DocType
}

func (r GSTINRequest) Validate() error {
	if r.DocNumber == "" {
		return fmt.Errorf("docNumber is required")
	}
	if r.TransID == "" {
		return fmt.Errorf("transID is required")
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

// PANAadhaarRequest represents the request structure for PAN to Aadhaar linking document search
type PANAadhaarRequest struct {
	DocType       DocumentType `json:"docType"`
	PANNumber     string       `json:"pan_number"`
	AadhaarNumber string       `json:"aadhaar_number"`
	TransID       string       `json:"transID,omitempty"`
}

// NewPANAadhaarRequest creates a new PANAadhaarRequest with default DocType
func NewPANAadhaarRequest(panNumber, aadhaarNumber, transID string) *PANAadhaarRequest {
	return &PANAadhaarRequest{
		DocType:       PAN_AADHAAR,
		PANNumber:     panNumber,
		AadhaarNumber: aadhaarNumber,
		TransID:       transID,
	}
}

func (r PANAadhaarRequest) GetDocType() DocumentType {
	return r.DocType
}

func (r PANAadhaarRequest) Validate() error {
	if r.PANNumber == "" {
		return fmt.Errorf("panNumber is required")
	}
	if r.AadhaarNumber == "" {
		return fmt.Errorf("aadhaarNumber is required")
	}
	if r.TransID == "" {
		return fmt.Errorf("transID is required")
	}
	return nil
}

func (r PANAadhaarRequest) ToJSON() (string, error) {
	jsonBytes, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("failed to marshal PAN to Aadhaar request: %v", err)
	}
	return string(jsonBytes), nil
}

// =============================================================================
// AADHAAR Document Types
// =============================================================================

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
		return fmt.Errorf("transID is required")
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
		return fmt.Errorf("tsTransID is required")
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

// =============================================================================
// Shared Types and Configuration
// =============================================================================

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

// =============================================================================
// Utility Functions
// =============================================================================

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

// =============================================================================
// Bank Verification Document Types
// =============================================================================

// BankVerificationRequest represents the request structure for bank verification
type BankVerificationRequest struct {
	DocType       DocumentType `json:"docType"`
	TransID       string       `json:"transID"`
	AccountNumber string       `json:"beneAccNo"`
	IFSC          string       `json:"ifsc"`
}

// NewBankVerificationRequest creates a new BankVerificationRequest with default DocType and action
func NewBankVerificationRequest(transID, accountNumber, ifsc string) *BankVerificationRequest {
	req := &BankVerificationRequest{
		DocType:       BANK_VERIFY,
		TransID:       transID,
		AccountNumber: accountNumber,
		IFSC:          ifsc,
	}

	return req
}

func (r BankVerificationRequest) GetDocType() DocumentType {
	return r.DocType
}

func (r BankVerificationRequest) Validate() error {
	if r.TransID == "" {
		return fmt.Errorf("transID is required")
	}
	if r.AccountNumber == "" {
		return fmt.Errorf("accountNumber is required")
	}
	if r.IFSC == "" {
		return fmt.Errorf("ifscCode is required")
	}
	return nil
}

func (r BankVerificationRequest) ToJSON() (string, error) {
	jsonBytes, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Bank verification request: %v", err)
	}
	return string(jsonBytes), nil
}
