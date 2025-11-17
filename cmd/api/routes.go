package main

import (
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
	"xcentro.in/internal/adapters/external/payout/factory"
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
	payoutSvc "xcentro.in/internal/services/payout"
	settlementSvc "xcentro.in/internal/services/settlement"
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

	_, _, _, _, _, busSvc, authSvc := factory.createCoreServices(nil)

	// Initialize payment gateway clients
	easebuzzClient := easebuzz.NewEasebuzzClient(cfg.Easebuzz.BaseURL, cfg.Easebuzz.MerchantKey, cfg.Easebuzz.Salt, cfg.Easebuzz.APIKey)
	jioPayClient := jiopay.NewJioPayClient(cfg.JioPay.BaseURL, cfg.JioPay.TokenizationBaseURL, cfg.JioPay.MerchantID, cfg.JioPay.APIKey, cfg.JioPay.AppID, cfg.JioPay.EncryptionKey, busSvc)
	mockPGClient := mockpg.NewMockPGClient(cfg.MockPG.MerchantID, cfg.MockPG.APIKey, cfg.MockPG.SimulateSuccess)

	// Create payment orchestrator with all gateway clients
	orchestrator := corePayment.NewPaymentOrchestrator(paymentRepository, storedCardRepository, jioPayClient, easebuzzClient, mockPGClient, cfg.MockPG.Enabled, logger.GetDefault())
	feeCalculator := business_rules.NewFeeCalculator()
	walletService := business_rules.NewWalletService(storedCardRepository, bankAccountRepository, clients.txManager, busSvc, logger.GetDefault())

	feeRulesRepository := paymentRepo.NewPostgresFeeRulesRepository(clients.db)
	databaseFeeCalculator := business_rules.NewDatabaseFeeCalculator(feeRulesRepository)
	paymentSourceService := payments.NewPaymentSourceService(walletService, databaseFeeCalculator)

	voucherRepository := voucherRepo.NewPostgresVoucherRepository(clients.db, cfg.BankAccountEncryption.EncryptionKey)
	queueRepository := voucherRepo.NewPostgresERPSyncQueueRepository(clients.db)
	voucherCoreService := coreVoucher.NewVoucherService(voucherRepository)
	businessPayeeRelationRepo := payeeRepo.NewPgBusinessPayeeRelationRepository(clients.db, cfg.BankAccountEncryption.EncryptionKey)
	notificationEventRepo := notificationRepo.NewPgNotificationEventRepository(clients.db)
	voucherService := voucherSvc.NewVoucherService(voucherRepository, queueRepository, voucherCoreService, businessPayeeRelationRepo, notificationEventRepo, logger.GetDefault())

	paymentService := payments.NewPaymentService(orchestrator, paymentRepository, storedCardRepository, bankAccountRepository, walletService, feeCalculator, clients.txManager, voucherService, voucherRepository, pendingVoucherRepository, busSvc, cfg, logger.GetDefault())

	processPaymentGroup := group.Group("/process_payment")
	payment.RegisterPaymentRoutes(processPaymentGroup, paymentService, paymentSourceService, authSvc, voucherRepository, stateChangeRepository, businessPayeeRelationRepo, storedCardRepository, bankAccountRepository, logger.GetDefault())

	walletGroup := group.Group("/wallet")
	payment.RegisterWalletRoutes(walletGroup, paymentService, paymentSourceService, authSvc, logger.GetDefault())

	// Mock 3DS page handler (for mock PG testing)
	mock3DSHandler := payment.NewMock3DSHandler(paymentRepository, cfg.Server.URL)
	processPaymentGroup.GET("/mock_3ds", mock3DSHandler.HandleMock3DSPage)

	callbackHandler := payment.NewCallbackHandler(paymentRepository, storedCardRepository, jioPayClient, easebuzzClient, mockPGClient, cfg.MockPG.Enabled, pendingVoucherRepository, voucherRepository, clients.txManager, stateChangeRepository)
	processPaymentGroup.POST("/callback", callbackHandler.HandleCallback)

	webhookHandler := payment.NewWebhookHandler(paymentRepository, jioPayClient, easebuzzClient, mockPGClient, cfg.MockPG.Enabled)
	webhookGroup := processPaymentGroup.Group("/webhooks")
	webhookGroup.POST("/payment_status", webhookHandler.HandlePaymentStatusWebhook)
	webhookGroup.POST("/settlement_status", webhookHandler.HandleSettlementStatusWebhook)

	// Payment callback page handler (user-facing HTML redirect pages)
	paymentCallbackHandler := payment.NewPaymentCallbackHandler(paymentRepository)
	processPaymentGroup.GET("/callback_page", paymentCallbackHandler.HandlePaymentCallback)
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
	configRepository := payoutRepo.NewPostgresPayoutConfigurationRepository(clients.db)
	voucherRepository := voucherRepo.NewPostgresVoucherRepository(clients.db, cfg.BankAccountEncryption.EncryptionKey)
	bankAccountRepository := payeeRepo.NewPgPayeeBankAccountRepository(clients.db, cfg.BankAccountEncryption.EncryptionKey)
	stateChangeRepository := paymentRepo.NewPostgresPaymentStateChangeRepository(clients.db)

	providerFactory := factory.NewPayoutProviderFactory(cfg)
	payoutService := payoutSvc.NewPayoutService(voucherRepository, transferRepository, bankAccountRepository, configRepository, providerFactory)
	settlementService := settlementSvc.NewSettlementVerificationService(logger.GetDefault())

	webhookHandler := payoutHandler.NewPayoutWebhookHandler(transferRepository, stateChangeRepository, cfg.Castler.WebhookSecret)
	payoutHandler := payoutHandler.NewPayoutHandler(payoutService, transferRepository, configRepository, voucherRepository, settlementService, providerFactory, stateChangeRepository, logger.GetDefault())

	// Payout routes - separated by purpose
	payoutGroup := group.Group("/payout")

	// Webhook endpoints (external - called by Castler)
	payoutGroup.POST("/webhooks/castler", webhookHandler.HandleCastlerWebhook)

	// Dashboard/Operations endpoints (internal - admin/ops team)
	dashboardGroup := payoutGroup.Group("/dashboard")
	dashboardGroup.POST("/initiate", payoutHandler.InitiatePayoutManually)
	dashboardGroup.GET("/pending-approvals", payoutHandler.GetPendingApprovals) // DEPRECATED
	dashboardGroup.POST("/:transfer_id/approve", payoutHandler.ApproveTransfer) // DEPRECATED
	dashboardGroup.POST("/:transfer_id/cancel", payoutHandler.CancelTransfer)
	dashboardGroup.GET("/:transfer_id", payoutHandler.GetTransferDetails)
	dashboardGroup.GET("/:transfer_id/webhooks", payoutHandler.GetWebhookHistory)
	dashboardGroup.GET("/timeouts", payoutHandler.GetTimeoutTransfers)
	dashboardGroup.POST("/:transfer_id/mark-completed", payoutHandler.MarkCompletedManually)
	dashboardGroup.GET("/config", payoutHandler.GetConfig)
	dashboardGroup.PUT("/config", payoutHandler.UpdateConfig)
	dashboardGroup.POST("/config", payoutHandler.CreateConfig)
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

	// Handler
	handler := batchPaymentHandler.NewBatchPaymentHandler(batchPaymentService)

	// Routes
	batchGroup := group.Group("/batch_payments")
	batchGroup.Use(middleware.JWTMiddleware(authSvc))

	batchGroup.POST("/upload", handler.UploadCSV)
	batchGroup.GET("", handler.GetBatchList)
	batchGroup.GET("/:batch_id", handler.GetBatchStatus)
	batchGroup.POST("/:batch_id/process", handler.ProcessBatch)
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
