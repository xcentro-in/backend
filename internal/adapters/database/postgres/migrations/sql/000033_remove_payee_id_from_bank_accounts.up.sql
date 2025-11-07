-- Migration: Remove payee_id from payee_bank_accounts to make bank accounts globally shared
-- This allows multiple payees to reference the same bank account (via business_payee_relation)
-- Key benefit: Reuse Castler beneficiary IDs for duplicate account numbers

-- Step 1: Drop indexes that reference payee_id
DROP INDEX IF EXISTS xcentro.idx_payee_bank_accounts_payee_id;
DROP INDEX IF EXISTS xcentro.idx_payee_bank_accounts_is_primary;
DROP INDEX IF EXISTS xcentro.idx_payee_bank_accounts_active;

-- Step 2: Drop the unique constraint that includes payee_id
ALTER TABLE xcentro.payee_bank_accounts
    DROP CONSTRAINT IF EXISTS unique_payee_account;

-- Step 3: Add new unique constraint on account_number_hash only
-- This ensures each unique account number is stored only once globally
ALTER TABLE xcentro.payee_bank_accounts
    ADD CONSTRAINT unique_account_hash UNIQUE (account_number_hash);

-- Step 4: Drop the foreign key constraint (implicit, will be removed with column)
-- Step 5: Drop the payee_id column
ALTER TABLE xcentro.payee_bank_accounts
    DROP COLUMN payee_id;

-- Note: business_payee_relation still maintains the mapping between business, payee, and account
-- The unique constraint on (business_id, payee_id, account_id) in business_payee_relation remains intact
