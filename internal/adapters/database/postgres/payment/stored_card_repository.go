package payment

import (
	"context"
	"fmt"
	"time"

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
	// TODO-PHASE-4.4: Implement SQL INSERT
	// - INSERT INTO xcentro.stored_cards (...) VALUES (...) RETURNING card_id
	// - card_id generated via uuidv7()
	// - Set card.CardID with returned UUID
	// - Serialize metadata to JSONB
	return nil
}

// GetStoredCardByID retrieves a stored card by ID
func (r *PgStoredCardRepository) GetStoredCardByID(ctx context.Context, cardID string) (*payment.StoredCard, error) {
	fmt.Printf("[PgStoredCardRepository] GetStoredCardByID called - CardID: %s\n", cardID)
	// TODO-PHASE-4.4: Implement SQL SELECT
	// - SELECT * FROM xcentro.stored_cards WHERE card_id = $1
	// - Check is_active = true
	// - Deserialize JSONB metadata
	return nil, nil
}

// GetStoredCardsByBusinessID retrieves all active cards for a business
func (r *PgStoredCardRepository) GetStoredCardsByBusinessID(ctx context.Context, businessID string) ([]*payment.StoredCard, error) {
	fmt.Printf("[PgStoredCardRepository] GetStoredCardsByBusinessID called - BusinessID: %s\n", businessID)
	// TODO-PHASE-4.4: Implement SQL SELECT WHERE is_active = true
	// - SELECT * FROM xcentro.stored_cards WHERE business_id = $1 AND is_active = true ORDER BY added_at DESC
	// - Return slice of StoredCard pointers
	return nil, nil
}

// DeactivateCard marks a card as inactive (soft delete)
func (r *PgStoredCardRepository) DeactivateCard(ctx context.Context, cardID string) error {
	fmt.Printf("[PgStoredCardRepository] DeactivateCard called - CardID: %s\n", cardID)
	// TODO-PHASE-4.4: Implement SQL UPDATE SET is_active = false
	// - UPDATE xcentro.stored_cards SET is_active = false WHERE card_id = $1
	return nil
}

// UpdateLastUsedAt updates the last used timestamp
func (r *PgStoredCardRepository) UpdateLastUsedAt(ctx context.Context, cardID string, usedAt time.Time) error {
	fmt.Printf("[PgStoredCardRepository] UpdateLastUsedAt called - CardID: %s, UsedAt: %s\n", cardID, usedAt.Format(time.RFC3339))
	// TODO-PHASE-4.4: Implement SQL UPDATE
	// - UPDATE xcentro.stored_cards SET last_used_at = $1 WHERE card_id = $2
	return nil
}