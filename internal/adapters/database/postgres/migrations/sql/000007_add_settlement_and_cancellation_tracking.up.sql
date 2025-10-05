-- #############################################################################
-- # Phase 7: Settlement & Cancellation Tracking
-- # Created: 2025-10-04
-- # Purpose: Track settlement status and support payment cancellations/refunds
-- #############################################################################

-- =============================================================================
-- Create ENUMs for Status Fields
-- =============================================================================

-- Settlement status for payments table
-- Tracks when gateway funds actually settle into XCentro escrow account
CREATE TYPE xcentro.settlement_status_enum AS ENUM (
    'pending',   -- Payment accepted by gateway, awaiting settlement (T+0 to T+2 days)
    'settled',   -- Funds settled into escrow account
    'failed',    -- Settlement failed (rare)
    'refunded'   -- Funds refunded to customer
);

-- Cancellation status for vouchers
-- Allows business to cancel payment before payout
CREATE TYPE xcentro.cancellation_status_enum AS ENUM (
    'not_cancelled',           -- Normal flow, no cancellation
    'cancellation_requested',  -- Business requested cancellation (pending approval)
    'cancelled'                -- Cancellation approved (trigger refund, not payout)
);

-- Refund status for cancelled vouchers
-- Tracks refund to customer when payment cancelled before payout
CREATE TYPE xcentro.refund_status_enum AS ENUM (
    'not_applicable',  -- Normal flow, no refund needed
    'pending',         -- Refund queued
    'initiated',       -- Refund API called
    'completed',       -- Refund settled to customer
    'failed'           -- Refund failed
);

-- =============================================================================
-- Add Settlement Tracking to Payments Table
-- =============================================================================

ALTER TABLE xcentro.payments
ADD COLUMN settlement_status xcentro.settlement_status_enum DEFAULT 'pending',
ADD COLUMN settled_at TIMESTAMPTZ,
ADD COLUMN settlement_reference_id VARCHAR(255),  -- Gateway's settlement reference
ADD COLUMN settlement_utr VARCHAR(100),           -- Bank UTR for escrow deposit
ADD COLUMN settlement_metadata JSONB;

-- Index for settlement queries (used by payout worker)
CREATE INDEX idx_payments_settlement
ON xcentro.payments(settlement_status, settled_at)
WHERE settlement_status = 'settled';

-- Column comments
COMMENT ON COLUMN xcentro.payments.settlement_status IS
'Settlement lifecycle: pending (T+0) → settled (T+1 to T+2) → ready for payout';

COMMENT ON COLUMN xcentro.payments.settled_at IS
'Timestamp when payment gateway funds actually settled into XCentro escrow account';

COMMENT ON COLUMN xcentro.payments.settlement_utr IS
'Bank UTR for escrow deposit (from gateway settlement report or webhook)';

-- =============================================================================
-- Add Cancellation & Refund Tracking to Vouchers Table
-- =============================================================================

ALTER TABLE xcentro.payment_vouchers
ADD COLUMN cancellation_status xcentro.cancellation_status_enum DEFAULT 'not_cancelled',
ADD COLUMN cancellation_requested_at TIMESTAMPTZ,
ADD COLUMN cancellation_requested_by UUID REFERENCES xcentro.user_table(id),
ADD COLUMN cancellation_reason TEXT,
ADD COLUMN cancellation_approved_at TIMESTAMPTZ,
ADD COLUMN cancellation_approved_by UUID REFERENCES xcentro.user_table(id),
ADD COLUMN refund_status xcentro.refund_status_enum DEFAULT 'not_applicable',
ADD COLUMN refund_initiated_at TIMESTAMPTZ,
ADD COLUMN refund_reference_id VARCHAR(255),  -- Payment gateway refund txn ID
ADD COLUMN refund_metadata JSONB;

-- Index for cancellation queries (operations dashboard)
CREATE INDEX idx_vouchers_cancellation
ON xcentro.payment_vouchers(cancellation_status, cancellation_requested_at)
WHERE cancellation_status IN ('cancellation_requested', 'cancelled');

-- Column comments
COMMENT ON COLUMN xcentro.payment_vouchers.cancellation_status IS
'Cancellation workflow: not_cancelled → cancellation_requested → cancelled';

COMMENT ON COLUMN xcentro.payment_vouchers.cancellation_reason IS
'Business reason for cancellation (e.g., "Vendor agreed to cancel invoice")';

COMMENT ON COLUMN xcentro.payment_vouchers.refund_status IS
'Refund to customer when payment cancelled BEFORE payout to payee';

COMMENT ON COLUMN xcentro.payment_vouchers.refund_reference_id IS
'Payment gateway refund transaction ID (for reconciliation)';
