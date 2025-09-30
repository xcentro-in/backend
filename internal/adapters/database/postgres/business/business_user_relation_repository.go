package business

import (
	"context"
	"fmt"

	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/shared"
)

// PgBusinessUserRelationRepository implements business.BusinessUserRelationRepository using PostgreSQL
type PgBusinessUserRelationRepository struct {
	querier client.Querier
}

// NewPgBusinessUserRelationRepository creates a new PostgreSQL business user relation repository
func NewPgBusinessUserRelationRepository(db client.DB) *PgBusinessUserRelationRepository {
	return &PgBusinessUserRelationRepository{
		querier: db,
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgBusinessUserRelationRepository) WithTx(tx client.Tx) business.BusinessUserRelationRepository {
	return &PgBusinessUserRelationRepository{
		querier: tx,
	}
}

func (r *PgBusinessUserRelationRepository) UpdateKYBInitiated(ctx context.Context, uid string, bid string, initiated bool) error {
	query := `
		UPDATE xcentro.business_user_relation 
		SET kyb_initiated = $3 
		WHERE user_id = $1 AND bid = $2
	`

	result, err := r.querier.Exec(ctx, query, uid, bid, initiated)
	if err != nil {
		return fmt.Errorf("error updating KYB initiated status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("user-business relation not found")
	}

	return nil
}

// FindByUser finds all business relations for a user
func (r *PgBusinessUserRelationRepository) FindByUser(ctx context.Context, userID string) ([]*business.BusinessUserRelation, error) {
	query := `
		SELECT 
			user_id, bid, role, role_claim, added_at, kyb_initiated, updated_at
		FROM xcentro.business_user_relation 
		WHERE user_id = $1
	`

	rows, err := r.querier.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("error finding business relations for user: %w", err)
	}
	defer rows.Close()

	var relations []*business.BusinessUserRelation

	for rows.Next() {
		var rel business.BusinessUserRelation
		var role, roleClaim int

		err := rows.Scan(
			&rel.UserID, &rel.BID, &role, &roleClaim,
			&rel.AddedAt, &rel.KybInitiated, &rel.Timestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning business relation: %w", err)
		}

		rel.Role = shared.UserRole(role)
		rel.RoleClaim = shared.UserRole(roleClaim)
		relations = append(relations, &rel)
	}

	return relations, nil
}

// FindByBusiness finds all user relations for a business
func (r *PgBusinessUserRelationRepository) FindByBusiness(ctx context.Context, bid string) ([]*business.BusinessUserRelation, error) {
	query := `
		SELECT 
			user_id, bid, role, role_claim, added_at, kyb_initiated, updated_at
		FROM xcentro.business_user_relation 
		WHERE bid = $1
	`

	rows, err := r.querier.Query(ctx, query, bid)
	if err != nil {
		return nil, fmt.Errorf("error finding user relations for business: %w", err)
	}
	defer rows.Close()

	var relations []*business.BusinessUserRelation

	for rows.Next() {
		var rel business.BusinessUserRelation
		var role, roleClaim int

		err := rows.Scan(
			&rel.UserID, &rel.BID, &role, &roleClaim,
			&rel.AddedAt, &rel.KybInitiated, &rel.Timestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning business relation: %w", err)
		}

		rel.Role = shared.UserRole(role)
		rel.RoleClaim = shared.UserRole(roleClaim)
		relations = append(relations, &rel)
	}

	return relations, nil
}

// AddUserToBusiness adds a user to a business with a specific role
func (r *PgBusinessUserRelationRepository) AddUserToBusiness(ctx context.Context, relation *business.BusinessUserRelation) error {
	query := `
		INSERT INTO xcentro.business_user_relation 
			(user_id, bid, role, role_claim, kyb_initiated)
		VALUES ($1, $2, $3, $4, $5)
	`

	_, err := r.querier.Exec(ctx, query,
		relation.UserID, relation.BID, int(relation.Role),
		int(relation.RoleClaim), relation.KybInitiated,
	)

	if err != nil {
		return fmt.Errorf("error adding user to business: %w", err)
	}

	return nil
}

// UpdateUserRole updates a user's role in a business
func (r *PgBusinessUserRelationRepository) UpdateUserRole(ctx context.Context, userID string, bid string, newRole shared.UserRole) error {
	query := `
		UPDATE xcentro.business_user_relation 
		SET role = $3 
		WHERE user_id = $1 AND bid = $2
	`

	result, err := r.querier.Exec(ctx, query, userID, bid, int(newRole))
	if err != nil {
		return fmt.Errorf("error updating user role: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("user-business relation not found")
	}

	return nil
}

// RemoveUserFromBusiness removes a user from a business
func (r *PgBusinessUserRelationRepository) RemoveUserFromBusiness(ctx context.Context, userID string, bid string) error {
	query := `
		DELETE FROM xcentro.business_user_relation 
		WHERE user_id = $1 AND bid = $2
	`

	result, err := r.querier.Exec(ctx, query, userID, bid)
	if err != nil {
		return fmt.Errorf("error removing user from business: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("user-business relation not found")
	}

	return nil
}
