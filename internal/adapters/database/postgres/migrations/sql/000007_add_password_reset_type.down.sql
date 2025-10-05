-- Remove kyc_needed column from user_table
ALTER TABLE xcentro.user_table DROP COLUMN IF EXISTS kyc_needed;

-- Note: PostgreSQL does not support removing enum values directly
-- If rollback is needed, the entire enum type would need to be recreated
-- For now, we leave the enum value in place as it doesn't break functionality
