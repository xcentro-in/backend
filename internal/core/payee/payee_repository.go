package payee

import (
	"context"

	"xcentro.in/internal/adapters/database/postgres/client"
)

// ############################################################################
// # Payee Master Repository
// ############################################################################

// PayeeMasterRepository manages the master routing table for all payees
type PayeeMasterRepository interface {
	WithTx(tx client.Tx) PayeeMasterRepository

	// CRUD operations
	Create(ctx context.Context, master *PayeeMaster) error
	FindByID(ctx context.Context, payeeID string) (*PayeeMaster, error)
	Update(ctx context.Context, master *PayeeMaster) error
	Delete(ctx context.Context, payeeID string) error

	// Query operations
	FindByType(ctx context.Context, payeeType PayeeType, limit, offset int) ([]*PayeeMaster, error)
	CountByType(ctx context.Context, payeeType PayeeType) (int, error)
}

// ############################################################################
// # Vendor Payee Repository
// ############################################################################

// PayeeVendorRepository manages vendor payee entities (businesses with GSTIN)
type PayeeVendorRepository interface {
	WithTx(tx client.Tx) PayeeVendorRepository

	// CRUD operations
	Create(ctx context.Context, vendor *PayeeVendor) error
	FindByID(ctx context.Context, payeeID string) (*PayeeVendor, error)
	FindByGSTIN(ctx context.Context, gstin string) (*PayeeVendor, error)
	Update(ctx context.Context, vendor *PayeeVendor) error

	// Search operations
	Search(ctx context.Context, filters VendorSearchFilters) ([]*PayeeVendor, error)
}

// ############################################################################
// # GST Payee Repository
// ############################################################################

// PayeeGSTRepository manages GST payment payee (government singleton)
type PayeeGSTRepository interface {
	WithTx(tx client.Tx) PayeeGSTRepository

	// Singleton operations
	GetGovernmentPayee(ctx context.Context) (*PayeeGST, error)
	Update(ctx context.Context, gst *PayeeGST) error
}

// PayeeGSTVirtualAccountRepository manages GST virtual bank accounts
type PayeeGSTVirtualAccountRepository interface {
	WithTx(tx client.Tx) PayeeGSTVirtualAccountRepository

	// CRUD operations
	Create(ctx context.Context, account *PayeeGSTVirtualAccount) error
	FindByID(ctx context.Context, virtualAccountID string) (*PayeeGSTVirtualAccount, error)
	FindByVirtualAccountNumber(ctx context.Context, virtualAccountNumber string) (*PayeeGSTVirtualAccount, error)
	Update(ctx context.Context, account *PayeeGSTVirtualAccount) error

	// Query operations
	FindByPayeeID(ctx context.Context, payeeID string) ([]*PayeeGSTVirtualAccount, error)
	FindByTaxPeriod(ctx context.Context, payeeID, taxPeriod string) (*PayeeGSTVirtualAccount, error)
	FindByFinancialYear(ctx context.Context, payeeID, financialYear string) ([]*PayeeGSTVirtualAccount, error)
	FindActiveAccounts(ctx context.Context, payeeID string) ([]*PayeeGSTVirtualAccount, error)

	// Analytics
	CountByFinancialYear(ctx context.Context, financialYear string) (int, error)
}

// ############################################################################
// # Biller Payee Repository
// ############################################################################

// PayeeBillerRepository manages biller payee entities (BBPS system)
type PayeeBillerRepository interface {
	WithTx(tx client.Tx) PayeeBillerRepository

	// CRUD operations
	Create(ctx context.Context, biller *PayeeBiller) error
	FindByID(ctx context.Context, payeeID string) (*PayeeBiller, error)
	FindByBillerID(ctx context.Context, billerID string) (*PayeeBiller, error)
	Update(ctx context.Context, biller *PayeeBiller) error

	// Search operations
	Search(ctx context.Context, filters BillerSearchFilters) ([]*PayeeBiller, error)
	FindByCategory(ctx context.Context, category string) ([]*PayeeBiller, error)
}

// PayeeBillerAccountRepository manages customer-biller configurations
type PayeeBillerAccountRepository interface {
	WithTx(tx client.Tx) PayeeBillerAccountRepository

	// CRUD operations
	Create(ctx context.Context, account *PayeeBillerAccount) error
	FindByID(ctx context.Context, billerAccountID string) (*PayeeBillerAccount, error)
	Update(ctx context.Context, account *PayeeBillerAccount) error
	Delete(ctx context.Context, billerAccountID string) error

	// Query operations
	FindByBusinessAndPayee(ctx context.Context, businessID, payeeID string) ([]*PayeeBillerAccount, error)
	FindByBusiness(ctx context.Context, businessID string) ([]*PayeeBillerAccount, error)
	CheckDuplicate(ctx context.Context, businessID, payeeID, mobileNumber string) (bool, error)

	// Auto-fetch operations
	FindAccountsForAutoFetch(ctx context.Context) ([]*PayeeBillerAccount, error)
}

