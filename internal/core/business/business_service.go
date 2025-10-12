package business

import (
	"context"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/shared"
)

// BusinessService defines the business logic interface for business operations
type BusinessService interface {
	// Utility methods
	UpdateBusinessDeedDocumentKey(ctx context.Context, bid string, key string) error
	GetUserRolesMiddleware() echo.MiddlewareFunc

	// Onboarding flow methods
	InitiateOnboarding(ctx context.Context, uid string, name string, gstin string) (*OnboardingInitResponse, error)
	CompleteKYB(ctx context.Context, uid string, req *CompleteKYBRequest) (*CompleteKYBResponse, error)
	GetOnboardingStatus(ctx context.Context, uid string) (*OnboardingStatusResponse, error)
	ValidateOnboardingEligibility(ctx context.Context, uid string) error

	// Business details
	GetBusinessDetails(ctx context.Context, bid string, userID string) (*BusinessDetailsResponse, error)

	// User management methods
	AddUserToBusiness(ctx context.Context, relation *BusinessUserRelation) error
	GetBusinessUsers(ctx context.Context, bid string) ([]*BusinessUserDTO, error)
	UpdateUserRoleInBusiness(ctx context.Context, bid string, targetUserID string, requesterRole shared.UserRole, newRole shared.UserRole) error
	RemoveUserFromBusiness(ctx context.Context, bid string, targetUserID string, targetUserRole shared.UserRole, requesterRole shared.UserRole) error
}
