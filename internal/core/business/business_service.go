package business

import (
	"context"

	"xcentro.in/internal/core/shared"
)

// BusinessService defines the business logic interface for business operations
type BusinessService interface {
	GetBusinessByID(ctx context.Context, bid string) (*Business, error)
	GetBusinessByGSTIN(ctx context.Context, gstin string) (*Business, error)
	CreateBusinessByNameAndGSTIN(ctx context.Context, name string, gstin string, uid string) (*Business, error)
	CreateBusiness(ctx context.Context, business *Business) (*Business, error)
	UpdateBusiness(ctx context.Context, business *Business) (*Business, error)
	UpdateKYBData(ctx context.Context, bid string, kybData *BusinessKYBData) (*BusinessKYBData, error)
	UpdateSubscriptionStatus(ctx context.Context, bid string, subData *BusinessSubscription) error

	// Business-User relations
	GetBusinessesByUser(ctx context.Context, userID string) ([]*BusinessUserRelation, error)
	GetUsersByBusiness(ctx context.Context, bid string) ([]*BusinessUserRelation, error)
	AddBusinessKYBInitiated(ctx context.Context, bid string, uid string) error
	AddUserToBusiness(ctx context.Context, relation *BusinessUserRelation) error
	UpdateUserRole(ctx context.Context, userID string, bid string, newRole shared.UserRole) error
	RemoveUserFromBusiness(ctx context.Context, userID string, bid string) error
}
