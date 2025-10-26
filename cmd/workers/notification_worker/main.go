package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/adapters/database/postgres/notification"
	"xcentro.in/internal/adapters/providers/mailers"
	notificationService "xcentro.in/internal/services/notification"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/workers"
)

func main() {
	fmt.Println("🚀 Starting Notification Worker...")

	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	fmt.Printf("📝 Loaded configuration\n")

	// Get worker configuration
	batchSize := cfg.Workers.Notification.BatchSize
	pollInterval := cfg.Workers.Notification.PollInterval
	maxRetries := cfg.Workers.Notification.MaxRetries

	fmt.Printf("⚙️  Worker config: batch_size=%d, poll_interval=%ds, max_retries=%d\n", batchSize, pollInterval, maxRetries)

	// Initialize database connection
	db, err := client.NewPgxDB(context.Background(), cfg.Database.ToDBConfig())
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	fmt.Println("✅ Database connection established")

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

	fmt.Println("✅ Repositories initialized")

	// Create transaction manager
	txManager := client.NewTxManager(db)

	// Create notification service (orchestrates business logic and transactions)
	svc := notificationService.NewNotificationService(
		eventRepo,
		userNotifRepo,
		preferencesRepo,
		emailClient,
		txManager,
		db,
		logger.GetDefault(),
	)

	fmt.Println("✅ Notification service initialized")

	// Initialize notification worker (minimal - just calls service)
	worker := workers.NewNotificationWorker(svc, batchSize, maxRetries)

	fmt.Println("✅ Notification worker initialized")

	fmt.Println("\n📦 Worker structure:")
	fmt.Println("  ✅ Worker logic: internal/workers/notification_worker.go (minimal)")
	fmt.Println("  ✅ Service logic: internal/services/notification/notification_service.go (orchestrates)")
	fmt.Println("  ✅ Entry point: cmd/workers/notification_worker/main.go")
	fmt.Println("  ✅ Repositories: notification_events, user_notifications, preferences")
	fmt.Println("  ✅ Email client: AWS SES")

	fmt.Println("\n💡 Service responsibilities:")
	fmt.Println("  1. Begin transaction")
	fmt.Println("  2. Fetch pending notification events (FOR UPDATE SKIP LOCKED)")
	fmt.Println("  3. Resolve target users (unicast/multicast/broadcast)")
	fmt.Println("  4. Create user notifications (single entry per user)")
	fmt.Println("  5. Mark events as processed")
	fmt.Println("  6. Commit transaction")
	fmt.Println("  7. Send emails/push AFTER transaction (outside DB tx)")
	fmt.Println("  8. Retry with exponential backoff")

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Listen for interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start worker scheduler
	ticker := time.NewTicker(time.Duration(pollInterval) * time.Second)
	defer ticker.Stop()

	fmt.Printf("\n🔄 Worker scheduler started (polls every %ds)\n", pollInterval)
	fmt.Println("⏸️  Press Ctrl+C to stop gracefully")

	// Run worker immediately on startup
	fmt.Println("🏃 Running initial worker cycle...")
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
			fmt.Println("✅ Worker stopped")
			return
		}
	}
}
