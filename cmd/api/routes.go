package main

import (
	"context"

	"github.com/labstack/echo/v4"
	authRepo "xcentro.in/internal/adapters/database/postgres/auth"
	batchPaymentRepo "xcentro.in/internal/adapters/database/postgres/batch_payment"
	"xcentro.in/internal/adapters/database/postgres/business"
	"xcentro.in/internal/adapters/database/postgres/client"
	featureFlagsRepo "xcentro.in/internal/adapters/database/postgres/feature_flags"
	notificationRepo "xcentro.in/internal/adapters/database/postgres/notification"
	payeeRepo "xcentro.in/internal/adapters/database/postgres/payee"
	paymentRepo "xcentro.in/internal/adapters/database/postgres/payment"
	payoutRepo "xcentro.in/internal/adapters/database/postgres/payout"
	userRepo "xcentro.in/internal/adapters/database/postgres/user"
	voucherRepo "xcentro.in/internal/adapters/database/postgres/voucher"
	"xcentro.in/internal/adapters/external/payment/easebuzz"
	"xcentro.in/internal/adapters/external/payment/jiopay"
	"xcentro.in/internal/adapters/external/payment/mockpg"
	"xcentro.in/internal/adapters/providers/fileupload"
	coreAuth "xcentro.in/internal/core/auth"
	coreBusiness "xcentro.in/internal/core/business"
	"xcentro.in/internal/core/business_rules"
	corePayment "xcentro.in/internal/core/payment"
	coreUser "xcentro.in/internal/core/user"
	coreVoucher "xcentro.in/internal/core/voucher"
	"xcentro.in/internal/handlers/http/auth"
	batchPaymentHandler "xcentro.in/internal/handlers/http/batch_payment"
	businessHandler "xcentro.in/internal/handlers/http/business"
	featureFlagsHandler "xcentro.in/internal/handlers/http/feature_flags"
	notificationHandler "xcentro.in/internal/handlers/http/notification"
	"xcentro.in/internal/handlers/http/payee"
	"xcentro.in/internal/handlers/http/payment"
	payoutHandler "xcentro.in/internal/handlers/http/payout"
	userHandler "xcentro.in/internal/handlers/http/user"
	voucherHandler "xcentro.in/internal/handlers/http/voucher"
	"xcentro.in/internal/middleware"
	authservice "xcentro.in/internal/services/auth"
	batchPaymentSvc "xcentro.in/internal/services/batch_payment"
	businessSvc "xcentro.in/internal/services/business"
	featureFlagsSvc "xcentro.in/internal/services/feature_flags"
	notificationService "xcentro.in/internal/services/notification"
	payeeSvc "xcentro.in/internal/services/payee"
	"xcentro.in/internal/services/payments"
	userSvc "xcentro.in/internal/services/user"
	"xcentro.in/internal/services/verification"
	voucherSvc "xcentro.in/internal/services/voucher"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
)

// Shared service factory to avoid duplication
type serviceFactory struct {
	clients *externalClients
	cfg     *config.Config
}

// feeCalculatorAdapter adapts business_rules.DatabaseFeeCalculator to payment.FeeCalculatorWithTax
type feeCalculatorAdapter struct {
	calc business_rules.DatabaseFeeCalculator
}

// CalculateWithTax adapts the call from payment package to business_rules package
func (a *feeCalculatorAdapter) CalculateWithTax(ctx context.Context, amount int64, method corePayment.PaymentMethod, cardType *corePayment.CardType, cardCategory *corePayment.CardCategory) (*corePayment.FeeCalculationWithTax, error) {
	result, err := a.calc.CalculateWithTax(ctx, amount, method, cardType, cardCategory)
	if err != nil {
		return nil, err
	}

	// Convert business_rules.FeeCalculationWithTax to payment.FeeCalculationWithTax
	return &corePayment.FeeCalculationWithTax{
		BaseAmount:            result.BaseAmount,
		ConvenienceFee:        result.ConvenienceFee,
		ConvenienceFeeGST:     result.ConvenienceFeeGST,
		TotalFee:              result.TotalFee,
		TotalAmount:           result.TotalAmount,
		ConvenienceFeePercent: result.ConvenienceFeePercent,
		GSTPercent:            result.GSTPercent,
	}, nil
}

