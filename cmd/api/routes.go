package main

import (
	"context"

	"github.com/labstack/echo/v4"
	authRepo "xcentro.in/internal/adapters/database/postgres/auth"
	batchPaymentRepo "xcentro.in/internal/adapters/database/postgres/batch_payment"
	"xcentro.in/internal/adapters/database/postgres/business"
	"xcentro.in/internal/adapters/database/postgres/client"
	dashboardRepo "xcentro.in/internal/adapters/database/postgres/dashboard"
	featureFlagsRepo "xcentro.in/internal/adapters/database/postgres/feature_flags"
	invoiceRepo "xcentro.in/internal/adapters/database/postgres/invoice"
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
	dashboardHandler "xcentro.in/internal/handlers/http/dashboard"
	featureFlagsHandler "xcentro.in/internal/handlers/http/feature_flags"
	invoiceHandler "xcentro.in/internal/handlers/http/invoice"
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
	dashboardSvc "xcentro.in/internal/services/dashboard"
	featureFlagsSvc "xcentro.in/internal/services/feature_flags"
	invoiceSvc "xcentro.in/internal/services/invoice"
	notificationService "xcentro.in/internal/services/notification"
	payeeSvc "xcentro.in/internal/services/payee"
	"xcentro.in/internal/services/payments"
	userSvc "xcentro.in/internal/services/user"
	"xcentro.in/internal/services/verification"
	voucherSvc "xcentro.in/internal/services/voucher"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/httputil"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/ratelimiter"
	"xcentro.in/internal/shared/utils"
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
	deviceVersionRepo := authRepo.NewPgDeviceVersionRepository(f.clients.db)
	businessRepo = business.NewPgBusinessRepository(f.clients.db).(*business.PgBusinessRepository)
	businessUserRelRepo = business.NewPgBusinessUserRelationRepository(f.clients.db).(*business.PgBusinessUserRelationRepository)
	paymentRepository := paymentRepo.NewPgPaymentRepository(f.clients.db, f.cfg.BankAccountEncryption.EncryptionKey)

	// Create services with proper dependency injection order
	// 1. User service (no circular dependencies)
	userService = userSvc.NewUserService(userRepository, businessRepo, businessUserRelRepo, verificationSvc, f.clients, f.cfg)
	// 2. Business service (depends on user service)
	busSvc = businessSvc.NewBusinessService(businessRepo, businessUserRelRepo, userRepository, userService, paymentRepository, f.clients, f.cfg)
	// 3. Auth service (depends on user service and business service, includes businessUserRelRepo for optimized role fetching)
	authSvc = authservice.NewAuthService(sessionRepo, deviceVersionRepo, businessUserRelRepo, userRepository, userService, busSvc, f.clients, f.cfg)
	return
}

func registerV1AuthRoutes(clients *externalClients, cfg *config.Config, group *echo.Group, cookieManager *httputil.CookieManager) {
	factory := &serviceFactory{clients, cfg}
	verificationSvc, err := verification.NewDocumentVerificationService()
	if err != nil {
		panic("Failed to create verification service: " + err.Error())
	}

	_, _, _, _, _, busSvc, authSvc := factory.createCoreServices(verificationSvc)
	auth.RegisterAuthRoutes(group, authSvc, busSvc, cfg, cookieManager)

	// Feature Flags
	featureFlagsRepository := featureFlagsRepo.NewPgFeatureFlagsRepository(clients.db, clients.rcache)
	featureFlagsService := featureFlagsSvc.NewFeatureFlagsService(featureFlagsRepository)
	featureFlagsHandlerInstance := featureFlagsHandler.NewFeatureFlagsHandler(featureFlagsService)

	authGroup := group.Group("/auth")
	authGroup.GET("/feature-flags", featureFlagsHandlerInstance.GetFeatureFlags)
}

