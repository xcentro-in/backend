-- Add failure_reason column to stored_cards table
-- This tracks why card tokenization failed (for audit and UX purposes)

ALTER TABLE xcentro.stored_cards
ADD COLUMN failure_reason TEXT;

COMMENT ON COLUMN xcentro.stored_cards.failure_reason IS 'Reason for tokenization failure (populated when is_active = false)';
