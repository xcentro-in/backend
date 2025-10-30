package notification

import (
	"context"

	"xcentro.in/internal/adapters/database/postgres/client"
)

// NotificationEventRepository handles notification event queue operations
type NotificationEventRepository interface {
	// CreateEvent adds a new notification event to the queue
	CreateEvent(ctx context.Context, event *NotificationEvent) error

	// FetchPendingEvents retrieves pending events for processing (with FOR UPDATE SKIP LOCKED)
	FetchPendingEvents(ctx context.Context, limit int) ([]*NotificationEvent, error)

	// DeleteEvents deletes events after fanout is complete (called in same transaction)
	DeleteEvents(ctx context.Context, eventIDs []int64) error

	// WithTx creates a new repository instance that uses a transaction
	WithTx(tx client.Tx) NotificationEventRepository
}

// UserNotificationRepository handles user notification inbox operations
type UserNotificationRepository interface {
	// CreateUserNotification adds a new notification to user's inbox
	CreateUserNotification(ctx context.Context, notification *UserNotification) error

	// CreateUserNotifications batch inserts multiple notifications
	CreateUserNotifications(ctx context.Context, notifications []*UserNotification) error

	// GetUserNotifications retrieves notifications for a user (sorted by created_at DESC)
	GetUserNotifications(ctx context.Context, userID string, limit, offset int) ([]*UserNotification, error)

	// MarkAsRead marks a notification as read
	MarkAsRead(ctx context.Context, notificationID int64, userID string) error

	// MarkMultipleAsRead marks multiple notifications as read
	MarkMultipleAsRead(ctx context.Context, notificationIDs []int64, userID string) error

	// GetUnreadCount gets count of unread notifications for a user
	GetUnreadCount(ctx context.Context, userID string) (int, error)

	// FetchAndMarkJobsAsProcessing atomically fetches pending/failed jobs and marks them as processing
	// This prevents duplicate processing by multiple workers and implements retry logic for failed jobs
	FetchAndMarkJobsAsProcessing(ctx context.Context, channel string, limit int, maxRetryAttempts int) ([]*UserNotification, error)

	// UpdateJobStatuses updates the status of notification jobs (used for failed status)
	UpdateJobStatuses(ctx context.Context, jobIDs []int64, status string) error

	// DeleteSentNotifications deletes successfully sent notifications (email/push only, NOT web)
	DeleteSentNotifications(ctx context.Context, jobIDs []int64) error

	// DeletePermanentlyFailedJobs deletes jobs that have exceeded max retry attempts
	DeletePermanentlyFailedJobs(ctx context.Context, channel string, maxRetryAttempts int) (int64, error)

	// RecoverStuckProcessingJobs resets stuck processing jobs back to pending
	RecoverStuckProcessingJobs(ctx context.Context, timeoutMinutes int) (int64, error)

	// WithTx creates a new repository instance that uses a transaction
	WithTx(tx client.Tx) UserNotificationRepository
}

// NotificationPreferencesRepository handles user notification preferences
type NotificationPreferencesRepository interface {
	// GetPreferences retrieves user's notification preferences
	GetPreferences(ctx context.Context, userID string) (*NotificationPreferences, error)

	// UpsertPreferences creates or updates user's notification preferences
	UpsertPreferences(ctx context.Context, prefs *NotificationPreferences) error

	// WithTx creates a new repository instance that uses a transaction
	WithTx(tx client.Tx) NotificationPreferencesRepository
}
