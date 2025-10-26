-- #############################################################################
-- # Rollback: Remove payment terms and vendor reference from payment vouchers
-- # Migration: 000024
-- #############################################################################

-- Drop the added columns
ALTER TABLE xcentro.payment_vouchers
  DROP COLUMN IF EXISTS payment_terms,
  DROP COLUMN IF EXISTS vendor_reference_number;
