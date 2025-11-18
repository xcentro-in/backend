-- Rollback: Make gateway_used NOT NULL again
-- WARNING: This will fail if there are any rows with NULL gateway_used

-- First, update any NULL values to a default (should not exist in production)
UPDATE xcentro.stored_cards
SET gateway_used = 'mockpg'
WHERE gateway_used IS NULL;

-- Then add back NOT NULL constraint
ALTER TABLE xcentro.stored_cards
ALTER COLUMN gateway_used SET NOT NULL;

COMMENT ON COLUMN xcentro.stored_cards.gateway_used IS 'Gateway that tokenized this card';
