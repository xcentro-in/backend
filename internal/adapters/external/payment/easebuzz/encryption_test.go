package easebuzz

import (
	"encoding/base64"
	"testing"
)

// TestEncryptCardField tests card field encryption using AES-256-CBC
func TestEncryptCardField(t *testing.T) {
	merchantKey := "test_merchant_key_12345"
	salt := "test_salt_67890"

	tests := []struct {
		name      string
		plaintext string
		wantErr   bool
	}{
		{
			name:      "Card token encryption",
			plaintext: "tok_1234567890abcdef",
			wantErr:   false,
		},
		{
			name:      "CVV encryption",
			plaintext: "123",
			wantErr:   false,
		},
		{
			name:      "Expiry date encryption",
			plaintext: "12/25",
			wantErr:   false,
		},
		{
			name:      "Long string encryption",
			plaintext: "this is a very long card token string that should be encrypted properly using AES-256",
			wantErr:   false,
		},
		{
			name:      "Special characters",
			plaintext: "token@#$%^&*()_+-=[]{}|;:',.<>?",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted, err := EncryptCardField(tt.plaintext, merchantKey, salt)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify encrypted output is base64
			if !isValidBase64(encrypted) {
				t.Errorf("encrypted output is not valid base64: %s", encrypted)
			}

			// Verify encrypted output is different from plaintext
			if encrypted == tt.plaintext {
				t.Error("encrypted output should differ from plaintext")
			}

			// Verify consistency - same input should produce same output (fixed IV)
			encrypted2, err := EncryptCardField(tt.plaintext, merchantKey, salt)
			if err != nil {
				t.Fatalf("second encryption failed: %v", err)
			}
			if encrypted != encrypted2 {
				t.Error("encryption should be deterministic with fixed IV")
			}
		})
	}
}

