-- Drop indexes
DROP INDEX IF EXISTS xcentro.idx_user_tokens_user_device_token;
DROP INDEX IF EXISTS xcentro.idx_user_tokens_user_device;

-- Recreate old index
CREATE INDEX idx_user_tokens_user_token ON xcentro.user_tokens(user_id, refresh_token_hash);

-- Remove columns from user_tokens
ALTER TABLE xcentro.user_tokens DROP COLUMN IF EXISTS device_version;
ALTER TABLE xcentro.user_tokens DROP COLUMN IF EXISTS device_type;

-- Drop table and enum
DROP TABLE IF EXISTS xcentro.user_device_versions;
DROP TYPE IF EXISTS xcentro.device_type_enum;