// createCoreServices creates commonly used repositories and services together
func (f *serviceFactory) createCoreServices(verificationSvc *verification.DocumentVerificationService) (
	userRepository *userRepo.PgUserRepository,
	sessionRepo *authRepo.PgSessionRepository,
	businessRepo *business.PgBusinessRepository,
	businessUserRelRepo *business.PgBusinessUserRelationRepository,
	userService coreUser.UserService,
	busSvc coreBusiness.BusinessService,
	authSvc coreAuth.AuthService,
) {
	// Create repositories
	userRepository = userRepo.NewPgUserRepository(f.clients.db)
	sessionRepo = authRepo.NewPgSessionRepository(f.clients.db)
	businessRepo = business.NewPgBusinessRepository(f.clients.db).(*business.PgBusinessRepository)
	businessUserRelRepo = business.NewPgBusinessUserRelationRepository(f.clients.db).(*business.PgBusinessUserRelationRepository)

	// Create services with proper dependency injection order
	// 1. User service (no circular dependencies)
	userService = userSvc.NewUserService(userRepository, businessRepo, businessUserRelRepo, verificationSvc, f.clients, f.cfg)
	// 2. Business service (depends on user service)
	busSvc = businessSvc.NewBusinessService(businessRepo, businessUserRelRepo, userRepository, userService, f.clients, f.cfg)
	// 3. Auth service (depends on user service and business service)
	authSvc = authservice.NewAuthService(sessionRepo, userRepository, userService, busSvc, f.clients, f.cfg)
	return
}

func registerV1AuthRoutes(clients *externalClients, cfg *config.Config, group *echo.Group) {
	factory := &serviceFactory{clients, cfg}
	verificationSvc, err := verification.NewDocumentVerificationService()
	if err != nil {
		panic("Failed to create verification service: " + err.Error())
	}

	_, _, _, _, _, busSvc, authSvc := factory.createCoreServices(verificationSvc)
	auth.RegisterAuthRoutes(group, authSvc, busSvc, cfg)

	// Feature Flags
	featureFlagsRepository := featureFlagsRepo.NewPgFeatureFlagsRepository(clients.db, clients.rcache)
	featureFlagsService := featureFlagsSvc.NewFeatureFlagsService(featureFlagsRepository)
	featureFlagsHandlerInstance := featureFlagsHandler.NewFeatureFlagsHandler(featureFlagsService)

	authGroup := group.Group("/auth")
	authGroup.GET("/feature-flags", featureFlagsHandlerInstance.GetFeatureFlags)
}

