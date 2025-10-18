-- Rollback: Remove relation_id from business_payee_relation

-- Drop index
DROP INDEX IF EXISTS xcentro.idx_business_payee_relation_relation_id;

-- Drop unique constraint
ALTER TABLE xcentro.business_payee_relation
DROP CONSTRAINT IF EXISTS business_payee_relation_relation_id_key;

-- Drop column
ALTER TABLE xcentro.business_payee_relation
DROP COLUMN IF EXISTS relation_id;
