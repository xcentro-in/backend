-- Rollback: Restore unique constraint on (business_id, token)
-- WARNING: This will fail if there are duplicate (business_id, token) pairs

-- Recreate unique index
CREATE UNIQUE INDEX idx_stored_cards_token_unique
ON xcentro.stored_cards(business_id, token);

COMMENT ON TABLE xcentro.stored_cards IS 'Tokenized card storage for recurring payments (PCI DSS compliant)';
