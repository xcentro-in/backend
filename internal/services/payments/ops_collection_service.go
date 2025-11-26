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
	GetCollectionDetails(ctx context.Context, paymentID string) (*payment.OpsCollectionDetails, error)
	GetCollectionActivity(ctx context.Context, paymentID string) (*payment.OpsCollectionActivityResponse, error)
	GetCollectionPayouts(ctx context.Context, paymentID string) (*payment.OpsCollectionPayoutsResponse, error)
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

// GetCollectionDetails retrieves complete details for a single collection
// Combines list data, summary, payment breakdown, and settlement information
func (s *OpsCollectionService) GetCollectionDetails(ctx context.Context, paymentID string) (*payment.OpsCollectionDetails, error) {
	s.logger.Infof("Ops: Fetching collection details - payment_id=%s", paymentID)

	if paymentID == "" {
		return nil, fmt.Errorf("payment_id is required")
	}

	details, err := s.repo.GetCollectionDetails(ctx, paymentID)
	if err != nil {
		s.logger.Errorf("Failed to get collection details: %v", err)
		return nil, err
	}

	s.logger.Infof("Ops: Collection details fetched - payment_id=%s, status=%s, total_amount=%d",
		paymentID, details.Status, details.PaymentBreakdown.TotalAmount)
	return details, nil
}

// GetCollectionActivity retrieves complete activity log for a single collection
func (s *OpsCollectionService) GetCollectionActivity(ctx context.Context, paymentID string) (*payment.OpsCollectionActivityResponse, error) {
	s.logger.Infof("Ops: Fetching collection activity - payment_id=%s", paymentID)

	if paymentID == "" {
		return nil, fmt.Errorf("payment_id is required")
	}

	activity, err := s.repo.GetCollectionActivity(ctx, paymentID)
	if err != nil {
		s.logger.Errorf("Failed to get collection activity: %v", err)
		return nil, err
	}

	s.logger.Infof("Ops: Collection activity fetched - payment_id=%s, count=%d", paymentID, activity.TotalCount)
	return activity, nil
}

// GetCollectionPayouts retrieves all voucher-payout mappings for a collection
func (s *OpsCollectionService) GetCollectionPayouts(ctx context.Context, paymentID string) (*payment.OpsCollectionPayoutsResponse, error) {
	s.logger.Infof("Ops: Fetching collection payouts - payment_id=%s", paymentID)

	if paymentID == "" {
		return nil, fmt.Errorf("payment_id is required")
	}

	payouts, err := s.repo.GetCollectionPayouts(ctx, paymentID)
	if err != nil {
		s.logger.Errorf("Failed to get collection payouts: %v", err)
		return nil, err
	}

	s.logger.Infof("Ops: Collection payouts fetched - payment_id=%s, count=%d", paymentID, payouts.TotalCount)
	return payouts, nil
}
