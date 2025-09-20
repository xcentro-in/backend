package main

import (
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/adapters/database/postgres/user_repository"
	"xcentro.in/internal/handlers/http/auth"
	"xcentro.in/internal/services/user"
)

func placeHolderFn(c echo.Context) error { return c.String(200, "This is a placeholder function") }

func registerV1AuthRoutes(s *xcentroServer) {
	userService := user.NewUserService(
		user_repository.NewPgUserRepository(s.clients.db),
		s.clients.txManager,
	)
	auth.RegisterAuthRoutes(s.echo, userService, nil)
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
	registerV1AuthRoutes(s)
	// User Routes
	// Buiseness Routes
}

func registerRouters(s *xcentroServer) {
	registerV1Routes(s)
}
