-- Create ops_session_table for ops dashboard user sessions
CREATE TABLE IF NOT EXISTS ops_session_table (
    session_id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    ops_user_id UUID NOT NULL REFERENCES ops_user_table(id) ON DELETE CASCADE,

    -- Store hash of refresh token sent to client for session validation
    refresh_token_hash VARCHAR(255) NOT NULL,

    -- Store encrypted OIDC refresh token to communicate with OIDC Provider
    -- MUST be encrypted at application level before storage
    oidc_refresh_token TEXT NOT NULL,

    -- Session metadata
    expires_at TIMESTAMPTZ NOT NULL,
    user_agent TEXT,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Create index on ops_user_id for efficient session lookups by user
CREATE INDEX idx_ops_session_user_id ON ops_session_table(ops_user_id);

-- Create index on token hash for session validation
CREATE INDEX idx_ops_session_token_hash ON ops_session_table(refresh_token_hash);

-- Create index on expires_at for cleanup of expired sessions
CREATE INDEX idx_ops_session_expires_at ON ops_session_table(expires_at);

-- Create composite index for common query pattern (user_id + token_hash)
CREATE INDEX idx_ops_session_user_token ON ops_session_table(ops_user_id, refresh_token_hash);

-- Add comments for documentation
COMMENT ON TABLE ops_session_table IS 'Stores active sessions for ops dashboard users';
COMMENT ON COLUMN ops_session_table.refresh_token_hash IS 'SHA-256 hash of the refresh token sent to client for session validation';
COMMENT ON COLUMN ops_session_table.oidc_refresh_token IS 'Encrypted OIDC refresh token used to communicate with OIDC Provider - MUST be encrypted at application level';
COMMENT ON COLUMN ops_session_table.last_used_at IS 'Timestamp of last session activity for tracking active sessions';