func registerV1PaymentRoutes(clients *externalClients, cfg *config.Config, group *echo.Group, cookieManager *httputil.CookieManager) {
	factory := &serviceFactory{clients, cfg}

	paymentRepository := paymentRepo.NewPgPaymentRepository(clients.db, cfg.BankAccountEncryption.EncryptionKey)
	storedCardRepository := paymentRepo.NewPgStoredCardRepository(clients.db)
	bankAccountRepository := paymentRepo.NewPgBankAccountRepository(clients.db, cfg.BankAccountEncryption.EncryptionKey)
	pendingVoucherRepository := paymentRepo.NewPaymentVoucherPendingRepository(clients.db)
	stateChangeRepository := paymentRepo.NewPostgresPaymentStateChangeRepository(clients.db)

	_, _, _, businessUserRelRepo, userService, busSvc, authSvc := factory.createCoreServices(nil)

	// Initialize payment gateway clients conditionally based on MockPG setting
	var easebuzzClient, jioPayClient, mockPGClient corePayment.PaymentGateway

	if cfg.MockPG.Enabled {
		// MockPG mode: Only initialize MockPG client
		mockPGClient = mockpg.NewMockPGClient(cfg.MockPG.MerchantID, cfg.MockPG.APIKey, cfg.MockPG.SimulateSuccess, cfg.MockPG.IsAggregatorModel)
		// Real gateways will be nil (never called when MockPG is enabled)
	} else {
		// Production mode: Only initialize real payment gateways
		easebuzzClient = easebuzz.NewEasebuzzClient(cfg.Easebuzz.BaseURL, cfg.Easebuzz.MerchantKey, cfg.Easebuzz.Salt, cfg.Easebuzz.APIKey, cfg.Easebuzz.MerchantTokenID, cfg.Easebuzz.IsAggregatorModel, userService)
		jioPayClient = jiopay.NewJioPayClient(cfg.JioPay.BaseURL, cfg.JioPay.TokenizationBaseURL, cfg.JioPay.MerchantID, cfg.JioPay.APIKey, cfg.JioPay.AppID, cfg.JioPay.EncryptionKey, cfg.JioPay.IsAggregatorModel, busSvc)
		// MockPG will be nil (not used in production)
	}

	// Initialize fee rules repository (direct access, no bloated calculator layer)
	feeRulesRepository := paymentRepo.NewPostgresFeeRulesRepository(clients.db)
	databaseFeeCalculator := business_rules.NewDatabaseFeeCalculator(feeRulesRepository)

	// Adapter for orchestrator (still needed for gateway fee calculations)
	feeCalculatorAdapter := &feeCalculatorAdapter{calc: databaseFeeCalculator}

	// Create payment orchestrator with all gateway clients
	orchestrator := corePayment.NewPaymentOrchestrator(paymentRepository, storedCardRepository, jioPayClient, easebuzzClient, mockPGClient, cfg.MockPG.Enabled, feeCalculatorAdapter, logger.GetDefault())
	walletService := business_rules.NewWalletService(storedCardRepository, bankAccountRepository, clients.txManager, busSvc, logger.GetDefault())
	paymentSourceService := payments.NewPaymentSourceService(walletService, databaseFeeCalculator)

	voucherRepository := voucherRepo.NewPostgresVoucherRepository(clients.db, cfg.BankAccountEncryption.EncryptionKey)
	queueRepository := voucherRepo.NewPostgresERPSyncQueueRepository(clients.db)
	voucherCoreService := coreVoucher.NewVoucherService(voucherRepository)
	businessPayeeRelationRepo := payeeRepo.NewPgBusinessPayeeRelationRepository(clients.db, cfg.BankAccountEncryption.EncryptionKey)
	notificationEventRepo := notificationRepo.NewPgNotificationEventRepository(clients.db)
	voucherService := voucherSvc.NewVoucherService(voucherRepository, queueRepository, voucherCoreService, businessPayeeRelationRepo, notificationEventRepo, logger.GetDefault())

	// Pass fee rules repository directly to payment service (no calculator wrapper)
	paymentService := payments.NewPaymentService(orchestrator, paymentRepository, storedCardRepository, bankAccountRepository, walletService, feeRulesRepository, clients.txManager, voucherService, voucherRepository, pendingVoucherRepository, busSvc, stateChangeRepository, cfg, logger.GetDefault())

	processPaymentGroup := group.Group("/process_payment")

	// Register public routes FIRST (no JWT required) - these are called by external systems
	// Invoice dependencies (needed by both callback and webhook handlers)
	businessRepo := business.NewPgBusinessRepository(clients.db).(*business.PgBusinessRepository)
	invoiceRepository := invoiceRepo.NewPgInvoiceRepository(clients.db)
	invoiceService := invoiceSvc.NewInvoiceService(invoiceRepository)
	pdfGenerator := utils.NewPDFGenerator()

	// 1. Webhooks (called by payment gateways)
	webhookHandler := payment.NewWebhookHandler(paymentRepository, jioPayClient, easebuzzClient, mockPGClient, cfg.MockPG.Enabled, stateChangeRepository, logger.GetDefault(), businessRepo, invoiceService)
	webhookGroup := processPaymentGroup.Group("/webhooks")
	webhookGroup.POST("/payment_status", webhookHandler.HandlePaymentStatusWebhook)
	webhookGroup.POST("/settlement_status", webhookHandler.HandleSettlementStatusWebhook)

	// 2. Callbacks (redirects from payment gateways after payment completion)
	callbackHandler := payment.NewCallbackHandler(paymentRepository, storedCardRepository, walletService, jioPayClient, easebuzzClient, mockPGClient, cfg.MockPG.Enabled, pendingVoucherRepository, voucherRepository, clients.txManager, stateChangeRepository, businessRepo, invoiceService)
	processPaymentGroup.POST("/callback", callbackHandler.HandleCallback)

	// 3. Mock 3DS page (simulates bank OTP/3DS page for testing)
	mock3DSHandler := payment.NewMock3DSHandler(paymentRepository, cfg.Server.URL)
	processPaymentGroup.GET("/mock_3ds", mock3DSHandler.HandleMock3DSPage)

	// 4. Payment callback page (user-facing HTML redirect pages)
	paymentCallbackHandler := payment.NewPaymentCallbackHandler(paymentRepository)
	processPaymentGroup.GET("/callback_page", paymentCallbackHandler.HandlePaymentCallback)

	// Invoice routes (user-facing)
	invoiceGroup := group.Group("/invoices")
	invoiceUserHandler := invoiceHandler.NewInvoiceHandler(invoiceService, pdfGenerator)
	invoiceGroup.Use(middleware.JWTMiddleware(authSvc))
	invoiceGroup.Use(middleware.CSRFMiddleware(cookieManager))
	invoiceGroup.GET("/download", invoiceUserHandler.DownloadInvoice)

	// Register JWT-protected routes (authenticated user operations)
	payment.RegisterPaymentRoutes(processPaymentGroup, paymentService, paymentSourceService, walletService, authSvc, voucherRepository, stateChangeRepository, businessPayeeRelationRepo, storedCardRepository, bankAccountRepository, businessUserRelRepo, clients.rcache, cookieManager, logger.GetDefault())

	walletGroup := group.Group("/wallet")
	payment.RegisterWalletRoutes(walletGroup, paymentService, paymentSourceService, walletService, authSvc, storedCardRepository, businessUserRelRepo, clients.rcache, cookieManager, logger.GetDefault())
}

