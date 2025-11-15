package notification

import "context"

// NotificationService defines the service layer interface for notification operations
type NotificationService interface {
	// User-facing operations
	GetUserNotifications(ctx context.Context, userID string, limit, offset int) ([]*UserNotification, error)
	GetUnreadCount(ctx context.Context, userID string) (int, error)
	MarkAsRead(ctx context.Context, notificationID int64, userID string) error
	MarkMultipleAsRead(ctx context.Context, notificationIDs []int64, userID string) error
	GetPreferences(ctx context.Context, userID string) (*NotificationPreferences, error)
	UpdatePreferences(ctx context.Context, prefs *NotificationPreferences) error

	// Manual trigger for ops/campaigns
	TriggerManualNotification(ctx context.Context, payload NotificationEventPayload) error

	// Bulk trigger for ops dashboard - triggers notifications for multiple businesses in a transaction
	BulkTriggerNotifications(ctx context.Context, request *OpsBulkTriggerRequest) (*OpsBulkTriggerResponse, error)
}

// FanoutService defines the interface for notification fan-out operations
// Handles the flow: notification_events -> user_notifications
type FanoutService interface {
	// ProcessPendingEvents fetches pending notification events and creates user notifications
	// Returns the number of events processed
	ProcessPendingEvents(ctx context.Context, batchSize int) (int, error)
}

// DispatchService defines the interface for notification dispatch operations
// Handles the flow: user_notifications (email channel) -> AWS SES
type DispatchService interface {
	// ProcessEmailDispatches fetches pending email jobs and sends them via SES
	// Returns the number of emails sent
	ProcessEmailDispatches(ctx context.Context, batchSize int) (int, error)
}

// RecoveryService defines the interface for notification recovery and cleanup operations
// Handles: stuck processing jobs → pending, permanently failed jobs → delete
type RecoveryService interface {
	// ProcessRecoveryAndCleanup performs stuck job recovery and cleanup
	// Returns (recoveredCount, deletedCount, error)
	ProcessRecoveryAndCleanup(ctx context.Context) (int64, int64, error)
}