// TestEncryptCardField_EmptyInputs tests error handling for empty inputs
func TestEncryptCardField_EmptyInputs(t *testing.T) {
	tests := []struct {
		name        string
		plaintext   string
		merchantKey string
		salt        string
		wantErr     bool
	}{
		{
			name:        "Empty plaintext",
			plaintext:   "",
			merchantKey: "key",
			salt:        "salt",
			wantErr:     true,
		},
		{
			name:        "Empty merchantKey",
			plaintext:   "data",
			merchantKey: "",
			salt:        "salt",
			wantErr:     true,
		},
		{
			name:        "Empty salt",
			plaintext:   "data",
			merchantKey: "key",
			salt:        "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := EncryptCardField(tt.plaintext, tt.merchantKey, tt.salt)
			if tt.wantErr && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestEncryptCardField_DifferentKeys tests that different keys produce different ciphertexts
func TestEncryptCardField_DifferentKeys(t *testing.T) {
	plaintext := "sensitive_card_token"
	salt := "common_salt"

	encrypted1, err := EncryptCardField(plaintext, "merchant_key_1", salt)
	if err != nil {
		t.Fatalf("first encryption failed: %v", err)
	}

	encrypted2, err := EncryptCardField(plaintext, "merchant_key_2", salt)
	if err != nil {
		t.Fatalf("second encryption failed: %v", err)
	}

	if encrypted1 == encrypted2 {
		t.Error("different merchant keys should produce different ciphertexts")
	}
}

// TestEncryptCardField_DifferentSalts tests that different salts produce different ciphertexts
func TestEncryptCardField_DifferentSalts(t *testing.T) {
	plaintext := "sensitive_card_token"
	merchantKey := "common_merchant_key"

	encrypted1, err := EncryptCardField(plaintext, merchantKey, "salt_1")
	if err != nil {
		t.Fatalf("first encryption failed: %v", err)
	}

	encrypted2, err := EncryptCardField(plaintext, merchantKey, "salt_2")
	if err != nil {
		t.Fatalf("second encryption failed: %v", err)
	}

	if encrypted1 == encrypted2 {
		t.Error("different salts should produce different ciphertexts (different IVs)")
	}
}

// TestEncryptFullPayload tests full JSON payload encryption for tokenization API
func TestEncryptFullPayload(t *testing.T) {
	merchantKey := "test_merchant_key"
	salt := "test_salt"

	// Derive keys using the same method as production
	sKey := deriveAES256Key(merchantKey)
	sIV := deriveFixedIV(salt)

	tests := []struct {
		name        string
		jsonPayload string
		wantErr     bool
	}{
		{
			name:        "Valid JSON payload",
			jsonPayload: `{"card_number":"4111111111111111","amount":"1.00"}`,
			wantErr:     false,
		},
		{
			name:        "Complex JSON with nested objects",
			jsonPayload: `{"user":{"name":"John","email":"john@example.com"},"card":{"number":"4111111111111111"}}`,
			wantErr:     false,
		},
		{
			name:        "Empty JSON object",
			jsonPayload: `{}`,
			wantErr:     false,
		},
		{
			name:        "Single character",
			jsonPayload: `a`,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted, err := EncryptFullPayload(tt.jsonPayload, sKey, sIV)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify base64 encoding
			if !isValidBase64(encrypted) {
				t.Errorf("encrypted payload is not valid base64")
			}

			// Verify we can decrypt it back
			decrypted, err := DecryptFullPayload(encrypted, sKey, sIV)
			if err != nil {
				t.Fatalf("decryption failed: %v", err)
			}

			if decrypted != tt.jsonPayload {
				t.Errorf("decrypted payload doesn't match original:\n  original:  %s\n  decrypted: %s", tt.jsonPayload, decrypted)
			}
		})
	}
}

// TestEncryptFullPayload_InvalidInputs tests error handling for invalid inputs
func TestEncryptFullPayload_InvalidInputs(t *testing.T) {
	validKey := deriveAES256Key("key")
	validIV := deriveFixedIV("salt")

	tests := []struct {
		name    string
		payload string
		sKey    []byte
		sIV     []byte
		wantErr bool
	}{
		{
			name:    "Empty payload",
			payload: "",
			sKey:    validKey,
			sIV:     validIV,
			wantErr: true,
		},
		{
			name:    "Invalid key length (too short)",
			payload: "test",
			sKey:    []byte("short"),
			sIV:     validIV,
			wantErr: true,
		},
		{
			name:    "Invalid IV length (too short)",
			payload: "test",
			sKey:    validKey,
			sIV:     []byte("short"),
			wantErr: true,
		},
		{
			name:    "Nil key",
			payload: "test",
			sKey:    nil,
			sIV:     validIV,
			wantErr: true,
		},
		{
			name:    "Nil IV",
			payload: "test",
			sKey:    validKey,
			sIV:     nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := EncryptFullPayload(tt.payload, tt.sKey, tt.sIV)
			if tt.wantErr && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestDecryptFullPayload tests payload decryption
func TestDecryptFullPayload(t *testing.T) {
	merchantKey := "test_merchant_key"
	salt := "test_salt"
	sKey := deriveAES256Key(merchantKey)
	sIV := deriveFixedIV(salt)

	tests := []struct {
		name      string
		plaintext string
	}{
		{
			name:      "Simple string",
			plaintext: "hello world",
		},
		{
			name:      "JSON payload",
			plaintext: `{"key":"value"}`,
		},
		{
			name:      "Long text",
			plaintext: "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encrypt
			encrypted, err := EncryptFullPayload(tt.plaintext, sKey, sIV)
			if err != nil {
				t.Fatalf("encryption failed: %v", err)
			}

			// Decrypt
			decrypted, err := DecryptFullPayload(encrypted, sKey, sIV)
			if err != nil {
				t.Fatalf("decryption failed: %v", err)
			}

			// Verify match
			if decrypted != tt.plaintext {
				t.Errorf("decryption mismatch:\n  original:  %s\n  decrypted: %s", tt.plaintext, decrypted)
			}
		})
	}
}

// TestDecryptFullPayload_InvalidInputs tests decryption error handling
func TestDecryptFullPayload_InvalidInputs(t *testing.T) {
	validKey := deriveAES256Key("key")
	validIV := deriveFixedIV("salt")

	// Create a valid encrypted payload for wrong key test
	validEncrypted, _ := EncryptFullPayload("test payload", validKey, validIV)

	tests := []struct {
		name      string
		encrypted string
		sKey      []byte
		sIV       []byte
		wantErr   bool
	}{
		{
			name:      "Empty encrypted data",
			encrypted: "",
			sKey:      validKey,
			sIV:       validIV,
			wantErr:   true,
		},
		{
			name:      "Invalid base64",
			encrypted: "not-valid-base64!!!",
			sKey:      validKey,
			sIV:       validIV,
			wantErr:   true,
		},
		{
			name:      "Wrong key",
			encrypted: validEncrypted, // valid encrypted data but wrong key
			sKey:      deriveAES256Key("wrong_key"),
			sIV:       validIV,
			wantErr:   true,
		},
		{
			name:      "Invalid key length",
			encrypted: validEncrypted,
			sKey:      []byte("short"),
			sIV:       validIV,
			wantErr:   true,
		},
		{
			name:      "Invalid IV length",
			encrypted: validEncrypted,
			sKey:      validKey,
			sIV:       []byte("short"),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecryptFullPayload(tt.encrypted, tt.sKey, tt.sIV)
			if tt.wantErr && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestDeriveAES256Key tests AES-256 key derivation
func TestDeriveAES256Key(t *testing.T) {
	tests := []struct {
		merchantKey string
		wantLen     int
	}{
		{
			merchantKey: "test_key",
			wantLen:     32, // AES-256 requires 32 bytes
		},
		{
			merchantKey: "very_long_merchant_key_with_many_characters_123456",
			wantLen:     32,
		},
		{
			merchantKey: "a",
			wantLen:     32,
		},
	}

	for _, tt := range tests {
		t.Run(tt.merchantKey, func(t *testing.T) {
			key := deriveAES256Key(tt.merchantKey)
			if len(key) != tt.wantLen {
				t.Errorf("key length = %d, want %d", len(key), tt.wantLen)
			}

			// Verify consistency
			key2 := deriveAES256Key(tt.merchantKey)
			if string(key) != string(key2) {
				t.Error("key derivation should be deterministic")
			}
		})
	}
}

// TestDeriveAES256Key_DifferentKeys tests that different merchant keys produce different derived keys
func TestDeriveAES256Key_DifferentKeys(t *testing.T) {
	key1 := deriveAES256Key("merchant_key_1")
	key2 := deriveAES256Key("merchant_key_2")

	if string(key1) == string(key2) {
		t.Error("different merchant keys should produce different derived keys")
	}
}

// TestDeriveFixedIV tests IV derivation from salt
func TestDeriveFixedIV(t *testing.T) {
	tests := []struct {
		salt    string
		wantLen int
	}{
		{
			salt:    "test_salt",
			wantLen: 16, // AES block size is 16 bytes
		},
		{
			salt:    "very_long_salt_string_with_many_characters",
			wantLen: 16,
		},
		{
			salt:    "s",
			wantLen: 16,
		},
	}

	for _, tt := range tests {
		t.Run(tt.salt, func(t *testing.T) {
			iv := deriveFixedIV(tt.salt)
			if len(iv) != tt.wantLen {
				t.Errorf("IV length = %d, want %d", len(iv), tt.wantLen)
			}

			// Verify consistency
			iv2 := deriveFixedIV(tt.salt)
			if string(iv) != string(iv2) {
				t.Error("IV derivation should be deterministic")
			}
		})
	}
}

// TestDeriveFixedIV_DifferentSalts tests that different salts produce different IVs
func TestDeriveFixedIV_DifferentSalts(t *testing.T) {
	iv1 := deriveFixedIV("salt_1")
	iv2 := deriveFixedIV("salt_2")

	if string(iv1) == string(iv2) {
		t.Error("different salts should produce different IVs")
	}
}

// TestPKCS7Padding tests PKCS7 padding and unpadding
func TestPKCS7Padding(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		blockSize int
	}{
		{
			name:      "Short data needing padding",
			data:      "Hello",
			blockSize: 16,
		},
		{
			name:      "Exact block size",
			data:      "1234567890123456", // 16 bytes
			blockSize: 16,
		},
		{
			name:      "Empty data",
			data:      "",
			blockSize: 16,
		},
		{
			name:      "Long data",
			data:      "This is a long string that needs padding",
			blockSize: 16,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte(tt.data)

			// Pad
			padded := pkcs7Pad(data, tt.blockSize)

			// Verify padded length is multiple of block size
			if len(padded)%tt.blockSize != 0 {
				t.Errorf("padded length %d is not multiple of block size %d", len(padded), tt.blockSize)
			}

			// Unpad
			unpadded, err := pkcs7Unpad(padded)
			if err != nil {
				t.Fatalf("unpadding failed: %v", err)
			}

			// Verify original data is recovered
			if string(unpadded) != tt.data {
				t.Errorf("unpadded data doesn't match original:\n  original: %q\n  unpadded: %q", tt.data, string(unpadded))
			}
		})
	}
}

// TestPKCS7Unpad_InvalidPadding tests unpadding with invalid padding
func TestPKCS7Unpad_InvalidPadding(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "Empty data",
			data: []byte{},
		},
		{
			name: "Invalid padding value",
			data: []byte{1, 2, 3, 4, 5, 20}, // padding = 20 but length = 6
		},
		{
			name: "Zero padding",
			data: []byte{1, 2, 3, 0},
		},
		{
			name: "Inconsistent padding",
			data: []byte{1, 2, 3, 4, 5, 5, 6}, // last byte says 6 but second-to-last is 5
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := pkcs7Unpad(tt.data)
			if err == nil {
				t.Error("expected error for invalid padding but got nil")
			}
		})
	}
}

// TestRealWorldTokenizationScenario tests a realistic Easebuzz tokenization flow
func TestRealWorldTokenizationScenario(t *testing.T) {
	// Simulate Easebuzz tokenization API encryption
	merchantKey := "EASEBUZZ_MERCHANT_KEY_PROD"
	salt := "EASEBUZZ_SALT_PROD"

	// Prepare tokenization request
	tokenizationJSON := `{
		"card_number": "4111111111111111",
		"card_cvv": "123",
		"card_expiry_month": "12",
		"card_expiry_year": "25",
		"customer_email": "customer@example.com",
		"customer_phone": "9876543210",
		"amount": "1.00"
	}`

	// Derive encryption keys
	sKey := deriveAES256Key(merchantKey)
	sIV := deriveFixedIV(salt)

	// Encrypt payload
	encryptedPayload, err := EncryptFullPayload(tokenizationJSON, sKey, sIV)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	// Verify encrypted payload is base64
	if !isValidBase64(encryptedPayload) {
		t.Error("encrypted payload should be valid base64")
	}

	// Simulate sending to Easebuzz API (encrypted as "enc_data" field)
	t.Logf("Encrypted payload (first 50 chars): %s...", truncate(encryptedPayload, 50))

	// Simulate response from Easebuzz (decrypt if needed)
	decrypted, err := DecryptFullPayload(encryptedPayload, sKey, sIV)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}

	if decrypted != tokenizationJSON {
		t.Error("decrypted response doesn't match original request")
	}
}

// Helper functions

// isValidBase64 checks if string is valid base64
func isValidBase64(s string) bool {
	_, err := base64.StdEncoding.DecodeString(s)
	return err == nil
}

// truncate truncates string to max length
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// TestIsValidBase64 tests the helper function
func TestIsValidBase64(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"SGVsbG8gV29ybGQ=", true},         // "Hello World"
		{"dGVzdA==", true},                 // "test"
		{"not-valid-base64!!!", false},     // invalid chars
		{"", true},                         // empty is valid base64
		{"YWJjZGVm", true},                 // "abcdef"
		{"invalid base64 with spaces", false},
	}

	for _, tt := range tests {
		got := isValidBase64(tt.input)
		if got != tt.want {
			t.Errorf("isValidBase64(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// Benchmark tests

// BenchmarkEncryptCardField benchmarks card field encryption
func BenchmarkEncryptCardField(b *testing.B) {
	merchantKey := "benchmark_merchant_key"
	salt := "benchmark_salt"
	plaintext := "card_token_1234567890"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = EncryptCardField(plaintext, merchantKey, salt)
	}
}

// BenchmarkEncryptFullPayload benchmarks full payload encryption
func BenchmarkEncryptFullPayload(b *testing.B) {
	merchantKey := "benchmark_merchant_key"
	salt := "benchmark_salt"
	sKey := deriveAES256Key(merchantKey)
	sIV := deriveFixedIV(salt)
	payload := `{"card_number":"4111111111111111","amount":"100.00","email":"test@example.com"}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = EncryptFullPayload(payload, sKey, sIV)
	}
}

// BenchmarkDecryptFullPayload benchmarks full payload decryption
func BenchmarkDecryptFullPayload(b *testing.B) {
	merchantKey := "benchmark_merchant_key"
	salt := "benchmark_salt"
	sKey := deriveAES256Key(merchantKey)
	sIV := deriveFixedIV(salt)
	payload := `{"card_number":"4111111111111111","amount":"100.00"}`

	// Pre-encrypt
	encrypted, _ := EncryptFullPayload(payload, sKey, sIV)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = DecryptFullPayload(encrypted, sKey, sIV)
	}
}
