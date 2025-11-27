package main

import (
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/adapters/database/postgres/business"
	notificationRepo "xcentro.in/internal/adapters/database/postgres/notification"
	payeeRepo "xcentro.in/internal/adapters/database/postgres/payee"
	paymentRepo "xcentro.in/internal/adapters/database/postgres/payment"
	payoutRepo "xcentro.in/internal/adapters/database/postgres/payout"
	userRepo "xcentro.in/internal/adapters/database/postgres/user"
	voucherRepo "xcentro.in/internal/adapters/database/postgres/voucher"
	"xcentro.in/internal/adapters/external/payout/factory"
	businessHandler "xcentro.in/internal/handlers/http/business"
	notificationHandler "xcentro.in/internal/handlers/http/notification"
	paymentHandler "xcentro.in/internal/handlers/http/payment"
	payoutHandler "xcentro.in/internal/handlers/http/payout"
	userHandler "xcentro.in/internal/handlers/http/user"
	"xcentro.in/internal/middleware"
	businessSvc "xcentro.in/internal/services/business"
	notificationSvc "xcentro.in/internal/services/notification"
	payoutSvc "xcentro.in/internal/services/payout"
	"xcentro.in/internal/services/payments"
	userSvc "xcentro.in/internal/services/user"
	"xcentro.in/internal/services/verification"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
)

// Shared service factory to avoid duplication
type serviceFactory struct {
	clients *externalClients
	cfg     *config.Config
}

// registerOpsBusinessRoutes registers ops dashboard business routes
func registerOpsBusinessRoutes(clients *externalClients, cfg *config.Config, group *echo.Group) {
	// Create repositories
	businessRepo := business.NewPgBusinessRepository(clients.db)
	businessUserRelRepo := business.NewPgBusinessUserRelationRepository(clients.db)
	userRepository := userRepo.NewPgUserRepository(clients.db)

	// Create business service (it initializes its own verification service internally)
	businessService := businessSvc.NewBusinessService(
		businessRepo,
		businessUserRelRepo,
		userRepository, // userRepository - needed for KYB verify endpoint
		nil,            // userService - not needed for ops endpoints
		clients,
		cfg,
	)

	// Register routes
	businessHandler.RegisterOpsBusinessRoutes(group, businessService, nil)
}

// registerOpsNotificationRoutes registers ops dashboard notification routes
func registerOpsNotificationRoutes(clients *externalClients, cfg *config.Config, group *echo.Group) {
	// Create notification repositories
	eventRepo := notificationRepo.NewPgNotificationEventRepository(clients.db)
	userNotifRepo := notificationRepo.NewPgUserNotificationRepository(clients.db)
	prefsRepo := notificationRepo.NewPgNotificationPreferencesRepository(clients.db)

	// Create notification service
	notificationService := notificationSvc.NewUserFacingNotificationService(
		eventRepo,
		userNotifRepo,
		prefsRepo,
		clients.txManager,
		clients.rcache.Client,
		logger.GetDefault(),
	)

	// Register routes
	notificationHandler.RegisterOpsNotificationRoutes(group, notificationService, nil)
}

// registerOpsUserRoutes registers ops dashboard user routes
func registerOpsUserRoutes(clients *externalClients, cfg *config.Config, group *echo.Group) {
	// Create user repository
	userRepository := userRepo.NewPgUserRepository(clients.db)

	// Create business user relation repository (needed for KYC rejection)
	businessUserRelRepo := business.NewPgBusinessUserRelationRepository(clients.db)

	// Create verification service (needed for reload-pan endpoint)
	verificationSvc, err := verification.NewDocumentVerificationService()
	if err != nil {
		logger.GetDefault().Errorf("Failed to create verification service: %v", err)
		verificationSvc = nil
	}

	// Create user service
	userService := userSvc.NewUserService(
		userRepository,
		nil,                    // businessRepository - not needed for ops endpoints
		businessUserRelRepo,    // businessUserRelRepository - needed for KYC rejection
		verificationSvc,        // verificationService - needed for reload-pan endpoint
		clients,
		cfg,
	)

	// Register routes
	userHandler.RegisterOpsUserRoutes(group, userService, nil)
}

