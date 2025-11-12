package utils

import (
	"strings"
	"testing"
)

func TestSanitizeCSVField(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Normal text",
			input:    "TechCorp Services",
			expected: "TechCorp Services",
		},
		{
			name:     "Formula injection - equals prefix",
			input:    "=1+1",
			expected: "1+1",
		},
		{
			name:     "Formula injection - plus prefix",
			input:    "+1234",
			expected: "1234",
		},
		{
			name:     "Formula injection - minus prefix",
			input:    "-cmd",
			expected: "cmd",
		},
		{
			name:     "Formula injection - at prefix",
			input:    "@SUM(A1:A10)",
			expected: "SUM(A1:A10)",
		},
		{
			name:     "Leading/trailing whitespace",
			input:    "  TechCorp  ",
			expected: "TechCorp",
		},
		{
			name:     "Zero-width space",
			input:    "Tech\u200BCorp",
			expected: "TechCorp",
		},
		{
			name:     "RTL override attack",
			input:    "Tech\u202ECorp",
			expected: "TechCorp",
		},
		{
			name:     "Null byte",
			input:    "Tech\x00Corp",
			expected: "TechCorp",
		},
		{
			name:     "Multiple spaces",
			input:    "Tech    Corp    Services",
			expected: "Tech Corp Services",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeCSVField(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeCSVField() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestValidatePayeeName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		rowNum    int
		wantError bool
		errorMsg  string
	}{
		{
			name:      "Valid name",
			input:     "TechCorp IT Services",
			rowNum:    1,
			wantError: false,
		},
		{
			name:      "Valid name with special chars",
			input:     "ABC Corp. (India) & Co.",
			rowNum:    1,
			wantError: false,
		},
		{
			name:      "Empty name",
			input:     "",
			rowNum:    1,
			wantError: true,
			errorMsg:  "cannot be empty",
		},
		{
			name:      "Name too long",
			input:     strings.Repeat("A", 101),
			rowNum:    1,
			wantError: true,
			errorMsg:  "exceeds maximum length",
		},
		{
			name:      "HTML script injection",
			input:     "<script>alert(1)</script>",
			rowNum:    1,
			wantError: true,
			errorMsg:  "invalid characters",
		},
		{
			name:      "Invalid characters",
			input:     "Tech<>Corp",
			rowNum:    1,
			wantError: true,
			errorMsg:  "invalid characters",
		},
		{
			name:      "Formula prefix removed and validated",
			input:     "=TechCorp",
			rowNum:    1,
			wantError: false, // Should be sanitized to "TechCorp"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidatePayeeName(tt.input, tt.rowNum)
			if tt.wantError {
				if err == nil {
					t.Errorf("ValidatePayeeName() expected error but got none")
				} else if tt.errorMsg != "" && !containsString(err.Error(), tt.errorMsg) {
					t.Errorf("ValidatePayeeName() error = %q, want substring %q", err.Error(), tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ValidatePayeeName() unexpected error = %v", err)
				}
				if result == "" {
					t.Errorf("ValidatePayeeName() returned empty string for valid input")
				}
			}
		})
	}
}

func TestValidateAmount(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		rowNum    int
		wantError bool
	}{
		{
			name:      "Valid integer amount",
			input:     "1000",
			rowNum:    1,
			wantError: false,
		},
		{
			name:      "Valid decimal amount",
			input:     "1000.50",
			rowNum:    1,
			wantError: false,
		},
		{
			name:      "Valid amount with comma",
			input:     "1,000.50",
			rowNum:    1,
			wantError: false,
		},
		{
			name:      "Empty amount",
			input:     "",
			rowNum:    1,
			wantError: true,
		},
		{
			name:      "Invalid characters",
			input:     "1000abc",
			rowNum:    1,
			wantError: true,
		},
		{
			name:      "Too many decimal places",
			input:     "1000.123",
			rowNum:    1,
			wantError: true,
		},
		{
			name:      "Formula injection",
			input:     "=1+1",
			rowNum:    1,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAmount(tt.input, tt.rowNum)
			if tt.wantError && err == nil {
				t.Errorf("ValidateAmount() expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("ValidateAmount() unexpected error = %v", err)
			}
		})
	}
}

func TestValidateDescription(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		rowNum    int
		wantError bool
	}{
		{
			name:      "Valid description",
			input:     "Payment for software license renewal Q1 2025",
			rowNum:    1,
			wantError: false,
		},
		{
			name:      "Empty description",
			input:     "",
			rowNum:    1,
			wantError: true,
		},
		{
			name:      "Description with HTML tags",
			input:     "Payment <script>alert(1)</script>",
			rowNum:    1,
			wantError: true,
		},
		{
			name:      "Description with curly braces",
			input:     "Payment {test}",
			rowNum:    1,
			wantError: true,
		},
		{
			name:      "Very long description",
			input:     strings.Repeat("A", 501),
			rowNum:    1,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateDescription(tt.input, tt.rowNum)
			if tt.wantError {
				if err == nil {
					t.Errorf("ValidateDescription() expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("ValidateDescription() unexpected error = %v", err)
				}
				if result == "" {
					t.Errorf("ValidateDescription() returned empty string")
				}
			}
		})
	}
}

