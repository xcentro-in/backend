package user

// OpsUserSearchRequest represents the search request for ops dashboard
type OpsUserSearchRequest struct {
	BusinessIDs []string `query:"business_id" validate:"required,min=1,max=20,dive,required"`
	SearchTerm  string   `query:"q" validate:"required,min=2,max=100"`
}

// OpsUserDetailsRequest represents the request for getting user details
type OpsUserDetailsRequest struct {
	UserID     string `param:"user_id" validate:"required"`
	BusinessID string `query:"bid"`
}

// OpsReloadUserPANRequest represents the request for reloading user PAN data
type OpsReloadUserPANRequest struct {
	UserID   string `param:"user_id" validate:"required"`
	DataType string `query:"type" json:"dataType" validate:"required,oneof=pan_com pan_aadhaar_link"`
}

// OpsKYCActionRequest represents the request for KYC action
type OpsKYCActionRequest struct {
	UserID string `param:"user_id" validate:"required"`
	Action string `query:"action" validate:"required,oneof=reject approve"`
	Notes  string `json:"notes" validate:"omitempty,max=500"`
}
