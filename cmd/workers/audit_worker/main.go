package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"xcentro.in/internal/adapters/database/postgres/audit"
	"xcentro.in/internal/adapters/database/postgres/client"
	auditService "xcentro.in/internal/services/audit"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/workers"
)

func main() {
	fmt.Println("🚀 Starting Audit Worker...")

	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	fmt.Printf("📝 Loaded configuration\n")

	// Get worker configuration
	batchSize := cfg.Workers.Audit.BatchSize
	pollInterval := cfg.Workers.Audit.PollInterval

	fmt.Printf("⚙️  Worker config: batch_size=%d, poll_interval=%ds\n", batchSize, pollInterval)

	// Initialize database connection
	db, err := client.NewPgxDB(context.Background(), cfg.Database.ToDBConfig())
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	fmt.Println("✅ Database connection established")

	// Create repositories
	eventRepo := audit.NewPgAuditEventRepository(db)
	trailRepo := audit.NewPgAuditTrailRepository(db)
	entityRepo := audit.NewPgAuditTrailEntityRepository(db)

	fmt.Println("✅ Repositories initialized")

	// Create transaction manager
	txManager := client.NewTxManager(db)

	// Create audit service (orchestrates business logic and transactions)
	svc := auditService.NewAuditService(eventRepo, trailRepo, entityRepo, txManager, logger.GetDefault())

	fmt.Println("✅ Audit service initialized")

	// Initialize audit worker (minimal - just calls service)
	worker := workers.NewAuditWorker(svc, batchSize)

	fmt.Println("✅ Audit worker initialized")

	fmt.Println("\n📦 Worker structure:")
	fmt.Println("  ✅ Worker logic: internal/workers/audit_worker.go (minimal)")
	fmt.Println("  ✅ Service logic: internal/services/audit/audit_service.go (orchestrates)")
	fmt.Println("  ✅ Entry point: cmd/workers/audit_worker/main.go")
	fmt.Println("  ✅ Repositories: audit_events, audit_trail")

	fmt.Println("\n💡 Service responsibilities:")
	fmt.Println("  1. Begin transaction")
	fmt.Println("  2. Fetch pending audit events (FOR UPDATE SKIP LOCKED)")
	fmt.Println("  3. Transform events to audit trail entries")
	fmt.Println("  4. Batch insert into audit_trail")
	fmt.Println("  5. Mark events as processed")
	fmt.Println("  6. Commit transaction (or rollback on error)")

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
