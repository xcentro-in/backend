-- Migration: Add CHECK constraint to ensure token_card payments have payment_source tracking
-- This prevents the bug where tokenized payments are created without source_id/source_type

-- Add CHECK constraint: token_card payments MUST have both payment_source_id and payment_source_type
ALTER TABLE xcentro.payments
ADD CONSTRAINT chk_token_card_requires_source
CHECK (
    -- If payment_method is 'token_card', then both source fields must be set
    payment_method != 'token_card' OR (
        payment_source_id IS NOT NULL AND
        payment_source_type IS NOT NULL
    )
);

COMMENT ON CONSTRAINT chk_token_card_requires_source ON xcentro.payments IS
'Ensures tokenized card payments always have payment_source_id and payment_source_type set for proper tracking and reporting';
