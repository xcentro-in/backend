package jiopay

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// extractCustomerName extracts customer name from metadata
func extractCustomerName(metadata map[string]any) string {
	if name, ok := metadata["customer_name"].(string); ok && name != "" {
		return name
	}
	return "Customer"
}

// extractCustomerEmail extracts customer email from metadata
func extractCustomerEmail(metadata map[string]any) string {
	if email, ok := metadata["customer_email"].(string); ok && email != "" {
		return email
	}
	return "customer@example.com"
}

// extractCustomerPhone extracts and sanitizes customer phone from metadata
func extractCustomerPhone(metadata map[string]any) (int64, error) {
	if phone, ok := metadata["customer_phone"].(string); ok && phone != "" {
		// Sanitize phone: remove spaces, dashes, parentheses
		sanitized := sanitizePhone(phone)

		// Convert to int64
		phoneNum, err := strconv.ParseInt(sanitized, 10, 64)
		if err != nil {
			return 9999999999, nil // Default fallback
		}
		return phoneNum, nil
	}
	return 9999999999, nil // Default 10-digit number
}

// sanitizePhone removes non-digit characters from phone number
func sanitizePhone(phone string) string {
	// Remove common separators
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")
	phone = strings.ReplaceAll(phone, "(", "")
	phone = strings.ReplaceAll(phone, ")", "")
	phone = strings.ReplaceAll(phone, "+", "")

	// If starts with country code (91), remove it
	if strings.HasPrefix(phone, "91") && len(phone) > 10 {
		phone = phone[2:]
	}

	// Take last 10 digits if longer
	if len(phone) > 10 {
		phone = phone[len(phone)-10:]
	}

	return phone
}

// buildConsentObject generates a consent object with unique ID and current timestamp
func buildConsentObject() ConsentObject {
	return ConsentObject{
		ID:        fmt.Sprintf("CONSENT_%s", uuid.Must(uuid.NewV7()).String()[:8]),
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano), // ISO 8601 format
	}
}

// buildAuditObject generates an audit object with authorization details
func buildAuditObject() AuditObject {
	return AuditObject{
		Authorized:        true,
		AuthorizationCode: fmt.Sprintf("AUTH_%s_%d", uuid.Must(uuid.NewV7()).String()[:6], time.Now().Unix()%100000),
	}
}

// buildMobileObject creates a mobile object if phone is available
func buildMobileObject(metadata map[string]any) *MobileObject {
	phone, err := extractCustomerPhone(metadata)
	if err != nil || phone == 9999999999 {
		// If we can't extract a valid phone, omit the mobile object
		return nil
	}

	return &MobileObject{
		CountryCode: 91, // India country code
		Number:      phone,
	}
}

// formatExpiryYear converts YY or YYYY format to YYYY
func formatExpiryYear(year string) string {
	// If already 4 digits, return as-is
	if len(year) == 4 {
		return year
	}

	// If 2 digits, convert to 4 digits
	if len(year) == 2 {
		yearNum, err := strconv.Atoi(year)
		if err != nil {
			return year
		}

		// Assume 20XX for years 00-99
		if yearNum >= 0 && yearNum <= 99 {
			return fmt.Sprintf("20%s", year)
		}
	}

	return year
}

// convertAmountToRupees converts paise (int64) to rupees (float64)
// JioPay expects amount in rupees, while internal system uses paise
func convertAmountToRupees(paise int64) float64 {
	return float64(paise) / 100.0
}

// mapJioPayCardType maps JioPay card type code to internal CardType
func mapJioPayCardType(typeCode int) string {
	switch typeCode {
	case 130: // Visa
		return "visa"
	case 131: // MasterCard
		return "mastercard"
	case 132: // RuPay
		return "rupay"
	case 133: // Amex
		return "amex"
	case 134: // Diners
		return "diners"
	default:
		return "unknown"
	}
}

// mapJioPayCardScheme maps JioPay scheme code to card network
func mapJioPayCardScheme(schemeCode int) string {
	switch schemeCode {
	case 103: // Visa
		return "VISA"
	case 104: // MasterCard
		return "MASTERCARD"
	case 105: // RuPay
		return "RUPAY"
	case 106: // Amex
		return "AMEX"
	case 107: // Diners
		return "DINERS"
	default:
		return "UNKNOWN"
	}
}

// formatJioPayTxnDate formats time to JioPay txnDate format (YYYYMMDDHHMISS)
// Example: "20250610125921" for June 10, 2025 12:59:21
func formatJioPayTxnDate(t time.Time) string {
	return t.Format("20060102150405")
}

