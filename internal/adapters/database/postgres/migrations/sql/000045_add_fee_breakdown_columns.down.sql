-- Rollback migration 000042: Remove fee breakdown columns

ALTER TABLE xcentro.payments
DROP COLUMN IF EXISTS convenience_fee,
DROP COLUMN IF EXISTS convenience_fee_gst;
