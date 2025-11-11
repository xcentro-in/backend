package main

import (
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/adapters/database/postgres/business"
	httpHandlers "xcentro.in/internal/handlers/http"
	businessSvc "xcentro.in/internal/services/business"
	"xcentro.in/internal/shared/config"
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
	httpHandlers.RegisterOpsBusinessRoutes(group, businessService, nil)
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

	// TODO: Register other route groups when ready
	// registerOpsUserRoutes(clients, cfg, v1)
	// registerOpsNotificationRoutes(clients, cfg, v1)
}

func registerRouters(s *opsDashboardServer) {
	registerOpsV1Routes(s)
}
