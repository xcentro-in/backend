package business_rules

import (
	"context"
	"fmt"
	"xcentro.in/internal/core/payment"
)

// FeeRule represents a fee calculation rule
type FeeRule struct {
	RuleID           string                `json:"rule_id,omitempty"`
	PaymentMethod    payment.PaymentMethod `json:"payment_method"`
	CardType         payment.CardType      `json:"card_type,omitempty"`
	CardCategory     payment.CardCategory  `json:"card_category,omitempty"`
	FeeType          payment.FeeType       `json:"fee_type"`
	PercentageFee    float64               `json:"percentage_fee"`    // Percentage (e.g., 2.5 for 2.5%)
	FixedFee         int64                 `json:"fixed_fee"`         // Fixed amount in smallest currency unit
	MinFee           int64                 `json:"min_fee"`           // Minimum fee
	MaxFee           int64                 `json:"max_fee"`           // Maximum fee
	GSTPercentage    float64               `json:"gst_percentage"`    // GST percentage (e.g., 18.0 for 18%)
	GSTApplicable    bool                  `json:"gst_applicable"`    // Whether GST applies
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

// DatabaseFeeCalculator defines the interface for fee calculation with GST (Phase 10)
type DatabaseFeeCalculator interface {
	CalculateWithTax(ctx context.Context, amount int64, method payment.PaymentMethod, cardType *payment.CardType, cardCategory *payment.CardCategory) (*FeeCalculationWithTax, error)
	GetFeeRule(ctx context.Context, method payment.PaymentMethod, cardType *payment.CardType, cardCategory *payment.CardCategory) (*FeeRule, error)
}

// FeeRulesRepository defines the interface for fee rules persistence
type FeeRulesRepository interface {
	// GetFeeRule fetches the most specific matching rule for a payment method
	GetFeeRule(ctx context.Context, method payment.PaymentMethod, cardType *payment.CardType, cardCategory *payment.CardCategory) (*FeeRule, error)

	// GetActiveFeeRules retrieves all active fee rules
	GetActiveFeeRules(ctx context.Context) ([]*FeeRule, error)

	// CreateFeeRule creates a new fee rule (for admin API in future)
	CreateFeeRule(ctx context.Context, rule *FeeRule) error

	// UpdateFeeRule updates an existing fee rule
	UpdateFeeRule(ctx context.Context, ruleID string, rule *FeeRule) error

	// DeactivateFeeRule soft deletes a fee rule
	DeactivateFeeRule(ctx context.Context, ruleID string) error
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

// FeeCalculationWithTax represents fee calculation with GST breakdown (Phase 10)
type FeeCalculationWithTax struct {
	// Amounts in paise
	BaseAmount          int64   `json:"base_amount"`
	ConvenienceFee      int64   `json:"convenience_fee"`
	ConvenienceFeeGST   int64   `json:"convenience_fee_gst"`
	TotalFee            int64   `json:"total_fee"`
	TotalAmount         int64   `json:"total_amount"`

	// Percentages for display
	ConvenienceFeePercent float64 `json:"convenience_fee_percent"` // e.g., 1.9
	GSTPercent            float64 `json:"gst_percent"`             // e.g., 18.0

	AppliedRule *FeeRule `json:"applied_rule"`
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

// PostgresDatabaseFeeCalculator implements DatabaseFeeCalculator using database-backed rules
type PostgresDatabaseFeeCalculator struct {
	repo FeeRulesRepository
}

// NewDatabaseFeeCalculator creates a new database-backed fee calculator
func NewDatabaseFeeCalculator(repo FeeRulesRepository) DatabaseFeeCalculator {
	return &PostgresDatabaseFeeCalculator{
		repo: repo,
	}
}

// GetFeeRule fetches the applicable fee rule from the database
func (calc *PostgresDatabaseFeeCalculator) GetFeeRule(
	ctx context.Context,
	method payment.PaymentMethod,
	cardType *payment.CardType,
	cardCategory *payment.CardCategory,
) (*FeeRule, error) {
	return calc.repo.GetFeeRule(ctx, method, cardType, cardCategory)
}

// CalculateWithTax calculates fee with GST breakdown for Phase 10
func (calc *PostgresDatabaseFeeCalculator) CalculateWithTax(
	ctx context.Context,
	amount int64,
	method payment.PaymentMethod,
	cardType *payment.CardType,
	cardCategory *payment.CardCategory,
) (*FeeCalculationWithTax, error) {
	// Fetch applicable rule from database
	rule, err := calc.repo.GetFeeRule(ctx, method, cardType, cardCategory)
	if err != nil {
		return nil, fmt.Errorf("failed to get fee rule: %w", err)
	}

	if rule == nil {
		return nil, fmt.Errorf("no fee rule found for payment method %s", method)
	}

	// Calculate base convenience fee
	var convenienceFee int64
	switch rule.FeeType {
	case payment.FeeTypePercentage:
		convenienceFee = int64(float64(amount) * rule.PercentageFee / 100)

	case payment.FeeTypeFixed:
		convenienceFee = rule.FixedFee

	case payment.FeeTypePercentagePlus:
		percentagePart := int64(float64(amount) * rule.PercentageFee / 100)
		convenienceFee = percentagePart + rule.FixedFee

	default:
		return nil, fmt.Errorf("unsupported fee type: %s", rule.FeeType)
	}

	// Apply min/max fee limits
	if rule.MinFee > 0 && convenienceFee < rule.MinFee {
		convenienceFee = rule.MinFee
	}
	if rule.MaxFee > 0 && convenienceFee > rule.MaxFee {
		convenienceFee = rule.MaxFee
	}

	// Calculate GST on convenience fee
	var convenienceFeeGST int64
	if rule.GSTApplicable {
		convenienceFeeGST = int64(float64(convenienceFee) * rule.GSTPercentage / 100)
	}

	totalFee := convenienceFee + convenienceFeeGST
	totalAmount := amount + totalFee

	return &FeeCalculationWithTax{
		BaseAmount:            amount,
		ConvenienceFee:        convenienceFee,
		ConvenienceFeeGST:     convenienceFeeGST,
		TotalFee:              totalFee,
		TotalAmount:           totalAmount,
		ConvenienceFeePercent: rule.PercentageFee,
		GSTPercent:            rule.GSTPercentage,
		AppliedRule:           rule,
	}, nil
}