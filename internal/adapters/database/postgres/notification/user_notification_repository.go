package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/notification"
	"xcentro.in/internal/shared/logger"
)

// PgUserNotificationRepository implements notification.UserNotificationRepository using PostgreSQL
type PgUserNotificationRepository struct {
	querier client.Querier
	logger  *logger.Logger
}

// NewPgUserNotificationRepository creates a new PostgreSQL user notification repository
func NewPgUserNotificationRepository(db client.DB) *PgUserNotificationRepository {
	return &PgUserNotificationRepository{
		querier: db,
		logger:  logger.GetDefault(),
	}
}

// CreateUserNotification adds a new notification to user's inbox
func (r *PgUserNotificationRepository) CreateUserNotification(ctx context.Context, notif *notification.UserNotification) error {
	query := `
		INSERT INTO xcentro.user_notifications (
			user_id, business_id, event_id, channel, status, content
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (event_id, user_id, channel) DO NOTHING
		RETURNING id, created_at
	`

	contentJSON, err := json.Marshal(notif.Content)
	if err != nil {
		return fmt.Errorf("failed to marshal content: %w", err)
	}

	err = r.querier.QueryRow(ctx, query,
		notif.UserID,
		notif.BusinessID,
		notif.EventID,
		notif.Channel,
		notif.Status,
		contentJSON,
	).Scan(&notif.ID, &notif.CreatedAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			// Conflict occurred, notification already exists (idempotency)
			return nil
		}
		return fmt.Errorf("failed to create user notification: %w", err)
	}

	return nil
}

// CreateUserNotifications batch inserts multiple notifications
func (r *PgUserNotificationRepository) CreateUserNotifications(ctx context.Context, notifications []*notification.UserNotification) error {
	if len(notifications) == 0 {
		return nil
	}

	query := `
		INSERT INTO xcentro.user_notifications (
			user_id, business_id, event_id, channel, status, content
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (event_id, user_id, channel) DO NOTHING
	`

	// Insert each notification individually (idempotent due to ON CONFLICT)
	for _, notif := range notifications {
		contentJSON, err := json.Marshal(notif.Content)
		if err != nil {
			return fmt.Errorf("failed to marshal content: %w", err)
		}

		_, err = r.querier.Exec(ctx, query,
			notif.UserID,
			notif.BusinessID,
			notif.EventID,
			notif.Channel,
			notif.Status,
			contentJSON,
		)
		if err != nil {
			return fmt.Errorf("failed to insert user notification: %w", err)
		}
	}

	return nil
}

// GetUserNotifications retrieves notifications for a user (sorted by created_at DESC)
// Index used: idx_user_notifications_user_id (user_id, created_at DESC)
// Performance: O(log n) + limit, scales well with pagination
func (r *PgUserNotificationRepository) GetUserNotifications(ctx context.Context, userID string, limit, offset int) ([]*notification.UserNotification, error) {
	query := `
		SELECT id, user_id, business_id, event_id, channel, status, content, retry_count,
		       created_at, sent_at, read_at
		FROM xcentro.user_notifications
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.querier.Query(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user notifications: %w", err)
	}
	defer rows.Close()

	return r.scanUserNotifications(rows)
}

// MarkAsRead marks a notification as read
// NOTE: Only web channel notifications can be marked as read (email/push cannot)
func (r *PgUserNotificationRepository) MarkAsRead(ctx context.Context, notificationID int64, userID string) error {
	query := `
		UPDATE xcentro.user_notifications
		SET read_at = NOW(), status = $3
		WHERE id = $1 AND user_id = $2 AND read_at IS NULL AND channel = $4
	`

	result, err := r.querier.Exec(ctx, query, notificationID, userID, notification.StatusRead, notification.ChannelWeb)
	if err != nil {
		return fmt.Errorf("failed to mark notification as read: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("notification not found, already read, or not a web notification")
	}

	return nil
}

// MarkMultipleAsRead marks multiple notifications as read
// NOTE: Only web channel notifications can be marked as read (email/push are ignored)
func (r *PgUserNotificationRepository) MarkMultipleAsRead(ctx context.Context, notificationIDs []int64, userID string) error {
	query := `
		UPDATE xcentro.user_notifications
		SET read_at = NOW(), status = $3
		WHERE id = ANY($1) AND user_id = $2 AND read_at IS NULL AND channel = $4
	`

	_, err := r.querier.Exec(ctx, query, notificationIDs, userID, notification.StatusRead, notification.ChannelWeb)
	if err != nil {
		return fmt.Errorf("failed to mark notifications as read: %w", err)
	}

	return nil
}

// GetUnreadCount gets count of unread notifications for a user
// Index used: idx_user_notifications_user_unread (user_id, created_at DESC) WHERE read_at IS NULL AND channel = 'web'
// Performance: O(log n), COUNT(*) optimized by partial index
func (r *PgUserNotificationRepository) GetUnreadCount(ctx context.Context, userID string) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM xcentro.user_notifications
		WHERE user_id = $1 AND read_at IS NULL AND channel = $2
	`

	var count int
	err := r.querier.QueryRow(ctx, query, userID, notification.ChannelWeb).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get unread count: %w", err)
	}

	return count, nil
}

