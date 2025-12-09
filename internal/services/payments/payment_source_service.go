package payments

import (
	"context"
	"fmt"

	"xcentro.in/internal/core/business_rules"
	"xcentro.in/internal/core/payment"
)

// PaymentSourceWithFees represents a payment source with fee breakdown for Phase 10
type PaymentSourceWithFees struct {
	// Source identification
	SourceID      string                    `json:"source_id"`
	SourceType    payment.PaymentSourceType `json:"source_type"`
	PaymentMethod payment.PaymentMethod     `json:"payment_method"`
	DisplayName   string                    `json:"display_name"`

	// Card-specific fields
	CardType       *payment.CardType     `json:"card_type,omitempty"`
	CardCategory   *payment.CardCategory `json:"card_category,omitempty"`
	LastFourDigits string                `json:"last_four_digits,omitempty"`

	// Bank account specific
	LastFiveDigits string `json:"last_five_digits,omitempty"`
	BankName       string `json:"bank_name,omitempty"`
	IFSCCode       string `json:"ifsc_code,omitempty"`

	// UPI specific
	UPIAddress string `json:"upi_address,omitempty"`

	// Restriction information
	IsAllowed         bool   `json:"is_allowed"`
	RestrictionReason string `json:"restriction_reason,omitempty"`

	// Fee breakdown (only for allowed sources)
	FeeBreakdown *business_rules.FeeCalculationWithTax `json:"fee_breakdown,omitempty"`
}

// GetPaymentSourcesResponse represents the API response for Phase 10
type GetPaymentSourcesResponse struct {
	BusinessID     string                   `json:"business_id"`
	PaymentContext payment.PaymentContext   `json:"payment_context"`
	BaseAmount     int64                    `json:"base_amount"`
	Sources        []*PaymentSourceWithFees `json:"sources"`
}

// PaymentSourceService handles payment source selection with context-aware fees
type PaymentSourceService struct {
	walletService business_rules.WalletService
	feeCalculator business_rules.DatabaseFeeCalculator
}

// NewPaymentSourceService creates a new payment source service
func NewPaymentSourceService(
	walletService business_rules.WalletService,
	feeCalculator business_rules.DatabaseFeeCalculator,
) *PaymentSourceService {
	return &PaymentSourceService{
		walletService: walletService,
		feeCalculator: feeCalculator,
	}
}

// GetPaymentSourcesWithFees returns payment sources filtered by context with fee breakdown
func (s *PaymentSourceService) GetPaymentSourcesWithFees(
	ctx context.Context,
	businessID string,
	paymentContext payment.PaymentContext,
	baseAmount int64,
) (*GetPaymentSourcesResponse, error) {
	// Validate payment context
	if !paymentContext.IsValid() {
		return nil, fmt.Errorf("invalid payment context: %s", paymentContext)
	}

	// Fetch business wallet (showInactive = false - only active payment sources)
	wallet, err := s.walletService.GetBusinessWallet(ctx, businessID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get business wallet: %w", err)
	}

	// OPTIMIZATION: Fetch all active fee rules once (single DB call)
	// Instead of calling DB for each payment source, we load all rules and match in-memory
	allFeeRules, err := s.feeCalculator.GetActiveFeeRules(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get fee rules: %w", err)
	}

	// Process all payment sources
	sources := make([]*PaymentSourceWithFees, 0)

	// Process stored cards
	for _, card := range wallet.StoredCards {
		if !card.IsActive {
			continue
		}

		sourceWithFees := s.processCardSource(card, paymentContext, baseAmount, allFeeRules)
		sources = append(sources, sourceWithFees)
	}

	// Process bank accounts (net banking)
	for _, account := range wallet.BankAccounts {
		if !account.IsActive || !account.IsVerified {
			continue
		}

		sourceWithFees := s.processBankAccountSource(account, paymentContext, baseAmount, allFeeRules)
		sources = append(sources, sourceWithFees)
	}

	// Process UPI details
	for _, upi := range wallet.UPIDetails {
		if !upi.IsActive || !upi.IsVerified {
			continue
		}

		sourceWithFees := s.processUPISource(upi, paymentContext, baseAmount, allFeeRules)
		sources = append(sources, sourceWithFees)
	}

	return &GetPaymentSourcesResponse{
		BusinessID:     businessID,
		PaymentContext: paymentContext,
		BaseAmount:     baseAmount,
		Sources:        sources,
	}, nil
}