func registerV1PaymentRoutes(clients *externalClients, cfg *config.Config, group *echo.Group) {
	factory := &serviceFactory{clients, cfg}

	paymentRepository := paymentRepo.NewPgPaymentRepository(clients.db, cfg.BankAccountEncryption.EncryptionKey)
	storedCardRepository := paymentRepo.NewPgStoredCardRepository(clients.db)
	bankAccountRepository := paymentRepo.NewPgBankAccountRepository(clients.db, cfg.BankAccountEncryption.EncryptionKey)
	pendingVoucherRepository := paymentRepo.NewPaymentVoucherPendingRepository(clients.db)
	stateChangeRepository := paymentRepo.NewPostgresPaymentStateChangeRepository(clients.db)

	_, _, _, businessUserRelRepo, _, busSvc, authSvc := factory.createCoreServices(nil)

	// Initialize payment gateway clients
	easebuzzClient := easebuzz.NewEasebuzzClient(cfg.Easebuzz.BaseURL, cfg.Easebuzz.MerchantKey, cfg.Easebuzz.Salt, cfg.Easebuzz.APIKey, cfg.Easebuzz.IsAggregatorModel)
	jioPayClient := jiopay.NewJioPayClient(cfg.JioPay.BaseURL, cfg.JioPay.TokenizationBaseURL, cfg.JioPay.MerchantID, cfg.JioPay.APIKey, cfg.JioPay.AppID, cfg.JioPay.EncryptionKey, cfg.JioPay.IsAggregatorModel, busSvc)
	mockPGClient := mockpg.NewMockPGClient(cfg.MockPG.MerchantID, cfg.MockPG.APIKey, cfg.MockPG.SimulateSuccess, cfg.MockPG.IsAggregatorModel)

	// Initialize fee calculator
	feeRulesRepository := paymentRepo.NewPostgresFeeRulesRepository(clients.db)
	databaseFeeCalculator := business_rules.NewDatabaseFeeCalculator(feeRulesRepository)

	// Adapter to convert business_rules.DatabaseFeeCalculator to payment.FeeCalculatorWithTax
	feeCalculatorAdapter := &feeCalculatorAdapter{calc: databaseFeeCalculator}

	// Create payment orchestrator with all gateway clients and fee calculator
	orchestrator := corePayment.NewPaymentOrchestrator(paymentRepository, storedCardRepository, jioPayClient, easebuzzClient, mockPGClient, cfg.MockPG.Enabled, feeCalculatorAdapter, logger.GetDefault())
	feeCalculator := business_rules.NewFeeCalculator()
	walletService := business_rules.NewWalletService(storedCardRepository, bankAccountRepository, clients.txManager, busSvc, logger.GetDefault())
	paymentSourceService := payments.NewPaymentSourceService(walletService, databaseFeeCalculator)

	voucherRepository := voucherRepo.NewPostgresVoucherRepository(clients.db, cfg.BankAccountEncryption.EncryptionKey)
	queueRepository := voucherRepo.NewPostgresERPSyncQueueRepository(clients.db)
	voucherCoreService := coreVoucher.NewVoucherService(voucherRepository)
	businessPayeeRelationRepo := payeeRepo.NewPgBusinessPayeeRelationRepository(clients.db, cfg.BankAccountEncryption.EncryptionKey)
	notificationEventRepo := notificationRepo.NewPgNotificationEventRepository(clients.db)
	voucherService := voucherSvc.NewVoucherService(voucherRepository, queueRepository, voucherCoreService, businessPayeeRelationRepo, notificationEventRepo, logger.GetDefault())

	paymentService := payments.NewPaymentService(orchestrator, paymentRepository, storedCardRepository, bankAccountRepository, walletService, feeCalculator, clients.txManager, voucherService, voucherRepository, pendingVoucherRepository, busSvc, stateChangeRepository, cfg, logger.GetDefault())

	processPaymentGroup := group.Group("/process_payment")

	// Register public routes FIRST (no JWT required) - these are called by external systems
	// 1. Webhooks (called by payment gateways)
	webhookHandler := payment.NewWebhookHandler(paymentRepository, jioPayClient, easebuzzClient, mockPGClient, cfg.MockPG.Enabled, stateChangeRepository)
	webhookGroup := processPaymentGroup.Group("/webhooks")
	webhookGroup.POST("/payment_status", webhookHandler.HandlePaymentStatusWebhook)
	webhookGroup.POST("/settlement_status", webhookHandler.HandleSettlementStatusWebhook)

	// 2. Callbacks (redirects from payment gateways after payment completion)
	callbackHandler := payment.NewCallbackHandler(paymentRepository, storedCardRepository, walletService, jioPayClient, easebuzzClient, mockPGClient, cfg.MockPG.Enabled, pendingVoucherRepository, voucherRepository, clients.txManager, stateChangeRepository)
	processPaymentGroup.POST("/callback", callbackHandler.HandleCallback)

	// 3. Mock 3DS page (simulates bank OTP/3DS page for testing)
	mock3DSHandler := payment.NewMock3DSHandler(paymentRepository, cfg.Server.URL)
	processPaymentGroup.GET("/mock_3ds", mock3DSHandler.HandleMock3DSPage)

	// 4. Payment callback page (user-facing HTML redirect pages)
	paymentCallbackHandler := payment.NewPaymentCallbackHandler(paymentRepository)
	processPaymentGroup.GET("/callback_page", paymentCallbackHandler.HandlePaymentCallback)

	// Register JWT-protected routes (authenticated user operations)
	payment.RegisterPaymentRoutes(processPaymentGroup, paymentService, paymentSourceService, walletService, authSvc, voucherRepository, stateChangeRepository, businessPayeeRelationRepo, storedCardRepository, bankAccountRepository, logger.GetDefault())

	walletGroup := group.Group("/wallet")
	payment.RegisterWalletRoutes(walletGroup, paymentService, paymentSourceService, walletService, authSvc, storedCardRepository, businessUserRelRepo, clients.rcache, logger.GetDefault())
}

