package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/auth"
)

// PgSessionRepository implements auth.SessionRepository using PostgreSQL
type PgSessionRepository struct {
	querier client.Querier
}

// NewPgSessionRepository creates a new PostgreSQL session repository
func NewPgSessionRepository(db client.DB) *PgSessionRepository {
	return &PgSessionRepository{
		querier: db,
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgSessionRepository) WithTx(tx interface{}) auth.SessionRepository {
	return &PgSessionRepository{
		querier: tx.(client.Tx),
	}
}

// CreateSession creates a new refresh token session with device information
func (r *PgSessionRepository) CreateSession(ctx context.Context, userID, tokenHash string, expiresAt time.Time, userAgent string, deviceType string, deviceVersion int) (string, error) {
	query := `
		INSERT INTO xcentro.user_tokens (user_id, refresh_token_hash, expires_at, user_agent, device_type, device_version)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING session_id
	`

	var sessionID string
	err := r.querier.QueryRow(ctx, query, userID, tokenHash, expiresAt, userAgent, deviceType, deviceVersion).Scan(&sessionID)
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	return sessionID, nil
}

// GetSessionByTokenHash retrieves a session by user ID and refresh token hash
func (r *PgSessionRepository) GetSessionByTokenHash(ctx context.Context, userID, tokenHash string) (*auth.Session, error) {
	query := `
		SELECT session_id, user_id, refresh_token_hash, expires_at, created_at, last_used_at, user_agent
		FROM xcentro.user_tokens
		WHERE user_id = $1 AND refresh_token_hash = $2
	`

	var session auth.Session
	var userAgent *string

	err := r.querier.QueryRow(ctx, query, userID, tokenHash).Scan(
		&session.SessionID,
		&session.UserID,
		&session.RefreshTokenHash,
		&session.ExpiresAt,
		&session.CreatedAt,
		&session.LastUsedAt,
		&userAgent,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("session not found")
		}
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	if userAgent != nil {
		session.UserAgent = *userAgent
	}

	return &session, nil
}

// UpdateLastUsed updates the last_used_at timestamp for a session
func (r *PgSessionRepository) UpdateLastUsed(ctx context.Context, userID, tokenHash string) error {
	query := `
		UPDATE xcentro.user_tokens
		SET last_used_at = NOW()
		WHERE user_id = $1 AND refresh_token_hash = $2
	`

	_, err := r.querier.Exec(ctx, query, userID, tokenHash)
	if err != nil {
		return fmt.Errorf("failed to update last used: %w", err)
	}

	return nil
}

// DeleteSession deletes a specific session by user ID and token hash
func (r *PgSessionRepository) DeleteSession(ctx context.Context, userID, tokenHash string) error {
	query := `
		DELETE FROM xcentro.user_tokens
		WHERE user_id = $1 AND refresh_token_hash = $2
	`

	_, err := r.querier.Exec(ctx, query, userID, tokenHash)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	return nil
}

// DeleteAllUserSessions deletes all sessions for a user and returns their token hashes
func (r *PgSessionRepository) DeleteAllUserSessions(ctx context.Context, userID string) ([]string, error) {
	query := `
		DELETE FROM xcentro.user_tokens
		WHERE user_id = $1
		RETURNING refresh_token_hash
	`

	rows, err := r.querier.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete user sessions: %w", err)
	}
	defer rows.Close()

	var tokenHashes []string
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			return nil, fmt.Errorf("failed to scan token hash: %w", err)
		}
		tokenHashes = append(tokenHashes, hash)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return tokenHashes, nil
}

// GetAllUserSessions retrieves all sessions for a user
func (r *PgSessionRepository) GetAllUserSessions(ctx context.Context, userID string) ([]*auth.Session, error) {
	query := `
		SELECT session_id, user_id, refresh_token_hash, expires_at, created_at, last_used_at, user_agent
		FROM xcentro.user_tokens
		WHERE user_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.querier.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*auth.Session
	for rows.Next() {
		var session auth.Session
		var userAgent *string

		err := rows.Scan(
			&session.SessionID,
			&session.UserID,
			&session.RefreshTokenHash,
			&session.ExpiresAt,
			&session.CreatedAt,
			&session.LastUsedAt,
			&userAgent,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}

		if userAgent != nil {
			session.UserAgent = *userAgent
		}

		sessions = append(sessions, &session)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return sessions, nil
}

// CountSessionsByDevice counts active sessions for a user by device type
func (r *PgSessionRepository) CountSessionsByDevice(ctx context.Context, userID string, deviceType string) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM xcentro.user_tokens
		WHERE user_id = $1 AND device_type = $2 AND expires_at > NOW()
	`

	var count int
	err := r.querier.QueryRow(ctx, query, userID, deviceType).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count sessions by device: %w", err)
	}

	return count, nil
}

// DeleteOldestSessionByDevice deletes the oldest session for a user by device type
func (r *PgSessionRepository) DeleteOldestSessionByDevice(ctx context.Context, userID string, deviceType string) error {
	query := `
		DELETE FROM xcentro.user_tokens
		WHERE session_id = (
			SELECT session_id
			FROM xcentro.user_tokens
			WHERE user_id = $1 AND device_type = $2
			ORDER BY created_at ASC
			LIMIT 1
		)
	`

	_, err := r.querier.Exec(ctx, query, userID, deviceType)
	if err != nil {
		return fmt.Errorf("failed to delete oldest session by device: %w", err)
	}

	return nil
}
