-- Revert kyc_needed column default back to FALSE
ALTER TABLE xcentro.user_table
ALTER COLUMN kyc_needed SET DEFAULT FALSE;
