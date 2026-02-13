package payee

import (
	"xcentro.in/internal/adapters/database/postgres/client"
	payeeRepo "xcentro.in/internal/adapters/database/postgres/payee"
	"xcentro.in/internal/adapters/external/payout/castler"
	payoutFactory "xcentro.in/internal/adapters/external/payout/factory"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/business_rules"
	"xcentro.in/internal/core/payout"
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

	// Create payout provider factory (supports mock mode via CASTLER_MOCK_ENABLED)
	providerFactory := payoutFactory.NewPayoutProviderFactory(f.config)

	// Get payout provider (returns mock or real based on config)
	payoutProvider, err := providerFactory.GetProvider(payout.ProviderCastler)
	if err != nil {
		panic("Failed to get payout provider: " + err.Error())
	}

	// Assert that the payout provider also implements CastlerPayeeOnboarder interface
	// Both real and mock clients implement this interface
	castlerClient, ok := payoutProvider.(CastlerPayeeOnboarder)
	if !ok {
		panic("Payout provider does not implement CastlerPayeeOnboarder interface")
	}

	// Create document verification service (AuthBridge - for GST, PAN verification)
	docVerificationSvc, err := verification.NewDocumentVerificationService()
	if err != nil {
		panic("Failed to create document verification service: " + err.Error())
	}

	// Create bank verification service
	// If mock mode is enabled, use mock bank verification; otherwise use Castler
	var bankVerificationSvc business_rules.BankVerificationService
	if f.config.Castler.MockEnabled {
		logger.Info("🎭 Bank Verification Mock Mode ENABLED - Using mock service")
		bankVerificationSvc = business_rules.NewMockBankVerificationService()
	} else {
		// For real Castler client, we need to assert it's a *castler.CastlerClient
		realCastlerClient, ok := payoutProvider.(*castler.CastlerClient)
		if !ok {
			panic("Real payout provider is not a *castler.CastlerClient")
		}
		bankVerificationSvc = castler.NewCastlerBankVerificationService(realCastlerClient)
	}

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
		payoutProvider,              // payout provider (mock or real based on config)
		castlerClient,               // castler client (mock or real based on config)
		docVerificationSvc,          // document verification (AuthBridge for GST/PAN)
		bankVerificationSvc,         // bank verification (mock or real based on config)
		crossVerificationSvc,
		f.txManager,
		f.businessService,
		&f.config.S3,                // S3 configuration for document uploads
		&f.config.PayeeValidation,   // payee validation configuration (enable/disable strict checks)
		log,                         // logger instance
	)
}
