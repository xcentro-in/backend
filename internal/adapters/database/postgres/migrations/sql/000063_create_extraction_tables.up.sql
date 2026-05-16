-- ============================================================
-- Extraction & Matching Engine — full schema
-- Covers: voucher_documents, invoice, purchase_orders,
--         line_items, match_results and all associated indexes.
-- Also: schema changes to support quick vendor creation from
--       document review (no bank account / GSTIN verification),
--       HITL correction audit trail, and template versioning.
--
-- Design notes:
--   - voucher_documents.bid scopes every document to its owning
--     business; file-hash uniqueness is enforced per-business.
--   - All monetary columns on invoice, purchase_orders, and
--     line_items are NUMERIC(15,2) rupees.
--   - document_id FKs use ON DELETE CASCADE so deleting a
--     voucher_document also removes any extracted invoice / PO rows.
--
-- Fully idempotent — safe to re-apply.
-- ============================================================

-- -------------------------------------------------------
-- 1. voucher_documents
-- -------------------------------------------------------
CREATE TABLE IF NOT EXISTS xcentro.voucher_documents (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    bid             UUID REFERENCES xcentro.business(bid),
    s3_path         TEXT NOT NULL,
    doc_type        TEXT,
    status          TEXT NOT NULL DEFAULT 'QUEUED',
    raw_ocr         TEXT,
    raw_extraction  JSONB,
    draft_data      JSONB,
    i2d_template    TEXT,
    processed_by    TEXT,
    validated_data  JSONB,
    file_hash       TEXT,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_voucher_docs_status ON xcentro.voucher_documents(status);

CREATE INDEX IF NOT EXISTS idx_voucher_docs_bid
    ON xcentro.voucher_documents(bid)
    WHERE bid IS NOT NULL;

-- File-hash uniqueness is per-business: two tenants uploading the same file
-- each deserve their own audit record.
CREATE UNIQUE INDEX IF NOT EXISTS idx_voucher_docs_bid_file_hash
    ON xcentro.voucher_documents(bid, file_hash)
    WHERE file_hash IS NOT NULL AND bid IS NOT NULL;

DROP TRIGGER IF EXISTS set_timestamp ON xcentro.voucher_documents;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON xcentro.voucher_documents
    FOR EACH ROW EXECUTE PROCEDURE xcentro.trigger_set_timestamp();

-- -------------------------------------------------------
-- 2. invoice_type_enum
-- -------------------------------------------------------
DO $$ BEGIN
    CREATE TYPE xcentro.invoice_type_enum AS ENUM ('SALES', 'PURCHASE', 'PROFORMA', 'CREDIT_NOTE');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

-- -------------------------------------------------------
-- 3. invoice
-- Amount columns are NUMERIC(15,2) rupees.
-- document_id cascades deletion from voucher_documents.
-- -------------------------------------------------------
CREATE TABLE IF NOT EXISTS xcentro.invoice (
    id               UUID PRIMARY KEY DEFAULT uuidv7(),
    inv_id           TEXT,
    type             xcentro.invoice_type_enum,
    bid              UUID REFERENCES xcentro.business(bid),
    payment_id       UUID REFERENCES xcentro.payments(payment_id),
    payee_id         UUID REFERENCES xcentro.payee_master(payee_id),
    inv_date         DATE,
    inv_due_date     DATE,
    cgst             NUMERIC(15,2),
    igst             NUMERIC(15,2),
    sgst             NUMERIC(15,2),
    total_amount     NUMERIC(15,2),
    subtotal         NUMERIC(15,2),
    gross_amount     NUMERIC(15,2),
    net_amount       NUMERIC(15,2),
    discount         NUMERIC(15,2),
    gst_type         TEXT,
    payment_terms    TEXT,
    expense_category TEXT,
    po_number        TEXT,
    po_id            UUID,
    match_status     TEXT NOT NULL DEFAULT 'PENDING_MATCH',
    currency         VARCHAR(3),
    invoice_url      TEXT,
    comments         TEXT,
    vendor_gstin     TEXT,
    buyer_gstin      TEXT,
    document_id      UUID REFERENCES xcentro.voucher_documents(id) ON DELETE CASCADE,
    created_at       TIMESTAMPTZ DEFAULT NOW(),
    updated_at       TIMESTAMPTZ DEFAULT NOW()
);

-- Ensure amount columns are NUMERIC(15,2) even if the table pre-existed with integer types.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'xcentro' AND table_name = 'invoice'
          AND column_name = 'total_amount' AND data_type != 'numeric'
    ) THEN
        ALTER TABLE xcentro.invoice
            ALTER COLUMN cgst         TYPE NUMERIC(15,2) USING cgst::numeric(15,2),
            ALTER COLUMN igst         TYPE NUMERIC(15,2) USING igst::numeric(15,2),
            ALTER COLUMN sgst         TYPE NUMERIC(15,2) USING sgst::numeric(15,2),
            ALTER COLUMN total_amount TYPE NUMERIC(15,2) USING total_amount::numeric(15,2),
            ALTER COLUMN subtotal     TYPE NUMERIC(15,2) USING subtotal::numeric(15,2),
            ALTER COLUMN gross_amount TYPE NUMERIC(15,2) USING gross_amount::numeric(15,2),
            ALTER COLUMN net_amount   TYPE NUMERIC(15,2) USING net_amount::numeric(15,2),
            ALTER COLUMN discount     TYPE NUMERIC(15,2) USING discount::numeric(15,2);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_invoice_doc_id       ON xcentro.invoice(document_id);
