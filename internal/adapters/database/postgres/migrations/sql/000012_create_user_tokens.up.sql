-- Add user_tokens table for JWT refresh token tracking
CREATE TABLE IF NOT EXISTS xcentro.user_tokens (
    session_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES xcentro.user_table(id) ON DELETE CASCADE,
    refresh_token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    last_used_at TIMESTAMPTZ DEFAULT NOW(),
    user_agent TEXT
);

-- Composite index on (user_id, refresh_token_hash) for fast lookups
CREATE INDEX idx_user_tokens_user_token ON xcentro.user_tokens(user_id, refresh_token_hash);

-- Index on expires_at for efficient cleanup of expired tokens
CREATE INDEX idx_user_tokens_expires_at ON xcentro.user_tokens(expires_at);

