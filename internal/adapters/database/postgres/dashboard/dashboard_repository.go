package dashboard

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/dashboard"
	"xcentro.in/internal/shared/logger"
)

// PgDashboardRepository implements dashboard.DashboardRepository using PostgreSQL
type PgDashboardRepository struct {
	querier client.Querier
	logger  *logger.Logger
}

// NewPgDashboardRepository creates a new PostgreSQL dashboard repository
func NewPgDashboardRepository(db client.DB) *PgDashboardRepository {
	return &PgDashboardRepository{
		querier: db,
		logger:  logger.GetDefault(),
	}
}

// GetTotalSpendThisMonth returns total successful payment amount for current month
func (r *PgDashboardRepository) GetTotalSpendThisMonth(ctx context.Context, businessID string) (int64, error) {
	query := `
		SELECT COALESCE(SUM(amount), 0) as total_spend
		FROM xcentro.payments
		WHERE business_id = $1
		  AND created_at >= date_trunc('month', CURRENT_DATE)
		  AND status = 'success'
	`

	var totalSpend int64
	err := r.querier.QueryRow(ctx, query, businessID).Scan(&totalSpend)
	if err != nil {
		r.logger.Errorf("Failed to get total spend this month for business %s: %v", businessID, err)
		return 0, fmt.Errorf("failed to get total spend: %w", err)
	}

	r.logger.Infof("Total spend this month for business %s: %d paise", businessID, totalSpend)
	return totalSpend, nil
}

// GetOverdueBills returns count and total value of overdue bills
func (r *PgDashboardRepository) GetOverdueBills(ctx context.Context, businessID string) (*dashboard.OverdueBillsSummary, error) {
	query := `
		SELECT
			COUNT(*) as overdue_count,
			COALESCE(SUM(net_amount), 0) as overdue_amount
		FROM xcentro.payment_vouchers
		WHERE business_id = $1
		  AND invoice_due_date < CURRENT_DATE
		  AND status IN ('draft', 'finalized')
	`

	var summary dashboard.OverdueBillsSummary
	err := r.querier.QueryRow(ctx, query, businessID).Scan(&summary.Count, &summary.TotalAmount)
	if err != nil {
		r.logger.Errorf("Failed to get overdue bills for business %s: %v", businessID, err)
		return nil, fmt.Errorf("failed to get overdue bills: %w", err)
	}

	r.logger.Infof("Overdue bills for business %s: count=%d, amount=%d", businessID, summary.Count, summary.TotalAmount)
	return &summary, nil
}

// GetRecentPayments returns last 6 successful payments in last 2 weeks
func (r *PgDashboardRepository) GetRecentPayments(ctx context.Context, businessID string) ([]dashboard.RecentPaymentSummary, error) {
	query := `
		SELECT
			payment_id,
			amount,
			payment_method,
			status,
			created_at
		FROM xcentro.payments
		WHERE business_id = $1
		  AND status = 'success'
		  AND created_at >= CURRENT_DATE - INTERVAL '14 days'
		ORDER BY created_at DESC
		LIMIT 6
	`

	rows, err := r.querier.Query(ctx, query, businessID)
	if err != nil {
		r.logger.Errorf("Failed to get recent payments for business %s: %v", businessID, err)
		return nil, fmt.Errorf("failed to get recent payments: %w", err)
	}
	defer rows.Close()

	var payments []dashboard.RecentPaymentSummary
	for rows.Next() {
		var payment dashboard.RecentPaymentSummary
		var paymentID uuid.UUID

		err := rows.Scan(
			&paymentID,
			&payment.Amount,
			&payment.PaymentMethod,
			&payment.Status,
			&payment.CreatedAt,
		)
		if err != nil {
			r.logger.Errorf("Failed to scan recent payment: %v", err)
			return nil, fmt.Errorf("failed to scan payment: %w", err)
		}

		payment.PaymentID = paymentID.String()
		payments = append(payments, payment)
	}

	if err = rows.Err(); err != nil {
		r.logger.Errorf("Error iterating recent payments: %v", err)
		return nil, fmt.Errorf("error iterating payments: %w", err)
	}

	r.logger.Infof("Recent payments for business %s: count=%d", businessID, len(payments))
	return payments, nil
}

