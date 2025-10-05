package shared

// RequiresCIN returns true if the business type requires a Company Identification Number (CIN)
// CIN is required for:
// - Private Limited Company
// - One Person Company
// - Section 8 Non-Profit
// - Limited Liability Partnership
func RequiresCIN(businessType BusinessType) bool {
	switch businessType {
	case PrivateLimitedCompany, OnePersonCompany, Section8NonProfit, LimitedLiabilityPartnership:
		return true
	default:
		return false
	}
}

// RequiresDeed returns true if the business type requires a business deed document upload
// Business deed is required for all business types that don't require CIN
func RequiresDeed(businessType BusinessType) bool {
	return !RequiresCIN(businessType)
}

// IsValidBusinessType checks if the provided business type is valid
func IsValidBusinessType(businessType BusinessType) bool {
	switch businessType {
	case SoleProprietorship, Partnership, LimitedLiabilityPartnership,
		PrivateLimitedCompany, OnePersonCompany, Section8NonProfit,
		Trust, NGO, Society, CooperativeSociety, Other:
		return true
	default:
		return false
	}
}
