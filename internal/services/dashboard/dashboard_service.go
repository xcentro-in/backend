package dashboard

import (
	"context"
	"fmt"
	"sync"

	"xcentro.in/internal/core/dashboard"
	"xcentro.in/internal/shared/logger"
)

// DashboardService orchestrates concurrent fetching of dashboard metrics
type DashboardService struct {
	repo   dashboard.DashboardRepository
	logger *logger.Logger
}

// NewDashboardService creates a new dashboard service
func NewDashboardService(repo dashboard.DashboardRepository, logger *logger.Logger) *DashboardService {
	return &DashboardService{
		repo:   repo,
		logger: logger,
	}
}

// metricResult holds the result of a metric fetch operation
type metricResult struct {
	name string
	err  error
}

// GetDashboardMetrics fetches all dashboard metrics concurrently using goroutines
func (s *DashboardService) GetDashboardMetrics(ctx context.Context, businessID string) (*dashboard.DashboardMetrics, error) {
	s.logger.Infof("Fetching dashboard metrics for business: %s", businessID)

	// Initialize the metrics response
	metrics := &dashboard.DashboardMetrics{}

	// Create a WaitGroup to track all goroutines
	var wg sync.WaitGroup

	// Channel to collect errors from goroutines
	errChan := make(chan metricResult, 6)

	// Goroutine 1: Total Spend This Month
	wg.Add(1)
	go func() {
		defer wg.Done()
		totalSpend, err := s.repo.GetTotalSpendThisMonth(ctx, businessID)
		if err != nil {
			errChan <- metricResult{name: "TotalSpendThisMonth", err: err}
			return
		}
		metrics.TotalSpendThisMonth = totalSpend
	}()

	// Goroutine 2: Overdue Bills
	wg.Add(1)
	go func() {
		defer wg.Done()
		overdueBills, err := s.repo.GetOverdueBills(ctx, businessID)
		if err != nil {
			errChan <- metricResult{name: "OverdueBills", err: err}
			return
		}
		metrics.OverdueBills = *overdueBills
	}()

	// Goroutine 3: Recent Payments
	wg.Add(1)
	go func() {
		defer wg.Done()
		recentPayments, err := s.repo.GetRecentPayments(ctx, businessID)
		if err != nil {
			errChan <- metricResult{name: "RecentPayments", err: err}
			return
		}
		metrics.RecentPayments = recentPayments
	}()

	// Goroutine 4: Upcoming Large Payments
	wg.Add(1)
	go func() {
		defer wg.Done()
		upcomingPayments, err := s.repo.GetUpcomingLargePayments(ctx, businessID)
		if err != nil {
			errChan <- metricResult{name: "UpcomingLargePayments", err: err}
			return
		}
		metrics.UpcomingLargePayments = upcomingPayments
	}()

	// Goroutine 5: Monthly Spends
	wg.Add(1)
	go func() {
		defer wg.Done()
		monthlySpends, err := s.repo.GetMonthlySpends(ctx, businessID)
		if err != nil {
			errChan <- metricResult{name: "MonthlySpends", err: err}
			return
		}
		metrics.MonthlySpends = monthlySpends
	}()

	// Goroutine 6: Payment Source Breakdown
	wg.Add(1)
	go func() {
		defer wg.Done()
		sourceBreakdown, err := s.repo.GetPaymentSourceBreakdown(ctx, businessID)
		if err != nil {
			errChan <- metricResult{name: "PaymentSourceBreakdown", err: err}
			return
		}
		metrics.PaymentSourceBreakdown = sourceBreakdown
	}()

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Collect and check for errors
	var errs []metricResult
	for result := range errChan {
		errs = append(errs, result)
	}

	// If any metric failed, return error
	if len(errs) > 0 {
		s.logger.Errorf("Failed to fetch %d metrics: %v", len(errs), errs)
		// Return the first error encountered
		return nil, fmt.Errorf("failed to fetch metric %s: %w", errs[0].name, errs[0].err)
	}

	s.logger.Infof("Successfully fetched all dashboard metrics for business: %s", businessID)
	return metrics, nil
}
