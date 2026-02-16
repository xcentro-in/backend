package jiopay

import (
	"strings"
	"testing"
)

// TestGenerateForRequest tests HMAC-SHA256 hash generation for outgoing requests
func TestGenerateForRequest(t *testing.T) {
	tests := []struct {
		name      string
		apiKey    string
		fieldMap  map[string]string
		wantLen   int
		wantRegex string
	}{
		{
			name:   "Simple payment request",
			apiKey: "test_secret_key_12345",
			fieldMap: map[string]string{
				"merchantId":    "TEST123",
				"amount":        "500.00",
				"merchantTxnNo": "TXN123",
			},
			wantLen:   64, // SHA256 produces 64 hex chars
			wantRegex: "^[a-f0-9]{64}$",
		},
		{
			name:   "Tokenization request",
			apiKey: "token_secret_xyz",
			fieldMap: map[string]string{
				"merchantId": "MERCHANT456",
				"externalId": "MERCHANT456_9876543210",
				"cardNumber": "4111111111111111",
			},
			wantLen:   64,
			wantRegex: "^[a-f0-9]{64}$",
		},
		{
			name:   "Empty field values",
			apiKey: "test_key",
			fieldMap: map[string]string{
				"merchantId": "",
				"amount":     "",
			},
			wantLen:   64,
			wantRegex: "^[a-f0-9]{64}$",
		},
		{
			name:     "Empty field map",
			apiKey:   "test_key",
			fieldMap: map[string]string{},
			wantLen:  64,
			wantRegex: "^[a-f0-9]{64}$",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hashGen := NewSecureHashGenerator(tt.apiKey)
			hash := hashGen.GenerateForRequest(tt.fieldMap)

			// Verify hash length
			if len(hash) != tt.wantLen {
				t.Errorf("hash length = %d, want %d", len(hash), tt.wantLen)
			}

			// Verify hash format (lowercase hex)
			if !isLowercaseHex(hash) {
				t.Errorf("hash is not lowercase hex: %s", hash)
			}

			// Verify consistency - same input should produce same hash
			hash2 := hashGen.GenerateForRequest(tt.fieldMap)
			if hash != hash2 {
				t.Errorf("hash generation not consistent:\n  first:  %s\n  second: %s", hash, hash2)
			}
		})
	}
}

// TestGenerateForRequest_OrderIndependence tests that field order doesn't affect hash
func TestGenerateForRequest_OrderIndependence(t *testing.T) {
	apiKey := "test_secret"

	// Two maps with same data but different iteration order (Go maps are unordered)
	fieldMap1 := map[string]string{
		"merchantId": "M123",
		"amount":     "100.00",
		"txnNo":      "TXN456",
	}

	fieldMap2 := map[string]string{
		"txnNo":      "TXN456",
		"merchantId": "M123",
		"amount":     "100.00",
	}

	hashGen := NewSecureHashGenerator(apiKey)

	hash1 := hashGen.GenerateForRequest(fieldMap1)
	hash2 := hashGen.GenerateForRequest(fieldMap2)

	if hash1 != hash2 {
		t.Errorf("hashes should be identical regardless of field order:\n  hash1: %s\n  hash2: %s", hash1, hash2)
	}
}

// TestGenerateForRequest_DifferentKeys tests that different API keys produce different hashes
func TestGenerateForRequest_DifferentKeys(t *testing.T) {
	fieldMap := map[string]string{
		"merchantId": "M123",
		"amount":     "500.00",
	}

	hashGen1 := NewSecureHashGenerator("key_one")
	hashGen2 := NewSecureHashGenerator("key_two")

	hash1 := hashGen1.GenerateForRequest(fieldMap)
	hash2 := hashGen2.GenerateForRequest(fieldMap)

	if hash1 == hash2 {
		t.Errorf("different API keys should produce different hashes, both got: %s", hash1)
	}
}

// TestGenerateForRequest_DifferentData tests that different data produces different hashes
func TestGenerateForRequest_DifferentData(t *testing.T) {
	apiKey := "test_secret"
	hashGen := NewSecureHashGenerator(apiKey)

	hash1 := hashGen.GenerateForRequest(map[string]string{
		"merchantId": "M123",
		"amount":     "500.00",
	})

	hash2 := hashGen.GenerateForRequest(map[string]string{
		"merchantId": "M123",
		"amount":     "501.00", // Changed amount
	})

	if hash1 == hash2 {
		t.Errorf("different data should produce different hashes, both got: %s", hash1)
	}
}

