package utils

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Field length limits
const (
	MaxPayeeNameLength      = 100
	MaxDescriptionLength    = 500
	MaxInvoiceNumberLength  = 50
	MaxExpenseCategoryLength = 100
	MaxPaymentContextLength = 50
	MaxAccountLast4Length   = 4
	MaxIFSCCodeLength       = 11
	MaxRowLength            = 1024 // 1KB per row
)

// CSV field validation patterns
var (
	// Payee name: alphanumeric, spaces, and safe symbols only
	payeeNamePattern = regexp.MustCompile(`^[a-zA-Z0-9\s\-\.,()\&']+$`)

	// Amount: decimal number with up to 2 decimal places
	amountPattern = regexp.MustCompile(`^\d+\.?\d{0,2}$`)

	// Description: no HTML tags, no curly braces, no script tags
	descriptionPattern = regexp.MustCompile(`^[^<>{}]*$`)

	// Account last 4: exactly 4 digits
	accountLast4Pattern = regexp.MustCompile(`^\d{4}$`)

	// IFSC code: Standard Indian IFSC format (4 letters + 0 + 6 alphanumeric)
	ifscCodePattern = regexp.MustCompile(`^[A-Z]{4}0[A-Z0-9]{6}$`)

	// Invoice number: alphanumeric with dash, slash, underscore
	invoiceNumberPattern = regexp.MustCompile(`^[a-zA-Z0-9\-/_]+$`)

	// Expense category: alphanumeric with spaces, dash, underscore
	expenseCategoryPattern = regexp.MustCompile(`^[a-zA-Z0-9\s\-_]+$`)

	// Payment context: alphanumeric with underscore only
	paymentContextPattern = regexp.MustCompile(`^[a-z_]+$`)

	// CSV injection patterns (formula prefixes)
	formulaPrefixPattern = regexp.MustCompile(`^[\s]*[=+\-@\t\r]`)

	// Dangerous HTML/Script patterns
	htmlScriptPattern = regexp.MustCompile(`(?i)<script|<iframe|<object|<embed|javascript:|onerror=|onload=`)

	// SQL injection patterns (basic detection)
	sqlInjectionPattern = regexp.MustCompile(`(?i)(union|select|insert|update|delete|drop|create|alter|exec|execute|script|javascript|<|>).*(-{2}|;|\/\*)`)
)

// CSVValidationError represents a CSV content validation error
type CSVValidationError struct {
	Row     int
	Field   string
	Message string
}

func (e *CSVValidationError) Error() string {
	return fmt.Sprintf("Row %d, Field '%s': %s", e.Row, e.Field, e.Message)
}

// SanitizeCSVField sanitizes a single CSV field
// - Removes CSV injection prefixes
// - Removes dangerous Unicode characters
// - Trims whitespace
// - Prevents XSS and HTML injection
func SanitizeCSVField(field string) string {
	// Trim leading/trailing whitespace
	sanitized := strings.TrimSpace(field)

	// Remove CSV injection prefixes (=, +, -, @, tab, carriage return)
	sanitized = formulaPrefixPattern.ReplaceAllString(sanitized, "")

	// Remove dangerous Unicode characters
	sanitized = removeDangerousUnicode(sanitized)

	// Remove null bytes
	sanitized = strings.ReplaceAll(sanitized, "\x00", "")

	// Normalize whitespace (replace multiple spaces with single space)
	sanitized = normalizeWhitespace(sanitized)

	return sanitized
}

// ValidatePayeeName validates and sanitizes payee name
func ValidatePayeeName(name string, rowNum int) (string, error) {
	// Sanitize first
	sanitized := SanitizeCSVField(name)

	// Check length
	if len(sanitized) == 0 {
		return "", &CSVValidationError{
			Row:     rowNum,
			Field:   "payee_name",
			Message: "Payee name cannot be empty",
		}
	}

	if len(sanitized) > MaxPayeeNameLength {
		return "", &CSVValidationError{
			Row:     rowNum,
			Field:   "payee_name",
			Message: fmt.Sprintf("Payee name exceeds maximum length of %d characters", MaxPayeeNameLength),
		}
	}

	// Check pattern
	if !payeeNamePattern.MatchString(sanitized) {
		return "", &CSVValidationError{
			Row:     rowNum,
			Field:   "payee_name",
			Message: "Payee name contains invalid characters. Only letters, numbers, spaces, and symbols (-.,'()&) are allowed",
		}
	}

	// Check for HTML/Script injection
	if htmlScriptPattern.MatchString(sanitized) {
		return "", &CSVValidationError{
			Row:     rowNum,
			Field:   "payee_name",
			Message: "Payee name contains potentially malicious content",
		}
	}

	return sanitized, nil
}