CREATE INDEX IF NOT EXISTS idx_invoice_po_number    ON xcentro.invoice(po_number);
CREATE INDEX IF NOT EXISTS idx_invoice_match_status ON xcentro.invoice(match_status);
CREATE INDEX IF NOT EXISTS idx_invoice_gst_type     ON xcentro.invoice(gst_type) WHERE gst_type IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_invoice_po_id        ON xcentro.invoice(po_id) WHERE po_id IS NOT NULL;

DROP TRIGGER IF EXISTS set_timestamp ON xcentro.invoice;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON xcentro.invoice
    FOR EACH ROW EXECUTE PROCEDURE xcentro.trigger_set_timestamp();

-- -------------------------------------------------------
-- 4. purchase_orders
-- Amount columns are NUMERIC(15,2) rupees.
-- document_id cascades deletion from voucher_documents.
-- -------------------------------------------------------
CREATE TABLE IF NOT EXISTS xcentro.purchase_orders (
    id                UUID PRIMARY KEY DEFAULT uuidv7(),
    document_id       UUID NOT NULL REFERENCES xcentro.voucher_documents(id) ON DELETE CASCADE,
    bid               UUID REFERENCES xcentro.business(bid),
    payee_id          UUID REFERENCES xcentro.payee_master(payee_id),
    po_number         TEXT,
    po_date           TEXT,
    vendor_name       TEXT,
    vendor_gstin      TEXT,
    buyer_gstin       TEXT,
    total_amount      NUMERIC(15,2),
    remaining_balance NUMERIC(15,2),
    discount          NUMERIC(15,2),
    discount_type     TEXT,
    po_status         TEXT NOT NULL DEFAULT 'OPEN',
    created_at        TIMESTAMPTZ DEFAULT NOW(),
    updated_at        TIMESTAMPTZ DEFAULT NOW()
);

-- Ensure amount columns are NUMERIC(15,2) even if the table pre-existed with integer types.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'xcentro' AND table_name = 'purchase_orders'
          AND column_name = 'total_amount' AND data_type != 'numeric'
    ) THEN
        ALTER TABLE xcentro.purchase_orders
            ALTER COLUMN total_amount      TYPE NUMERIC(15,2) USING total_amount::numeric(15,2),
            ALTER COLUMN remaining_balance TYPE NUMERIC(15,2) USING remaining_balance::numeric(15,2),
            ALTER COLUMN discount          TYPE NUMERIC(15,2) USING discount::numeric(15,2);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_purchase_orders_doc_id ON xcentro.purchase_orders(document_id);
CREATE INDEX IF NOT EXISTS idx_po_po_number           ON xcentro.purchase_orders(po_number);
CREATE INDEX IF NOT EXISTS idx_po_po_status           ON xcentro.purchase_orders(po_status);