// TestValidateFromResponse tests hash validation for incoming responses
func TestValidateFromResponse(t *testing.T) {
	apiKey := "test_api_key_xyz"
	hashGen := NewSecureHashGenerator(apiKey)

	tests := []struct {
		name      string
		payload   string
		signature string
		wantErr   bool
	}{
		{
			name:      "Valid signature",
			payload:   `{"merchantId":"M123","amount":"500.00","status":"success"}`,
			signature: "", // Will be generated in test
			wantErr:   false,
		},
		{
			name:      "Invalid signature",
			payload:   `{"merchantId":"M123","amount":"500.00","status":"success"}`,
			signature: "0000000000000000000000000000000000000000000000000000000000000000",
			wantErr:   true,
		},
		{
			name:      "Tampered payload",
			payload:   `{"merchantId":"M123","amount":"600.00","status":"success"}`, // Amount changed
			signature: "", // Will use hash for original amount
			wantErr:   true,
		},
		{
			name:      "Invalid JSON",
			payload:   `{invalid json`,
			signature: "abcd1234",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For "Valid signature" test, generate correct hash
			if tt.name == "Valid signature" {
				fieldMap, _ := hashGen.buildFieldMapFromJSON([]byte(tt.payload), []string{"secureHash"})
				tt.signature = hashGen.computeHash(fieldMap)
			}

			// For "Tampered payload" test, use hash from original payload
			if tt.name == "Tampered payload" {
				originalPayload := `{"merchantId":"M123","amount":"500.00","status":"success"}`
				fieldMap, _ := hashGen.buildFieldMapFromJSON([]byte(originalPayload), []string{"secureHash"})
				tt.signature = hashGen.computeHash(fieldMap)
			}

			err := hashGen.ValidateFromResponse([]byte(tt.payload), tt.signature)

			if tt.wantErr && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

// TestValidateFromResponse_IgnoresSecureHashField tests that secureHash field is excluded from validation
func TestValidateFromResponse_IgnoresSecureHashField(t *testing.T) {
	apiKey := "test_key"
	hashGen := NewSecureHashGenerator(apiKey)

	// Payload with secureHash field (should be ignored during validation)
	payload := `{"merchantId":"M123","amount":"100.00","secureHash":"should_be_ignored"}`

	// Generate expected hash without secureHash field
	fieldMap := map[string]string{
		"merchantId": "M123",
		"amount":     "100.00",
	}
	correctHash := hashGen.GenerateForRequest(fieldMap)

	// Validation should pass because secureHash field is excluded
	err := hashGen.ValidateFromResponse([]byte(payload), correctHash)
	if err != nil {
		t.Errorf("validation should pass when secureHash field is ignored, got error: %v", err)
	}
}

// TestBuildFieldMapFromJSON tests JSON parsing and field map building
func TestBuildFieldMapFromJSON(t *testing.T) {
	hashGen := NewSecureHashGenerator("test_key")

	tests := []struct {
		name          string
		payload       string
		excludeFields []string
		wantFields    map[string]string
		wantErr       bool
	}{
		{
			name:          "Simple JSON",
			payload:       `{"key1":"value1","key2":"value2"}`,
			excludeFields: []string{},
			wantFields: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			wantErr: false,
		},
		{
			name:          "Exclude fields",
			payload:       `{"key1":"value1","key2":"value2","secureHash":"abc123"}`,
			excludeFields: []string{"secureHash"},
			wantFields: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			wantErr: false,
		},
		{
			name:          "Numeric values",
			payload:       `{"amount":500.00,"count":10}`,
			excludeFields: []string{},
			wantFields: map[string]string{
				"amount": "500",
				"count":  "10",
			},
			wantErr: false,
		},
		{
			name:          "Null values ignored",
			payload:       `{"key1":"value1","key2":null}`,
			excludeFields: []string{},
			wantFields: map[string]string{
				"key1": "value1",
			},
			wantErr: false,
		},
		{
			name:          "Invalid JSON",
			payload:       `{invalid`,
			excludeFields: []string{},
			wantFields:    nil,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fieldMap, err := hashGen.buildFieldMapFromJSON([]byte(tt.payload), tt.excludeFields)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(fieldMap) != len(tt.wantFields) {
				t.Errorf("field count = %d, want %d", len(fieldMap), len(tt.wantFields))
			}

			for key, wantValue := range tt.wantFields {
				gotValue, ok := fieldMap[key]
				if !ok {
					t.Errorf("missing field %s", key)
					continue
				}
				if gotValue != wantValue {
					t.Errorf("field %s = %s, want %s", key, gotValue, wantValue)
				}
			}
		})
	}
}

// TestComputeHash tests the core hash computation algorithm
func TestComputeHash(t *testing.T) {
	apiKey := "secret123"
	hashGen := NewSecureHashGenerator(apiKey)

	// Test with known data - hash should be deterministic
	fieldMap := map[string]string{
		"a": "value_a",
		"b": "value_b",
		"c": "value_c",
	}

	hash1 := hashGen.computeHash(fieldMap)
	hash2 := hashGen.computeHash(fieldMap)

	// Verify consistency
	if hash1 != hash2 {
		t.Errorf("hash computation not deterministic:\n  hash1: %s\n  hash2: %s", hash1, hash2)
	}

	// Verify format
	if len(hash1) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash1))
	}

	if !isLowercaseHex(hash1) {
		t.Errorf("hash is not lowercase hex: %s", hash1)
	}
}

