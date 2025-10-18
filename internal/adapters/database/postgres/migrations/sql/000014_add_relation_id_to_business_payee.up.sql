-- Migration: Add relation_id to business_payee_relation for simplified frontend API
-- Purpose: Single UUID identifier instead of composite (payee_id, account_id)

-- Add relation_id column with default UUID generation
ALTER TABLE xcentro.business_payee_relation
ADD COLUMN relation_id UUID DEFAULT gen_random_uuid() NOT NULL;

-- Add unique constraint on relation_id
ALTER TABLE xcentro.business_payee_relation
ADD CONSTRAINT business_payee_relation_relation_id_key UNIQUE (relation_id);

-- Create index on relation_id for fast lookups
CREATE INDEX idx_business_payee_relation_relation_id
ON xcentro.business_payee_relation(relation_id);

-- Add comment explaining the surrogate key
COMMENT ON COLUMN xcentro.business_payee_relation.relation_id IS 'Surrogate key for simplified frontend API. Single UUID identifying business-payee-account relationship.';
