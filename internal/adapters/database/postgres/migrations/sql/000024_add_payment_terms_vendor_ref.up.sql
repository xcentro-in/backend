-- #############################################################################
-- # Add payment terms and vendor reference to payment vouchers
-- # Migration: 000024
-- #############################################################################

-- Add payment terms and vendor reference number columns
ALTER TABLE xcentro.payment_vouchers
  ADD COLUMN payment_terms VARCHAR(50),
  ADD COLUMN vendor_reference_number VARCHAR(100);

-- Add comments for documentation
COMMENT ON COLUMN xcentro.payment_vouchers.payment_terms IS 'Payment terms from invoice (e.g., "Net 30", "Net 45", "Due on Receipt", "2/10 Net 30")';
COMMENT ON COLUMN xcentro.payment_vouchers.vendor_reference_number IS 'Vendor''s reference or your PO number that appears on the invoice';
