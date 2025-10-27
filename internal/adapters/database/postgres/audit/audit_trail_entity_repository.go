package audit

import (
	"context"
	"fmt"

	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/audit"
	"xcentro.in/internal/shared/logger"
)

// PgAuditTrailEntityRepository implements audit.AuditTrailEntityRepository using PostgreSQL
type PgAuditTrailEntityRepository struct {
	querier client.Querier
	logger  *logger.Logger
}

// NewPgAuditTrailEntityRepository creates a new PostgreSQL audit trail entity repository
func NewPgAuditTrailEntityRepository(db client.DB) *PgAuditTrailEntityRepository {
	return &PgAuditTrailEntityRepository{
		querier: db,
		logger:  logger.GetDefault(),
	}
}

// CreateEntityLinks creates links between an audit trail entry and entities
func (r *PgAuditTrailEntityRepository) CreateEntityLinks(ctx context.Context, entities []*audit.AuditTrailEntity) error {
	if len(entities) == 0 {
		return nil
	}

	query := `
		INSERT INTO xcentro.audit_trail_entities (
			business_id, audit_trail_id, entity_type, entity_id
		) VALUES ($1, $2, $3, $4)
	`

	// Insert entity links (within same transaction as audit trail entry)
	for _, entity := range entities {
		_, err := r.querier.Exec(ctx, query,
			entity.BusinessID,
			entity.AuditTrailID,
			entity.EntityType,
			entity.EntityID,
		)
		if err != nil {
			return fmt.Errorf("failed to insert entity link (type=%s, id=%s): %w",
				entity.EntityType, entity.EntityID, err)
		}
	}

	return nil
}

// GetEntitiesByAuditTrailID retrieves all entity links for an audit trail entry
func (r *PgAuditTrailEntityRepository) GetEntitiesByAuditTrailID(ctx context.Context, auditTrailID int64) ([]*audit.AuditTrailEntity, error) {
	query := `
		SELECT id, business_id, audit_trail_id, entity_type, entity_id
		FROM xcentro.audit_trail_entities
		WHERE audit_trail_id = $1
		ORDER BY entity_type, entity_id
	`

	rows, err := r.querier.Query(ctx, query, auditTrailID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch entity links: %w", err)
	}
	defer rows.Close()

	var entities []*audit.AuditTrailEntity
	for rows.Next() {
		var entity audit.AuditTrailEntity
		err := rows.Scan(
			&entity.ID,
			&entity.BusinessID,
			&entity.AuditTrailID,
			&entity.EntityType,
			&entity.EntityID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan entity link: %w", err)
		}
		entities = append(entities, &entity)
	}

	return entities, nil
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgAuditTrailEntityRepository) WithTx(tx client.Tx) audit.AuditTrailEntityRepository {
	return &PgAuditTrailEntityRepository{
		querier: tx,
		logger:  r.logger,
	}
}
