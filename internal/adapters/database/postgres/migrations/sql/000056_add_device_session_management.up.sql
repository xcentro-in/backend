-- Create device_type enum for session management
CREATE TYPE xcentro.device_type_enum AS ENUM ('web', 'mobile');

-- Create user_device_versions table to track JWT version per device type
CREATE TABLE xcentro.user_device_versions (
    user_id UUID NOT NULL REFERENCES xcentro.user_table(id) ON DELETE CASCADE,
    device_type xcentro.device_type_enum NOT NULL,
    current_version INT NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_id, device_type)
);

-- Add trigger to update updated_at
CREATE TRIGGER set_timestamp
BEFORE UPDATE ON xcentro.user_device_versions
FOR EACH ROW
EXECUTE PROCEDURE xcentro.trigger_set_timestamp();

-- Add device_type column to user_tokens table
ALTER TABLE xcentro.user_tokens
ADD COLUMN device_type xcentro.device_type_enum NOT NULL DEFAULT 'web';

-- Add device_version column to user_tokens table
ALTER TABLE xcentro.user_tokens
ADD COLUMN device_version INT NOT NULL DEFAULT 1;

-- Create index for efficient device-specific session queries
CREATE INDEX idx_user_tokens_user_device ON xcentro.user_tokens(user_id, device_type, created_at DESC);

-- Drop old composite index as we have better one now
DROP INDEX IF EXISTS xcentro.idx_user_tokens_user_token;

-- Recreate composite index with device_type
CREATE INDEX idx_user_tokens_user_device_token ON xcentro.user_tokens(user_id, device_type, refresh_token_hash);

COMMENT ON TABLE xcentro.user_device_versions IS 'Tracks JWT version number per user per device type for session invalidation';
COMMENT ON COLUMN xcentro.user_device_versions.current_version IS 'Incremental version number - incremented when max sessions reached for device type';
COMMENT ON COLUMN xcentro.user_tokens.device_type IS 'Device type for this session (web/mobile)';
COMMENT ON COLUMN xcentro.user_tokens.device_version IS 'JWT version number at time of session creation - used for bulk invalidation';
