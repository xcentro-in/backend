package workers

import (
	"context"
	"fmt"

	auditService "xcentro.in/internal/services/audit"
)

// AuditWorker processes audit events from the queue
// PHASE2_TODO: Add redisClient field for push-based processing (BLPOP instead of polling)
type AuditWorker struct {
	auditService *auditService.AuditService
	batchSize    int
	// PHASE2_TODO: Add field: redisClient *redis.Client
}

// NewAuditWorker creates a new audit worker
func NewAuditWorker(
	auditService *auditService.AuditService,
	batchSize int,
) *AuditWorker {
	return &AuditWorker{
		auditService: auditService,
		batchSize:    batchSize,
	}
}

// Run executes the worker cycle
// Worker is kept minimal - all business logic is in the service
// PHASE2_TODO: This will be called when an event ID arrives via Redis BLPOP
func (w *AuditWorker) Run(ctx context.Context) error {
	fmt.Println("=== AUDIT WORKER: Starting run ===")

	// Call service to process pending events (service handles transaction, transformation, etc.)
	processedCount, err := w.auditService.ProcessPendingAuditEvents(ctx, w.batchSize)
	if err != nil {
		fmt.Printf("❌ Error processing audit events: %v\n", err)
		return fmt.Errorf("failed to process audit events: %w", err)
	}

	if processedCount == 0 {
		fmt.Println("ℹ️  No pending audit events")
	} else {
		fmt.Printf("✅ Successfully processed %d audit events\n", processedCount)
	}

	fmt.Println("=== AUDIT WORKER: Run complete ===")
	return nil
}
