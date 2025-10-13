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
			business_id, token, last_four_digits, card_type, card_category,
			expiry_month, expiry_year, issuer_bank, card_holder_name,
			is_active, gateway_used, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING card_id, added_at
	`

	var metadataJSON []byte
	if card.Metadata != nil {
		var err error
		metadataJSON, err = json.Marshal(card.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	err := r.querier.QueryRow(ctx, query,
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
		card.GatewayUsed,
		metadataJSON,
	).Scan(&card.CardID, &card.AddedAt)

	if err != nil {
		return fmt.Errorf("failed to create stored card: %w", err)
	}

	return nil
}

// GetStoredCardByID retrieves a stored card by ID
func (r *PgStoredCardRepository) GetStoredCardByID(ctx context.Context, cardID string) (*payment.StoredCard, error) {
	fmt.Printf("[PgStoredCardRepository] GetStoredCardByID called - CardID: %s\n", cardID)

	query := `
		SELECT card_id, business_id, token, last_four_digits, card_type, card_category,
		       expiry_month, expiry_year, issuer_bank, card_holder_name,
		       is_active, gateway_used, added_at, last_used_at, metadata
		FROM xcentro.stored_cards
		WHERE card_id = $1 AND is_active = TRUE
	`

	var card payment.StoredCard
	var lastUsedAt *time.Time
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
		&card.GatewayUsed,
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

	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &card.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &card, nil
}

// GetStoredCardsByBusinessID retrieves all active cards for a business
func (r *PgStoredCardRepository) GetStoredCardsByBusinessID(ctx context.Context, businessID string) ([]*payment.StoredCard, error) {
	fmt.Printf("[PgStoredCardRepository] GetStoredCardsByBusinessID called - BusinessID: %s\n", businessID)

	query := `
		SELECT card_id, business_id, token, last_four_digits, card_type, card_category,
		       expiry_month, expiry_year, issuer_bank, card_holder_name,
		       is_active, gateway_used, added_at, last_used_at, metadata
		FROM xcentro.stored_cards
		WHERE business_id = $1 AND is_active = TRUE
		ORDER BY added_at DESC
	`

	rows, err := r.querier.Query(ctx, query, businessID)
	if err != nil {
		return nil, fmt.Errorf("failed to query stored cards: %w", err)
	}
	defer rows.Close()

	var cards []*payment.StoredCard
	for rows.Next() {
		var card payment.StoredCard
		var lastUsedAt *time.Time
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
			&card.GatewayUsed,
			&card.AddedAt,
			&lastUsedAt,
			&metadataJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan stored card: %w", err)
		}

		card.LastUsedAt = lastUsedAt

		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &card.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		cards = append(cards, &card)
	}

	return cards, nil
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