// formatJioPayCardExpiry formats card expiry to JioPay format (YYYYMM)
// Takes expiryMonth (MM) and expiryYear (YY or YYYY)
// Returns: "YYYYMM" (e.g., "202608")
func formatJioPayCardExpiry(expiryMonth, expiryYear string) string {
	// Ensure year is 4 digits
	year := formatExpiryYear(expiryYear)

	// Pad month to 2 digits if needed
	month := expiryMonth
	if len(month) == 1 {
		month = "0" + month
	}

	return year + month
}

// formatAmountWithDecimals formats amount in paise to rupees string with 2 decimals
// Example: 10050 paise → "100.50"
func formatAmountWithDecimals(paise int64) string {
	rupees := float64(paise) / 100.0
	return fmt.Sprintf("%.2f", rupees)
}

// ============================================================================
// HASH GENERATION
// ============================================================================
// NOTE: Hash generation and validation logic has been moved to hash.go
// See SecureHashGenerator for:
//   - GenerateForRequest(): Calculate hash for outgoing API requests
//   - ValidateFromResponse(): Validate signature from incoming callbacks/webhooks
// ============================================================================

// ============================================================================
// JIOPAY VAULT API HEADERS
// ============================================================================

// generateXTimestampID generates timestamp in yyyyMMddHHmmss format
// Example: "20250610125921" for June 10, 2025 12:59:21
func generateXTimestampID() string {
	return time.Now().Format("20060102150405")
}

// generateXTraceID generates a UUID for request tracing
// Returns lowercase UUID with hyphens (matches Java's UUID.randomUUID().toString())
func generateXTraceID() string {
	return uuid.Must(uuid.NewV7()).String()
}

// generateXAppToken generates the x-app-token header using AES-128-GCM encryption
// Algorithm (matches JioPay Java implementation):
// 1. Concatenate: x-app-id + "$" + x-trace-id + "$" + x-timestamp-id
// 2. Encrypt using AES-128-GCM with random 12-byte IV
// 3. Prepend IV to encrypted data (IV + encrypted + auth tag)
// 4. Base64 encode the result
func generateXAppToken(appID, traceID, timestampID, encryptionKey string) (string, error) {
	// Step 1: Concatenate with "$" separator
	rawData := fmt.Sprintf("%s$%s$%s", appID, traceID, timestampID)

	// Step 2: AES-128-GCM encryption with random IV
	encrypted, err := encryptAES128GCM([]byte(rawData), []byte(encryptionKey))
	if err != nil {
		return "", fmt.Errorf("AES-GCM encryption failed: %w", err)
	}

	// Step 3: Base64 encode (IV + encrypted + auth tag)
	token := base64.StdEncoding.EncodeToString(encrypted)

	return token, nil
}

// encryptAES128GCM encrypts data using AES-128 in GCM mode
// This matches JioPay's Java implementation using AES/GCM/NoPadding
// GCM provides both confidentiality and authenticity with a 128-bit authentication tag
// Key must be exactly 16 bytes (128 bits)
//
// Returns: IV (12 bytes) + Encrypted Data + Authentication Tag (16 bytes)
func encryptAES128GCM(plaintext, key []byte) ([]byte, error) {
	// Validate key size (must be 16 bytes for AES-128)
	if len(key) != 16 {
		return nil, fmt.Errorf("invalid key size: expected 16 bytes, got %d", len(key))
	}

	// Create AES cipher block
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Create GCM mode with standard 128-bit authentication tag
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random 12-byte IV (nonce) for this encryption
	// IV must be unique for each encryption with the same key
	iv := make([]byte, 12) // GCM standard nonce size
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, fmt.Errorf("failed to generate IV: %w", err)
	}

	// Encrypt and authenticate
	// GCM.Seal appends the encrypted data + auth tag to the dst slice
	ciphertext := gcm.Seal(nil, iv, plaintext, nil)

	// Prepend IV to ciphertext (IV + encrypted data + auth tag)
	// This matches Java implementation: System.arraycopy(iv, 0, encryptedWithIV, 0, iv.length)
	result := make([]byte, len(iv)+len(ciphertext))
	copy(result, iv)
	copy(result[len(iv):], ciphertext)

	return result, nil
}

// ============================================================================
// JIOPAY INITIATE SALE HASH GENERATION
// ============================================================================

