package main

import (
	"context"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"xcentro.in/internal/shared/logger"
)

type XcentroServer struct {
	echo *echo.Echo
	log  *logger.Logger
}

func NewXcentroServer() *XcentroServer {
	server := &XcentroServer{
		echo: echo.New(),
		log:  logger.GetDefault(),
	}

	server.RegisterGlobalMiddleware()
	server.RegisterRouters()

	return server
}

func (s *XcentroServer) RegisterGlobalMiddleware() {
	s.echo.Use(middleware.RequestID())
	s.echo.Use(middleware.Logger())
	s.echo.Use(middleware.Recover())
}

func (s *XcentroServer) RegisterRouters() {
	RegisterRouters(s.echo)
}

func (s *XcentroServer) Start(address string) {
	err := s.echo.Start(address)
	if err != nil {
		s.log.Errorf("Failed to start server error %v", err)
	}
}

func (s *XcentroServer) Stop() {
	err := s.echo.Shutdown(context.TODO())
	if err != nil {
		s.log.Errorf("Failed to shutdown server error %v", err)
	}
}

func main() {
	server := NewXcentroServer()
	server.Start(":8888")
}
