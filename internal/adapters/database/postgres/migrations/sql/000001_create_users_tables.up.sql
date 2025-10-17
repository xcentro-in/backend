-- #############################################################################
-- # Custom ENUM Types for Data Integrity
-- #############################################################################
-- Enum literals (snake_case) for consistency across DB + serialization layers.
CREATE TYPE xcentro.verification_type AS ENUM ('email_verification', 'mobile_verification','password_reset');
CREATE TYPE xcentro.kyc_status_enum AS ENUM ('kyc_not_started', 'kyc_in_progress', 'kyc_completed', 'kyc_verified', 'kyc_needs_update', 'kyc_rejected');
CREATE TYPE xcentro.user_role_enum AS ENUM ('owner', 'admin', 'manager', 'auditor', 'none');
CREATE TYPE xcentro.business_type_enum AS ENUM (
    'sole_proprietorship',
    'partnership',
    'limited_liability_partnership',
    'private_limited_company',
    'one_person_company',
    'section_8_non_profit',
    'trust',
    'ngo',
    'society',
    'cooperative_society',
    'other'
);
CREATE TYPE xcentro.subscription_type_enum AS ENUM ('free_plan', 'basic_plan', 'premium_plan', 'enterprise_plan');


-- UUIDv7 Generator - Fast, Secure, RFC 9562 Compliant
-- Note: place the function inside the xcentro schema for namespacing
-- CREATE OR REPLACE FUNCTION xcentro.generate_uuid_v7(
--     p_timestamp TIMESTAMPTZ DEFAULT clock_timestamp()
-- )
-- RETURNS UUID
-- LANGUAGE plpgsql
-- VOLATILE
-- AS $$
-- DECLARE
--     -- Epoch time as double precision for microsecond extraction
--     v_time          DOUBLE PRECISION;
--     -- 48-bit UNIX timestamp in milliseconds
--     unix_ts_ms      BIGINT;
--     -- Microsecond component for sub-millisecond precision
--     v_microsec      BIGINT;
--     -- The full 16-byte UUID data
--     uuid_bytes      BYTEA;
    
--     -- Constants for microsecond calculation
--     c_milli         DOUBLE PRECISION := 1000.0;      -- 1,000
--     c_micro         DOUBLE PRECISION := 1000000.0;   -- 1,000,000
--     c_scale         DOUBLE PRECISION := 4.096;       -- 4.0 * (1024 / 1000) for 12-bit precision
-- BEGIN
--     -- 1. Extract epoch time with microsecond precision
--     v_time := EXTRACT(EPOCH FROM p_timestamp);
    
--     -- 2. Get millisecond timestamp (48 bits)
--     unix_ts_ms := TRUNC(v_time * c_milli);
    
--     -- 3. Extract microseconds within the current millisecond and encode in 12 bits
--     -- This gives us sub-millisecond ordering capability
--     v_microsec := TRUNC((v_time * c_micro - unix_ts_ms * c_milli) * c_scale);
    
--     -- 4. Generate 16 cryptographically secure random bytes as base
--     uuid_bytes := xcentro.gen_random_bytes(16);
    
--     -- 5. Overwrite the first 6 bytes (48 bits) with the big-endian timestamp
--     uuid_bytes := OVERLAY(uuid_bytes PLACING SUBSTRING(INT8SEND(unix_ts_ms) FROM 3 FOR 6) FROM 1);
    
--     -- 6. Set the version (7) and embed microsecond precision in bytes 6-7
--     -- Byte 6: version (4 bits) + high 4 bits of microseconds
--     -- Byte 7: low 8 bits of microseconds
--     uuid_bytes := SET_BYTE(uuid_bytes, 6, ((7 << 4) | ((v_microsec >> 8) & 15))::INTEGER);
--     uuid_bytes := SET_BYTE(uuid_bytes, 7, (v_microsec & 255)::INTEGER);
    
--     -- 7. Set the 2-bit variant (RFC 4122) in the 9th byte
--     --    Variant is 10xxxxxx, preserving the random bits in the lower 6 bits
--     uuid_bytes := SET_BYTE(uuid_bytes, 8, (GET_BYTE(uuid_bytes, 8) & 63) | 128);
    
--     -- 8. Convert the final byte array to a UUID and return it
--     RETURN ENCODE(uuid_bytes, 'hex')::UUID;
-- END;
-- $$;

-- #############################################################################
-- # Automatic Timestamp Trigger Function
-- #############################################################################
CREATE OR REPLACE FUNCTION xcentro.trigger_set_timestamp()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
COMMENT ON FUNCTION xcentro.trigger_set_timestamp() IS 'This trigger function automatically sets the updated_at column to the current timestamp whenever a row is updated.';


-- #############################################################################
-- # User Identity / KYC (Externalized, Meaningful Names)
-- #############################################################################
CREATE TABLE xcentro.user_identity (
    tax_id TEXT PRIMARY KEY,          -- e.g. PAN / generic tax identifier
    tax_id_data JSONB,                -- structured data / verification payload for tax identifier
    national_id TEXT UNIQUE,          -- e.g. Aadhaar / SSN / National Registry number (optional)
    national_id_data JSONB,           -- structured data / verification payload for national id
    n_txn_id TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ
);
COMMENT ON TABLE xcentro.user_identity IS 'Externalized personal identity & compliance artifacts (tax_id, national_id, with associated metadata).';
CREATE INDEX IF NOT EXISTS idx_person_identity_national_id ON xcentro.user_identity(national_id);
CREATE TRIGGER set_timestamp
BEFORE UPDATE ON xcentro.user_identity
FOR EACH ROW
EXECUTE PROCEDURE xcentro.trigger_set_timestamp();


