-- Rollback: Remove Castler columns from payee_bank_accounts

DROP INDEX IF EXISTS xcentro.idx_bank_accounts_castler_onboarded;

ALTER TABLE xcentro.payee_bank_accounts
DROP COLUMN IF EXISTS castler_beneficiary_id,
DROP COLUMN IF EXISTS castler_onboarded_at;
