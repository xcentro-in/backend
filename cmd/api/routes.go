package main

import (
	"github.com/labstack/echo/v4"
	authRepo "xcentro.in/internal/adapters/database/postgres/auth"
	"xcentro.in/internal/adapters/database/postgres/business"
	payeeRepo "xcentro.in/internal/adapters/database/postgres/payee"
	paymentRepo "xcentro.in/internal/adapters/database/postgres/payment"
	payoutRepo "xcentro.in/internal/adapters/database/postgres/payout"
	userRepo "xcentro.in/internal/adapters/database/postgres/user"
	voucherRepo "xcentro.in/internal/adapters/database/postgres/voucher"
	"xcentro.in/internal/adapters/external/payment/easebuzz"
	"xcentro.in/internal/adapters/external/payment/jiopay"
	"xcentro.in/internal/adapters/external/payout/factory"
	"xcentro.in/internal/core/business_rules"
	corePayment "xcentro.in/internal/core/payment"
	coreVoucher "xcentro.in/internal/core/voucher"
	"xcentro.in/internal/handlers/http/auth"
	businessHandler "xcentro.in/internal/handlers/http/business"
	"xcentro.in/internal/handlers/http/payee"
	"xcentro.in/internal/handlers/http/payment"
	payoutHandler "xcentro.in/internal/handlers/http/payout"
	userHandler "xcentro.in/internal/handlers/http/user"
	voucherHandler "xcentro.in/internal/handlers/http/voucher"
	authservice "xcentro.in/internal/services/auth"
	businessSvc "xcentro.in/internal/services/business"
	payeeSvc "xcentro.in/internal/services/payee"
	"xcentro.in/internal/services/payments"
	payoutSvc "xcentro.in/internal/services/payout"
	"xcentro.in/internal/services/user"
	"xcentro.in/internal/services/verification"
	voucherSvc "xcentro.in/internal/services/voucher"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
)

func registerV1AuthRoutes(clients *externalClients, cfg *config.Config, group *echo.Group) {
	// Create verification service
	verificationService, err := verification.NewDocumentVerificationService()
	if err != nil {
		panic("Failed to create verification service: " + err.Error())
	}

	// Create repositories
	userRepository := userRepo.NewPgUserRepository(clients.db)
	sessionRepo := authRepo.NewPgSessionRepository(clients.db)

	// Create business service - pass clients and config
	busSvc := businessSvc.NewBusinessService(
		business.NewPgBusinessRepository(clients.db),
		business.NewPgBusinessUserRelationRepository(clients.db),
		userRepository,
		clients,
		cfg,
	)

	// Create user service - pass clients and config
	userService := user.NewUserService(
		userRepository,
		business.NewPgBusinessRepository(clients.db),
		business.NewPgBusinessUserRelationRepository(clients.db),
		verificationService,
		clients,
		cfg,
	)

	// Create auth service - pass clients and config
	authService := authservice.NewAuthService(
		sessionRepo,
		userRepository,
		userService,
		busSvc,
		clients,
		cfg,
	)

	// Register auth routes
	auth.RegisterAuthRoutes(group, authService, busSvc, cfg)
}

