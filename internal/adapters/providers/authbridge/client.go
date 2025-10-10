package authbridge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
	"xcentro.in/internal/shared/crypto"
)

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

// AuthBridgeClient is the main client for document verification
type AuthBridgeClient struct {
	config     *Config
	httpClient *HTTPClient
}

// NewClient creates a new AuthBridge client with shared HTTP client
func NewClient() (*AuthBridgeClient, error) {
	config, err := loadConfig()
	if err != nil {
		return nil, err
	}

	return &AuthBridgeClient{
		config:     config,
		httpClient: getHTTPClient(), // Reuse singleton instance
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

	endpoint := GetEndpoint(docType)

	// Convert to JSON using the request's ToJSON method
	inputJSON, err := req.ToJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to convert request to JSON: %v", err)
	}

	fmt.Printf("Document Type: %s (%d)\n", GetDocumentName(docType), docType)
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
	fmt.Printf("Headers: %v\n", headers)
	fmt.Printf("Request Body: %s\n", string(requestBody))

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
}
