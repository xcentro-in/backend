-- Drop is_primary column and related index from payee_bank_accounts
-- Reason: Business model is one payee = one account per business, making "primary" concept unnecessary

-- Drop the partial index on is_primary
DROP INDEX IF EXISTS xcentro.idx_payee_bank_accounts_is_primary;

-- Drop the is_primary column
ALTER TABLE xcentro.payee_bank_accounts
DROP COLUMN IF EXISTS is_primary;