func TestValidateAccountLast4(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		rowNum    int
		wantError bool
	}{
		{
			name:      "Valid account last 4",
			input:     "1234",
			rowNum:    1,
			wantError: false,
		},
		{
			name:      "Empty (optional field)",
			input:     "",
			rowNum:    1,
			wantError: false,
		},
		{
			name:      "Too short",
			input:     "123",
			rowNum:    1,
			wantError: true,
		},
		{
			name:      "Too long",
			input:     "12345",
			rowNum:    1,
			wantError: true,
		},
		{
			name:      "Non-numeric",
			input:     "12ab",
			rowNum:    1,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateAccountLast4(tt.input, tt.rowNum)
			if tt.wantError {
				if err == nil {
					t.Errorf("ValidateAccountLast4() expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("ValidateAccountLast4() unexpected error = %v", err)
				}
				if tt.input != "" && result == "" {
					t.Errorf("ValidateAccountLast4() returned empty for non-empty input")
				}
			}
		})
	}
}

func TestValidateIFSCCode(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		rowNum    int
		wantError bool
	}{
		{
			name:      "Valid IFSC code",
			input:     "HDFC0001234",
			rowNum:    1,
			wantError: false,
		},
		{
			name:      "Valid IFSC lowercase converted",
			input:     "hdfc0001234",
			rowNum:    1,
			wantError: false,
		},
		{
			name:      "Empty (optional field)",
			input:     "",
			rowNum:    1,
			wantError: false,
		},
		{
			name:      "Invalid format - wrong 5th char",
			input:     "HDFC1001234",
			rowNum:    1,
			wantError: true,
		},
		{
			name:      "Too short",
			input:     "HDFC00012",
			rowNum:    1,
			wantError: true,
		},
		{
			name:      "Too long",
			input:     "HDFC00012345",
			rowNum:    1,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateIFSCCode(tt.input, tt.rowNum)
			if tt.wantError {
				if err == nil {
					t.Errorf("ValidateIFSCCode() expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("ValidateIFSCCode() unexpected error = %v", err)
				}
				if tt.input != "" && result == "" {
					t.Errorf("ValidateIFSCCode() returned empty for non-empty input")
				}
			}
		})
	}
}

func TestValidateInvoiceNumber(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		rowNum    int
		wantError bool
	}{
		{
			name:      "Valid invoice number",
			input:     "INV-2025-001",
			rowNum:    1,
			wantError: false,
		},
		{
			name:      "Valid with underscore",
			input:     "INV_2025_001",
			rowNum:    1,
			wantError: false,
		},
		{
			name:      "Valid with slash",
			input:     "INV/2025/001",
			rowNum:    1,
			wantError: false,
		},
		{
			name:      "Empty (optional field)",
			input:     "",
			rowNum:    1,
			wantError: false,
		},
		{
			name:      "Invalid characters",
			input:     "INV@2025",
			rowNum:    1,
			wantError: true,
		},
		{
			name:      "Too long",
			input:     strings.Repeat("A", 51),
			rowNum:    1,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateInvoiceNumber(tt.input, tt.rowNum)
			if tt.wantError {
				if err == nil {
					t.Errorf("ValidateInvoiceNumber() expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("ValidateInvoiceNumber() unexpected error = %v", err)
				}
			}
			_ = result // Avoid unused variable warning
		})
	}
}

func TestValidatePaymentContext(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		rowNum    int
		wantError bool
	}{
		{
			name:      "Valid vendor_payment",
			input:     "vendor_payment",
			rowNum:    1,
			wantError: false,
		},
		{
			name:      "Valid payroll",
			input:     "payroll",
			rowNum:    1,
			wantError: false,
		},
		{
			name:      "Uppercase converted",
			input:     "VENDOR_PAYMENT",
			rowNum:    1,
			wantError: false,
		},
		{
			name:      "Empty",
			input:     "",
			rowNum:    1,
			wantError: true,
		},
		{
			name:      "Invalid characters",
			input:     "vendor-payment",
			rowNum:    1,
			wantError: true,
		},
		{
			name:      "Invalid space",
			input:     "vendor payment",
			rowNum:    1,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidatePaymentContext(tt.input, tt.rowNum)
			if tt.wantError {
				if err == nil {
					t.Errorf("ValidatePaymentContext() expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("ValidatePaymentContext() unexpected error = %v", err)
				}
				if result == "" {
					t.Errorf("ValidatePaymentContext() returned empty string")
				}
			}
		})
	}
}

func TestDetectSQLInjection(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "Normal text",
			input:    "TechCorp Services",
			expected: false,
		},
		{
			name:     "SQL injection - DROP TABLE",
			input:    "'; DROP TABLE users--",
			expected: true,
		},
		{
			name:     "SQL injection - UNION SELECT",
			input:    "1' UNION SELECT * FROM users--",
			expected: true,
		},
		{
			name:     "SQL injection - INSERT with comment",
			input:    "INSERT INTO users /*",
			expected: true,
		},
		{
			name:     "Safe text with select word",
			input:    "Please select option",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectSQLInjection(tt.input)
			if result != tt.expected {
				t.Errorf("DetectSQLInjection() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIsValidUTF8(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "Valid ASCII",
			input:    "Hello World",
			expected: true,
		},
		{
			name:     "Valid UTF-8",
			input:    "Hello 世界",
			expected: true,
		},
		{
			name:     "Valid UTF-8 Indian",
			input:    "नमस्ते",
			expected: true,
		},
		{
			name:     "Invalid UTF-8",
			input:    string([]byte{0xff, 0xfe, 0xfd}),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidUTF8(tt.input)
			if result != tt.expected {
				t.Errorf("IsValidUTF8() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// Helper function to check if string contains substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
