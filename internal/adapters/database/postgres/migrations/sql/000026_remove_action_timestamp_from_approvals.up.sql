-- #############################################################################
-- # Remove action_timestamp column from voucher_approvals
-- # Migration: 000026
-- # Each row is a new approval event, created_at is sufficient
-- #############################################################################

-- Drop action_timestamp column (not needed, created_at is enough)
ALTER TABLE xcentro.voucher_approvals
    DROP COLUMN IF EXISTS action_timestamp;

-- Remove the partial unique index (we want multiple entries per voucher)
DROP INDEX IF EXISTS xcentro.idx_voucher_approvals_pending_unique;

-- Update comment
COMMENT ON TABLE xcentro.voucher_approvals IS 'Approval workflow audit log. Each row represents an approval action/event. Query latest status with ORDER BY created_at DESC.';
COMMENT ON COLUMN xcentro.voucher_approvals.action IS 'Approval action taken: pending (requested), approved, rejected, request_changes. Each action creates a new row.';
COMMENT ON COLUMN xcentro.voucher_approvals.created_at IS 'When this approval action was taken (chronological order for audit trail)';
