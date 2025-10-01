-- #############################################################################
-- # Payment System Database Schema
-- # Phase 4.3: Create payment tables for XCentro payment processing
-- #############################################################################

-- #############################################################################
-- # Custom ENUM Types for Payment System
-- #############################################################################

-- Payment status lifecycle
CREATE TYPE xcentro.payment_status_enum AS ENUM ('pending', 'success', 'failed', 'cancelled');

-- Payment gateway providers
CREATE TYPE xcentro.gateway_type_enum AS ENUM ('jiopay', 'easebuzz');

-- Payment methods supported
CREATE TYPE xcentro.payment_method_enum AS ENUM ('card', 'token_card', 'net_banking', 'upi', 'wallet');

-- Card types (brands)
CREATE TYPE xcentro.card_type_enum AS ENUM ('visa', 'mastercard', 'rupay', 'amex', 'diners');

-- Card categories
CREATE TYPE xcentro.card_category_enum AS ENUM ('credit', 'debit', 'corporate');

-- Payment source types (polymorphic reference)
CREATE TYPE xcentro.payment_source_type_enum AS ENUM ('card', 'bank_account', 'upi', 'credit_line');

-- Currency codes (ISO 4217)
CREATE TYPE xcentro.currency_code_enum AS ENUM ('INR', 'USD', 'EUR', 'GBP', 'AED');


-- #############################################################################
-- # Stored Cards Table (PCI DSS Compliant - Token Storage Only)
-- #############################################################################
CREATE TABLE xcentro.stored_cards (
    card_id UUID PRIMARY KEY DEFAULT uuidv7(),
    business_id UUID NOT NULL REFERENCES xcentro.business(bid) ON DELETE CASCADE,

    -- Gateway token (NOT actual card number - PCI DSS compliant)
    token TEXT NOT NULL,

    -- Card display information (safe to store)
    last_four_digits VARCHAR(4) NOT NULL,
    card_type xcentro.card_type_enum NOT NULL,
    card_category xcentro.card_category_enum NOT NULL,
    expiry_month VARCHAR(2) NOT NULL,
    expiry_year VARCHAR(4) NOT NULL,
    issuer_bank TEXT,
    card_holder_name TEXT,

    -- Status and tracking
    is_active BOOLEAN DEFAULT TRUE,
    gateway_used xcentro.gateway_type_enum NOT NULL,
    added_at TIMESTAMPTZ DEFAULT NOW(),
    last_used_at TIMESTAMPTZ,

    -- Flexible metadata storage (tokenization details, etc.)
    metadata JSONB
);

COMMENT ON TABLE xcentro.stored_cards IS 'Tokenized card storage for recurring payments (PCI DSS compliant)';
COMMENT ON COLUMN xcentro.stored_cards.token IS 'Gateway-provided token - NOT actual card number';
COMMENT ON COLUMN xcentro.stored_cards.last_four_digits IS 'Last 4 digits for display purposes only';

-- Unique constraint: one token per business per gateway
CREATE UNIQUE INDEX idx_stored_cards_token_unique ON xcentro.stored_cards(business_id, token);

-- Performance indexes (commented - enable based on metrics)
-- CREATE INDEX idx_stored_cards_business_id ON xcentro.stored_cards(business_id);
-- CREATE INDEX idx_stored_cards_active ON xcentro.stored_cards(business_id, is_active) WHERE is_active = TRUE;
-- CREATE INDEX idx_stored_cards_gateway ON xcentro.stored_cards(gateway_used);
-- CREATE INDEX idx_stored_cards_card_type ON xcentro.stored_cards(card_type, card_category);


-- #############################################################################
-- # Bank Accounts Table (with Encryption Strategy)
-- #############################################################################
CREATE TABLE xcentro.bank_accounts (
    account_id UUID PRIMARY KEY DEFAULT uuidv7(),
    business_id UUID NOT NULL REFERENCES xcentro.business(bid) ON DELETE CASCADE,

    -- Encrypted account number storage
    account_number_hash VARCHAR(64) NOT NULL,  -- SHA-256 hash for duplicate detection
    account_number_encrypted TEXT NOT NULL,     -- AES-256-GCM encrypted actual number

    -- Bank details
    ifsc_code VARCHAR(11) NOT NULL,
    account_holder_name TEXT NOT NULL,
    bank_name TEXT,
    branch_name TEXT,
    account_type VARCHAR(20),  -- savings, current, etc.

    -- Verification and status
    is_verified BOOLEAN DEFAULT FALSE,
    is_active BOOLEAN DEFAULT TRUE,
    added_at TIMESTAMPTZ DEFAULT NOW(),

    -- Flexible metadata storage (penny drop verification details, etc.)
    metadata JSONB
);

COMMENT ON TABLE xcentro.bank_accounts IS 'Bank accounts with encrypted account numbers for payment sources';
COMMENT ON COLUMN xcentro.bank_accounts.account_number_hash IS 'SHA-256 hash for O(1) duplicate detection';
COMMENT ON COLUMN xcentro.bank_accounts.account_number_encrypted IS 'AES-256-GCM encrypted account number';
COMMENT ON COLUMN xcentro.bank_accounts.metadata IS 'Stores penny drop verification details (status, date, amount, reference)';

-- Unique constraint: prevent duplicate bank accounts per business
CREATE UNIQUE INDEX idx_bank_accounts_unique ON xcentro.bank_accounts(business_id, account_number_hash);

