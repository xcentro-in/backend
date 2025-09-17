package business

import "xcentro.in/internal/core/shared"

// BusinessRepository defines the interface for interacting with business data.
type BusinessRepository interface {
	FindByID(id string) (*Business, error)
	FindByGSTIN(gstin string) (*Business, error)
	Create(business *Business) error
	Update(business *Business) error
	UpdateKYBData(bid string, kybData *BusinessKYBData) (*BusinessKYBData, error)
	UpdateSubscriptionStatus(bid string, subData *BusinessSubscription) error
}

// BusinessUserRelationRepository defines the interface for the relationship data.
type BusinessUserRelationRepository interface {
	FindByUser(userID string) ([]*BusinessUserRelation, error)
	FindByBusiness(bid string) ([]*BusinessUserRelation, error)
	AddUserToBusiness(relation *BusinessUserRelation) error
	UpdateUserRole(userID string, bid string, newRole shared.UserRole) error
	RemoveUserFromBusiness(userID string, bid string) error
}
