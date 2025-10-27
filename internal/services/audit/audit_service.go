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
	auditEventRepo       audit.AuditEventRepository
	auditTrailRepo       audit.AuditTrailRepository
	auditTrailEntityRepo audit.AuditTrailEntityRepository
	txManager            client.TxManager
	logger               *logger.Logger
}

// NewAuditService creates a new audit service
func NewAuditService(
	auditEventRepo audit.AuditEventRepository,
	auditTrailRepo audit.AuditTrailRepository,
	auditTrailEntityRepo audit.AuditTrailEntityRepository,
	txManager client.TxManager,
	logger *logger.Logger,
) *AuditService {
	return &AuditService{
		auditEventRepo:       auditEventRepo,
		auditTrailRepo:       auditTrailRepo,
		auditTrailEntityRepo: auditTrailEntityRepo,
		txManager:            txManager,
		logger:               logger,
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
		txEntityRepo := s.auditTrailEntityRepo.WithTx(tx)

		// Fetch pending events with FOR UPDATE SKIP LOCKED (within transaction)
		events, err := txEventRepo.FetchPendingEvents(ctx, batchSize)
		if err != nil {
			return fmt.Errorf("failed to fetch pending events: %w", err)
		}

		if len(events) == 0 {
			return nil // No events to process
		}

		// Process each event: insert trail entry + entity links
		var eventIDs []int64

		for _, event := range events {
			// Transform event to audit trail entry
			entry := transformEventToTrailEntry(event)
			if entry == nil {
				continue // Skip invalid events
			}

			// Insert the audit trail entry and get its ID
			if err := txTrailRepo.CreateTrailEntry(ctx, entry); err != nil {
				return fmt.Errorf("failed to create trail entry for event %d: %w", event.ID, err)
			}

			// Build entity links from the event payload
			entityLinks := buildEntityLinks(event, entry.ID, entry.BusinessID)

			// Insert entity links
			if len(entityLinks) > 0 {
				if err := txEntityRepo.CreateEntityLinks(ctx, entityLinks); err != nil {
					return fmt.Errorf("failed to create entity links for event %d: %w", event.ID, err)
				}
			}

			eventIDs = append(eventIDs, event.ID)
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

// GetTrailByEntityID retrieves audit trail for a specific entity (payment, voucher, payee, etc.)
func (s *AuditService) GetTrailByEntityID(ctx context.Context, entityType, entityID string, limit, offset int) ([]*audit.AuditTrailEntry, error) {
	return s.auditTrailRepo.GetTrailByEntityID(ctx, entityType, entityID, limit, offset)
}

// transformEventToTrailEntry transforms an audit event to a trail entry
// Note: payment_id removed from audit_trail (normalized design)
func transformEventToTrailEntry(event *audit.AuditEvent) *audit.AuditTrailEntry {
	payload := event.Payload

	// Validate required business_id
	if payload.Entities.BusinessID == "" {
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
		BusinessID:     payload.Entities.BusinessID,
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

// buildEntityLinks builds entity links from the event payload
// This extracts all entity references and creates audit_trail_entities rows
func buildEntityLinks(event *audit.AuditEvent, auditTrailID int64, businessID string) []*audit.AuditTrailEntity {
	var links []*audit.AuditTrailEntity
	entities := event.Payload.Entities

	// Link to payment
	if entities.PaymentID != nil {
		links = append(links, &audit.AuditTrailEntity{
			BusinessID:   businessID,
			AuditTrailID: auditTrailID,
			EntityType:   audit.EntityTypePayment,
			EntityID:     *entities.PaymentID,
		})
	}

	// Link to voucher
	if entities.VoucherID != nil {
		links = append(links, &audit.AuditTrailEntity{
			BusinessID:   businessID,
			AuditTrailID: auditTrailID,
			EntityType:   audit.EntityTypeVoucher,
			EntityID:     *entities.VoucherID,
		})
	}

	// Link to payee
	if entities.PayeeID != nil {
		links = append(links, &audit.AuditTrailEntity{
			BusinessID:   businessID,
			AuditTrailID: auditTrailID,
			EntityType:   audit.EntityTypePayee,
			EntityID:     *entities.PayeeID,
		})
	}

	// Link to user
	if entities.UserID != nil {
		links = append(links, &audit.AuditTrailEntity{
			BusinessID:   businessID,
			AuditTrailID: auditTrailID,
			EntityType:   audit.EntityTypeUser,
			EntityID:     *entities.UserID,
		})
	}

	// Link to wallet
	if entities.WalletID != nil {
		links = append(links, &audit.AuditTrailEntity{
			BusinessID:   businessID,
			AuditTrailID: auditTrailID,
			EntityType:   audit.EntityTypeWallet,
			EntityID:     *entities.WalletID,
		})
	}

	return links
}