DROP TRIGGER IF EXISTS set_timestamp ON xcentro.purchase_orders;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON xcentro.purchase_orders
    FOR EACH ROW EXECUTE PROCEDURE xcentro.trigger_set_timestamp();

-- -------------------------------------------------------
-- 5. Add FK from invoice.po_id → purchase_orders.id
-- -------------------------------------------------------
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'fk_invoice_po_id'
    ) THEN
        ALTER TABLE xcentro.invoice
            ADD CONSTRAINT fk_invoice_po_id
            FOREIGN KEY (po_id) REFERENCES xcentro.purchase_orders(id) ON DELETE SET NULL;
    END IF;
END $$;

-- -------------------------------------------------------
-- 6. line_items
-- Amount columns are NUMERIC(15,2) rupees.
-- -------------------------------------------------------
CREATE TABLE IF NOT EXISTS xcentro.line_items (
    id                UUID PRIMARY KEY DEFAULT uuidv7(),
    purchase_order_id UUID REFERENCES xcentro.purchase_orders(id) ON DELETE CASCADE,
    invoice_id        UUID REFERENCES xcentro.invoice(id) ON DELETE CASCADE,
    sku               TEXT,
    description       TEXT,
    quantity          DECIMAL(12,4),
    qty_received      DECIMAL(12,4) NOT NULL DEFAULT 0,
    unit_price        NUMERIC(15,2),
    tax_rate          DECIMAL(5,2),
    tax_amount        NUMERIC(15,2),
    total_amount      NUMERIC(15,2),
    created_at        TIMESTAMPTZ DEFAULT NOW(),
    updated_at        TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT ref_check CHECK (
        (purchase_order_id IS NOT NULL AND invoice_id IS NULL) OR
        (invoice_id IS NOT NULL AND purchase_order_id IS NULL)
    )
);

-- Ensure amount columns are NUMERIC(15,2) even if the table pre-existed with integer types.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'xcentro' AND table_name = 'line_items'
          AND column_name = 'unit_price' AND data_type != 'numeric'
    ) THEN
        ALTER TABLE xcentro.line_items
            ALTER COLUMN unit_price   TYPE NUMERIC(15,2) USING unit_price::numeric(15,2),
            ALTER COLUMN tax_amount   TYPE NUMERIC(15,2) USING tax_amount::numeric(15,2),
            ALTER COLUMN total_amount TYPE NUMERIC(15,2) USING total_amount::numeric(15,2);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_line_items_po_id  ON xcentro.line_items(purchase_order_id);
CREATE INDEX IF NOT EXISTS idx_line_items_inv_id ON xcentro.line_items(invoice_id);
CREATE INDEX IF NOT EXISTS idx_line_items_sku    ON xcentro.line_items(sku);

DROP TRIGGER IF EXISTS set_timestamp ON xcentro.line_items;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON xcentro.line_items
    FOR EACH ROW EXECUTE PROCEDURE xcentro.trigger_set_timestamp();

-- -------------------------------------------------------
-- 7. match_results (audit trail)
-- -------------------------------------------------------
CREATE TABLE IF NOT EXISTS xcentro.match_results (
    id                UUID PRIMARY KEY DEFAULT uuidv7(),
    invoice_id        UUID NOT NULL REFERENCES xcentro.invoice(id) ON DELETE CASCADE,
    purchase_order_id UUID REFERENCES xcentro.purchase_orders(id) ON DELETE SET NULL,
    match_status      TEXT NOT NULL,
    match_details     JSONB,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_match_results_inv ON xcentro.match_results(invoice_id);

-- -------------------------------------------------------
-- 8. Allow quick vendor creation (no GSTIN / bank verification)
-- Idempotent — checks current nullable state before altering.
-- -------------------------------------------------------

-- Make gstin nullable if it isn't already
DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'xcentro'
          AND table_name   = 'payee_vendor'
          AND column_name  = 'gstin'
          AND is_nullable  = 'NO'
    ) THEN
        ALTER TABLE xcentro.payee_vendor ALTER COLUMN gstin DROP NOT NULL;
        ALTER TABLE xcentro.payee_vendor ALTER COLUMN legal_name DROP NOT NULL;
        ALTER TABLE xcentro.payee_vendor ALTER COLUMN gst_status DROP NOT NULL;
    END IF;
