package workers

import (
	"context"
	"fmt"

	notificationService "xcentro.in/internal/services/notification"
)

// NotificationWorker processes notification events from the queue
// PHASE2_TODO: Add redisClient field for push-based processing (BLPOP instead of polling)
type NotificationWorker struct {
	notificationService *notificationService.NotificationService
	batchSize           int
	maxRetries          int
	// PHASE2_TODO: Add field: redisClient *redis.Client
	// PHASE2_TODO: Add field: pushNotificationClient PushNotificationService (FCM/APNS)
}

// NewNotificationWorker creates a new notification worker
func NewNotificationWorker(
	notificationService *notificationService.NotificationService,
	batchSize int,
	maxRetries int,
) *NotificationWorker {
	return &NotificationWorker{
		notificationService: notificationService,
		batchSize:           batchSize,
		maxRetries:          maxRetries,
	}
}

// Run executes the worker cycle
// Worker is kept minimal - all business logic is in the service
// PHASE2_TODO: This will be called when an event ID arrives via Redis BLPOP
func (w *NotificationWorker) Run(ctx context.Context) error {
	fmt.Println("=== NOTIFICATION WORKER: Starting run ===")

	// Call service to process pending events (service handles transaction, user resolution, sending, etc.)
	processedCount, err := w.notificationService.ProcessPendingNotificationEvents(ctx, w.batchSize, w.maxRetries)
	if err != nil {
		fmt.Printf("❌ Error processing notification events: %v\n", err)
		return fmt.Errorf("failed to process notification events: %w", err)
	}

	if processedCount == 0 {
		fmt.Println("ℹ️  No pending notification events")
	} else {
		fmt.Printf("✅ Successfully processed %d notification events\n", processedCount)
	}

	fmt.Println("=== NOTIFICATION WORKER: Run complete ===")
	return nil
}
