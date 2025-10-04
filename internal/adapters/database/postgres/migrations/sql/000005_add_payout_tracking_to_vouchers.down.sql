-- Rollback migration: Remove payout tracking columns from payment_vouchers table
-- Created: 2025-10-03

-- Drop indexes
DROP INDEX IF EXISTS xcentro.idx_vouchers_payout_pending;
DROP INDEX IF EXISTS xcentro.idx_vouchers_payout_history;

-- Drop columns
ALTER TABLE xcentro.payment_vouchers
DROP COLUMN IF EXISTS payout_status,
DROP COLUMN IF EXISTS payout_initiated_at,
DROP COLUMN IF EXISTS payout_initiated_by,
DROP COLUMN IF EXISTS payout_completed_at,
DROP COLUMN IF EXISTS payout_reference_id,
DROP COLUMN IF EXISTS payout_utr,
DROP COLUMN IF EXISTS payout_metadata;
