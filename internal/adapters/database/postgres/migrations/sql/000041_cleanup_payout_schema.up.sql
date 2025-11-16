-- Migration 000041: Comprehensive payout schema cleanup
-- Purpose:
--   1. Remove all payout/cancellation/refund columns from payment_vouchers
--   2. Add payment_id and escrow_id to payout_transfer_requests
--   3. Remove approval workflow from payout_transfer_requests
--   4. Simplify payout_transfer_status_enum

-- ============================================================================
-- PART 1: CLEANUP payment_vouchers TABLE
-- ============================================================================

-- Drop all payout tracking columns (moving to payout_transfer_requests only)
ALTER TABLE xcentro.payment_vouchers
DROP COLUMN IF EXISTS payout_status,
DROP COLUMN IF EXISTS payout_initiated_at,
DROP COLUMN IF EXISTS payout_initiated_by,
DROP COLUMN IF EXISTS payout_completed_at,
DROP COLUMN IF EXISTS payout_reference_id,
DROP COLUMN IF EXISTS payout_utr,
DROP COLUMN IF EXISTS payout_metadata;

-- Drop all cancellation tracking columns (not needed for MVP)
ALTER TABLE xcentro.payment_vouchers
DROP COLUMN IF EXISTS cancellation_status,
DROP COLUMN IF EXISTS cancellation_requested_at,
DROP COLUMN IF EXISTS cancellation_requested_by,
DROP COLUMN IF EXISTS cancellation_reason,
DROP COLUMN IF EXISTS cancellation_approved_at,
DROP COLUMN IF EXISTS cancellation_approved_by;

-- Drop all refund tracking columns (not needed for MVP)
ALTER TABLE xcentro.payment_vouchers
DROP COLUMN IF EXISTS refund_status,
DROP COLUMN IF EXISTS refund_initiated_at,
DROP COLUMN IF EXISTS refund_reference_id,
DROP COLUMN IF EXISTS refund_metadata;

-- Drop vendor reference number (not needed)
ALTER TABLE xcentro.payment_vouchers
DROP COLUMN IF EXISTS vendor_reference_number;

-- Drop indexes for removed columns
DROP INDEX IF EXISTS xcentro.idx_vouchers_payout_pending;
DROP INDEX IF EXISTS xcentro.idx_vouchers_payout_history;

COMMENT ON TABLE xcentro.payment_vouchers IS
'Payment vouchers serve as the accounting document. All payout tracking moved to payout_transfer_requests table.';


-- ============================================================================
-- PART 2: ENHANCE payout_transfer_requests TABLE
-- ============================================================================

-- Add payment_id column for direct reference
ALTER TABLE xcentro.payout_transfer_requests
ADD COLUMN payment_id UUID
  REFERENCES xcentro.payments(payment_id)
  ON DELETE RESTRICT;

-- Backfill payment_id from vouchers
UPDATE xcentro.payout_transfer_requests ptr
SET payment_id = v.payment_id
FROM xcentro.payment_vouchers v
WHERE ptr.voucher_id = v.voucher_id;

-- Make payment_id NOT NULL after backfill
ALTER TABLE xcentro.payout_transfer_requests
ALTER COLUMN payment_id SET NOT NULL;

-- Add escrow_id column for Castler escrow account reference
ALTER TABLE xcentro.payout_transfer_requests
ADD COLUMN escrow_id VARCHAR(255);

COMMENT ON COLUMN xcentro.payout_transfer_requests.payment_id IS
'Direct reference to the payment that triggered this payout';

COMMENT ON COLUMN xcentro.payout_transfer_requests.escrow_id IS
'Castler escrow account ID (source account for transfer)';

-- Create index for payment_id queries
CREATE INDEX idx_payout_transfers_payment_id
  ON xcentro.payout_transfer_requests(payment_id);

-- Note: Composite index with status will be created after enum migration (Part 5)


-- ============================================================================
-- PART 3: REMOVE APPROVAL WORKFLOW
-- ============================================================================

