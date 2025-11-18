package jiopay

import (
	"bytes"
	"io"
	"net/http"
	"strings"
)

// MockTransport is a custom RoundTripper for mocking HTTP responses in tests
// Usage:
//   transport := &MockTransport{}
//   transport.AddRoute("POST", "/initiateSale", mockResponse, 200)
//   client := &http.Client{Transport: transport}
type MockTransport struct {
	// Routes maps "METHOD URL" to a response
	Routes map[string]*MockResponse
}

// MockResponse represents a mocked HTTP response
type MockResponse struct {
	StatusCode int
	Body       string
	Headers    map[string]string
}

// NewMockTransport creates a new mock transport
func NewMockTransport() *MockTransport {
	return &MockTransport{
		Routes: make(map[string]*MockResponse),
	}
}

// AddRoute adds a mock response for a specific HTTP method and URL path
// Example: transport.AddRoute("POST", "/initiateSale", `{"status":"success"}`, 200)
func (m *MockTransport) AddRoute(method, urlPath, responseBody string, statusCode int) {
	key := method + " " + urlPath
	m.Routes[key] = &MockResponse{
		StatusCode: statusCode,
		Body:       responseBody,
		Headers:    make(map[string]string),
	}
}

// AddRouteWithHeaders adds a mock response with custom headers
func (m *MockTransport) AddRouteWithHeaders(method, urlPath, responseBody string, statusCode int, headers map[string]string) {
	key := method + " " + urlPath
	m.Routes[key] = &MockResponse{
		StatusCode: statusCode,
		Body:       responseBody,
		Headers:    headers,
	}
}

// RoundTrip implements http.RoundTripper interface
// This is called by http.Client for every HTTP request
func (m *MockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Extract path from URL (remove query params for matching)
	path := req.URL.Path
	key := req.Method + " " + path

	// Check if we have a mock response for this route
	mockResp, exists := m.Routes[key]
	if !exists {
		// Fallback: check if any key contains this path (partial matching)
		for routeKey, resp := range m.Routes {
			if strings.Contains(routeKey, path) || strings.Contains(path, strings.Split(routeKey, " ")[1]) {
				mockResp = resp
				exists = true
				break
			}
		}
	}

	if !exists {
		// No mock found - return 404
		return &http.Response{
			StatusCode: 404,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error":"route not mocked"}`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	}

	// Build response headers
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	for k, v := range mockResp.Headers {
		headers.Set(k, v)
	}

	// Return mocked response
	return &http.Response{
		StatusCode: mockResp.StatusCode,
		Body:       io.NopCloser(bytes.NewBufferString(mockResp.Body)),
		Header:     headers,
		Request:    req,
	}, nil
}
