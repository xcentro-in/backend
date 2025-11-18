-- Add is_failed_tokenization column to stored_cards table
-- This explicitly marks cards where tokenization payment failed
-- Differentiates from user-deactivated cards

ALTER TABLE xcentro.stored_cards
ADD COLUMN is_failed_tokenization BOOLEAN DEFAULT FALSE NOT NULL;

COMMENT ON COLUMN xcentro.stored_cards.is_failed_tokenization IS 'True if tokenization payment failed (vs user deactivation). Failed cards shown to user for 7 days.';

-- Backfill existing failed cards (where is_active = false AND failure_reason is not null)
UPDATE xcentro.stored_cards
SET is_failed_tokenization = TRUE
WHERE is_active = FALSE AND failure_reason IS NOT NULL;
