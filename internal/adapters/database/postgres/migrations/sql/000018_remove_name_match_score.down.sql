-- Rollback: Add back name_match_score column to payee_bank_accounts

ALTER TABLE xcentro.payee_bank_accounts ADD COLUMN name_match_score NUMERIC(3,2);
