-- #############################################################################
-- # Payment Invoices System Database Schema
-- # Phase 4.4: Create payment invoices table for GST invoice generation
-- #############################################################################

-- #############################################################################
-- # Custom ENUM Types for Invoice System
-- #############################################################################

-- IRN (Invoice Reference Number) status lifecycle
CREATE TYPE xcentro.irn_status_enum AS ENUM ('pending', 'irn_generated', 'failed', 'cancelled');

-- Invoice filing status with GST portal
CREATE TYPE xcentro.filing_status_enum AS ENUM ('not_filed', 'filed', 'failed');


-- #############################################################################
-- # Payment Invoices Table (GST Invoice Generation)
-- #############################################################################
CREATE TABLE xcentro.payment_invoices (
    invoice_id SERIAL PRIMARY KEY,
    payment_id UUID NOT NULL UNIQUE REFERENCES xcentro.payments(payment_id) ON DELETE CASCADE,

    -- Invoice identification
    invoice_number VARCHAR(50) NOT NULL,     -- Invoice reference number (e.g. INV-001)
    gstin_supplier VARCHAR(15) NOT NULL DEFAULT '27AATCM5865F1ZU',     -- Supplier GSTIN (XCentro)
    gstin_recipient VARCHAR(15) NOT NULL,    -- Client's GSTIN

    invoice_date DATE NOT NULL,

    -- IRN (Invoice Reference Number) details
    irn CHAR(64) UNIQUE,                      -- Invoice Reference Number (SHA256 hash)
    irn_ack_no VARCHAR(20),                   -- Acknowledgement number from IRP
    irn_ack_date TIMESTAMPTZ,                 -- Acknowledgement datetime
    irn_status xcentro.irn_status_enum NOT NULL DEFAULT 'pending',

    -- QR code and filing status
    qr_code TEXT,                             -- Base64 encoded QR code PNG from IRP
    filing_status xcentro.filing_status_enum NOT NULL DEFAULT 'not_filed',

    -- Invoice line items (convenience fee)
    description TEXT NOT NULL DEFAULT 'Convenience Fee',
    sac_code VARCHAR(10) NOT NULL DEFAULT '997159',
    taxable_value NUMERIC(12,2) NOT NULL,    -- Convenience fee taxable amount
    gst_rate NUMERIC(5,2) NOT NULL DEFAULT 18.0,
    gst_amount NUMERIC(12,2) NOT NULL,
    total_amount NUMERIC(12,2) NOT NULL,

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

COMMENT ON TABLE xcentro.payment_invoices IS 'GST invoices for successful payments with IRN generation';
COMMENT ON COLUMN xcentro.payment_invoices.invoice_number IS 'Invoice reference number format: INV-<invoice_id>';
COMMENT ON COLUMN xcentro.payment_invoices.irn IS 'Invoice Reference Number - SHA256 hash from GSTIN|DocType|DocNumber|FY';
COMMENT ON COLUMN xcentro.payment_invoices.irn_ack_no IS 'Acknowledgement number received from GST IRP portal';
COMMENT ON COLUMN xcentro.payment_invoices.qr_code IS 'Base64 encoded QR code PNG image from IRP';
COMMENT ON COLUMN xcentro.payment_invoices.taxable_value IS 'Taxable amount (before GST)';
COMMENT ON COLUMN xcentro.payment_invoices.gst_amount IS 'GST amount (taxable_value * gst_rate / 100)';
COMMENT ON COLUMN xcentro.payment_invoices.total_amount IS 'Total invoice amount (taxable_value + gst_amount)';

-- Performance indexes for filtering and lookup
CREATE INDEX idx_payment_invoices_payment_id ON xcentro.payment_invoices(payment_id);
CREATE INDEX idx_payment_invoices_invoice_number ON xcentro.payment_invoices(invoice_number);
CREATE INDEX idx_payment_invoices_irn ON xcentro.payment_invoices(irn) WHERE irn IS NOT NULL;
CREATE INDEX idx_payment_invoices_gstin_recipient ON xcentro.payment_invoices(gstin_recipient);
CREATE INDEX idx_payment_invoices_irn_status ON xcentro.payment_invoices(irn_status);
CREATE INDEX idx_payment_invoices_filing_status ON xcentro.payment_invoices(filing_status);
CREATE INDEX idx_payment_invoices_invoice_date ON xcentro.payment_invoices(invoice_date DESC);
CREATE INDEX idx_payment_invoices_created_at ON xcentro.payment_invoices(created_at DESC);

-- Composite indexes for common filtering patterns
CREATE INDEX idx_payment_invoices_date_range ON xcentro.payment_invoices(invoice_date, created_at);
CREATE INDEX idx_payment_invoices_status_combo ON xcentro.payment_invoices(irn_status, filing_status, created_at DESC);

-- Timestamp trigger
CREATE TRIGGER set_timestamp
BEFORE UPDATE ON xcentro.payment_invoices
FOR EACH ROW
EXECUTE PROCEDURE xcentro.trigger_set_timestamp();