func registerV1BusinessRoutes(clients *externalClients, cfg *config.Config, group *echo.Group, cookieManager *httputil.CookieManager) {
	factory := &serviceFactory{clients, cfg}
	verificationSvc, err := verification.NewDocumentVerificationService()
	if err != nil {
		panic("Failed to create verification service: " + err.Error())
	}

	_, _, _, businessUserRelRepo, userService, busSvc, authSvc := factory.createCoreServices(verificationSvc)
	businessHandler.RegisterBusinessRoutes(group, busSvc, authSvc, userService, &cfg.S3, businessUserRelRepo, clients.rcache, cookieManager)
}

func registerV1UserRoutes(clients *externalClients, cfg *config.Config, group *echo.Group, cookieManager *httputil.CookieManager) {
	factory := &serviceFactory{clients, cfg}
	verificationSvc, err := verification.NewDocumentVerificationService()
	if err != nil {
		panic("Failed to create verification service: " + err.Error())
	}

	_, _, _, businessUserRelRepo, userService, _, authSvc := factory.createCoreServices(verificationSvc)
	userHandler.RegisterUserRoutes(group, userService, authSvc, businessUserRelRepo, clients.rcache, cfg, cookieManager)
}

func registerV1PayeeRoutes(clients *externalClients, cfg *config.Config, group *echo.Group, cookieManager *httputil.CookieManager) {
	factory := &serviceFactory{clients, cfg}
	_, _, _, businessUserRelationRepo, _, busSvc, authSvc := factory.createCoreServices(nil)

	payeeFactory := payeeSvc.NewPayeeServiceFactory(clients.db, clients.txManager, busSvc, cfg)
	payeeService := payeeFactory.NewPayeeService()

	payeeGroup := group.Group("/payees")
	payeeGroup.Use(middleware.JWTMiddleware(authSvc))
	payeeGroup.Use(middleware.BusinessAccessMiddleware(businessUserRelationRepo, clients.rcache))
	payeeGroup.Use(ratelimiter.MiddlewareDefault())

	payee.RegisterPayeeRoutes(payeeGroup, payeeService, authSvc, cookieManager)
}

