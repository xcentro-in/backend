package jiopay

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// SecureHashGenerator handles HMAC-SHA256 hash generation and validation for JioPay
//
// JioPay uses HMAC-SHA256 for request/response integrity:
//  1. Sort all fields alphabetically by key
//  2. Concatenate all VALUES in sorted order (no separators)
//  3. Calculate HMAC-SHA256 with apiKey as secret
//  4. Return lowercase hex string (64 characters)
//
// Usage:
//   - Outgoing requests: GenerateForRequest() calculates hash to inject into request body
//   - Incoming responses: ValidateFromResponse() validates secureHash from callback/webhook
type SecureHashGenerator struct {
	apiKey string // JIOPAY_API_KEY from environment
}

// NewSecureHashGenerator creates a new hash generator
func NewSecureHashGenerator(apiKey string) *SecureHashGenerator {
	return &SecureHashGenerator{
		apiKey: apiKey,
	}
}

// GenerateForRequest generates HMAC-SHA256 hash for outgoing API requests
//
// This method is used when making requests TO JioPay (e.g., initiateSale, initiateRefund).
// The generated hash should be added to the request body as "secureHash" field.
//
// Parameters:
//   - fieldMap: Map of all request fields (excluding secureHash itself)
//
// Returns:
//   - Lowercase hex string (64 characters)
//
// Example:
//
//	hashGen := NewSecureHashGenerator(apiKey)
//	fieldMap := map[string]string{
//	    "merchantId": "123243",
//	    "amount": "500.00",
//	    "merchantTxnNo": "TXN123",
//	}
//	hash := hashGen.GenerateForRequest(fieldMap)
//	request.SecureHash = hash
func (h *SecureHashGenerator) GenerateForRequest(fieldMap map[string]string) string {
	fmt.Printf("🔐 [Hash Generator] Generating hash for outgoing request\n")
	fmt.Printf("📊 [Hash Generator] Fields count: %d\n", len(fieldMap))

	// Compute HMAC-SHA256 hash
	hash := h.computeHash(fieldMap)

	fmt.Printf("✅ [Hash Generator] Generated hash: %s...\n", hash[:20])

	return hash
}

// ValidateFromResponse validates HMAC-SHA256 signature from incoming callbacks/webhooks
//
// This method is used when receiving responses FROM JioPay (e.g., callback, webhook).
// The secureHash field in the response is validated against the expected hash.
//
// Validation steps:
//  1. Parse JSON payload to extract all fields
//  2. Build field map (excluding secureHash field itself)
//  3. Generate expected hash using computeHash()
//  4. Compare with provided signature using constant-time comparison
//
// Parameters:
//   - payload: Raw JSON payload from JioPay
//   - signature: The secureHash value from the response
//
// Returns:
//   - nil if signature is valid
//   - error if signature is invalid or parsing fails
//
// Example:
//
//	hashGen := NewSecureHashGenerator(apiKey)
//	err := hashGen.ValidateFromResponse(bodyBytes, callback.SecureHash)
//	if err != nil {
//	    return fmt.Errorf("invalid signature: %w", err)
//	}
func (h *SecureHashGenerator) ValidateFromResponse(payload []byte, signature string) error {
	fmt.Printf("🔐 [Hash Generator] Validating hash from incoming response\n")
	fmt.Printf("📝 [Hash Generator] Payload length: %d bytes\n", len(payload))
	fmt.Printf("🔏 [Hash Generator] Provided signature: %s...\n", signature[:min(20, len(signature))])

	// Step 1: Parse JSON payload and build field map (exclude secureHash)
	fieldMap, err := h.buildFieldMapFromJSON(payload, []string{"secureHash"})
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	fmt.Printf("📊 [Hash Generator] Fields for verification: %d (excluding secureHash)\n", len(fieldMap))

	// Step 2: Generate expected hash
	expectedHash := h.computeHash(fieldMap)

	fmt.Printf("🔐 [Hash Generator] Expected signature: %s...\n", expectedHash[:20])

	// Step 3: Constant-time comparison to prevent timing attacks
	if !hmac.Equal([]byte(expectedHash), []byte(signature)) {
		fmt.Printf("❌ [Hash Generator] Signature mismatch!\n")
		fmt.Printf("   Expected: %s\n", expectedHash)
		fmt.Printf("   Received: %s\n", signature)
		return fmt.Errorf("invalid signature: hash mismatch")
	}

	fmt.Println("✅ [Hash Generator] Signature verified successfully")
	return nil
}

// computeHash generates HMAC-SHA256 hash from field map
//
// Core algorithm (same for both requests and responses):
//  1. Sort field keys alphabetically
//  2. Concatenate field VALUES in sorted order (no separators)
//  3. Calculate HMAC-SHA256(apiKey, concatenated_values)
//  4. Return lowercase hex string
//
// This is the single source of truth for hash calculation.
func (h *SecureHashGenerator) computeHash(fieldMap map[string]string) string {
	// Sort keys alphabetically
	keys := make([]string, 0, len(fieldMap))
	for k := range fieldMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Concatenate values in sorted key order (no separators)
	var concatenated strings.Builder
	for _, key := range keys {
		concatenated.WriteString(fieldMap[key])
	}

	message := concatenated.String()
	fmt.Printf("🔐 [Hash Generator] HMAC message: %s\n", message)

	// Calculate HMAC-SHA256
	h256 := hmac.New(sha256.New, []byte(h.apiKey))
	h256.Write([]byte(message))

	// Return lowercase hexadecimal
	return hex.EncodeToString(h256.Sum(nil))
}

// buildFieldMapFromJSON parses JSON payload and builds field map
//
// Parameters:
//   - payload: Raw JSON bytes
//   - excludeFields: List of field names to exclude (e.g., ["secureHash"])
//
// Returns:
//   - Field map with all fields except excluded ones
//   - Error if JSON parsing fails
func (h *SecureHashGenerator) buildFieldMapFromJSON(payload []byte, excludeFields []string) (map[string]string, error) {
	// Parse JSON to generic map
	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, fmt.Errorf("json unmarshal failed: %w", err)
	}

	// Build exclusion set for fast lookup
	excludeSet := make(map[string]bool)
	for _, field := range excludeFields {
		excludeSet[field] = true
	}

	// Build field map (convert all values to strings)
	fieldMap := make(map[string]string)
	for key, value := range data {
		// Skip excluded fields and nil values
		if excludeSet[key] || value == nil {
			continue
		}

		// Convert value to string for hash calculation
		fieldMap[key] = fmt.Sprintf("%v", value)
	}

	return fieldMap, nil
}
