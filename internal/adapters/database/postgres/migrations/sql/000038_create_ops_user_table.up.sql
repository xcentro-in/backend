-- Create ops_user_table for ops dashboard users with OIDC authentication
CREATE TABLE IF NOT EXISTS ops_user_table (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL UNIQUE,

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

-- Add updated_at trigger
CREATE TRIGGER update_ops_user_updated_at
    BEFORE UPDATE ON ops_user_table
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Add comments for documentation
COMMENT ON TABLE ops_user_table IS 'Stores ops dashboard user accounts with OIDC authentication';
COMMENT ON COLUMN ops_user_table.roles IS 'Array of role strings extracted from OIDC token claims, e.g., ["admin", "viewer"]';
