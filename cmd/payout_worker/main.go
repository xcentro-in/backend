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
	// TODO: Create these repository implementations
	// transferRepository := payoutRepo.NewPostgresTransferRepository(db)
	// configRepository := payoutRepo.NewPostgresConfigurationRepository(db)
	mappingRepository := payoutRepo.NewPostgresProviderMappingRepository(db)

	// Create payout provider factory
	providerFactory := factory.NewPayoutProviderFactory(&factory.PayoutProviderConfig{
		CastlerBaseURL:   "https://api.castler.in", // TODO: Add to config
		CastlerAPIKey:    "dummy-key",              // TODO: Add to config
		CastlerSecretKey: "dummy-secret",           // TODO: Add to config
		DefaultProvider:  "castler",
	})

	// TODO: Complete worker initialization once repository implementations are ready
	// payoutSvc := payoutService.NewPayoutService(
	// 	voucherRepository,
	// 	transferRepository,
	// 	mappingRepository,
	// 	configRepository,
	// 	providerFactory,
	// )

	// worker := workers.NewPayoutWorker(
	// 	voucherRepository,
	// 	transferRepository,
	// 	mappingRepository,
	// 	configRepository,
	// 	payoutSvc,
	// 	providerFactory,
	// )

	fmt.Println("\n✅ Payout worker binary ready (repositories pending implementation)")
	fmt.Println("TODO: Implement NewPostgresTransferRepository in internal/adapters/database/postgres/payout/")
	fmt.Println("TODO: Implement NewPostgresConfigurationRepository in internal/adapters/database/postgres/payout/")
	fmt.Println("TODO: Add Castler config to config.Config struct")
	fmt.Println("TODO: Complete worker initialization and scheduler")

	// For now, just demonstrate the structure is in place
	fmt.Println("\n📦 Worker structure implemented:")
	fmt.Println("  - Worker logic: internal/workers/payout_worker.go")
	fmt.Println("  - Entry point: cmd/payout_worker/main.go")
	fmt.Println("  - Service layer: internal/services/payout/payout_service.go")
	fmt.Println("\n💡 Once repositories are implemented, worker will:")
	fmt.Println("  1. Process ready vouchers (create transfer requests)")
	fmt.Println("  2. Initiate approved transfers (call Castler API)")
	fmt.Println("  3. Flag timeout transfers (no webhook after 60min)")
	fmt.Println("  4. Handle failed transfers (placeholder for retry strategy)")

	_ = voucherRepository
	_ = mappingRepository
	_ = providerFactory
}
