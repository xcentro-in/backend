-- Rollback: Make added_by column nullable again in business_payee_relation

ALTER TABLE xcentro.business_payee_relation
ALTER COLUMN added_by DROP NOT NULL;

COMMENT ON COLUMN xcentro.business_payee_relation.added_by IS 'User who added this payee relation (optional)';
