package payment

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// ValidateCardExpiry validates that a card is not expired
func ValidateCardExpiry(expiryMonth, expiryYear string) error {
	// Parse month
	month, err := strconv.Atoi(expiryMonth)
	if err != nil {
		return fmt.Errorf("invalid expiry month: must be numeric")
	}
	if month < 1 || month > 12 {
		return fmt.Errorf("invalid expiry month: must be between 1 and 12")
	}

	// Parse year (support both YY and YYYY formats)
	year, err := strconv.Atoi(expiryYear)
	if err != nil {
		return fmt.Errorf("invalid expiry year: must be numeric")
	}

	// Convert YY to YYYY (assume 2000s for years < 100)
	if year < 100 {
		year += 2000
	}

	// Check if card is expired
	now := time.Now()
	currentYear := now.Year()
	currentMonth := int(now.Month())

	if year < currentYear {
		return fmt.Errorf("card has expired")
	}

	if year == currentYear && month < currentMonth {
		return fmt.Errorf("card has expired")
	}

	return nil
}

// ValidateCVV validates CVV format (3 or 4 digits)
func ValidateCVV(cvv string) error {
	if cvv == "" {
		return fmt.Errorf("CVV is required")
	}

	// CVV should be 3 or 4 digits
	if len(cvv) < 3 || len(cvv) > 4 {
		return fmt.Errorf("CVV must be 3 or 4 digits")
	}

	// CVV should be numeric
	matched, _ := regexp.MatchString(`^\d{3,4}$`, cvv)
	if !matched {
		return fmt.Errorf("CVV must contain only digits")
	}

	return nil
}

// ValidateCardHolderName validates card holder name
func ValidateCardHolderName(name string) error {
	if name == "" {
		return fmt.Errorf("card holder name is required")
	}

	// Name should be at least 2 characters
	if len(name) < 2 {
		return fmt.Errorf("card holder name must be at least 2 characters")
	}

	// Name should not exceed 100 characters
	if len(name) > 100 {
		return fmt.Errorf("card holder name must not exceed 100 characters")
	}

	return nil
}

// ValidatePaymentAmount validates that payment amount is within acceptable range
func ValidatePaymentAmount(amount int64) error {
	if amount <= 0 {
		return fmt.Errorf("amount must be greater than 0")
	}

	// Minimum payment amount: ₹1 (100 paise)
	if amount < 100 {
		return fmt.Errorf("amount must be at least ₹1 (100 paise)")
	}

	// Maximum payment amount: ₹10,00,000 (1 crore paise)
	maxAmount := int64(100000000) // ₹10,00,000 in paise
	if amount > maxAmount {
		return fmt.Errorf("amount must not exceed ₹10,00,000")
	}

	return nil
}

// ValidateCardType validates that card type is supported
func ValidateCardType(cardType CardType) error {
	switch cardType {
	case CardTypeVisa, CardTypeMastercard, CardTypeRupay, CardTypeAmex, CardTypeDiners:
		return nil
	default:
		return fmt.Errorf("unsupported card type: %s", cardType)
	}
}

// ValidateCardCategory validates that card category is supported
func ValidateCardCategory(cardCategory CardCategory) error {
	switch cardCategory {
	case CardCategoryCredit, CardCategoryDebit, CardCategoryCorporate:
		return nil
	default:
		return fmt.Errorf("unsupported card category: %s", cardCategory)
	}
}

// ValidateBusinessID validates business ID format (UUID)
func ValidateBusinessID(businessID string) error {
	if businessID == "" {
		return fmt.Errorf("business ID is required")
	}

	// UUID format validation (basic check)
	uuidRegex := `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`
	matched, _ := regexp.MatchString(uuidRegex, businessID)
	if !matched {
		return fmt.Errorf("business ID must be a valid UUID")
	}

	return nil
}

// PaiseToDecimal converts paise (int64) to decimal string format
// Example: 50000 → "500.00"
func PaiseToDecimal(paise int64) string {
	rupees := float64(paise) / 100.0
	return fmt.Sprintf("%.2f", rupees)
}

// DecimalToPaise converts decimal string to paise (int64)
// Example: "500.00" → 50000
func DecimalToPaise(decimal string) (int64, error) {
	// Parse decimal string
	rupees, err := strconv.ParseFloat(decimal, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid decimal format: %w", err)
	}

	// Convert to paise (multiply by 100)
	paise := int64(rupees * 100)

	return paise, nil
}
