package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/notification"
	"xcentro.in/internal/shared/logger"
)

// PgNotificationPreferencesRepository implements notification.NotificationPreferencesRepository using PostgreSQL
type PgNotificationPreferencesRepository struct {
	querier client.Querier
	logger  *logger.Logger
}

// NewPgNotificationPreferencesRepository creates a new PostgreSQL notification preferences repository
func NewPgNotificationPreferencesRepository(db client.DB) *PgNotificationPreferencesRepository {
	return &PgNotificationPreferencesRepository{
		querier: db,
		logger:  logger.GetDefault(),
	}
}

// GetPreferences retrieves user's notification preferences
func (r *PgNotificationPreferencesRepository) GetPreferences(ctx context.Context, userID string) (*notification.NotificationPreferences, error) {
	query := `
		SELECT user_id, email_enabled, push_enabled, messages_enabled,
		       category_preferences, created_at, updated_at
		FROM xcentro.notification_preferences
		WHERE user_id = $1
	`

	var prefs notification.NotificationPreferences
	var categoryPrefsJSON []byte
	var updatedAt *time.Time

	err := r.querier.QueryRow(ctx, query, userID).Scan(
		&prefs.UserID,
		&prefs.EmailEnabled,
		&prefs.PushEnabled,
		&prefs.MessagesEnabled,
		&categoryPrefsJSON,
		&prefs.CreatedAt,
		&updatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			// Return default preferences if not found
			return &notification.NotificationPreferences{
				UserID:              userID,
				EmailEnabled:        true,
				PushEnabled:         true,
				MessagesEnabled:     true,
				CategoryPreferences: getDefaultCategoryPreferences(),
				CreatedAt:           time.Now(),
			}, nil
		}
		return nil, fmt.Errorf("failed to get notification preferences: %w", err)
	}

	prefs.UpdatedAt = updatedAt

	if err := json.Unmarshal(categoryPrefsJSON, &prefs.CategoryPreferences); err != nil {
		return nil, fmt.Errorf("failed to unmarshal category preferences: %w", err)
	}

	return &prefs, nil
}

// UpsertPreferences creates or updates user's notification preferences
func (r *PgNotificationPreferencesRepository) UpsertPreferences(ctx context.Context, prefs *notification.NotificationPreferences) error {
	query := `
		INSERT INTO xcentro.notification_preferences (
			user_id, email_enabled, push_enabled, messages_enabled, category_preferences
		) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id) DO UPDATE SET
			email_enabled = EXCLUDED.email_enabled,
			push_enabled = EXCLUDED.push_enabled,
			messages_enabled = EXCLUDED.messages_enabled,
			category_preferences = EXCLUDED.category_preferences,
			updated_at = NOW()
		RETURNING created_at, updated_at
	`

	categoryPrefsJSON, err := json.Marshal(prefs.CategoryPreferences)
	if err != nil {
		return fmt.Errorf("failed to marshal category preferences: %w", err)
	}

	var updatedAt *time.Time
	err = r.querier.QueryRow(ctx, query,
		prefs.UserID,
		prefs.EmailEnabled,
		prefs.PushEnabled,
		prefs.MessagesEnabled,
		categoryPrefsJSON,
	).Scan(&prefs.CreatedAt, &updatedAt)

	if err != nil {
		return fmt.Errorf("failed to upsert notification preferences: %w", err)
	}

	prefs.UpdatedAt = updatedAt
	return nil
}

// getDefaultCategoryPreferences returns default category preferences
func getDefaultCategoryPreferences() map[string]interface{} {
	return map[string]interface{}{
		"announcements": true,
		"rewards":       true,
		"payments":      true,
		"payees":        true,
		"approvals":     true,
		"security":      true,
		"insights":      true,
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgNotificationPreferencesRepository) WithTx(tx client.Tx) notification.NotificationPreferencesRepository {
	return &PgNotificationPreferencesRepository{
		querier: tx,
		logger:  r.logger,
	}
}
