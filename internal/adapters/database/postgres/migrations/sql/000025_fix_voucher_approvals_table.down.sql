-- #############################################################################
-- # Rollback: Revert voucher_approvals table to original structure
-- # Migration: 000025
-- #############################################################################

-- Drop the new voucher_approvals table
DROP TABLE IF EXISTS xcentro.voucher_approvals;

-- Drop the ENUM type
DROP TYPE IF EXISTS xcentro.approval_action_enum;

-- Recreate original voucher_approvals table structure
CREATE TABLE xcentro.voucher_approvals (
    approval_id UUID PRIMARY KEY DEFAULT uuidv7(),
    voucher_id UUID NOT NULL REFERENCES xcentro.payment_vouchers(voucher_id) ON DELETE CASCADE,

    -- Approval Hierarchy
    approval_level INT NOT NULL,  -- 1, 2, 3 for multi-level approvals
    approver_user_id UUID NOT NULL REFERENCES xcentro.user_table(id) ON DELETE CASCADE,
    approver_role VARCHAR(50),  -- Manager, Finance Head, etc.

    -- Action Details
    action VARCHAR(50) NOT NULL DEFAULT 'pending',  -- pending, approved, rejected
    action_timestamp TIMESTAMPTZ,
    remarks TEXT,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Composite unique constraint (one approval per level per voucher)
    UNIQUE (voucher_id, approval_level)
);

-- Recreate indexes
CREATE INDEX idx_approvals_voucher ON xcentro.voucher_approvals(voucher_id);
CREATE INDEX idx_approvals_approver ON xcentro.voucher_approvals(approver_user_id, action);
