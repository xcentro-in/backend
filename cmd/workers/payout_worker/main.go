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
	paymentRepo "xcentro.in/internal/adapters/database/postgres/payment"
	payoutRepo "xcentro.in/internal/adapters/database/postgres/payout"
	voucherRepo "xcentro.in/internal/adapters/database/postgres/voucher"
	"xcentro.in/internal/adapters/external/payout/factory"
	"xcentro.in/internal/services/payout"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/workers"
)

func main() {
	fmt.Println("🚀 Starting Payout Worker...")

	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	fmt.Printf("📝 Loaded configuration\n")

	// Initialize database connection
	db, err := client.NewPgxDB(context.Background(), cfg.Database.ToDBConfig())
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	fmt.Println("✅ Database connection established")

	// Create transaction manager
	txManager := client.NewTransactionManager(db)

	// Create repositories
	paymentRepository := paymentRepo.NewPostgresPaymentRepository(db, txManager)
	voucherRepository := voucherRepo.NewPostgresVoucherRepository(db, cfg.BankAccountEncryption.EncryptionKey)
	transferRepository := payoutRepo.NewPostgresPayoutTransferRepository(db)
	configRepository := payoutRepo.NewPostgresPayoutConfigurationRepository(db)
	stateChangeRepository := paymentRepo.NewPostgresPaymentStateChangeRepository(db)

	fmt.Println("✅ Repositories initialized")

	// Create payout provider factory with Castler config from environment
	providerFactory := factory.NewPayoutProviderFactory(&cfg)

	fmt.Println("✅ Payout provider factory initialized (using config: Castler)")

	// Initialize payout service (business logic layer)
	// Note: Worker uses paymentRepository for settlement status checks
	payoutSvc := payout.NewPayoutService(
		paymentRepository,
		voucherRepository,
		transferRepository,
		nil, // bankAccountRepository - not used in worker, only in API handlers
		configRepository,
		providerFactory,
	)

	fmt.Println("✅ Payout service initialized")

	// Initialize payout worker (orchestration/scheduling layer)
	worker := workers.NewPayoutWorker(
		voucherRepository,
		transferRepository,
		configRepository,
		stateChangeRepository,
		payoutSvc,
		providerFactory,
	)

	fmt.Println("✅ Payout worker initialized")

	fmt.Println("\n📦 Worker structure:")
	fmt.Println("  ✅ Worker logic: internal/workers/payout_worker.go")
	fmt.Println("  ✅ Entry point: cmd/payout_worker/main.go")
	fmt.Println("  ✅ Service layer: internal/services/payout/payout_service.go")
	fmt.Println("  ✅ Repositories: transfer, configuration, mapping, voucher")
	fmt.Println("  ✅ Provider factory: Castler client initialized")

	fmt.Println("\n💡 Worker tasks:")
	fmt.Println("  1. Process ready vouchers (create transfer requests)")
	fmt.Println("  2. Initiate approved transfers (call Castler API)")
	fmt.Println("  3. Flag timeout transfers (no webhook after 60min)")
	fmt.Println("  4. Handle failed transfers (placeholder for retry strategy)")

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Listen for interrupt signals (Ctrl+C, SIGTERM)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start worker scheduler (simple ticker - runs every 1 minute)
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	fmt.Println("\n🔄 Worker scheduler started (runs every 1 minute)")
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
			// Execute worker tasks every minute
			if err := worker.Run(ctx); err != nil {
				log.Printf("⚠️  Worker run failed: %v", err)
			}

		case sig := <-sigChan:
			// Graceful shutdown
			fmt.Printf("\n\n🛑 Received signal: %v\n", sig)
			fmt.Println("🔄 Shutting down worker gracefully...")
			cancel()
			fmt.Println("✅ Worker stopped")
			return
		}
	}
}
