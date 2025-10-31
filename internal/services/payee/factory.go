package payee

import (
	"xcentro.in/internal/adapters/database/postgres/client"
	payeeRepo "xcentro.in/internal/adapters/database/postgres/payee"
	"xcentro.in/internal/adapters/external/payout/castler"
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

	// Create payout provider client (Castler with encryption support)
	castlerClient, err := castler.NewCastlerClient(
		f.config.Castler.BaseURL,
		f.config.Castler.APIKey,
		f.config.Castler.SecretKey,
		f.config.Castler.EncryptionKey,
		f.config.Castler.EncryptionIV,
	)
	if err != nil {
		panic("Failed to create Castler client: " + err.Error())
	}

	// Create document verification service (AuthBridge - for GST, PAN verification)
	docVerificationSvc, err := verification.NewDocumentVerificationService()
	if err != nil {
		panic("Failed to create document verification service: " + err.Error())
	}

	// Create verification services
	bankVerificationSvc := castler.NewCastlerBankVerificationService(castlerClient) // Use Castler for penny-drop
	crossVerificationSvc := business_rules.NewCrossVerificationService()

	// Get logger instance
	log := logger.GetDefault()

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
		castlerClient,       // payout provider (for transfers)
		castlerClient,       // castler client (for payee onboarding)
		docVerificationSvc,  // document verification (AuthBridge for GST/PAN)
		bankVerificationSvc,
		crossVerificationSvc,
		f.txManager,
		f.businessService,
		log,                 // logger instance
	)
}
