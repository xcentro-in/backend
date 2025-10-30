package notification

import (
	"context"
	"fmt"

	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/adapters/providers/mailers"
	"xcentro.in/internal/core/notification"
	"xcentro.in/internal/core/user"
	"xcentro.in/internal/shared/logger"
)

// DispatchService handles email dispatch logic: user_notifications (email channel) -> AWS SES
// This service processes pending email notification jobs and sends them
type DispatchService struct {
	userNotifRepo    notification.UserNotificationRepository
	userRepo         user.UserRepository
	emailClient      *mailers.SESClient
	txManager        client.TxManager
	logger           *logger.Logger
	maxRetryAttempts int
	fromEmail        string
	fromName         string
}

// NewDispatchService creates a new dispatch service
func NewDispatchService(
	userNotifRepo notification.UserNotificationRepository,
	userRepo user.UserRepository,
	emailClient *mailers.SESClient,
	txManager client.TxManager,
	maxRetryAttempts int,
	fromEmail string,
	fromName string,
	logger *logger.Logger,
) *DispatchService {
	return &DispatchService{
		userNotifRepo:    userNotifRepo,
		userRepo:         userRepo,
		emailClient:      emailClient,
		txManager:        txManager,
		logger:           logger,
		maxRetryAttempts: maxRetryAttempts,
		fromEmail:        fromEmail,
		fromName:         fromName,
	}
}

// ProcessEmailDispatches fetches pending email jobs and sends them
// This is the "Email Dispatch" worker logic from the design document
// Implements the at-least-once delivery pattern with proper error handling
func (s *DispatchService) ProcessEmailDispatches(ctx context.Context, batchSize int) (int, error) {
	var jobsToSend []*notification.UserNotification

	// Step 1: Atomically fetch pending email jobs and mark them as processing
	// This prevents duplicate processing by multiple workers
	err := s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txUserNotifRepo := s.userNotifRepo.WithTx(tx)

		// Fetch pending email jobs with FOR UPDATE SKIP LOCKED and atomically mark as processing
		// This includes retrying failed jobs that haven't exceeded maxRetryAttempts
		jobs, err := txUserNotifRepo.FetchAndMarkJobsAsProcessing(ctx, notification.ChannelEmail, batchSize, s.maxRetryAttempts)
		if err != nil {
			return fmt.Errorf("failed to fetch and mark jobs as processing: %w", err)
		}

		if len(jobs) == 0 {
			return nil // No jobs to send
		}

		jobsToSend = jobs
		return nil
	})

	if err != nil {
		return 0, err
	}

	if len(jobsToSend) == 0 {
		return 0, nil // No jobs to send
	}

	// Step 2: Send emails OUTSIDE the transaction (no DB locks held)
	// This allows the transaction to commit quickly and other workers to proceed
	var succeededIDs, failedIDs []int64

	for _, job := range jobsToSend {
		// Get user email (we need to fetch fresh user data for email address)
		u, err := s.userRepo.GetUserByID(ctx, job.UserID)
		if err != nil {
			s.logger.Errorf("Failed to get user %s for email dispatch: %v", job.UserID, err)
			failedIDs = append(failedIDs, job.ID)
			continue
		}

		if u.Email == "" {
			s.logger.Errorf("User %s has no email address", job.UserID)
			failedIDs = append(failedIDs, job.ID)
			continue
		}

		// Extract email content from notification content
		title, _ := job.Content["title"].(string)
		body, _ := job.Content["body"].(string)

		if title == "" || body == "" {
			s.logger.Errorf("Job %d has invalid email content", job.ID)
			failedIDs = append(failedIDs, job.ID)
			continue
		}

		// Extract action buttons from content if present
		var actions []notification.ActionButton
		if actionsData, ok := job.Content["actions"].([]interface{}); ok {
			for _, actionData := range actionsData {
				if actionMap, ok := actionData.(map[string]interface{}); ok {
					action := notification.ActionButton{
						Type: getString(actionMap, "type"),
						Text: getString(actionMap, "text"),
						Href: getString(actionMap, "href"),
					}
					actions = append(actions, action)
				}
			}
		}

		// Send email via SES with idempotency protection
		// The notification ID is passed as an idempotency key via SES tags
		// This prevents duplicate emails if worker crashes between send and status update
		if err := SendEmailNotification(ctx, s.emailClient, job.ID, u.Email, title, body, actions, s.fromEmail, s.fromName); err != nil {
			s.logger.Errorf("Failed to send email for job %d to %s: %v", job.ID, u.Email, err)
			failedIDs = append(failedIDs, job.ID)
		} else {
			succeededIDs = append(succeededIDs, job.ID)
		}
	}

	// Step 3: Delete successful jobs, update failed jobs in a NEW transaction
	// Email/push notifications are DELETED after successful send (not persisted)
	// Web notifications would stay (but email channel doesn't hit this code path)
	err = s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txUserNotifRepo := s.userNotifRepo.WithTx(tx)

		// Delete successfully sent email/push notifications (keeps table tiny)
		// Web notifications are NOT deleted (handled by channel != 'web' filter in DeleteSentNotifications)
		if len(succeededIDs) > 0 {
			if err := txUserNotifRepo.DeleteSentNotifications(ctx, succeededIDs); err != nil {
				return fmt.Errorf("failed to delete sent notifications: %w", err)
			}
		}

		// Update failed jobs status for retry
		if len(failedIDs) > 0 {
			if err := txUserNotifRepo.UpdateJobStatuses(ctx, failedIDs, notification.StatusFailed); err != nil {
				return fmt.Errorf("failed to mark jobs as failed: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		s.logger.Errorf("Failed to update job statuses: %v", err)
		// IMPORTANT: If this transaction fails, jobs remain in "processing" status
		// Recovery worker will reset them back to "pending" after timeout
		return 0, fmt.Errorf("failed to update job statuses after sending: %w", err)
	}

	return len(succeededIDs), nil
}

// getString safely extracts a string value from a map[string]interface{}
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}
