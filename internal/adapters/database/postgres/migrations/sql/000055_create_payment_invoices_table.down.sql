-- #############################################################################
-- # Rollback Payment Invoices System
-- #############################################################################

-- Drop the payment_invoices table
DROP TABLE IF EXISTS xcentro.payment_invoices;

-- Drop the custom enum types
DROP TYPE IF EXISTS xcentro.filing_status_enum;
DROP TYPE IF EXISTS xcentro.irn_status_enum;
