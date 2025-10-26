package audit

import (
	"context"
	"fmt"
	"time"

	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/audit"
	"xcentro.in/internal/shared/logger"
)

// AuditService handles audit trail operations
type AuditService struct {
	auditEventRepo audit.AuditEventRepository
	auditTrailRepo audit.AuditTrailRepository
	txManager      client.TxManager
	logger         *logger.Logger
}

// NewAuditService creates a new audit service
func NewAuditService(
	auditEventRepo audit.AuditEventRepository,
	auditTrailRepo audit.AuditTrailRepository,
	txManager client.TxManager,
	logger *logger.Logger,
) *AuditService {
	return &AuditService{
		auditEventRepo: auditEventRepo,
		auditTrailRepo: auditTrailRepo,
		txManager:      txManager,
		logger:         logger,
	}
}

// ProcessPendingAuditEvents fetches, transforms, and processes audit events in a transaction
// This is called by the audit worker
func (s *AuditService) ProcessPendingAuditEvents(ctx context.Context, batchSize int) (int, error) {
	var processedCount int

	err := s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		// Create transactional repository instances
		txEventRepo := s.auditEventRepo.WithTx(tx)
		txTrailRepo := s.auditTrailRepo.WithTx(tx)

		// Fetch pending events with FOR UPDATE SKIP LOCKED (within transaction)
		events, err := txEventRepo.FetchPendingEvents(ctx, batchSize)
		if err != nil {
			return fmt.Errorf("failed to fetch pending events: %w", err)
		}

		if len(events) == 0 {
			return nil // No events to process
		}

		// Transform events to audit trail entries
		var entries []*audit.AuditTrailEntry
		var eventIDs []int64

		for _, event := range events {
			entry := transformEventToTrailEntry(event)
			if entry != nil {
				entries = append(entries, entry)
				eventIDs = append(eventIDs, event.ID)
			}
		}

		// Batch insert into audit_trail
		if len(entries) > 0 {
			if err := txTrailRepo.CreateTrailEntries(ctx, entries); err != nil {
				return fmt.Errorf("failed to create trail entries: %w", err)
			}
		}

		// Mark events as processed
		if len(eventIDs) > 0 {
			if err := txEventRepo.MarkEventsProcessed(ctx, eventIDs); err != nil {
				return fmt.Errorf("failed to mark events as processed: %w", err)
			}
		}

		processedCount = len(eventIDs)
		return nil
	})

	if err != nil {
		return 0, err
	}

	return processedCount, nil
}

// GetTrailByBusinessID retrieves audit trail for a business
func (s *AuditService) GetTrailByBusinessID(ctx context.Context, businessID string, limit, offset int) ([]*audit.AuditTrailEntry, error) {
	return s.auditTrailRepo.GetTrailByBusinessID(ctx, businessID, limit, offset)
}

// GetTrailByPaymentID retrieves audit trail for a specific payment
func (s *AuditService) GetTrailByPaymentID(ctx context.Context, businessID, paymentID string) ([]*audit.AuditTrailEntry, error) {
	return s.auditTrailRepo.GetTrailByPaymentID(ctx, businessID, paymentID)
}

// transformEventToTrailEntry transforms an audit event to a trail entry
func transformEventToTrailEntry(event *audit.AuditEvent) *audit.AuditTrailEntry {
	payload := event.Payload

	// Extract business_id from entities
	if payload.Entities.BusinessID == nil {
		return nil
	}

	// Parse event timestamp
	eventTime, err := time.Parse(time.RFC3339, payload.Timestamp)
	if err != nil {
		eventTime = event.CreatedAt
	}

	// Build details JSONB
	details := make(map[string]any)
	if payload.Actor.IPAddress != nil {
		details["ip_address"] = *payload.Actor.IPAddress
	}
	if len(payload.ChangeLog) > 0 {
		details["change_log"] = payload.ChangeLog
	}

	return &audit.AuditTrailEntry{
		BusinessID:     *payload.Entities.BusinessID,
		PaymentID:      payload.Entities.PaymentID,
		EventTimestamp: eventTime,
		EventType:      payload.EventType,
		ActorType:      payload.Actor.Type,
		ActorID:        payload.Actor.ID,
		ActorName:      payload.Actor.Name,
		Note:           payload.Note,
		Details:        details,
		SourceEventID:  event.ID,
	}
}
