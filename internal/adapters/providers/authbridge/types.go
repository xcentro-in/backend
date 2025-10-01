package authbridge

import (
	"encoding/json"
	"fmt"
)

/*
This package implements a dynamic request system for different document types.

Architecture Overview:
- DocumentRequest interface: Defines common methfunc (r PANCOMRequest) Validate() error {
	if r.DocNumber == "" {
		return fmt.Errorf("docNumber is required for Company PAN search")
	}
	if len(r.DocNumber) != 10 {
		return fmt.Errorf("Company PAN number must be 10 characters long")
	}
	return nil
}

func (r PANCOMRequest) ToJSON() (string, error) {
	jsonBytes, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Company PAN request: %v", err)
	}
	return string(jsonBytes), nil
}ll request types
- Document-specific structs: PANRequest, GSTINRequest, CINRequest, etc.
- Each request type can have different fields and validation rules
- APIRequest: The final encrypted wrapper that gets sent to the API

Usage Example:
	// Create a PAN request
	req, err := CreateDocumentRequest(authbridge.PAN, "ABCDE1234F", "trans123", 0)
	if err != nil {
		// handle error
	}

	// Validate the request
	if err := req.Validate(); err != nil {
		// handle validation error
	}

	// Or use the combined function
	req, err := ValidateAndCreateRequest(authbridge.PAN, "ABCDE1234F", "trans123", 0)

The system allows each document type to have:
- Different field requirements
- Custom validation logic
- Type-specific additional fields
*/

// DocumentType represents the document type constants
type DocumentType int

// Document type constants from TruthScreen API
const (
	PAN     DocumentType = 2
	GSTIN   DocumentType = 23
	CIN     DocumentType = 15
	DIN     DocumentType = 14
	PAN_COM DocumentType = 523
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
		Endpoint: "https://www.truthscreen.com/api/v2.2/idsearch",
	},
	GSTIN: {
		Type:     GSTIN,
		Name:     "GSTIN",
		Endpoint: "https://www.truthscreen.com/api/v2.2/idsearch", // Can be different endpoint
	},
	CIN: {
		Type:     CIN,
		Name:     "CIN",
		Endpoint: "https://www.truthscreen.com/api/v2.2/idsearch", // Can be different endpoint
	},
	DIN: {
		Type:     DIN,
		Name:     "DIN",
		Endpoint: "https://www.truthscreen.com/api/v2.2/idsearch", // Can be different endpoint
	},
	PAN_COM: {
		Type:     PAN_COM,
		Name:     "PAN_COM",
		Endpoint: "https://www.truthscreen.com/api/v2.2/idsearch", // Can be different endpoint
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
	IsAsync   int          `json:"isAsync,omitempty"`
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

// DINRequest represents the request structure for DIN document search
type DINRequest struct {
	DocType   DocumentType `json:"docType"`
	DocNumber string       `json:"docNumber"`
	TransID   string       `json:"transID,omitempty"`
	IsAsync   int          `json:"isAsync,omitempty"`
	// DIN might have additional fields
	DirectorName string `json:"directorName,omitempty"`
}

func (r DINRequest) GetDocType() DocumentType {
	return r.DocType
}

func (r DINRequest) Validate() error {
	if r.DocNumber == "" {
		return fmt.Errorf("docNumber is required for DIN search")
	}
	return nil
}

func (r DINRequest) ToJSON() (string, error) {
	jsonBytes, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("failed to marshal DIN request: %v", err)
	}
	return string(jsonBytes), nil
}

// PANCOMRequest represents the request structure for Company PAN document search
type PANCOMRequest struct {
	DocType   DocumentType `json:"docType"`
	DocNumber string       `json:"docNumber"`
	TransID   string       `json:"transID,omitempty"`
	IsAsync   int          `json:"isAsync,omitempty"`
	// Company PAN might have additional fields
	CompanyName string `json:"companyName,omitempty"`
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
