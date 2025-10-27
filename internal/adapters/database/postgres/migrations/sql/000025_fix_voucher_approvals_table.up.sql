-- #############################################################################
-- # Fix voucher_approvals table structure
-- # Migration: 000025
-- #############################################################################

-- Create ENUM for approval action states
CREATE TYPE xcentro.approval_action_enum AS ENUM (
    'pending',          -- Waiting for approval
    'approved',         -- Approved by approver
    'rejected',         -- Rejected by approver
    'request_changes'   -- Approver requested changes
);

-- Drop the old voucher_approvals table and recreate with correct structure
DROP TABLE IF EXISTS xcentro.voucher_approvals;

-- Recreate voucher_approvals table with proper structure
CREATE TABLE xcentro.voucher_approvals (
    approval_id UUID PRIMARY KEY DEFAULT uuidv7(),
    voucher_id UUID NOT NULL REFERENCES xcentro.payment_vouchers(voucher_id) ON DELETE CASCADE,

    -- Requester (who initiated the payment requiring approval)
    requester_user_id UUID NOT NULL REFERENCES xcentro.user_table(id) ON DELETE CASCADE,

    -- Approver (who needs to approve/reject)
    approver_user_id UUID NOT NULL REFERENCES xcentro.user_table(id) ON DELETE CASCADE,

    -- Action Details
    action xcentro.approval_action_enum NOT NULL DEFAULT 'pending',
    action_timestamp TIMESTAMPTZ,
    remarks TEXT,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Partial unique index: Only one pending approval per voucher at a time
CREATE UNIQUE INDEX idx_voucher_approvals_pending_unique
    ON xcentro.voucher_approvals(voucher_id)
    WHERE action = 'pending';

-- Indexes for performance
CREATE INDEX idx_voucher_approvals_voucher ON xcentro.voucher_approvals(voucher_id);
CREATE INDEX idx_voucher_approvals_approver ON xcentro.voucher_approvals(approver_user_id, action);
CREATE INDEX idx_voucher_approvals_requester ON xcentro.voucher_approvals(requester_user_id);

-- Comments for documentation
COMMENT ON TABLE xcentro.voucher_approvals IS 'Approval workflow for voucher payments. Tracks who requested payment and who approved/rejected it.';
COMMENT ON COLUMN xcentro.voucher_approvals.requester_user_id IS 'User who initiated the payment requiring approval';
COMMENT ON COLUMN xcentro.voucher_approvals.approver_user_id IS 'User who approved/rejected the payment (role fetched from user table)';
COMMENT ON COLUMN xcentro.voucher_approvals.action IS 'Approval state: pending → approved/rejected/request_changes';
COMMENT ON COLUMN xcentro.voucher_approvals.remarks IS 'Approver notes explaining their decision';
