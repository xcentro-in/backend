-- ############################################################################
-- Rollback: Remove retry tracking columns from payments table
-- ############################################################################

BEGIN;

-- Drop index
DROP INDEX IF EXISTS xcentro.idx_payments_retry_of;

-- Drop columns
ALTER TABLE xcentro.payments DROP COLUMN IF EXISTS retry_count;
ALTER TABLE xcentro.payments DROP COLUMN IF EXISTS retry_of_payment_id;

COMMIT;
