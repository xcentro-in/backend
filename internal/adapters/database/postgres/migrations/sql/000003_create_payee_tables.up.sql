-- #############################################################################
-- # Payee Management System Database Schema
-- # Phase 5.3: Create payee tables for XCentro payee onboarding
-- #############################################################################

-- #############################################################################
-- # Custom ENUM Types for Payee System
-- #############################################################################

-- Verification status for GST, bank, and cross-verification
CREATE TYPE xcentro.verification_status_enum AS ENUM (
    'pending',
    'in_progress',
    'success',
    'failed'
);

-- #############################################################################
-- # Payee Business Table (Global Canonical Payee Entity)
-- #############################################################################
-- Stores the canonical business entity as a payee
-- One entry per GSTIN globally (shared across all customers)

CREATE TABLE xcentro.payee_business (
    payee_id UUID PRIMARY KEY DEFAULT uuidv7(),
    gstin VARCHAR(15) UNIQUE NOT NULL,

    -- Business details from GST verification
    legal_name VARCHAR(500) NOT NULL,
    business_type VARCHAR(100),            -- Proprietorship, Partnership, Private Limited, etc.
    registration_date DATE,
    gst_status VARCHAR(50) NOT NULL,       -- Active, Cancelled, Suspended
    state_code VARCHAR(2),

    -- Primary address from GST
    building_name VARCHAR(255),
    street VARCHAR(255),
    city VARCHAR(100),
    state VARCHAR(100),
    pincode VARCHAR(10),

    -- GST verification
    gst_verification_status xcentro.verification_status_enum DEFAULT 'pending',
    gst_verified_at TIMESTAMPTZ,
    gst_data JSONB,                        -- Full GST API response

    -- Audit
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ,
    metadata JSONB
);

-- Indexes for payee_business
-- CREATE INDEX idx_payee_business_gstin ON xcentro.payee_business(gstin);
-- CREATE INDEX idx_payee_business_legal_name ON xcentro.payee_business(legal_name);
-- CREATE INDEX idx_payee_business_gst_status ON xcentro.payee_business(gst_status);

-- Comments
COMMENT ON TABLE xcentro.payee_business IS 'Canonical payee businesses identified by GSTIN (global, shared across customers)';
COMMENT ON COLUMN xcentro.payee_business.gstin IS 'Unique 15-character GST identification number';
COMMENT ON COLUMN xcentro.payee_business.gst_data IS 'Full GST verification API response including directors';

-- Timestamp trigger
CREATE TRIGGER set_timestamp_payee_business
BEFORE UPDATE ON xcentro.payee_business
FOR EACH ROW
EXECUTE PROCEDURE xcentro.trigger_set_timestamp();


-- #############################################################################
-- # Payee Bank Accounts Table (One-to-Many with Payee)
-- #############################################################################
-- Stores bank accounts for payees
-- One payee can have multiple accounts (current, savings, different banks)

CREATE TABLE xcentro.payee_bank_accounts (
    account_id UUID PRIMARY KEY DEFAULT uuidv7(),
    payee_id UUID NOT NULL REFERENCES xcentro.payee_business(payee_id) ON DELETE CASCADE,

    -- Bank details (encrypted at rest - Phase 5.5)
    account_number_encrypted TEXT NOT NULL,
    account_number_hash VARCHAR(64) NOT NULL,  -- SHA-256 for duplicate detection
    ifsc_code VARCHAR(11) NOT NULL,
    bank_name VARCHAR(255),
    branch_name VARCHAR(255),
    account_type xcentro.bank_account_type_enum NOT NULL,
    account_holder_name VARCHAR(255) NOT NULL,

    -- Bank verification (penny drop)
    is_verified BOOLEAN DEFAULT FALSE,
    verified_at TIMESTAMPTZ,
    bank_verification_status xcentro.verification_status_enum DEFAULT 'pending',
    bank_verification_data JSONB,              -- Penny drop API response

    -- Cross-verification (GST name vs Bank name)
    cross_verification_status xcentro.verification_status_enum DEFAULT 'pending',
    name_match_score NUMERIC(3,2),             -- 0.00 to 1.00

    -- Account status
    is_active BOOLEAN DEFAULT TRUE,
    is_primary BOOLEAN DEFAULT FALSE,          -- Default account for this payee
    account_label VARCHAR(100),                -- "Retail Operations", "Wholesale", etc.

    -- Audit
    added_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ,
    metadata JSONB,

    -- Constraints
    CONSTRAINT unique_payee_account UNIQUE (payee_id, account_number_hash)
);

