package auth

type AuthPayload struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	AuthCode string `query:"code"`
}

// type AuthJWTClaim struct {
// 	jwt.RegisteredClaims
// 	Roles []shared.UserRole `json:"rls"`
// 	BIDs  []string          `json:"bids"`
// }