func registerV1PaymentRoutes(clients *externalClients, cfg *config.Config, group *echo.Group) {
	// Create payment repository instances (Phase 4.2)
	paymentRepository := paymentRepo.NewPgPaymentRepository(clients.db)
	storedCardRepository := paymentRepo.NewPgStoredCardRepository(clients.db)
	bankAccountRepository := paymentRepo.NewPgBankAccountRepository(clients.db)

	// Create business service for onboarding flags
	busSvc := businessSvc.NewBusinessService(
		business.NewPgBusinessRepository(clients.db),
		business.NewPgBusinessUserRelationRepository(clients.db),
		userRepo.NewPgUserRepository(clients.db),
		clients,
		cfg,
	)

	// Create real payment gateway clients
	easebuzzClient := easebuzz.NewEasebuzzClient(
		cfg.Easebuzz.BaseURL,
		cfg.Easebuzz.MerchantKey,
		cfg.Easebuzz.Salt,
		cfg.Easebuzz.APIKey,
	)

	jioPayClient := jiopay.NewJioPayClient(
		cfg.JioPay.BaseURL,
		cfg.JioPay.TokenizationBaseURL,
		cfg.JioPay.MerchantID,
		cfg.JioPay.APIKey,
		cfg.JioPay.AppID,
		cfg.JioPay.EncryptionKey,
	)

	// Create business rules services
	orchestrator := corePayment.NewMockPaymentOrchestrator(paymentRepository, storedCardRepository, jioPayClient, easebuzzClient)
	feeCalculator := business_rules.NewMockFeeCalculator()
	walletService := business_rules.NewMockWalletService(storedCardRepository, bankAccountRepository, clients.txManager, busSvc)

	// Phase 10: Create database-backed fee calculator
	feeRulesRepository := paymentRepo.NewPostgresFeeRulesRepository(clients.db)
	databaseFeeCalculator := business_rules.NewDatabaseFeeCalculator(feeRulesRepository)

	// Phase 10: Create payment source service
	paymentSourceService := payments.NewPaymentSourceService(walletService, databaseFeeCalculator)

	// PHASE 6: Create voucher service for payment integration
	voucherRepository := voucherRepo.NewPostgresVoucherRepository(clients.db)
	approvalRepository := voucherRepo.NewPostgresApprovalRepository(clients.db)
	queueRepository := voucherRepo.NewPostgresERPSyncQueueRepository(clients.db)
	voucherCoreService := coreVoucher.NewVoucherService(voucherRepository)

	// Create business-payee relation repository for voucher relation_id resolution
	businessPayeeRelationRepo := payeeRepo.NewPgBusinessPayeeRelationRepository(clients.db)

	voucherService := voucherSvc.NewVoucherService(
		voucherRepository,
		approvalRepository,
		queueRepository,
		voucherCoreService,
		businessPayeeRelationRepo, // Pass repository to resolve relation_id
	)

	// Create payment service layer (Phase 4.4+ / Phase 6)
	paymentService := payments.NewPaymentService(
		orchestrator,
		paymentRepository,
		storedCardRepository,
		bankAccountRepository,
		walletService,
		feeCalculator,
		clients.txManager,
		voucherService,       // PHASE 6: Inject voucher service
		busSvc,               // Business service for onboarding flags
		logger.GetDefault(),  // Logger instance
	)

	// Create auth service for JWT middleware
	sessionRepo := authRepo.NewPgSessionRepository(clients.db)
	userService := user.NewUserService(
		userRepo.NewPgUserRepository(clients.db),
		business.NewPgBusinessRepository(clients.db),
		business.NewPgBusinessUserRelationRepository(clients.db),
		nil, // verificationService not needed for JWT
		clients,
		cfg,
	)
	authService := authservice.NewAuthService(
		sessionRepo,
		userRepo.NewPgUserRepository(clients.db),
		userService,
		busSvc,
		clients,
		cfg,
	)

	// Register payment routes with payment service (Phase 10: Added paymentSourceService)
	processPaymentGroup := group.Group("/process_payment")
	payment.RegisterPaymentRoutes(processPaymentGroup, paymentService, paymentSourceService, authService)

	// Register wallet routes (wallet management separated from payment processing)
	walletGroup := group.Group("/wallet")
	payment.RegisterWalletRoutes(walletGroup, paymentService, paymentSourceService, authService)

	// Register callback handler for payment gateway responses (SURL/FURL)
	// These are user-facing endpoints where gateways POST payment results
	callbackHandler := payment.NewCallbackHandler(
		paymentRepository,
		jioPayClient,
		easebuzzClient,
	)
	processPaymentGroup.POST("/callback", callbackHandler.HandleCallback)

	// Register webhook handlers for async payment status notifications
	// These are backend-to-backend notifications for payment status updates
	webhookHandler := payment.NewWebhookHandler(
		paymentRepository,
		jioPayClient,
		easebuzzClient,
	)
	webhookGroup := processPaymentGroup.Group("/webhooks")

	// Payment status webhooks (real-time payment updates)
	// Endpoint: /api/v1/process_payment/webhooks/payment_status?type=<gateway>
	webhookGroup.POST("/payment_status", webhookHandler.HandlePaymentStatusWebhook)

	// Settlement webhooks (batch settlement notifications T+1 to T+3)
	// Endpoint: /api/v1/process_payment/webhooks/settlement_status?type=<gateway>
	webhookGroup.POST("/settlement_status", webhookHandler.HandleSettlementStatusWebhook)
}

