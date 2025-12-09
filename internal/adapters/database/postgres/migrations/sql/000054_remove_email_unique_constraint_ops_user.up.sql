-- Remove UNIQUE constraint from email column in ops_user_table
-- OID is now the primary key, email can change for the same user
ALTER TABLE xcentro.ops_user_table
DROP CONSTRAINT IF EXISTS ops_user_table_email_key;

-- Add comment explaining the change
COMMENT ON COLUMN xcentro.ops_user_table.email IS 'User email from Entra ID (can change, use OID as primary identifier)';
COMMENT ON COLUMN xcentro.ops_user_table.id IS 'OID from Entra ID access token (immutable user identifier)';
