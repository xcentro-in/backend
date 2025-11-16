-- Migration 000041 Rollback: Restore original payout schema
-- WARNING: This rollback will lose data in removed columns

-- ============================================================================
-- PART 1: RESTORE ENUMS
-- ============================================================================

-- Recreate cancellation_status_enum
CREATE TYPE xcentro.cancellation_status_enum AS ENUM (
  'not_cancelled',
  'pending',
  'approved',
  'rejected'
);

-- Recreate refund_status_enum
CREATE TYPE xcentro.refund_status_enum AS ENUM (
  'not_applicable',
  'pending',
  'initiated',
  'completed',
  'failed'
);


-- ============================================================================
-- PART 2: RESTORE STATUS ENUM WITH APPROVAL STATES
-- ============================================================================

-- Rename current enum
ALTER TYPE xcentro.payout_transfer_status_enum RENAME TO payout_transfer_status_enum_new;

-- Recreate old enum with approval states
CREATE TYPE xcentro.payout_transfer_status_enum AS ENUM (
  'pending_approval',
  'approved',
  'initiated',
  'processing',
  'completed',
  'failed',
  'cancelled',
  'timeout'
);

-- Drop the current default before changing column type (avoids casting issues)
ALTER TABLE xcentro.payout_transfer_requests
  ALTER COLUMN status DROP DEFAULT;

-- Update table to use old enum
ALTER TABLE xcentro.payout_transfer_requests
  ALTER COLUMN status TYPE xcentro.payout_transfer_status_enum
  USING status::text::xcentro.payout_transfer_status_enum;

-- Drop new enum
DROP TYPE xcentro.payout_transfer_status_enum_new;

-- Restore old default
ALTER TABLE xcentro.payout_transfer_requests
  ALTER COLUMN status SET DEFAULT 'approved'::xcentro.payout_transfer_status_enum;


-- ============================================================================
-- PART 3: RESTORE APPROVAL WORKFLOW COLUMNS
-- ============================================================================

ALTER TABLE xcentro.payout_transfer_requests
ADD COLUMN requires_approval BOOLEAN NOT NULL DEFAULT false,
ADD COLUMN approval_mode VARCHAR(20),
ADD COLUMN approved_by UUID REFERENCES xcentro.user_table(id),
ADD COLUMN approved_at TIMESTAMPTZ;

-- Recreate indexes
CREATE INDEX idx_payout_pending_approval
  ON xcentro.payout_transfer_requests(status, amount DESC)
  WHERE status = 'pending_approval';

CREATE INDEX idx_payout_approved
  ON xcentro.payout_transfer_requests(status, created_at)
  WHERE status = 'approved';


-- ============================================================================
-- PART 4: REMOVE NEW COLUMNS FROM payout_transfer_requests
-- ============================================================================

-- Drop new indexes
DROP INDEX IF EXISTS xcentro.idx_payout_transfers_payment_id;
DROP INDEX IF EXISTS xcentro.idx_payout_transfers_payment_status;
DROP INDEX IF EXISTS xcentro.idx_payout_timeout_monitor;

-- Remove new columns
ALTER TABLE xcentro.payout_transfer_requests
DROP COLUMN IF EXISTS payment_id,
DROP COLUMN IF EXISTS escrow_id;


-- ============================================================================
-- PART 5: RESTORE payment_vouchers COLUMNS
-- ============================================================================

-- Restore payout tracking columns
ALTER TABLE xcentro.payment_vouchers
ADD COLUMN payout_status VARCHAR(50) DEFAULT 'pending'
    CHECK (payout_status IN ('pending', 'initiated', 'completed', 'failed')),
ADD COLUMN payout_initiated_at TIMESTAMP,
ADD COLUMN payout_initiated_by UUID REFERENCES xcentro.user_table(id),
ADD COLUMN payout_completed_at TIMESTAMP,
ADD COLUMN payout_reference_id VARCHAR(255),
ADD COLUMN payout_utr VARCHAR(100),
ADD COLUMN payout_metadata JSONB;

-- Restore cancellation tracking columns
ALTER TABLE xcentro.payment_vouchers
ADD COLUMN cancellation_status xcentro.cancellation_status_enum DEFAULT 'not_cancelled',
ADD COLUMN cancellation_requested_at TIMESTAMPTZ,
ADD COLUMN cancellation_requested_by UUID REFERENCES xcentro.user_table(id),
ADD COLUMN cancellation_reason TEXT,
ADD COLUMN cancellation_approved_at TIMESTAMPTZ,
ADD COLUMN cancellation_approved_by UUID REFERENCES xcentro.user_table(id);

-- Restore refund tracking columns
ALTER TABLE xcentro.payment_vouchers
ADD COLUMN refund_status xcentro.refund_status_enum DEFAULT 'not_applicable',
ADD COLUMN refund_initiated_at TIMESTAMPTZ,
ADD COLUMN refund_reference_id VARCHAR(255),
ADD COLUMN refund_metadata JSONB;

-- Restore vendor reference number
ALTER TABLE xcentro.payment_vouchers
ADD COLUMN vendor_reference_number VARCHAR(100);

-- Recreate indexes
CREATE INDEX idx_vouchers_payout_pending
  ON xcentro.payment_vouchers(payout_status, payee_id)
  WHERE payout_status IN ('pending', 'initiated');

CREATE INDEX idx_vouchers_payout_history
  ON xcentro.payment_vouchers(payee_id, payout_completed_at DESC)
  WHERE payout_status = 'completed';

-- Restore index
CREATE INDEX idx_payout_initiated
  ON xcentro.payout_transfer_requests(status, initiated_at)
  WHERE status = 'initiated';


-- ============================================================================
-- PART 6: RESTORE COMMENTS
-- ============================================================================

COMMENT ON TABLE xcentro.payment_vouchers IS
'Payment vouchers serve as the accounting document linking payments to ERP systems. Created in DRAFT state during payment flow, finalized after payment success.';

COMMENT ON COLUMN xcentro.payment_vouchers.payout_status IS
'Payout lifecycle: pending (ready) → initiated (API called) → completed (settled) / failed';

COMMENT ON COLUMN xcentro.payment_vouchers.payout_utr IS
'Bank UTR (Unique Transaction Reference) number for reconciliation with bank statements';

COMMENT ON TABLE xcentro.payout_transfer_requests IS
'Audit trail for all payout transfer attempts with webhook-driven status updates';

COMMENT ON COLUMN xcentro.payout_transfer_requests.status IS
'Transfer status with approval workflow support';
