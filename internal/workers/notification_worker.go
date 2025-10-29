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

	fanoutBatchSize          int
	fanoutPollInterval       int
	fanoutWorkerCount        int
	dispatchBatchSize        int
	dispatchPollInterval     int
	emailDispatchWorkerCount int
	recoveryWorkerCount      int
	recoveryIntervalSeconds  int
}

// NotificationWorkerConfig holds worker configuration
type NotificationWorkerConfig struct {
	FanoutBatchSize          int
	FanoutPollInterval       int
	FanoutWorkerCount        int
	DispatchBatchSize        int
	DispatchPollInterval     int
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
		fanoutBatchSize:          config.FanoutBatchSize,
		fanoutPollInterval:       config.FanoutPollInterval,
		fanoutWorkerCount:        config.FanoutWorkerCount,
		dispatchBatchSize:        config.DispatchBatchSize,
		dispatchPollInterval:     config.DispatchPollInterval,
		emailDispatchWorkerCount: config.EmailDispatchWorkerCount,
		recoveryWorkerCount:      config.RecoveryWorkerCount,
		recoveryIntervalSeconds:  config.RecoveryIntervalSeconds,
	}
}

// RunFanout executes fanout worker pool with its own poll interval
func (w *NotificationWorker) RunFanout(ctx context.Context) error {
	ticker := time.NewTicker(time.Duration(w.fanoutPollInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-ticker.C:
			var wg sync.WaitGroup
			for i := 0; i < w.fanoutWorkerCount; i++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()
					processedCount, err := w.fanoutService.ProcessPendingEvents(ctx, w.fanoutBatchSize)
					if err != nil {
						fmt.Printf("Fanout worker %d error: %v\n", workerID, err)
					} else if processedCount > 0 {
						fmt.Printf("Fanout worker %d: processed %d events\n", workerID, processedCount)
					}
				}(i + 1)
			}
			wg.Wait()
		}
	}
}

// RunDispatch executes dispatch worker pool with its own poll interval
func (w *NotificationWorker) RunDispatch(ctx context.Context) error {
	ticker := time.NewTicker(time.Duration(w.dispatchPollInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-ticker.C:
			var wg sync.WaitGroup
			for i := 0; i < w.emailDispatchWorkerCount; i++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()
					sentCount, err := w.dispatchService.ProcessEmailDispatches(ctx, w.dispatchBatchSize)
					if err != nil {
						fmt.Printf("Dispatch worker %d error: %v\n", workerID, err)
					} else if sentCount > 0 {
						fmt.Printf("Dispatch worker %d: sent %d emails\n", workerID, sentCount)
					}
				}(i + 1)
			}
			wg.Wait()
		}
	}
}

// RunRecovery executes recovery goroutine pool in a separate ticker loop
// This runs less frequently than the main worker (every recoveryIntervalSeconds)
func (w *NotificationWorker) RunRecovery(ctx context.Context) error {
	ticker := time.NewTicker(time.Duration(w.recoveryIntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-ticker.C:
			var wg sync.WaitGroup
			for i := 0; i < w.recoveryWorkerCount; i++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()
					recovered, deleted, err := w.recoveryService.ProcessRecoveryAndCleanup(ctx)
					if err != nil {
						fmt.Printf("Recovery worker %d error: %v\n", workerID, err)
					} else if recovered > 0 || deleted > 0 {
						fmt.Printf("Recovery worker %d: recovered %d, deleted %d\n", workerID, recovered, deleted)
					}
				}(i + 1)
			}
			wg.Wait()
		}
	}
}
