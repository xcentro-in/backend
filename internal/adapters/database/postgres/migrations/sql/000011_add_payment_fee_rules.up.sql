-- #############################################################################
-- # Payment Fee Rules Table
-- # Phase 10: Database-backed convenience fee configuration
-- #############################################################################

-- Create fee type enum for type-safe fee calculation methods
CREATE TYPE xcentro.fee_type_enum AS ENUM ('percentage', 'fixed', 'hybrid');

-- Fee rules table for configurable convenience fees
CREATE TABLE xcentro.payment_fee_rules (
    rule_id UUID PRIMARY KEY DEFAULT uuidv7(),

    -- Rule matching criteria (REUSE existing enums from 000002)
    payment_method xcentro.payment_method_enum NOT NULL,
    card_type xcentro.card_type_enum,              -- NULL = applies to all card types
    card_category xcentro.card_category_enum,      -- NULL = applies to all categories

    -- Fee structure
    fee_type xcentro.fee_type_enum NOT NULL,
    percentage_fee DECIMAL(5,2),                   -- e.g., 1.90 for 1.9%
    fixed_fee BIGINT DEFAULT 0,                    -- in paise (e.g., 2800 for ₹28)
    min_fee BIGINT DEFAULT 0,                      -- minimum fee cap in paise
    max_fee BIGINT,                                -- maximum fee cap in paise

    -- GST configuration
    gst_percentage DECIMAL(5,2) DEFAULT 18.00,     -- GST rate (18% for India)
    gst_applicable BOOLEAN DEFAULT true,            -- whether GST applies to this fee

    -- Rule lifecycle management
    priority INT DEFAULT 0,                         -- higher priority rules take precedence
    is_active BOOLEAN DEFAULT true,
    effective_from TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    effective_until TIMESTAMPTZ,                    -- NULL = no expiry

    -- Audit fields
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by UUID REFERENCES xcentro.user_table(id),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Table and column comments for clarity
COMMENT ON TABLE xcentro.payment_fee_rules IS 'Configurable convenience fee rules for payment methods - enables quarterly fee updates without code deployment';
COMMENT ON COLUMN xcentro.payment_fee_rules.fixed_fee IS 'Fixed fee in paise (e.g., 2800 = ₹28.00)';
COMMENT ON COLUMN xcentro.payment_fee_rules.percentage_fee IS 'Percentage fee (e.g., 1.90 = 1.9%)';
COMMENT ON COLUMN xcentro.payment_fee_rules.fee_type IS 'Calculation method: percentage, fixed, or hybrid (both)';
COMMENT ON COLUMN xcentro.payment_fee_rules.priority IS 'Higher values take precedence when multiple rules match';
COMMENT ON COLUMN xcentro.payment_fee_rules.effective_from IS 'Rule becomes active from this timestamp';
COMMENT ON COLUMN xcentro.payment_fee_rules.effective_until IS 'Rule expires at this timestamp (NULL = never expires)';

-- Indexes for fast rule lookups
CREATE INDEX idx_fee_rules_active ON xcentro.payment_fee_rules(is_active, effective_from)
    WHERE is_active = true;

CREATE INDEX idx_fee_rules_lookup ON xcentro.payment_fee_rules(
    payment_method, card_type, card_category, priority
) WHERE is_active = true;

-- Trigger to auto-update updated_at timestamp
CREATE TRIGGER set_timestamp
BEFORE UPDATE ON xcentro.payment_fee_rules
FOR EACH ROW
EXECUTE PROCEDURE xcentro.trigger_set_timestamp();

-- #############################################################################
-- # Seed Data: Current Fee Rules
-- #############################################################################

-- Card fees (percentage-based)
INSERT INTO xcentro.payment_fee_rules (
    payment_method, card_type, card_category,
    fee_type, percentage_fee, gst_percentage, priority
) VALUES
    -- Mastercard
    ('card', 'mastercard', 'debit', 'percentage', 1.90, 18.00, 10),
    ('card', 'mastercard', 'credit', 'percentage', 2.20, 18.00, 10),

    -- Visa
    ('card', 'visa', 'debit', 'percentage', 1.90, 18.00, 10),
    ('card', 'visa', 'credit', 'percentage', 2.20, 18.00, 10),

    -- RuPay (lower fee - government preferred)
    ('card', 'rupay', NULL, 'percentage', 0.90, 18.00, 10),

    -- Premium cards (higher fees)
    ('card', 'amex', NULL, 'percentage', 3.00, 18.00, 10),
    ('card', 'diners', NULL, 'percentage', 3.00, 18.00, 10);

-- Net Banking fee (FIXED ₹28)
INSERT INTO xcentro.payment_fee_rules (
    payment_method, card_type, card_category,
    fee_type, fixed_fee, gst_percentage, priority
) VALUES
    ('net_banking', NULL, NULL, 'fixed', 2800, 18.00, 10);

-- UPI fee (FIXED ₹5)
INSERT INTO xcentro.payment_fee_rules (
    payment_method, card_type, card_category,
    fee_type, fixed_fee, gst_percentage, priority
) VALUES
    ('upi', NULL, NULL, 'fixed', 500, 18.00, 10);
