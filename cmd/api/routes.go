package main

import (
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/adapters/database/postgres/business"
	paymentRepo "xcentro.in/internal/adapters/database/postgres/payment"
	userRepo "xcentro.in/internal/adapters/database/postgres/user"
	"xcentro.in/internal/core/business_rules"
	corePayment "xcentro.in/internal/core/payment"
	"xcentro.in/internal/handlers/http/auth"
	businessHandler "xcentro.in/internal/handlers/http/business"
	"xcentro.in/internal/handlers/http/payee"
	"xcentro.in/internal/handlers/http/payment"
	userHandler "xcentro.in/internal/handlers/http/user"
	authservice "xcentro.in/internal/services/auth"
	businessSvc "xcentro.in/internal/services/business"
	payeeSvc "xcentro.in/internal/services/payee"
	"xcentro.in/internal/services/payments"
	"xcentro.in/internal/services/user"
)

func registerV1AuthRoutes(meta *serverMetaData, group *echo.Group) {
	userService := user.NewUserService(
		userRepo.NewPgUserRepository(meta.clients.db),
		business.NewPgBusinessUserRelationRepository(meta.clients.db),
		meta.clients.txManager,
	)
	authService := authservice.NewAuthService(&meta.config.JWT, meta.clients.rcache)
	auth.RegisterAuthRoutes(group, userService, authService)
}

func registerV1PaymentRoutes(meta *serverMetaData, group *echo.Group) {
	// Create payment repository instances (Phase 4.2)
	paymentRepository := paymentRepo.NewPgPaymentRepository(meta.clients.db)
	storedCardRepository := paymentRepo.NewPgStoredCardRepository(meta.clients.db)
	bankAccountRepository := paymentRepo.NewPgBankAccountRepository(meta.clients.db)

	// Create business rules services
	orchestrator := corePayment.NewMockPaymentOrchestrator(paymentRepository, storedCardRepository)
	feeCalculator := business_rules.NewMockFeeCalculator()
	walletService := business_rules.NewMockWalletService(storedCardRepository, bankAccountRepository, meta.clients.txManager)

	// Create payment service layer (Phase 4.4+)
	paymentService := payments.NewPaymentService(
		orchestrator,
		paymentRepository,
		storedCardRepository,
		bankAccountRepository,
		walletService,
		feeCalculator,
		meta.clients.txManager,
	)

	// Register payment routes with payment service
	processPaymentGroup := group.Group("/process_payment")
	payment.RegisterPaymentRoutes(processPaymentGroup, paymentService)
}

func registerV1BusinessRoutes(meta *serverMetaData, group *echo.Group) {
	busSvc := businessSvc.NewBusinessService(business.NewPgBusinessRepository(meta.clients.db),
		business.NewPgBusinessUserRelationRepository(meta.clients.db),
		userRepo.NewPgUserRepository(meta.clients.db),
		meta.clients.txManager,
		meta.clients.rcache,
	)
	authService := authservice.NewAuthService(&meta.config.JWT, meta.clients.rcache)
	businessHandler.RegisterBusinessRoutes(group, busSvc, authService)
}

func registerV1UserRoutes(meta *serverMetaData, group *echo.Group) {
	userService := user.NewUserService(
		userRepo.NewPgUserRepository(meta.clients.db),
		business.NewPgBusinessUserRelationRepository(meta.clients.db),
		meta.clients.txManager,
	)
	authService := authservice.NewAuthService(&meta.config.JWT, meta.clients.rcache)
	userHandler.RegisterUserRoutes(group, userService, authService)
}

func registerV1PayeeRoutes(meta *serverMetaData, group *echo.Group) {
	// Use factory pattern to create PayeeService with all dependencies
	factory := payeeSvc.NewPayeeServiceFactory(meta.clients.db, meta.clients.txManager)
	payeeService := factory.NewPayeeService()

	// Register payee routes
	payeeGroup := group.Group("/payees")
	payee.RegisterPayeeRoutes(payeeGroup, payeeService)
}

func registerV1Routes(s *xcentroServer) {
	v1 := s.echo.Group("/v1")

	// Health Check Endpoint
	v1.GET("/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{
			"status": "ok",
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
}

func registerRouters(s *xcentroServer) {
	registerV1Routes(s)
}
