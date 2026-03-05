package payee

import (
	"xcentro.in/internal/adapters/database/postgres/client"
	payeeRepo "xcentro.in/internal/adapters/database/postgres/payee"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/business_rules"
	"xcentro.in/internal/services/verification"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
)

// PayeeServiceFactory creates PayeeService with all dependencies
// This pattern keeps routes.go clean and centralizes dependency management
type PayeeServiceFactory struct {
	db              client.DB
	txManager       client.TxManager
	businessService business.BusinessService
	config          *config.Config
}

// NewPayeeServiceFactory creates a new factory for PayeeService
func NewPayeeServiceFactory(db client.DB, txManager client.TxManager, businessService business.BusinessService, cfg *config.Config) *PayeeServiceFactory {
	return &PayeeServiceFactory{
		db:              db,
		txManager:       txManager,
		businessService: businessService,
		config:          cfg,
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
	payeeBankAccountRepo := payeeRepo.NewPgPayeeBankAccountRepository(f.db, f.config.BankAccountEncryption.EncryptionKey)
	businessPayeeRelationRepo := payeeRepo.NewPgBusinessPayeeRelationRepository(f.db, f.config.BankAccountEncryption.EncryptionKey)

	// Create document verification service (AuthBridge - for GST, PAN, bank verification)
	docVerificationSvc, err := verification.NewDocumentVerificationService()
	if err != nil {
		panic("Failed to create document verification service: " + err.Error())
	}

	// Bank verification service: use mock until Authbridge bank verification is wired
	// TODO: replace with Authbridge bank verification service implementation
	logger.Info("Bank verification: using mock service (pending Authbridge integration)")
	bankVerificationSvc := business_rules.NewMockBankVerificationService()

	crossVerificationSvc := business_rules.NewCrossVerificationService()

	log := logger.GetDefault()

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
		docVerificationSvc,
		bankVerificationSvc,
		crossVerificationSvc,
		f.txManager,
		f.businessService,
		&f.config.S3,
		&f.config.PayeeValidation,
		log,
	)
}
