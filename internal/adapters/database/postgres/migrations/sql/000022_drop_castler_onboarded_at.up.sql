-- Drop castler_onboarded_at column from payee_bank_accounts
-- Reason: Redundant timestamp - Castler onboarding happens synchronously during account creation
-- The added_at column already captures when the account (and Castler onboarding) was created

ALTER TABLE xcentro.payee_bank_accounts
DROP COLUMN IF EXISTS castler_onboarded_at;
