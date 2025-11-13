-- Migration 000042: Add fee breakdown columns to payments table
-- Purpose: Store convenience fee breakdown to avoid reverse calculation and rounding errors

-- Add columns to store fee breakdown
ALTER TABLE xcentro.payments
ADD COLUMN convenience_fee BIGINT DEFAULT 0 CHECK (convenience_fee >= 0),
ADD COLUMN convenience_fee_gst BIGINT DEFAULT 0 CHECK (convenience_fee_gst >= 0);

COMMENT ON COLUMN xcentro.payments.convenience_fee IS 'Base convenience fee before GST (in paise)';
COMMENT ON COLUMN xcentro.payments.convenience_fee_gst IS 'GST on convenience fee - 18% (in paise)';
COMMENT ON COLUMN xcentro.payments.fee_amount IS 'Total fee amount (convenience_fee + convenience_fee_gst) - kept for backward compatibility';
