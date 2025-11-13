-- Drop ops_user_table
DROP TRIGGER IF EXISTS update_ops_user_updated_at ON xcentro.ops_user_table;
DROP TABLE IF EXISTS xcentro.ops_user_table;
