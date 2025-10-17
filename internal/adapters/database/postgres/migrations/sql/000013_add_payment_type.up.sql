-- Add payment_type enum column to payments table
-- This distinguishes between regular payments, tokenization payments, and refunds

-- Create payment_type enum type
CREATE TYPE xcentro.payment_type AS ENUM ('payment', 'tokenization', 'refund');

-- Add payment_type column to payments table
ALTER TABLE xcentro.payments
ADD COLUMN payment_type xcentro.payment_type NOT NULL DEFAULT 'payment';

-- Add index for querying by payment type
CREATE INDEX idx_payments_payment_type ON xcentro.payments(payment_type);

-- Add comment for documentation
COMMENT ON COLUMN xcentro.payments.payment_type IS 'Type of payment: payment (regular), tokenization (₹1 card tokenization), refund (refund transaction)';
