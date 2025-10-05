package main

import (
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/adapters/database/postgres/business"
	paymentRepo "xcentro.in/internal/adapters/database/postgres/payment"
	userRepo "xcentro.in/internal/adapters/database/postgres/user"
	voucherRepo "xcentro.in/internal/adapters/database/postgres/voucher"
	"xcentro.in/internal/core/business_rules"
	corePayment "xcentro.in/internal/core/payment"
	coreVoucher "xcentro.in/internal/core/voucher"
	"xcentro.in/internal/handlers/http/auth"
	businessHandler "xcentro.in/internal/handlers/http/business"
	"xcentro.in/internal/handlers/http/payee"
	"xcentro.in/internal/handlers/http/payment"
	userHandler "xcentro.in/internal/handlers/http/user"
	voucherHandler "xcentro.in/internal/handlers/http/voucher"
	authservice "xcentro.in/internal/services/auth"
	businessSvc "xcentro.in/internal/services/business"
	payeeSvc "xcentro.in/internal/services/payee"
	"xcentro.in/internal/services/payments"
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

	userService := user.NewUserService(
		userRepo.NewPgUserRepository(meta.clients.db),
		business.NewPgBusinessUserRelationRepository(meta.clients.db),
		meta.clients.txManager,
		meta.clients.emailService,
		verificationService,
		&meta.config,
	)
	authService := authservice.NewAuthService(&meta.config.JWT, meta.clients.rcache)

	// Create business service for login user state
	busSvc := businessSvc.NewBusinessService(
		business.NewPgBusinessRepository(meta.clients.db),
		business.NewPgBusinessUserRelationRepository(meta.clients.db),
		userRepo.NewPgUserRepository(meta.clients.db),
		meta.clients.txManager,
		meta.clients.rcache,
	)

	auth.RegisterAuthRoutes(group, userService, authService, busSvc)
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

	// Register payment routes with payment service
	processPaymentGroup := group.Group("/process_payment")
	payment.RegisterPaymentRoutes(processPaymentGroup, paymentService)
}

func registerV1BusinessRoutes(meta *serverMetaData, group *echo.Group) {
	// Create verification service
	verificationService, err := verification.NewDocumentVerificationService()
	if err != nil {
		panic("Failed to create verification service: " + err.Error())
	}

	busSvc := businessSvc.NewBusinessService(business.NewPgBusinessRepository(meta.clients.db),
		business.NewPgBusinessUserRelationRepository(meta.clients.db),
		userRepo.NewPgUserRepository(meta.clients.db),
		meta.clients.txManager,
		meta.clients.rcache,
	)
	authService := authservice.NewAuthService(&meta.config.JWT, meta.clients.rcache)

	userService := user.NewUserService(
		userRepo.NewPgUserRepository(meta.clients.db),
		business.NewPgBusinessUserRelationRepository(meta.clients.db),
		meta.clients.txManager,
		meta.clients.emailService,
		verificationService,
		&meta.config,
	)

	businessHandler.RegisterBusinessRoutes(group, busSvc, authService, userService, &meta.config.S3)
}

func registerV1UserRoutes(meta *serverMetaData, group *echo.Group) {
	// Create verification service
	verificationService, err := verification.NewDocumentVerificationService()
	if err != nil {
		panic("Failed to create verification service: " + err.Error())
	}

	userService := user.NewUserService(
		userRepo.NewPgUserRepository(meta.clients.db),
		business.NewPgBusinessUserRelationRepository(meta.clients.db),
		meta.clients.txManager,
		meta.clients.emailService,
		verificationService,
		&meta.config,
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

	// Voucher Routes
	registerV1VoucherRoutes(&s.meta, v1)
}

func registerRouters(s *xcentroServer) {
	registerV1Routes(s)
}
