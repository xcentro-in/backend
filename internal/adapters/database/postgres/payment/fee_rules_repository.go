package payment

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/business_rules"
	"xcentro.in/internal/core/payment"
)

// PostgresFeeRulesRepository implements FeeRulesRepository using PostgreSQL
type PostgresFeeRulesRepository struct {
	querier client.Querier
}

// NewPostgresFeeRulesRepository creates a new PostgreSQL fee rules repository
func NewPostgresFeeRulesRepository(db client.DB) business_rules.FeeRulesRepository {
	return &PostgresFeeRulesRepository{
		querier: db,
	}
}

// GetFeeRule fetches the most specific matching rule for a payment method
// Priority: exact match (method + card_type + card_category) > partial match > generic match
func (r *PostgresFeeRulesRepository) GetFeeRule(
	ctx context.Context,
	method payment.PaymentMethod,
	cardType *payment.CardType,
	cardCategory *payment.CardCategory,
) (*business_rules.FeeRule, error) {
	query := `
		SELECT
			rule_id, payment_method, card_type, card_category,
			fee_type, percentage_fee, fixed_fee, min_fee, max_fee,
			gst_percentage, gst_applicable, priority
		FROM xcentro.payment_fee_rules
		WHERE payment_method = $1
			AND is_active = true
			AND effective_from <= NOW()
			AND (effective_until IS NULL OR effective_until > NOW())
			AND (card_type IS NOT DISTINCT FROM $2)
			AND (card_category IS NOT DISTINCT FROM $3)
		ORDER BY
			priority DESC,
			(card_type IS NOT NULL)::int DESC,          -- Prefer specific card type
			(card_category IS NOT NULL)::int DESC       -- Prefer specific category
		LIMIT 1
	`

	var rule business_rules.FeeRule
	var ruleID, paymentMethodStr, cardTypeStr, cardCategoryStr, feeTypeStr string
	var cardTypeNullable, cardCategoryNullable *string
	var percentageFeeNullable *float64
	var minFeeNullable, maxFeeNullable *int64

	// Convert pointers to interface{} for SQL query (nil or value)
	var cardTypeParam, cardCategoryParam interface{}
	if cardType != nil {
		cardTypeParam = string(*cardType)
	}
	if cardCategory != nil {
		cardCategoryParam = string(*cardCategory)
	}

	err := r.querier.QueryRow(ctx, query, method, cardTypeParam, cardCategoryParam).Scan(
		&ruleID,
		&paymentMethodStr,
		&cardTypeNullable,
		&cardCategoryNullable,
		&feeTypeStr,
		&percentageFeeNullable,
		&rule.FixedFee,
		&minFeeNullable,
		&maxFeeNullable,
		&rule.GSTPercentage,
		&rule.GSTApplicable,
		&rule.Priority,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("no fee rule found for method=%s, card_type=%v, card_category=%v", method, cardType, cardCategory)
		}
		return nil, fmt.Errorf("failed to get fee rule: %w", err)
	}

	// Map to domain types
	rule.RuleID = ruleID
	rule.PaymentMethod = payment.PaymentMethod(paymentMethodStr)
	rule.FeeType = payment.FeeType(feeTypeStr)

	if cardTypeNullable != nil {
		cardTypeStr = *cardTypeNullable
		rule.CardType = payment.CardType(cardTypeStr)
	}

	if cardCategoryNullable != nil {
		cardCategoryStr = *cardCategoryNullable
		rule.CardCategory = payment.CardCategory(cardCategoryStr)
	}

	// Handle nullable percentage_fee, min_fee and max_fee
	if percentageFeeNullable != nil {
		rule.PercentageFee = *percentageFeeNullable
	}
	if minFeeNullable != nil {
		rule.MinFee = *minFeeNullable
	}
	if maxFeeNullable != nil {
		rule.MaxFee = *maxFeeNullable
	}

	rule.IsActive = true
	rule.Currency = payment.CurrencyINR

	return &rule, nil
}

