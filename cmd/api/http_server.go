package main

import (
	"context"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/shared/logger"
)

type externalClients struct {
	db        client.DB
	txManager client.TxManager
}

type xcentroServer struct {
	echo    *echo.Echo
	log     *logger.Logger
	clients externalClients
}

func NewXcentroServer() *xcentroServer {
	server := &xcentroServer{
		echo: echo.New(),
		log:  logger.GetDefault(),
	}

	server.registerGlobalMiddleware()
	server.registerClients()
	server.registerRouters()

	return server
}

func (s *xcentroServer) registerGlobalMiddleware() {
	s.echo.Use(middleware.RequestID())
	s.echo.Use(middleware.Logger())
	s.echo.Use(middleware.Recover())
}

func (s *xcentroServer) registerClients() {
	// Database client
	dbConfig := client.DefaultDBConfig()
	pgxclient, err := client.NewPgxDB(context.Background(), dbConfig)
	if err != nil {
		s.log.Errorf("Failed to create database client: %v", err)
		panic(err)
	}
	s.log.Info("Database client created successfully")
	s.clients.db = pgxclient
	s.clients.txManager = client.NewTxManager(pgxclient)
}

func (s *xcentroServer) registerRouters() {
	registerRouters(s)
}

func (s *xcentroServer) start(address string) {
	err := s.echo.Start(address)
	if err != nil {
		s.log.Errorf("Failed to start server error %v", err)
	}
}

// func (s *xcentroServer) stop() {
// 	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
// 	defer cancel()

// 	err := s.echo.Shutdown(ctx)
// 	if err != nil {
// 		s.log.Errorf("Failed to shutdown server error %v", err)
// 	}
// }

func main() {
	server := NewXcentroServer()
	server.start(":8888")
}
