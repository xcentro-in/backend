package main

import (
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/adapters/database/postgres/business"
	notificationRepo "xcentro.in/internal/adapters/database/postgres/notification"
	userRepo "xcentro.in/internal/adapters/database/postgres/user"
	businessHandler "xcentro.in/internal/handlers/http/business"
	notificationHandler "xcentro.in/internal/handlers/http/notification"
	userHandler "xcentro.in/internal/handlers/http/user"
	businessSvc "xcentro.in/internal/services/business"
	notificationSvc "xcentro.in/internal/services/notification"
	userSvc "xcentro.in/internal/services/user"
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
	// Create business repository
	businessRepo := business.NewPgBusinessRepository(clients.db)
	businessUserRelRepo := business.NewPgBusinessUserRelationRepository(clients.db)

	// Create business service
	businessService := businessSvc.NewBusinessService(
		businessRepo,
		businessUserRelRepo,
		nil, // userRepository - not needed for ops endpoints
		nil, // userService - not needed for ops endpoints
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

	// Create user service
	userService := userSvc.NewUserService(
		userRepository,
		nil, // businessRepository - not needed for ops endpoints
		nil, // businessUserRelRepository - not needed for ops endpoints
		nil, // verificationService - not needed for ops endpoints
		clients,
		cfg,
	)

	// Register routes
	userHandler.RegisterOpsUserRoutes(group, userService, nil)
}

func registerOpsV1Routes(s *opsDashboardServer) {
	v1 := s.echo.Group("/ops/v1")

	v1.GET("/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{"status": "ok", "service": "ops-dashboard"})
	})

	clients := &s.meta.clients
	cfg := &s.meta.config

	// Register route groups
	registerOpsBusinessRoutes(clients, cfg, v1)
	registerOpsNotificationRoutes(clients, cfg, v1)
	registerOpsUserRoutes(clients, cfg, v1)
}

func registerRouters(s *opsDashboardServer) {
	registerOpsV1Routes(s)
}
