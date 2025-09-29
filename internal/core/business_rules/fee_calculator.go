package business_rules

import (
	"fmt"
	"xcentro.in/internal/core/payment"
)

// FeeRule represents a fee calculation rule
type FeeRule struct {
	PaymentMethod    payment.PaymentMethod `json:"payment_method"`
	CardType         payment.CardType      `json:"card_type,omitempty"`
	CardCategory     payment.CardCategory  `json:"card_category,omitempty"`
	FeeType          payment.FeeType       `json:"fee_type"`
	PercentageFee    float64               `json:"percentage_fee"`    // Percentage (e.g., 2.5 for 2.5%)
	FixedFee         int64                 `json:"fixed_fee"`         // Fixed amount in smallest currency unit
	MinFee           int64                 `json:"min_fee"`           // Minimum fee
	MaxFee           int64                 `json:"max_fee"`           // Maximum fee
	Currency         payment.Currency      `json:"currency"`
	IsActive         bool                  `json:"is_active"`
	Priority         int                   `json:"priority"`          // Higher priority rules override lower ones
}

// FeeCalculator defines the interface for fee calculation
type FeeCalculator interface {
	CalculateFee(amount int64, method payment.PaymentMethod, cardType payment.CardType, cardCategory payment.CardCategory) (*FeeCalculation, error)
	GetFeeRules(method payment.PaymentMethod) ([]*FeeRule, error)
	GetAllFeeRules() ([]*FeeRule, error)
}

// FeeCalculation represents the result of fee calculation
type FeeCalculation struct {
	BaseAmount    int64                `json:"base_amount"`
	FeeAmount     int64                `json:"fee_amount"`
	TotalAmount   int64                `json:"total_amount"`
	Currency      payment.Currency     `json:"currency"`
	AppliedRule   *FeeRule             `json:"applied_rule"`
	Breakdown     *FeeBreakdown        `json:"breakdown"`
}

// FeeBreakdown provides detailed breakdown of fee calculation
type FeeBreakdown struct {
	PercentageFee float64 `json:"percentage_fee"`
	CalculatedPercentageFee int64 `json:"calculated_percentage_fee"`
	FixedFee      int64   `json:"fixed_fee"`
	MinFeeApplied bool    `json:"min_fee_applied"`
	MaxFeeApplied bool    `json:"max_fee_applied"`
	Description   string  `json:"description"`
}

// MockFeeCalculator provides a mock implementation with hardcoded rules
type MockFeeCalculator struct {
	rules []*FeeRule
}

// NewMockFeeCalculator creates a new mock fee calculator with hardcoded rules
func NewMockFeeCalculator() FeeCalculator {
	return &MockFeeCalculator{
		rules: getMockFeeRules(),
	}
}