-- Drop approval workflow columns (Castler confirmed no approval needed)
ALTER TABLE xcentro.payout_transfer_requests
DROP COLUMN IF EXISTS requires_approval,
DROP COLUMN IF EXISTS approval_mode,
DROP COLUMN IF EXISTS approved_by,
DROP COLUMN IF EXISTS approved_at;

-- Drop ALL indexes that reference status column (must be done before enum rename)
DROP INDEX IF EXISTS xcentro.idx_payout_transfers_status;
DROP INDEX IF EXISTS xcentro.idx_payout_pending_approval;
DROP INDEX IF EXISTS xcentro.idx_payout_approved;
DROP INDEX IF EXISTS xcentro.idx_payout_initiated;
DROP INDEX IF EXISTS xcentro.idx_payout_timeout;
DROP INDEX IF EXISTS xcentro.idx_payout_retry;


-- ============================================================================
-- PART 4: SIMPLIFY STATUS ENUM
-- ============================================================================

-- Rename old enum
ALTER TYPE xcentro.payout_transfer_status_enum RENAME TO payout_transfer_status_enum_old;

-- Create simplified enum (remove approval-related statuses)
CREATE TYPE xcentro.payout_transfer_status_enum AS ENUM (
  'initiated',   -- Sent to Castler
  'processing',  -- Castler confirmed processing (from webhook)
  'completed',   -- Successfully completed with UTR (from webhook)
  'failed',      -- Failed (from webhook) - manual retry in MVP
  'timeout'      -- No webhook received after timeout period (needs investigation)
);

-- Update table to use new enum
-- First update any 'pending_approval' or 'approved' to 'initiated'
UPDATE xcentro.payout_transfer_requests
SET status = 'initiated'::xcentro.payout_transfer_status_enum_old
WHERE status::text IN ('pending_approval', 'approved');

-- Drop the old default before changing column type (avoids casting issues)
ALTER TABLE xcentro.payout_transfer_requests
  ALTER COLUMN status DROP DEFAULT;

-- Now alter column type
ALTER TABLE xcentro.payout_transfer_requests
  ALTER COLUMN status TYPE xcentro.payout_transfer_status_enum
  USING status::text::xcentro.payout_transfer_status_enum;

-- Drop old enum
DROP TYPE xcentro.payout_transfer_status_enum_old;

-- Set the new default value for new records
ALTER TABLE xcentro.payout_transfer_requests
  ALTER COLUMN status SET DEFAULT 'initiated'::xcentro.payout_transfer_status_enum;


-- ============================================================================
-- PART 5: RECREATE INDEXES WITH NEW ENUM
-- ============================================================================

-- Create composite index for common queries
CREATE INDEX idx_payout_transfers_payment_status
  ON xcentro.payout_transfer_requests(payment_id, status);

-- Create index for status-based queries
CREATE INDEX idx_payout_transfers_status
  ON xcentro.payout_transfer_requests(status);

-- Create index for initiated/processing statuses
CREATE INDEX idx_payout_initiated
  ON xcentro.payout_transfer_requests(status, initiated_at)
  WHERE status IN ('initiated', 'processing');

-- Index for timeout monitoring
CREATE INDEX idx_payout_timeout_monitor
  ON xcentro.payout_transfer_requests(webhook_timeout_at, status)
  WHERE status IN ('initiated', 'processing') AND webhook_timeout_at IS NOT NULL;


-- ============================================================================
-- PART 6: DROP ENUMS FOR REMOVED COLUMNS
-- ============================================================================

-- Drop enums that were used by removed columns
DROP TYPE IF EXISTS xcentro.cancellation_status_enum CASCADE;
DROP TYPE IF EXISTS xcentro.refund_status_enum CASCADE;


-- ============================================================================
-- FINAL COMMENTS
-- ============================================================================

COMMENT ON TABLE xcentro.payout_transfer_requests IS
'Audit trail for all payout transfer attempts. Each voucher maps to exactly one transfer. Payment ID provides direct access to settlement status.';

COMMENT ON COLUMN xcentro.payout_transfer_requests.status IS
'Transfer status: initiated (sent to Castler) → processing (Castler webhook) → completed (with UTR) / failed / timeout';
