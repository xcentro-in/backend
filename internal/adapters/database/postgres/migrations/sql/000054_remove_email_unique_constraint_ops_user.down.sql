-- Restore UNIQUE constraint on email column
-- This migration cannot be fully reversed if duplicate emails exist
ALTER TABLE xcentro.ops_user_table
ADD CONSTRAINT ops_user_table_email_key UNIQUE (email);

-- Remove comments
COMMENT ON COLUMN xcentro.ops_user_table.email IS NULL;
COMMENT ON COLUMN xcentro.ops_user_table.id IS NULL;
