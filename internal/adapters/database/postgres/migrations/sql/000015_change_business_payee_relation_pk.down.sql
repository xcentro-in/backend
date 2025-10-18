-- Rollback: Revert primary key change from relation_id back to composite key

-- Step 1: Drop foreign key from payment_vouchers first
ALTER TABLE xcentro.payment_vouchers
DROP CONSTRAINT payment_vouchers_business_id_payee_id_account_id_fkey;

-- Step 2: Drop the indexes we created
DROP INDEX IF EXISTS xcentro.idx_business_payee_relation_business_id;
DROP INDEX IF EXISTS xcentro.idx_business_payee_relation_payee_id;

-- Step 3: Drop the current primary key (relation_id)
ALTER TABLE xcentro.business_payee_relation
DROP CONSTRAINT business_payee_relation_pkey;

-- Step 4: Restore the original composite primary key
ALTER TABLE xcentro.business_payee_relation
ADD PRIMARY KEY (business_id, payee_id, account_id);

-- Step 5: Drop the UNIQUE constraint on composite (no longer needed since it's now PK)
ALTER TABLE xcentro.business_payee_relation
DROP CONSTRAINT business_payee_relation_business_payee_account_key;

-- Step 6: Re-add UNIQUE constraint on relation_id (since it's no longer PK)
ALTER TABLE xcentro.business_payee_relation
ADD CONSTRAINT business_payee_relation_relation_id_key UNIQUE (relation_id);

-- Step 7: Re-create the index on relation_id
CREATE INDEX idx_business_payee_relation_relation_id
ON xcentro.business_payee_relation(relation_id);

-- Step 8: Re-create the foreign key from payment_vouchers
ALTER TABLE xcentro.payment_vouchers
ADD CONSTRAINT payment_vouchers_business_id_payee_id_account_id_fkey
FOREIGN KEY (business_id, payee_id, account_id)
    REFERENCES xcentro.business_payee_relation(business_id, payee_id, account_id)
    ON DELETE RESTRICT;
