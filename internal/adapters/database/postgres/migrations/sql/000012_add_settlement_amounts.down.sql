-- #############################################################################
-- # Phase 7.1: Rollback Settlement Amount Tracking
-- # Purpose: Remove settlement amount columns from payments table
-- #############################################################################

-- =============================================================================
-- Drop Indexes
-- =============================================================================

DROP INDEX IF EXISTS xcentro.idx_payments_mdr_tracking;
DROP INDEX IF EXISTS xcentro.idx_payments_settlement_amounts;

-- =============================================================================
-- Drop Constraints
-- =============================================================================

ALTER TABLE xcentro.payments
DROP CONSTRAINT IF EXISTS chk_net_settlement_calculation;

ALTER TABLE xcentro.payments
DROP CONSTRAINT IF EXISTS chk_settlement_amount_valid;

-- =============================================================================
-- Drop Columns
-- =============================================================================

ALTER TABLE xcentro.payments
DROP COLUMN IF EXISTS net_settlement_amount,
DROP COLUMN IF EXISTS mdr_amount,
DROP COLUMN IF EXISTS settlement_amount;
