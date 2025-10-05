-- Remove name and dob fields from user_table
ALTER TABLE xcentro.user_table
DROP COLUMN IF EXISTS name,
DROP COLUMN IF EXISTS dob;
