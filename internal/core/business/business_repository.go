package business

import (
	"context"

	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/shared"
)

// BusinessRepository defines the interface for interacting with business data.
type BusinessRepository interface {
	WithTx(tx client.Tx) BusinessRepository
	FindByID(ctx context.Context, id string) (*Business, error)
	FindByGSTIN(ctx context.Context, gstin string) (*Business, error)
	CreateBusinessByNameAndGSTIN(ctx context.Context, name string, gstin string) (*Business, error)
	AddKYBSuccessData(ctx context.Context, bus *Business) error
	UpdateKYBData(ctx context.Context, bid string, kybData *BusinessKYBData) (*BusinessKYBData, error)
	UpdateBusinessDeedDocumentKey(ctx context.Context, bid string, key string) error

	// New onboarding-specific methods
	UpdateGSTData(ctx context.Context, bid string, gstDataJSON string) error
	GetBusinessWithKYBData(ctx context.Context, bid string) (*Business, *BusinessKYBData, error)
	GetKYBDataByPAN(ctx context.Context, pan string) (*BusinessKYBData, error)
	UpdateOnboardingFlag(ctx context.Context, bid string, flagName string, value bool) error

	// Ops dashboard methods
	ListBusinessesPaginated(ctx context.Context, page int, limit int, kybStatus string) ([]*OpsBusinessListItem, int, error)
	SearchBusinesses(ctx context.Context, searchTerm string, kybStatus string) ([]*OpsBusinessSearchResult, error)
	GetBusinessDetailsForOps(ctx context.Context, bid string) (*OpsBusinessDetails, error)
	UpdateGSTINData(ctx context.Context, bid string, gstDataJSON string) error
	UpdateIncometTaxIDData(ctx context.Context, itid string, itidDataJSON string) error
	UpdateCompanyIDData(ctx context.Context, itid string, cidDataJSON string) error
	SetKYBStatusRejected(ctx context.Context, bid string) error
	ClearKYBData(ctx context.Context, itid string) error
	SetKYBStatusVerified(ctx context.Context, bid string) error
	GetOwnerUserID(ctx context.Context, bid string) (string, error)
}

// BusinessUserRelationRepository defines the interface for the relationship data.
type BusinessUserRelationRepository interface {
	WithTx(tx client.Tx) BusinessUserRelationRepository
	GetKYBInitiatedStatus(ctx context.Context, uid string) (*BusinessKYBData, *Business, error)
	FindByUser(ctx context.Context, userID string) ([]*BusinessUserRelation, error)
	FindByBusiness(ctx context.Context, bid string) ([]*BusinessUserRelation, error)
	GetBusinessUsersWithDetails(ctx context.Context, bid string) ([]*BusinessUserDTO, error)
	AddUserToBusiness(ctx context.Context, relation *BusinessUserRelation) error
	UpdateUserRole(ctx context.Context, userID string, bid string, newRole shared.UserRole) error
	UpdateRoleClaim(ctx context.Context, userID string, bid string, roleClaim shared.UserRole) error
	UpdateKYBInitiated(ctx context.Context, uid string, bid string, initiated bool) error
	UpdateKycNeeded(ctx context.Context, userID string, bid string, kycNeeded bool) error
	RemoveUserFromBusiness(ctx context.Context, userID string, bid string) error
	SetOwnerRoleToNull(ctx context.Context, bid string) error
	ResetAllUserRelationsForRejectedKYC(ctx context.Context, userID string) error
	OpsUpdateUserRole(ctx context.Context, bid string, userID string, role shared.UserRole) error
}
