-- #############################################################################
-- # Remove DEFAULT uuidv7() from payments.payment_id
-- #
-- # Reason: Application layer generates payment IDs and passes them explicitly.
-- # The DEFAULT was causing PostgreSQL to ignore application-provided IDs and
-- # generate its own, leading to ID mismatch between code and database.
-- #
-- # After this migration:
-- # - Application MUST provide payment_id explicitly
-- # - INSERT will fail if payment_id is NULL (fail-fast behavior)
-- # - No more silent ID replacement by database
-- #############################################################################

ALTER TABLE xcentro.payments
ALTER COLUMN payment_id DROP DEFAULT;

COMMENT ON COLUMN xcentro.payments.payment_id IS 'Application-generated UUID (must be provided by application layer)';
