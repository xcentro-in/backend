package audit

import (
	"context"

	"xcentro.in/internal/adapters/database/postgres/client"
)

// AuditEventRepository handles audit event queue operations
type AuditEventRepository interface {
	// CreateEvent adds a new audit event to the queue
	CreateEvent(ctx context.Context, event *AuditEvent) error

	// FetchPendingEvents retrieves pending events for processing (with FOR UPDATE SKIP LOCKED)
	FetchPendingEvents(ctx context.Context, limit int) ([]*AuditEvent, error)

	// MarkEventsProcessed marks events as processed
	MarkEventsProcessed(ctx context.Context, eventIDs []int64) error

	// WithTx creates a new repository instance that uses a transaction
	WithTx(tx client.Tx) AuditEventRepository
}

// AuditTrailRepository handles audit trail operations
type AuditTrailRepository interface {
	// CreateTrailEntry adds a new entry to the permanent audit trail
	CreateTrailEntry(ctx context.Context, entry *AuditTrailEntry) error

	// CreateTrailEntries batch inserts multiple entries
	CreateTrailEntries(ctx context.Context, entries []*AuditTrailEntry) error

	// GetTrailByBusinessID retrieves audit trail for a business (sorted by timestamp DESC)
	GetTrailByBusinessID(ctx context.Context, businessID string, limit, offset int) ([]*AuditTrailEntry, error)

	// GetTrailByPaymentID retrieves audit trail for a specific payment (sorted by timestamp DESC)
	GetTrailByPaymentID(ctx context.Context, businessID, paymentID string) ([]*AuditTrailEntry, error)

	// WithTx creates a new repository instance that uses a transaction
	WithTx(tx client.Tx) AuditTrailRepository
}
