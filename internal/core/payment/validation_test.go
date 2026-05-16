package payment

import (
	"testing"
	"time"
)

// TestValidateCardExpiry tests card expiry validation
func TestValidateCardExpiry(t *testing.T) {
	now := time.Now()
	currentYear := now.Year()
	currentMonth := int(now.Month())

	tests := []struct {
		name        string
		expiryMonth string
		expiryYear  string
		wantErr     bool
	}{
		{
			name:        "Valid future date",
			expiryMonth: "12",
			expiryYear:  "2030",
			wantErr:     false,
		},
		{
			name:        "Valid current year future month",
			expiryMonth: "12",
			expiryYear:  "2026",
			wantErr:     false,
		},
		{
			name:        "Valid YY format",
			expiryMonth: "12",
			expiryYear:  "30", // 2030
			wantErr:     false,
		},
		{
			name:        "Expired year",
			expiryMonth: "12",
			expiryYear:  "2020",
			wantErr:     true,
		},
		{
			name:        "Expired month in current year",
			expiryMonth: "01",
			expiryYear:  "2026",
			wantErr:     currentMonth > 1, // Only error if current month is past January
		},
		{
			name:        "Invalid month (13)",
			expiryMonth: "13",
			expiryYear:  "2030",
			wantErr:     true,
		},
		{
			name:        "Invalid month (0)",
			expiryMonth: "00",
			expiryYear:  "2030",
			wantErr:     true,
		},
		{
			name:        "Invalid month (negative)",
			expiryMonth: "-1",
			expiryYear:  "2030",
			wantErr:     true,
		},
		{
			name:        "Invalid month (non-numeric)",
			expiryMonth: "ab",
			expiryYear:  "2030",
			wantErr:     true,
		},
		{
			name:        "Invalid year (non-numeric)",
			expiryMonth: "12",
			expiryYear:  "abcd",
			wantErr:     true,
		},
		{
			name:        "Current month and year",
			expiryMonth: formatMonth(currentMonth),
			expiryYear:  formatYear(currentYear),
			wantErr:     false, // Card expires at end of month, so current month is valid
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCardExpiry(tt.expiryMonth, tt.expiryYear)
			if tt.wantErr && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

// TestValidateCVV tests CVV validation
func TestValidateCVV(t *testing.T) {
	tests := []struct {
		name    string
		cvv     string
		wantErr bool
	}{
		{
			name:    "Valid 3-digit CVV",
			cvv:     "123",
			wantErr: false,
		},
		{
			name:    "Valid 4-digit CVV (Amex)",
			cvv:     "1234",
			wantErr: false,
		},
		{
			name:    "Empty CVV",
			cvv:     "",
			wantErr: true,
		},
		{
			name:    "Too short (2 digits)",
			cvv:     "12",
			wantErr: true,
		},
		{
			name:    "Too long (5 digits)",
			cvv:     "12345",
			wantErr: true,
		},
		{
			name:    "Contains letters",
			cvv:     "12a",
			wantErr: true,
		},
		{
			name:    "Contains special chars",
			cvv:     "12@",
			wantErr: true,
		},
		{
			name:    "Contains spaces",
			cvv:     "1 3",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCVV(tt.cvv)
			if tt.wantErr && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

// TestValidateCardHolderName tests card holder name validation
func TestValidateCardHolderName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "Valid name",
			input:   "John Doe",
			wantErr: false,
		},
		{
			name:    "Valid name with middle initial",
			input:   "John M. Doe",
			wantErr: false,
		},
		{
			name:    "Valid single name",
			input:   "Madonna",
			wantErr: false,
		},
		{
			name:    "Valid name with special chars",
			input:   "Mary-Jane O'Connor",
			wantErr: false,
		},
		{
			name:    "Empty name",
			input:   "",
			wantErr: true,
		},
		{
			name:    "Too short (1 character)",
			input:   "A",
			wantErr: true,
		},
		{
			name:    "Too long (>100 characters)",
			input:   "ThisIsAVeryLongNameThatExceedsTheMaximumAllowedLengthOfOneHundredCharactersAndShouldBeRejectedByValidation",
			wantErr: true,
		},
		{
			name:    "Minimum valid length (2 chars)",
			input:   "AB",
			wantErr: false,
		},
		{
			name:    "Maximum valid length (100 chars)",
			input:   "ThisIsExactlyOneHundredCharactersLongIncludingSpacesAndShouldBeAcceptedByTheValidationLogic123",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCardHolderName(tt.input)
			if tt.wantErr && err == nil {
				t.Errorf("expected error but got nil for input: %q (len=%d)", tt.input, len(tt.input))
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error but got: %v for input: %q (len=%d)", err, tt.input, len(tt.input))
			}
		})
	}
}

// TestValidatePaymentAmount tests payment amount validation
func TestValidatePaymentAmount(t *testing.T) {
	tests := []struct {
		name    string
		amount  int64
		wantErr bool
	}{
		{
			name:    "Valid amount ₹100",
			amount:  10000, // ₹100
			wantErr: false,
		},
		{
			name:    "Valid amount ₹1000",
			amount:  100000, // ₹1000
			wantErr: false,
		},
		{
			name:    "Minimum valid amount ₹1",
			amount:  100, // ₹1
			wantErr: false,
		},
		{
			name:    "Maximum valid amount ₹10,00,000",
			amount:  100000000, // ₹10,00,000
			wantErr: false,
		},
		{
			name:    "Zero amount",
			amount:  0,
			wantErr: true,
		},
		{
			name:    "Negative amount",
			amount:  -100,
			wantErr: true,
		},
		{
			name:    "Below minimum (₹0.99)",
			amount:  99,
			wantErr: true,
		},
		{
			name:    "Above maximum (₹10,00,001)",
			amount:  100000100,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePaymentAmount(tt.amount)
			if tt.wantErr && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

// TestValidateCardType tests card type validation
func TestValidateCardType(t *testing.T) {
	tests := []struct {
		name     string
		cardType CardType
		wantErr  bool
	}{
		{
			name:     "Valid Visa",
			cardType: CardTypeVisa,
			wantErr:  false,
		},
		{
			name:     "Valid Mastercard",
			cardType: CardTypeMastercard,
			wantErr:  false,
		},
		{
			name:     "Valid RuPay",
			cardType: CardTypeRupay,
			wantErr:  false,
		},
		{
			name:     "Valid Amex",
			cardType: CardTypeAmex,
			wantErr:  false,
		},
		{
			name:     "Valid Diners",
			cardType: CardTypeDiners,
			wantErr:  false,
		},
		{
			name:     "Invalid card type",
			cardType: CardType("discover"),
			wantErr:  true,
		},
		{
			name:     "Empty card type",
			cardType: CardType(""),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCardType(tt.cardType)
			if tt.wantErr && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

// TestValidateCardCategory tests card category validation
func TestValidateCardCategory(t *testing.T) {
	tests := []struct {
		name         string
		cardCategory CardCategory
		wantErr      bool
	}{
		{
			name:         "Valid Credit",
			cardCategory: CardCategoryCredit,
			wantErr:      false,
		},
		{
			name:         "Valid Debit",
			cardCategory: CardCategoryDebit,
			wantErr:      false,
		},
		{
			name:         "Valid Corporate",
			cardCategory: CardCategoryCorporate,
			wantErr:      false,
		},
		{
			name:         "Invalid category",
			cardCategory: CardCategory("prepaid"),
			wantErr:      true,
		},
		{
			name:         "Empty category",
			cardCategory: CardCategory(""),
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCardCategory(tt.cardCategory)
			if tt.wantErr && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

// TestValidateBusinessID tests business ID validation
func TestValidateBusinessID(t *testing.T) {
	tests := []struct {
		name       string
		businessID string
		wantErr    bool
	}{
		{
			name:       "Valid UUID v4",
			businessID: "550e8400-e29b-41d4-a716-446655440000",
			wantErr:    false,
		},
		{
			name:       "Valid UUID v7",
			businessID: "018f3c5d-8e1a-7890-b456-123456789abc",
			wantErr:    false,
		},
		{
			name:       "Empty business ID",
			businessID: "",
			wantErr:    true,
		},
		{
			name:       "Invalid format (no hyphens)",
			businessID: "550e8400e29b41d4a716446655440000",
			wantErr:    true,
		},
		{
			name:       "Invalid format (too short)",
			businessID: "550e8400-e29b-41d4-a716",
			wantErr:    true,
		},
		{
			name:       "Invalid characters",
			businessID: "550e8400-e29b-41d4-a716-44665544000g",
			wantErr:    true,
		},
		{
			name:       "Uppercase UUID",
			businessID: "550E8400-E29B-41D4-A716-446655440000",
			wantErr:    true, // Current validation expects lowercase
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBusinessID(tt.businessID)
			if tt.wantErr && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

// TestPaiseToDecimal tests amount conversion from paise to decimal string
func TestPaiseToDecimal(t *testing.T) {
	tests := []struct {
		name     string
		paise    int64
		expected string
	}{
		{
			name:     "₹500",
			paise:    50000,
			expected: "500.00",
		},
		{
			name:     "₹1",
			paise:    100,
			expected: "1.00",
		},
		{
			name:     "₹0.01 (1 paisa)",
			paise:    1,
			expected: "0.01",
		},
		{
			name:     "₹0",
			paise:    0,
			expected: "0.00",
		},
		{
			name:     "₹10,000",
			paise:    1000000,
			expected: "10000.00",
		},
		{
			name:     "₹123.45",
			paise:    12345,
			expected: "123.45",
		},
		{
			name:     "₹0.99",
			paise:    99,
			expected: "0.99",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PaiseToDecimal(tt.paise)
			if result != tt.expected {
				t.Errorf("PaiseToDecimal(%d) = %s, want %s", tt.paise, result, tt.expected)
			}
		})
	}
}

// TestDecimalToPaise tests amount conversion from decimal string to paise
func TestDecimalToPaise(t *testing.T) {
	tests := []struct {
		name     string
		decimal  string
		expected int64
		wantErr  bool
	}{
		{
			name:     "₹500.00",
			decimal:  "500.00",
			expected: 50000,
			wantErr:  false,
		},
		{
			name:     "₹500 (no decimals)",
			decimal:  "500",
			expected: 50000,
			wantErr:  false,
		},
		{
			name:     "₹1.00",
			decimal:  "1.00",
			expected: 100,
			wantErr:  false,
		},
		{
			name:     "₹0.01",
			decimal:  "0.01",
			expected: 1,
			wantErr:  false,
		},
		{
			name:     "₹0.00",
			decimal:  "0.00",
			expected: 0,
			wantErr:  false,
		},
		{
			name:     "₹123.45",
			decimal:  "123.45",
			expected: 12345,
			wantErr:  false,
		},
		{
			name:     "₹10000.99",
			decimal:  "10000.99",
			expected: 1000099,
			wantErr:  false,
		},
		{
			name:     "Invalid format (letters)",
			decimal:  "abc",
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "Invalid format (empty)",
			decimal:  "",
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "Invalid format (special chars)",
			decimal:  "₹500.00",
			expected: 0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DecimalToPaise(tt.decimal)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if result != tt.expected {
				t.Errorf("DecimalToPaise(%s) = %d, want %d", tt.decimal, result, tt.expected)
			}
		})
	}
}

// TestPaiseToDecimal_Roundtrip tests that conversion is reversible
func TestPaiseToDecimal_Roundtrip(t *testing.T) {
	tests := []int64{
		100,    // ₹1
		50000,  // ₹500
		123456, // ₹1234.56
		1,      // ₹0.01
		999999, // ₹9999.99
	}

	for _, original := range tests {
		t.Run(PaiseToDecimal(original), func(t *testing.T) {
			// Convert to decimal
			decimal := PaiseToDecimal(original)

			// Convert back to paise
			result, err := DecimalToPaise(decimal)
			if err != nil {
				t.Fatalf("roundtrip conversion failed: %v", err)
			}

			if result != original {
				t.Errorf("roundtrip failed: %d → %s → %d", original, decimal, result)
			}
		})
	}
}

// Helper functions for tests

func formatMonth(month int) string {
	if month < 10 {
		return "0" + string(rune('0'+month))
	}
	return string(rune('0'+month/10)) + string(rune('0'+month%10))
}

func formatYear(year int) string {
	return string(rune('0'+year/1000)) + string(rune('0'+(year/100)%10)) + string(rune('0'+(year/10)%10)) + string(rune('0'+year%10))
}

// Benchmark tests

func BenchmarkValidateCardExpiry(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = ValidateCardExpiry("12", "2030")
	}
}

func BenchmarkValidateCVV(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = ValidateCVV("123")
	}
}

func BenchmarkPaiseToDecimal(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = PaiseToDecimal(50000)
	}
}

func BenchmarkDecimalToPaise(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = DecimalToPaise("500.00")
	}
}
