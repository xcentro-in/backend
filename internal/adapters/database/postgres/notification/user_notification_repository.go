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
func (r *PgUserNotificationRepository) GetUserNotifications(ctx context.Context, userID string, limit, offset int) ([]*notification.UserNotification, error) {
	query := `
		SELECT id, user_id, business_id, event_id, channel, status, content,
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
func (r *PgUserNotificationRepository) MarkAsRead(ctx context.Context, notificationID int64, userID string) error {
	query := `
		UPDATE xcentro.user_notifications
		SET read_at = NOW()
		WHERE id = $1 AND user_id = $2 AND read_at IS NULL
	`

	result, err := r.querier.Exec(ctx, query, notificationID, userID)
	if err != nil {
		return fmt.Errorf("failed to mark notification as read: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("notification not found or already read")
	}

	return nil
}

// MarkMultipleAsRead marks multiple notifications as read
func (r *PgUserNotificationRepository) MarkMultipleAsRead(ctx context.Context, notificationIDs []int64, userID string) error {
	query := `
		UPDATE xcentro.user_notifications
		SET read_at = NOW()
		WHERE id = ANY($1) AND user_id = $2 AND read_at IS NULL
	`

	_, err := r.querier.Exec(ctx, query, notificationIDs, userID)
	if err != nil {
		return fmt.Errorf("failed to mark notifications as read: %w", err)
	}

	return nil
}

// GetUnreadCount gets count of unread notifications for a user
func (r *PgUserNotificationRepository) GetUnreadCount(ctx context.Context, userID string) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM xcentro.user_notifications
		WHERE user_id = $1 AND read_at IS NULL
	`

	var count int
	err := r.querier.QueryRow(ctx, query, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get unread count: %w", err)
	}

	return count, nil
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