func registerV1BusinessRoutes(clients *externalClients, cfg *config.Config, group *echo.Group) {
	factory := &serviceFactory{clients, cfg}
	verificationSvc, err := verification.NewDocumentVerificationService()
	if err != nil {
		panic("Failed to create verification service: " + err.Error())
	}

	_, _, _, businessUserRelRepo, userService, busSvc, authSvc := factory.createCoreServices(verificationSvc)
	businessHandler.RegisterBusinessRoutes(group, busSvc, authSvc, userService, &cfg.S3, businessUserRelRepo, clients.rcache)
}

func registerV1UserRoutes(clients *externalClients, cfg *config.Config, group *echo.Group) {
	factory := &serviceFactory{clients, cfg}
	verificationSvc, err := verification.NewDocumentVerificationService()
	if err != nil {
		panic("Failed to create verification service: " + err.Error())
	}

	_, _, _, businessUserRelRepo, userService, _, authSvc := factory.createCoreServices(verificationSvc)
	userHandler.RegisterUserRoutes(group, userService, authSvc, businessUserRelRepo, clients.rcache, cfg)
}

func registerV1PayeeRoutes(clients *externalClients, cfg *config.Config, group *echo.Group) {
	factory := &serviceFactory{clients, cfg}
	_, _, _, _, _, busSvc, authSvc := factory.createCoreServices(nil)

	payeeFactory := payeeSvc.NewPayeeServiceFactory(clients.db, clients.txManager, busSvc, cfg)
	payeeService := payeeFactory.NewPayeeService()

	payeeGroup := group.Group("/payees")
	payee.RegisterPayeeRoutes(payeeGroup, payeeService, authSvc)
	payee.RegisterVendorDropdownRoute(group, payeeService, authSvc)
	payee.RegisterEmployeeListRoute(group, payeeService, authSvc)
}

func registerV1VoucherRoutes(clients *externalClients, cfg *config.Config, group *echo.Group) {
	factory := &serviceFactory{clients, cfg}
	_, _, _, businessUserRelRepo, _, _, authSvc := factory.createCoreServices(nil)

	voucherRepository := voucherRepo.NewPostgresVoucherRepository(clients.db, cfg.BankAccountEncryption.EncryptionKey)
	queueRepository := voucherRepo.NewPostgresERPSyncQueueRepository(clients.db)
	voucherCoreService := coreVoucher.NewVoucherService(voucherRepository)
	businessPayeeRelationRepo := payeeRepo.NewPgBusinessPayeeRelationRepository(clients.db, cfg.BankAccountEncryption.EncryptionKey)
	notificationEventRepo := notificationRepo.NewPgNotificationEventRepository(clients.db)
	voucherService := voucherSvc.NewVoucherService(voucherRepository, queueRepository, voucherCoreService, businessPayeeRelationRepo, notificationEventRepo, logger.GetDefault())

	voucherGroup := group.Group("/vouchers")
	voucherHandler.RegisterVoucherRoutes(voucherGroup, voucherService, authSvc, businessUserRelRepo, clients.rcache)
}

func registerV1PayoutRoutes(clients *externalClients, cfg *config.Config, group *echo.Group) {
	transferRepository := payoutRepo.NewPostgresPayoutTransferRepository(clients.db)
	stateChangeRepository := paymentRepo.NewPostgresPaymentStateChangeRepository(clients.db)

	webhookHandler := payoutHandler.NewPayoutWebhookHandler(transferRepository, stateChangeRepository, cfg.Castler.WebhookSecret)

	// Payout routes - separated by purpose
	payoutGroup := group.Group("/payout")

	// Webhook endpoints (external - called by Castler)
	payoutGroup.POST("/webhooks/castler", webhookHandler.HandleCastlerWebhook)

	// NOTE: Dashboard/Operations endpoints have been moved to cmd/ops (ops dashboard service)
	// See cmd/ops/routes.go -> registerOpsPayoutRoutes() for all payout operations endpoints
}