// processCardSource processes a stored card and calculates fees
func (s *PaymentSourceService) processCardSource(
	card *payment.StoredCard,
	paymentContext payment.PaymentContext,
	baseAmount int64,
	allFeeRules []*business_rules.FeeRule,
) *PaymentSourceWithFees {
	source := &PaymentSourceWithFees{
		SourceID:       card.CardID,
		SourceType:     payment.PaymentSourceCard,
		PaymentMethod:  payment.PaymentMethodCard,
		DisplayName:    fmt.Sprintf("%s %s ending in %s", card.CardCategory, card.CardType, card.LastFourDigits),
		CardType:       &card.CardType,
		CardCategory:   &card.CardCategory,
		LastFourDigits: card.LastFourDigits,
	}

	// Check if card is allowed for this payment context
	isAllowed, reason := business_rules.IsPaymentSourceAllowed(
		payment.PaymentMethodCard,
		&card.CardType,
		paymentContext,
	)

	source.IsAllowed = isAllowed
	source.RestrictionReason = reason

	// Calculate fees only if allowed
	if isAllowed {
		// OPTIMIZATION: Find matching rule in-memory (no DB call)
		rule, err := business_rules.FindMatchingFeeRule(
			allFeeRules,
			payment.PaymentMethodCard,
			&card.CardType,
			&card.CardCategory,
		)
		if err != nil {
			// Log error but don't fail - just don't include fee breakdown
			source.IsAllowed = false
			source.RestrictionReason = "Fee rule not found"
		} else {
			// Calculate fee using the found rule
			feeBreakdown, err := business_rules.CalculateFeeWithRule(baseAmount, rule)
			if err != nil {
				source.IsAllowed = false
				source.RestrictionReason = "Fee calculation failed"
			} else {
				source.FeeBreakdown = feeBreakdown
			}
		}
	}

	return source
}

// processBankAccountSource processes a bank account (net banking) and calculates fees
func (s *PaymentSourceService) processBankAccountSource(
	account *payment.BankAccount,
	paymentContext payment.PaymentContext,
	baseAmount int64,
	allFeeRules []*business_rules.FeeRule,
) *PaymentSourceWithFees {
	source := &PaymentSourceWithFees{
		SourceID:       account.AccountID,
		SourceType:     payment.PaymentSourceBankAccount,
		PaymentMethod:  payment.PaymentMethodNetBanking,
		DisplayName:    fmt.Sprintf("%s ending in %s", account.BankName, account.LastFiveDigits),
		BankName:       account.BankName,
		IFSCCode:       account.IFSCCode,
		LastFiveDigits: account.LastFiveDigits,
	}

	// Check if net banking is allowed for this payment context
	isAllowed, reason := business_rules.IsPaymentSourceAllowed(
		payment.PaymentMethodNetBanking,
		nil,
		paymentContext,
	)

	source.IsAllowed = isAllowed
	source.RestrictionReason = reason

	// Calculate fees only if allowed
	if isAllowed {
		// OPTIMIZATION: Find matching rule in-memory (no DB call)
		rule, err := business_rules.FindMatchingFeeRule(
			allFeeRules,
			payment.PaymentMethodNetBanking,
			nil,
			nil,
		)
		if err != nil {
			source.IsAllowed = false
			source.RestrictionReason = "Fee rule not found"
		} else {
			// Calculate fee using the found rule
			feeBreakdown, err := business_rules.CalculateFeeWithRule(baseAmount, rule)
			if err != nil {
				source.IsAllowed = false
				source.RestrictionReason = "Fee calculation failed"
			} else {
				source.FeeBreakdown = feeBreakdown
			}
		}
	}

	return source
}

// processUPISource processes a UPI detail and calculates fees
func (s *PaymentSourceService) processUPISource(
	upi *payment.UPIDetail,
	paymentContext payment.PaymentContext,
	baseAmount int64,
	allFeeRules []*business_rules.FeeRule,
) *PaymentSourceWithFees {
	source := &PaymentSourceWithFees{
		SourceID:      upi.UPIID,
		SourceType:    payment.PaymentSourceUPI,
		PaymentMethod: payment.PaymentMethodUPI,
		DisplayName:   fmt.Sprintf("UPI: %s", upi.UPIAddress),
		UPIAddress:    upi.UPIAddress,
	}

	// Check if UPI is allowed for this payment context
	isAllowed, reason := business_rules.IsPaymentSourceAllowed(
		payment.PaymentMethodUPI,
		nil,
		paymentContext,
	)

	source.IsAllowed = isAllowed
	source.RestrictionReason = reason

	// Calculate fees only if allowed
	if isAllowed {
		// OPTIMIZATION: Find matching rule in-memory (no DB call)
		rule, err := business_rules.FindMatchingFeeRule(
			allFeeRules,
			payment.PaymentMethodUPI,
			nil,
			nil,
		)
		if err != nil {
			source.IsAllowed = false
			source.RestrictionReason = "Fee rule not found"
		} else {
			// Calculate fee using the found rule
			feeBreakdown, err := business_rules.CalculateFeeWithRule(baseAmount, rule)
			if err != nil {
				source.IsAllowed = false
				source.RestrictionReason = "Fee calculation failed"
			} else {
				source.FeeBreakdown = feeBreakdown
			}
		}
	}

	return source
}