// GetActiveFeeRules retrieves all active fee rules
func (r *PostgresFeeRulesRepository) GetActiveFeeRules(ctx context.Context) ([]*business_rules.FeeRule, error) {
	query := `
		SELECT
			rule_id, payment_method, card_type, card_category,
			fee_type, percentage_fee, fixed_fee, min_fee, max_fee,
			gst_percentage, gst_applicable, priority
		FROM xcentro.payment_fee_rules
		WHERE is_active = true
			AND effective_from <= NOW()
			AND (effective_until IS NULL OR effective_until > NOW())
		ORDER BY payment_method, priority DESC, card_type, card_category
	`

	rows, err := r.querier.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query active fee rules: %w", err)
	}
	defer rows.Close()

	var rules []*business_rules.FeeRule

	for rows.Next() {
		var rule business_rules.FeeRule
		var ruleID, paymentMethodStr, feeTypeStr string
		var cardTypeNullable, cardCategoryNullable *string

		err := rows.Scan(
			&ruleID,
			&paymentMethodStr,
			&cardTypeNullable,
			&cardCategoryNullable,
			&feeTypeStr,
			&rule.PercentageFee,
			&rule.FixedFee,
			&rule.MinFee,
			&rule.MaxFee,
			&rule.GSTPercentage,
			&rule.GSTApplicable,
			&rule.Priority,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan fee rule: %w", err)
		}

		// Map to domain types
		rule.RuleID = ruleID
		rule.PaymentMethod = payment.PaymentMethod(paymentMethodStr)
		rule.FeeType = payment.FeeType(feeTypeStr)

		if cardTypeNullable != nil {
			rule.CardType = payment.CardType(*cardTypeNullable)
		}

		if cardCategoryNullable != nil {
			rule.CardCategory = payment.CardCategory(*cardCategoryNullable)
		}

		rule.IsActive = true
		rule.Currency = payment.CurrencyINR

		rules = append(rules, &rule)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating fee rule rows: %w", err)
	}

	return rules, nil
}

// CreateFeeRule creates a new fee rule (for admin API in future)
func (r *PostgresFeeRulesRepository) CreateFeeRule(ctx context.Context, rule *business_rules.FeeRule) error {
	query := `
		INSERT INTO xcentro.payment_fee_rules (
			rule_id, payment_method, card_type, card_category,
			fee_type, percentage_fee, fixed_fee, min_fee, max_fee,
			gst_percentage, gst_applicable, priority,
			is_active
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8, $9,
			$10, $11, $12,
			$13
		)
	`

	ruleID := uuid.Must(uuid.NewV7()).String()

	_, err := r.querier.Exec(ctx, query,
		ruleID,
		rule.PaymentMethod,
		nullableString(rule.CardType),
		nullableString(rule.CardCategory),
		rule.FeeType,
		rule.PercentageFee,
		rule.FixedFee,
		rule.MinFee,
		rule.MaxFee,
		rule.GSTPercentage,
		rule.GSTApplicable,
		rule.Priority,
		rule.IsActive,
	)

	if err != nil {
		return fmt.Errorf("failed to create fee rule: %w", err)
	}

	rule.RuleID = ruleID
	return nil
}

// UpdateFeeRule updates an existing fee rule
func (r *PostgresFeeRulesRepository) UpdateFeeRule(ctx context.Context, ruleID string, rule *business_rules.FeeRule) error {
	query := `
		UPDATE xcentro.payment_fee_rules
		SET payment_method = $2,
			card_type = $3,
			card_category = $4,
			fee_type = $5,
			percentage_fee = $6,
			fixed_fee = $7,
			min_fee = $8,
			max_fee = $9,
			gst_percentage = $10,
			gst_applicable = $11,
			priority = $12,
			updated_at = NOW()
		WHERE rule_id = $1
	`

	result, err := r.querier.Exec(ctx, query,
		ruleID,
		rule.PaymentMethod,
		nullableString(rule.CardType),
		nullableString(rule.CardCategory),
		rule.FeeType,
		rule.PercentageFee,
		rule.FixedFee,
		rule.MinFee,
		rule.MaxFee,
		rule.GSTPercentage,
		rule.GSTApplicable,
		rule.Priority,
	)

	if err != nil {
		return fmt.Errorf("failed to update fee rule: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("fee rule not found: %s", ruleID)
	}

	return nil
}

// DeactivateFeeRule soft deletes a fee rule
func (r *PostgresFeeRulesRepository) DeactivateFeeRule(ctx context.Context, ruleID string) error {
	query := `
		UPDATE xcentro.payment_fee_rules
		SET is_active = false,
			updated_at = NOW()
		WHERE rule_id = $1
	`

	result, err := r.querier.Exec(ctx, query, ruleID)
	if err != nil {
		return fmt.Errorf("failed to deactivate fee rule: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("fee rule not found: %s", ruleID)
	}

	return nil
}

// Helper function to handle nullable string types
func nullableString(s interface{}) *string {
	switch v := s.(type) {
	case string:
		if v == "" {
			return nil
		}
		return &v
	case payment.CardType:
		if v == "" {
			return nil
		}
		str := string(v)
		return &str
	case payment.CardCategory:
		if v == "" {
			return nil
		}
		str := string(v)
		return &str
	default:
		return nil
	}
}
