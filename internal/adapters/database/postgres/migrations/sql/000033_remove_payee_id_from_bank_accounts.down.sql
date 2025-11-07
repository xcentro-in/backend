-- Rollback: Restore payee_id column to payee_bank_accounts
-- WARNING: This rollback is destructive if multiple payees share the same account_number_hash
-- It will fail due to unique constraint violations if the same account is used by multiple payees

-- Step 1: Add payee_id column back (nullable initially to allow data migration)
ALTER TABLE xcentro.payee_bank_accounts
    ADD COLUMN payee_id UUID;

-- Step 2: Populate payee_id from business_payee_relation
-- This takes the first payee_id that references each account_id
-- WARNING: If multiple payees use the same account, only one will be restored
UPDATE xcentro.payee_bank_accounts pba
SET payee_id = (
    SELECT bpr.payee_id
    FROM xcentro.business_payee_relation bpr
    WHERE bpr.account_id = pba.account_id
    LIMIT 1
);

-- Step 3: Make payee_id NOT NULL
ALTER TABLE xcentro.payee_bank_accounts
    ALTER COLUMN payee_id SET NOT NULL;

-- Step 4: Add foreign key constraint
ALTER TABLE xcentro.payee_bank_accounts
    ADD CONSTRAINT payee_bank_accounts_payee_id_fkey
    FOREIGN KEY (payee_id) REFERENCES xcentro.payee_master(payee_id) ON DELETE CASCADE;

-- Step 5: Drop the global unique constraint
ALTER TABLE xcentro.payee_bank_accounts
    DROP CONSTRAINT IF EXISTS unique_account_hash;

-- Step 6: Add back the compound unique constraint
ALTER TABLE xcentro.payee_bank_accounts
    ADD CONSTRAINT unique_payee_account UNIQUE (payee_id, account_number_hash);

-- Step 7: Recreate indexes
CREATE INDEX idx_payee_bank_accounts_payee_id ON xcentro.payee_bank_accounts(payee_id);
CREATE INDEX idx_payee_bank_accounts_is_primary ON xcentro.payee_bank_accounts(payee_id, is_primary) WHERE is_primary = TRUE;
CREATE INDEX idx_payee_bank_accounts_active ON xcentro.payee_bank_accounts(payee_id, is_active) WHERE is_active = TRUE;
