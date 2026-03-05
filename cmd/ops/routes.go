package main

import (
	"github.com/labstack/echo/v4"
	authRepo "xcentro.in/internal/adapters/database/postgres/auth"
	"xcentro.in/internal/adapters/database/postgres/business"
	coreAuth "xcentro.in/internal/core/auth"
	invoiceRepo "xcentro.in/internal/adapters/database/postgres/invoice"
	notificationRepo "xcentro.in/internal/adapters/database/postgres/notification"
	opsAuthRepo "xcentro.in/internal/adapters/database/postgres/opsauth"
	paymentRepo "xcentro.in/internal/adapters/database/postgres/payment"
	userRepo "xcentro.in/internal/adapters/database/postgres/user"
	businessHandler "xcentro.in/internal/handlers/http/business"
	invoiceHandler "xcentro.in/internal/handlers/http/invoice"
	notificationHandler "xcentro.in/internal/handlers/http/notification"
	opsAuthHandler "xcentro.in/internal/handlers/http/opsauth"
	paymentHandler "xcentro.in/internal/handlers/http/payment"
	userHandler "xcentro.in/internal/handlers/http/user"
	"xcentro.in/internal/middleware"
	authservice "xcentro.in/internal/services/auth"
	businessSvc "xcentro.in/internal/services/business"
	invoiceSvc "xcentro.in/internal/services/invoice"
	notificationSvc "xcentro.in/internal/services/notification"
	opsAuthSvc "xcentro.in/internal/services/opsauth"
	"xcentro.in/internal/services/payments"
	userSvc "xcentro.in/internal/services/user"
	"xcentro.in/internal/services/verification"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/httputil"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/utils"
)

// Shared service factory to avoid duplication
type serviceFactory struct {
	clients *externalClients
	cfg     *config.Config
}

// registerOpsBusinessRoutes registers ops dashboard business routes
func registerOpsBusinessRoutes(clients *externalClients, cfg *config.Config, group *echo.Group, cookieManager *httputil.CookieManager, tokenRevoker coreAuth.AuthService) {
	// Create repositories
	businessRepo := business.NewPgBusinessRepository(clients.db)
	businessUserRelRepo := business.NewPgBusinessUserRelationRepository(clients.db)
	userRepository := userRepo.NewPgUserRepository(clients.db)
	paymentRepository := paymentRepo.NewPgPaymentRepository(clients.db, cfg.BankAccountEncryption.EncryptionKey)

	// Create business service (it initializes its own verification service internally)
	businessService := businessSvc.NewBusinessService(
		businessRepo,
		businessUserRelRepo,
		userRepository,    // userRepository - needed for KYB verify endpoint
		nil,               // userService - not needed for ops endpoints
		paymentRepository, // paymentRepository - needed for showDashboard logic
		clients,
		cfg,
	)

	// Wire auth service so KYB reject/verify can revoke user tokens immediately
	if busSvcImpl, ok := businessService.(*businessSvc.BusinessServiceImpl); ok {
		busSvcImpl.SetAuthService(tokenRevoker)
	}

	// Register routes
	businessHandler.RegisterOpsBusinessRoutes(group, businessService, cookieManager)
}

// registerOpsNotificationRoutes registers ops dashboard notification routes
func registerOpsNotificationRoutes(clients *externalClients, cfg *config.Config, group *echo.Group, cookieManager *httputil.CookieManager) {
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
	notificationHandler.RegisterOpsNotificationRoutes(group, notificationService, cookieManager)
}

// registerOpsAuthRoutes registers ops dashboard authentication routes and returns the auth service
func registerOpsAuthRoutes(clients *externalClients, cfg *config.Config, group *echo.Group) *opsAuthSvc.OpsAuthService {
	// Create ops auth repositories
	opsUserRepo := opsAuthRepo.NewPgOpsUserRepository(clients.db)
	opsSessionRepo := opsAuthRepo.NewPgOpsSessionRepository(clients.db)

	// Create ops auth service
	opsAuthService, err := opsAuthSvc.NewOpsAuthService(
		opsUserRepo,
		opsSessionRepo,
		clients.rcache,
		clients.txManager,
		cfg,
	)
	if err != nil {
		logger.GetDefault().Errorf("Failed to create ops auth service: %v", err)
		// Fatal error - ops dashboard won't work without auth
		panic(err)
	}

	// Register auth routes (public - login, callback, refresh)
	opsAuthHandler.RegisterOpsAuthRoutes(group, opsAuthService, cfg)

	return opsAuthService
}

