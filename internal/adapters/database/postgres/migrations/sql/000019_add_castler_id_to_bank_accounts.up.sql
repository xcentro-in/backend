-- Migration: Add Castler beneficiary ID directly to payee_bank_accounts
-- Reason: Simplify payout worker - no joins needed, direct iteration
-- The payee_provider_mappings table is over-engineered for single provider use case

ALTER TABLE xcentro.payee_bank_accounts
ADD COLUMN castler_beneficiary_id VARCHAR(255),
ADD COLUMN castler_onboarded_at TIMESTAMPTZ;

-- Index for payout worker to find onboarded accounts
CREATE INDEX idx_bank_accounts_castler_onboarded
ON xcentro.payee_bank_accounts(castler_beneficiary_id)
WHERE castler_beneficiary_id IS NOT NULL;

COMMENT ON COLUMN xcentro.payee_bank_accounts.castler_beneficiary_id IS
'Castler provider beneficiary ID (result from /api/v1/payee API) - used for payouts';

COMMENT ON COLUMN xcentro.payee_bank_accounts.castler_onboarded_at IS
'Timestamp when account was successfully onboarded to Castler';
