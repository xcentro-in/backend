package opsauth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/opsauth"
	"xcentro.in/internal/shared/logger"
)

type PgOpsSessionRepository struct {
	querier client.Querier
}

func NewPgOpsSessionRepository(db client.DB) *PgOpsSessionRepository {
	return &PgOpsSessionRepository{querier: db}
}

func (r *PgOpsSessionRepository) WithTx(tx interface{}) opsauth.OpsSessionRepository {
	return &PgOpsSessionRepository{querier: tx.(client.Tx)}
}

func (r *PgOpsSessionRepository) CreateSession(
	ctx context.Context,
	opsUserID string,
	refreshTokenHash string,
	oidcRefreshToken string,
	expiresAt time.Time,
	userAgent string,
) (string, error) {
	logger.Infof("Creating ops session for user: %s", opsUserID)

	query := `
		INSERT INTO xcentro.ops_session_table (
			ops_user_id,
			refresh_token_hash,
			oidc_refresh_token,
			expires_at,
			user_agent
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING session_id
	`

	var sessionID string
	err := r.querier.QueryRow(
		ctx,
		query,
		opsUserID,
		refreshTokenHash,
		oidcRefreshToken,
		expiresAt,
		userAgent,
	).Scan(&sessionID)

	if err != nil {
		logger.Errorf("Failed to create ops session: %v", err)
		return "", fmt.Errorf("failed to create ops session: %w", err)
	}

	logger.Infof("Ops session created successfully: session_id=%s", sessionID)
	return sessionID, nil
}

func (r *PgOpsSessionRepository) GetSessionByTokenHash(
	ctx context.Context,
	opsUserID string,
	refreshTokenHash string,
) (*opsauth.OpsSession, error) {
	query := `
		SELECT session_id, ops_user_id, refresh_token_hash, oidc_refresh_token,
		       expires_at, user_agent, created_at, last_used_at
		FROM xcentro.ops_session_table
		WHERE ops_user_id = $1 AND refresh_token_hash = $2
	`

	var session opsauth.OpsSession

	err := r.querier.QueryRow(ctx, query, opsUserID, refreshTokenHash).Scan(
		&session.SessionID,
		&session.OpsUserID,
		&session.RefreshTokenHash,
		&session.OIDCRefreshToken,
		&session.ExpiresAt,
		&session.UserAgent,
		&session.CreatedAt,
		&session.LastUsedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get ops session: %w", err)
	}

	return &session, nil
}

func (r *PgOpsSessionRepository) UpdateLastUsed(
	ctx context.Context,
	opsUserID string,
	refreshTokenHash string,
) error {
	query := `
		UPDATE xcentro.ops_session_table
		SET last_used_at = NOW()
		WHERE ops_user_id = $1 AND refresh_token_hash = $2
	`

	cmdTag, err := r.querier.Exec(ctx, query, opsUserID, refreshTokenHash)
	if err != nil {
		return fmt.Errorf("failed to update last used: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("session not found")
	}

	return nil
}

func (r *PgOpsSessionRepository) UpdateOIDCRefreshToken(
	ctx context.Context,
	opsUserID string,
	refreshTokenHash string,
	newEncryptedOIDCRefreshToken string,
) error {
	query := `
		UPDATE xcentro.ops_session_table
		SET oidc_refresh_token = $3, last_used_at = NOW()
		WHERE ops_user_id = $1 AND refresh_token_hash = $2
	`

	cmdTag, err := r.querier.Exec(ctx, query, opsUserID, refreshTokenHash, newEncryptedOIDCRefreshToken)
	if err != nil {
		return fmt.Errorf("failed to update OIDC refresh token: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("session not found")
	}

	return nil
}

func (r *PgOpsSessionRepository) DeleteSession(
	ctx context.Context,
	opsUserID string,
	refreshTokenHash string,
) error {
	query := `
		DELETE FROM xcentro.ops_session_table
		WHERE ops_user_id = $1 AND refresh_token_hash = $2
	`

	cmdTag, err := r.querier.Exec(ctx, query, opsUserID, refreshTokenHash)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("session not found")
	}

	return nil
}

func (r *PgOpsSessionRepository) DeleteAllUserSessions(
	ctx context.Context,
	opsUserID string,
) ([]string, error) {
	query := `
		DELETE FROM xcentro.ops_session_table
		WHERE ops_user_id = $1
		RETURNING refresh_token_hash
	`

	rows, err := r.querier.Query(ctx, query, opsUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete all user sessions: %w", err)
	}
	defer rows.Close()

	var tokenHashes []string
	for rows.Next() {
		var tokenHash string
		if err := rows.Scan(&tokenHash); err != nil {
			return nil, fmt.Errorf("failed to scan token hash: %w", err)
		}
		tokenHashes = append(tokenHashes, tokenHash)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return tokenHashes, nil
}

func (r *PgOpsSessionRepository) GetAllUserSessions(
	ctx context.Context,
	opsUserID string,
) ([]*opsauth.OpsSession, error) {
	query := `
		SELECT session_id, ops_user_id, refresh_token_hash, oidc_refresh_token,
		       expires_at, user_agent, created_at, last_used_at
		FROM xcentro.ops_session_table
		WHERE ops_user_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.querier.Query(ctx, query, opsUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get all user sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*opsauth.OpsSession
	for rows.Next() {
		var session opsauth.OpsSession
		err := rows.Scan(
			&session.SessionID,
			&session.OpsUserID,
			&session.RefreshTokenHash,
			&session.OIDCRefreshToken,
			&session.ExpiresAt,
			&session.UserAgent,
			&session.CreatedAt,
			&session.LastUsedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}
		sessions = append(sessions, &session)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return sessions, nil
}
