package main

import (
	"xcentro.in/internal/shared/config"
)

// Shared service factory to avoid duplication
type serviceFactory struct {
	clients *externalClients
	cfg     *config.Config
}

// TODO: Add route registration functions here
// Example:
// func registerOpsUserRoutes(clients *externalClients, cfg *config.Config, group *echo.Group) {
//     // Handler registration will go here
// }

func registerOpsV1Routes(s *opsDashboardServer) {
	v1 := s.echo.Group("/ops/v1")

	v1.GET("/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{"status": "ok", "service": "ops-dashboard"})
	})

	clients := &s.meta.clients
	cfg := &s.meta.config

	// TODO: Register route groups here when handlers are ready
	// registerOpsUserRoutes(clients, cfg, v1)
	// registerOpsBusinessRoutes(clients, cfg, v1)
	// registerOpsNotificationRoutes(clients, cfg, v1)

	_ = clients
	_ = cfg
}

func registerRouters(s *opsDashboardServer) {
	registerOpsV1Routes(s)
}