// ValidateAmount validates amount string format
func ValidateAmount(amountStr string, rowNum int) error {
	sanitized := strings.TrimSpace(amountStr)

	if sanitized == "" {
		return &CSVValidationError{
			Row:     rowNum,
			Field:   "amount",
			Message: "Amount cannot be empty",
		}
	}

	// Remove commas if present (e.g., "1,000.00" -> "1000.00")
	sanitized = strings.ReplaceAll(sanitized, ",", "")

	if !amountPattern.MatchString(sanitized) {
		return &CSVValidationError{
			Row:     rowNum,
			Field:   "amount",
			Message: "Amount must be a valid number (e.g., 1000 or 1000.50)",
		}
	}

	return nil
}

// ValidateDescription validates and sanitizes description
func ValidateDescription(description string, rowNum int) (string, error) {
	sanitized := SanitizeCSVField(description)

	if len(sanitized) == 0 {
		return "", &CSVValidationError{
			Row:     rowNum,
			Field:   "description",
			Message: "Description cannot be empty",
		}
	}

	if len(sanitized) > MaxDescriptionLength {
		return "", &CSVValidationError{
			Row:     rowNum,
			Field:   "description",
			Message: fmt.Sprintf("Description exceeds maximum length of %d characters", MaxDescriptionLength),
		}
	}

	if !descriptionPattern.MatchString(sanitized) {
		return "", &CSVValidationError{
			Row:     rowNum,
			Field:   "description",
			Message: "Description contains invalid characters (HTML tags or special symbols not allowed)",
		}
	}

	// Check for script injection
	if htmlScriptPattern.MatchString(sanitized) {
		return "", &CSVValidationError{
			Row:     rowNum,
			Field:   "description",
			Message: "Description contains potentially malicious content",
		}
	}

	return sanitized, nil
}

// ValidateAccountLast4 validates account last 4 digits (optional field)
func ValidateAccountLast4(accountLast4 string, rowNum int) (string, error) {
	sanitized := strings.TrimSpace(accountLast4)

	// Empty is allowed (optional field)
	if sanitized == "" {
		return "", nil
	}

	if len(sanitized) != MaxAccountLast4Length {
		return "", &CSVValidationError{
			Row:     rowNum,
			Field:   "account_last_4",
			Message: "Account last 4 must be exactly 4 digits",
		}
	}

	if !accountLast4Pattern.MatchString(sanitized) {
		return "", &CSVValidationError{
			Row:     rowNum,
			Field:   "account_last_4",
			Message: "Account last 4 must contain only digits",
		}
	}

	return sanitized, nil
}

// ValidateIFSCCode validates IFSC code format (optional field)
func ValidateIFSCCode(ifscCode string, rowNum int) (string, error) {
	sanitized := strings.TrimSpace(strings.ToUpper(ifscCode))

	// Empty is allowed (optional field)
	if sanitized == "" {
		return "", nil
	}

	if len(sanitized) != MaxIFSCCodeLength {
		return "", &CSVValidationError{
			Row:     rowNum,
			Field:   "ifsc_code",
			Message: "IFSC code must be exactly 11 characters",
		}
	}

	if !ifscCodePattern.MatchString(sanitized) {
		return "", &CSVValidationError{
			Row:     rowNum,
			Field:   "ifsc_code",
			Message: "Invalid IFSC code format. Must be 4 letters + 0 + 6 alphanumeric characters (e.g., HDFC0001234)",
		}
	}

	return sanitized, nil
}

// ValidateInvoiceNumber validates and sanitizes invoice number (optional field)
func ValidateInvoiceNumber(invoiceNumber string, rowNum int) (string, error) {
	sanitized := SanitizeCSVField(invoiceNumber)

	// Empty is allowed (optional field)
	if sanitized == "" {
		return "", nil
	}

	if len(sanitized) > MaxInvoiceNumberLength {
		return "", &CSVValidationError{
			Row:     rowNum,
			Field:   "invoice_number",
			Message: fmt.Sprintf("Invoice number exceeds maximum length of %d characters", MaxInvoiceNumberLength),
		}
	}

	if !invoiceNumberPattern.MatchString(sanitized) {
		return "", &CSVValidationError{
			Row:     rowNum,
			Field:   "invoice_number",
			Message: "Invoice number contains invalid characters. Only letters, numbers, dash, slash, and underscore are allowed",
		}
	}

	return sanitized, nil
}