func registerV1BusinessRoutes(clients *externalClients, cfg *config.Config, group *echo.Group) {
	// Create verification service
	verificationService, err := verification.NewDocumentVerificationService()
	if err != nil {
		panic("Failed to create verification service: " + err.Error())
	}

	// Create repositories
	userRepository := userRepo.NewPgUserRepository(clients.db)
	sessionRepo := authRepo.NewPgSessionRepository(clients.db)

	// Create user service - pass clients and config
	userService := user.NewUserService(
		userRepository,
		business.NewPgBusinessRepository(clients.db),
		business.NewPgBusinessUserRelationRepository(clients.db),
		verificationService,
		clients,
		cfg,
	)

	// Create business service - pass clients and config
	busSvc := businessSvc.NewBusinessService(
		business.NewPgBusinessRepository(clients.db),
		business.NewPgBusinessUserRelationRepository(clients.db),
		userRepository,
		clients,
		cfg,
	)

	// Create auth service - pass clients and config
	authService := authservice.NewAuthService(
		sessionRepo,
		userRepository,
		userService,
		busSvc,
		clients,
		cfg,
	)

	businessUserRelRepo := business.NewPgBusinessUserRelationRepository(clients.db)
	businessHandler.RegisterBusinessRoutes(group, busSvc, authService, userService, &cfg.S3, businessUserRelRepo, clients.rcache)
}

func registerV1UserRoutes(clients *externalClients, cfg *config.Config, group *echo.Group) {
	// Create verification service
	verificationService, err := verification.NewDocumentVerificationService()
	if err != nil {
		panic("Failed to create verification service: " + err.Error())
	}

	// Create repositories
	userRepository := userRepo.NewPgUserRepository(clients.db)
	sessionRepo := authRepo.NewPgSessionRepository(clients.db)

	// Create user service - pass clients and config
	userService := user.NewUserService(
		userRepository,
		business.NewPgBusinessRepository(clients.db),
		business.NewPgBusinessUserRelationRepository(clients.db),
		verificationService,
		clients,
		cfg,
	)

	// Create business service - pass clients and config
	busSvc := businessSvc.NewBusinessService(
		business.NewPgBusinessRepository(clients.db),
		business.NewPgBusinessUserRelationRepository(clients.db),
		userRepository,
		clients,
		cfg,
	)

	// Create auth service - pass clients and config
	authService := authservice.NewAuthService(
		sessionRepo,
		userRepository,
		userService,
		busSvc,
		clients,
		cfg,
	)

	businessUserRelRepo := business.NewPgBusinessUserRelationRepository(clients.db)
	userHandler.RegisterUserRoutes(group, userService, authService, businessUserRelRepo, clients.rcache)
}

func registerV1PayeeRoutes(clients *externalClients, cfg *config.Config, group *echo.Group) {
	// Create auth service for JWT middleware
	userRepository := userRepo.NewPgUserRepository(clients.db)
	sessionRepo := authRepo.NewPgSessionRepository(clients.db)

	// Create business service
	busSvc := businessSvc.NewBusinessService(
		business.NewPgBusinessRepository(clients.db),
		business.NewPgBusinessUserRelationRepository(clients.db),
		userRepository,
		clients,
		cfg,
	)

	// Create user service
	userService := user.NewUserService(
		userRepository,
		business.NewPgBusinessRepository(clients.db),
		business.NewPgBusinessUserRelationRepository(clients.db),
		nil, // verificationService not needed for JWT
		clients,
		cfg,
	)

	// Create auth service
	authService := authservice.NewAuthService(
		sessionRepo,
		userRepository,
		userService,
		busSvc,
		clients,
		cfg,
	)

	// Use factory pattern to create PayeeService with all dependencies
	factory := payeeSvc.NewPayeeServiceFactory(clients.db, clients.txManager, busSvc, cfg)
	payeeService := factory.NewPayeeService()

	// Register payee routes with auth middleware
	payeeGroup := group.Group("/payees")
	payee.RegisterPayeeRoutes(payeeGroup, payeeService, authService)

	// Register vendor dropdown route (lightweight API for voucher creation)
	payee.RegisterVendorDropdownRoute(group, payeeService)
}

