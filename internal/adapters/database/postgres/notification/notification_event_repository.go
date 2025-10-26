package notification

import (
	"context"
	"encoding/json"
	"fmt"

	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/notification"
	"xcentro.in/internal/shared/logger"
)

// PgNotificationEventRepository implements notification.NotificationEventRepository using PostgreSQL
type PgNotificationEventRepository struct {
	querier client.Querier
	logger  *logger.Logger
}

// NewPgNotificationEventRepository creates a new PostgreSQL notification event repository
func NewPgNotificationEventRepository(db client.DB) *PgNotificationEventRepository {
	return &PgNotificationEventRepository{
		querier: db,
		logger:  logger.GetDefault(),
	}
}

// CreateEvent adds a new notification event to the queue
func (r *PgNotificationEventRepository) CreateEvent(ctx context.Context, event *notification.NotificationEvent) error {
	query := `
		INSERT INTO xcentro.notification_events (event_type, payload)
		VALUES ($1, $2)
		RETURNING id, created_at
	`

	payloadJSON, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	err = r.querier.QueryRow(ctx, query, event.EventType, payloadJSON).Scan(&event.ID, &event.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create notification event: %w", err)
	}

	return nil
}

// FetchPendingEvents retrieves pending events for processing with FOR UPDATE SKIP LOCKED
func (r *PgNotificationEventRepository) FetchPendingEvents(ctx context.Context, limit int) ([]*notification.NotificationEvent, error) {
	query := `
		SELECT id, event_type, payload, created_at
		FROM xcentro.notification_events
		WHERE processed_at IS NULL
		ORDER BY id
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`

	rows, err := r.querier.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pending events: %w", err)
	}
	defer rows.Close()

	var events []*notification.NotificationEvent
	for rows.Next() {
		var event notification.NotificationEvent
		var payloadJSON []byte

		err := rows.Scan(&event.ID, &event.EventType, &payloadJSON, &event.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan notification event: %w", err)
		}

		if err := json.Unmarshal(payloadJSON, &event.Payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
		}

		events = append(events, &event)
	}

	return events, nil
}

// MarkEventsProcessed marks events as processed
func (r *PgNotificationEventRepository) MarkEventsProcessed(ctx context.Context, eventIDs []int64) error {
	query := `
		UPDATE xcentro.notification_events
		SET processed_at = NOW()
		WHERE id = ANY($1)
	`

	_, err := r.querier.Exec(ctx, query, eventIDs)
	if err != nil {
		return fmt.Errorf("failed to mark events as processed: %w", err)
	}

	return nil
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgNotificationEventRepository) WithTx(tx client.Tx) notification.NotificationEventRepository {
	return &PgNotificationEventRepository{
		querier: tx,
		logger:  r.logger,
	}
}