// registerOpsCollectionRoutes registers ops dashboard collection (payment) routes
func registerOpsCollectionRoutes(clients *externalClients, cfg *config.Config, group *echo.Group) {
	// Create payment repository (encryption key needed for decryption but ops listing doesn't need it)
	paymentRepository := paymentRepo.NewPgPaymentRepository(clients.db, cfg.BankAccountEncryption.EncryptionKey)

	// Create ops collection service (lightweight, doesn't need full PaymentService dependencies)
	collectionService := payments.NewOpsCollectionService(paymentRepository)

	// Register routes
	paymentHandler.RegisterOpsCollectionRoutes(group, collectionService)
}

// registerOpsFeeRulesRoutes registers ops dashboard fee rules management routes
func registerOpsFeeRulesRoutes(clients *externalClients, cfg *config.Config, group *echo.Group) {
	// Create fee rules repository
	feeRulesRepository := paymentRepo.NewPostgresFeeRulesRepository(clients.db)

	// Create ops fee rules handler
	feeRulesHandler := paymentHandler.NewOpsFeeRulesHandler(
		feeRulesRepository,
		logger.GetDefault(),
	)

	// Register routes
	paymentHandler.RegisterOpsFeeRulesRoutes(group, feeRulesHandler)
}

// registerOpsPayoutRoutes registers ops dashboard payout routes
func registerOpsPayoutRoutes(clients *externalClients, cfg *config.Config, group *echo.Group) {
	// Create repositories
	payoutRepository := payoutRepo.NewPostgresOpsPayoutRepository(clients.db)
	paymentRepository := paymentRepo.NewPgPaymentRepository(clients.db, cfg.BankAccountEncryption.EncryptionKey)
	transferRepository := payoutRepo.NewPostgresPayoutTransferRepository(clients.db)
	configRepository := payoutRepo.NewPostgresPayoutConfigurationRepository(clients.db)
	voucherRepository := voucherRepo.NewPostgresVoucherRepository(clients.db, cfg.BankAccountEncryption.EncryptionKey)
	bankAccountRepository := payeeRepo.NewPgPayeeBankAccountRepository(clients.db, cfg.BankAccountEncryption.EncryptionKey)
	stateChangeRepository := paymentRepo.NewPostgresPaymentStateChangeRepository(clients.db)

	// Create ops payout service (for listing endpoints)
	opsPayoutService := payoutSvc.NewOpsPayoutService(payoutRepository)

	// Create payout provider factory and full payout service (for operations)
	providerFactory := factory.NewPayoutProviderFactory(cfg)
	payoutService := payoutSvc.NewPayoutService(
		paymentRepository,
		voucherRepository,
		transferRepository,
		bankAccountRepository,
		configRepository,
		stateChangeRepository,
		providerFactory,
	)

	// Register all payout routes (listing + operations)
	payoutHandler.RegisterOpsPayoutRoutes(group, opsPayoutService, payoutService)
}

func registerOpsV1Routes(s *opsDashboardServer) {
	v1 := s.echo.Group("/ops/v1")

	// Apply ops access control middleware to all routes
	v1.Use(middleware.OpsAccessMiddleware(&s.meta.config))

	v1.GET("/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{"status": "ok", "service": "ops-dashboard"})
	})

	clients := &s.meta.clients
	cfg := &s.meta.config

	// Register route groups
	registerOpsBusinessRoutes(clients, cfg, v1)
	registerOpsNotificationRoutes(clients, cfg, v1)
	registerOpsUserRoutes(clients, cfg, v1)
	registerOpsPayoutRoutes(clients, cfg, v1)
	registerOpsCollectionRoutes(clients, cfg, v1)
	registerOpsFeeRulesRoutes(clients, cfg, v1)
}

func initializeNotificationTrigger(clients *externalClients) {
	eventRepo := notificationRepo.NewPgNotificationEventRepository(clients.db)
	notificationSvc.InitializeNotificationTrigger(eventRepo)
	logger.GetDefault().Info("Notification trigger initialized for ops dashboard")
}

func registerRouters(s *opsDashboardServer) {
	initializeNotificationTrigger(&s.meta.clients)
	registerOpsV1Routes(s)
}
