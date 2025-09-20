-- Users table
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) UNIQUE NOT NULL,
    mobile VARCHAR(20),
    pan VARCHAR(10),
    kyc_status INTEGER NOT NULL DEFAULT 0,
    kyc_data TEXT,
    email_verified BOOLEAN NOT NULL DEFAULT FALSE,
    mobile_verified BOOLEAN NOT NULL DEFAULT FALSE,
    role INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- User verification codes table
CREATE TABLE IF NOT EXISTS user_verification_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code VARCHAR(10) NOT NULL,
    code_type INTEGER NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    used BOOLEAN NOT NULL DEFAULT FALSE,
    retry_attempts INTEGER NOT NULL DEFAULT 0
);

-- Create indexes for better performance
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_mobile ON users(mobile);
CREATE INDEX IF NOT EXISTS idx_verification_codes_user_id ON user_verification_codes(user_id);
CREATE INDEX IF NOT EXISTS idx_verification_codes_code ON user_verification_codes(code);
CREATE INDEX IF NOT EXISTS idx_verification_codes_expires_at ON user_verification_codes(expires_at);

-- Create updated_at trigger function
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger for users table
DROP TRIGGER IF EXISTS update_users_updated_at ON users;
CREATE TRIGGER update_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Add comments for better documentation
COMMENT ON TABLE users IS 'User accounts and profile information';
COMMENT ON COLUMN users.kyc_status IS '0=NotStarted, 1=InReview, 2=NeedsUpdate, 3=Verified, 4=Rejected';
COMMENT ON COLUMN users.role IS '0=Owner, 1=Admin, 2=Finance, 3=Support';

COMMENT ON TABLE user_verification_codes IS 'Email and mobile verification codes';
COMMENT ON COLUMN user_verification_codes.code_type IS '0=EmailVerification, 1=MobileVerification';