package main

import (
	"github.com/labstack/echo/v4"
	authRepo "xcentro.in/internal/adapters/database/postgres/auth"
	"xcentro.in/internal/adapters/database/postgres/business"
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
)

func registerV1AuthRoutes(meta *serverMetaData, group *echo.Group) {
	// Create verification service
	verificationService, err := verification.NewDocumentVerificationService()
	if err != nil {
		panic("Failed to create verification service: " + err.Error())
	}

	// Create repositories
	userRepository := userRepo.NewPgUserRepository(meta.clients.db)
	sessionRepo := authRepo.NewPgSessionRepository(meta.clients.db)

	// Create business service
	busSvc := businessSvc.NewBusinessService(
		business.NewPgBusinessRepository(meta.clients.db),
		business.NewPgBusinessUserRelationRepository(meta.clients.db),
		userRepository,
		meta.clients.txManager,
		meta.clients.rcache,
	)

	// Create user service
	userService := user.NewUserService(
		userRepository,
		business.NewPgBusinessRepository(meta.clients.db),
		business.NewPgBusinessUserRelationRepository(meta.clients.db),
		meta.clients.txManager,
		meta.clients.emailService,
		verificationService,
		&meta.config,
		meta.clients.rcache,
	)

	// Create auth service with all dependencies
	authService := authservice.NewAuthService(
		&meta.config.JWT,
		meta.clients.rcache,
		sessionRepo,
		userRepository,
		userService,
		busSvc,
		meta.clients.emailService,
		&meta.config,
	)

	// Register auth routes
	auth.RegisterAuthRoutes(group, authService, &meta.config)
}