// GetUpcomingLargePayments returns top 3 upcoming bills by amount (not overdue)
func (r *PgDashboardRepository) GetUpcomingLargePayments(ctx context.Context, businessID string) ([]dashboard.UpcomingPaymentSummary, error) {
	query := `
		SELECT
			voucher_id,
			voucher_number,
			payee_id,
			net_amount,
			invoice_due_date,
			invoice_number
		FROM xcentro.payment_vouchers
		WHERE business_id = $1
		  AND invoice_due_date >= CURRENT_DATE
		  AND status IN ('draft', 'finalized')
		ORDER BY net_amount DESC
		LIMIT 3
	`

	rows, err := r.querier.Query(ctx, query, businessID)
	if err != nil {
		r.logger.Errorf("Failed to get upcoming large payments for business %s: %v", businessID, err)
		return nil, fmt.Errorf("failed to get upcoming payments: %w", err)
	}
	defer rows.Close()

	var payments []dashboard.UpcomingPaymentSummary
	for rows.Next() {
		var payment dashboard.UpcomingPaymentSummary
		var voucherID, payeeID uuid.UUID
		var invoiceDueDate sql.NullTime
		var invoiceNumber sql.NullString

		err := rows.Scan(
			&voucherID,
			&payment.VoucherNumber,
			&payeeID,
			&payment.NetAmount,
			&invoiceDueDate,
			&invoiceNumber,
		)
		if err != nil {
			r.logger.Errorf("Failed to scan upcoming payment: %v", err)
			return nil, fmt.Errorf("failed to scan payment: %w", err)
		}

		payment.VoucherID = voucherID.String()
		payment.PayeeID = payeeID.String()
		if invoiceDueDate.Valid {
			payment.InvoiceDueDate = &invoiceDueDate.Time
		}
		if invoiceNumber.Valid {
			payment.InvoiceNumber = &invoiceNumber.String
		}

		payments = append(payments, payment)
	}

	if err = rows.Err(); err != nil {
		r.logger.Errorf("Error iterating upcoming payments: %v", err)
		return nil, fmt.Errorf("error iterating payments: %w", err)
	}

	r.logger.Infof("Upcoming large payments for business %s: count=%d", businessID, len(payments))
	return payments, nil
}

// GetMonthlySpends returns total spend for last 6 months (or fewer if newly onboarded)
func (r *PgDashboardRepository) GetMonthlySpends(ctx context.Context, businessID string) ([]dashboard.MonthlySpendSummary, error) {
	query := `
		SELECT
			date_trunc('month', created_at) as month,
			SUM(amount) as total_spend,
			COUNT(*) as txn_count
		FROM xcentro.payments
		WHERE business_id = $1
		  AND status = 'success'
		  AND created_at >= date_trunc('month', CURRENT_DATE) - INTERVAL '5 months'
		GROUP BY month
		ORDER BY month
	`

	rows, err := r.querier.Query(ctx, query, businessID)
	if err != nil {
		r.logger.Errorf("Failed to get monthly spends for business %s: %v", businessID, err)
		return nil, fmt.Errorf("failed to get monthly spends: %w", err)
	}
	defer rows.Close()

	var spends []dashboard.MonthlySpendSummary
	for rows.Next() {
		var spend dashboard.MonthlySpendSummary

		err := rows.Scan(
			&spend.Month,
			&spend.TotalSpend,
			&spend.TxnCount,
		)
		if err != nil {
			r.logger.Errorf("Failed to scan monthly spend: %v", err)
			return nil, fmt.Errorf("failed to scan spend: %w", err)
		}

		spends = append(spends, spend)
	}

	if err = rows.Err(); err != nil {
		r.logger.Errorf("Error iterating monthly spends: %v", err)
		return nil, fmt.Errorf("error iterating spends: %w", err)
	}

	r.logger.Infof("Monthly spends for business %s: count=%d", businessID, len(spends))
	return spends, nil
}

