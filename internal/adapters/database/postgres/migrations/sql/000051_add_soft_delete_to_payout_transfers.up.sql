-- Migration 000051: Add soft delete support for payout transfer retry mechanism
-- Purpose:
--   1. Add soft delete columns (is_deleted, deleted_at, deleted_reason)
--   2. Remove unique constraint on voucher_id to allow multiple attempts
--   3. Add partial unique index to ensure only ONE active transfer per voucher
--   4. Add indexes for efficient queries

-- ============================================================================
-- STEP 1: Add soft delete columns
-- ============================================================================

ALTER TABLE xcentro.payout_transfer_requests
ADD COLUMN is_deleted BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE xcentro.payout_transfer_requests
ADD COLUMN deleted_at TIMESTAMPTZ;

ALTER TABLE xcentro.payout_transfer_requests
ADD COLUMN deleted_reason TEXT;

COMMENT ON COLUMN xcentro.payout_transfer_requests.is_deleted IS
'Soft delete flag for retry mechanism. When retry is initiated, old failed/timeout attempts are marked deleted and new attempt is created.';

COMMENT ON COLUMN xcentro.payout_transfer_requests.deleted_at IS
'Timestamp when this transfer was soft deleted (marked as obsolete by a retry or manual action).';

COMMENT ON COLUMN xcentro.payout_transfer_requests.deleted_reason IS
'Reason for soft deletion: "retry" (user initiated retry), "superseded" (new attempt created), "manual_cleanup" (ops action), etc.';


-- ============================================================================
-- STEP 2: Drop the unique constraint on voucher_id
-- ============================================================================

-- This allows multiple transfer attempts per voucher (for retry support)
ALTER TABLE xcentro.payout_transfer_requests
DROP CONSTRAINT unique_voucher_transfer;


-- ============================================================================
-- STEP 3: Create partial unique index (only ONE active transfer per voucher)
-- ============================================================================

-- This prevents accidentally creating multiple active (non-deleted) transfers for same voucher
-- But allows multiple deleted (historical) transfers
CREATE UNIQUE INDEX unique_voucher_transfer_active
ON xcentro.payout_transfer_requests(voucher_id)
WHERE is_deleted = false;

COMMENT ON INDEX xcentro.unique_voucher_transfer_active IS
'Ensures only ONE active (non-deleted) transfer exists per voucher at any time. Allows multiple soft-deleted historical attempts.';


-- ============================================================================
-- STEP 4: Create indexes for efficient queries
-- ============================================================================

-- Index for getting latest transfer by voucher (chronological order)
CREATE INDEX idx_payout_transfers_voucher_chronological
ON xcentro.payout_transfer_requests(voucher_id, created_at DESC)
WHERE is_deleted = false;

-- Index for soft deleted records (for audit trail and cleanup)
CREATE INDEX idx_payout_transfers_deleted
ON xcentro.payout_transfer_requests(deleted_at)
WHERE is_deleted = true;

-- Index for retry count queries
CREATE INDEX idx_payout_transfers_voucher_all_attempts
ON xcentro.payout_transfer_requests(voucher_id, created_at ASC);


-- ============================================================================
-- COMMENTS
-- ============================================================================

COMMENT ON TABLE xcentro.payout_transfer_requests IS
'Payout transfer requests with soft delete support for retry mechanism. Each voucher can have multiple attempts (only one active). Failed/timeout transfers are soft deleted when retry is initiated.';
