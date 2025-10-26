package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/audit"
	"xcentro.in/internal/shared/logger"
)

// PgAuditEventRepository implements audit.AuditEventRepository using PostgreSQL
type PgAuditEventRepository struct {
	querier client.Querier
	logger  *logger.Logger
}

// NewPgAuditEventRepository creates a new PostgreSQL audit event repository
func NewPgAuditEventRepository(db client.DB) *PgAuditEventRepository {
	return &PgAuditEventRepository{
		querier: db,
		logger:  logger.GetDefault(),
	}
}

// CreateEvent adds a new audit event to the queue
func (r *PgAuditEventRepository) CreateEvent(ctx context.Context, event *audit.AuditEvent) error {
	query := `
		INSERT INTO xcentro.audit_events (event_type, payload)
		VALUES ($1, $2)
		RETURNING id, created_at
	`

	payloadJSON, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	err = r.querier.QueryRow(ctx, query, event.EventType, payloadJSON).Scan(&event.ID, &event.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create audit event: %w", err)
	}

	return nil
}

// FetchPendingEvents retrieves pending events for processing with FOR UPDATE SKIP LOCKED
func (r *PgAuditEventRepository) FetchPendingEvents(ctx context.Context, limit int) ([]*audit.AuditEvent, error) {
	query := `
		SELECT id, event_type, payload, created_at
		FROM xcentro.audit_events
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

	var events []*audit.AuditEvent
	for rows.Next() {
		var event audit.AuditEvent
		var payloadJSON []byte

		err := rows.Scan(&event.ID, &event.EventType, &payloadJSON, &event.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit event: %w", err)
		}

		if err := json.Unmarshal(payloadJSON, &event.Payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
		}

		events = append(events, &event)
	}

	return events, nil
}

// MarkEventsProcessed marks events as processed
func (r *PgAuditEventRepository) MarkEventsProcessed(ctx context.Context, eventIDs []int64) error {
	query := `
		UPDATE xcentro.audit_events
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
func (r *PgAuditEventRepository) WithTx(tx client.Tx) audit.AuditEventRepository {
	return &PgAuditEventRepository{
		querier: tx,
		logger:  r.logger,
	}
}
