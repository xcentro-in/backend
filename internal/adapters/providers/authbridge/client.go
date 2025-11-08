package authbridge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/crypto"
)

// HTTPClient singleton for making HTTP requests
var (
	httpClientInstance *HTTPClient
	httpClientOnce     sync.Once
)

// HTTPClient is the HTTP client implementation
type HTTPClient struct {
	client *http.Client
}

// getHTTPClient returns a singleton HTTP client instance
func getHTTPClient() *HTTPClient {
	httpClientOnce.Do(func() {
		httpClientInstance = &HTTPClient{
			client: &http.Client{
				Timeout: 30 * time.Second,
			},
		}
	})
	return httpClientInstance
}

// Post performs HTTP POST request and returns status code, response body, and error
func (c *HTTPClient) Post(url string, headers map[string]string, body []byte) (int, []byte, error) {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return 0, nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Set headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("failed to read response: %v", err)
	}

	return resp.StatusCode, respBody, nil
}

// Singleton instance variables
var (
	instance *AuthBridgeClient
	once     sync.Once
	initErr  error
)

// AuthBridgeClient is the main client for document verification
type AuthBridgeClient struct {
	config     *Config
	httpClient *HTTPClient
	useMock    bool
}

// Initialize initializes the singleton AuthBridgeClient instance.
// Must be called once at application startup before any usage.
func Initialize(cfg *config.Config) error {
	once.Do(func() {
		instance = &AuthBridgeClient{
			config: &Config{
				Username: cfg.AuthBridge.Username,
				Password: cfg.AuthBridge.Password,
			},
			httpClient: getHTTPClient(),
			useMock:    cfg.AuthBridge.UseMock,
		}
	})
	return initErr
}

// GetInstance returns the singleton AuthBridgeClient instance.
// Panics if Initialize has not been called.
func GetInstance() *AuthBridgeClient {
	if instance == nil {
		panic("AuthBridgeClient not initialized. Call Initialize() first")
	}
	return instance
}

// NewClient returns the singleton AuthBridge client instance.
// For backward compatibility - returns the singleton if Initialize() was called.
// Deprecated: Use Initialize() and GetInstance() instead.
func NewClient() (*AuthBridgeClient, error) {
	if instance == nil {
		return nil, fmt.Errorf("AuthBridgeClient not initialized. Call authbridge.Initialize() with config at application startup")
	}
	return instance, nil
}

// SearchDocument performs document verification - main function
func (c *AuthBridgeClient) SearchDocument(req DocumentRequest) (map[string]interface{}, error) {
	// Validate the request
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("request validation failed: %v", err)
	}

	// Get document type and endpoint
	docType := req.GetDocType()
	if !IsValidDocumentType(docType) {
		return nil, fmt.Errorf("unsupported document type: %d", docType)
	}

	fmt.Printf("Document Type: %s (%d)\n", GetDocumentName(docType), docType)

	// SIMPLE SWITCH: Use mock or real implementation based on config
	if c.useMock {
		fmt.Println("Using MOCK AuthBridge responses")
		return c.getMockResponse(req)
	}

	fmt.Println("Using REAL AuthBridge API")
	return c.callRealAPI(req, docType)
}

