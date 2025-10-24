-- Restore is_primary column and related index to payee_bank_accounts

-- Add is_primary column back
ALTER TABLE xcentro.payee_bank_accounts
ADD COLUMN is_primary BOOLEAN DEFAULT FALSE;

-- Recreate the partial index
CREATE INDEX idx_payee_bank_accounts_is_primary
ON xcentro.payee_bank_accounts(payee_id, is_primary)
WHERE is_primary = TRUE;

-- Add comment
COMMENT ON COLUMN xcentro.payee_bank_accounts.is_primary IS 'Only one primary account per payee';
