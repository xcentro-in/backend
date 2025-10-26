package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/audit"
	"xcentro.in/internal/shared/logger"
)

// PgAuditTrailRepository implements audit.AuditTrailRepository using PostgreSQL
type PgAuditTrailRepository struct {
	querier client.Querier
	logger  *logger.Logger
}

// NewPgAuditTrailRepository creates a new PostgreSQL audit trail repository
func NewPgAuditTrailRepository(db client.DB) *PgAuditTrailRepository {
	return &PgAuditTrailRepository{
		querier: db,
		logger:  logger.GetDefault(),
	}
}

// CreateTrailEntry adds a new entry to the permanent audit trail
func (r *PgAuditTrailRepository) CreateTrailEntry(ctx context.Context, entry *audit.AuditTrailEntry) error {
	query := `
		INSERT INTO xcentro.audit_trail (
			business_id, payment_id, event_timestamp, event_type,
			actor_type, actor_id, actor_name, note, details, source_event_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (source_event_id) DO NOTHING
		RETURNING id
	`

	var detailsJSON []byte
	if entry.Details != nil {
		var err error
		detailsJSON, err = json.Marshal(entry.Details)
		if err != nil {
			return fmt.Errorf("failed to marshal details: %w", err)
		}
	}

	err := r.querier.QueryRow(ctx, query,
		entry.BusinessID,
		entry.PaymentID,
		entry.EventTimestamp,
		entry.EventType,
		entry.ActorType,
		entry.ActorID,
		entry.ActorName,
		entry.Note,
		detailsJSON,
		entry.SourceEventID,
	).Scan(&entry.ID)

	if err != nil {
		if err == pgx.ErrNoRows {
			// Conflict occurred, entry already exists (idempotency)
			return nil
		}
		return fmt.Errorf("failed to create audit trail entry: %w", err)
	}

	return nil
}

// CreateTrailEntries batch inserts multiple entries
func (r *PgAuditTrailRepository) CreateTrailEntries(ctx context.Context, entries []*audit.AuditTrailEntry) error {
	if len(entries) == 0 {
		return nil
	}

	query := `
		INSERT INTO xcentro.audit_trail (
			business_id, payment_id, event_timestamp, event_type,
			actor_type, actor_id, actor_name, note, details, source_event_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (source_event_id) DO NOTHING
	`

	// Insert each entry individually (idempotent due to ON CONFLICT)
	for _, entry := range entries {
		var detailsJSON []byte
		if entry.Details != nil {
			var err error
			detailsJSON, err = json.Marshal(entry.Details)
			if err != nil {
				return fmt.Errorf("failed to marshal details: %w", err)
			}
		}

		_, err := r.querier.Exec(ctx, query,
			entry.BusinessID,
			entry.PaymentID,
			entry.EventTimestamp,
			entry.EventType,
			entry.ActorType,
			entry.ActorID,
			entry.ActorName,
			entry.Note,
			detailsJSON,
			entry.SourceEventID,
		)
		if err != nil {
			return fmt.Errorf("failed to insert audit trail entry: %w", err)
		}
	}

	return nil
}

// GetTrailByBusinessID retrieves audit trail for a business (sorted by timestamp DESC)
func (r *PgAuditTrailRepository) GetTrailByBusinessID(ctx context.Context, businessID string, limit, offset int) ([]*audit.AuditTrailEntry, error) {
	query := `
		SELECT id, business_id, payment_id, event_timestamp, event_type,
		       actor_type, actor_id, actor_name, note, details, source_event_id
		FROM xcentro.audit_trail
		WHERE business_id = $1
		ORDER BY event_timestamp DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.querier.Query(ctx, query, businessID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch audit trail: %w", err)
	}
	defer rows.Close()

	return r.scanTrailEntries(rows)
}

// GetTrailByPaymentID retrieves audit trail for a specific payment (sorted by timestamp DESC)
func (r *PgAuditTrailRepository) GetTrailByPaymentID(ctx context.Context, businessID, paymentID string) ([]*audit.AuditTrailEntry, error) {
	query := `
		SELECT id, business_id, payment_id, event_timestamp, event_type,
		       actor_type, actor_id, actor_name, note, details, source_event_id
		FROM xcentro.audit_trail
		WHERE business_id = $1 AND payment_id = $2
		ORDER BY event_timestamp DESC
	`

	rows, err := r.querier.Query(ctx, query, businessID, paymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch audit trail: %w", err)
	}
	defer rows.Close()

	return r.scanTrailEntries(rows)
}

// scanTrailEntries is a helper to scan rows into AuditTrailEntry slice
func (r *PgAuditTrailRepository) scanTrailEntries(rows pgx.Rows) ([]*audit.AuditTrailEntry, error) {
	var entries []*audit.AuditTrailEntry

	for rows.Next() {
		var entry audit.AuditTrailEntry
		var paymentID *string
		var note *string
		var detailsJSON []byte

		err := rows.Scan(
			&entry.ID,
			&entry.BusinessID,
			&paymentID,
			&entry.EventTimestamp,
			&entry.EventType,
			&entry.ActorType,
			&entry.ActorID,
			&entry.ActorName,
			&note,
			&detailsJSON,
			&entry.SourceEventID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit trail entry: %w", err)
		}

		entry.PaymentID = paymentID
		entry.Note = note

		if detailsJSON != nil {
			if err := json.Unmarshal(detailsJSON, &entry.Details); err != nil {
				return nil, fmt.Errorf("failed to unmarshal details: %w", err)
			}
		}

		entries = append(entries, &entry)
	}

	return entries, nil
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgAuditTrailRepository) WithTx(tx client.Tx) audit.AuditTrailRepository {
	return &PgAuditTrailRepository{
		querier: tx,
		logger:  r.logger,
	}
}
