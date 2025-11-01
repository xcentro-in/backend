package repositories

import (
	"context"

	"github.com/google/uuid"
	"xcentro.in/internal/core/domain"
)

// BatchPaymentRepository handles batch payment data operations
type BatchPaymentRepository interface {
	// CreateBatch creates a new batch payment record
	CreateBatch(ctx context.Context, batch *domain.BatchPayment) error

	// CreateBatchItems creates multiple batch payment items
	CreateBatchItems(ctx context.Context, items []*domain.BatchPaymentItem) error

	// GetBatchByID retrieves a batch by ID
	GetBatchByID(ctx context.Context, batchID uuid.UUID) (*domain.BatchPayment, error)

	// GetBatchItems retrieves all items for a batch
	GetBatchItems(ctx context.Context, batchID uuid.UUID) ([]*domain.BatchPaymentItem, error)

	// GetBatchItemsByStatus retrieves items filtered by status
	GetBatchItemsByStatus(ctx context.Context, batchID uuid.UUID, status domain.BatchPaymentItemStatus) ([]*domain.BatchPaymentItem, error)

	// UpdateBatchStatus updates the batch status
	UpdateBatchStatus(ctx context.Context, batchID uuid.UUID, status domain.BatchPaymentStatus) error

	// UpdateBatchItem updates a batch item
	UpdateBatchItem(ctx context.Context, item *domain.BatchPaymentItem) error

	// UpdateItemMatching updates the matched relation ID for an item
	UpdateItemMatching(ctx context.Context, itemID uuid.UUID, relationID uuid.UUID) error

	// GetBusinessBatches retrieves all batches for a business
	GetBusinessBatches(ctx context.Context, businessID uuid.UUID, limit, offset int) ([]*domain.BatchPayment, int, error)
}
