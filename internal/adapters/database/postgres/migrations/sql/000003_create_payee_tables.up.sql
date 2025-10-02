-- #############################################################################
-- # Polymorphic Payee Management System Database Schema
-- # Phase 5.X: Support for 4 payee types (Vendor, GST, Biller, Employee)
-- #############################################################################

-- #############################################################################
-- # Custom ENUM Types
-- #############################################################################

-- Payee type enum (polymorphic discrimination)
CREATE TYPE xcentro.payee_type_enum AS ENUM (
    'vendor',
    'gst_payment',
    'biller',
    'employee'
);

-- Verification status for GST, bank, PAN verification
CREATE TYPE xcentro.verification_status_enum AS ENUM (
    'pending',
    'in_progress',
    'success',
    'failed'
);

-- #############################################################################
-- # Payee Master Table (Routing/Discrimination Table)
-- #############################################################################
-- Thin master table for polymorphic routing
-- All payees start here, then route to type-specific tables

CREATE TABLE xcentro.payee_master (
    payee_id UUID PRIMARY KEY DEFAULT uuidv7(),
    payee_type xcentro.payee_type_enum NOT NULL,

    -- Minimal shared fields
    display_name VARCHAR(500) NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,

    -- Audit
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ
);

-- Indexes
CREATE INDEX idx_payee_master_type ON xcentro.payee_master(payee_type);
CREATE INDEX idx_payee_master_active ON xcentro.payee_master(is_active) WHERE is_active = TRUE;

-- Comments
COMMENT ON TABLE xcentro.payee_master IS 'Master routing table for all payee types (vendor, GST, biller, employee)';
COMMENT ON COLUMN xcentro.payee_master.payee_type IS 'Discriminator for routing to type-specific tables';

-- Timestamp trigger
CREATE TRIGGER set_timestamp_payee_master
BEFORE UPDATE ON xcentro.payee_master
FOR EACH ROW
EXECUTE PROCEDURE xcentro.trigger_set_timestamp();


-- #############################################################################
-- # Payee Vendor Table (Business with GSTIN)
-- #############################################################################
-- Vendors are businesses identified by GSTIN
-- One entry per GSTIN globally (shared across all customers)

CREATE TABLE xcentro.payee_vendor (
    payee_id UUID PRIMARY KEY REFERENCES xcentro.payee_master(payee_id) ON DELETE CASCADE,
    gstin VARCHAR(15) UNIQUE NOT NULL,

    -- Business details from GST verification
    legal_name VARCHAR(500) NOT NULL,
    business_type VARCHAR(100),
    registration_date DATE,
    gst_status VARCHAR(50) NOT NULL,

    -- Primary address from GST
    building_name VARCHAR(255),
    street VARCHAR(255),
    city VARCHAR(100),
    state VARCHAR(100),
    pincode VARCHAR(10),

    -- GST verification
    gst_verification_status xcentro.verification_status_enum DEFAULT 'pending',
    gst_verified_at TIMESTAMPTZ,
    gst_data JSONB,

    -- Audit
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ,
    metadata JSONB
);

-- Indexes
CREATE INDEX idx_payee_vendor_gstin ON xcentro.payee_vendor(gstin);
CREATE INDEX idx_payee_vendor_legal_name ON xcentro.payee_vendor(legal_name);
CREATE INDEX idx_payee_vendor_gst_status ON xcentro.payee_vendor(gst_status);

-- Comments
COMMENT ON TABLE xcentro.payee_vendor IS 'Vendor payees identified by GSTIN (businesses we pay)';
COMMENT ON COLUMN xcentro.payee_vendor.gstin IS 'Unique 15-character GST identification number';
COMMENT ON COLUMN xcentro.payee_vendor.gst_data IS 'Full GST verification API response including directors';

-- Timestamp trigger
CREATE TRIGGER set_timestamp_payee_vendor
BEFORE UPDATE ON xcentro.payee_vendor
FOR EACH ROW
EXECUTE PROCEDURE xcentro.trigger_set_timestamp();


-- #############################################################################
-- # Payee GST Table (Government - Singleton)
-- #############################################################################
-- GST payments are made to ONE entity: Government of India
-- Only one row will exist in this table

