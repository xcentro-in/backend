package easebuzz

import (
	"crypto/sha512"
	"fmt"
	"strings"

	"xcentro.in/internal/core/payment"
)

// generateHash generates SHA-512 hash for Easebuzz API security
//
// Hash Sequence for Initiate Payment API:
// key|txnid|amount|productinfo|firstname|email|udf1|udf2|udf3|udf4|udf5|udf6|udf7|udf8|udf9|udf10|salt
//
// For empty UDF fields (which is typical):
// key|txnid|amount|productinfo|firstname|email|||||||||||salt
//
// Parameters:
//   - hashSequence: Pipe-separated values following Easebuzz hash format
//
// Returns:
//   - SHA-512 hash as lowercase hexadecimal string (128 characters)
//
// Usage:
//
//	hashInput := fmt.Sprintf("%s|%s|%.2f|%s|%s|%s|||||||||||%s",
//	    merchantKey, txnID, amount, productInfo, firstName, email, salt)
//	hash := generateHash(hashInput)
func generateHash(hashSequence string) string {
	hasher := sha512.New()
	hasher.Write([]byte(hashSequence))
	hashBytes := hasher.Sum(nil)
	return fmt.Sprintf("%x", hashBytes) // Lowercase hex
}

// generateReverseHash generates SHA-512 reverse hash for webhook verification
//
// Reverse Hash Sequence (for SURL/FURL response):
// salt|status||||||||||udf5|udf4|udf3|udf2|udf1|email|firstname|productinfo|amount|txnid|key
//
// With empty UDFs (typical):
// salt|status||||||||||||||email|firstname|productinfo|amount|txnid|key
//
// Usage:
//
//	reverseHash := generateReverseHash(salt, status, email, firstname, productinfo, amount, txnid, key)
func generateReverseHash(salt, status, email, firstname, productinfo, amount, txnid, key string) string {
	// Reverse hash sequence with empty UDFs
	hashSequence := fmt.Sprintf("%s|%s||||||||||||||%s|%s|%s|%s|%s|%s",
		salt, status, email, firstname, productinfo, amount, txnid, key)
	return generateHash(hashSequence)
}

// extractFirstName extracts first name from GatewayPaymentRequest metadata
//
// Priority:
//  1. Metadata["firstname"]
//  2. Metadata["customer_name"] → split and take first word
//  3. Default: "Customer"
func extractFirstName(req payment.GatewayPaymentRequest) string {
	if req.Metadata == nil {
		return "Customer"
	}

	// Check direct firstname field
	if firstname, ok := req.Metadata["firstname"].(string); ok && firstname != "" {
		return sanitizeName(firstname)
	}

	// Check customer_name and extract first word
	if fullName, ok := req.Metadata["customer_name"].(string); ok && fullName != "" {
		parts := strings.Fields(fullName)
		if len(parts) > 0 {
			return sanitizeName(parts[0])
		}
	}

	return "Customer"
}

// extractEmail extracts email from GatewayPaymentRequest metadata
//
// Priority:
//  1. Metadata["email"]
//  2. Metadata["customer_email"]
//  3. Default: "customer@example.com"
func extractEmail(req payment.GatewayPaymentRequest) string {
	if req.Metadata == nil {
		return "customer@example.com"
	}

	if email, ok := req.Metadata["email"].(string); ok && email != "" {
		return email
	}

	if customerEmail, ok := req.Metadata["customer_email"].(string); ok && customerEmail != "" {
		return customerEmail
	}

	return "customer@example.com"
}

// extractPhone extracts phone number from GatewayPaymentRequest metadata
//
// Priority:
//  1. Metadata["phone"]
//  2. Metadata["customer_phone"]
//  3. Default: "9999999999"
func extractPhone(req payment.GatewayPaymentRequest) string {
	if req.Metadata == nil {
		return "9999999999"
	}

	if phone, ok := req.Metadata["phone"].(string); ok && phone != "" {
		return sanitizePhone(phone)
	}

	if customerPhone, ok := req.Metadata["customer_phone"].(string); ok && customerPhone != "" {
		return sanitizePhone(customerPhone)
	}

	return "9999999999"
}

// sanitizeName sanitizes name for Easebuzz API
// Allowed: Letters, numbers, hyphen, underscore, space, dot, ampersand, parentheses, comma
func sanitizeName(name string) string {
	// Remove leading/trailing whitespace
	name = strings.TrimSpace(name)

	// Easebuzz allows: a-zA-Z0-9&\-._ '()/,@
	// For safety, we'll keep alphanumeric, space, hyphen, underscore, dot
	var sanitized strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == ' ' || r == '-' || r == '_' || r == '.' || r == '&' {
			sanitized.WriteRune(r)
		}
	}

	result := sanitized.String()
	if result == "" {
		return "Customer"
	}
	return result
}

// sanitizePhone sanitizes phone number for Easebuzz API
// Removes non-numeric characters, ensures 10 digits
func sanitizePhone(phone string) string {
	// Remove all non-numeric characters
	var digits strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}

	phoneDigits := digits.String()

	// If phone starts with +91 or 91, remove country code
	if len(phoneDigits) > 10 {
		if phoneDigits[:2] == "91" {
			phoneDigits = phoneDigits[2:]
		}
	}

	// Ensure exactly 10 digits
	if len(phoneDigits) < 10 {
		return "9999999999" // Default fallback
	}

	return phoneDigits[:10] // Take first 10 digits
}

// determineCardPaymentMode determines if card is credit or debit
func determineCardPaymentMode(cardCategory payment.CardCategory) CardPaymentMode {
	switch cardCategory {
	case payment.CardCategoryCredit:
		return CardPaymentModeCredit
	case payment.CardCategoryCorporate:
		return CardPaymentModeCredit
	case payment.CardCategoryDebit:
		return CardPaymentModeDebit
	default:
		return CardPaymentModeCredit // Default to credit
	}
}

// extractTokenExpiry extracts token expiry from metadata
// Expected format: "MM/YYYY" (e.g., "12/2025")
func extractTokenExpiry(req payment.GatewayPaymentRequest) (string, error) {
	if req.Metadata == nil {
		return "", fmt.Errorf("metadata is nil")
	}

	if expiry, ok := req.Metadata["token_expiry"].(string); ok && expiry != "" {
		// Validate format MM/YYYY
		if len(expiry) == 7 && expiry[2] == '/' {
			return expiry, nil
		}
		return "", fmt.Errorf("invalid token_expiry format, expected MM/YYYY, got: %s", expiry)
	}

	return "", fmt.Errorf("token_expiry not found in metadata")
}

// extractTokenRequesterID extracts token requester ID (TRID) from metadata
func extractTokenRequesterID(req payment.GatewayPaymentRequest) (string, error) {
	if req.Metadata == nil {
		return "", fmt.Errorf("metadata is nil")
	}

	if trid, ok := req.Metadata["token_requester_id"].(string); ok && trid != "" {
		return trid, nil
	}

	return "", fmt.Errorf("token_requester_id not found in metadata")
}
