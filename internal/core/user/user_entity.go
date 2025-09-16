package user

import "xcentro.in/internal/core/shared"

type User struct {
	Id        string           `json:"id"`
	Email     string           `json:"email"`
	PwdHash   string           `json:"-"`
	Mobile    string           `json:"mobile"`
	PAN       string           `json:"pan"`
	KycStatus shared.KYCStatus `json:"kycStatus"`
	KycData   string           `json:"kycData"`
}
