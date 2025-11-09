-- #############################################################################
-- # Rollback: Restore DEFAULT uuidv7() to payments.payment_id
-- #############################################################################

ALTER TABLE xcentro.payments
ALTER COLUMN payment_id SET DEFAULT uuidv7();

COMMENT ON COLUMN xcentro.payments.payment_id IS 'Payment transaction UUID (auto-generated if not provided)';