// callRealAPI makes actual API call to AuthBridge
func (c *AuthBridgeClient) callRealAPI(req DocumentRequest, docType DocumentType) (map[string]interface{}, error) {
	endpoint := GetEndpoint(docType)

	// Convert to JSON using the request's ToJSON method
	inputJSON, err := req.ToJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to convert request to JSON: %v", err)
	}

	fmt.Printf("Input JSON: %s\n", inputJSON)

	// Encrypt the input (nil IV = random generation for AuthBridge)
	encryptedInput, err := crypto.EncryptAES256CBC(inputJSON, c.config.Password, nil)
	if err != nil {
		return nil, fmt.Errorf("encryption failed: %v", err)
	}

	// Prepare API request body
	apiReq := APIRequest{
		RequestData: encryptedInput,
	}

	requestBody, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal API request: %v", err)
	}

	// Log encrypted request
	fmt.Printf("[AuthBridge] Encrypted Request: %s\n", string(requestBody))

	// Prepare headers
	headers := map[string]string{
		"username": c.config.Username,
	}

	fmt.Printf("Making request to: %s\n", endpoint)

	// Make HTTP request
	statusCode, responseBody, err := c.httpClient.Post(endpoint, headers, requestBody)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %v", err)
	}

	// Log encrypted response
	fmt.Printf("[AuthBridge] HTTP Status: %d\n", statusCode)
	fmt.Printf("[AuthBridge] Encrypted Response: %s\n", string(responseBody))

	// Handle non-2xx status codes
	if statusCode < 200 || statusCode >= 300 {
		// Try to parse and decrypt error response
		var apiResp APIResponse
		if err := json.Unmarshal(responseBody, &apiResp); err != nil {
			// If can't parse as APIResponse, return raw error
			return nil, fmt.Errorf("HTTP %d: %s", statusCode, string(responseBody))
		}

		// Try to decrypt error response
		if apiResp.ResponseData != "" {
			decryptedError, err := crypto.DecryptAES256CBC(apiResp.ResponseData, c.config.Password)
			if err != nil {
				// If decryption fails, return encrypted error
				return nil, fmt.Errorf("HTTP %d: failed to decrypt error response: %v (encrypted: %s)", statusCode, err, apiResp.ResponseData)
			}
			// Log and return decrypted error
			fmt.Printf("[AuthBridge] Decrypted Error: %s\n", decryptedError)
			return nil, fmt.Errorf("HTTP %d: %s", statusCode, decryptedError)
		}

		// No response_data field, return raw error
		return nil, fmt.Errorf("HTTP %d: %s", statusCode, string(responseBody))
	}

	// Parse success response
	var apiResp APIResponse
	if err := json.Unmarshal(responseBody, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal API response: %v", err)
	}

	// Decrypt response
	decryptedResponse, err := crypto.DecryptAES256CBC(apiResp.ResponseData, c.config.Password)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %v", err)
	}

	fmt.Printf("[AuthBridge] Decrypted Response: %s\n", decryptedResponse)

	// Parse decrypted response
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(decryptedResponse), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal decrypted response: %v", err)
	}

	return result, nil
}

