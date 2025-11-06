-- #############################################################################
-- # Rollback: Restore Payment Fee Rules Table to Previous State
-- #############################################################################

-- Drop simplified index
DROP INDEX IF EXISTS xcentro.idx_fee_rules_lookup;

-- Add back removed columns
ALTER TABLE xcentro.payment_fee_rules
    ADD COLUMN priority INT DEFAULT 0,
    ADD COLUMN effective_from TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN effective_until TIMESTAMPTZ,
    ADD COLUMN created_by UUID REFERENCES xcentro.user_table(id),
    ADD COLUMN min_fee BIGINT DEFAULT 0,
    ADD COLUMN max_fee BIGINT;

-- Restore original indexes
CREATE INDEX idx_fee_rules_active ON xcentro.payment_fee_rules(is_active, effective_from)
    WHERE is_active = true;

CREATE INDEX idx_fee_rules_lookup ON xcentro.payment_fee_rules(
    payment_method, card_type, card_category, priority
) WHERE is_active = true;

-- Delete explicit rules added in the up migration
DELETE FROM xcentro.payment_fee_rules
WHERE card_type IN ('amex', 'rupay', 'diners') AND card_category IS NOT NULL;

-- Restore generic rules (NULL card_category)
INSERT INTO xcentro.payment_fee_rules (
    payment_method, card_type, card_category,
    fee_type, percentage_fee, gst_percentage, priority
) VALUES
    ('card', 'rupay', NULL, 'percentage', 0.90, 18.00, 10),
    ('card', 'amex', NULL, 'percentage', 3.00, 18.00, 10),
    ('card', 'diners', NULL, 'percentage', 3.00, 18.00, 10);

-- Restore original comments
COMMENT ON TABLE xcentro.payment_fee_rules IS 'Configurable convenience fee rules for payment methods - enables quarterly fee updates without code deployment';
COMMENT ON COLUMN xcentro.payment_fee_rules.priority IS 'Higher values take precedence when multiple rules match';
COMMENT ON COLUMN xcentro.payment_fee_rules.effective_from IS 'Rule becomes active from this timestamp';
COMMENT ON COLUMN xcentro.payment_fee_rules.effective_until IS 'Rule expires at this timestamp (NULL = never expires)';
