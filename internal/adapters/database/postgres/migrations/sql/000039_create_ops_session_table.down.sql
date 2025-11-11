-- Drop ops_session_table
DROP INDEX IF EXISTS idx_ops_session_user_token;
DROP INDEX IF EXISTS idx_ops_session_expires_at;
DROP INDEX IF EXISTS idx_ops_session_token_hash;
DROP INDEX IF EXISTS idx_ops_session_user_id;
DROP TABLE IF EXISTS ops_session_table;