func registerV1VoucherRoutes(clients *externalClients, cfg *config.Config, group *echo.Group, cookieManager *httputil.CookieManager) {
	factory := &serviceFactory{clients, cfg}
	_, _, _, businessUserRelRepo, _, _, authSvc := factory.createCoreServices(nil)

	voucherRepository := voucherRepo.NewPostgresVoucherRepository(clients.db, cfg.BankAccountEncryption.EncryptionKey)
	queueRepository := voucherRepo.NewPostgresERPSyncQueueRepository(clients.db)
	voucherCoreService := coreVoucher.NewVoucherService(voucherRepository)
	businessPayeeRelationRepo := payeeRepo.NewPgBusinessPayeeRelationRepository(clients.db, cfg.BankAccountEncryption.EncryptionKey)
	notificationEventRepo := notificationRepo.NewPgNotificationEventRepository(clients.db)
	voucherService := voucherSvc.NewVoucherService(voucherRepository, queueRepository, voucherCoreService, businessPayeeRelationRepo, notificationEventRepo, logger.GetDefault())

	// S3 client for document uploads — document upload is a first-class feature
	// and the service panics if the client is nil, so fail fast at startup.
	s3Client, err := fileupload.NewS3ClientFromConfig(cfg.S3)
	if err != nil {
		panic("Failed to create S3 client for voucher uploads: " + err.Error())
	}

	// Document upload service & handler
	uploadService := voucherSvc.NewDocumentUploadService(voucherRepository, s3Client, clients.rcache.Client, logger.GetDefault())
	uploadHandler := voucherHandler.NewDocumentUploadHandler(uploadService, logger.GetDefault())

	// Document review (HITL) service & handler
	payeeVendorRepository := payeeRepo.NewPgPayeeVendorRepository(clients.db)
	payeeMasterRepository := payeeRepo.NewPgPayeeMasterRepository(clients.db)
	reviewBusinessPayeeRelationRepo := payeeRepo.NewPgBusinessPayeeRelationRepository(clients.db, cfg.BankAccountEncryption.EncryptionKey)
	matchingEngine := voucherSvc.NewMatchingEngine(voucherRepository, clients.rcache.Client, clients.txManager, logger.GetDefault())
	reviewService := voucherSvc.NewDocumentReviewService(voucherRepository, payeeVendorRepository, clients.rcache.Client, matchingEngine, payeeMasterRepository, reviewBusinessPayeeRelationRepo, clients.txManager, logger.GetDefault())
	reviewHandler := voucherHandler.NewDocumentReviewHandler(reviewService, logger.GetDefault())

	// Async workers for the voucher document pipeline.
	// StartMatchingWorker drains the matching queue on a blocking BRPop loop.
	// StartReconcileLoop re-enqueues extraction tasks for documents stuck in
	// QUEUED past the threshold (e.g. lost during a Redis outage at upload time).
	go matchingEngine.StartMatchingWorker(context.Background())
	go uploadService.StartReconcileLoop(context.Background())

	voucherGroup := group.Group("/vouchers")
	voucherHandler.RegisterVoucherRoutes(voucherGroup, voucherService, uploadHandler, reviewHandler, authSvc, businessUserRelRepo, clients.rcache, cookieManager)
}

