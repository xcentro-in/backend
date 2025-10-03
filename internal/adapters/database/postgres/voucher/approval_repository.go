package voucher

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/voucher"
)

// PostgresApprovalRepository implements the ApprovalRepository interface using PostgreSQL
type PostgresApprovalRepository struct {
	querier client.Querier
}

// NewPostgresApprovalRepository creates a new PostgreSQL approval repository
func NewPostgresApprovalRepository(db client.DB) voucher.ApprovalRepository {
	return &PostgresApprovalRepository{
		querier: db,
	}
}

// CreateApproval creates a new approval record
func (r *PostgresApprovalRepository) CreateApproval(ctx context.Context, approval *voucher.VoucherApproval) error {
	query := `
		INSERT INTO xcentro.voucher_approvals (
			approval_id, voucher_id, approval_level, approver_user_id,
			approver_role, action, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7
		)
	`

	_, err := r.querier.Exec(ctx, query,
		approval.ApprovalID,
		approval.VoucherID,
		approval.ApprovalLevel,
		approval.ApproverUserID,
		approval.ApproverRole,
		approval.Action,
		approval.CreatedAt,
	)

	if err != nil {
		// Check for unique constraint violation (approval already exists for this voucher + level)
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			return fmt.Errorf("approval already exists for voucher at level %d", approval.ApprovalLevel)
		}
		// Check for foreign key violation
		if strings.Contains(err.Error(), "foreign key") {
			if strings.Contains(err.Error(), "voucher_id") {
				return fmt.Errorf("voucher not found: %w", err)
			}
			if strings.Contains(err.Error(), "approver_user_id") {
				return fmt.Errorf("approver user not found: %w", err)
			}
		}
		return fmt.Errorf("failed to create approval: %w", err)
	}

	return nil
}

// GetApprovalsByVoucher retrieves all approvals for a voucher
func (r *PostgresApprovalRepository) GetApprovalsByVoucher(ctx context.Context, voucherID uuid.UUID) ([]*voucher.VoucherApproval, error) {
	query := `
		SELECT approval_id, voucher_id, approval_level, approver_user_id,
		       approver_role, action, action_timestamp, remarks, created_at
		FROM xcentro.voucher_approvals
		WHERE voucher_id = $1
		ORDER BY approval_level
	`

	rows, err := r.querier.Query(ctx, query, voucherID)
	if err != nil {
		return nil, fmt.Errorf("failed to query approvals by voucher: %w", err)
	}
	defer rows.Close()

	var approvals []*voucher.VoucherApproval
	for rows.Next() {
		approval, err := r.scanApprovalRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan approval row: %w", err)
		}
		approvals = append(approvals, approval)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating approval rows: %w", err)
	}

	return approvals, nil
}

// UpdateApprovalAction updates the approval action (approve/reject)
func (r *PostgresApprovalRepository) UpdateApprovalAction(ctx context.Context, approvalID uuid.UUID, action string, remarks string) error {
	query := `
		UPDATE xcentro.voucher_approvals
		SET action = $2,
		    action_timestamp = NOW(),
		    remarks = $3
		WHERE approval_id = $1 AND action = 'pending'
	`

	result, err := r.querier.Exec(ctx, query, approvalID, action, remarks)
	if err != nil {
		return fmt.Errorf("failed to update approval action: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		// Check if approval exists
		var exists bool
		var currentAction string
		checkQuery := `SELECT EXISTS(SELECT 1 FROM xcentro.voucher_approvals WHERE approval_id = $1),
		                COALESCE((SELECT action FROM xcentro.voucher_approvals WHERE approval_id = $1), '')`
		err = r.querier.QueryRow(ctx, checkQuery, approvalID).Scan(&exists, &currentAction)
		if err != nil {
			return fmt.Errorf("failed to check approval existence: %w", err)
		}
		if !exists {
			return fmt.Errorf("approval not found")
		}
		if currentAction != "pending" {
			return fmt.Errorf("approval is not in pending status (current status: %s)", currentAction)
		}
	}

	return nil
}

// GetPendingApprovals retrieves pending approvals for a user
func (r *PostgresApprovalRepository) GetPendingApprovals(ctx context.Context, userID uuid.UUID) ([]*voucher.VoucherApproval, error) {
	query := `
		SELECT a.approval_id, a.voucher_id, a.approval_level, a.approver_user_id,
		       a.approver_role, a.action, a.action_timestamp, a.remarks, a.created_at
		FROM xcentro.voucher_approvals a
		JOIN xcentro.payment_vouchers v ON a.voucher_id = v.voucher_id
		WHERE a.approver_user_id = $1
		  AND a.action = 'pending'
		ORDER BY a.created_at
	`

	rows, err := r.querier.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending approvals: %w", err)
	}
	defer rows.Close()

	var approvals []*voucher.VoucherApproval
	for rows.Next() {
		approval, err := r.scanApprovalRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan approval row: %w", err)
		}
		approvals = append(approvals, approval)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating approval rows: %w", err)
	}

	return approvals, nil
}

// Helper methods

// scanApproval scans a single row into a VoucherApproval struct
func (r *PostgresApprovalRepository) scanApproval(row pgx.Row) (*voucher.VoucherApproval, error) {
	var a voucher.VoucherApproval

	err := row.Scan(
		&a.ApprovalID,
		&a.VoucherID,
		&a.ApprovalLevel,
		&a.ApproverUserID,
		&a.ApproverRole,
		&a.Action,
		&a.ActionTimestamp,
		&a.Remarks,
		&a.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &a, nil
}

// scanApprovalRow scans rows (for Query) into a VoucherApproval struct
func (r *PostgresApprovalRepository) scanApprovalRow(rows pgx.Rows) (*voucher.VoucherApproval, error) {
	var a voucher.VoucherApproval

	err := rows.Scan(
		&a.ApprovalID,
		&a.VoucherID,
		&a.ApprovalLevel,
		&a.ApproverUserID,
		&a.ApproverRole,
		&a.Action,
		&a.ActionTimestamp,
		&a.Remarks,
		&a.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &a, nil
}
