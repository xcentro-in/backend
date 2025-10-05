-- Rollback: Remove settlement and cancellation tracking
-- Phase 7 rollback

-- Remove columns from payment_vouchers
ALTER TABLE xcentro.payment_vouchers
DROP COLUMN IF EXISTS cancellation_status,
DROP COLUMN IF EXISTS cancellation_requested_at,
DROP COLUMN IF EXISTS cancellation_requested_by,
DROP COLUMN IF EXISTS cancellation_reason,
DROP COLUMN IF EXISTS cancellation_approved_at,
DROP COLUMN IF EXISTS cancellation_approved_by,
DROP COLUMN IF EXISTS refund_status,
DROP COLUMN IF EXISTS refund_initiated_at,
DROP COLUMN IF EXISTS refund_reference_id,
DROP COLUMN IF EXISTS refund_metadata;

-- Remove columns from payments
ALTER TABLE xcentro.payments
DROP COLUMN IF EXISTS settlement_status,
DROP COLUMN IF EXISTS settled_at,
DROP COLUMN IF EXISTS settlement_reference_id,
DROP COLUMN IF EXISTS settlement_utr,
DROP COLUMN IF EXISTS settlement_metadata;

-- Drop ENUMs (in reverse order of creation)
DROP TYPE IF EXISTS xcentro.refund_status_enum;
DROP TYPE IF EXISTS xcentro.cancellation_status_enum;
DROP TYPE IF EXISTS xcentro.settlement_status_enum;
