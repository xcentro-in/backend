-- Migration: Change primary key from composite to surrogate key (relation_id)
-- Purpose: Improve performance and align with surrogate key pattern

-- Step 1: Drop foreign key from payment_vouchers that depends on the PK
ALTER TABLE xcentro.payment_vouchers
DROP CONSTRAINT payment_vouchers_business_id_payee_id_account_id_fkey;

-- Step 2: Drop the existing primary key constraint
ALTER TABLE xcentro.business_payee_relation
DROP CONSTRAINT business_payee_relation_pkey;

-- Step 3: Add UNIQUE constraint on the original composite key (business_id, payee_id, account_id)
-- This maintains the business rule: one relation per business-payee-account combination
ALTER TABLE xcentro.business_payee_relation
ADD CONSTRAINT business_payee_relation_business_payee_account_key
UNIQUE (business_id, payee_id, account_id);

-- Step 4: Make relation_id the new primary key
ALTER TABLE xcentro.business_payee_relation
ADD PRIMARY KEY (relation_id);

-- Note: relation_id already has a UNIQUE constraint from migration 000014
-- We can drop it now since PRIMARY KEY implies UNIQUE
ALTER TABLE xcentro.business_payee_relation
DROP CONSTRAINT business_payee_relation_relation_id_key;

-- Step 5: Update existing index
-- The previous index on relation_id can be dropped since PRIMARY KEY creates an index
DROP INDEX IF EXISTS xcentro.idx_business_payee_relation_relation_id;

-- Step 6: Re-create the foreign key from payment_vouchers
-- PostgreSQL allows FKs to reference UNIQUE constraints, not just PRIMARY KEYs
ALTER TABLE xcentro.payment_vouchers
ADD CONSTRAINT payment_vouchers_business_id_payee_id_account_id_fkey
FOREIGN KEY (business_id, payee_id, account_id)
    REFERENCES xcentro.business_payee_relation(business_id, payee_id, account_id)
    ON DELETE RESTRICT;

-- Step 7: Add indexes for common lookup patterns
CREATE INDEX idx_business_payee_relation_business_id
ON xcentro.business_payee_relation(business_id);

CREATE INDEX idx_business_payee_relation_payee_id
ON xcentro.business_payee_relation(payee_id);

-- Add comment explaining the design
COMMENT ON CONSTRAINT business_payee_relation_pkey ON xcentro.business_payee_relation IS
'Surrogate key (relation_id) as primary key for performance. Natural composite key enforced via UNIQUE constraint.';