// ValidateExpenseCategory validates and sanitizes expense category (optional field)
func ValidateExpenseCategory(category string, rowNum int) (string, error) {
	sanitized := SanitizeCSVField(category)

	// Empty is allowed (optional field)
	if sanitized == "" {
		return "", nil
	}

	if len(sanitized) > MaxExpenseCategoryLength {
		return "", &CSVValidationError{
			Row:     rowNum,
			Field:   "expense_category",
			Message: fmt.Sprintf("Expense category exceeds maximum length of %d characters", MaxExpenseCategoryLength),
		}
	}

	if !expenseCategoryPattern.MatchString(sanitized) {
		return "", &CSVValidationError{
			Row:     rowNum,
			Field:   "expense_category",
			Message: "Expense category contains invalid characters",
		}
	}

	return sanitized, nil
}

// ValidatePaymentContext validates payment context (required field)
func ValidatePaymentContext(context string, rowNum int) (string, error) {
	sanitized := strings.TrimSpace(strings.ToLower(context))

	if sanitized == "" {
		return "", &CSVValidationError{
			Row:     rowNum,
			Field:   "payment_context",
			Message: "Payment context cannot be empty",
		}
	}

	if len(sanitized) > MaxPaymentContextLength {
		return "", &CSVValidationError{
			Row:     rowNum,
			Field:   "payment_context",
			Message: fmt.Sprintf("Payment context exceeds maximum length of %d characters", MaxPaymentContextLength),
		}
	}

	if !paymentContextPattern.MatchString(sanitized) {
		return "", &CSVValidationError{
			Row:     rowNum,
			Field:   "payment_context",
			Message: "Payment context must be lowercase letters and underscores only (e.g., vendor_payment, payroll)",
		}
	}

	return sanitized, nil
}

// ValidateRowLength checks if entire row exceeds maximum length
func ValidateRowLength(row []string, rowNum int) error {
	totalLength := 0
	for _, field := range row {
		totalLength += len(field)
	}

	if totalLength > MaxRowLength {
		return &CSVValidationError{
			Row:     rowNum,
			Field:   "row",
			Message: fmt.Sprintf("Row exceeds maximum length of %d bytes", MaxRowLength),
		}
	}

	return nil
}

// DetectSQLInjection performs basic SQL injection detection
func DetectSQLInjection(value string) bool {
	return sqlInjectionPattern.MatchString(value)
}

// removeDangerousUnicode removes potentially dangerous Unicode characters
func removeDangerousUnicode(s string) string {
	var result strings.Builder
	result.Grow(len(s))

	for _, r := range s {
		// Skip dangerous Unicode characters
		switch r {
		case '\u200B', // Zero-width space
			'\u200C', // Zero-width non-joiner
			'\u200D', // Zero-width joiner
			'\u200E', // Left-to-right mark
			'\u200F', // Right-to-left mark
			'\u202A', // Left-to-right embedding
			'\u202B', // Right-to-left embedding
			'\u202C', // Pop directional formatting
			'\u202D', // Left-to-right override
			'\u202E', // Right-to-left override
			'\uFEFF': // Zero-width no-break space (BOM)
			continue
		}

		// Only allow printable characters and common whitespace
		if unicode.IsPrint(r) || r == '\n' || r == '\r' || r == '\t' {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// normalizeWhitespace replaces multiple consecutive spaces with single space
func normalizeWhitespace(s string) string {
	// Replace multiple spaces/tabs with single space
	space := regexp.MustCompile(`\s+`)
	return space.ReplaceAllString(s, " ")
}

// IsValidUTF8 checks if string is valid UTF-8
func IsValidUTF8(s string) bool {
	return utf8.ValidString(s)
}

// SanitizeAndValidateRow performs complete validation and sanitization on a CSV row
func SanitizeAndValidateRow(row []string, rowNum int) error {
	// Check row length
	if err := ValidateRowLength(row, rowNum); err != nil {
		return err
	}

	// Check each field for UTF-8 validity
	for i, field := range row {
		if !IsValidUTF8(field) {
			return &CSVValidationError{
				Row:     rowNum,
				Field:   fmt.Sprintf("column_%d", i),
				Message: "Contains invalid UTF-8 characters",
			}
		}
	}

	return nil
}
