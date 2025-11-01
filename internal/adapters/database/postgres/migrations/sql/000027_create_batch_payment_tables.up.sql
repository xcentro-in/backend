-- Create batch_payments table for tracking CSV upload jobs
CREATE TABLE IF NOT EXISTS xcentro.batch_payments (
    batch_id UUID PRIMARY KEY DEFAULT uuidv7(),
    business_id UUID NOT NULL REFERENCES xcentro.business(bid) ON DELETE CASCADE,
    uploaded_by UUID NOT NULL REFERENCES xcentro.user_table(id) ON DELETE RESTRICT,
    file_name VARCHAR(255), -- Optional: for audit trail
    status VARCHAR(20) NOT NULL CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'cancelled')),

    -- Row tracking
    total_rows INT NOT NULL DEFAULT 0,
    total_amount BIGINT NOT NULL DEFAULT 0, -- Total CSV amount in paise (informational only)
    processed_rows INT NOT NULL DEFAULT 0,
    success_rows INT NOT NULL DEFAULT 0,
    failed_rows INT NOT NULL DEFAULT 0,

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE
);

-- Create batch_payment_items table
CREATE TABLE IF NOT EXISTS xcentro.batch_payment_items (
    item_id UUID PRIMARY KEY DEFAULT uuidv7(),
    batch_id UUID NOT NULL REFERENCES xcentro.batch_payments(batch_id) ON DELETE CASCADE,
    row_number INT NOT NULL,
    status VARCHAR(20) NOT NULL CHECK (status IN ('matched', 'ambiguous', 'not_found', 'pending', 'processing', 'success', 'failed', 'skipped')),

    -- CSV data (as provided by user)
    csv_payee_name VARCHAR(255) NOT NULL,
    csv_account_last_4 VARCHAR(4),
    csv_amount BIGINT NOT NULL, -- In paise
    payment_context VARCHAR(50) NOT NULL,
    description TEXT,
    expense_category VARCHAR(100),
    gst_amount BIGINT DEFAULT 0,
    tds_amount BIGINT DEFAULT 0,
    invoice_number VARCHAR(100),

    -- Matching result
    matched_relation_id UUID, -- FK to business_payee_relation (stored as string in table)
    match_confidence DECIMAL(3,2), -- 0.00 to 1.00

    -- Voucher creation result
    voucher_id UUID, -- FK to vouchers (created draft voucher)
    error_message TEXT,

    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMP WITH TIME ZONE,

    UNIQUE(batch_id, row_number)
);

-- Create indexes for performance
CREATE INDEX idx_batch_payments_business_id ON xcentro.batch_payments(business_id);
CREATE INDEX idx_batch_payments_status ON xcentro.batch_payments(status);
CREATE INDEX idx_batch_payments_created_at ON xcentro.batch_payments(created_at);

CREATE INDEX idx_batch_payment_items_batch_id ON xcentro.batch_payment_items(batch_id);
CREATE INDEX idx_batch_payment_items_status ON xcentro.batch_payment_items(status);
CREATE INDEX idx_batch_payment_items_voucher_id ON xcentro.batch_payment_items(voucher_id);
