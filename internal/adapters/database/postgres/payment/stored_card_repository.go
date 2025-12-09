package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/payment"
)

// PgStoredCardRepository implements payment.StoredCardRepository using PostgreSQL
type PgStoredCardRepository struct {
	querier client.Querier
}

// NewPgStoredCardRepository creates a new PostgreSQL stored card repository
func NewPgStoredCardRepository(db client.DB) *PgStoredCardRepository {
	return &PgStoredCardRepository{
		querier: db,
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgStoredCardRepository) WithTx(tx interface{}) payment.StoredCardRepository {
	return &PgStoredCardRepository{
		querier: tx.(client.Tx),
	}
}

// CreateStoredCard creates a new stored card record
func (r *PgStoredCardRepository) CreateStoredCard(ctx context.Context, card *payment.StoredCard) error {
	fmt.Printf("[PgStoredCardRepository] CreateStoredCard called - BusinessID: %s, CardType: %s, Last4: %s\n",
		card.BusinessID, card.CardType, card.LastFourDigits)

	query := `
		INSERT INTO xcentro.stored_cards (
			card_id, business_id, token, last_four_digits, card_type, card_category,
			expiry_month, expiry_year, issuer_bank, card_holder_name,
			is_active, gateway_used, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING added_at
	`

	var metadataJSON []byte
	if card.Metadata != nil {
		var err error
		metadataJSON, err = json.Marshal(card.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	// Handle empty gateway_used (Phase 1) by using NULL
	var gatewayUsed interface{}
	if card.GatewayUsed == "" {
		gatewayUsed = nil
	} else {
		gatewayUsed = card.GatewayUsed
	}

	err := r.querier.QueryRow(ctx, query,
		card.CardID,
		card.BusinessID,
		card.Token,
		card.LastFourDigits,
		card.CardType,
		card.CardCategory,
		card.ExpiryMonth,
		card.ExpiryYear,
		card.IssuerBank,
		card.CardHolderName,
		card.IsActive,
		gatewayUsed,
		metadataJSON,
	).Scan(&card.AddedAt)

	if err != nil {
		return fmt.Errorf("failed to create stored card: %w", err)
	}

	return nil
}

// GetStoredCardByID retrieves a stored card by ID (regardless of active status)
// Callers should check IsActive flag if needed
func (r *PgStoredCardRepository) GetStoredCardByID(ctx context.Context, cardID string) (*payment.StoredCard, error) {
	fmt.Printf("[PgStoredCardRepository] GetStoredCardByID called - CardID: %s\n", cardID)

	query := `
		SELECT card_id, business_id, token, last_four_digits, card_type, card_category,
		       expiry_month, expiry_year, issuer_bank, card_holder_name,
		       is_active, is_failed_tokenization, failure_reason, gateway_used, added_at, last_used_at, metadata
		FROM xcentro.stored_cards
		WHERE card_id = $1
	`

	var card payment.StoredCard
	var lastUsedAt *time.Time
	var failureReason *string
	var gatewayUsed *string
	var metadataJSON []byte

	err := r.querier.QueryRow(ctx, query, cardID).Scan(
		&card.CardID,
		&card.BusinessID,
		&card.Token,
		&card.LastFourDigits,
		&card.CardType,
		&card.CardCategory,
		&card.ExpiryMonth,
		&card.ExpiryYear,
		&card.IssuerBank,
		&card.CardHolderName,
		&card.IsActive,
		&card.IsFailedTokenization,
		&failureReason,
		&gatewayUsed,
		&card.AddedAt,
		&lastUsedAt,
		&metadataJSON,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get stored card: %w", err)
	}

	card.LastUsedAt = lastUsedAt
	if failureReason != nil {
		card.FailureReason = *failureReason
	}
	if gatewayUsed != nil {
		card.GatewayUsed = payment.GatewayType(*gatewayUsed)
	}

	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &card.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &card, nil
}

// GetStoredCardsByBusinessID retrieves cards for a business
// When showInactive = false: only active cards
// When showInactive = true: active + user-deactivated + pending (1 hour) + failed (2 days)
func (r *PgStoredCardRepository) GetStoredCardsByBusinessID(ctx context.Context, businessID string, showInactive bool) ([]*payment.StoredCard, error) {
	fmt.Printf("[PgStoredCardRepository] GetStoredCardsByBusinessID called - BusinessID: %s, ShowInactive: %t\n", businessID, showInactive)

	var query string
	if showInactive {
		// Show: Active + Pending (1 hour window) + Failed (2 day window) + User-Deactivated (has token)
		query = `
			SELECT card_id, business_id, token, last_four_digits, card_type, card_category,
			       expiry_month, expiry_year, issuer_bank, card_holder_name,
			       is_active, is_failed_tokenization, failure_reason, gateway_used, added_at, last_used_at, metadata
			FROM xcentro.stored_cards
			WHERE business_id = $1
			  AND (
			      is_active = TRUE
			      OR (is_failed_tokenization = TRUE AND added_at > NOW() - INTERVAL '2 days')
			      OR (is_active = FALSE AND is_failed_tokenization = FALSE AND added_at > NOW() - INTERVAL '1 hour')
			      OR (is_active = FALSE AND is_failed_tokenization = FALSE AND token != '')
			  )
			ORDER BY
			  CASE
			    WHEN is_active = TRUE THEN 1
			    WHEN is_active = FALSE AND is_failed_tokenization = FALSE AND token != '' THEN 2
			    WHEN is_active = FALSE AND is_failed_tokenization = FALSE THEN 3
			    ELSE 4
			  END,
			  added_at DESC
		`
	} else {
		// Show only active cards
		query = `
			SELECT card_id, business_id, token, last_four_digits, card_type, card_category,
			       expiry_month, expiry_year, issuer_bank, card_holder_name,
			       is_active, is_failed_tokenization, failure_reason, gateway_used, added_at, last_used_at, metadata
			FROM xcentro.stored_cards
			WHERE business_id = $1 AND is_active = TRUE
			ORDER BY added_at DESC
		`
	}

	rows, err := r.querier.Query(ctx, query, businessID)
	if err != nil {
		return nil, fmt.Errorf("failed to query stored cards: %w", err)
	}
	defer rows.Close()

	var cards []*payment.StoredCard
	for rows.Next() {
		var card payment.StoredCard
		var lastUsedAt *time.Time
		var failureReason *string
		var gatewayUsed *string
		var metadataJSON []byte

		err := rows.Scan(
			&card.CardID,
			&card.BusinessID,
			&card.Token,
			&card.LastFourDigits,
			&card.CardType,
			&card.CardCategory,
			&card.ExpiryMonth,
			&card.ExpiryYear,
			&card.IssuerBank,
			&card.CardHolderName,
			&card.IsActive,
			&card.IsFailedTokenization,
			&failureReason,
			&gatewayUsed,
			&card.AddedAt,
			&lastUsedAt,
			&metadataJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan stored card: %w", err)
		}

		card.LastUsedAt = lastUsedAt
		if failureReason != nil {
			card.FailureReason = *failureReason
		}
		if gatewayUsed != nil {
			card.GatewayUsed = payment.GatewayType(*gatewayUsed)
		}

		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &card.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		cards = append(cards, &card)
	}

	return cards, nil
}

// UpdateStoredCard updates an existing stored card record
// Used for Phase 2 of tokenization to update token, issuer bank, and status
func (r *PgStoredCardRepository) UpdateStoredCard(ctx context.Context, card *payment.StoredCard) error {
	fmt.Printf("[PgStoredCardRepository] UpdateStoredCard called - CardID: %s\n", card.CardID)

	query := `
		UPDATE xcentro.stored_cards
		SET token = $1,
		    issuer_bank = $2,
		    is_active = $3,
		    gateway_used = $4,
		    metadata = $5
		WHERE card_id = $6
	`

	var metadataJSON []byte
	if card.Metadata != nil {
		var err error
		metadataJSON, err = json.Marshal(card.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	// Handle empty gateway_used by using NULL
	var gatewayUsed interface{}
	if card.GatewayUsed == "" {
		gatewayUsed = nil
	} else {
		gatewayUsed = card.GatewayUsed
	}

	_, err := r.querier.Exec(ctx, query,
		card.Token,
		card.IssuerBank,
		card.IsActive,
		gatewayUsed,
		metadataJSON,
		card.CardID,
	)

	if err != nil {
		return fmt.Errorf("failed to update stored card: %w", err)
	}

	return nil
}

// DeactivateCard marks a card as inactive (soft delete)
func (r *PgStoredCardRepository) DeactivateCard(ctx context.Context, cardID string) error {
	fmt.Printf("[PgStoredCardRepository] DeactivateCard called - CardID: %s\n", cardID)

	query := `UPDATE xcentro.stored_cards SET is_active = FALSE WHERE card_id = $1`
	_, err := r.querier.Exec(ctx, query, cardID)
	return err
}

// UpdateLastUsedAt updates the last used timestamp
func (r *PgStoredCardRepository) UpdateLastUsedAt(ctx context.Context, cardID string, usedAt time.Time) error {
	fmt.Printf("[PgStoredCardRepository] UpdateLastUsedAt called - CardID: %s, UsedAt: %s\n", cardID, usedAt.Format(time.RFC3339))

	query := `UPDATE xcentro.stored_cards SET last_used_at = $1 WHERE card_id = $2`
	_, err := r.querier.Exec(ctx, query, usedAt, cardID)
	return err
}

// UpdateCardStatus updates card active status and failure reason
// Used when tokenization fails (FURL callback) to mark card inactive with reason
func (r *PgStoredCardRepository) UpdateCardStatus(ctx context.Context, cardID string, isActive bool, failureReason string) error {
	fmt.Printf("[PgStoredCardRepository] UpdateCardStatus called - CardID: %s, IsActive: %t, FailureReason: %s\n",
		cardID, isActive, failureReason)

	// When marking as inactive with failure reason, set is_failed_tokenization = true
	isFailedTokenization := !isActive && failureReason != ""

	query := `
		UPDATE xcentro.stored_cards
		SET is_active = $1,
		    failure_reason = $2,
		    is_failed_tokenization = $3
		WHERE card_id = $4
	`

	_, err := r.querier.Exec(ctx, query, isActive, failureReason, isFailedTokenization, cardID)
	if err != nil {
		return fmt.Errorf("failed to update card status: %w", err)
	}

	return nil
}
