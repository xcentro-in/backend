package payee

import (
	"xcentro.in/internal/adapters/database/postgres/client"
	payeeRepo "xcentro.in/internal/adapters/database/postgres/payee"
	"xcentro.in/internal/core/business_rules"
)

// PayeeServiceFactory creates PayeeService with all dependencies
// This pattern keeps routes.go clean and centralizes dependency management
type PayeeServiceFactory struct {
	db        client.DB
	txManager client.TxManager
}

// NewPayeeServiceFactory creates a new factory for PayeeService
func NewPayeeServiceFactory(db client.DB, txManager client.TxManager) *PayeeServiceFactory {
	return &PayeeServiceFactory{
		db:        db,
		txManager: txManager,
	}
}

// NewPayeeService creates a fully configured PayeeService with all repositories and services
func (f *PayeeServiceFactory) NewPayeeService() *PayeeService {
	// Create all payee repositories
	payeeMasterRepo := payeeRepo.NewPgPayeeMasterRepository(f.db)
	payeeVendorRepo := payeeRepo.NewPgPayeeVendorRepository(f.db)
	payeeGSTRepo := payeeRepo.NewPgPayeeGSTRepository(f.db)
	payeeGSTVirtualAccountRepo := payeeRepo.NewPgPayeeGSTVirtualAccountRepository(f.db)
	payeeBillerRepo := payeeRepo.NewPgPayeeBillerRepository(f.db)
	payeeBillerAccountRepo := payeeRepo.NewPgPayeeBillerAccountRepository(f.db)
	payeeEmployeeRepo := payeeRepo.NewPgPayeeEmployeeRepository(f.db)
	payeeBankAccountRepo := payeeRepo.NewPgPayeeBankAccountRepository(f.db)
	businessPayeeRelationRepo := payeeRepo.NewPgBusinessPayeeRelationRepository(f.db)

	// Create verification services (mock implementations for now)
	gstVerificationSvc := business_rules.NewMockGSTVerificationService()
	bankVerificationSvc := business_rules.NewMockBankVerificationService()
	crossVerificationSvc := business_rules.NewMockCrossVerificationService()

	// Create and return PayeeService with all dependencies
	return NewPayeeService(
		payeeMasterRepo,
		payeeVendorRepo,
		payeeGSTRepo,
		payeeGSTVirtualAccountRepo,
		payeeBillerRepo,
		payeeBillerAccountRepo,
		payeeEmployeeRepo,
		payeeBankAccountRepo,
		businessPayeeRelationRepo,
		gstVerificationSvc,
		bankVerificationSvc,
		crossVerificationSvc,
		f.txManager,
	)
}