END $$;

-- Drop the FK on payment_vouchers that depends on the unique constraint (idempotent)
DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'payment_vouchers_business_id_payee_id_account_id_fkey'
    ) THEN
        ALTER TABLE xcentro.payment_vouchers
            DROP CONSTRAINT payment_vouchers_business_id_payee_id_account_id_fkey;
    END IF;
END $$;

-- Drop composite unique constraint and make account_id nullable if not already done
DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'xcentro'
          AND table_name   = 'business_payee_relation'
          AND column_name  = 'account_id'
          AND is_nullable  = 'NO'
    ) THEN
        ALTER TABLE xcentro.business_payee_relation
            DROP CONSTRAINT IF EXISTS business_payee_relation_business_payee_account_key;
        ALTER TABLE xcentro.business_payee_relation ALTER COLUMN account_id DROP NOT NULL;
    END IF;
END $$;

-- When account_id IS NOT NULL: enforce uniqueness per business-payee-account triple
CREATE UNIQUE INDEX IF NOT EXISTS business_payee_relation_bpa_unique
    ON xcentro.business_payee_relation(business_id, payee_id, account_id)
    WHERE account_id IS NOT NULL;

-- When account_id IS NULL: enforce at most one pending relation per business-payee pair
CREATE UNIQUE INDEX IF NOT EXISTS business_payee_relation_bp_pending_unique
    ON xcentro.business_payee_relation(business_id, payee_id)
    WHERE account_id IS NULL;

-- -------------------------------------------------------
-- 9. templates (invoice2data YAML template storage)
-- -------------------------------------------------------
CREATE TABLE IF NOT EXISTS xcentro.templates (
    id         UUID PRIMARY KEY DEFAULT uuidv7(),
    name       TEXT NOT NULL UNIQUE,   -- matches voucher_documents.i2d_template
    content    TEXT NOT NULL,          -- raw YAML string
    issuer     TEXT,                   -- from YAML issuer field, for reference
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_templates_name ON xcentro.templates(name);

DROP TRIGGER IF EXISTS set_timestamp ON xcentro.templates;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON xcentro.templates
    FOR EACH ROW EXECUTE PROCEDURE xcentro.trigger_set_timestamp();

-- -------------------------------------------------------
-- 10. document_corrections (HITL audit trail)
-- Every field-level correction a human makes during document
-- review is stored here for compliance audits.
-- -------------------------------------------------------
CREATE TABLE IF NOT EXISTS xcentro.document_corrections (
    id           UUID         PRIMARY KEY DEFAULT uuidv7(),
    document_id  UUID         NOT NULL REFERENCES xcentro.voucher_documents(id) ON DELETE CASCADE,
    user_id      TEXT         NOT NULL,
    corrections  JSONB        NOT NULL,   -- [{field, old_value, new_value}, ...]
    corrected_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_doc_corrections_doc_id
    ON xcentro.document_corrections(document_id);

CREATE INDEX IF NOT EXISTS idx_doc_corrections_user_id
    ON xcentro.document_corrections(user_id);

-- -------------------------------------------------------
-- 11. templates_history (template versioning)
-- Every time a template is overwritten the previous YAML is
-- preserved here for auditing, diffing, or rollback.
-- -------------------------------------------------------
CREATE TABLE IF NOT EXISTS xcentro.templates_history (
    id           UUID         PRIMARY KEY DEFAULT uuidv7(),
    name         TEXT         NOT NULL,         -- matches xcentro.templates.name
    content      TEXT         NOT NULL,         -- the YAML at the time it was replaced
    issuer       TEXT,
    replaced_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    replaced_by  TEXT                            -- short tag e.g. "hitl_regen", "manual_edit"
);

CREATE INDEX IF NOT EXISTS idx_templates_history_name
    ON xcentro.templates_history(name, replaced_at DESC);