CREATE TABLE xcentro.payee_gst (
    payee_id UUID PRIMARY KEY REFERENCES xcentro.payee_master(payee_id) ON DELETE CASCADE,

    government_entity_name VARCHAR(255) NOT NULL DEFAULT 'Government of India - GST',
    gst_portal_url VARCHAR(500) DEFAULT 'https://www.gst.gov.in',

    -- Audit
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ,
    metadata JSONB
);

-- Comments
COMMENT ON TABLE xcentro.payee_gst IS 'Government GST payment entity (singleton - only one row)';
COMMENT ON COLUMN xcentro.payee_gst.government_entity_name IS 'Fixed entity: Government of India';

-- Timestamp trigger
CREATE TRIGGER set_timestamp_payee_gst
BEFORE UPDATE ON xcentro.payee_gst
FOR EACH ROW
EXECUTE PROCEDURE xcentro.trigger_set_timestamp();


-- #############################################################################
-- # Payee GST Virtual Accounts (Virtual Bank Accounts)
-- #############################################################################
-- Each tax period generates a new virtual ID (virtual bank account)
-- These are temporary bank account proxies for specific tax liabilities

CREATE TABLE xcentro.payee_gst_virtual_accounts (
    virtual_account_id UUID PRIMARY KEY DEFAULT uuidv7(),
    payee_id UUID NOT NULL REFERENCES xcentro.payee_master(payee_id) ON DELETE CASCADE,

    -- Virtual bank account details (from government GST portal)
    virtual_account_number VARCHAR(50) UNIQUE NOT NULL,
    ifsc_code VARCHAR(11) NOT NULL,
    bank_name VARCHAR(255) DEFAULT 'Government GST Collection Account',

    -- Tax period tracking
    tax_period VARCHAR(20) NOT NULL,
    financial_year VARCHAR(10) NOT NULL,
    tax_type VARCHAR(50),

    -- Validity (virtual accounts expire after payment window)
    valid_from TIMESTAMPTZ NOT NULL,
    valid_until TIMESTAMPTZ NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,

    -- Document containing this virtual ID
    challan_document_url TEXT,
    uploaded_at TIMESTAMPTZ,

    -- Audit
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ,
    metadata JSONB
);

-- Indexes for analytics and querying
CREATE INDEX idx_gst_virtual_payee ON xcentro.payee_gst_virtual_accounts(payee_id);
CREATE INDEX idx_gst_virtual_tax_period ON xcentro.payee_gst_virtual_accounts(tax_period);
CREATE INDEX idx_gst_virtual_financial_year ON xcentro.payee_gst_virtual_accounts(financial_year);
CREATE INDEX idx_gst_virtual_active ON xcentro.payee_gst_virtual_accounts(payee_id, is_active) WHERE is_active = TRUE;

-- Comments
COMMENT ON TABLE xcentro.payee_gst_virtual_accounts IS 'Virtual bank accounts for GST payments (generated monthly/quarterly)';
COMMENT ON COLUMN xcentro.payee_gst_virtual_accounts.virtual_account_number IS 'Temporary virtual account number from GST portal';
COMMENT ON COLUMN xcentro.payee_gst_virtual_accounts.tax_period IS 'Tax period (e.g., Jan-2024, Q1-2024)';
COMMENT ON COLUMN xcentro.payee_gst_virtual_accounts.challan_document_url IS 'S3 URL of tax challan document containing virtual ID';

-- Timestamp trigger
CREATE TRIGGER set_timestamp_payee_gst_virtual_accounts
BEFORE UPDATE ON xcentro.payee_gst_virtual_accounts
FOR EACH ROW
EXECUTE PROCEDURE xcentro.trigger_set_timestamp();


-- #############################################################################
-- # Payee Biller Table (BBPS System Billers)
-- #############################################################################
-- Billers are entities in BBPS registry (electricity, telecom, water, etc.)
-- Payments made via NPCI, no direct bank account

CREATE TABLE xcentro.payee_biller (
    payee_id UUID PRIMARY KEY REFERENCES xcentro.payee_master(payee_id) ON DELETE CASCADE,

    -- Biller identity (from BBPS registry)
    biller_id VARCHAR(100) UNIQUE NOT NULL,
    biller_name VARCHAR(500) NOT NULL,
    biller_category VARCHAR(100) NOT NULL,
    biller_description TEXT,

    -- BBPS/NPCI details
    bbps_biller_code VARCHAR(50),
    npci_registered BOOLEAN DEFAULT TRUE,
    supports_adhoc BOOLEAN DEFAULT TRUE,
    supports_recurring BOOLEAN DEFAULT FALSE,

    -- Payment processing
    payment_modes TEXT[],
    avg_processing_time_hours INT,

    -- Audit
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ,
    metadata JSONB
);

