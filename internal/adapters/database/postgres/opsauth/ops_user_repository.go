package opsauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/opsauth"
	"xcentro.in/internal/shared/logger"
)

type PgOpsUserRepository struct {
	querier client.Querier
}

func NewPgOpsUserRepository(db client.DB) *PgOpsUserRepository {
	return &PgOpsUserRepository{querier: db}
}

func (r *PgOpsUserRepository) WithTx(tx interface{}) opsauth.OpsUserRepository {
	return &PgOpsUserRepository{querier: tx.(client.Tx)}
}

func (r *PgOpsUserRepository) CreateOrUpdateUser(ctx context.Context, oid, email, name string, roles []string) (string, bool, error) {
	logger.Infof("Creating or updating ops user: oid=%s, email=%s, name=%s", oid, email, name)

	// Convert roles to JSONB
	rolesJSON, err := json.Marshal(roles)
	if err != nil {
		return "", false, fmt.Errorf("failed to marshal roles: %w", err)
	}

	// Use OID as the primary key - if it exists, update email/name/roles
	// This allows email changes for the same user (identified by OID)
	query := `
		INSERT INTO xcentro.ops_user_table (id, email, name, roles, status)
		VALUES ($1, $2, $3, $4, 'active')
		ON CONFLICT (id)
		DO UPDATE SET
			email = EXCLUDED.email,
			name = EXCLUDED.name,
			roles = EXCLUDED.roles,
			updated_at = NOW()
		RETURNING id, (xmax = 0) AS inserted
	`

	var userID string
	var inserted bool

	err = r.querier.QueryRow(ctx, query, oid, email, name, rolesJSON).Scan(&userID, &inserted)
	if err != nil {
		logger.Errorf("Failed to create or update ops user: %v", err)
		return "", false, fmt.Errorf("failed to create or update ops user: %w", err)
	}

	logger.Infof("Ops user created/updated successfully: id=%s, inserted=%v", userID, inserted)
	return userID, inserted, nil
}

func (r *PgOpsUserRepository) GetUserByID(ctx context.Context, userID string) (*opsauth.OpsUser, error) {
	query := `
		SELECT id, name, email, roles, status,
		       created_at::text, updated_at::text
		FROM xcentro.ops_user_table
		WHERE id = $1
	`

	var user opsauth.OpsUser
	var rolesJSON []byte

	err := r.querier.QueryRow(ctx, query, userID).Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&rolesJSON,
		&user.Status,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get ops user by ID: %w", err)
	}

	// Unmarshal roles from JSONB
	if err := json.Unmarshal(rolesJSON, &user.Roles); err != nil {
		return nil, fmt.Errorf("failed to unmarshal roles: %w", err)
	}

	return &user, nil
}

func (r *PgOpsUserRepository) GetUserByEmail(ctx context.Context, email string) (*opsauth.OpsUser, error) {
	query := `
		SELECT id, name, email, roles, status,
		       created_at::text, updated_at::text
		FROM xcentro.ops_user_table
		WHERE email = $1
	`

	var user opsauth.OpsUser
	var rolesJSON []byte

	err := r.querier.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&rolesJSON,
		&user.Status,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get ops user by email: %w", err)
	}

	// Unmarshal roles from JSONB
	if err := json.Unmarshal(rolesJSON, &user.Roles); err != nil {
		return nil, fmt.Errorf("failed to unmarshal roles: %w", err)
	}

	return &user, nil
}

func (r *PgOpsUserRepository) UpdateUserRoles(ctx context.Context, userID string, roles []string) error {
	rolesJSON, err := json.Marshal(roles)
	if err != nil {
		return fmt.Errorf("failed to marshal roles: %w", err)
	}

	query := `
		UPDATE xcentro.ops_user_table
		SET roles = $2, updated_at = NOW()
		WHERE id = $1
	`

	cmdTag, err := r.querier.Exec(ctx, query, userID, rolesJSON)
	if err != nil {
		return fmt.Errorf("failed to update ops user roles: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("ops user not found")
	}

	return nil
}

func (r *PgOpsUserRepository) UpdateUserStatus(ctx context.Context, userID string, status string) error {
	query := `
		UPDATE xcentro.ops_user_table
		SET status = $2, updated_at = NOW()
		WHERE id = $1
	`

	cmdTag, err := r.querier.Exec(ctx, query, userID, status)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23514" {
			return fmt.Errorf("invalid status value")
		}
		return fmt.Errorf("failed to update ops user status: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("ops user not found")
	}

	return nil
}
