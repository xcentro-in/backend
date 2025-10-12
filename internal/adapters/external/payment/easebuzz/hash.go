package easebuzz

import (
	"crypto/sha512"
	"crypto/subtle"
	"fmt"
)

// SecureHashGenerator handles SHA-512 hash generation and validation for Easebuzz
//
// Easebuzz uses SHA-512 with a fixed field order and pipe separators:
//   - Forward (request): key|txnid|amount|productinfo|firstname|email|udf1|...|udf10|salt
//   - Reverse (response): salt|status|udf10|...|udf1|email|firstname|productinfo|amount|txnid|key
//
// Key differences from JioPay:
//  1. SHA-512 (not HMAC-SHA256)
//  2. Fixed field order with pipes (not alphabetically sorted)
//  3. Includes field names in some contexts
//  4. Supports 10 user-defined fields (UDFs)
//
// Usage:
//   - Outgoing requests: GenerateForRequest() calculates hash to inject into request
//   - Incoming responses: ValidateFromResponse() validates hash from callback/webhook
type SecureHashGenerator struct {
	merchantKey string // EASEBUZZ_MERCHANT_KEY from environment
	salt        string // EASEBUZZ_SALT from environment
}

// RequestHashParams contains all fields needed for forward hash generation
// All UDF fields are optional (use empty string if not needed)
type RequestHashParams struct {
	TxnID       string // Transaction ID
	Amount      string // Amount in rupees (e.g., "500.00")
	ProductInfo string // Product/payment description
	FirstName   string // Customer first name
	Email       string // Customer email
	UDF1        string // User-defined field 1 (optional)
	UDF2        string // User-defined field 2 (optional)
	UDF3        string // User-defined field 3 (optional)
	UDF4        string // User-defined field 4 (optional)
	UDF5        string // User-defined field 5 (optional)
	UDF6        string // User-defined field 6 (optional)
	UDF7        string // User-defined field 7 (optional)
	UDF8        string // User-defined field 8 (optional)
	UDF9        string // User-defined field 9 (optional)
	UDF10       string // User-defined field 10 (optional)
}

// NewSecureHashGenerator creates a new hash generator for Easebuzz
func NewSecureHashGenerator(merchantKey, salt string) *SecureHashGenerator {
	return &SecureHashGenerator{
		merchantKey: merchantKey,
		salt:        salt,
	}
}

// GenerateForRequest generates SHA-512 hash for outgoing API requests
//
// Hash Sequence (forward):
// key|txnid|amount|productinfo|firstname|email|udf1|udf2|udf3|udf4|udf5|udf6|udf7|udf8|udf9|udf10|salt
//
// Parameters:
//   - params: RequestHashParams with all required and optional fields
//
// Returns:
//   - Lowercase hex string (128 characters)
//
// Example:
//
//	hashGen := NewSecureHashGenerator(merchantKey, salt)
//	params := RequestHashParams{
//	    TxnID:       "TXN123",
//	    Amount:      "500.00",
//	    ProductInfo: "Payment for order",
//	    FirstName:   "John",
//	    Email:       "john@example.com",
//	    UDF1:        "custom_value",  // Optional
//	    // UDF2-10 can be empty strings
//	}
//	hash := hashGen.GenerateForRequest(params)
//	// Inject hash into request body
func (h *SecureHashGenerator) GenerateForRequest(params RequestHashParams) string {
	fmt.Printf("🔐 [Easebuzz Hash] Generating forward hash for outgoing request\n")
	fmt.Printf("📊 [Easebuzz Hash] TxnID: %s, Amount: %s\n", params.TxnID, params.Amount)

	// Build hash sequence in fixed order
	// Format: key|txnid|amount|productinfo|firstname|email|udf1|...|udf10|salt
	sequence := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
		h.merchantKey,
		params.TxnID,
		params.Amount,
		params.ProductInfo,
		params.FirstName,
		params.Email,
		params.UDF1,
		params.UDF2,
		params.UDF3,
		params.UDF4,
		params.UDF5,
		params.UDF6,
		params.UDF7,
		params.UDF8,
		params.UDF9,
		params.UDF10,
		h.salt,
	)

	fmt.Printf("🔐 [Easebuzz Hash] Hash sequence: %s\n", sequence)

	// Compute SHA-512 hash
	hash := h.computeSHA512(sequence)

	fmt.Printf("✅ [Easebuzz Hash] Generated hash: %s...\n", hash[:20])

	return hash
}

// ValidateFromResponse validates SHA-512 hash from incoming callbacks/webhooks
//
// Hash Sequence (reverse):
// salt|status|udf10|udf9|udf8|udf7|udf6|udf5|udf4|udf3|udf2|udf1|email|firstname|productinfo|amount|txnid|key
//
// Note: UDFs are in REVERSE order (10 to 1) in the reverse hash
//
// Parameters:
//   - payload: CallbackPayload with all fields including hash
//
// Returns:
//   - nil if signature is valid
//   - error if signature is invalid or parsing fails
//
// Example:
//
//	hashGen := NewSecureHashGenerator(merchantKey, salt)
//	err := hashGen.ValidateFromResponse(callbackPayload)
//	if err != nil {
//	    return fmt.Errorf("invalid signature: %w", err)
//	}
func (h *SecureHashGenerator) ValidateFromResponse(payload CallbackPayload) error {
	fmt.Printf("🔐 [Easebuzz Hash] Validating reverse hash from incoming response\n")
	fmt.Printf("📝 [Easebuzz Hash] TxnID: %s, Status: %s\n", payload.TxnID, payload.Status)
	fmt.Printf("🔏 [Easebuzz Hash] Provided hash: %s...\n", payload.Hash[:min(20, len(payload.Hash))])

	// Build reverse hash sequence
	// Format: salt|status|udf10|udf9|...|udf1|email|firstname|productinfo|amount|txnid|key
	// Note: UDFs are in REVERSE order
	sequence := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
		h.salt,
		payload.Status,
		payload.UDF10, // UDF10 first
		payload.UDF9,
		payload.UDF8,
		payload.UDF7,
		payload.UDF6,
		payload.UDF5,
		payload.UDF4,
		payload.UDF3,
		payload.UDF2,
		payload.UDF1, // UDF1 last
		payload.Email,
		payload.FirstName,
		payload.ProductInfo,
		payload.Amount,
		payload.TxnID,
		h.merchantKey,
	)

	fmt.Printf("🔐 [Easebuzz Hash] Reverse hash sequence: %s\n", sequence)

	// Compute expected hash
	expectedHash := h.computeSHA512(sequence)

	fmt.Printf("🔐 [Easebuzz Hash] Expected hash: %s...\n", expectedHash[:20])

	// Constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(expectedHash), []byte(payload.Hash)) != 1 {
		fmt.Printf("❌ [Easebuzz Hash] Signature mismatch!\n")
		fmt.Printf("   Expected: %s\n", expectedHash)
		fmt.Printf("   Received: %s\n", payload.Hash)
		return fmt.Errorf("invalid hash signature: mismatch")
	}

	fmt.Println("✅ [Easebuzz Hash] Signature verified successfully")
	return nil
}

// computeSHA512 calculates SHA-512 hash of the input string
// Returns lowercase hexadecimal string (128 characters)
func (h *SecureHashGenerator) computeSHA512(input string) string {
	hasher := sha512.New()
	hasher.Write([]byte(input))
	hashBytes := hasher.Sum(nil)
	return fmt.Sprintf("%x", hashBytes) // Lowercase hex
}

// min returns the minimum of two integers (helper for logging)
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
