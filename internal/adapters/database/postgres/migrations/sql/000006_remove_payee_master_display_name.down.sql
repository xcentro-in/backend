-- Rollback: Add back display_name to payee_master
-- Note: This is for rollback only. Original data will be lost.

ALTER TABLE xcentro.payee_master
ADD COLUMN display_name VARCHAR(500) NOT NULL DEFAULT 'Unknown Payee';

-- Remove default after adding column (for new inserts, they must provide it)
ALTER TABLE xcentro.payee_master
ALTER COLUMN display_name DROP DEFAULT;
