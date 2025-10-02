package payee

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/lib/pq"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/payee"
)

// PgBusinessPayeeRelationRepository implements payee.BusinessPayeeRelationRepository using PostgreSQL
type PgBusinessPayeeRelationRepository struct {
	querier client.Querier
}

// NewPgBusinessPayeeRelationRepository creates a new PostgreSQL business-payee relation repository
func NewPgBusinessPayeeRelationRepository(db client.DB) payee.BusinessPayeeRelationRepository {
	return &PgBusinessPayeeRelationRepository{
		querier: db,
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgBusinessPayeeRelationRepository) WithTx(tx client.Tx) payee.BusinessPayeeRelationRepository {
	return &PgBusinessPayeeRelationRepository{
		querier: tx,
	}
}

// CreateRelation creates a new business-payee relation
func (r *PgBusinessPayeeRelationRepository) CreateRelation(ctx context.Context, relation *payee.BusinessPayeeRelation) error {
	fmt.Printf("[PgBusinessPayeeRelationRepository] CreateRelation called - BusinessID: %s, PayeeID: %s, AccountID: %s\n",
		relation.BusinessID, relation.PayeeID, relation.AccountID)

	query := `
		INSERT INTO xcentro.business_payee_relation (
			business_id, payee_id, account_id,
			display_name, contact_email, contact_phone, notes,
			is_active, is_favorite, tags,
			added_by, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING added_at
	`

	var metadataJSON []byte
	var err error

	if relation.Metadata != nil {
		metadataJSON, err = json.Marshal(relation.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	err = r.querier.QueryRow(ctx, query,
		relation.BusinessID,
		relation.PayeeID,
		relation.AccountID,
		relation.DisplayName,
		relation.ContactEmail,
		relation.ContactPhone,
		relation.Notes,
		relation.IsActive,
		relation.IsFavorite,
		pq.Array(relation.Tags), // PostgreSQL array
		relation.AddedBy,
		metadataJSON,
	).Scan(&relation.AddedAt)

	if err != nil {
		return fmt.Errorf("failed to create business-payee relation: %w", err)
	}

	fmt.Printf("[PgBusinessPayeeRelationRepository] Relation created at: %s\n", relation.AddedAt)
	return nil
}

// FindRelation retrieves a specific business-payee relation
func (r *PgBusinessPayeeRelationRepository) FindRelation(ctx context.Context, businessID, payeeID, accountID string) (*payee.BusinessPayeeRelation, error) {
	fmt.Printf("[PgBusinessPayeeRelationRepository] FindRelation called - BusinessID: %s, PayeeID: %s, AccountID: %s\n",
		businessID, payeeID, accountID)

	query := `
		SELECT business_id, payee_id, account_id,
		       display_name, contact_email, contact_phone, notes,
		       is_active, is_favorite, tags,
		       added_at, added_by, last_used_at, updated_at, metadata
		FROM xcentro.business_payee_relation
		WHERE business_id = $1 AND payee_id = $2 AND account_id = $3
	`

	var relation payee.BusinessPayeeRelation
	var contactEmail, contactPhone, notes sql.NullString
	var addedBy sql.NullString
	var lastUsedAt, updatedAt sql.NullTime
	var metadataJSON []byte

	err := r.querier.QueryRow(ctx, query, businessID, payeeID, accountID).Scan(
		&relation.BusinessID,
		&relation.PayeeID,
		&relation.AccountID,
		&relation.DisplayName,
		&contactEmail,
		&contactPhone,
		&notes,
		&relation.IsActive,
		&relation.IsFavorite,
		pq.Array(&relation.Tags),
		&relation.AddedAt,
		&addedBy,
		&lastUsedAt,
		&updatedAt,
		&metadataJSON,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("business-payee relation not found")
		}
		return nil, fmt.Errorf("failed to find business-payee relation: %w", err)
	}

	// Handle nullable fields
	relation.ContactEmail = contactEmail.String
	relation.ContactPhone = contactPhone.String
	relation.Notes = notes.String
	relation.AddedBy = addedBy.String
	if lastUsedAt.Valid {
		relation.LastUsedAt = &lastUsedAt.Time
	}
	if updatedAt.Valid {
		relation.UpdatedAt = updatedAt.Time
	}

	// Deserialize JSONB
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &relation.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &relation, nil
}

// FindByBusiness retrieves all payee relations for a business with filters
func (r *PgBusinessPayeeRelationRepository) FindByBusiness(ctx context.Context, businessID string, filters payee.RelationSearchFilters) ([]*payee.BusinessPayeeRelation, error) {
	fmt.Printf("[PgBusinessPayeeRelationRepository] FindByBusiness called - BusinessID: %s\n", businessID)

	query := `
		SELECT r.business_id, r.payee_id, r.account_id,
		       r.display_name, r.contact_email, r.contact_phone, r.notes,
		       r.is_active, r.is_favorite, r.tags,
		       r.added_at, r.added_by, r.last_used_at, r.updated_at, r.metadata
		FROM xcentro.business_payee_relation r
	`

	// Join with payee_master if filtering by payee_type
	if filters.PayeeType != "" {
		query += `
		INNER JOIN xcentro.payee_master m ON r.payee_id = m.payee_id
		`
	}

	query += `WHERE r.business_id = $1`

	args := []interface{}{businessID}
	argIdx := 2

	// Add filters
	if filters.DisplayName != "" {
		query += fmt.Sprintf(" AND r.display_name ILIKE $%d", argIdx)
		args = append(args, "%"+filters.DisplayName+"%")
		argIdx++
	}

	if filters.PayeeType != "" {
		query += fmt.Sprintf(" AND m.payee_type = $%d", argIdx)
		args = append(args, filters.PayeeType)
		argIdx++
	}

	if filters.IsActive != nil {
		query += fmt.Sprintf(" AND r.is_active = $%d", argIdx)
		args = append(args, *filters.IsActive)
		argIdx++
	}

	if filters.IsFavorite != nil {
		query += fmt.Sprintf(" AND r.is_favorite = $%d", argIdx)
		args = append(args, *filters.IsFavorite)
		argIdx++
	}

	if len(filters.Tags) > 0 {
		query += fmt.Sprintf(" AND r.tags && $%d", argIdx) // Array overlap operator
		args = append(args, pq.Array(filters.Tags))
		argIdx++
	}

	// Add ordering and pagination
	query += " ORDER BY r.is_favorite DESC, r.display_name ASC"

	if filters.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIdx)
		args = append(args, filters.Limit)
		argIdx++
	}

	if filters.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argIdx)
		args = append(args, filters.Offset)
	}

	rows, err := r.querier.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to find business-payee relations: %w", err)
	}
	defer rows.Close()

	var relations []*payee.BusinessPayeeRelation

	for rows.Next() {
		var relation payee.BusinessPayeeRelation
		var contactEmail, contactPhone, notes sql.NullString
		var addedBy sql.NullString
		var lastUsedAt, updatedAt sql.NullTime
		var metadataJSON []byte

		err := rows.Scan(
			&relation.BusinessID,
			&relation.PayeeID,
			&relation.AccountID,
			&relation.DisplayName,
			&contactEmail,
			&contactPhone,
			&notes,
			&relation.IsActive,
			&relation.IsFavorite,
			pq.Array(&relation.Tags),
			&relation.AddedAt,
			&addedBy,
			&lastUsedAt,
			&updatedAt,
			&metadataJSON,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan business-payee relation: %w", err)
		}

		// Handle nullable fields
		relation.ContactEmail = contactEmail.String
		relation.ContactPhone = contactPhone.String
		relation.Notes = notes.String
		relation.AddedBy = addedBy.String
		if lastUsedAt.Valid {
			relation.LastUsedAt = &lastUsedAt.Time
		}
		if updatedAt.Valid {
			relation.UpdatedAt = updatedAt.Time
		}

		// Deserialize JSONB
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &relation.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		relations = append(relations, &relation)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating business-payee relations: %w", err)
	}

	return relations, nil
}

