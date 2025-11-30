package dashboard

import "context"

// DashboardRepository defines the interface for fetching dashboard metrics
type DashboardRepository interface {
	// GetTotalSpendThisMonth returns total successful payment amount for current month
	GetTotalSpendThisMonth(ctx context.Context, businessID string) (int64, error)

	// GetOverdueBills returns count and total value of overdue bills
	GetOverdueBills(ctx context.Context, businessID string) (*OverdueBillsSummary, error)

	// GetRecentPayments returns last 6 successful payments in last 2 weeks
	GetRecentPayments(ctx context.Context, businessID string) ([]RecentPaymentSummary, error)

	// GetUpcomingLargePayments returns top 3 upcoming bills by amount (not overdue)
	GetUpcomingLargePayments(ctx context.Context, businessID string) ([]UpcomingPaymentSummary, error)

	// GetMonthlySpends returns total spend for last 6 months (or fewer if newly onboarded)
	GetMonthlySpends(ctx context.Context, businessID string) ([]MonthlySpendSummary, error)

	// GetPaymentSourceBreakdown returns spending distribution by payment method for current month
	GetPaymentSourceBreakdown(ctx context.Context, businessID string) ([]PaymentSourceSummary, error)

	// GetPendingApprovalsCount returns count of pending voucher approvals
	GetPendingApprovalsCount(ctx context.Context, businessID string) (int, error)
}
