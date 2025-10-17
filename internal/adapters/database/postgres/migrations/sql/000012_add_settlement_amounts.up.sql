-- #############################################################################
-- # Phase 7.1: Settlement Amount Tracking
-- # Created: 2025-10-13
-- # Purpose: Add financial tracking for settlement amounts and gateway fees
-- #############################################################################

-- =============================================================================
-- Add Settlement Amount Columns to Payments Table
-- =============================================================================

-- Settlement financial tracking
-- These columns track the actual money flow vs what customer paid
ALTER TABLE xcentro.payments
ADD COLUMN settlement_amount BIGINT CHECK (settlement_amount >= 0),
ADD COLUMN mdr_amount BIGINT DEFAULT 0 CHECK (mdr_amount >= 0),
ADD COLUMN net_settlement_amount BIGINT CHECK (net_settlement_amount >= 0);

-- Column comments explaining the financial flow
COMMENT ON COLUMN xcentro.payments.settlement_amount IS
'Actual amount settled by gateway (may differ from total_amount due to chargebacks/adjustments)';

COMMENT ON COLUMN xcentro.payments.mdr_amount IS
'Merchant Discount Rate - gateway fees deducted from settlement (MDR %)';

COMMENT ON COLUMN xcentro.payments.net_settlement_amount IS
'Final amount received after all deductions: settlement_amount - mdr_amount';

-- =============================================================================
-- Indexes for Financial Reconciliation
-- =============================================================================

-- Index for finding payments with settlement amount discrepancies
CREATE INDEX idx_payments_settlement_amounts
ON xcentro.payments(business_id, settlement_status, settlement_amount, total_amount)
WHERE settlement_status = 'settled' AND settlement_amount IS NOT NULL;

-- Index for MDR reporting and analytics
CREATE INDEX idx_payments_mdr_tracking
ON xcentro.payments(gateway_used, settled_at, mdr_amount)
WHERE mdr_amount > 0 AND settled_at IS NOT NULL;

-- =============================================================================
-- Add Check Constraint for Data Integrity
-- =============================================================================

-- Ensure settlement amount logic is valid
-- settlement_amount should generally be <= total_amount (customer paid)
-- net_settlement_amount should be = settlement_amount - mdr_amount
ALTER TABLE xcentro.payments
ADD CONSTRAINT chk_settlement_amount_valid
CHECK (
    (settlement_amount IS NULL) OR
    (settlement_amount <= total_amount)
);

ALTER TABLE xcentro.payments
ADD CONSTRAINT chk_net_settlement_calculation
CHECK (
    (net_settlement_amount IS NULL) OR
    (settlement_amount IS NULL) OR
    (net_settlement_amount = settlement_amount - mdr_amount)
);

-- =============================================================================
-- Documentation Comments
-- =============================================================================

COMMENT ON TABLE xcentro.payments IS
'Main payment transaction records with gateway routing, fee tracking, and settlement reconciliation';

-- Financial flow example:
-- Customer pays (total_amount): ₹1000
-- Gateway settles (settlement_amount): ₹1000 (usually same, but can differ)
-- Gateway charges MDR (mdr_amount): ₹20 (2% MDR)
-- Business receives (net_settlement_amount): ₹980
