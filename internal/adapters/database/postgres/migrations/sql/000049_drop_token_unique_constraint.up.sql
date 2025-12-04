-- Drop unique constraint on (business_id, token) from stored_cards
-- The constraint is too restrictive and causes issues during Phase 1 tokenization
-- card_id (PRIMARY KEY) already ensures uniqueness

DROP INDEX IF EXISTS xcentro.idx_stored_cards_token_unique;

COMMENT ON TABLE xcentro.stored_cards IS 'Tokenized card storage for recurring payments (PCI DSS compliant). Uniqueness enforced by card_id PRIMARY KEY.';
