-- Migration 000063: Drop payout infrastructure (Castler escrow model replaced by split settlement)
-- With split settlement, payment gateways distribute funds directly to payees.
-- The payout_transfer_requests table and castler_beneficiary_id column are no longer needed.

-- Step 1: Drop FK constraint from payment_state_changes to payout_transfer_requests
-- (transfer_request_id column kept for historical data, FK no longer valid)
ALTER TABLE xcentro.payment_state_changes
    DROP CONSTRAINT IF EXISTS payment_state_changes_transfer_request_id_fkey;

-- Step 2: Remove castler_beneficiary_id from payee_bank_accounts
ALTER TABLE xcentro.payee_bank_accounts
    DROP COLUMN IF EXISTS castler_beneficiary_id;

-- Step 3: Drop payout_transfer_requests table
-- (ON DELETE RESTRICT FK from payment_state_changes already removed above)
DROP TABLE IF EXISTS xcentro.payout_transfer_requests;

-- Step 4: Drop the payout_transfer_status_enum type
DROP TYPE IF EXISTS xcentro.payout_transfer_status_enum;
