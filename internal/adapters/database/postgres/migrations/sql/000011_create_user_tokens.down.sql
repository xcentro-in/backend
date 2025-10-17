-- Drop user_tokens table
DROP INDEX IF EXISTS xcentro.idx_user_tokens_expires_at;
DROP INDEX IF EXISTS xcentro.idx_user_tokens_user_token;
DROP TABLE IF EXISTS xcentro.user_tokens;