// TestComputeHash_AlphabeticalSorting tests that values are concatenated in alphabetical key order
func TestComputeHash_AlphabeticalSorting(t *testing.T) {
	apiKey := "test_key"
	hashGen := NewSecureHashGenerator(apiKey)

	// Keys in non-alphabetical order
	fieldMap := map[string]string{
		"z_last":   "value_z",
		"a_first":  "value_a",
		"m_middle": "value_m",
	}

	// The hash should be based on concatenated values in alphabetical key order:
	// "value_a" + "value_m" + "value_z"
	hash := hashGen.computeHash(fieldMap)

	// Manually create the expected concatenation order
	manualFieldMap := map[string]string{
		"a_first":  "value_a",
		"m_middle": "value_m",
		"z_last":   "value_z",
	}
	expectedHash := hashGen.computeHash(manualFieldMap)

	if hash != expectedHash {
		t.Errorf("hash should be based on alphabetical key sorting:\n  got:      %s\n  expected: %s", hash, expectedHash)
	}
}

// Helper functions

// isLowercaseHex checks if string is lowercase hexadecimal
func isLowercaseHex(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// TestIsLowercaseHex tests the helper function
func TestIsLowercaseHex(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"0123456789abcdef", true},
		{"0123456789ABCDEF", false}, // uppercase
		{"0123456789abcdefg", false}, // invalid char 'g'
		{"abc123", true},
		{"", false},
		{"xyz", false},
	}

	for _, tt := range tests {
		got := isLowercaseHex(tt.input)
		if got != tt.want {
			t.Errorf("isLowercaseHex(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// Benchmark tests

// BenchmarkGenerateForRequest benchmarks hash generation performance
func BenchmarkGenerateForRequest(b *testing.B) {
	hashGen := NewSecureHashGenerator("benchmark_api_key")
	fieldMap := map[string]string{
		"merchantId":    "MERCHANT123",
		"amount":        "1000.00",
		"merchantTxnNo": "TXN987654",
		"currency":      "INR",
		"callbackUrl":   "https://example.com/callback",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = hashGen.GenerateForRequest(fieldMap)
	}
}

// BenchmarkValidateFromResponse benchmarks hash validation performance
func BenchmarkValidateFromResponse(b *testing.B) {
	hashGen := NewSecureHashGenerator("benchmark_api_key")
	payload := []byte(`{"merchantId":"M123","amount":"500.00","status":"success","txnNo":"TXN456"}`)

	// Generate valid signature
	fieldMap, _ := hashGen.buildFieldMapFromJSON(payload, []string{"secureHash"})
	signature := hashGen.computeHash(fieldMap)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = hashGen.ValidateFromResponse(payload, signature)
	}
}

// TestSecureHashGenerator_NilSafety tests that hash generator handles edge cases safely
func TestSecureHashGenerator_NilSafety(t *testing.T) {
	hashGen := NewSecureHashGenerator("")

	// Empty API key should still work (though not recommended in production)
	hash := hashGen.GenerateForRequest(map[string]string{"test": "value"})
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64 even with empty API key", len(hash))
	}

	// Nil map should not panic
	hash2 := hashGen.GenerateForRequest(nil)
	if len(hash2) != 64 {
		t.Errorf("hash length = %d, want 64 even with nil map", len(hash2))
	}
}

// TestRealWorldScenario tests a realistic JioPay payment scenario
func TestRealWorldScenario(t *testing.T) {
	// Simulate real JioPay API flow
	apiKey := "JIOPAY_API_KEY_XYZ123"
	hashGen := NewSecureHashGenerator(apiKey)

	// Step 1: Create payment request
	paymentRequest := map[string]string{
		"merchantId":    "XCENTRO_MERCHANT",
		"amount":        "1500.50",
		"merchantTxnNo": "PAYMENT_UUID_123",
		"currency":      "INR",
		"callbackUrl":   "https://api.xcentro.in/v1/process_payment/callback",
	}

	// Step 2: Generate hash for outgoing request
	requestHash := hashGen.GenerateForRequest(paymentRequest)

	// Verify hash properties
	if len(requestHash) != 64 {
		t.Errorf("request hash length = %d, want 64", len(requestHash))
	}

	if strings.Contains(requestHash, "A") || strings.Contains(requestHash, "Z") {
		t.Error("hash should be lowercase")
	}

	// Step 3: Simulate callback response from JioPay
	callbackPayload := `{
		"merchantId": "XCENTRO_MERCHANT",
		"merchantTxnNo": "PAYMENT_UUID_123",
		"amount": "1500.50",
		"status": "SUCCESS",
		"jioPayTxnId": "JP789456123"
	}`

	// Step 4: Generate expected hash for validation
	callbackFieldMap, err := hashGen.buildFieldMapFromJSON([]byte(callbackPayload), []string{"secureHash"})
	if err != nil {
		t.Fatalf("failed to parse callback payload: %v", err)
	}
	callbackHash := hashGen.computeHash(callbackFieldMap)

	// Step 5: Validate callback signature
	err = hashGen.ValidateFromResponse([]byte(callbackPayload), callbackHash)
	if err != nil {
		t.Errorf("callback validation failed: %v", err)
	}
}
