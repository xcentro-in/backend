package main

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	httphandler "xcentro.in/internal/handlers/http"
	"xcentro.in/internal/shared/logger"
)

type XcentroServer struct {
	echo *echo.Echo
	log  *logger.Logger
}

func NewXcentroServer() *XcentroServer {
	e := echo.New()

	e.Use(middleware.RequestID())
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	return &XcentroServer{
		echo: e,
		log:  logger.GetDefault(),
	}
}

func (s *XcentroServer) RegisterRouters() {
	httphandler.RegisterRouters(s.echo)
}

func (s *XcentroServer) Start(address string) {
	err := s.echo.Start(address)
	if err != nil {
		s.log.Errorf("Failed to start server error %v", err)
	}
}