-- Indexes
CREATE INDEX idx_payee_biller_biller_id ON xcentro.payee_biller(biller_id);
CREATE INDEX idx_payee_biller_category ON xcentro.payee_biller(biller_category);

-- Comments
COMMENT ON TABLE xcentro.payee_biller IS 'BBPS system billers (electricity, telecom, water, etc.)';
COMMENT ON COLUMN xcentro.payee_biller.biller_id IS 'Unique biller ID from BBPS registry';
COMMENT ON COLUMN xcentro.payee_biller.biller_category IS 'Category: Electricity, Telecom, Water, Gas, etc.';

-- Timestamp trigger
CREATE TRIGGER set_timestamp_payee_biller
BEFORE UPDATE ON xcentro.payee_biller
FOR EACH ROW
EXECUTE PROCEDURE xcentro.trigger_set_timestamp();


-- #############################################################################
-- # Payee Biller Accounts (Customer-Biller Configuration)
-- #############################################################################
-- Each business can have multiple accounts with same biller
-- Stores customer ID, mobile number, and BBPS parameters

CREATE TABLE xcentro.payee_biller_accounts (
    biller_account_id UUID PRIMARY KEY DEFAULT uuidv7(),
    payee_id UUID NOT NULL REFERENCES xcentro.payee_master(payee_id) ON DELETE CASCADE,
    business_id UUID NOT NULL REFERENCES xcentro.business(bid) ON DELETE CASCADE,

    -- Customer identification at biller (for BBPS API)
    mobile_number VARCHAR(20) NOT NULL,
    customer_params JSONB NOT NULL,

    -- Bill fetching configuration
    auto_fetch_enabled BOOLEAN DEFAULT FALSE,
    fetch_schedule VARCHAR(50),
    preferred_fetch_day INT,

    -- Cached bill information (last fetched)
    last_bill_fetched_at TIMESTAMPTZ,
    last_bill_amount BIGINT,
    last_bill_due_date DATE,

    -- Status
    is_active BOOLEAN DEFAULT TRUE,

    -- Audit
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ,
    metadata JSONB,

    -- Constraints
    CONSTRAINT unique_business_biller UNIQUE (business_id, payee_id, mobile_number)
);

-- Indexes
CREATE INDEX idx_payee_biller_accounts_payee ON xcentro.payee_biller_accounts(payee_id);
CREATE INDEX idx_payee_biller_accounts_business ON xcentro.payee_biller_accounts(business_id);
CREATE INDEX idx_payee_biller_accounts_auto_fetch ON xcentro.payee_biller_accounts(auto_fetch_enabled) WHERE auto_fetch_enabled = TRUE;

-- Comments
COMMENT ON TABLE xcentro.payee_biller_accounts IS 'Customer-biller configurations (pseudo-account for BBPS payments)';
COMMENT ON COLUMN xcentro.payee_biller_accounts.customer_params IS 'JSONB array of biller-specific parameters (e.g., [{"key": "CA_Number", "value": "123"}])';
COMMENT ON COLUMN xcentro.payee_biller_accounts.mobile_number IS 'Registered mobile number at biller';

-- Timestamp trigger
CREATE TRIGGER set_timestamp_payee_biller_accounts
BEFORE UPDATE ON xcentro.payee_biller_accounts
FOR EACH ROW
EXECUTE PROCEDURE xcentro.trigger_set_timestamp();


-- #############################################################################
-- # Payee Employee Table (Payroll Employees)
-- #############################################################################
-- Employees are individuals (not businesses) paid via payroll
-- Identified by PAN instead of GSTIN

