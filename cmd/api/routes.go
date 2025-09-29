package main

import (
	"github.com/labstack/echo/v4"
	userRepo "xcentro.in/internal/adapters/database/postgres/user"
	"xcentro.in/internal/handlers/http/auth"
	"xcentro.in/internal/handlers/http/payment"
	authservice "xcentro.in/internal/services/auth"
	"xcentro.in/internal/services/user"
)

func registerV1AuthRoutes(meta *serverMetaData, group *echo.Group) {
	userService := user.NewUserService(
		userRepo.NewPgUserRepository(meta.clients.db),
		meta.clients.txManager,
	)
	authService := authservice.NewAuthService(&meta.config.JWT, meta.clients.rcache)
	auth.RegisterAuthRoutes(group, userService, authService)
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
	processPaymentGroup := v1.Group("/process_payment")
	payment.RegisterPaymentRoutes(processPaymentGroup)

	// User Routes
	// Business Routes
}

func registerRouters(s *xcentroServer) {
	registerV1Routes(s)
}
