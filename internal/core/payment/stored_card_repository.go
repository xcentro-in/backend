package payment

import (
	"context"
	"time"
)

// StoredCardRepository defines database operations for tokenized cards
type StoredCardRepository interface {
	// Core CRUD
	CreateStoredCard(ctx context.Context, card *StoredCard) error
	GetStoredCardByID(ctx context.Context, cardID string) (*StoredCard, error)
	GetStoredCardsByBusinessID(ctx context.Context, businessID string) ([]*StoredCard, error)

	// Card lifecycle management
	DeactivateCard(ctx context.Context, cardID string) error
	UpdateLastUsedAt(ctx context.Context, cardID string, usedAt time.Time) error

	// Transaction support
	WithTx(tx interface{}) StoredCardRepository
}