// ############################################################################
// # Employee Payee Repository
// ############################################################################

// PayeeEmployeeRepository manages employee payee entities (payroll)
type PayeeEmployeeRepository interface {
	WithTx(tx client.Tx) PayeeEmployeeRepository

	// CRUD operations
	Create(ctx context.Context, employee *PayeeEmployee) error
	FindByID(ctx context.Context, payeeID string) (*PayeeEmployee, error)
	Update(ctx context.Context, employee *PayeeEmployee) error

	// Document management
	UpdateRemunerationDocumentKey(ctx context.Context, payeeID, s3Key string) error

	// Search operations
	Search(ctx context.Context, filters EmployeeSearchFilters) ([]*PayeeEmployee, error)
	FindByBusiness(ctx context.Context, businessID string, filters EmployeeSearchFilters) ([]*PayeeEmployee, error)
}

// ############################################################################
// # Bank Account Repository (Shared by Vendor + Employee)
// ############################################################################

// PayeeBankAccountRepository manages bank accounts for payees
// Used by Vendor and Employee payee types (NOT by GST or Biller)
// Bank accounts are globally shared - multiple payees can reference the same account
type PayeeBankAccountRepository interface {
	WithTx(tx client.Tx) PayeeBankAccountRepository

	// CRUD operations
	CreateBankAccount(ctx context.Context, account *PayeeBankAccount) error
	FindByID(ctx context.Context, accountID string) (*PayeeBankAccount, error)
	UpdateBankAccount(ctx context.Context, account *PayeeBankAccount) error

	// Global account lookup (for reusing Castler beneficiary IDs)
	FindByAccountHash(ctx context.Context, accountHash string) (*PayeeBankAccount, error)
}

// ############################################################################
// # Business-Payee Relation Repository (Polymorphic)
// ############################################################################

// BusinessPayeeRelationRepository manages customer-payee-account relationships
// Handles polymorphic many-to-many: business → payee → account (bank/virtual/biller)
type BusinessPayeeRelationRepository interface {
	WithTx(tx client.Tx) BusinessPayeeRelationRepository

	// CRUD operations
	CreateRelation(ctx context.Context, relation *BusinessPayeeRelation) error
	FindRelationByID(ctx context.Context, relationID string) (*BusinessPayeeRelation, error) // New: Lookup by surrogate key
	FindRelation(ctx context.Context, businessID, payeeID, accountID string) (*BusinessPayeeRelation, error)
	FindByBusiness(ctx context.Context, businessID string, filters RelationSearchFilters) ([]*BusinessPayeeRelation, error)
	UpdateRelation(ctx context.Context, relation *BusinessPayeeRelation) error
	DeleteRelation(ctx context.Context, businessID, payeeID, accountID string) error

	// Checks
	RelationExists(ctx context.Context, businessID, payeeID, accountID string) (bool, error)

	// Aggregations
	GetPayeeCount(ctx context.Context, businessID string) (int, error)
	GetPayeeCountByType(ctx context.Context, businessID string, payeeType PayeeType) (int, error)

	// Vendor Dropdown (lightweight query for voucher creation)
	GetVendorsForDropdown(ctx context.Context, businessID string) ([]*VendorDropdownItem, error)

	// Employee lightweight list (for tabulated display)
	GetEmployeesForList(ctx context.Context, businessID string) ([]*EmployeeLightweight, error)

	// Validation helper (used by voucher service)
	ValidateRelationForBusiness(ctx context.Context, businessID, relationID string) (payeeID, accountID string, err error)

	// Get full payee details by relation_id (polymorphic - vendor/employee/biller)
	// Validates that relation belongs to the specified business
	GetPayeeFullDetailsByRelation(ctx context.Context, businessID, relationID string) (*PayeeFullDetails, error)
}

// ############################################################################
// # Search Filter Structs
// ############################################################################

// VendorSearchFilters defines search criteria for vendor payees
type VendorSearchFilters struct {
	LegalName string // Partial match on legal name
	GSTIN     string // Exact match on GSTIN
	GSTStatus string // Active, Cancelled, Suspended
	Limit     int
	Offset    int
}

// BillerSearchFilters defines search criteria for biller payees
type BillerSearchFilters struct {
	BillerName     string // Partial match on biller name
	BillerCategory string // Exact match on category
	BillerID       string // Exact match on biller ID
	Limit          int
	Offset         int
}

// EmployeeSearchFilters defines search criteria for employee payees
type EmployeeSearchFilters struct {
	FullName       string // Partial match on full name
	Department     string // Exact match on department
	EmploymentType string // Exact match on employment type
	Limit          int
	Offset         int
}

// RelationSearchFilters defines search criteria for business-payee relations
type RelationSearchFilters struct {
	DisplayName string    // Partial match on display name
	PayeeType   PayeeType // Filter by payee type
	IsActive    *bool     // Filter by active status
	IsFavorite  *bool     // Filter by favorite status
	Tags        []string  // Filter by tags (ANY match)
	Limit       int
	Offset      int
}
