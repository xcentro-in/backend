-- Restore castler_onboarded_at column to payee_bank_accounts

ALTER TABLE xcentro.payee_bank_accounts
ADD COLUMN castler_onboarded_at TIMESTAMPTZ;

-- Add comment
COMMENT ON COLUMN xcentro.payee_bank_accounts.castler_onboarded_at IS 'When onboarded to Castler';