// registerOpsUserRoutes registers ops dashboard user routes
func registerOpsUserRoutes(clients *externalClients, cfg *config.Config, group *echo.Group, cookieManager *httputil.CookieManager, tokenRevoker coreAuth.AuthService) {
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
		nil,                 // businessRepository - not needed for ops endpoints
		businessUserRelRepo, // businessUserRelRepository - needed for KYC rejection
		verificationSvc,     // verificationService - needed for reload-pan endpoint
		clients,
		cfg,
	)

	// Wire auth service so KYC reject can revoke user tokens immediately
	if userSvcImpl, ok := userService.(*userSvc.UserServiceImpl); ok {
		userSvcImpl.SetAuthService(tokenRevoker)
	}

	// Register routes
	userHandler.RegisterOpsUserRoutes(group, userService, cookieManager)
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


// registerOpsInvoiceRoutes registers ops dashboard invoice routes
func registerOpsInvoiceRoutes(clients *externalClients, cfg *config.Config, group *echo.Group) {
	// Create invoice repository
	invoiceRepository := invoiceRepo.NewPgInvoiceRepository(clients.db)

	// Create invoice service
	invoiceService := invoiceSvc.NewInvoiceService(invoiceRepository)
	pdfGenerator := utils.NewPDFGenerator()

	// Create handler
	opsInvoiceHandler := invoiceHandler.NewOpsInvoiceHandler(invoiceService, pdfGenerator)

	// Register routes
	invoiceGroup := group.Group("/invoices")
	invoiceGroup.GET("", opsInvoiceHandler.ListInvoices)
	invoiceGroup.GET("/export", opsInvoiceHandler.ExportInvoices)
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

	// Create ops cookie manager (for CSRF tokens)
	opsCookieManager := httputil.NewOpsCookieManager(cfg)

	// Register auth routes (public - login, callback, refresh) and get auth service
	opsAuthService := registerOpsAuthRoutes(clients, cfg, v1)

	// Create a user-facing AuthService solely for token revocation triggered by ops actions
	// (KYB reject/verify, KYC reject). This breaks the circular dep: authSvc has no
	// dependency on business/user services when constructed with nil for those fields,
	// and RevokeAllUserTokens only uses deviceVersionRepo, sessionRepo, txManager, rclient.
	sessionRepo := authRepo.NewPgSessionRepository(clients.db)
	deviceVersionRepo := authRepo.NewPgDeviceVersionRepository(clients.db)
	tokenRevoker := authservice.NewAuthService(sessionRepo, deviceVersionRepo, nil, nil, nil, nil, clients, cfg)

	// Create protected group with only JWT middleware (CSRF applied per handler as needed)
	protectedGroup := v1.Group("")
	protectedGroup.Use(middleware.OpsJWTMiddleware(opsAuthService, cfg))

	// Register all protected route groups
	// Routes will apply CSRF middleware internally for state-changing operations
	opsAuthHandler.RegisterOpsUserRoutes(protectedGroup, opsAuthService, opsCookieManager)          // User profile endpoint
	registerOpsBusinessRoutes(clients, cfg, protectedGroup, opsCookieManager, tokenRevoker)         // Business management
	registerOpsNotificationRoutes(clients, cfg, protectedGroup, opsCookieManager)                   // Notifications
	registerOpsUserRoutes(clients, cfg, protectedGroup, opsCookieManager, tokenRevoker)             // User management
	registerOpsCollectionRoutes(clients, cfg, protectedGroup)                                       // Collections/Payments
	registerOpsFeeRulesRoutes(clients, cfg, protectedGroup)                                         // Fee rules
	registerOpsInvoiceRoutes(clients, cfg, protectedGroup)                                          // Invoices
}

func initializeNotificationTrigger(clients *externalClients) {
	eventRepo := notificationRepo.NewPgNotificationEventRepository(clients.db)
	notificationSvc.InitializeNotificationTrigger(eventRepo)
	logger.GetDefault().Info("Notification trigger initialized for ops dashboard")
}

func registerRouters(s *opsDashboardServer) {
	initializeNotificationTrigger(&s.meta.clients)

	// Public health check route (no authentication required)
	s.echo.GET("/ops/health", func(c echo.Context) error {
		if err := s.meta.clients.db.Ping(c.Request().Context()); err != nil {
			logger.GetDefault().Errorf("Health check failed: %v", err)
			return c.JSON(503, map[string]string{"status": "degraded"})
		}
		return c.JSON(200, map[string]string{"status": "ok"})
	})

	registerOpsV1Routes(s)
}
