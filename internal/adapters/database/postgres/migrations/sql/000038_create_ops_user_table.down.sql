-- Drop ops_user_table
DROP TRIGGER IF EXISTS update_ops_user_updated_at ON ops_user_table;
DROP INDEX IF EXISTS idx_ops_user_roles;
DROP INDEX IF EXISTS idx_ops_user_status;
DROP INDEX IF EXISTS idx_ops_user_email;
DROP TABLE IF EXISTS ops_user_table;