// GetPaymentSourceBreakdown returns spending distribution by actual payment source for current month
// Groups by specific cards (e.g., "Visa •••• 1234"), bank accounts, UPI, etc.
func (r *PgDashboardRepository) GetPaymentSourceBreakdown(ctx context.Context, businessID string) ([]dashboard.PaymentSourceSummary, error) {
	query := `
		SELECT
			p.payment_method,
			p.payment_source_type,
			sc.card_type,
			sc.last_four_digits,
			ba.bank_name,
			SUM(p.amount) as total_amount,
			COUNT(*) as txn_count
		FROM xcentro.payments p
		LEFT JOIN xcentro.stored_cards sc
		  ON p.payment_source_type = 'card'
		  AND p.payment_source_id = sc.card_id
		LEFT JOIN xcentro.bank_accounts ba
		  ON p.payment_source_type = 'bank_account'
		  AND p.payment_source_id = ba.account_id
		WHERE p.business_id = $1
		  AND p.created_at >= date_trunc('month', CURRENT_DATE)
		  AND p.status = 'success'
		GROUP BY
		  p.payment_method,
		  p.payment_source_type,
		  sc.card_type,
		  sc.last_four_digits,
		  ba.bank_name
		ORDER BY total_amount DESC
	`

	rows, err := r.querier.Query(ctx, query, businessID)
	if err != nil {
		r.logger.Errorf("Failed to get payment source breakdown for business %s: %v", businessID, err)
		return nil, fmt.Errorf("failed to get payment source breakdown: %w", err)
	}
	defer rows.Close()

	var breakdown []dashboard.PaymentSourceSummary
	for rows.Next() {
		var source dashboard.PaymentSourceSummary
		var paymentSourceType sql.NullString
		var cardType, lastFourDigits, bankName sql.NullString

		err := rows.Scan(
			&source.PaymentMethod,
			&paymentSourceType,
			&cardType,
			&lastFourDigits,
			&bankName,
			&source.TotalAmount,
			&source.TxnCount,
		)
		if err != nil {
			r.logger.Errorf("Failed to scan payment source: %v", err)
			return nil, fmt.Errorf("failed to scan source: %w", err)
		}

		// Construct source label based on payment source type
		if paymentSourceType.Valid && paymentSourceType.String == "card" && cardType.Valid && lastFourDigits.Valid {
			// Format: "Visa •••• 1234"
			network := capitalizeFirst(cardType.String)
			source.SourceLabel = fmt.Sprintf("%s •••• %s", network, lastFourDigits.String)
			source.CardNetwork = &cardType.String
			source.LastFourDigits = &lastFourDigits.String
		} else if paymentSourceType.Valid && paymentSourceType.String == "bank_account" && bankName.Valid {
			// Format: "HDFC Bank"
			source.SourceLabel = bankName.String
			source.BankName = &bankName.String
		} else {
			// Fallback: use payment method (UPI, wallet, etc.)
			source.SourceLabel = capitalizeFirst(source.PaymentMethod)
		}

		breakdown = append(breakdown, source)
	}

	if err = rows.Err(); err != nil {
		r.logger.Errorf("Error iterating payment sources: %v", err)
		return nil, fmt.Errorf("error iterating sources: %w", err)
	}

	r.logger.Infof("Payment source breakdown for business %s: count=%d", businessID, len(breakdown))
	return breakdown, nil
}

// GetPendingApprovalsCount returns count of vouchers with latest approval status as 'pending'
// Uses the same logic as /v1/vouchers/approval-inbox/counts endpoint
func (r *PgDashboardRepository) GetPendingApprovalsCount(ctx context.Context, businessID string) (int, error) {
	query := `
		WITH latest_approvals AS (
			SELECT DISTINCT ON (voucher_id)
				voucher_id,
				action
			FROM xcentro.voucher_approvals
			ORDER BY voucher_id, created_at DESC
		)
		SELECT COUNT(*)
		FROM xcentro.payment_vouchers v
		INNER JOIN latest_approvals la ON v.voucher_id = la.voucher_id
		WHERE v.business_id = $1
		  AND la.action = 'pending'
	`

	var count int
	err := r.querier.QueryRow(ctx, query, businessID).Scan(&count)
	if err != nil {
		r.logger.Errorf("Failed to count pending approvals for business %s: %v", businessID, err)
		return 0, fmt.Errorf("failed to count pending approvals: %w", err)
	}

	r.logger.Infof("Pending approvals for business %s: count=%d", businessID, count)
	return count, nil
}

// capitalizeFirst capitalizes the first letter of a string
func capitalizeFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(string(s[0])) + strings.ToLower(s[1:])
}
