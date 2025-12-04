-- Make gateway_used nullable in stored_cards table
-- This allows cards to be created in Phase 1 (before gateway selection)
-- and updated in Phase 2 (after tokenization with selected gateway)

ALTER TABLE xcentro.stored_cards
ALTER COLUMN gateway_used DROP NOT NULL;

COMMENT ON COLUMN xcentro.stored_cards.gateway_used IS 'Gateway that tokenized this card. NULL during Phase 1 (pending tokenization), set in Phase 2 after gateway selection.';
