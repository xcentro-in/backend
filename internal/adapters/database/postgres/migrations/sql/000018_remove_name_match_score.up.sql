-- Migration: Remove name_match_score column from payee_bank_accounts
-- Reason: Name matching is not feasible with GST-Bank verification
--         Money rotation check is handled separately with director comparison

ALTER TABLE xcentro.payee_bank_accounts DROP COLUMN IF EXISTS name_match_score;
