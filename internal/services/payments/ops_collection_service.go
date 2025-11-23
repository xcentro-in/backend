package payments

import (
	"context"
	"fmt"

	"xcentro.in/internal/core/payment"
	"xcentro.in/internal/shared/logger"
)

// OpsCollectionLister interface for repositories that support ops collection listing
type OpsCollectionLister interface {
	ListCollectionsPaginated(ctx context.Context, page int, limit int, status string) ([]*payment.OpsCollectionListItem, int, error)
}

// OpsCollectionService provides ops-specific payment collection operations
// This is a lightweight service that doesn't require all PaymentService dependencies
type OpsCollectionService struct {
	repo   OpsCollectionLister
	logger *logger.Logger
}

// NewOpsCollectionService creates a new ops collection service
func NewOpsCollectionService(repo OpsCollectionLister) *OpsCollectionService {
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
