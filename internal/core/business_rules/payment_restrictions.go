package business_rules

import (
	"xcentro.in/internal/core/payment"
)

// PaymentRestrictionRule defines the rules for a specific payment context
type PaymentRestrictionRule struct {
	Context          payment.PaymentContext
	AllowedMethods   []payment.PaymentMethod
	BlockedCardTypes []payment.CardType
	Reason           string // User-facing message for frontend tooltip when blocked
}

// GetRestrictionRules returns all payment restriction rules by context
func GetRestrictionRules() map[payment.PaymentContext]*PaymentRestrictionRule {
	return map[payment.PaymentContext]*PaymentRestrictionRule{
		// Payroll: Block Amex and Diners per card scheme compliance rules
		payment.PaymentContextPayroll: {
			Context: payment.PaymentContextPayroll,
			AllowedMethods: []payment.PaymentMethod{
				payment.PaymentMethodCard,
				payment.PaymentMethodNetBanking,
				payment.PaymentMethodUPI,
			},
			BlockedCardTypes: []payment.CardType{
				payment.CardTypeAmex,
				payment.CardTypeDiners,
			},
			Reason: "Amex and Diners cards are not allowed for payroll payments per card scheme rules",
		},

		// GST Payment: Only net banking or UPI allowed
		payment.PaymentContextGSTPayment: {
			Context: payment.PaymentContextGSTPayment,
			AllowedMethods: []payment.PaymentMethod{
				payment.PaymentMethodNetBanking,
				payment.PaymentMethodUPI,
			},
			BlockedCardTypes: []payment.CardType{}, // All cards blocked implicitly by allowed methods
			Reason:           "Only net banking or UPI payments are allowed for GST payments",
		},

		// Vendor Payment: All payment methods allowed
		payment.PaymentContextVendorPayment: {
			Context: payment.PaymentContextVendorPayment,
			AllowedMethods: []payment.PaymentMethod{
				payment.PaymentMethodCard,
				payment.PaymentMethodNetBanking,
				payment.PaymentMethodUPI,
			},
			BlockedCardTypes: []payment.CardType{},
			Reason:           "",
		},

		// Bill Payment: All payment methods allowed
		payment.PaymentContextBillPayment: {
			Context: payment.PaymentContextBillPayment,
			AllowedMethods: []payment.PaymentMethod{
				payment.PaymentMethodCard,
				payment.PaymentMethodNetBanking,
				payment.PaymentMethodUPI,
			},
			BlockedCardTypes: []payment.CardType{},
			Reason:           "",
		},
	}
}

// IsPaymentSourceAllowed checks if a payment source is allowed for the given context
// Returns (isAllowed, reason) where reason is a user-facing message if blocked
func IsPaymentSourceAllowed(
	paymentMethod payment.PaymentMethod,
	cardType *payment.CardType,
	context payment.PaymentContext,
) (bool, string) {
	rules := GetRestrictionRules()

	rule, exists := rules[context]
	if !exists {
		// Unknown context - allow by default
		return true, ""
	}

	// Check if payment method is allowed
	methodAllowed := false
	for _, allowed := range rule.AllowedMethods {
		if paymentMethod == allowed {
			methodAllowed = true
			break
		}
	}

	if !methodAllowed {
		return false, rule.Reason
	}

	// If payment method is card, check card type restrictions
	if paymentMethod == payment.PaymentMethodCard && cardType != nil {
		for _, blocked := range rule.BlockedCardTypes {
			if *cardType == blocked {
				return false, rule.Reason
			}
		}
	}

	return true, ""
}

// GetBlockedCardTypesForContext returns the list of blocked card types for a context
func GetBlockedCardTypesForContext(context payment.PaymentContext) []payment.CardType {
	rules := GetRestrictionRules()
	if rule, exists := rules[context]; exists {
		return rule.BlockedCardTypes
	}
	return []payment.CardType{}
}

// GetAllowedMethodsForContext returns the list of allowed payment methods for a context
func GetAllowedMethodsForContext(context payment.PaymentContext) []payment.PaymentMethod {
	rules := GetRestrictionRules()
	if rule, exists := rules[context]; exists {
		return rule.AllowedMethods
	}
	// Default: allow all methods
	return []payment.PaymentMethod{
		payment.PaymentMethodCard,
		payment.PaymentMethodNetBanking,
		payment.PaymentMethodUPI,
	}
}
