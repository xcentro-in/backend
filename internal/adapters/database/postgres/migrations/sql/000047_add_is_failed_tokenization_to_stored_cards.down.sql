-- Remove is_failed_tokenization column from stored_cards table

ALTER TABLE xcentro.stored_cards
DROP COLUMN IF EXISTS is_failed_tokenization;
