-- Rollback: Remove CHECK constraint for token_card payment source tracking

ALTER TABLE xcentro.payments
DROP CONSTRAINT IF EXISTS chk_token_card_requires_source;