// FetchAndMarkJobsAsProcessing atomically fetches pending/failed jobs and marks them as processing
// This prevents duplicate processing by multiple workers and implements retry logic for failed jobs
// NOTE: retry_count is only incremented for failed jobs (retries), not for pending jobs (first attempt)
// Index used: idx_user_notifications_worker_jobs (channel, status, retry_count, created_at) WHERE status IN ('pending', 'failed')
// Performance: O(log n) + limit, FOR UPDATE SKIP LOCKED prevents lock contention, highly concurrent
func (r *PgUserNotificationRepository) FetchAndMarkJobsAsProcessing(ctx context.Context, channel string, limit int, maxRetryAttempts int) ([]*notification.UserNotification, error) {
	query := `
		WITH selected_jobs AS (
			SELECT id, status
			FROM xcentro.user_notifications
			WHERE channel = $1
			  AND (status = $2 OR (status = $3 AND retry_count < $4))
			ORDER BY created_at
			LIMIT $5
			FOR UPDATE SKIP LOCKED
		)
		UPDATE xcentro.user_notifications un
		SET status = $6,
		    retry_count = CASE
		        WHEN un.status = $3 THEN un.retry_count + 1  -- Increment only for failed (retries)
		        ELSE un.retry_count                           -- Keep same for pending (first attempt)
		    END
		FROM selected_jobs
		WHERE un.id = selected_jobs.id
		RETURNING un.id, un.user_id, un.business_id, un.event_id, un.channel, un.status, un.content, un.retry_count,
		          un.created_at, un.sent_at, un.read_at
	`

	rows, err := r.querier.Query(ctx, query,
		channel,
		notification.StatusPending,
		notification.StatusFailed,
		maxRetryAttempts,
		limit,
		notification.StatusProcessing,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch and mark jobs as processing for channel %s: %w", channel, err)
	}
	defer rows.Close()

	return r.scanUserNotifications(rows)
}

// UpdateJobStatuses updates the status and sent_at timestamp of notification jobs
func (r *PgUserNotificationRepository) UpdateJobStatuses(ctx context.Context, jobIDs []int64, status string) error {
	if len(jobIDs) == 0 {
		return nil
	}

	query := `
		UPDATE xcentro.user_notifications
		SET status = $1, sent_at = NOW()
		WHERE id = ANY($2)
	`

	_, err := r.querier.Exec(ctx, query, status, jobIDs)
	if err != nil {
		return fmt.Errorf("failed to update job statuses: %w", err)
	}

	return nil
}

// DeletePermanentlyFailedJobs deletes jobs that have exceeded max retry attempts
// This cleans up jobs that cannot be delivered after multiple retries
func (r *PgUserNotificationRepository) DeletePermanentlyFailedJobs(ctx context.Context, channel string, maxRetryAttempts int) (int64, error) {
	query := `
		DELETE FROM xcentro.user_notifications
		WHERE channel = $1 AND status = $2 AND retry_count >= $3
	`

	result, err := r.querier.Exec(ctx, query, channel, notification.StatusFailed, maxRetryAttempts)
	if err != nil {
		return 0, fmt.Errorf("failed to delete permanently failed jobs: %w", err)
	}

	return result.RowsAffected(), nil
}

// RecoverStuckProcessingJobs resets stuck processing jobs back to pending
// This handles cases where workers crash or timeout while processing jobs
// Index used: idx_user_notifications_stuck_processing (created_at) WHERE status = 'processing'
// Performance: O(log n) + matching rows, partial index keeps this lightweight
func (r *PgUserNotificationRepository) RecoverStuckProcessingJobs(ctx context.Context, timeoutMinutes int) (int64, error) {
	query := `
		UPDATE xcentro.user_notifications
		SET status = $1
		WHERE status = $2 AND created_at < NOW() - INTERVAL '1 minute' * $3
	`

	result, err := r.querier.Exec(ctx, query, notification.StatusPending, notification.StatusProcessing, timeoutMinutes)
	if err != nil {
		return 0, fmt.Errorf("failed to recover stuck processing jobs: %w", err)
	}

	return result.RowsAffected(), nil
}

// scanUserNotifications is a helper to scan rows into UserNotification slice
func (r *PgUserNotificationRepository) scanUserNotifications(rows pgx.Rows) ([]*notification.UserNotification, error) {
	var notifications []*notification.UserNotification

	for rows.Next() {
		var notif notification.UserNotification
		var businessID *string
		var sentAt, readAt *time.Time
		var contentJSON []byte

		err := rows.Scan(
			&notif.ID,
			&notif.UserID,
			&businessID,
			&notif.EventID,
			&notif.Channel,
			&notif.Status,
			&contentJSON,
			&notif.RetryCount,
			&notif.CreatedAt,
			&sentAt,
			&readAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user notification: %w", err)
		}

		notif.BusinessID = businessID
		notif.SentAt = sentAt
		notif.ReadAt = readAt

		if err := json.Unmarshal(contentJSON, &notif.Content); err != nil {
			return nil, fmt.Errorf("failed to unmarshal content: %w", err)
		}

		notifications = append(notifications, &notif)
	}

	return notifications, nil
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgUserNotificationRepository) WithTx(tx client.Tx) notification.UserNotificationRepository {
	return &PgUserNotificationRepository{
		querier: tx,
		logger:  r.logger,
	}
}