-- Indexes for payee_bank_accounts
-- CREATE INDEX idx_payee_bank_accounts_payee_id ON xcentro.payee_bank_accounts(payee_id);
-- CREATE INDEX idx_payee_bank_accounts_is_primary ON xcentro.payee_bank_accounts(payee_id, is_primary) WHERE is_primary = TRUE;
-- CREATE INDEX idx_payee_bank_accounts_active ON xcentro.payee_bank_accounts(payee_id, is_active) WHERE is_active = TRUE;

-- Comments
COMMENT ON TABLE xcentro.payee_bank_accounts IS 'Bank accounts for payees (one payee can have multiple accounts)';
COMMENT ON COLUMN xcentro.payee_bank_accounts.account_number_hash IS 'SHA-256 hash for duplicate detection per payee';
COMMENT ON COLUMN xcentro.payee_bank_accounts.account_number_encrypted IS 'Encrypted account number (plaintext in Phase 5.3, encrypted in Phase 5.5)';
COMMENT ON COLUMN xcentro.payee_bank_accounts.is_primary IS 'Only one primary account per payee (enforced in service layer)';

-- Timestamp trigger
CREATE TRIGGER set_timestamp_payee_bank_accounts
BEFORE UPDATE ON xcentro.payee_bank_accounts
FOR EACH ROW
EXECUTE PROCEDURE xcentro.trigger_set_timestamp();


-- #############################################################################
-- # Business-Payee Relation Table (Many-to-Many-to-Many)
-- #############################################################################
-- Maps customers to payees and their bank accounts
-- Stores customer-specific metadata (display name, contacts, notes)

CREATE TABLE xcentro.business_payee_relation (
    business_id UUID NOT NULL REFERENCES xcentro.business(bid) ON DELETE CASCADE,
    payee_id UUID NOT NULL REFERENCES xcentro.payee_business(payee_id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES xcentro.payee_bank_accounts(account_id) ON DELETE CASCADE,

    -- Customer-specific metadata (how THIS customer sees the payee)
    display_name VARCHAR(255) NOT NULL,
    contact_email VARCHAR(254),
    contact_phone VARCHAR(20),
    notes TEXT,

    -- Relationship metadata
    is_active BOOLEAN DEFAULT TRUE,
    is_favorite BOOLEAN DEFAULT FALSE,
    tags TEXT[],                               -- Array of tags for categorization

    -- Audit
    added_at TIMESTAMPTZ DEFAULT NOW(),
    added_by UUID REFERENCES xcentro.user_table(id) ON DELETE SET NULL,
    last_used_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    metadata JSONB,

    -- Composite primary key
    PRIMARY KEY (business_id, payee_id, account_id)
);

-- Indexes for business_payee_relation
-- CREATE INDEX idx_business_payee_relation_business ON xcentro.business_payee_relation(business_id);
-- CREATE INDEX idx_business_payee_relation_payee ON xcentro.business_payee_relation(payee_id);
-- CREATE INDEX idx_business_payee_relation_account ON xcentro.business_payee_relation(account_id);
-- CREATE INDEX idx_business_payee_relation_display_name ON xcentro.business_payee_relation(business_id, display_name);
-- CREATE INDEX idx_business_payee_relation_active ON xcentro.business_payee_relation(business_id, is_active) WHERE is_active = TRUE;
-- CREATE INDEX idx_business_payee_relation_favorite ON xcentro.business_payee_relation(business_id, is_favorite) WHERE is_favorite = TRUE;

-- Comments
COMMENT ON TABLE xcentro.business_payee_relation IS 'Customer-payee-account relationships (how each customer sees their payees)';
COMMENT ON COLUMN xcentro.business_payee_relation.display_name IS 'Customer-specific name for payee (can differ from legal_name)';
COMMENT ON COLUMN xcentro.business_payee_relation.tags IS 'Array of tags for flexible categorization';
COMMENT ON COLUMN xcentro.business_payee_relation.last_used_at IS 'Last payment sent to this payee (updated by payment system)';

-- Timestamp trigger
CREATE TRIGGER set_timestamp_business_payee_relation
BEFORE UPDATE ON xcentro.business_payee_relation
FOR EACH ROW
EXECUTE PROCEDURE xcentro.trigger_set_timestamp();