// CalculateFee calculates fee based on hardcoded rules with detailed logging
func (calc *MockFeeCalculator) CalculateFee(amount int64, method payment.PaymentMethod, cardType payment.CardType, cardCategory payment.CardCategory) (*FeeCalculation, error) {
	fmt.Println("=== FEE CALCULATOR: CalculateFee Called ===")
	fmt.Printf("Base Amount: %d\n", amount)
	fmt.Printf("Payment Method: %s\n", method)
	if method == payment.PaymentMethodCard {
		fmt.Printf("Card Type: %s\n", cardType)
		fmt.Printf("Card Category: %s\n", cardCategory)
	}

	// Find applicable rule
	rule := calc.findApplicableRule(method, cardType, cardCategory)
	if rule == nil {
		fmt.Println("❌ No applicable fee rule found, using default")
		rule = &FeeRule{
			PaymentMethod: method,
			FeeType:       payment.FeeTypePercentage,
			PercentageFee: 2.0,
			Currency:      payment.CurrencyINR,
			IsActive:      true,
		}
	}

	fmt.Printf("📋 Applied Rule: %s fee for %s", rule.FeeType, rule.PaymentMethod)
	if rule.CardType != "" {
		fmt.Printf(" (%s %s)", rule.CardCategory, rule.CardType)
	}
	fmt.Println()

	// Calculate fee
	var feeAmount int64
	var breakdown *FeeBreakdown

	switch rule.FeeType {
	case payment.FeeTypePercentage:
		calculatedFee := int64(float64(amount) * rule.PercentageFee / 100)
		feeAmount = calculatedFee
		breakdown = &FeeBreakdown{
			PercentageFee:           rule.PercentageFee,
			CalculatedPercentageFee: calculatedFee,
			Description:             fmt.Sprintf("%.2f%% of %d", rule.PercentageFee, amount),
		}
		fmt.Printf("💰 Percentage Fee: %.2f%% of %d = %d\n", rule.PercentageFee, amount, calculatedFee)

	case payment.FeeTypeFixed:
		feeAmount = rule.FixedFee
		breakdown = &FeeBreakdown{
			FixedFee:    rule.FixedFee,
			Description: fmt.Sprintf("Fixed fee: %d", rule.FixedFee),
		}
		fmt.Printf("💰 Fixed Fee: %d\n", rule.FixedFee)

	case payment.FeeTypePercentagePlus:
		calculatedPercentageFee := int64(float64(amount) * rule.PercentageFee / 100)
		feeAmount = calculatedPercentageFee + rule.FixedFee
		breakdown = &FeeBreakdown{
			PercentageFee:           rule.PercentageFee,
			CalculatedPercentageFee: calculatedPercentageFee,
			FixedFee:                rule.FixedFee,
			Description:             fmt.Sprintf("%.2f%% + %d = %d + %d", rule.PercentageFee, rule.FixedFee, calculatedPercentageFee, rule.FixedFee),
		}
		fmt.Printf("💰 Percentage + Fixed Fee: %.2f%% (%d) + %d = %d\n", rule.PercentageFee, calculatedPercentageFee, rule.FixedFee, feeAmount)
	}

	// Apply min/max fee limits
	if rule.MinFee > 0 && feeAmount < rule.MinFee {
		fmt.Printf("⬆️  Applying minimum fee: %d → %d\n", feeAmount, rule.MinFee)
		feeAmount = rule.MinFee
		breakdown.MinFeeApplied = true
	}

	if rule.MaxFee > 0 && feeAmount > rule.MaxFee {
		fmt.Printf("⬇️  Applying maximum fee: %d → %d\n", feeAmount, rule.MaxFee)
		feeAmount = rule.MaxFee
		breakdown.MaxFeeApplied = true
	}

	calculation := &FeeCalculation{
		BaseAmount:  amount,
		FeeAmount:   feeAmount,
		TotalAmount: amount + feeAmount,
		Currency:    rule.Currency,
		AppliedRule: rule,
		Breakdown:   breakdown,
	}

	fmt.Printf("📊 Final Calculation: %d + %d = %d %s\n", amount, feeAmount, calculation.TotalAmount, calculation.Currency)
	fmt.Println("=== FEE CALCULATOR: CalculateFee Complete ===")

	return calculation, nil
}

// GetFeeRules returns fee rules for a specific payment method
func (calc *MockFeeCalculator) GetFeeRules(method payment.PaymentMethod) ([]*FeeRule, error) {
	fmt.Printf("=== FEE CALCULATOR: GetFeeRules for %s ===\n", method)

	var rules []*FeeRule
	for _, rule := range calc.rules {
		if rule.PaymentMethod == method && rule.IsActive {
			rules = append(rules, rule)
		}
	}

	fmt.Printf("Found %d fee rules for %s\n", len(rules), method)
	for i, rule := range rules {
		fmt.Printf("%d. %s fee: %.2f%% + %d (min: %d, max: %d)\n",
			i+1, rule.FeeType, rule.PercentageFee, rule.FixedFee, rule.MinFee, rule.MaxFee)
	}

	return rules, nil
}

