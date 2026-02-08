package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"xcentro.in/internal/adapters/database/postgres/business"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/adapters/database/postgres/notification"
	userRepo "xcentro.in/internal/adapters/database/postgres/user"
	"xcentro.in/internal/adapters/providers/mailers"
	notificationService "xcentro.in/internal/services/notification"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/ratelimiter"
	"xcentro.in/internal/shared/secrets"
	"xcentro.in/internal/workers"
)

func main() {
	ctx := context.Background()

	// Load .env file into environment variables
	if err := godotenv.Load(".env"); err != nil {
		log.Printf("Failed to load .env file: %v (will use existing environment variables)", err)
	}

	// Initialize secrets manager
	useFallback, _ := strconv.ParseBool(os.Getenv("SECRETS_USE_FALLBACK"))
	secretsMgr, err := secrets.NewSecretsManager(secrets.Config{
		AWSRegion:    os.Getenv("AWS_REGION"),
		SecretName:   os.Getenv("AWS_SECRET_NAME"),
		UseCache:     true,
		CacheTTL:     15 * time.Minute,
		UseFallback:  useFallback,
		AWSAccessKey: os.Getenv("AWS_ACCESS_KEY_ID"),
		AWSSecretKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
	})
	if err != nil {
		log.Fatalf("Failed to initialize secrets manager: %v", err)
	}
	defer secretsMgr.Close()

	// Load all secrets
	secretsMap, err := secretsMgr.LoadAllSecrets(ctx)
	if err != nil {
		log.Fatalf("Failed to load secrets: %v", err)
	}

	// Load configuration with secrets
	cfg, err := config.LoadWithSecrets(".env", secretsMap)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	workerCfg := cfg.Workers.Notification

	// Initialize database connection
	db, err := client.NewPgxDB(context.Background(), cfg.Database.ToDBConfig())
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Initialize Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Rcache.Host, cfg.Rcache.Port),
		Username: cfg.Rcache.Username,
		Password: cfg.Rcache.Password,
		DB:       cfg.Rcache.DB,
	})

	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer redisClient.Close()

	// Initialize rate limiter (depends on Redis)
	if err := ratelimiter.Initialize(
		redisClient,
		&cfg.RateLimiter,
		cfg.Cookie.Secure,
		cfg.Cookie.SameSite,
		cfg.Cookie.Domain,
	); err != nil {
		log.Fatalf("Failed to initialize rate limiter: %v", err)
	}

	// Initialize SES client (depends on rate limiter)
	emailClient, err := mailers.NewSESClientFromConfig(cfg.SES)
	if err != nil {
		log.Fatalf("Failed to initialize SES client: %v", err)
	}

	// Create repositories
	eventRepo := notification.NewPgNotificationEventRepository(db)
	userNotifRepo := notification.NewPgUserNotificationRepository(db)
	preferencesRepo := notification.NewPgNotificationPreferencesRepository(db)
	userRepository := userRepo.NewPgUserRepository(db)
	businessRelationRepo := business.NewPgBusinessUserRelationRepository(db)
	txManager := client.NewTxManager(db)

	// Create services
	fanoutService := notificationService.NewFanoutService(
		eventRepo, userNotifRepo, preferencesRepo, userRepository,
		businessRelationRepo, redisClient, txManager, logger.GetDefault(),
	)

	dispatchService := notificationService.NewDispatchService(
		userNotifRepo, userRepository, emailClient, txManager,
		workerCfg.MaxRetryAttempts,
		cfg.SES.NotificationFromEmail,
		cfg.SES.NotificationFromName,
		logger.GetDefault(),
	)

	recoveryService := notificationService.NewRecoveryService(
		userNotifRepo, workerCfg.ProcessingTimeoutMinutes,
		workerCfg.MaxRetryAttempts, logger.GetDefault(),
	)

	// Initialize worker
	worker := workers.NewNotificationWorker(
		fanoutService, dispatchService, recoveryService,
		workers.NotificationWorkerConfig{
			FanoutBatchSize:          workerCfg.FanoutBatchSize,
			FanoutPollInterval:       workerCfg.FanoutPollInterval,
			FanoutWorkerCount:        workerCfg.FanoutWorkers,
			DispatchBatchSize:        workerCfg.DispatchBatchSize,
			DispatchPollInterval:     workerCfg.DispatchPollInterval,
			EmailDispatchWorkerCount: workerCfg.EmailDispatchWorkers,
			RecoveryWorkerCount:      workerCfg.RecoveryWorkers,
			RecoveryIntervalSeconds:  workerCfg.RecoveryIntervalSeconds,
		},
	)

	fmt.Printf("✅ Notification worker initialized | Fanout: %dw/%ds | Dispatch: %dw/%ds | Recovery: %dw/%ds\n",
		workerCfg.FanoutWorkers, workerCfg.FanoutPollInterval,
		workerCfg.EmailDispatchWorkers, workerCfg.DispatchPollInterval,
		workerCfg.RecoveryWorkers, workerCfg.RecoveryIntervalSeconds)

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Listen for interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start worker pools
	go func() {
		if err := worker.RunFanout(ctx); err != nil {
			log.Printf("Fanout worker error: %v", err)
		}
	}()

	go func() {
		if err := worker.RunDispatch(ctx); err != nil {
			log.Printf("Dispatch worker error: %v", err)
		}
	}()

	go func() {
		if err := worker.RunRecovery(ctx); err != nil {
			log.Printf("Recovery worker error: %v", err)
		}
	}()

	// Wait for interrupt signal
	sig := <-sigChan
	fmt.Printf("\n🛑 Received %v, shutting down...\n", sig)
	cancel()
	time.Sleep(2 * time.Second)
	fmt.Println("✅ Worker stopped")

}
