-- Add name and dob fields to user_table for user onboarding
ALTER TABLE xcentro.user_table
ADD COLUMN name TEXT,
ADD COLUMN dob DATE;

COMMENT ON COLUMN xcentro.user_table.name IS 'User full name collected during onboarding';
COMMENT ON COLUMN xcentro.user_table.dob IS 'User date of birth collected during onboarding';