CREATE TABLE xcentro.payee_employee (
    payee_id UUID PRIMARY KEY REFERENCES xcentro.payee_master(payee_id) ON DELETE CASCADE,

    -- Employee identity
    employee_code VARCHAR(100),
    full_name VARCHAR(500) NOT NULL,
    pan VARCHAR(10) UNIQUE NOT NULL,
    email VARCHAR(254),
    phone VARCHAR(20),

    -- Employment details
    designation VARCHAR(200),
    department VARCHAR(200),
    date_of_joining DATE,
    employment_type VARCHAR(50),

    -- Remuneration document (S3 URL, versioned at S3 level)
    remuneration_doc_url TEXT,
    remuneration_doc_uploaded_at TIMESTAMPTZ,

    -- Sensitivity flag (always true for employees)
    is_sensitive BOOLEAN DEFAULT TRUE,

    -- PAN verification
    pan_verification_status xcentro.verification_status_enum DEFAULT 'pending',
    pan_verified_at TIMESTAMPTZ,

    -- Audit
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ,
    metadata JSONB
);

-- Indexes
CREATE INDEX idx_payee_employee_pan ON xcentro.payee_employee(pan);
CREATE INDEX idx_payee_employee_full_name ON xcentro.payee_employee(full_name);

-- Comments
COMMENT ON TABLE xcentro.payee_employee IS 'Employee payees for payroll (individuals, not businesses)';
COMMENT ON COLUMN xcentro.payee_employee.pan IS 'Permanent Account Number (individual tax ID)';
COMMENT ON COLUMN xcentro.payee_employee.remuneration_doc_url IS 'S3 URL of remuneration document (versioned at S3 level)';
COMMENT ON COLUMN xcentro.payee_employee.is_sensitive IS 'Always true - employee data is sensitive';

-- Timestamp trigger
CREATE TRIGGER set_timestamp_payee_employee
BEFORE UPDATE ON xcentro.payee_employee
FOR EACH ROW
EXECUTE PROCEDURE xcentro.trigger_set_timestamp();


-- #############################################################################
-- # Payee Bank Accounts (Shared by Vendor + Employee)
-- #############################################################################
-- Bank accounts for payees (vendors and employees)
-- GST and Biller types do NOT have bank accounts (use virtual/BBPS)

CREATE TABLE xcentro.payee_bank_accounts (
    account_id UUID PRIMARY KEY DEFAULT uuidv7(),
    payee_id UUID NOT NULL REFERENCES xcentro.payee_master(payee_id) ON DELETE CASCADE,

    -- Bank details (encrypted at rest - Phase 5.5)
    account_number_encrypted TEXT NOT NULL,
    account_number_hash VARCHAR(64) NOT NULL,
    ifsc_code VARCHAR(11) NOT NULL,
    bank_name VARCHAR(255),
    branch_name VARCHAR(255),
    account_type xcentro.bank_account_type_enum NOT NULL,
    account_holder_name VARCHAR(255) NOT NULL,

    -- Bank verification (penny drop)
    is_verified BOOLEAN DEFAULT FALSE,
    verified_at TIMESTAMPTZ,
    bank_verification_status xcentro.verification_status_enum DEFAULT 'pending',
    bank_verification_data JSONB,

    -- Cross-verification (GST/PAN name vs Bank name)
    cross_verification_status xcentro.verification_status_enum DEFAULT 'pending',
    name_match_score NUMERIC(3,2),

    -- Account status
    is_active BOOLEAN DEFAULT TRUE,
    is_primary BOOLEAN DEFAULT FALSE,
    account_label VARCHAR(100),

    -- Audit
    added_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ,
    metadata JSONB,

    -- Constraints
    CONSTRAINT unique_payee_account UNIQUE (payee_id, account_number_hash)
);

-- Indexes
CREATE INDEX idx_payee_bank_accounts_payee_id ON xcentro.payee_bank_accounts(payee_id);
CREATE INDEX idx_payee_bank_accounts_is_primary ON xcentro.payee_bank_accounts(payee_id, is_primary) WHERE is_primary = TRUE;
CREATE INDEX idx_payee_bank_accounts_active ON xcentro.payee_bank_accounts(payee_id, is_active) WHERE is_active = TRUE;

-- Comments
COMMENT ON TABLE xcentro.payee_bank_accounts IS 'Bank accounts for vendor and employee payees (NOT for GST/Biller)';
COMMENT ON COLUMN xcentro.payee_bank_accounts.account_number_hash IS 'SHA-256 hash for duplicate detection per payee';
COMMENT ON COLUMN xcentro.payee_bank_accounts.account_number_encrypted IS 'Encrypted account number (plaintext in Phase 5.3, encrypted in Phase 5.5)';
COMMENT ON COLUMN xcentro.payee_bank_accounts.is_primary IS 'Only one primary account per payee';