func registerV1BatchPaymentRoutes(clients *externalClients, cfg *config.Config, group *echo.Group) {
	factory := &serviceFactory{clients, cfg}
	_, _, _, _, _, _, authSvc := factory.createCoreServices(nil)

	// Repositories
	batchPaymentRepository := batchPaymentRepo.NewPgBatchPaymentRepository(clients.db)
	businessPayeeRelationRepo := payeeRepo.NewPgBusinessPayeeRelationRepository(clients.db, cfg.BankAccountEncryption.EncryptionKey)

	// Voucher service dependencies
	voucherRepository := voucherRepo.NewPostgresVoucherRepository(clients.db, cfg.BankAccountEncryption.EncryptionKey)
	queueRepository := voucherRepo.NewPostgresERPSyncQueueRepository(clients.db)
	voucherCoreService := coreVoucher.NewVoucherService(voucherRepository)
	notificationEventRepo := notificationRepo.NewPgNotificationEventRepository(clients.db)
	voucherService := voucherSvc.NewVoucherService(voucherRepository, queueRepository, voucherCoreService, businessPayeeRelationRepo, notificationEventRepo, logger.GetDefault())

	// Batch payment service
	batchPaymentService := batchPaymentSvc.NewBatchPaymentService(
		batchPaymentRepository,
		businessPayeeRelationRepo,
		voucherService,
	)

	// S3 client for sample CSV downloads
	s3Client, err := fileupload.NewS3ClientFromConfig(cfg.S3)
	if err != nil {
		logger.GetDefault().Warnf("Failed to create S3 client for batch payments: %v (sample CSV download will be unavailable)", err)
	}

	// Handler
	handler := batchPaymentHandler.NewBatchPaymentHandler(batchPaymentService, s3Client)

	// Routes
	batchGroup := group.Group("/batch_payments")
	batchGroup.Use(middleware.JWTMiddleware(authSvc))

	batchGroup.POST("/upload", handler.UploadCSV)
	batchGroup.GET("", handler.GetBatchList)
	batchGroup.GET("/:batch_id", handler.GetBatchStatus)
	batchGroup.POST("/:batch_id/process", handler.ProcessBatch)
	batchGroup.GET("/sample_csv", handler.DownloadSampleCSV) // Download sample CSV template
}

func registerV1NotificationRoutes(clients *externalClients, cfg *config.Config, group *echo.Group) {
	factory := &serviceFactory{clients, cfg}

	userNotifRepo := notificationRepo.NewPgUserNotificationRepository(clients.db)
	prefsRepo := notificationRepo.NewPgNotificationPreferencesRepository(clients.db)
	eventRepo := notificationRepo.NewPgNotificationEventRepository(clients.db)
	notifService := notificationService.NewUserFacingNotificationService(eventRepo, userNotifRepo, prefsRepo, clients.txManager, clients.rcache.Client, logger.GetDefault())

	_, _, _, _, _, _, authSvc := factory.createCoreServices(nil)
	notificationHandler.RegisterNotificationRoutes(group, notifService, authSvc)
}

func registerV1Routes(s *xcentroServer) {
	v1 := s.echo.Group("/v1")

	v1.GET("/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{"status": "ok"})
	})

	clients := &s.meta.clients
	cfg := &s.meta.config

	registerV1AuthRoutes(clients, cfg, v1)
	registerV1PaymentRoutes(clients, cfg, v1)
	registerV1UserRoutes(clients, cfg, v1)
	registerV1BusinessRoutes(clients, cfg, v1)
	registerV1PayeeRoutes(clients, cfg, v1)
	registerV1VoucherRoutes(clients, cfg, v1)
	registerV1PayoutRoutes(clients, cfg, v1)
	registerV1BatchPaymentRoutes(clients, cfg, v1)
	registerV1NotificationRoutes(clients, cfg, v1)
}

func initializeNotificationTrigger(db client.DB) {
	eventRepo := notificationRepo.NewPgNotificationEventRepository(db)
	notificationService.InitializeNotificationTrigger(eventRepo)
	logger.GetDefault().Info("Notification trigger initialized")
}

func registerRouters(s *xcentroServer) {
	initializeNotificationTrigger(s.meta.clients.db)
	registerV1Routes(s)
}
