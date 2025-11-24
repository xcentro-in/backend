package payments

import (
	"context"
	"fmt"

	"xcentro.in/internal/core/payment"
	"xcentro.in/internal/shared/logger"
)

// OpsCollectionRepository interface for repositories that support ops collection operations
type OpsCollectionRepository interface {
	ListCollectionsPaginated(ctx context.Context, page int, limit int, status string) ([]*payment.OpsCollectionListItem, int, error)
	GetCollectionSummary(ctx context.Context, paymentID string) (*payment.OpsCollectionSummary, error)
}

// OpsCollectionService provides ops-specific payment collection operations
// This is a lightweight service that doesn't require all PaymentService dependencies
type OpsCollectionService struct {
	repo   OpsCollectionRepository
	logger *logger.Logger
}

// NewOpsCollectionService creates a new ops collection service
func NewOpsCollectionService(repo OpsCollectionRepository) *OpsCollectionService {
	return &OpsCollectionService{
		repo:   repo,
		logger: logger.GetDefault(),
	}
}

// ListCollections retrieves paginated list of collections for ops dashboard
func (s *OpsCollectionService) ListCollections(ctx context.Context, page int, limit int, status string) (*payment.OpsListCollectionsResponse, error) {
	s.logger.Infof("Ops: Fetching collections list - page=%d, limit=%d, status=%s", page, limit, status)

	// Set defaults
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// Fetch from repository
	collections, totalCount, err := s.repo.ListCollectionsPaginated(ctx, page, limit, status)
	if err != nil {
		s.logger.Errorf("Failed to list collections: %v", err)
		return nil, fmt.Errorf("failed to list collections: %w", err)
	}

	// Calculate total pages
	totalPages := (totalCount + limit - 1) / limit

	s.logger.Infof("Ops: Collections list fetched - count=%d, total=%d", len(collections), totalCount)

	return &payment.OpsListCollectionsResponse{
		Data: collections,
		Pagination: payment.PaginationResponse{
			CurrentPage: page,
			PageSize:    limit,
			TotalItems:  totalCount,
			TotalPages:  totalPages,
		},
	}, nil
}

// GetCollectionSummary retrieves lightweight summary for a single collection
func (s *OpsCollectionService) GetCollectionSummary(ctx context.Context, paymentID string) (*payment.OpsCollectionSummary, error) {
	s.logger.Infof("Ops: Fetching collection summary - payment_id=%s", paymentID)

	if paymentID == "" {
		return nil, fmt.Errorf("payment_id is required")
	}

	summary, err := s.repo.GetCollectionSummary(ctx, paymentID)
	if err != nil {
		s.logger.Errorf("Failed to get collection summary: %v", err)
		return nil, err
	}

	s.logger.Infof("Ops: Collection summary fetched - payment_id=%s, status=%s", paymentID, summary.Status)
	return summary, nil
}