// buildInitiateSaleFieldMap builds a map of all request fields for hash generation
// Excludes secureHash field itself and any empty/nil fields
func buildInitiateSaleFieldMap(req *InitiateSaleRequest) map[string]string {
	fieldMap := make(map[string]string)

	// Add all non-empty mandatory fields
	if req.MerchantID != "" {
		fieldMap["merchantId"] = req.MerchantID
	}
	if req.MerchantTxnNo != "" {
		fieldMap["merchantTxnNo"] = req.MerchantTxnNo
	}
	if req.Amount != "" {
		fieldMap["amount"] = req.Amount
	}
	if req.CurrencyCode != "" {
		fieldMap["currencyCode"] = req.CurrencyCode
	}
	if req.PayType != "" {
		fieldMap["payType"] = req.PayType
	}
	if req.CustomerEmailID != "" {
		fieldMap["customerEmailID"] = req.CustomerEmailID
	}
	if req.TransactionType != "" {
		fieldMap["transactionType"] = req.TransactionType
	}
	if req.ReturnURL != "" {
		fieldMap["returnURL"] = req.ReturnURL
	}
	if req.TxnDate != "" {
		fieldMap["txnDate"] = req.TxnDate
	}

	// Add optional fields only if present
	if req.PaymentMode != "" {
		fieldMap["paymentMode"] = req.PaymentMode
	}
	if req.CardNo != "" {
		fieldMap["cardNo"] = req.CardNo
	}
	if req.CardExpiry != "" {
		fieldMap["cardExpiry"] = req.CardExpiry
	}
	if req.NameOnCard != "" {
		fieldMap["nameOnCard"] = req.NameOnCard
	}
	if req.CVV != "" {
		fieldMap["cvv"] = req.CVV
	}
	if req.TokenCryptogram != "" {
		fieldMap["tokenCryptogram"] = req.TokenCryptogram
	}
	if req.TokenRefID != "" {
		fieldMap["tokenRefId"] = req.TokenRefID
	}
	if req.CustomerName != "" {
		fieldMap["customerName"] = req.CustomerName
	}
	if req.CustomerMobileNo != "" {
		fieldMap["customerMobileNo"] = req.CustomerMobileNo
	}
	if req.InvoiceNo != "" {
		fieldMap["invoiceNo"] = req.InvoiceNo
	}
	if req.OrderDesc != "" {
		fieldMap["orderDesc"] = req.OrderDesc
	}
	if req.AggregatorID != "" {
		fieldMap["aggregatorID"] = req.AggregatorID
	}
	if req.CustomerID != "" {
		fieldMap["customerID"] = req.CustomerID
	}

	// Do NOT include secureHash field itself
	return fieldMap
}

// ============================================================================
// JIOPAY CALLBACK URL BUILDING
// ============================================================================

// buildCallbackURL appends gateway type query parameter to callback URL
// This allows our callback handler to route requests to the appropriate gateway handler
//
// Parameters:
//   - baseURL: Base callback URL (e.g., "https://api.xcentro.com/api/v1/process_payment/callback")
//   - gatewayType: Gateway identifier ("jiopay", "easebuzz", etc.)
//
// Returns:
//   - Full callback URL with type parameter (e.g., ".../callback?type=jiopay")
func buildCallbackURL(baseURL, gatewayType string) string {
	// Check if URL already has query parameters
	if strings.Contains(baseURL, "?") {
		// Append with &
		return fmt.Sprintf("%s&type=%s", baseURL, gatewayType)
	}
	// Append with ?
	return fmt.Sprintf("%s?type=%s", baseURL, gatewayType)
}

// ============================================================================
// JIOPAY CALLBACK STATUS MAPPING
// ============================================================================

// MapJioPayCallbackStatus maps JioPay callback response codes to internal PaymentStatus
//
// Status determination logic (based on user-provided documentation):
//  1. Primary indicator: responseCode
//     - "000" or "0000" → Success
//     - "R1000" → Initiated/Pending (for UPI/NACH out-of-band payments)
//     - Any other value → Failed
//  2. Secondary indicator: txnStatus (for validation)
//     - "SUC" → Success (assumption - validate with PG team)
//     - "FAIL" → Failure (assumption)
//     - "PEN" → Pending (assumption)
//
// Parameters:
//   - responseCode: Primary status code from JioPay callback
//   - txnStatus: Transaction status (SUC/FAIL/PEN)
//
// Returns:
//   - "success": Payment completed successfully
//   - "pending": Payment initiated but not completed (UPI/NACH)
//   - "failed": Payment failed or cancelled
func MapJioPayCallbackStatus(responseCode, txnStatus string) string {
	// Primary: Check responseCode
	switch responseCode {
	case "000", "0000":
		// Success response codes
		return "success"
	case "R1000":
		// Request initiated successfully (UPI/NACH - out of band payment)
		return "pending"
	default:
		// Any other responseCode indicates failure
		// Secondary: Check txnStatus for more context
		switch txnStatus {
		case "SUC":
			// Transaction successful (rare case where responseCode != 000 but txn succeeded)
			return "success"
		case "PEN":
			// Transaction pending
			return "pending"
		default:
			// Transaction failed (FAIL or unknown status)
			return "failed"
		}
	}
}