-- #############################################################################
-- # User Entity
-- #############################################################################

-- Table for user accounts, authentication, and KYC data
CREATE TABLE xcentro.user_table (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    name TEXT,
    dob DATE,
    email VARCHAR(254) UNIQUE NOT NULL,
    pwd_hash TEXT NOT NULL,
    mobile VARCHAR(20),
    tax_id TEXT REFERENCES xcentro.user_identity(tax_id), -- externalized PAN (generic tax identifier)
    kyc_status xcentro.kyc_status_enum NOT NULL DEFAULT 'kyc_not_started',
    kyc_needed BOOLEAN DEFAULT TRUE, -- kyc_needed column default to TRUE (new users should do KYC by default)
    email_verified BOOLEAN DEFAULT FALSE,
    mobile_verified BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ
);
COMMENT ON TABLE xcentro.user_table IS 'User accounts, authentication, and KYC data.';
COMMENT ON COLUMN xcentro.user_table.name IS 'User full name collected during onboarding';
COMMENT ON COLUMN xcentro.user_table.dob IS 'User date of birth collected during onboarding';
COMMENT ON COLUMN xcentro.user_table.kyc_needed IS 'Whether KYC is required for this user (default TRUE for new users, can be set to FALSE for invited users based on business requirements)';

-- Table for user verification codes
CREATE TABLE xcentro.user_verification_codes (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id UUID REFERENCES xcentro.user_table(id) ON DELETE CASCADE,
    code TEXT,
    code_type xcentro.verification_type,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    used BOOLEAN DEFAULT FALSE,
    retry_attempts INTEGER DEFAULT 0
);
CREATE INDEX idx_user_verification_codes_user_id ON xcentro.user_verification_codes(user_id);


-- #############################################################################
-- # Business Entity
-- #############################################################################

-- Table for Know Your Business (KYB) identity and corporate data
CREATE TABLE xcentro.kyb_data (
    income_tax_id TEXT PRIMARY KEY,
    income_tax_id_data JSONB,
    company_id TEXT,
    company_id_data JSONB,
    business_deed TEXT
);
COMMENT ON TABLE xcentro.kyb_data IS 'Stores Know Your Business (KYB) identity and corporate data.';

-- Table for business entities and their KYB information
CREATE TABLE xcentro.business (
    bid UUID PRIMARY KEY DEFAULT uuidv7(),
    name TEXT,
    gstin VARCHAR(15) UNIQUE,
    gst_data JSONB,
    income_tax_id TEXT REFERENCES xcentro.kyb_data(income_tax_id),
    kyb_status xcentro.kyc_status_enum DEFAULT 'kyc_not_started',
    business_type xcentro.business_type_enum,
    business_model TEXT,
    business_turnover TEXT,
    country_code VARCHAR(3),
    currency_code VARCHAR(3),
    updated_at TIMESTAMPTZ
);
COMMENT ON TABLE xcentro.business IS 'Business entities and their KYB information.';
-- Correct column for income tax reference index
CREATE INDEX idx_business_income_tax_id ON xcentro.business(income_tax_id);
CREATE TRIGGER set_timestamp
BEFORE UPDATE ON xcentro.business
FOR EACH ROW
EXECUTE PROCEDURE xcentro.trigger_set_timestamp();

-- Table to map users to businesses with specific roles
CREATE TABLE xcentro.business_user_relation (
    user_id UUID NOT NULL REFERENCES xcentro.user_table(id) ON DELETE CASCADE,
    bid UUID NOT NULL REFERENCES xcentro.business(bid) ON DELETE CASCADE,
    role xcentro.user_role_enum,
    role_claim xcentro.user_role_enum,
    kyb_initiated BOOLEAN DEFAULT FALSE,
    kyc_needed BOOLEAN DEFAULT TRUE,
    added_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ,
    PRIMARY KEY (user_id, bid)
);
COMMENT ON TABLE xcentro.business_user_relation IS 'Maps users to businesses with specific roles.';
CREATE INDEX idx_business_user_relation_bid ON xcentro.business_user_relation(bid);
CREATE TRIGGER set_timestamp
BEFORE UPDATE ON xcentro.business_user_relation
FOR EACH ROW
EXECUTE PROCEDURE xcentro.trigger_set_timestamp();


-- ##############################################################################
-- # Subscription Entity
-- #############################################################################

CREATE TABLE xcentro.subscription_status (
    bid UUID PRIMARY KEY NOT NULL REFERENCES xcentro.business(bid) ON DELETE CASCADE,
    subscription_type xcentro.subscription_type_enum NOT NULL,
    start_date DATE,
    expiry_date DATE
);
COMMENT ON TABLE xcentro.subscription_status IS 'Tracks the subscription status of businesses.';

-- Trigger for user_table updates (missing earlier)
CREATE TRIGGER set_timestamp
BEFORE UPDATE ON xcentro.user_table
FOR EACH ROW
EXECUTE PROCEDURE xcentro.trigger_set_timestamp();