func registerV1VoucherRoutes(clients *externalClients, group *echo.Group) {
	// Create voucher repository instances
	voucherRepository := voucherRepo.NewPostgresVoucherRepository(clients.db)
	approvalRepository := voucherRepo.NewPostgresApprovalRepository(clients.db)
	queueRepository := voucherRepo.NewPostgresERPSyncQueueRepository(clients.db)

	// Create voucher core service (domain logic)
	voucherCoreService := coreVoucher.NewVoucherService(voucherRepository)

	// Create business-payee relation repository for voucher relation_id resolution
	businessPayeeRelationRepo := payeeRepo.NewPgBusinessPayeeRelationRepository(clients.db)

	// Create voucher service layer (orchestration)
	voucherService := voucherSvc.NewVoucherService(
		voucherRepository,
		approvalRepository,
		queueRepository,
		voucherCoreService,
		businessPayeeRelationRepo, // Pass repository to resolve relation_id
	)

	// Register voucher routes
	voucherGroup := group.Group("/vouchers")
	voucherHandler.RegisterVoucherRoutes(voucherGroup, voucherService)
}

func registerV1PayoutRoutes(clients *externalClients, cfg *config.Config, group *echo.Group) {
	// Create payout repository instances
	transferRepository := payoutRepo.NewPostgresPayoutTransferRepository(clients.db)
	configRepository := payoutRepo.NewPostgresPayoutConfigurationRepository(clients.db)
	voucherRepository := voucherRepo.NewPostgresVoucherRepository(clients.db)
	bankAccountRepository := payeeRepo.NewPgPayeeBankAccountRepository(clients.db)

	// Get webhook secret from configuration
	webhookSecret := cfg.Castler.WebhookSecret

	// Create provider factory
	providerFactory := factory.NewPayoutProviderFactory(cfg)

	// Create payout service
	payoutService := payoutSvc.NewPayoutService(
		voucherRepository,
		transferRepository,
		bankAccountRepository,
		configRepository,
		providerFactory,
	)

	// Create webhook handler
	webhookHandler := payoutHandler.NewPayoutWebhookHandler(
		transferRepository,
		voucherRepository,
		webhookSecret,
	)

	// Create operations dashboard handler
	dashboardHandler := payoutHandler.NewPayoutHandler(
		payoutService,
		transferRepository,
		configRepository,
	)

	// Register payout routes
	payoutGroup := group.Group("/payout")

	// Webhook endpoint (no auth required - verified via HMAC signature)
	payoutGroup.POST("/webhooks/castler", webhookHandler.HandleCastlerWebhook)

	// Operations dashboard endpoints
	payoutGroup.GET("/pending-approvals", dashboardHandler.GetPendingApprovals)
	payoutGroup.POST("/:transfer_id/approve", dashboardHandler.ApproveTransfer)
	payoutGroup.POST("/:transfer_id/cancel", dashboardHandler.CancelTransfer)
	payoutGroup.GET("/:transfer_id", dashboardHandler.GetTransferDetails)
	payoutGroup.GET("/:transfer_id/webhooks", dashboardHandler.GetWebhookHistory)
	payoutGroup.GET("/timeouts", dashboardHandler.GetTimeoutTransfers)
	payoutGroup.POST("/:transfer_id/mark-completed", dashboardHandler.MarkCompletedManually)
	payoutGroup.GET("/config", dashboardHandler.GetConfig)
	payoutGroup.PUT("/config", dashboardHandler.UpdateConfig)
	payoutGroup.POST("/config", dashboardHandler.CreateConfig)
}

func registerV1Routes(s *xcentroServer) {
	v1 := s.echo.Group("/v1")

	// Health Check Endpoint
	v1.GET("/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{
			"status": "ok",
		})
	})

	// implement callback endpoint and print headers, and body
	v1.POST("/callback", func(c echo.Context) error {
		headers := c.Request().Header
		body := make([]byte, c.Request().ContentLength)
		c.Request().Body.Read(body)

		s.log.Infof("Headers: %v", headers)
		s.log.Infof("Body: %s", string(body))

		return c.JSON(200, map[string]string{
			"status": "received",
		})
	})

	// Get references to clients and config
	clients := &s.meta.clients
	cfg := &s.meta.config

	// Authentication Routes
	registerV1AuthRoutes(clients, cfg, v1)

	// Payment Routes
	registerV1PaymentRoutes(clients, cfg, v1)

	// User Routes
	registerV1UserRoutes(clients, cfg, v1)

	// Business Routes
	registerV1BusinessRoutes(clients, cfg, v1)

	// Payee Routes
	registerV1PayeeRoutes(clients, cfg, v1)

	// Voucher Routes
	registerV1VoucherRoutes(clients, v1)

	// Payout Routes (Phase 9: Webhooks + Operations Dashboard)
	registerV1PayoutRoutes(clients, cfg, v1)
}

func registerRouters(s *xcentroServer) {
	registerV1Routes(s)
}
