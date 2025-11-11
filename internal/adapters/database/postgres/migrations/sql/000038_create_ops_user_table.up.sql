-- Create ops_user_table for ops dashboard users with OIDC authentication
CREATE TABLE IF NOT EXISTS ops_user_table (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL UNIQUE,

    -- OIDC tokens (should be encrypted at application level before storage)
    -- These are stored to enable token refresh and fetching updated user info from OP
    oidc_id_token TEXT,
    oidc_access_token TEXT,
    oidc_refresh_token TEXT,

    -- Role management
    -- Simple array format: ["admin", "viewer", "ops_manager"]
    -- Extracted from OIDC access token claims
    roles JSONB NOT NULL DEFAULT '[]'::jsonb,

    -- User status
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive', 'suspended')),

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Create index on email for faster lookups
CREATE INDEX idx_ops_user_email ON ops_user_table(email);

-- Create index on status for filtering active users
CREATE INDEX idx_ops_user_status ON ops_user_table(status);

-- Create GIN index on roles for efficient role-based queries
CREATE INDEX idx_ops_user_roles ON ops_user_table USING GIN(roles);

-- Add updated_at trigger
CREATE TRIGGER update_ops_user_updated_at
    BEFORE UPDATE ON ops_user_table
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Add comments for documentation
COMMENT ON TABLE ops_user_table IS 'Stores ops dashboard user accounts with OIDC authentication';
COMMENT ON COLUMN ops_user_table.oidc_id_token IS 'OIDC ID token - should be encrypted at application level';
COMMENT ON COLUMN ops_user_table.oidc_access_token IS 'OIDC access token - should be encrypted at application level';
COMMENT ON COLUMN ops_user_table.oidc_refresh_token IS 'OIDC refresh token - MUST be encrypted at application level';
COMMENT ON COLUMN ops_user_table.roles IS 'Array of role strings extracted from OIDC token claims, e.g., ["admin", "viewer"]';
