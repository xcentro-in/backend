package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"xcentro.in/internal/adapters/database/postgres/business"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/adapters/database/postgres/notification"
	userRepo "xcentro.in/internal/adapters/database/postgres/user"
	"xcentro.in/internal/adapters/providers/mailers"
	notificationService "xcentro.in/internal/services/notification"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/workers"
)

func main() {
	fmt.Println("🚀 Starting Notification Worker (Multi-Pool Architecture)...")

	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	fmt.Printf("📝 Loaded configuration\n")

	// Get worker configuration
	workerCfg := cfg.Workers.Notification

	fmt.Printf("⚙️  Worker configuration:\n")
	fmt.Printf("  - Batch size: %d\n", workerCfg.BatchSize)
	fmt.Printf("  - Poll interval: %ds\n", workerCfg.PollInterval)
	fmt.Printf("  - Max retry attempts: %d\n", workerCfg.MaxRetryAttempts)
	fmt.Printf("  - Fanout workers: %d\n", workerCfg.FanoutWorkers)
	fmt.Printf("  - Email dispatch workers: %d\n", workerCfg.EmailDispatchWorkers)
	fmt.Printf("  - Recovery workers: %d\n", workerCfg.RecoveryWorkers)
	fmt.Printf("  - Recovery interval: %ds\n", workerCfg.RecoveryIntervalSeconds)
	fmt.Printf("  - Processing timeout: %d min\n", workerCfg.ProcessingTimeoutMinutes)

	// Initialize database connection
	db, err := client.NewPgxDB(context.Background(), cfg.Database.ToDBConfig())
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	fmt.Println("✅ Database connection established")

	// Initialize Redis client for caching
	redisClient := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Rcache.Host, cfg.Rcache.Port),
		Username: cfg.Rcache.Username,
		Password: cfg.Rcache.Password,
		DB:       cfg.Rcache.DB,
	})

	// Test Redis connection
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer redisClient.Close()

	fmt.Println("✅ Redis connection established")

	// Initialize SES client for email notifications
	emailClient, err := mailers.NewSESClientFromConfig(cfg.SES)
	if err != nil {
		log.Fatalf("Failed to initialize SES client: %v", err)
	}

	fmt.Println("✅ SES email client initialized")

	// Create repositories
	eventRepo := notification.NewPgNotificationEventRepository(db)
	userNotifRepo := notification.NewPgUserNotificationRepository(db)
	preferencesRepo := notification.NewPgNotificationPreferencesRepository(db)
	userRepository := userRepo.NewPgUserRepository(db)
	businessRelationRepo := business.NewPgBusinessUserRelationRepository(db)

	fmt.Println("✅ Repositories initialized")

	// Create transaction manager
	txManager := client.NewTxManager(db)

	// Create services
	// 1. Fanout service (notification_events -> user_notifications)
	fanoutService := notificationService.NewFanoutService(
		eventRepo,
		userNotifRepo,
		preferencesRepo,
		userRepository,
		businessRelationRepo,
		redisClient,
		txManager,
		logger.GetDefault(),
	)

	// 2. Dispatch service (user_notifications [email] -> SES)
	dispatchService := notificationService.NewDispatchService(
		userNotifRepo,
		userRepository,
		emailClient,
		txManager,
		workerCfg.MaxRetryAttempts,
		logger.GetDefault(),
	)

	// 3. Recovery service (stuck/failed job cleanup)
	recoveryService := notificationService.NewRecoveryService(
		userNotifRepo,
		workerCfg.ProcessingTimeoutMinutes,
		workerCfg.MaxRetryAttempts,
		logger.GetDefault(),
	)

	fmt.Println("✅ Services initialized (Fanout + Dispatch + Recovery)")

	// Initialize notification worker (multi-pool architecture)
	worker := workers.NewNotificationWorker(
		fanoutService,
		dispatchService,
		recoveryService,
		workers.NotificationWorkerConfig{
			BatchSize:                workerCfg.BatchSize,
			FanoutWorkerCount:        workerCfg.FanoutWorkers,
			EmailDispatchWorkerCount: workerCfg.EmailDispatchWorkers,
			RecoveryWorkerCount:      workerCfg.RecoveryWorkers,
			RecoveryIntervalSeconds:  workerCfg.RecoveryIntervalSeconds,
		},
	)

	fmt.Println("✅ Notification worker initialized")

	fmt.Println("\n📦 Worker architecture:")
	fmt.Println("  ✅ Worker: internal/workers/notification_worker.go (multi-pool)")
	fmt.Println("  ✅ Fanout Service: internal/services/notification/fanout_service.go")
	fmt.Println("  ✅ Dispatch Service: internal/services/notification/dispatch_service.go")
	fmt.Println("  ✅ Recovery Service: internal/services/notification/recovery_service.go")
	fmt.Println("  ✅ Entry point: cmd/workers/notification_worker/main.go")

	fmt.Println("\n💡 Goroutine Pool 1 (Fanout) responsibilities:")
	fmt.Println("  1. Fetch pending notification_events (FOR UPDATE SKIP LOCKED)")
	fmt.Println("  2. Resolve target users (unicast/multicast/broadcast)")
	fmt.Println("  3. Get user preferences (Redis cached)")
	fmt.Println("  4. Create user_notifications rows (one per enabled channel)")
	fmt.Println("  5. Mark events as processed")
	fmt.Println("  6. Commit transaction")

	fmt.Println("\n💡 Goroutine Pool 2 (Email Dispatch) responsibilities:")
	fmt.Println("  1. Fetch pending/failed user_notifications (channel=email, retry_count < max)")
	fmt.Println("  2. Atomically mark as 'processing' and increment retry_count")
	fmt.Println("  3. Send emails via AWS SES (OUTSIDE transaction)")
	fmt.Println("  4. Update job statuses (sent/failed)")
	fmt.Println("  5. Commit transaction")

	fmt.Println("\n💡 Goroutine Pool 3 (Recovery) responsibilities:")
	fmt.Println("  1. Reset stuck 'processing' jobs to 'pending' (timeout-based)")
	fmt.Println("  2. Delete permanently failed jobs (retry_count >= max)")
	fmt.Println("  3. Runs on separate interval (less frequently)")

	fmt.Println("\n🔑 Key design principles:")
	fmt.Println("  ✅ Preferences cached in Redis (1 hour TTL + explicit invalidation)")
	fmt.Println("  ✅ All DB operations in transactions")
	fmt.Println("  ✅ Email sending OUTSIDE transactions")
	fmt.Println("  ✅ Atomic job claiming with 'processing' status (race condition prevention)")
	fmt.Println("  ✅ Retry logic for failed jobs (configurable max attempts)")
	fmt.Println("  ✅ Stuck job recovery (worker crash handling)")
	fmt.Println("  ✅ Read status only for web channel (validation enforced)")

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Listen for interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start recovery worker in background goroutine
	go func() {
		if err := worker.RunRecovery(ctx); err != nil {
			log.Printf("⚠️  Recovery worker error: %v", err)
		}
	}()

	// Start main worker scheduler
	ticker := time.NewTicker(time.Duration(workerCfg.PollInterval) * time.Second)
	defer ticker.Stop()

	fmt.Printf("\n🔄 Worker scheduler started (polls every %ds)\n", workerCfg.PollInterval)
	fmt.Println("⏸️  Press Ctrl+C to stop gracefully")

	// Run worker immediately on startup
	fmt.Println("\n🏃 Running initial worker cycle...")
	if err := worker.Run(ctx); err != nil {
		log.Printf("⚠️  Initial worker run failed: %v", err)
	}

	// Main event loop
	for {
		select {
		case <-ticker.C:
			if err := worker.Run(ctx); err != nil {
				log.Printf("⚠️  Worker run failed: %v", err)
			}

		case sig := <-sigChan:
			fmt.Printf("\n\n🛑 Received signal: %v\n", sig)
			fmt.Println("🔄 Shutting down worker gracefully...")
			cancel()
			time.Sleep(1 * time.Second) // Give recovery worker time to stop
			fmt.Println("✅ Worker stopped")
			return
		}
	}
}
