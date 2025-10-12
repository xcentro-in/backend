package auth

type AuthPayload struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	AuthCode string `query:"code"`
}

// SignupResponse represents the response after successful user registration
type SignupResponse struct {
	UserID  string `json:"userId"`
	Message string `json:"message"`
}

// LoginResponse represents the response after successful login
// Only contains essential information - token and onboarding flags
type LoginResponse struct {
	Token     string `json:"token"`
	KycNeeded bool   `json:"kycNeeded"` // Whether user needs to complete KYC
	KybNeeded bool   `json:"kybNeeded"` // Whether user needs to complete KYB for their business
}

// OnboardingStatusCache represents cached onboarding status for login optimization
// Cached to avoid repeated database queries during login
type OnboardingStatusCache struct {
	KycNeeded bool `json:"kycNeeded"`
	KybNeeded bool `json:"kybNeeded"`
}

// type AuthJWTClaim struct {
// 	jwt.RegisteredClaims
// 	Roles []shared.UserRole `json:"rls"`
// 	BIDs  []string          `json:"bids"`
// }
