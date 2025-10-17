-- Rollback: Remove payment_type column from payments table

-- Drop index
DROP INDEX IF EXISTS xcentro.idx_payments_payment_type;

-- Drop column
ALTER TABLE xcentro.payments
DROP COLUMN IF EXISTS payment_type;

-- Drop enum type
DROP TYPE IF EXISTS xcentro.payment_type;
