-- Remove failure_reason column from stored_cards table

ALTER TABLE xcentro.stored_cards
DROP COLUMN IF EXISTS failure_reason;
