package shared

type KYCStatus string

const (
	KYCNotStarted  KYCStatus = "kyc_not_started"
	KYCInProgress  KYCStatus = "kyc_in_progress"
	KYCCompleted   KYCStatus = "kyc_completed"
	KYCVerified    KYCStatus = "kyc_verified"
	KYCNeedsUpdate KYCStatus = "kyc_needs_update"
	KYCRejected    KYCStatus = "kyc_rejected"
)

type UserRole string

const (
	Owner   UserRole = "owner"
	Admin   UserRole = "admin"
	Finance UserRole = "finance"
	Support UserRole = "support"
)

type BusinessType string

const (
	SoleProprietorship          BusinessType = "sole_proprietorship"
	Partnership                 BusinessType = "partnership"
	LimitedLiabilityPartnership BusinessType = "limited_liability_partnership"
	PrivateLimitedCompany       BusinessType = "private_limited_company"
	OnePersonCompany            BusinessType = "one_person_company"
	Section8NonProfit           BusinessType = "section_8_non_profit"
	Trust                       BusinessType = "trust"
	NGO                         BusinessType = "ngo"
	Society                     BusinessType = "society"
	CooperativeSociety          BusinessType = "cooperative_society"
	Other                       BusinessType = "other"
)

type KYBstatus string

const (
	KYBNotStarted  KYBstatus = "kyc_not_started"
	KYBInProgress  KYBstatus = "kyc_in_progress"
	KYBCompleted   KYBstatus = "kyc_completed"
	KYBVerified    KYBstatus = "kyc_verified"
	KYBNeedsUpdate KYBstatus = "kyc_needs_update"
	KYBRejected    KYBstatus = "kyc_rejected"
)

type VerificationType string

const (
	EmailVerification  VerificationType = "email_verification"
	MobileVerification VerificationType = "mobile_verification"
	PasswordReset      VerificationType = "password_reset"
)

type SubscriptionType string

const (
	FreePlan       SubscriptionType = "free_plan"
	BasicPlan      SubscriptionType = "basic_plan"
	PremiumPlan    SubscriptionType = "premium_plan"
	EnterprisePlan SubscriptionType = "enterprise_plan"
)
