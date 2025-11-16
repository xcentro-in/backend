-- ############################################################################
-- Migration: Add retry tracking columns to payments table
-- Purpose: Enable retry functionality and track payment retry relationships
-- ############################################################################

BEGIN;

-- Add retry_of_payment_id to track which payment this is a retry of
ALTER TABLE xcentro.payments
ADD COLUMN retry_of_payment_id UUID REFERENCES xcentro.payments(payment_id) ON DELETE SET NULL;

-- Add retry_count to track how many times this payment has been retried
ALTER TABLE xcentro.payments
ADD COLUMN retry_count INTEGER NOT NULL DEFAULT 0;

-- Add index for efficient retry queries
CREATE INDEX idx_payments_retry_of
ON xcentro.payments(retry_of_payment_id)
WHERE retry_of_payment_id IS NOT NULL;

-- Add comment for documentation
COMMENT ON COLUMN xcentro.payments.retry_of_payment_id IS
'References the original payment that this payment is retrying. NULL for original payments.';

COMMENT ON COLUMN xcentro.payments.retry_count IS
'Number of times this payment has been retried. 0 for first attempt.';

COMMIT;
