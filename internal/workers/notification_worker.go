package workers

import (
	"context"
	"fmt"
	"sync"
	"time"

	"xcentro.in/internal/core/notification"
)

// NotificationWorker processes notification events using multiple goroutine pools:
// 1. Fanout pool: notification_events -> user_notifications (fan-out)
// 2. Email dispatch pool: user_notifications (email channel) -> AWS SES (dispatch)
// 3. Recovery pool: stuck processing jobs -> pending, permanently failed -> delete
type NotificationWorker struct {
	fanoutService   notification.FanoutService
	dispatchService notification.DispatchService
	recoveryService notification.RecoveryService

	batchSize                int
	fanoutWorkerCount        int
	emailDispatchWorkerCount int
	recoveryWorkerCount      int
	recoveryIntervalSeconds  int
}

// NotificationWorkerConfig holds worker configuration
type NotificationWorkerConfig struct {
	BatchSize                int
	FanoutWorkerCount        int
	EmailDispatchWorkerCount int
	RecoveryWorkerCount      int
	RecoveryIntervalSeconds  int
}

// NewNotificationWorker creates a new notification worker with configurable goroutine pools
func NewNotificationWorker(
	fanoutService notification.FanoutService,
	dispatchService notification.DispatchService,
	recoveryService notification.RecoveryService,
	config NotificationWorkerConfig,
) *NotificationWorker {
	return &NotificationWorker{
		fanoutService:            fanoutService,
		dispatchService:          dispatchService,
		recoveryService:          recoveryService,
		batchSize:                config.BatchSize,
		fanoutWorkerCount:        config.FanoutWorkerCount,
		emailDispatchWorkerCount: config.EmailDispatchWorkerCount,
		recoveryWorkerCount:      config.RecoveryWorkerCount,
		recoveryIntervalSeconds:  config.RecoveryIntervalSeconds,
	}
}

// Run executes all worker goroutine pools concurrently
// Worker is kept minimal - all business logic is in the services
func (w *NotificationWorker) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	errChan := make(chan error, w.fanoutWorkerCount+w.emailDispatchWorkerCount)

	// Goroutine Pool 1: Fanout workers (notification_events -> user_notifications)
	for i := 0; i < w.fanoutWorkerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			processedCount, err := w.fanoutService.ProcessPendingEvents(ctx, w.batchSize)
			if err != nil {
				errChan <- fmt.Errorf("fanout worker %d error: %w", workerID, err)
			} else if processedCount > 0 {
				fmt.Printf("✅ Fanout worker %d: processed %d events\n", workerID, processedCount)
			}
		}(i + 1)
	}

	// Goroutine Pool 2: Email dispatch workers (user_notifications [email] -> SES)
	for i := 0; i < w.emailDispatchWorkerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			sentCount, err := w.dispatchService.ProcessEmailDispatches(ctx, w.batchSize)
			if err != nil {
				errChan <- fmt.Errorf("email dispatch worker %d error: %w", workerID, err)
			} else if sentCount > 0 {
				fmt.Printf("✅ Email dispatch worker %d: sent %d emails\n", workerID, sentCount)
			}
		}(i + 1)
	}

	// Wait for all workers to complete
	wg.Wait()
	close(errChan)

	// Collect and report errors
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("worker errors: %v", errs)
	}

	return nil
}

// RunRecovery executes recovery goroutine pool in a separate ticker loop
// This runs less frequently than the main worker (every recoveryIntervalSeconds)
func (w *NotificationWorker) RunRecovery(ctx context.Context) error {
	ticker := time.NewTicker(time.Duration(w.recoveryIntervalSeconds) * time.Second)
	defer ticker.Stop()

	fmt.Printf("🔄 Recovery worker started (interval: %ds, workers: %d)\n", w.recoveryIntervalSeconds, w.recoveryWorkerCount)

	for {
		select {
		case <-ctx.Done():
			fmt.Println("🛑 Recovery worker stopped")
			return nil

		case <-ticker.C:
			// Run recovery workers concurrently
			var wg sync.WaitGroup
			for i := 0; i < w.recoveryWorkerCount; i++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()
					recovered, deleted, err := w.recoveryService.ProcessRecoveryAndCleanup(ctx)
					if err != nil {
						fmt.Printf("❌ Recovery worker %d error: %v\n", workerID, err)
					} else if recovered > 0 || deleted > 0 {
						fmt.Printf("🔄 Recovery worker %d: recovered %d, deleted %d\n", workerID, recovered, deleted)
					}
				}(i + 1)
			}
			wg.Wait()
		}
	}
}