func registerV1PaymentRoutes(meta *serverMetaData, group *echo.Group) {
	// Create payment repository instances (Phase 4.2)
	paymentRepository := paymentRepo.NewPgPaymentRepository(meta.clients.db)
	storedCardRepository := paymentRepo.NewPgStoredCardRepository(meta.clients.db)
	bankAccountRepository := paymentRepo.NewPgBankAccountRepository(meta.clients.db)

	// Create real payment gateway clients
	easebuzzClient := easebuzz.NewEasebuzzClient(
		meta.config.Easebuzz.BaseURL,
		meta.config.Easebuzz.MerchantKey,
		meta.config.Easebuzz.Salt,
		meta.config.Easebuzz.APIKey,
	)

	jioPayClient := jiopay.NewJioPayClient(
		meta.config.JioPay.BaseURL,
		meta.config.JioPay.TokenizationBaseURL,
		meta.config.JioPay.MerchantID,
		meta.config.JioPay.APIKey,
		meta.config.JioPay.AppID,
		meta.config.JioPay.EncryptionKey,
	)

	// Create business rules services
	orchestrator := corePayment.NewMockPaymentOrchestrator(paymentRepository, storedCardRepository, jioPayClient, easebuzzClient)
	feeCalculator := business_rules.NewMockFeeCalculator()
	walletService := business_rules.NewMockWalletService(storedCardRepository, bankAccountRepository, meta.clients.txManager)

	// Phase 10: Create database-backed fee calculator
	feeRulesRepository := paymentRepo.NewPostgresFeeRulesRepository(meta.clients.db)
	databaseFeeCalculator := business_rules.NewDatabaseFeeCalculator(feeRulesRepository)

	// Phase 10: Create payment source service
	paymentSourceService := payments.NewPaymentSourceService(walletService, databaseFeeCalculator)

	// PHASE 6: Create voucher service for payment integration
	voucherRepository := voucherRepo.NewPostgresVoucherRepository(meta.clients.db)
	approvalRepository := voucherRepo.NewPostgresApprovalRepository(meta.clients.db)
	queueRepository := voucherRepo.NewPostgresERPSyncQueueRepository(meta.clients.db)
	voucherCoreService := coreVoucher.NewVoucherService(voucherRepository)
	voucherService := voucherSvc.NewVoucherService(
		voucherRepository,
		approvalRepository,
		queueRepository,
		voucherCoreService,
	)

	// Create payment service layer (Phase 4.4+ / Phase 6)
	paymentService := payments.NewPaymentService(
		orchestrator,
		paymentRepository,
		storedCardRepository,
		bankAccountRepository,
		walletService,
		feeCalculator,
		meta.clients.txManager,
		voucherService, // PHASE 6: Inject voucher service
	)

	// Register payment routes with payment service (Phase 10: Added paymentSourceService)
	processPaymentGroup := group.Group("/process_payment")
	payment.RegisterPaymentRoutes(processPaymentGroup, paymentService, paymentSourceService)

	// Register wallet routes (wallet management separated from payment processing)
	walletGroup := group.Group("/wallet")
	payment.RegisterWalletRoutes(walletGroup, paymentService, paymentSourceService)

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

func registerV1BusinessRoutes(meta *serverMetaData, group *echo.Group) {
	// Create verification service
	verificationService, err := verification.NewDocumentVerificationService()
	if err != nil {
		panic("Failed to create verification service: " + err.Error())
	}

	// Create repositories
	userRepository := userRepo.NewPgUserRepository(meta.clients.db)
	sessionRepo := authRepo.NewPgSessionRepository(meta.clients.db)

	// Create user service
	userService := user.NewUserService(
		userRepository,
		business.NewPgBusinessRepository(meta.clients.db),
		business.NewPgBusinessUserRelationRepository(meta.clients.db),
		meta.clients.txManager,
		meta.clients.emailService,
		verificationService,
		&meta.config,
		meta.clients.rcache,
	)

	// Create business service
	busSvc := businessSvc.NewBusinessService(
		business.NewPgBusinessRepository(meta.clients.db),
		business.NewPgBusinessUserRelationRepository(meta.clients.db),
		userRepository,
		meta.clients.txManager,
		meta.clients.rcache,
	)

	// Create auth service with all dependencies
	authService := authservice.NewAuthService(
		&meta.config.JWT,
		meta.clients.rcache,
		sessionRepo,
		userRepository,
		userService,
		busSvc,
		meta.clients.emailService,
		&meta.config,
	)

	businessUserRelRepo := business.NewPgBusinessUserRelationRepository(meta.clients.db)
	businessHandler.RegisterBusinessRoutes(group, busSvc, authService, userService, &meta.config.S3, businessUserRelRepo, meta.clients.rcache)
}

func registerV1UserRoutes(meta *serverMetaData, group *echo.Group) {
	// Create verification service
	verificationService, err := verification.NewDocumentVerificationService()
	if err != nil {
		panic("Failed to create verification service: " + err.Error())
	}

	// Create repositories
	userRepository := userRepo.NewPgUserRepository(meta.clients.db)
	sessionRepo := authRepo.NewPgSessionRepository(meta.clients.db)

	// Create user service
	userService := user.NewUserService(
		userRepository,
		business.NewPgBusinessRepository(meta.clients.db),
		business.NewPgBusinessUserRelationRepository(meta.clients.db),
		meta.clients.txManager,
		meta.clients.emailService,
		verificationService,
		&meta.config,
		meta.clients.rcache,
	)

	// Create business service
	busSvc := businessSvc.NewBusinessService(
		business.NewPgBusinessRepository(meta.clients.db),
		business.NewPgBusinessUserRelationRepository(meta.clients.db),
		userRepository,
		meta.clients.txManager,
		meta.clients.rcache,
	)

	// Create auth service with all dependencies
	authService := authservice.NewAuthService(
		&meta.config.JWT,
		meta.clients.rcache,
		sessionRepo,
		userRepository,
		userService,
		busSvc,
		meta.clients.emailService,
		&meta.config,
	)

	businessUserRelRepo := business.NewPgBusinessUserRelationRepository(meta.clients.db)
	userHandler.RegisterUserRoutes(group, userService, authService, businessUserRelRepo, meta.clients.rcache)
}

func registerV1PayeeRoutes(meta *serverMetaData, group *echo.Group) {
	// Use factory pattern to create PayeeService with all dependencies
	factory := payeeSvc.NewPayeeServiceFactory(meta.clients.db, meta.clients.txManager)
	payeeService := factory.NewPayeeService()

	// Register payee routes
	payeeGroup := group.Group("/payees")
	payee.RegisterPayeeRoutes(payeeGroup, payeeService)
}

func registerV1VoucherRoutes(meta *serverMetaData, group *echo.Group) {
	// Create voucher repository instances
	voucherRepository := voucherRepo.NewPostgresVoucherRepository(meta.clients.db)
	approvalRepository := voucherRepo.NewPostgresApprovalRepository(meta.clients.db)
	queueRepository := voucherRepo.NewPostgresERPSyncQueueRepository(meta.clients.db)

	// Create voucher core service (domain logic)
	voucherCoreService := coreVoucher.NewVoucherService(voucherRepository)

	// Create voucher service layer (orchestration)
	voucherService := voucherSvc.NewVoucherService(
		voucherRepository,
		approvalRepository,
		queueRepository,
		voucherCoreService,
	)

	// Register voucher routes
	voucherGroup := group.Group("/vouchers")
	voucherHandler.RegisterVoucherRoutes(voucherGroup, voucherService)
}

func registerV1PayoutRoutes(meta *serverMetaData, group *echo.Group) {
	// Create payout repository instances
	transferRepository := payoutRepo.NewPostgresPayoutTransferRepository(meta.clients.db)
	configRepository := payoutRepo.NewPostgresPayoutConfigurationRepository(meta.clients.db)
	mappingRepository := payoutRepo.NewPostgresPayoutProviderMappingRepository(meta.clients.db)
	voucherRepository := voucherRepo.NewPostgresVoucherRepository(meta.clients.db)

	// Get webhook secret from configuration
	webhookSecret := meta.config.Castler.WebhookSecret

	// Create provider factory
	providerFactory := factory.NewPayoutProviderFactory(&meta.config)

	// Create payout service
	payoutService := payoutSvc.NewPayoutService(
		voucherRepository,
		transferRepository,
		mappingRepository,
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

	// Authentication Routes
	registerV1AuthRoutes(&s.meta, v1)

	// Payment Routes
	registerV1PaymentRoutes(&s.meta, v1)

	// User Routes
	registerV1UserRoutes(&s.meta, v1)

	// Business Routes
	registerV1BusinessRoutes(&s.meta, v1)

	// Payee Routes
	registerV1PayeeRoutes(&s.meta, v1)

	// Voucher Routes
	registerV1VoucherRoutes(&s.meta, v1)

	// Payout Routes (Phase 9: Webhooks + Operations Dashboard)
	registerV1PayoutRoutes(&s.meta, v1)
}

func registerRouters(s *xcentroServer) {
	registerV1Routes(s)
}