// GetAllFeeRules returns all active fee rules
func (calc *MockFeeCalculator) GetAllFeeRules() ([]*FeeRule, error) {
	fmt.Println("=== FEE CALCULATOR: GetAllFeeRules ===")

	var activeRules []*FeeRule
	for _, rule := range calc.rules {
		if rule.IsActive {
			activeRules = append(activeRules, rule)
		}
	}

	fmt.Printf("Found %d active fee rules\n", len(activeRules))
	return activeRules, nil
}

// findApplicableRule finds the most specific applicable rule
func (calc *MockFeeCalculator) findApplicableRule(method payment.PaymentMethod, cardType payment.CardType, cardCategory payment.CardCategory) *FeeRule {
	var bestRule *FeeRule
	highestPriority := -1

	for _, rule := range calc.rules {
		if !rule.IsActive {
			continue
		}

		if rule.PaymentMethod != method {
			continue
		}

		// For card payments, check card-specific rules
		if method == payment.PaymentMethodCard {
			// Skip if rule specifies card type/category but doesn't match
			if rule.CardType != "" && rule.CardType != cardType {
				continue
			}
			if rule.CardCategory != "" && rule.CardCategory != cardCategory {
				continue
			}
		}

		// Select rule with highest priority
		if rule.Priority > highestPriority {
			highestPriority = rule.Priority
			bestRule = rule
		}
	}

	return bestRule
}

// getMockFeeRules returns hardcoded fee rules for testing
func getMockFeeRules() []*FeeRule {
	return []*FeeRule{
		// Card payment rules
		{
			PaymentMethod: payment.PaymentMethodCard,
			CardCategory:  payment.CardCategoryCredit,
			FeeType:       payment.FeeTypePercentage,
			PercentageFee: 2.5,
			MinFee:        500,  // ₹5 minimum
			MaxFee:        10000, // ₹100 maximum
			Currency:      payment.CurrencyINR,
			IsActive:      true,
			Priority:      10,
		},
		{
			PaymentMethod: payment.PaymentMethodCard,
			CardCategory:  payment.CardCategoryDebit,
			FeeType:       payment.FeeTypePercentage,
			PercentageFee: 2.0,
			MinFee:        300,  // ₹3 minimum
			MaxFee:        8000, // ₹80 maximum
			Currency:      payment.CurrencyINR,
			IsActive:      true,
			Priority:      10,
		},
		{
			PaymentMethod: payment.PaymentMethodCard,
			CardCategory:  payment.CardCategoryCorporate,
			FeeType:       payment.FeeTypePercentage,
			PercentageFee: 1.8,
			MinFee:        1000, // ₹10 minimum
			MaxFee:        15000, // ₹150 maximum
			Currency:      payment.CurrencyINR,
			IsActive:      true,
			Priority:      15, // Higher priority than generic card rules
		},

		// Net banking rules
		{
			PaymentMethod: payment.PaymentMethodNetBanking,
			FeeType:       payment.FeeTypeFixed,
			FixedFee:      1500, // ₹15 flat fee
			Currency:      payment.CurrencyINR,
			IsActive:      true,
			Priority:      5,
		},

		// UPI rules
		{
			PaymentMethod: payment.PaymentMethodUPI,
			FeeType:       payment.FeeTypeFixed,
			FixedFee:      0, // Free UPI
			Currency:      payment.CurrencyINR,
			IsActive:      true,
			Priority:      5,
		},

		// Wallet rules
		{
			PaymentMethod: payment.PaymentMethodWallet,
			FeeType:       payment.FeeTypePercentage,
			PercentageFee: 1.0,
			MinFee:        200,  // ₹2 minimum
			MaxFee:        5000, // ₹50 maximum
			Currency:      payment.CurrencyINR,
			IsActive:      true,
			Priority:      5,
		},
	}
}