-- Restore is_primary column and related index to payee_bank_accounts

-- Add is_primary column back
ALTER TABLE xcentro.payee_bank_accounts
ADD COLUMN is_primary BOOLEAN DEFAULT FALSE;

-- Recreate the partial index only if payee_id column exists
-- Note: payee_id was removed in migration 000033, so this index can only be created
-- if migration 000033 has been rolled back first
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'xcentro'
        AND table_name = 'payee_bank_accounts'
        AND column_name = 'payee_id'
    ) THEN
        CREATE INDEX idx_payee_bank_accounts_is_primary
        ON xcentro.payee_bank_accounts(payee_id, is_primary)
        WHERE is_primary = TRUE;
    END IF;
END $$;

-- Add comment
COMMENT ON COLUMN xcentro.payee_bank_accounts.is_primary IS 'Only one primary account per payee';