-- Timestamp trigger
CREATE TRIGGER set_timestamp_payee_bank_accounts
BEFORE UPDATE ON xcentro.payee_bank_accounts
FOR EACH ROW
EXECUTE PROCEDURE xcentro.trigger_set_timestamp();


-- #############################################################################
-- # Business-Payee Relation Table (Polymorphic Many-to-Many)
-- #############################################################################
-- Maps customers to payees and their accounts (bank/virtual/biller)
-- Stores customer-specific metadata (display name, contacts, notes)

CREATE TABLE xcentro.business_payee_relation (
    business_id UUID NOT NULL REFERENCES xcentro.business(bid) ON DELETE CASCADE,
    payee_id UUID NOT NULL REFERENCES xcentro.payee_master(payee_id) ON DELETE CASCADE,
    account_id UUID NOT NULL,

    -- Customer-specific metadata (how THIS customer sees the payee)
    display_name VARCHAR(255) NOT NULL,
    contact_email VARCHAR(254),
    contact_phone VARCHAR(20),
    notes TEXT,

    -- Relationship metadata
    is_active BOOLEAN DEFAULT TRUE,
    is_favorite BOOLEAN DEFAULT FALSE,
    tags TEXT[],

    -- Type-specific data (JSONB for flexibility)
    type_specific_data JSONB,

    -- Audit
    added_at TIMESTAMPTZ DEFAULT NOW(),
    added_by UUID REFERENCES xcentro.user_table(id) ON DELETE SET NULL,
    last_used_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    metadata JSONB,

    -- Composite primary key
    PRIMARY KEY (business_id, payee_id, account_id)
);

-- Indexes
CREATE INDEX idx_business_payee_relation_business ON xcentro.business_payee_relation(business_id);
CREATE INDEX idx_business_payee_relation_payee ON xcentro.business_payee_relation(payee_id);
CREATE INDEX idx_business_payee_relation_account ON xcentro.business_payee_relation(account_id);
CREATE INDEX idx_business_payee_relation_display_name ON xcentro.business_payee_relation(business_id, display_name);
CREATE INDEX idx_business_payee_relation_active ON xcentro.business_payee_relation(business_id, is_active) WHERE is_active = TRUE;
CREATE INDEX idx_business_payee_relation_favorite ON xcentro.business_payee_relation(business_id, is_favorite) WHERE is_favorite = TRUE;

-- Comments
COMMENT ON TABLE xcentro.business_payee_relation IS 'Customer-payee relationships (polymorphic - supports all payee types)';
COMMENT ON COLUMN xcentro.business_payee_relation.account_id IS 'References bank_account, virtual_account, or biller_account based on payee type';
COMMENT ON COLUMN xcentro.business_payee_relation.display_name IS 'Customer-specific name (can differ from legal name)';
COMMENT ON COLUMN xcentro.business_payee_relation.type_specific_data IS 'Type-specific metadata (e.g., GST: tax liability, Biller: customer ID)';
COMMENT ON COLUMN xcentro.business_payee_relation.last_used_at IS 'Last payment sent to this payee (updated by payment system)';

-- Timestamp trigger
CREATE TRIGGER set_timestamp_business_payee_relation
BEFORE UPDATE ON xcentro.business_payee_relation
FOR EACH ROW
EXECUTE PROCEDURE xcentro.trigger_set_timestamp();


-- #############################################################################
-- # Insert Government GST Payee (Singleton)
-- #############################################################################

-- Insert master entry
INSERT INTO xcentro.payee_master (payee_id, payee_type, display_name)
VALUES ('00000000-0000-0000-0000-000000000001'::UUID, 'gst_payment', 'Government of India - GST')
ON CONFLICT DO NOTHING;

-- Insert GST detail entry
INSERT INTO xcentro.payee_gst (payee_id, government_entity_name, gst_portal_url)
VALUES ('00000000-0000-0000-0000-000000000001'::UUID, 'Government of India - GST', 'https://www.gst.gov.in')
ON CONFLICT DO NOTHING;