func registerV1PayoutRoutes(clients *externalClients, cfg *config.Config, group *echo.Group, cookieManager *httputil.CookieManager) {
	transferRepository := payoutRepo.NewPostgresPayoutTransferRepository(clients.db)
	paymentRepository := paymentRepo.NewPgPaymentRepository(clients.db, cfg.BankAccountEncryption.EncryptionKey)
	stateChangeRepository := paymentRepo.NewPostgresPaymentStateChangeRepository(clients.db)

	webhookHandler := payoutHandler.NewPayoutWebhookHandler(transferRepository, paymentRepository, stateChangeRepository, cfg.Castler.WebhookSecret)

	// Payout routes - separated by purpose
	payoutGroup := group.Group("/payout")

	// Webhook endpoints (external - called by Castler)
	payoutGroup.POST("/webhooks/castler", webhookHandler.HandleCastlerWebhook)

	// NOTE: Dashboard/Operations endpoints have been moved to cmd/ops (ops dashboard service)
	// See cmd/ops/routes.go -> registerOpsPayoutRoutes() for all payout operations endpoints
}

func registerV1BatchPaymentRoutes(clients *externalClients, cfg *config.Config, group *echo.Group, cookieManager *httputil.CookieManager) {
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
	batchGroup.Use(middleware.CSRFMiddleware(cookieManager))
	batchGroup.Use(ratelimiter.MiddlewareDefault())

	batchGroup.POST("/upload", handler.UploadCSV)
	batchGroup.GET("", handler.GetBatchList)
	batchGroup.GET("/:batch_id", handler.GetBatchStatus)
	batchGroup.POST("/:batch_id/process", handler.ProcessBatch)
	batchGroup.GET("/template", handler.DownloadSampleCSV) // Download sample CSV template
}

func registerV1NotificationRoutes(clients *externalClients, cfg *config.Config, group *echo.Group, cookieManager *httputil.CookieManager) {
	factory := &serviceFactory{clients, cfg}

	userNotifRepo := notificationRepo.NewPgUserNotificationRepository(clients.db)
	prefsRepo := notificationRepo.NewPgNotificationPreferencesRepository(clients.db)
	eventRepo := notificationRepo.NewPgNotificationEventRepository(clients.db)
	notifService := notificationService.NewUserFacingNotificationService(eventRepo, userNotifRepo, prefsRepo, clients.txManager, clients.rcache.Client, logger.GetDefault())

	_, _, _, _, _, _, authSvc := factory.createCoreServices(nil)
	notificationHandler.RegisterNotificationRoutes(group, notifService, authSvc, cookieManager)
}

func registerV1DashboardRoutes(clients *externalClients, cfg *config.Config, group *echo.Group, cookieManager *httputil.CookieManager) {
	factory := &serviceFactory{clients, cfg}

	// Create dashboard repository
	dashboardRepository := dashboardRepo.NewPgDashboardRepository(clients.db)

	// Create dashboard service
	dashboardService := dashboardSvc.NewDashboardService(dashboardRepository, logger.GetDefault())

	// Create dashboard handler
	handler := dashboardHandler.NewDashboardHandler(dashboardService, logger.GetDefault())

	// Get auth service for JWT middleware
	_, _, _, _, _, _, authSvc := factory.createCoreServices(nil)

	group.Use(middleware.JWTMiddleware(authSvc))
	group.Use(middleware.CSRFMiddleware(cookieManager))
	group.Use(ratelimiter.MiddlewareDefault())

	// Register route with JWT middleware
	group.GET("/dashboard", handler.GetDashboardMetrics)
}

func registerV1Routes(s *xcentroServer) {
	v1 := s.echo.Group("/v1")

	v1.GET("/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{"status": "ok"})
	})

	clients := &s.meta.clients
	cfg := &s.meta.config

	// Create a single CookieManager instance for all routes
	cookieManager := httputil.NewCookieManager(cfg)

	registerV1AuthRoutes(clients, cfg, v1, cookieManager)
	registerV1PaymentRoutes(clients, cfg, v1, cookieManager)
	registerV1UserRoutes(clients, cfg, v1, cookieManager)
	registerV1BusinessRoutes(clients, cfg, v1, cookieManager)
	registerV1PayeeRoutes(clients, cfg, v1, cookieManager)
	registerV1VoucherRoutes(clients, cfg, v1, cookieManager)
	registerV1PayoutRoutes(clients, cfg, v1, cookieManager)
	registerV1BatchPaymentRoutes(clients, cfg, v1, cookieManager)
	registerV1NotificationRoutes(clients, cfg, v1, cookieManager)
	registerV1DashboardRoutes(clients, cfg, v1, cookieManager)
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