// UpdateRelation updates a business-payee relation
func (r *PgBusinessPayeeRelationRepository) UpdateRelation(ctx context.Context, relation *payee.BusinessPayeeRelation) error {
	fmt.Printf("[PgBusinessPayeeRelationRepository] UpdateRelation called - BusinessID: %s, PayeeID: %s, AccountID: %s\n",
		relation.BusinessID, relation.PayeeID, relation.AccountID)

	query := `
		UPDATE xcentro.business_payee_relation
		SET display_name = $4, contact_email = $5, contact_phone = $6, notes = $7,
		    is_active = $8, is_favorite = $9, tags = $10, metadata = $11
		WHERE business_id = $1 AND payee_id = $2 AND account_id = $3
	`

	var metadataJSON []byte
	var err error

	if relation.Metadata != nil {
		metadataJSON, err = json.Marshal(relation.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	result, err := r.querier.Exec(ctx, query,
		relation.BusinessID,
		relation.PayeeID,
		relation.AccountID,
		relation.DisplayName,
		relation.ContactEmail,
		relation.ContactPhone,
		relation.Notes,
		relation.IsActive,
		relation.IsFavorite,
		pq.Array(relation.Tags),
		metadataJSON,
	)

	if err != nil {
		return fmt.Errorf("failed to update business-payee relation: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("business-payee relation not found")
	}

	return nil
}

// DeleteRelation deletes (soft delete) a business-payee relation
func (r *PgBusinessPayeeRelationRepository) DeleteRelation(ctx context.Context, businessID, payeeID, accountID string) error {
	fmt.Printf("[PgBusinessPayeeRelationRepository] DeleteRelation called - BusinessID: %s, PayeeID: %s, AccountID: %s\n",
		businessID, payeeID, accountID)

	// Soft delete: set is_active = FALSE
	query := `
		UPDATE xcentro.business_payee_relation
		SET is_active = FALSE
		WHERE business_id = $1 AND payee_id = $2 AND account_id = $3
	`

	result, err := r.querier.Exec(ctx, query, businessID, payeeID, accountID)
	if err != nil {
		return fmt.Errorf("failed to delete business-payee relation: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("business-payee relation not found")
	}

	return nil
}

// RelationExists checks if a business-payee relation exists
func (r *PgBusinessPayeeRelationRepository) RelationExists(ctx context.Context, businessID, payeeID, accountID string) (bool, error) {
	fmt.Printf("[PgBusinessPayeeRelationRepository] RelationExists called - BusinessID: %s, PayeeID: %s, AccountID: %s\n",
		businessID, payeeID, accountID)

	query := `
		SELECT EXISTS(
			SELECT 1 FROM xcentro.business_payee_relation
			WHERE business_id = $1 AND payee_id = $2 AND account_id = $3
		)
	`

	var exists bool
	err := r.querier.QueryRow(ctx, query, businessID, payeeID, accountID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check business-payee relation existence: %w", err)
	}

	return exists, nil
}

// GetPayeeCount returns the count of active payees for a business
func (r *PgBusinessPayeeRelationRepository) GetPayeeCount(ctx context.Context, businessID string) (int, error) {
	fmt.Printf("[PgBusinessPayeeRelationRepository] GetPayeeCount called - BusinessID: %s\n", businessID)

	query := `
		SELECT COUNT(DISTINCT (payee_id, account_id))
		FROM xcentro.business_payee_relation
		WHERE business_id = $1 AND is_active = TRUE
	`

	var count int
	err := r.querier.QueryRow(ctx, query, businessID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get payee count: %w", err)
	}

	return count, nil
}

// GetPayeeCountByType returns the count of active payees of a specific type for a business
func (r *PgBusinessPayeeRelationRepository) GetPayeeCountByType(ctx context.Context, businessID string, payeeType payee.PayeeType) (int, error) {
	fmt.Printf("[PgBusinessPayeeRelationRepository] GetPayeeCountByType called - BusinessID: %s, PayeeType: %s\n", businessID, payeeType)

	query := `
		SELECT COUNT(DISTINCT (r.payee_id, r.account_id))
		FROM xcentro.business_payee_relation r
		INNER JOIN xcentro.payee_master m ON r.payee_id = m.payee_id
		WHERE r.business_id = $1 AND r.is_active = TRUE AND m.payee_type = $2
	`

	var count int
	err := r.querier.QueryRow(ctx, query, businessID, payeeType).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get payee count by type: %w", err)
	}

	return count, nil
}
