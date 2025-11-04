-- #############################################################################
-- Rollback Seed Data for Payment State Changes
-- #############################################################################
-- This script removes all payment state change records for the test payment.
--
-- Payment ID: 01eeeeee-0909-7000-0000-000000000009
-- #############################################################################

BEGIN;

-- Delete all payment state changes for the test payment
DELETE FROM xcentro.payment_state_changes
WHERE payment_id = '01eeeeee-0909-7000-0000-000000000009';

COMMIT;

-- Verification query (should return 0 rows after rollback)
-- SELECT COUNT(*) FROM xcentro.payment_state_changes WHERE payment_id = '01eeeeee-0909-7000-0000-000000000009';
