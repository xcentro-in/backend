package business

// OpsListBusinessesRequest represents the request for listing businesses
type OpsListBusinessesRequest struct {
	Page      int    `query:"page" validate:"omitempty,gte=1"`
	Limit     int    `query:"limit" validate:"omitempty,gte=1,lte=100"`
	KYBStatus string `query:"kyb_status" validate:"omitempty,oneof=kyc_not_started kyc_in_progress kyc_completed kyc_verified kyc_needs_update kyc_rejected"`
}

// OpsSearchBusinessesRequest represents the request for searching businesses
type OpsSearchBusinessesRequest struct {
	SearchTerm string `query:"q" validate:"required,min=2"`
	KYBStatus  string `query:"kyb_status" validate:"omitempty,oneof=kyc_not_started kyc_in_progress kyc_completed kyc_verified kyc_needs_update kyc_rejected"`
}

// OpsGetBusinessDetailsRequest represents the request for getting business details
type OpsGetBusinessDetailsRequest struct {
	BusinessID string `param:"bid" validate:"required"`
}

// OpsReloadBusinessDataRequest represents the request for reloading business data
type OpsReloadBusinessDataRequest struct {
	BusinessID string `param:"bid" validate:"required"`
	DataType   string `query:"type" validate:"required,oneof=gstin income_tax_id company_id"`
}

// OpsKYBActionRequest represents the request for KYB action
type OpsKYBActionRequest struct {
	BusinessID string `param:"bid" validate:"required"`
	Action     string `query:"action" validate:"required,oneof=reject verify"`
}

// OpsUpdateUserRoleRequest represents the request for updating user role in a business
type OpsUpdateUserRoleRequest struct {
	BusinessID string `param:"bid" validate:"required"`
	UserID     string `query:"user_id" validate:"required"`
	Action     string `query:"action" validate:"required,oneof=set_owner set_admin set_manager set_auditor"`
}

// PaginationResponse represents pagination metadata
type PaginationResponse struct {
	CurrentPage int `json:"currentPage"`
	PageSize    int `json:"pageSize"`
	TotalItems  int `json:"totalItems"`
	TotalPages  int `json:"totalPages"`
}

// OpsListBusinessesResponse represents the response for listing businesses
type OpsListBusinessesResponse struct {
	Data       []*OpsBusinessListItem `json:"data"`
	Pagination PaginationResponse     `json:"pagination"`
}
