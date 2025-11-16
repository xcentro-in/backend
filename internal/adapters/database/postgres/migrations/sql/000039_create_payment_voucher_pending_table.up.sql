-- Create payment_voucher_pending table for temporary voucher-payment linkage
-- This table stores voucher IDs that are part of a payment during payment initiation
-- When payment succeeds in callback, vouchers are finalized and records are deleted
-- Records left in this table indicate abandoned/failed payments (can be cleaned up by cron)

CREATE TABLE IF NOT EXISTS xcentro.payment_voucher_pending (
    -- Composite primary key: one payment can have multiple vouchers
    payment_id UUID NOT NULL,
    voucher_id UUID NOT NULL,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Constraints
    PRIMARY KEY (payment_id, voucher_id),

    -- Foreign keys
    CONSTRAINT fk_payment_voucher_pending_payment
        FOREIGN KEY (payment_id)
        REFERENCES xcentro.payments(payment_id)
        ON DELETE CASCADE,  -- If payment is deleted, remove pending records

    CONSTRAINT fk_payment_voucher_pending_voucher
        FOREIGN KEY (voucher_id)
        REFERENCES xcentro.payment_vouchers(voucher_id)
        ON DELETE CASCADE  -- If voucher is deleted, remove pending records
);

-- Index for efficient lookup by payment_id (to find all vouchers for a payment)
CREATE INDEX idx_payment_voucher_pending_payment_id
    ON xcentro.payment_voucher_pending(payment_id);

-- Index for efficient lookup by voucher_id (to prevent double-payment of same voucher)
CREATE INDEX idx_payment_voucher_pending_voucher_id
    ON xcentro.payment_voucher_pending(voucher_id);

-- Index for cleanup of old abandoned payments (find records older than X hours)
CREATE INDEX idx_payment_voucher_pending_created_at
    ON xcentro.payment_voucher_pending(created_at);

-- Comment on table
COMMENT ON TABLE xcentro.payment_voucher_pending IS
    'Temporary table storing voucher-payment relationships during payment initiation. Records are deleted when payment succeeds or can be cleaned up if payment is abandoned.';

COMMENT ON COLUMN xcentro.payment_voucher_pending.payment_id IS
    'Payment ID that was initiated with these vouchers';

COMMENT ON COLUMN xcentro.payment_voucher_pending.voucher_id IS
    'Voucher ID that is part of this payment';

COMMENT ON COLUMN xcentro.payment_voucher_pending.created_at IS
    'Timestamp when this pending relationship was created (used for cleanup of abandoned payments)';
