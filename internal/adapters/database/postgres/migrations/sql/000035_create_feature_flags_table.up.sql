-- #############################################################################
-- # Create feature_flags table for global/universal feature toggles
-- # Migration: 000035
-- #############################################################################

-- Create table for feature flags
CREATE TABLE xcentro.feature_flags (
    flag_id UUID PRIMARY KEY DEFAULT uuidv7(),

    -- Feature flag name (e.g., REWARD_POINT_SYSTEM, PAYMENT_METHOD_CARD)
    flag_name VARCHAR(100) NOT NULL UNIQUE,

    -- Feature flag value (can be boolean, string, or JSON)
    flag_value JSONB NOT NULL DEFAULT 'false',

    -- Description of what this flag controls
    description TEXT,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for performance
CREATE INDEX idx_feature_flags_name ON xcentro.feature_flags(flag_name);

-- Comments for documentation
COMMENT ON TABLE xcentro.feature_flags IS 'Global/universal feature flags for enabling/disabling features across all businesses';
COMMENT ON COLUMN xcentro.feature_flags.flag_name IS 'Name of the feature flag (e.g., REWARD_POINT_SYSTEM) - unique across system';
COMMENT ON COLUMN xcentro.feature_flags.flag_value IS 'Value of the flag - can be boolean, string, array, or object stored as JSONB';

INSERT INTO xcentro.feature_flags (flag_name, flag_value, description)
  VALUES
    ('REWARD_POINT_SYSTEM', 'false', 'Enable reward points system'),
    ('BUSINESS_ANALYTICS', 'false', 'Enable business analytics'),
    ('GST_PAYMENTS', 'false', 'Enable GST payments'),
    ('CREDIT_CARD_MANAGEMENT', 'false', 'Enable credit card management'),
    ('BILLER_PAYMENTS', 'false', 'Enable biller payments'),
    ('ALLOWED_FIELDS', 'true', 'Enable allowed fields'),
    ('REFERRAL_SYSTEM', 'false', 'Enable referral system'),
    ('ERP_INTEGRATION', 'false', 'Enable ERP integration'),
    ('TWOFA_AUTHENTICATION', 'false', 'Enable 2FA'),
    ('AUTHORIZED_DEVICES', 'false', 'Enable authorized devices'),
    ('PAYMENT_CALENDAR_ADVANCED', 'false', 'Enable advanced payment calendar'),
    ('PAYMENT_METHOD_CARD', 'true', 'Enable card payment method'),
    ('PAYMENT_METHOD_BANK', 'false', 'Enable bank payment method'),
    ('PAYMENT_METHOD_BNPL', 'false', 'Enable BNPL payment method'),
    ('PAYMENT_METHOD_UPI', 'false', 'Enable UPI payment method'),
    ('MULTIPLE_ALLOWANCES', '[{"userRoles":["developer","owner"]}]', 'Role-based allowances');