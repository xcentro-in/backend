package notification

import (
	"context"
	"fmt"

	"xcentro.in/internal/core/notification"
	"xcentro.in/internal/shared/logger"
)

// RecoveryService handles cleanup and recovery of stuck/failed notification jobs
// This service runs periodically to:
// 1. Reset stuck "processing" jobs back to "pending" (worker crash recovery)
// 2. Delete permanently failed jobs that exceeded max retry attempts (cleanup)
type RecoveryService struct {
	userNotifRepo        notification.UserNotificationRepository
	logger               *logger.Logger
	processingTimeoutMin int
	maxRetryAttempts     int
}

// NewRecoveryService creates a new recovery service
func NewRecoveryService(
	userNotifRepo notification.UserNotificationRepository,
	processingTimeoutMin int,
	maxRetryAttempts int,
	logger *logger.Logger,
) *RecoveryService {
	return &RecoveryService{
		userNotifRepo:        userNotifRepo,
		logger:               logger,
		processingTimeoutMin: processingTimeoutMin,
		maxRetryAttempts:     maxRetryAttempts,
	}
}

// ProcessRecoveryAndCleanup performs stuck job recovery and cleanup
// Returns (recoveredCount, deletedCount, error)
func (s *RecoveryService) ProcessRecoveryAndCleanup(ctx context.Context) (int64, int64, error) {
	// Step 1: Recover stuck processing jobs (reset to pending)
	recoveredCount, err := s.userNotifRepo.RecoverStuckProcessingJobs(ctx, s.processingTimeoutMin)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to recover stuck processing jobs: %w", err)
	}

	if recoveredCount > 0 {
		s.logger.Infof("✅ Recovery: Reset %d stuck processing jobs to pending", recoveredCount)
	}

	// Step 2: Delete permanently failed email jobs (exceeded max retries)
	deletedEmailCount, err := s.userNotifRepo.DeletePermanentlyFailedJobs(ctx, notification.ChannelEmail, s.maxRetryAttempts)
	if err != nil {
		return recoveredCount, 0, fmt.Errorf("failed to delete permanently failed email jobs: %w", err)
	}

	if deletedEmailCount > 0 {
		s.logger.Infof("🗑️  Cleanup: Deleted %d permanently failed email jobs", deletedEmailCount)
	}

	// Note: In future, add push channel cleanup when push is implemented

	return recoveredCount, deletedEmailCount, nil
}
