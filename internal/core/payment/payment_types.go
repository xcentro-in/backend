package payment

// PaymentMethod represents different payment methods
type PaymentMethod string

const (
	PaymentMethodCard       PaymentMethod = "card"
	PaymentMethodTokenCard  PaymentMethod = "token_card"
	PaymentMethodNetBanking PaymentMethod = "net_banking"
	PaymentMethodUPI        PaymentMethod = "upi"
	PaymentMethodWallet     PaymentMethod = "wallet"
)

// CardType represents different card types
type CardType string

const (
	CardTypeVisa       CardType = "visa"
	CardTypeMastercard CardType = "mastercard"
	CardTypeRupay      CardType = "rupay"
	CardTypeAmex       CardType = "amex"
	CardTypeDiners     CardType = "diners"
)

// CardCategory represents different card categories
type CardCategory string

const (
	CardCategoryCredit    CardCategory = "credit"
	CardCategoryDebit     CardCategory = "debit"
	CardCategoryCorporate CardCategory = "corporate"
)

// PaymentStatus represents payment processing status
type PaymentStatus string

const (
	PaymentStatusPending   PaymentStatus = "pending"
	PaymentStatusSuccess   PaymentStatus = "success"
	PaymentStatusFailed    PaymentStatus = "failed"
	PaymentStatusCancelled PaymentStatus = "cancelled"
)

// PaymentType represents the type of payment transaction
type PaymentType string

const (
	PaymentTypePayment      PaymentType = "payment"      // Regular payment transaction
	PaymentTypeTokenization PaymentType = "tokenization" // ₹1 card tokenization payment
	PaymentTypeRefund       PaymentType = "refund"       // Refund transaction
)

// SettlementStatus represents settlement status (Phase 7)
// Tracks when gateway funds settle into XCentro escrow account
type SettlementStatus string

const (
	SettlementStatusPending  SettlementStatus = "pending"  // Gateway accepted, awaiting settlement (T+0 to T+2)
	SettlementStatusSettled  SettlementStatus = "settled"  // Funds settled into escrow
	SettlementStatusFailed   SettlementStatus = "failed"   // Settlement failed
	SettlementStatusRefunded SettlementStatus = "refunded" // Funds refunded to customer
)

// GatewayType represents different payment gateways
type GatewayType string

const (
	GatewayJioPay   GatewayType = "jiopay"
	GatewayEasebuzz GatewayType = "easebuzz"
	GatewayMockPG   GatewayType = "mockpg" // Mock gateway for testing
)

// PaymentSourceType represents different types of payment sources
type PaymentSourceType string

const (
	PaymentSourceCard        PaymentSourceType = "card"
	PaymentSourceBankAccount PaymentSourceType = "bank_account"
	PaymentSourceUPI         PaymentSourceType = "upi"
	PaymentSourceCreditLine  PaymentSourceType = "credit_line"
)

// BankAccountType represents different types of bank accounts
type BankAccountType string

const (
	BankAccountTypeSavings BankAccountType = "savings"
	BankAccountTypeCurrent BankAccountType = "current"
	BankAccountTypeCC      BankAccountType = "cc" // Cash Credit
	BankAccountTypeOD      BankAccountType = "od" // Overdraft
)

// UPIProvider represents different UPI service providers
type UPIProvider string

const (
	UPIProviderGooglePay UPIProvider = "googlepay"
	UPIProviderPhonePe   UPIProvider = "phonepe"
	UPIProviderPaytm     UPIProvider = "paytm"
	UPIProviderBHIM      UPIProvider = "bhim"
	UPIProviderAmazonPay UPIProvider = "amazonpay"
	UPIProviderOther     UPIProvider = "other"
)

// CreditLineStatus represents the status of a credit line
type CreditLineStatus string

const (
	CreditLineStatusActive    CreditLineStatus = "active"
	CreditLineStatusInactive  CreditLineStatus = "inactive"
	CreditLineStatusSuspended CreditLineStatus = "suspended"
	CreditLineStatusExpired   CreditLineStatus = "expired"
)

// Currency represents supported currencies
type Currency string

const (
	CurrencyINR Currency = "INR"
	CurrencyUSD Currency = "USD"
)

// FeeType represents different fee calculation types
type FeeType string

const (
	FeeTypePercentage     FeeType = "percentage"
	FeeTypeFixed          FeeType = "fixed"
	FeeTypeHybrid FeeType = "hybrid"
)

// CallingDevice represents the device type initiating the payment
type CallingDevice string

const (
	CallingDeviceWeb    CallingDevice = "web"
	CallingDeviceMobile CallingDevice = "mobile"
	CallingDevicePOS    CallingDevice = "pos"
)

// IsValid checks if the calling device is valid
func (cd CallingDevice) IsValid() bool {
	switch cd {
	case CallingDeviceWeb, CallingDeviceMobile, CallingDevicePOS:
		return true
	default:
		return false
	}
}
