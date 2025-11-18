package business

import (
	"context"

	"xcentro.in/internal/core/shared"
)

// BusinessService defines the business logic interface for business operations
type BusinessService interface {
	// Utility methods
	UpdateBusinessDeedDocumentKey(ctx context.Context, bid string, key string) error
	GetKYBDocumentUploadURL(ctx context.Context, userRoles []*BusinessUserRelation) (string, error)
	ConfirmKYBDocumentUpload(ctx context.Context, userRoles []*BusinessUserRelation) (string, error)

	// Onboarding flow methods
	InitiateOnboarding(ctx context.Context, uid string, name string, gstin string) (*OnboardingInitResponse, error)
	CompleteKYB(ctx context.Context, uid string, userRoles []*BusinessUserRelation, req *CompleteKYBRequest) (*CompleteKYBResponse, error)
	GetOnboardingStatus(ctx context.Context, uid string, userRoles []*BusinessUserRelation) (*OnboardingStatusResponse, error)

	// Business details
	GetBusinessDetails(ctx context.Context, bid string, userRole shared.UserRole) (*BusinessDetailsResponse, error)
	GetBusinessKYBData(ctx context.Context, bid string) (*BusinessKYBData, error)
	GetBusiness(ctx context.Context, bid string) (*Business, error)

	GetBusinessOnboardingFlags(ctx context.Context, bid string) (*BusinessOnboardingStatusDTO, error)

	UpdateOnboardingStatus(ctx context.Context, bid string, flagName string, value bool) error

	// User management methods
	InviteUserToBusiness(ctx context.Context, bid string, email string, name string, role shared.UserRole, requesterRole shared.UserRole) (*InviteUserResponse, error)
	AddUserToBusiness(ctx context.Context, relation *BusinessUserRelation) error
	GetBusinessUsers(ctx context.Context, bid string, requesterRole shared.UserRole) ([]*BusinessUserDTO, error)
	UpdateUserRoleInBusiness(ctx context.Context, bid string, targetUserID string, requesterRole shared.UserRole, newRole shared.UserRole) error
	RemoveUserFromBusiness(ctx context.Context, bid string, targetUserID string, requesterRole shared.UserRole) error

	// Ops dashboard methods
	ListBusinessesPaginated(ctx context.Context, page int, limit int, kybStatus string) ([]*OpsBusinessListItem, int, error)
	SearchBusinesses(ctx context.Context, searchTerm string, kybStatus string) ([]*OpsBusinessSearchResult, error)
	GetBusinessDetailsForOps(ctx context.Context, bid string) (*OpsBusinessDetails, error)
	ReloadBusinessData(ctx context.Context, bid string, dataType string) error
	RejectKYB(ctx context.Context, bid string, notes string) error
	VerifyKYB(ctx context.Context, bid string, notes string) error
	OpsUpdateUserRole(ctx context.Context, bid string, userID string, role shared.UserRole) error

	// Payment gateway support methods
	// GetOwnerMobile retrieves the mobile number of the business owner
	// Used by payment gateways (e.g., JioPay) that require owner mobile for externalId construction
	GetOwnerMobile(ctx context.Context, businessID string) (string, error)
}
