-- #############################################################################
-- # Payment Voucher System
-- # Phase 1: Create voucher tables for accounting and ERP integration
-- #############################################################################

-- Payment Vouchers Table
-- Serves as the accounting document linking payments to ERP systems
-- Vouchers are created in DRAFT state when user transitions from payment details to payment source selection
-- Finalized after payment success
CREATE TABLE xcentro.payment_vouchers (
    -- Primary Key
    voucher_id UUID PRIMARY KEY DEFAULT uuidv7(),

    -- Voucher Identification
    voucher_number VARCHAR(50) NOT NULL UNIQUE,  -- Format: PV-2024-00001
    voucher_date DATE NOT NULL DEFAULT CURRENT_DATE,
    fiscal_year VARCHAR(10) NOT NULL,  -- FY2024-25
    accounting_period VARCHAR(7) NOT NULL,  -- 2024-04 (Apr 2024)
    voucher_type VARCHAR(50) NOT NULL DEFAULT 'payment', -- identify other voucher types

    -- Business Context (Owner of this voucher)
    business_id UUID NOT NULL REFERENCES xcentro.business(bid) ON DELETE CASCADE,

    -- Payee Reference (Composite FK to business_payee_relation)
    -- This ensures the payee-account combination is valid for this business
    payee_id UUID NOT NULL,
    account_id UUID NOT NULL,
    FOREIGN KEY (business_id, payee_id, account_id)
        REFERENCES xcentro.business_payee_relation(business_id, payee_id, account_id)
        ON DELETE RESTRICT,

    -- Invoice Reference (Mandatory field, manually entered or from OCR)
    invoice_number VARCHAR(100),
    invoice_date DATE,
    invoice_due_date DATE,

    -- Core Amounts (All amounts in paise for INR)
    gross_amount BIGINT NOT NULL,  -- Total amount before deductions
    net_amount BIGINT NOT NULL,    -- Amount actually paid (after TDS/discounts)
    currency VARCHAR(3) NOT NULL DEFAULT 'INR',

    -- Tax Details (First-class columns for ERP worker queries)
    gst_amount BIGINT DEFAULT 0,
    gst_rate DECIMAL(5,2),  -- e.g., 18.00
    cgst_amount BIGINT DEFAULT 0,
    sgst_amount BIGINT DEFAULT 0,
    igst_amount BIGINT DEFAULT 0,

    -- TDS Details
    tds_amount BIGINT DEFAULT 0,
    tds_rate DECIMAL(5,2),
    tds_section VARCHAR(20),  -- e.g., 194C, 194J

    -- Other Charges
    fee_amount BIGINT DEFAULT 0,  -- Payment gateway fees
    discount_amount BIGINT DEFAULT 0,

    -- Additional Financial Details (JSONB for flexibility)
    -- Structure: {
    --   "line_items": [{"description": "...", "hsn_code": "...", "amount": ...}],
    --   "tax_breakdown": {...},
    --   "custom_fields": {...}
    -- }
    financial_details JSONB,

    -- Attachments (JSONB)
    -- Structure: {
    --   "invoice_pdf": {"url": "...", "uploaded_at": "..."},
    --   "supporting_docs": [...]
    -- }
    attachments JSONB,

    -- Classification
    expense_category VARCHAR(100),  -- Travel, Office Supplies, Professional Fees, etc.
    description TEXT,

    -- Status (Draft-first approach)
    -- Flow: draft → finalized → posted → synced
    status VARCHAR(50) NOT NULL DEFAULT 'draft',

    -- Payment Link (NULL when voucher is in draft)
    payment_id UUID REFERENCES xcentro.payments(payment_id) ON DELETE RESTRICT,

    -- ERP Integration (Critical first-class fields for worker queries)
    erp_sync_status VARCHAR(50) NOT NULL DEFAULT 'not_synced',
    -- not_synced → pending → synced → failed
    erp_reference_id VARCHAR(255),  -- ERP system's voucher ID
    erp_posting_date DATE,
    erp_last_sync_at TIMESTAMPTZ,
    erp_sync_attempts INT DEFAULT 0,

    -- General Ledger Codes
    gl_expense_account VARCHAR(50),  -- e.g., 5000 (Expense Account)
    gl_bank_account VARCHAR(50),     -- e.g., 1020 (Bank Account)
    cost_center VARCHAR(50),         -- For departmental accounting

    -- ERP Metadata (JSONB for vendor-specific fields)
    -- Structure: {
    --   "zoho_books": {"contact_id": "...", "tax_id": "..."},
    --   "tally": {"ledger_name": "...", "voucher_class": "..."}
    -- }
    erp_metadata JSONB,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by_user_id UUID REFERENCES xcentro.user_table(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ,

    -- Additional Metadata
    metadata JSONB
);

-- Voucher Approvals Table
-- Separate table to support multi-level approval workflows
-- Approval happens at payment level, not voucher level (RBAC enforcement)
CREATE TABLE xcentro.voucher_approvals (
    approval_id UUID PRIMARY KEY DEFAULT uuidv7(),
    voucher_id UUID NOT NULL REFERENCES xcentro.payment_vouchers(voucher_id) ON DELETE CASCADE,

    -- Approval Hierarchy
    approval_level INT NOT NULL,  -- 1, 2, 3 for multi-level approvals
    approver_user_id UUID NOT NULL REFERENCES xcentro.user_table(id) ON DELETE CASCADE,
    approver_role VARCHAR(50),  -- Manager, Finance Head, etc.

    -- Action Details
    action VARCHAR(50) NOT NULL DEFAULT 'pending',  -- pending, approved, rejected
    action_timestamp TIMESTAMPTZ,
    remarks TEXT,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Composite unique constraint (one approval per level per voucher)
    UNIQUE (voucher_id, approval_level)
);

-- ERP Sync Queue Table
-- Async queue for background workers to process ERP synchronization
CREATE TABLE xcentro.erp_sync_queue (
    queue_id UUID PRIMARY KEY DEFAULT uuidv7(),
    voucher_id UUID NOT NULL REFERENCES xcentro.payment_vouchers(voucher_id) ON DELETE CASCADE,

    -- Sync Direction (Initially one-way push, later two-way)
    sync_direction VARCHAR(20) NOT NULL,  -- push, pull

    -- Queue Status
    queue_status VARCHAR(50) NOT NULL DEFAULT 'pending',
    -- pending → processing → completed → failed

    -- Retry Logic
    retry_count INT DEFAULT 0,
    max_retries INT DEFAULT 3,
    next_retry_at TIMESTAMPTZ,

    -- Request/Response Payloads
    request_payload JSONB,
    response_payload JSONB,

    -- Error Details
    error_code VARCHAR(100),
    error_message TEXT,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,

    -- Index for worker queries
    CHECK (retry_count <= max_retries)
);

-- ============================================================================
-- Indexes for Performance
-- ============================================================================

-- Voucher lookups by business
-- CREATE INDEX idx_vouchers_business_id ON xcentro.payment_vouchers(business_id);

-- Voucher number uniqueness and lookup
-- CREATE INDEX idx_vouchers_number ON xcentro.payment_vouchers(voucher_number);

-- ERP worker queries (critical for background jobs)
-- CREATE INDEX idx_vouchers_erp_sync_status ON xcentro.payment_vouchers(erp_sync_status)
--    WHERE erp_sync_status IN ('not_synced', 'pending', 'failed');

-- Status filtering
-- CREATE INDEX idx_vouchers_status ON xcentro.payment_vouchers(status);

-- Date-based queries for accounting periods
-- CREATE INDEX idx_vouchers_date_range ON xcentro.payment_vouchers(business_id, voucher_date DESC);

-- Fiscal year and accounting period queries
-- CREATE INDEX idx_vouchers_fiscal_period ON xcentro.payment_vouchers(fiscal_year, accounting_period);

-- Payment reference lookup
-- CREATE INDEX idx_vouchers_payment_id ON xcentro.payment_vouchers(payment_id)
--    WHERE payment_id IS NOT NULL;

-- Payee-based voucher queries
-- CREATE INDEX idx_vouchers_payee ON xcentro.payment_vouchers(business_id, payee_id, account_id);

-- Approval workflow queries
-- CREATE INDEX idx_approvals_voucher ON xcentro.voucher_approvals(voucher_id);
-- CREATE INDEX idx_approvals_approver ON xcentro.voucher_approvals(approver_user_id, action);

-- ERP sync queue worker queries
-- CREATE INDEX idx_sync_queue_status ON xcentro.erp_sync_queue(queue_status, next_retry_at)
--    WHERE queue_status IN ('pending', 'failed');

-- CREATE INDEX idx_sync_queue_voucher ON xcentro.erp_sync_queue(voucher_id);

-- ============================================================================
-- Comments for Documentation
-- ============================================================================

COMMENT ON TABLE xcentro.payment_vouchers IS 'Payment vouchers serve as the accounting document linking payments to ERP systems. Created in DRAFT state during payment flow, finalized after payment success.';

COMMENT ON COLUMN xcentro.payment_vouchers.voucher_number IS 'Unique voucher identifier in format PV-YYYY-NNNNN. Auto-generated based on fiscal year sequence.';

COMMENT ON COLUMN xcentro.payment_vouchers.status IS 'Voucher lifecycle: draft (created on Page 1→2 transition) → finalized (after payment) → posted (accounting complete) → synced (ERP synchronized)';

COMMENT ON COLUMN xcentro.payment_vouchers.erp_sync_status IS 'ERP synchronization state: not_synced → pending (in queue) → synced (success) → failed (retry needed)';

COMMENT ON COLUMN xcentro.payment_vouchers.financial_details IS 'JSONB field for flexible financial data: line items, HSN codes, tax breakdowns, custom fields';

COMMENT ON COLUMN xcentro.payment_vouchers.erp_metadata IS 'Vendor-specific ERP fields (Zoho Books, Tally, SAP, etc.) stored as JSONB for flexibility';

COMMENT ON TABLE xcentro.voucher_approvals IS 'Multi-level approval workflow for vouchers. Approval happens at payment level via RBAC before payment execution.';

COMMENT ON TABLE xcentro.erp_sync_queue IS 'Async queue for ERP synchronization workers. Supports retry logic with exponential backoff.';
