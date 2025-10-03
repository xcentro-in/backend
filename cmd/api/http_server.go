package main

import (
	"context"
	"fmt"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	redis "xcentro.in/internal/adapters/cache/redis/client"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
)

type externalClients struct {
	db        client.DB
	txManager client.TxManager
	rcache    *redis.RedisClient
}

type serverMetaData struct {
	clients externalClients
	config  config.Config
}

type xcentroServer struct {
	echo *echo.Echo
	log  *logger.Logger
	meta serverMetaData
}

func NewXcentroServer() *xcentroServer {
	server := &xcentroServer{
		echo: echo.New(),
		log:  logger.GetDefault(),
	}

	server.loadConfig()
	server.registerGlobalMiddleware()
	server.registerClients()
	server.registerRouters()

	return server
}

func (s *xcentroServer) registerGlobalMiddleware() {
	s.echo.Use(middleware.RequestID())
	s.echo.Use(middleware.Logger())
	s.echo.Use(middleware.Recover())
	s.echo.Use(middleware.CORS())
}

func (s *xcentroServer) loadConfig() {
	cfg, err := config.Load("")
	if err != nil {
		s.log.Errorf("Failed to load config: %v", err)
		panic(err)
	}
	s.log.Info("Config loaded successfully")
	s.meta.config = cfg
}

func (s *xcentroServer) registerClients() {
	// Postgres client
	pgxclient, err := client.NewPgxDB(context.Background(), s.meta.config.Database.ToDBConfig())
	if err != nil {
		s.log.Errorf("Failed to create database client: %v", err)
		panic(err)
	}
	s.log.Info("Database client created successfully")
	s.meta.clients.db = pgxclient
	s.meta.clients.txManager = client.NewTxManager(pgxclient)

	// Redis client
	redisClient, err := redis.NewRedisClient(s.meta.config.Rcache)
	if err != nil {
		s.log.Errorf("Failed to create Redis client: %v", err)
		panic(err)
	}
	s.log.Info("Redis client created successfully")
	s.meta.clients.rcache = redisClient
}

func (s *xcentroServer) registerRouters() {
	registerRouters(s)
}

func (s *xcentroServer) start() {
	addr := fmt.Sprintf("%s:%d", s.meta.config.Server.Host, s.meta.config.Server.Port)
	s.log.Infof("Starting server at %s", addr)
	err := s.echo.Start(addr)
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
	server.start()
}
