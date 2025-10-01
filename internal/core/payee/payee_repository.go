package payee

import (
	"context"

	"xcentro.in/internal/adapters/database/postgres/client"
)

// PayeeBusinessRepository manages canonical payee business entities
// Handles global payee records identified by GSTIN
type PayeeBusinessRepository interface {
	WithTx(tx client.Tx) PayeeBusinessRepository

	// CRUD operations
	CreatePayeeBusiness(ctx context.Context, payee *PayeeBusiness) error
	FindByID(ctx context.Context, payeeID string) (*PayeeBusiness, error)
	FindByGSTIN(ctx context.Context, gstin string) (*PayeeBusiness, error)
	UpdatePayeeBusiness(ctx context.Context, payee *PayeeBusiness) error

	// Search operations
	SearchPayees(ctx context.Context, filters PayeeBusinessSearchFilters) ([]*PayeeBusiness, error)
}

// PayeeBankAccountRepository manages bank accounts for payees
// Handles one-to-many relationship: one payee can have multiple bank accounts
type PayeeBankAccountRepository interface {
	WithTx(tx client.Tx) PayeeBankAccountRepository

	// CRUD operations
	CreateBankAccount(ctx context.Context, account *PayeeBankAccount) error
	FindByID(ctx context.Context, accountID string) (*PayeeBankAccount, error)
	FindByPayeeID(ctx context.Context, payeeID string) ([]*PayeeBankAccount, error)
	UpdateBankAccount(ctx context.Context, account *PayeeBankAccount) error

	// Duplicate detection
	// Checks if account with same hash already exists for this payee
	CheckDuplicateAccount(ctx context.Context, payeeID, accountHash string) (bool, error)

	// Primary account management
	GetPrimaryAccount(ctx context.Context, payeeID string) (*PayeeBankAccount, error)
	SetPrimaryAccount(ctx context.Context, accountID string) error
}

// BusinessPayeeRelationRepository manages customer-payee-account relationships
// Handles many-to-many-to-many mapping: business → payee → account
type BusinessPayeeRelationRepository interface {
	WithTx(tx client.Tx) BusinessPayeeRelationRepository

	// CRUD operations
	CreateRelation(ctx context.Context, relation *BusinessPayeeRelation) error
	FindRelation(ctx context.Context, businessID, payeeID, accountID string) (*BusinessPayeeRelation, error)
	FindByBusiness(ctx context.Context, businessID string, filters RelationSearchFilters) ([]*BusinessPayeeRelation, error)
	UpdateRelation(ctx context.Context, relation *BusinessPayeeRelation) error
	DeleteRelation(ctx context.Context, businessID, payeeID, accountID string) error

	// Checks
	RelationExists(ctx context.Context, businessID, payeeID, accountID string) (bool, error)

	// Aggregations
	GetPayeeCount(ctx context.Context, businessID string) (int, error)
}

// PayeeBusinessSearchFilters defines search criteria for payee businesses
type PayeeBusinessSearchFilters struct {
	LegalName string // Partial match on legal name
	GSTIN     string // Exact match on GSTIN
	GSTStatus string // Active, Cancelled, Suspended
	Limit     int
	Offset    int
}

// RelationSearchFilters defines search criteria for business-payee relations
type RelationSearchFilters struct {
	DisplayName string   // Partial match on display name
	IsActive    *bool    // Filter by active status
	IsFavorite  *bool    // Filter by favorite status
	Tags        []string // Filter by tags (ANY match)
	Limit       int
	Offset      int
}
