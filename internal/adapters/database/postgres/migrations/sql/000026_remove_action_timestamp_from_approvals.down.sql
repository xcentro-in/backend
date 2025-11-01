-- #############################################################################
-- # Rollback: Add back action_timestamp column
-- # Migration: 000026
-- #############################################################################

-- Add back action_timestamp column
ALTER TABLE xcentro.voucher_approvals
    ADD COLUMN action_timestamp TIMESTAMPTZ;

-- Restore partial unique index
CREATE UNIQUE INDEX idx_voucher_approvals_pending_unique
    ON xcentro.voucher_approvals(approval_id)
    WHERE action = 'pending';
