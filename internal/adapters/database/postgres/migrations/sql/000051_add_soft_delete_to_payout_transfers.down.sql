-- Migration 000051 Rollback: Remove soft delete support for payout transfers

-- ============================================================================
-- STEP 1: Drop indexes created in up migration
-- ============================================================================

DROP INDEX IF EXISTS xcentro.idx_payout_transfers_voucher_all_attempts;
DROP INDEX IF EXISTS xcentro.idx_payout_transfers_deleted;
DROP INDEX IF EXISTS xcentro.idx_payout_transfers_voucher_chronological;
DROP INDEX IF EXISTS xcentro.unique_voucher_transfer_active;


-- ============================================================================
-- STEP 2: Restore original unique constraint on voucher_id
-- ============================================================================

-- First, delete all soft-deleted records to avoid constraint violation
DELETE FROM xcentro.payout_transfer_requests
WHERE is_deleted = true;

-- For vouchers with multiple active transfers, keep only the latest one
DELETE FROM xcentro.payout_transfer_requests
WHERE transfer_request_id IN (
    SELECT ptr.transfer_request_id
    FROM xcentro.payout_transfer_requests ptr
    INNER JOIN (
        SELECT voucher_id, MAX(created_at) as latest_created_at
        FROM xcentro.payout_transfer_requests
        WHERE is_deleted = false
        GROUP BY voucher_id
        HAVING COUNT(*) > 1
    ) latest ON ptr.voucher_id = latest.voucher_id
    WHERE ptr.is_deleted = false
      AND ptr.created_at < latest.latest_created_at
);

-- Restore the unique constraint
ALTER TABLE xcentro.payout_transfer_requests
ADD CONSTRAINT unique_voucher_transfer UNIQUE(voucher_id);


-- ============================================================================
-- STEP 3: Drop soft delete columns
-- ============================================================================

ALTER TABLE xcentro.payout_transfer_requests
DROP COLUMN IF EXISTS deleted_reason;

ALTER TABLE xcentro.payout_transfer_requests
DROP COLUMN IF EXISTS deleted_at;

ALTER TABLE xcentro.payout_transfer_requests
DROP COLUMN IF EXISTS is_deleted;


-- ============================================================================
-- RESTORE COMMENTS
-- ============================================================================

COMMENT ON TABLE xcentro.payout_transfer_requests IS
'Audit trail for all payout transfer attempts. Each voucher maps to exactly one transfer. Payment ID provides direct access to settlement status.';
