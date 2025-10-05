-- Update kyc_needed column default to TRUE (new users should do KYC by default)
ALTER TABLE xcentro.user_table
ALTER COLUMN kyc_needed SET DEFAULT TRUE;

COMMENT ON COLUMN xcentro.user_table.kyc_needed IS 'Whether KYC is required for this user (default TRUE for new users, can be set to FALSE for invited users based on business requirements)';
