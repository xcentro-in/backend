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
	UpdateStoredCard(ctx context.Context, card *StoredCard) error // Update card details (e.g., token after tokenization)

	// Card lifecycle management
	DeactivateCard(ctx context.Context, cardID string) error
	UpdateLastUsedAt(ctx context.Context, cardID string, usedAt time.Time) error
	UpdateCardStatus(ctx context.Context, cardID string, isActive bool, failureReason string) error // Update card status and failure reason

	// Transaction support
	WithTx(tx interface{}) StoredCardRepository
}
