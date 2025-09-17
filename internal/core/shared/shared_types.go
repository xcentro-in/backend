package shared

type KYCStatus uint8

const (
	KYCNotStarted KYCStatus = iota
	KYCInReview
	KYCNeedsUpdate
	KYCVerified
	KYCRejected
)

type UserRole uint8

const (
	Owner UserRole = iota
	Admin
	Finance
	Support
)

type BusinessType uint8

const (
	SoleProprietorship BusinessType = iota
	Partnership
	LimitedLiabilityPartnership
	PrivateLimitedCompany
	OnePersonCompany
	Section8NonProfit
	Trust
	NGO
	Society
	CooperativeSociety
	Other
)

type KYBstatus uint8

const (
	KYBNotStarted KYBstatus = iota
	KYBInReview
	KYBNeedsUpdate
	KYBVerified
	KYBRejected
)

type VerificationType uint8

const (
	EmailVerification VerificationType = iota
	MobileVerification
)

type SubscriptionType uint8

const (
	FreePlan SubscriptionType = iota
	BasicPlan
	PremiumPlan
	EnterprisePlan
)
