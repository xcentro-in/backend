package payout

import (
	"context"
	"fmt"

	"xcentro.in/internal/core/payout"
	"xcentro.in/internal/shared/logger"
)

// OpsPayoutService provides ops-specific payout transfer operations
// This is a lightweight service that doesn't require all PayoutService dependencies
type OpsPayoutService struct {
	repo   payout.OpsPayoutRepository
	logger *logger.Logger
}

// NewOpsPayoutService creates a new ops payout service
func NewOpsPayoutService(repo payout.OpsPayoutRepository) *OpsPayoutService {
	return &OpsPayoutService{
		repo:   repo,
		logger: logger.GetDefault(),
	}
}

// ListPayouts retrieves paginated list of payout transfers for ops dashboard
func (s *OpsPayoutService) ListPayouts(ctx context.Context, page int, limit int, status string) (*payout.OpsListPayoutsResponse, error) {
	s.logger.Infof("Ops: Fetching payouts list - page=%d, limit=%d, status=%s", page, limit, status)

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
	payouts, totalCount, err := s.repo.ListPayoutsPaginated(ctx, page, limit, status)
	if err != nil {
		s.logger.Errorf("Failed to list payouts: %v", err)
		return nil, fmt.Errorf("failed to list payouts: %w", err)
	}

	// Calculate total pages
	totalPages := (totalCount + limit - 1) / limit

	s.logger.Infof("Ops: Payouts list fetched - count=%d, total=%d", len(payouts), totalCount)

	return &payout.OpsListPayoutsResponse{
		Data: payouts,
		Pagination: payout.OpsPaginationResponse{
			CurrentPage: page,
			PageSize:    limit,
			TotalItems:  totalCount,
			TotalPages:  totalPages,
		},
	}, nil
}
