-- Migration: Add payout tracking columns to payment_vouchers table
-- Created: 2025-10-03
-- Purpose: Track payout initiation and completion for each voucher

-- Add payout tracking columns
ALTER TABLE xcentro.payment_vouchers
ADD COLUMN payout_status VARCHAR(50) DEFAULT 'pending'
    CHECK (payout_status IN ('pending', 'initiated', 'completed', 'failed')),
ADD COLUMN payout_initiated_at TIMESTAMP,
ADD COLUMN payout_initiated_by UUID REFERENCES xcentro.user_table(id),
ADD COLUMN payout_completed_at TIMESTAMP,
ADD COLUMN payout_reference_id VARCHAR(255),
ADD COLUMN payout_utr VARCHAR(100),
ADD COLUMN payout_metadata JSONB;

-- Index for operations team queries (pending payouts)
-- Used by: Operations dashboard to show pending payouts grouped by payee
CREATE INDEX idx_vouchers_payout_pending
ON xcentro.payment_vouchers(payout_status, payee_id)
WHERE payout_status IN ('pending', 'initiated');

-- Index for payout history queries
-- Used by: Finance team for reconciliation reports
CREATE INDEX idx_vouchers_payout_history
ON xcentro.payment_vouchers(payee_id, payout_completed_at DESC)
WHERE payout_status = 'completed';

-- Column comments for documentation
COMMENT ON COLUMN xcentro.payment_vouchers.payout_status IS
'Payout lifecycle: pending (ready) → initiated (API called) → completed (settled) / failed';

COMMENT ON COLUMN xcentro.payment_vouchers.payout_initiated_at IS
'Timestamp when operations team initiated payout via payout API';

COMMENT ON COLUMN xcentro.payment_vouchers.payout_initiated_by IS
'Operations user who initiated the payout';

COMMENT ON COLUMN xcentro.payment_vouchers.payout_completed_at IS
'Timestamp when payout settled in payee bank account (from webhook or manual update)';

COMMENT ON COLUMN xcentro.payment_vouchers.payout_reference_id IS
'Payout gateway transaction ID (e.g., Razorpay payout_id, Cashfree transfer_id)';

COMMENT ON COLUMN xcentro.payment_vouchers.payout_utr IS
'Bank UTR (Unique Transaction Reference) number for reconciliation with bank statements';

COMMENT ON COLUMN xcentro.payment_vouchers.payout_metadata IS
'Additional payout details: gateway response, failure reasons, retry attempts, etc.';
