package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	redis "xcentro.in/internal/adapters/cache/redis/client"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/adapters/providers/authbridge"
	"xcentro.in/internal/adapters/providers/mailers"
	metricsProvider "xcentro.in/internal/adapters/providers/metrics"
	"xcentro.in/internal/core/metrics"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/ratelimiter"
	customValidator "xcentro.in/internal/shared/validator"
)

type externalClients struct {
	db           client.DB
	txManager    client.TxManager
	rcache       *redis.RedisClient
	emailService mailers.EmailService
	metrics      metrics.MetricsProvider
}

// Getter methods for services to extract what they need
func (e *externalClients) GetDB() client.DB {
	return e.db
}

func (e *externalClients) GetTxManager() client.TxManager {
	return e.txManager
}

func (e *externalClients) GetRedisClient() *redis.RedisClient {
	return e.rcache
}

func (e *externalClients) GetEmailService() mailers.EmailService {
	return e.emailService
}

func (e *externalClients) GetMetrics() interface{} {
	return e.metrics
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
	server.registerValidator()
	server.registerGlobalMiddleware()
	server.registerClients()
	server.registerRouters()

	return server
}

func (s *xcentroServer) registerGlobalMiddleware() {
	s.echo.Use(middleware.RequestID())
	s.echo.Use(middleware.Logger())
	s.echo.Use(middleware.Recover())
	s.echo.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     s.meta.config.CORS.AllowOrigins,
		AllowCredentials: s.meta.config.CORS.AllowCredentials,
		AllowHeaders: []string{
			"Content-Type",
			"Authorization",
			"X-Business-Id",
			"Accept",
			"Accept-Language",
			"Accept-Encoding",
			"X-Requested-With",
			"Origin",
			"Cache-Control",
			"X-CSRF-Token",
		},
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"},
		ExposeHeaders: []string{
			"Content-Length",
			"Content-Type",
			"X-Request-Id",
			"X-RateLimit-Limit",
			"X-RateLimit-Remaining",
		},
		MaxAge: s.meta.config.CORS.MaxAge,
	}))

	// Configure secure middleware based on environment
	s.echo.Use(middleware.SecureWithConfig(buildSecureConfig(s.meta.config.Environment, s.meta.config.Frontend.URL)))
}

// buildSecureConfig returns security middleware configuration based on environment
func buildSecureConfig(env, frontendURL string) middleware.SecureConfig {
	if env == "production" {
		return middleware.SecureConfig{
			XSSProtection:         "",       // Modern browsers ignore this header, rely on CSP
			ContentTypeNosniff:    "nosniff", // Prevent MIME type sniffing
			XFrameOptions:         "DENY",   // Prevent clickjacking
			HSTSMaxAge:            31536000, // 1 year
			HSTSExcludeSubdomains: false,
			HSTSPreloadEnabled:    false,
			ContentSecurityPolicy: "default-src 'self'; base-uri 'self'; object-src 'none'; frame-ancestors 'none'; form-action 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; script-src 'self'; style-src 'self';",
			CSPReportOnly:         false,
			ReferrerPolicy:        "strict-origin-when-cross-origin",
		}
	}

	// dev environment
	return middleware.SecureConfig{
		XSSProtection:         "",        // Modern browsers ignore this header
		ContentTypeNosniff:    "nosniff", // Prevent MIME type sniffing
		XFrameOptions:         "",        // Let CSP handle frame-ancestors
		HSTSMaxAge:            0,         // Disable HSTS in dev
		HSTSExcludeSubdomains: true,
		HSTSPreloadEnabled:    false,
		ContentSecurityPolicy: fmt.Sprintf("default-src 'self' %s; base-uri 'self'; object-src 'none'; frame-ancestors 'self' %s; form-action 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self' %s; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline';", frontendURL, frontendURL, frontendURL),
		CSPReportOnly:         true, // Report-only in dev to avoid breaking things
		ReferrerPolicy:        "no-referrer-when-downgrade",
	}
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

func (s *xcentroServer) registerValidator() {
	s.echo.Validator = customValidator.NewCustomValidator()
	s.log.Info("Custom validator registered successfully")
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

	// Rate limiter (depends on Redis)
	if err := ratelimiter.Initialize(
		redisClient.Client,
		&s.meta.config.RateLimiter,
		s.meta.config.Cookie.Secure,
		s.meta.config.Cookie.SameSite,
		s.meta.config.Cookie.Domain,
	); err != nil {
		s.log.Errorf("Failed to initialize rate limiter: %v", err)
		panic(err)
	}
	s.log.Info("Rate limiter initialized successfully")

	// Email service (SES) - depends on rate limiter
	emailService, err := mailers.NewSESClientFromConfig(s.meta.config.SES)
	if err != nil {
		s.log.Errorf("Failed to create email service: %v", err)
		panic(err)
	}
	s.log.Info("Email service created successfully")
	s.meta.clients.emailService = emailService

	// Metrics provider (CloudWatch)
	metricsClient, err := metricsProvider.NewMetricsProvider(s.meta.config)
	if err != nil {
		s.log.Warnf("Failed to create metrics provider: %v (using no-op provider)", err)
		metricsClient = metrics.NewNoOpMetricsProvider()
	}
	if s.meta.config.CloudWatch.Enabled {
		s.log.Info("CloudWatch metrics provider created successfully")
	} else {
		s.log.Info("Metrics provider disabled (using no-op)")
	}
	s.meta.clients.metrics = metricsClient

	// AuthBridge client (singleton)
	if err := authbridge.Initialize(&s.meta.config); err != nil {
		s.log.Errorf("Failed to initialize AuthBridge client: %v", err)
		panic(err)
	}
	s.log.Info("AuthBridge client initialized successfully")
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

func (s *xcentroServer) stop() {
	s.log.Info("Shutting down server gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Flush pending metrics before shutdown
	if err := s.meta.clients.metrics.Flush(ctx); err != nil {
		s.log.Errorf("Failed to flush metrics: %v", err)
	} else {
		s.log.Info("Metrics flushed successfully")
	}

	// Close metrics provider
	if err := s.meta.clients.metrics.Close(); err != nil {
		s.log.Errorf("Failed to close metrics provider: %v", err)
	} else {
		s.log.Info("Metrics provider closed successfully")
	}

	// Shutdown HTTP server
	err := s.echo.Shutdown(ctx)
	if err != nil {
		s.log.Errorf("Failed to shutdown server: %v", err)
	} else {
		s.log.Info("Server shutdown complete")
	}
}

func main() {
	server := NewXcentroServer()

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start server in goroutine
	go func() {
		server.start()
	}()

	// Wait for shutdown signal
	<-sigChan
	server.stop()
}