-- Performance indexes (commented - enable based on metrics)
-- CREATE INDEX idx_bank_accounts_business_id ON xcentro.bank_accounts(business_id);
-- CREATE INDEX idx_bank_accounts_verified ON xcentro.bank_accounts(business_id, is_verified, is_active) WHERE is_verified = TRUE AND is_active = TRUE;
-- CREATE INDEX idx_bank_accounts_ifsc ON xcentro.bank_accounts(ifsc_code);


-- #############################################################################
-- # Payments Table (Main Transaction Records)
-- #############################################################################
CREATE TABLE xcentro.payments (
    payment_id UUID PRIMARY KEY DEFAULT uuidv7(),

    -- User/Business references
    user_id UUID REFERENCES xcentro.user_table(id) ON DELETE SET NULL,
    business_id UUID NOT NULL REFERENCES xcentro.business(bid) ON DELETE CASCADE,

    -- Payment details
    amount BIGINT NOT NULL CHECK (amount > 0),  -- Amount in smallest currency unit (paise for INR)
    currency xcentro.currency_code_enum NOT NULL DEFAULT 'INR',
    payment_method xcentro.payment_method_enum NOT NULL,
    status xcentro.payment_status_enum NOT NULL DEFAULT 'pending',
    description TEXT,

    -- Gateway information
    gateway_used xcentro.gateway_type_enum,
    gateway_payment_id TEXT,  -- Gateway's transaction ID (critical for webhook lookup)

    -- URLs
    callback_url TEXT,
    redirect_url TEXT,

    -- Payment source linkage (polymorphic - can reference stored_cards OR bank_accounts)
    payment_source_id UUID,  -- No FK constraint - polymorphic reference
    payment_source_type xcentro.payment_source_type_enum,

    -- Fee breakdown
    fee_amount BIGINT DEFAULT 0 CHECK (fee_amount >= 0),
    total_amount BIGINT NOT NULL CHECK (total_amount >= amount),  -- amount + fee_amount (GST inclusive)

    -- Flexible metadata storage (gateway responses, payment details, etc.)
    metadata JSONB,

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ
);

COMMENT ON TABLE xcentro.payments IS 'Main payment transaction records with gateway routing and fee tracking';
COMMENT ON COLUMN xcentro.payments.amount IS 'Payment amount in smallest currency unit (paise for INR)';
COMMENT ON COLUMN xcentro.payments.gateway_payment_id IS 'Gateway transaction ID for webhook payment lookup';
COMMENT ON COLUMN xcentro.payments.payment_source_id IS 'Polymorphic reference to stored_cards.card_id or bank_accounts.account_id';
COMMENT ON COLUMN xcentro.payments.payment_source_type IS 'Discriminator for payment_source_id polymorphic relationship';
COMMENT ON COLUMN xcentro.payments.metadata IS 'Stores gateway responses, payment attempts, failure reasons';

-- Unique constraint: prevent duplicate gateway payment IDs
CREATE UNIQUE INDEX idx_payments_gateway_payment_id_unique ON xcentro.payments(gateway_payment_id) WHERE gateway_payment_id IS NOT NULL;

-- Performance indexes (commented - enable based on metrics and query patterns)
-- CREATE INDEX idx_payments_user_id ON xcentro.payments(user_id);
-- CREATE INDEX idx_payments_business_id ON xcentro.payments(business_id);
-- CREATE INDEX idx_payments_gateway_payment_id ON xcentro.payments(gateway_payment_id);  -- Critical for webhooks
-- CREATE INDEX idx_payments_status ON xcentro.payments(status);
-- CREATE INDEX idx_payments_created_at ON xcentro.payments(created_at DESC);
-- CREATE INDEX idx_payments_source ON xcentro.payments(payment_source_id, payment_source_type);
-- CREATE INDEX idx_payments_gateway ON xcentro.payments(gateway_used);
-- CREATE INDEX idx_payments_method ON xcentro.payments(payment_method);
-- CREATE INDEX idx_payments_business_status ON xcentro.payments(business_id, status, created_at DESC);  -- Common query pattern

-- Timestamp trigger
CREATE TRIGGER set_timestamp
BEFORE UPDATE ON xcentro.payments
FOR EACH ROW
EXECUTE PROCEDURE xcentro.trigger_set_timestamp();


-- #############################################################################
-- # TODO-FUTURE: Payment Attempts Table (Retry Tracking & Audit Trail)
-- #############################################################################
-- Deferred to later phase when retry logic is implemented
--
-- CREATE TABLE xcentro.payment_attempts (
--     attempt_id UUID PRIMARY KEY DEFAULT uuidv7(),
--     payment_id UUID NOT NULL REFERENCES xcentro.payments(payment_id) ON DELETE CASCADE,
--
--     -- Attempt details
--     attempt_number INTEGER NOT NULL,
--     gateway_used xcentro.gateway_type_enum NOT NULL,
--     gateway_request JSONB,   -- Full request sent to gateway
--     gateway_response JSONB,  -- Full response from gateway
--
--     -- Status and timing
--     status xcentro.payment_status_enum NOT NULL,
--     error_code TEXT,
--     error_message TEXT,
--     attempted_at TIMESTAMPTZ DEFAULT NOW(),
--     response_time_ms INTEGER,  -- Gateway response time for monitoring
--
--     -- Metadata
--     metadata JSONB
-- );
--
-- COMMENT ON TABLE xcentro.payment_attempts IS 'Audit trail for payment retry attempts and gateway interactions';
--
-- CREATE INDEX idx_payment_attempts_payment_id ON xcentro.payment_attempts(payment_id, attempt_number);
-- CREATE INDEX idx_payment_attempts_attempted_at ON xcentro.payment_attempts(attempted_at DESC);
-- CREATE INDEX idx_payment_attempts_status ON xcentro.payment_attempts(status);