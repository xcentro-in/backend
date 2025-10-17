package authbridge

import (
	"fmt"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

// COMMENTED OUT - HTTP CLIENT CODE FOR REAL API CALLS
// Uncomment this section when switching back to real AuthBridge API calls
/*
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

// Post performs HTTP POST request
func (c *HTTPClient) Post(url string, headers map[string]string, body []byte) ([]byte, error) {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Set headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}
*/

// AuthBridgeClient is the main client for document verification
type AuthBridgeClient struct {
	config *Config
}

// NewClient creates a new AuthBridge client
func NewClient() (*AuthBridgeClient, error) {
	config, err := loadConfig()
	if err != nil {
		return nil, err
	}

	return &AuthBridgeClient{
		config: config,
	}, nil
}

// loadConfig loads configuration from environment
func loadConfig() (*Config, error) {
	var config struct {
		Username string `env:"TRUTHSCREEN_USERNAME" env-required:"true"`
		Password string `env:"TRUTHSCREEN_PASSWORD" env-required:"true"`
	}

	if err := cleanenv.ReadEnv(&config); err != nil {
		return nil, fmt.Errorf("failed to load config: %v", err)
	}

	return &Config{
		Username: config.Username,
		Password: config.Password,
	}, nil
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

	// TEMPORARY: Return mock responses based on document type
	return c.getMockResponse(req)
}

// getMockResponse returns mock responses for different document types
func (c *AuthBridgeClient) getMockResponse(req DocumentRequest) (map[string]interface{}, error) {
	docType := req.GetDocType()

	switch docType {
	case AADHAAR:
		// Check if this is LINK or STATUS action
		if linkReq, ok := req.(*AadhaarLinkRequest); ok && linkReq.Action == "LINK" {
			// Mock Aadhaar LINK response (DigiLocker URL generation)
			return map[string]interface{}{
				"status":      float64(1),
				"message":     "Aadhaar verification link generated successfully",
				"ts_trans_id": "TS_" + linkReq.TransID + "_MOCK",
				"data": map[string]interface{}{
					"url":        "http://localhost:8888/v1/health",
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
				"cin":                "U12345KA2018PTC123456",
				"company_name":       "ABC Private Limited",
				"registration_date":  "2018-01-15",
				"company_status":     "Active",
				"company_category":   "Company limited by shares",
				"company_sub_category": "Non-govt company",
				"authorized_capital": "10000000",
				"paid_up_capital":    "5000000",
				"registered_address": "123 Business Park, Bangalore - 560001",
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

/*
===============================================================================
REAL API IMPLEMENTATION (COMMENTED OUT)
===============================================================================

To restore real AuthBridge API calls:

1. Uncomment the HTTP client code at the top of the file (lines 12-64)
2. Add back these imports:
   - "bytes"
   - "encoding/json"
   - "io"
   - "net/http"
   - "sync"
   - "xcentro.in/internal/shared/crypto"

3. Update AuthBridgeClient struct to include httpClient:
   type AuthBridgeClient struct {
       config     *Config
       httpClient *HTTPClient
   }

4. Update NewClient to initialize httpClient:
   return &AuthBridgeClient{
       config:     config,
       httpClient: getHTTPClient(),
   }, nil

5. Replace the getMockResponse call in SearchDocument with this code:

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

   // Prepare headers
   headers := map[string]string{
       "username": c.config.Username,
   }

   fmt.Printf("Making request to: %s\n", endpoint)

   // Make HTTP request
   responseBody, err := c.httpClient.Post(endpoint, headers, requestBody)
   if err != nil {
       return nil, fmt.Errorf("HTTP request failed: %v", err)
   }

   // Parse response
   var apiResp APIResponse
   if err := json.Unmarshal(responseBody, &apiResp); err != nil {
       return nil, fmt.Errorf("failed to unmarshal API response: %v", err)
   }

   // Decrypt response
   decryptedResponse, err := crypto.DecryptAES256CBC(apiResp.ResponseData, c.config.Password)
   if err != nil {
       return nil, fmt.Errorf("decryption failed: %v", err)
   }

   // Parse decrypted response
   var result map[string]interface{}
   if err := json.Unmarshal([]byte(decryptedResponse), &result); err != nil {
       return nil, fmt.Errorf("failed to unmarshal decrypted response: %v", err)
   }

   return result, nil

6. Remove or comment out the getMockResponse function

===============================================================================
*/
