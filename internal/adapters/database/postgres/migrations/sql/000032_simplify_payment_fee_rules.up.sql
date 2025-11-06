-- #############################################################################
-- # Simplify Payment Fee Rules Table
-- # Remove unnecessary columns and complexity
-- #############################################################################

-- Drop unnecessary indexes
DROP INDEX IF EXISTS xcentro.idx_fee_rules_active;
DROP INDEX IF EXISTS xcentro.idx_fee_rules_lookup;

-- Remove unnecessary columns
ALTER TABLE xcentro.payment_fee_rules
    DROP COLUMN IF EXISTS priority,
    DROP COLUMN IF EXISTS effective_from,
    DROP COLUMN IF EXISTS effective_until,
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS min_fee,
    DROP COLUMN IF EXISTS max_fee;

-- Create simpler lookup index (exact match only)
CREATE INDEX idx_fee_rules_lookup ON xcentro.payment_fee_rules(
    payment_method, card_type, card_category
) WHERE is_active = true;

-- Delete existing generic rules (NULL card_category for Amex/RuPay/Diners)
DELETE FROM xcentro.payment_fee_rules
WHERE card_type IN ('amex', 'rupay', 'diners') AND card_category IS NULL;

-- Insert explicit rules for each card type + category combination
-- This eliminates the need for NULL fallback logic
INSERT INTO xcentro.payment_fee_rules (
    payment_method, card_type, card_category,
    fee_type, percentage_fee, gst_percentage
) VALUES
    -- RuPay: same fee for all categories (0.9%)
    ('card', 'rupay', 'credit', 'percentage', 0.90, 18.00),
    ('card', 'rupay', 'debit', 'percentage', 0.90, 18.00),
    ('card', 'rupay', 'corporate', 'percentage', 0.90, 18.00),

    -- Amex: same fee for all categories (3.0%)
    ('card', 'amex', 'credit', 'percentage', 3.00, 18.00),
    ('card', 'amex', 'debit', 'percentage', 3.00, 18.00),
    ('card', 'amex', 'corporate', 'percentage', 3.00, 18.00),

    -- Diners: same fee for all categories (3.0%)
    ('card', 'diners', 'credit', 'percentage', 3.00, 18.00),
    ('card', 'diners', 'debit', 'percentage', 3.00, 18.00),
    ('card', 'diners', 'corporate', 'percentage', 3.00, 18.00);

-- Update table comment
COMMENT ON TABLE xcentro.payment_fee_rules IS 'Simplified fee rules with explicit rules only - no NULL fallback complexity';
COMMENT ON COLUMN xcentro.payment_fee_rules.card_type IS 'Card type (NULL only for non-card payment methods like net_banking, upi)';
COMMENT ON COLUMN xcentro.payment_fee_rules.card_category IS 'Card category (NULL only for non-card payment methods)';