// getMockResponse returns mock responses for different document types
func (c *AuthBridgeClient) getMockResponse(req DocumentRequest) (map[string]interface{}, error) {
	docType := req.GetDocType()

	switch docType {
	case AADHAAR:
		// Check if this is LINK or STATUS action
		if linkReq, ok := req.(*AadhaarLinkRequest); ok && linkReq.Action == "LINK" {
			// Mock Aadhaar LINK response (DigiLocker URL generation)
			// Use the redirect URL provided in the request for mock testing
			mockURL := linkReq.RedirectURL
			if mockURL == "" {
				mockURL = "http://localhost:8888/v1/user/onboard/aadhaar/callback"
			}
			return map[string]interface{}{
				"status":      float64(1),
				"message":     "Aadhaar verification link generated successfully",
				"ts_trans_id": "TS_" + linkReq.TransID + "_MOCK",
				"data": map[string]interface{}{
					"url":        mockURL,
					"expires_at": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
				},
			}, nil
		} else if statusReq, ok := req.(*AadhaarStatusRequest); ok && statusReq.Action == "STATUS" {
			// Mock Aadhaar STATUS response (verification completed)
			return map[string]interface{}{
				"status":  float64(1),
				"message": "Aadhaar verification completed successfully",
				"data": map[string]interface{}{
					"name":           "John Doe",
					"dob":            "01-01-1990",
					"gender":         "M",
					"address":        "123 Mock Street, Mock City, Mock State - 123456",
					"aadhaar_number": "XXXX-XXXX-1234",
					"photo_link":     "https://mock-storage.example.com/photo.jpg",
					"verified":       true,
					"verified_at":    time.Now().Format(time.RFC3339),
				},
			}, nil
		}

	case PAN_COM:
		// Mock PAN Comprehensive response
		return map[string]interface{}{
			"status":  float64(1),
			"message": "PAN verification successful",
			"data": map[string]interface{}{
				"pan_number":     "ABCDE1234F",
				"name":           "John Doe",
				"category":       "Individual",
				"status":         "Active",
				"is_valid":       true,
				"aadhaar_linked": true,
			},
		}, nil

	case PAN_AADHAAR:
		// Mock PAN-Aadhaar link verification
		return map[string]interface{}{
			"status":  float64(1),
			"message": "PAN-Aadhaar link verified successfully",
			"data": map[string]interface{}{
				"pan_number":     "ABCDE1234F",
				"aadhaar_number": "XXXX-XXXX-1234",
				"linked":         true,
				"link_date":      "2020-01-01",
			},
		}, nil

	case GSTIN:
		// Mock GSTIN verification
		return map[string]interface{}{
			"status":  float64(1),
			"message": "GSTIN verification successful",
			"data": map[string]interface{}{
				"gstin":              "29ABCDE1234F1Z5",
				"legal_name":         "ABC Private Limited",
				"trade_name":         "ABC Corp",
				"status":             "Active",
				"registration_date":  "2018-07-01",
				"state":              "Karnataka",
				"taxpayer_type":      "Regular",
				"business_type":      "Private Limited Company",
				"principal_address":  "123 Business Park, Bangalore - 560001",
				"directors": []interface{}{
					map[string]interface{}{
						"name": "John Doe",
						"din":  "12345678",
					},
				},
			},
		}, nil

	case CIN:
		// Mock CIN verification
		return map[string]interface{}{
			"status":  float64(1),
			"message": "CIN verification successful",
			"data": map[string]interface{}{
				"cin":                  "U12345KA2018PTC123456",
				"company_name":         "ABC Private Limited",
				"registration_date":    "2018-01-15",
				"company_status":       "Active",
				"company_category":     "Company limited by shares",
				"company_sub_category": "Non-govt company",
				"authorized_capital":   "10000000",
				"paid_up_capital":      "5000000",
				"registered_address":   "123 Business Park, Bangalore - 560001",
			},
		}, nil

	case PAN:
		// Mock basic PAN verification
		return map[string]interface{}{
			"status":  float64(1),
			"message": "PAN verification successful",
			"data": map[string]interface{}{
				"pan_number": "ABCDE1234F",
				"name":       "John Doe",
				"is_valid":   true,
			},
		}, nil

	case BANK_VERIFY:
		// Mock Bank Account verification
		return map[string]interface{}{
			"msg": map[string]interface{}{
				"accountType":       "SAVINGS",
				"beneficiaryName":   "DARAHAS KOPPARAPU",
				"clientRefId":       "02112025859128",
				"responseCode":      "00",
				"responseMsg":       "SUCCESS",
				"rrn":               "530613010252",
				"status":            "SUCCESS",
				"transDateTime":     "02/11/2025 13:45:04",
			},
			"status":    float64(1),
			"transId":   "12",
			"tsTransId": "TS-UXR-307481",
		}, nil

	default:
		// Generic mock response for other document types
		return map[string]interface{}{
			"status":  float64(1),
			"message": "Document verification successful (mock)",
			"data": map[string]interface{}{
				"verified":    true,
				"verified_at": time.Now().Format(time.RFC3339),
				"random_data": map[string]interface{}{
					"field1": "value1",
					"field2": "value2",
					"field3": 12345,
				},
			},
		}, nil
	}

	return nil, fmt.Errorf("unsupported request type")
}
