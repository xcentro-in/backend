package main

import (
	"context"
	"fmt"
	"log"

	"xcentro.in/internal/adapters/database/postgres/client"
	payoutRepo "xcentro.in/internal/adapters/database/postgres/payout"
	voucherRepo "xcentro.in/internal/adapters/database/postgres/voucher"
	"xcentro.in/internal/adapters/external/payout/factory"
	"xcentro.in/internal/shared/config"
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

	// Create repositories
	voucherRepository := voucherRepo.NewPostgresVoucherRepository(db)
	transferRepository := payoutRepo.NewPostgresPayoutTransferRepository(db)
	configRepository := payoutRepo.NewPostgresPayoutConfigurationRepository(db)
	mappingRepository := payoutRepo.NewPostgresProviderMappingRepository(db)

	fmt.Println("✅ Repositories initialized")

	// Create payout provider factory with Castler config from environment
	providerFactory := factory.NewPayoutProviderFactory(&cfg)

	fmt.Println("✅ Payout provider factory initialized (using config: Castler)")

	// TODO-WORKER-INIT: Uncomment worker initialization after review
	// TODO-WORKER-INIT: This requires review of:
	// TODO-WORKER-INIT:   - PayoutService constructor parameters
	// TODO-WORKER-INIT:   - PayoutWorker constructor parameters
	// TODO-WORKER-INIT:   - Worker task scheduling strategy (cron vs timer)
	//
	// payoutSvc := payoutService.NewPayoutService(
	// 	voucherRepository,
	// 	transferRepository,
	// 	mappingRepository,
	// 	configRepository,
	// 	providerFactory,
	// )
	//
	// worker := workers.NewPayoutWorker(
	// 	voucherRepository,
	// 	transferRepository,
	// 	mappingRepository,
	// 	configRepository,
	// 	payoutSvc,
	// 	providerFactory,
	// )
	//
	// TODO-SCHEDULER: Add worker scheduler here
	// TODO-SCHEDULER: Options:
	// TODO-SCHEDULER:   1. Cron-based (e.g., every 1 minute)
	// TODO-SCHEDULER:   2. Timer-based (e.g., time.Ticker)
	// TODO-SCHEDULER:   3. Event-driven (e.g., listen to voucher status changes)
	// TODO-SCHEDULER:
	// TODO-SCHEDULER: Example:
	// TODO-SCHEDULER:   ticker := time.NewTicker(1 * time.Minute)
	// TODO-SCHEDULER:   defer ticker.Stop()
	// TODO-SCHEDULER:   for range ticker.C {
	// TODO-SCHEDULER:       worker.ProcessReadyVouchers()
	// TODO-SCHEDULER:       worker.InitiateApprovedTransfers()
	// TODO-SCHEDULER:       worker.FlagTimeoutTransfers()
	// TODO-SCHEDULER:   }

	fmt.Println("\n✅ Payout worker binary ready")
	fmt.Println("✅ All repositories implemented and initialized")
	fmt.Println("✅ Castler configuration wired from environment")

	fmt.Println("\n📦 Worker structure:")
	fmt.Println("  ✅ Worker logic: internal/workers/payout_worker.go")
	fmt.Println("  ✅ Entry point: cmd/payout_worker/main.go")
	fmt.Println("  ✅ Service layer: internal/services/payout/payout_service.go")
	fmt.Println("  ✅ Repositories: transfer, configuration, mapping, voucher")
	fmt.Println("  ✅ Provider factory: Castler client initialized")

	fmt.Println("\n⏳ Pending (requires review before uncommenting):")
	fmt.Println("  - Worker initialization (see TODO-WORKER-INIT comments)")
	fmt.Println("  - Scheduler implementation (see TODO-SCHEDULER comments)")

	fmt.Println("\n💡 Once worker is initialized, it will:")
	fmt.Println("  1. Process ready vouchers (create transfer requests)")
	fmt.Println("  2. Initiate approved transfers (call Castler API)")
	fmt.Println("  3. Flag timeout transfers (no webhook after 60min)")
	fmt.Println("  4. Handle failed transfers (placeholder for retry strategy)")

	// Keep references to avoid unused variable warnings
	_ = voucherRepository
	_ = transferRepository
	_ = configRepository
	_ = mappingRepository
	_ = providerFactory
}
