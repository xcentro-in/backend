package payee

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/payee"
)

// PgPayeeMasterRepository implements payee.PayeeMasterRepository using PostgreSQL
type PgPayeeMasterRepository struct {
	querier client.Querier
}

// NewPgPayeeMasterRepository creates a new PostgreSQL payee master repository
func NewPgPayeeMasterRepository(db client.DB) payee.PayeeMasterRepository {
	return &PgPayeeMasterRepository{
		querier: db,
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgPayeeMasterRepository) WithTx(tx client.Tx) payee.PayeeMasterRepository {
	return &PgPayeeMasterRepository{
		querier: tx,
	}
}

// Create creates a new payee master entry
func (r *PgPayeeMasterRepository) Create(ctx context.Context, master *payee.PayeeMaster) error {
	query := `
		INSERT INTO xcentro.payee_master (
			payee_id, payee_type, display_name, is_active
		) VALUES ($1, $2, $3, $4)
		RETURNING created_at, updated_at
	`

	err := r.querier.QueryRow(ctx, query,
		master.PayeeID,
		master.PayeeType,
		master.DisplayName,
		master.IsActive,
	).Scan(&master.CreatedAt, &master.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create payee master: %w", err)
	}

	return nil
}

// FindByID retrieves a payee master entry by ID
func (r *PgPayeeMasterRepository) FindByID(ctx context.Context, payeeID string) (*payee.PayeeMaster, error) {
	query := `
		SELECT payee_id, payee_type, display_name, is_active, created_at, updated_at
		FROM xcentro.payee_master
		WHERE payee_id = $1
	`

	var master payee.PayeeMaster
	err := r.querier.QueryRow(ctx, query, payeeID).Scan(
		&master.PayeeID,
		&master.PayeeType,
		&master.DisplayName,
		&master.IsActive,
		&master.CreatedAt,
		&master.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("payee master not found")
		}
		return nil, fmt.Errorf("failed to find payee master: %w", err)
	}

	return &master, nil
}

// Update updates a payee master entry
func (r *PgPayeeMasterRepository) Update(ctx context.Context, master *payee.PayeeMaster) error {
	query := `
		UPDATE xcentro.payee_master
		SET display_name = $2, is_active = $3
		WHERE payee_id = $1
	`

	result, err := r.querier.Exec(ctx, query,
		master.PayeeID,
		master.DisplayName,
		master.IsActive,
	)

	if err != nil {
		return fmt.Errorf("failed to update payee master: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("payee master not found")
	}

	return nil
}

// Delete deletes a payee master entry (soft delete by setting is_active = false)
func (r *PgPayeeMasterRepository) Delete(ctx context.Context, payeeID string) error {
	query := `
		UPDATE xcentro.payee_master
		SET is_active = false
		WHERE payee_id = $1
	`

	result, err := r.querier.Exec(ctx, query, payeeID)

	if err != nil {
		return fmt.Errorf("failed to delete payee master: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("payee master not found")
	}

	return nil
}

// FindByType retrieves payee master entries by type with pagination
func (r *PgPayeeMasterRepository) FindByType(ctx context.Context, payeeType payee.PayeeType, limit, offset int) ([]*payee.PayeeMaster, error) {
	query := `
		SELECT payee_id, payee_type, display_name, is_active, created_at, updated_at
		FROM xcentro.payee_master
		WHERE payee_type = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.querier.Query(ctx, query, payeeType, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to find payees by type: %w", err)
	}
	defer rows.Close()

	var masters []*payee.PayeeMaster
	for rows.Next() {
		var m payee.PayeeMaster
		err := rows.Scan(
			&m.PayeeID,
			&m.PayeeType,
			&m.DisplayName,
			&m.IsActive,
			&m.CreatedAt,
			&m.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan payee master: %w", err)
		}
		masters = append(masters, &m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating payee masters: %w", err)
	}

	return masters, nil
}

// CountByType counts payee master entries by type
func (r *PgPayeeMasterRepository) CountByType(ctx context.Context, payeeType payee.PayeeType) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM xcentro.payee_master
		WHERE payee_type = $1
	`

	var count int
	err := r.querier.QueryRow(ctx, query, payeeType).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count payees by type: %w", err)
	}

	return count, nil
}
