-- Create Voucher Documents Table
CREATE TABLE IF NOT EXISTS xcentro.voucher_documents (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    s3_path TEXT NOT NULL,
    doc_type TEXT,
    status TEXT NOT NULL DEFAULT 'QUEUED',
    raw_ocr TEXT,
    draft_data JSONB,
    i2d_template TEXT,
    processed_by TEXT,
    validated_data JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Create Invoice Type Enum if it doesn't exist
DO $$ BEGIN
    CREATE TYPE xcentro.invoice_type_enum AS ENUM ('SALES', 'PURCHASE', 'PROFORMA', 'CREDIT_NOTE');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

-- Create Invoices Table (matching schema.sql definition)

CREATE TABLE IF NOT EXISTS xcentro.invoice (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    inv_id TEXT,
    type xcentro.invoice_type_enum,
    bid UUID REFERENCES xcentro.business(bid),
    payment_id UUID REFERENCES xcentro.payments(payment_id),
    payee_id UUID REFERENCES xcentro.payee_master(payee_id),
    inv_date DATE,
    inv_due_date DATE,
    cgst DECIMAL(10,2),
    igst DECIMAL(10,2),
    sgst DECIMAL(10,2),
    total_amount DECIMAL(20,2),
    currency VARCHAR(3),
    invoice_url TEXT,
    comments TEXT,
    vendor_gstin TEXT,
    buyer_gstin TEXT,
    document_id UUID REFERENCES xcentro.voucher_documents(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Create Purchase Orders Table
CREATE TABLE IF NOT EXISTS xcentro.purchase_orders (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    document_id UUID NOT NULL REFERENCES xcentro.voucher_documents(id),
    bid UUID REFERENCES xcentro.business(bid),
    payee_id UUID REFERENCES xcentro.payee_master(payee_id),
    po_number TEXT,
    po_date TEXT,
    vendor_name TEXT,
    vendor_gstin TEXT,
    buyer_gstin TEXT,
    total_amount DECIMAL(20,2),
    discount DECIMAL(20,2),
    discount_type TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Create Line Items Table
CREATE TABLE IF NOT EXISTS xcentro.line_items (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    purchase_order_id UUID REFERENCES xcentro.purchase_orders(id) ON DELETE CASCADE,
    invoice_id UUID REFERENCES xcentro.invoice(id) ON DELETE CASCADE,
    description TEXT,
    quantity DECIMAL(12,4),
    unit_price BIGINT, -- in paise
    tax_rate DECIMAL(5,2),
    tax_amount BIGINT, -- in paise
    total_amount BIGINT, -- in paise
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT ref_check CHECK (
        (purchase_order_id IS NOT NULL AND invoice_id IS NULL) OR
        (invoice_id IS NOT NULL AND purchase_order_id IS NULL)
    )
);

-- Triggers for automatic timestamps
CREATE TRIGGER set_timestamp BEFORE UPDATE ON xcentro.voucher_documents FOR EACH ROW EXECUTE PROCEDURE xcentro.trigger_set_timestamp();
CREATE TRIGGER set_timestamp BEFORE UPDATE ON xcentro.invoice FOR EACH ROW EXECUTE PROCEDURE xcentro.trigger_set_timestamp();
CREATE TRIGGER set_timestamp BEFORE UPDATE ON xcentro.purchase_orders FOR EACH ROW EXECUTE PROCEDURE xcentro.trigger_set_timestamp();
CREATE TRIGGER set_timestamp BEFORE UPDATE ON xcentro.line_items FOR EACH ROW EXECUTE PROCEDURE xcentro.trigger_set_timestamp();

-- Indexes
CREATE INDEX idx_voucher_docs_status ON xcentro.voucher_documents(status);
CREATE INDEX idx_purchase_orders_doc_id ON xcentro.purchase_orders(document_id);
CREATE INDEX idx_invoice_doc_id ON xcentro.invoice(document_id);
CREATE INDEX idx_line_items_po_id ON xcentro.line_items(purchase_order_id);
CREATE INDEX idx_line_items_inv_id ON xcentro.line_items(invoice_id);
