package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"xcentro.in/internal/adapters/database/postgres/client"
	payeeRepo "xcentro.in/internal/adapters/database/postgres/payee"
	paymentRepo "xcentro.in/internal/adapters/database/postgres/payment"
	payoutRepo "xcentro.in/internal/adapters/database/postgres/payout"
	voucherRepo "xcentro.in/internal/adapters/database/postgres/voucher"
	"xcentro.in/internal/adapters/external/payout/factory"
	"xcentro.in/internal/services/payout"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/secrets"
	"xcentro.in/internal/workers"
)

func main() {
	log.Println("Starting Payout Worker...")

	// Parse command line flags
	var envFile = flag.String("env", ".env", "Path to environment file (default: .env)")
	flag.Parse()

	ctx := context.Background()

	// Load .env file into environment variables
	if err := godotenv.Load(*envFile); err != nil {
		log.Printf("Failed to load %s file: %v (will use existing environment variables)", *envFile, err)
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
	cfg, err := config.LoadWithSecrets(*envFile, secretsMap)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Println("Configuration loaded successfully with secrets")

	// Initialize database connection
	db, err := client.NewPgxDB(context.Background(), cfg.Database.ToDBConfig())
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	log.Println("Database connection established")

	// Create repositories
	paymentRepository := paymentRepo.NewPgPaymentRepository(db, cfg.BankAccountEncryption.EncryptionKey)
	voucherRepository := voucherRepo.NewPostgresVoucherRepository(db, cfg.BankAccountEncryption.EncryptionKey)
	transferRepository := payoutRepo.NewPostgresPayoutTransferRepository(db)
	bankAccountRepository := payeeRepo.NewPgPayeeBankAccountRepository(db, cfg.BankAccountEncryption.EncryptionKey)
	stateChangeRepository := paymentRepo.NewPostgresPaymentStateChangeRepository(db)

	log.Println("Repositories initialized")

	// Create payout provider factory with Castler config from environment
	providerFactory := factory.NewPayoutProviderFactory(&cfg)

	log.Println("Payout provider factory initialized (Castler)")

	// Initialize payout service (business logic layer)
	// Note: Worker needs bankAccountRepository to fetch Castler beneficiary IDs for payouts
	payoutSvc := payout.NewPayoutService(
		paymentRepository,
		voucherRepository,
		transferRepository,
		bankAccountRepository,
		stateChangeRepository,
		providerFactory,
		&cfg,
	)

	log.Println("Payout service initialized")

	// Initialize payout worker (orchestration/scheduling layer)
	worker := workers.NewPayoutWorker(
		paymentRepository,
		voucherRepository,
		transferRepository,
		stateChangeRepository,
		payoutSvc,
		providerFactory,
		&cfg,
	)

	log.Println("Payout worker initialized")
	log.Printf("Auto-payout enabled: %v", cfg.Workers.AutoPayout.Enabled)
	if cfg.Workers.AutoPayout.Enabled {
		log.Printf("Auto-payout batch size: %d", cfg.Workers.AutoPayout.BatchSize)
		log.Printf("Auto-payout poll interval: %d minutes", cfg.Workers.AutoPayout.PollIntervalMinutes)
	}

	log.Println("Worker structure:")
	log.Println("  - Worker logic: internal/workers/payout_worker.go")
	log.Println("  - Entry point: cmd/payout_worker/main.go")
	log.Println("  - Service layer: internal/services/payout/payout_service.go")
	log.Println("  - Repositories: payment, transfer, configuration, mapping, voucher")
	log.Println("  - Provider factory: Castler client initialized")

	log.Println("Worker tasks:")
	log.Println("  1. Auto-retry initiated transfers (recover from crashed async goroutines)")
	log.Println("  2. Flag timeout transfers (no webhook after 60min)")
	log.Println("  3. Retry failed transfers (placeholder for future)")
	log.Println("  4. Auto-payout for settled payments (main functionality)")

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Listen for interrupt signals (Ctrl+C, SIGTERM)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start worker scheduler using configured poll interval
	// Note: This controls ALL worker tasks (monitoring, timeout checks, auto-payout)
	pollInterval := time.Duration(cfg.Workers.AutoPayout.PollIntervalMinutes) * time.Minute
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	log.Printf("Worker scheduler started (runs every %d minutes)", cfg.Workers.AutoPayout.PollIntervalMinutes)
	log.Println("Press Ctrl+C to stop gracefully")

	// Run worker immediately on startup
	log.Println("Running initial worker cycle...")
	if err := worker.Run(ctx); err != nil {
		log.Printf("Initial worker run failed: %v", err)
	}

	// Main event loop
	for {
		select {
		case <-ticker.C:
			// Execute worker tasks based on poll interval
			if err := worker.Run(ctx); err != nil {
				log.Printf("Worker run failed: %v", err)
			}

		case sig := <-sigChan:
			// Graceful shutdown
			log.Printf("Received signal: %v", sig)
			log.Println("Shutting down worker gracefully...")
			cancel()
			log.Println("Worker stopped")
			return
		}
	}
}
