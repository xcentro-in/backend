-- Drop schema if it exists to start fresh (for testing purposes)
-- DROP SCHEMA IF EXISTS xcentro CASCADE;

-- Create schema if it doesn't exist
CREATE SCHEMA IF NOT EXISTS xcentro;

-- #############################################################################
-- # Custom ENUM Types for Data Integrity
-- #############################################################################
CREATE TYPE xcentro.verification_type AS ENUM ('email', 'mobile', 'otp');
CREATE TYPE xcentro.kyc_status_enum AS ENUM ('NOT_STARTED', 'PENDING', 'VERIFIED', 'FAILED', 'REJECTED');
CREATE TYPE xcentro.payment_status_enum AS ENUM ('PENDING', 'PROCESSING', 'SUCCESS', 'FAILED', 'CANCELLED', 'REFUNDED');
CREATE TYPE xcentro.settlement_state_enum AS ENUM ('PENDING', 'IN_TRANSIT', 'SETTLED', 'FAILED');
CREATE TYPE xcentro.payee_type_enum AS ENUM ('STANDARD', 'GST_COLLECTOR', 'PAYROLL', 'BILLER');
CREATE TYPE xcentro.subscription_type_enum AS ENUM ('FREE', 'BASIC', 'PREMIUM', 'ENTERPRISE');
CREATE TYPE xcentro.transfer_type_enum AS ENUM ('PARTNER', 'BUSINESS_TO_BUSINESS');
CREATE TYPE xcentro.user_role_enum AS ENUM ('OWNER', 'ADMIN', 'FINANCE_MANAGER', 'VIEWER');
CREATE TYPE xcentro.payment_method_type AS ENUM ('BANK', 'CARD');
CREATE TYPE xcentro.transaction_status AS ENUM ('PENDING', 'COMPLETED', 'FAILED');
CREATE TYPE xcentro.business_type_enum AS ENUM ('SOLE_PROPRIETORSHIP', 'PARTNERSHIP', 'PRIVATE_LIMITED', 'PUBLIC_LIMITED');
CREATE TYPE xcentro.card_type_enum AS ENUM ('CREDIT', 'DEBIT', 'PREPAID');
CREATE TYPE xcentro.invoice_type_enum AS ENUM ('SALES', 'PURCHASE', 'PROFORMA', 'CREDIT_NOTE');
CREATE TYPE xcentro.process_status_enum AS ENUM ('QUEUED', 'PROCESSING', 'COMPLETED', 'FAILED');
CREATE TYPE xcentro.notification_type_enum AS ENUM ('PAYMENT_RECEIVED', 'PAYMENT_SENT', 'KYC_APPROVED', 'KYC_REJECTED', 'PASSWORD_RESET', 'UPCOMING_PAYMENT');
-- NEW ENUMS from final review
CREATE TYPE xcentro.fee_multiplier_type AS ENUM ('FLAT', 'PERCENTAGE');
CREATE TYPE xcentro.payment_cycle_enum AS ENUM ('WEEKLY', 'BIWEEKLY', 'MONTHLY', 'QUARTERLY', 'ANNUALLY');


-- UUIDv7 Generator - Fast, Secure, RFC 9562 Compliant
CREATE OR REPLACE FUNCTION generate_uuid_v7(
    p_timestamp TIMESTAMPTZ DEFAULT clock_timestamp()
)
RETURNS UUID
LANGUAGE plpgsql
VOLATILE
AS $$
DECLARE
    -- Epoch time as double precision for microsecond extraction
    v_time          DOUBLE PRECISION;
    -- 48-bit UNIX timestamp in milliseconds
    unix_ts_ms      BIGINT;
    -- Microsecond component for sub-millisecond precision
    v_microsec      BIGINT;
    -- The full 16-byte UUID data
    uuid_bytes      BYTEA;
    
    -- Constants for microsecond calculation
    c_milli         DOUBLE PRECISION := 1000.0;      -- 1,000
    c_micro         DOUBLE PRECISION := 1000000.0;   -- 1,000,000
    c_scale         DOUBLE PRECISION := 4.096;       -- 4.0 * (1024 / 1000) for 12-bit precision
BEGIN
    -- 1. Extract epoch time with microsecond precision
    v_time := EXTRACT(EPOCH FROM p_timestamp);
    
    -- 2. Get millisecond timestamp (48 bits)
    unix_ts_ms := TRUNC(v_time * c_milli);
    
    -- 3. Extract microseconds within the current millisecond and encode in 12 bits
    -- This gives us sub-millisecond ordering capability
    v_microsec := TRUNC((v_time * c_micro - unix_ts_ms * c_milli) * c_scale);
    
    -- 4. Generate 16 cryptographically secure random bytes as base
    uuid_bytes := gen_random_bytes(16);
    
    -- 5. Overwrite the first 6 bytes (48 bits) with the big-endian timestamp
    uuid_bytes := OVERLAY(uuid_bytes PLACING SUBSTRING(INT8SEND(unix_ts_ms) FROM 3 FOR 6) FROM 1);
    
    -- 6. Set the version (7) and embed microsecond precision in bytes 6-7
    -- Byte 6: version (4 bits) + high 4 bits of microseconds
    -- Byte 7: low 8 bits of microseconds
    uuid_bytes := SET_BYTE(uuid_bytes, 6, ((7 << 4) | ((v_microsec >> 8) & 15))::INTEGER);
    uuid_bytes := SET_BYTE(uuid_bytes, 7, (v_microsec & 255)::INTEGER);
    
    -- 7. Set the 2-bit variant (RFC 4122) in the 9th byte
    --    Variant is 10xxxxxx, preserving the random bits in the lower 6 bits
    uuid_bytes := SET_BYTE(uuid_bytes, 8, (GET_BYTE(uuid_bytes, 8) & 63) | 128);
    
    -- 8. Convert the final byte array to a UUID and return it
    RETURN ENCODE(uuid_bytes, 'hex')::UUID;
END;
$$;

-- #############################################################################
-- # Automatic Timestamp Trigger Function
-- #############################################################################
CREATE OR REPLACE FUNCTION xcentro.trigger_set_timestamp()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
COMMENT ON FUNCTION xcentro.trigger_set_timestamp() IS 'This trigger function automatically sets the updated_at column to the current timestamp whenever a row is updated.';


-- #############################################################################
-- # User and Business Core Tables
-- #############################################################################

CREATE TABLE xcentro.user_table (
    id UUID PRIMARY KEY DEFAULT generate_uuid_v7(),
    email VARCHAR(254) UNIQUE NOT NULL,
    pwd_hash TEXT NOT NULL,
    mobile VARCHAR(20),
    pan VARCHAR(10),
    kyc_status xcentro.kyc_status_enum NOT NULL DEFAULT 'NOT_STARTED',
    kyc_data JSONB,
    email_verified BOOLEAN DEFAULT FALSE,
    mobile_verified BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ
);
COMMENT ON TABLE xcentro.user_table IS 'User accounts, authentication, and KYC data.';

CREATE TABLE xcentro.kyb_data (
    itid UUID PRIMARY KEY DEFAULT generate_uuid_v7(),
    itid_data JSONB,
    cid UUID DEFAULT generate_uuid_v7(),
    cid_data JSONB,
    doc_url TEXT
);
COMMENT ON TABLE xcentro.kyb_data IS 'Stores Know Your Business (KYB) identity and corporate data.';

CREATE TABLE xcentro.business (
    bid UUID PRIMARY KEY DEFAULT generate_uuid_v7(),
    name TEXT,
    gstin VARCHAR(15) UNIQUE,
    gst_data JSONB,
    itid UUID REFERENCES xcentro.kyb_data(itid),
    kyb_status xcentro.kyc_status_enum DEFAULT 'NOT_STARTED',
    business_type xcentro.business_type_enum,
    country_code VARCHAR(3),
    currency_code VARCHAR(3),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ -- ADDED this column
);
COMMENT ON TABLE xcentro.business IS 'Business entities and their KYB information.';

CREATE TABLE xcentro.business_user_relation (
    user_id UUID NOT NULL REFERENCES xcentro.user_table(id) ON DELETE CASCADE,
    bid UUID NOT NULL REFERENCES xcentro.business(bid) ON DELETE CASCADE,
    role xcentro.user_role_enum NOT NULL,
    role_claim TEXT NOT NULL,
    added_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ,
    PRIMARY KEY (user_id, bid)
);
COMMENT ON TABLE xcentro.business_user_relation IS 'Maps users to businesses with specific roles.';

-- #############################################################################
-- # Wallet and Payment Method Tables
-- #############################################################################

CREATE TABLE xcentro.wallet (
    bid UUID PRIMARY KEY NOT NULL REFERENCES xcentro.business(bid) ON DELETE CASCADE
);
COMMENT ON TABLE xcentro.wallet IS 'Represents the wallet for a business.';

CREATE TABLE xcentro.bank (
    id UUID PRIMARY KEY DEFAULT generate_uuid_v7(),
    bank_name TEXT NOT NULL,
    account_holder_name TEXT NOT NULL,
    account_number TEXT NOT NULL,
    ifsc_code VARCHAR(11) NOT NULL,
    verified_name TEXT
);
COMMENT ON TABLE xcentro.bank IS 'Stores verified bank account details.';

CREATE TABLE xcentro.card (
    id UUID PRIMARY KEY DEFAULT generate_uuid_v7(),
    card_holder_name TEXT NOT NULL,
    masked_num VARCHAR(20) NOT NULL,
    card_type xcentro.card_type_enum NOT NULL,
    billing_address JSONB NOT NULL,
    bank_name TEXT NOT NULL
);
COMMENT ON TABLE xcentro.card IS 'Stores tokenized and masked card details. Billing address is stored as JSONB e.g., {"street": "123 Main St", "city": "Anytown", "zip": "12345", "country": "IN"}.';

CREATE TABLE xcentro.card_token (
    id UUID PRIMARY KEY NOT NULL REFERENCES xcentro.card(id) ON DELETE CASCADE,
    network_token TEXT NOT NULL
);

CREATE TABLE xcentro.wallet_payment_method (
    bid UUID UNIQUE NOT NULL REFERENCES xcentro.wallet(bid) ON DELETE CASCADE,
    payment_type xcentro.payment_method_type NOT NULL,
    bank_id UUID REFERENCES xcentro.bank(id),
    card_id UUID REFERENCES xcentro.card(id),
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    CONSTRAINT chk_payment_method CHECK (
        (payment_type = 'BANK' AND bank_id IS NOT NULL AND card_id IS NULL) OR
        (payment_type = 'CARD' AND card_id IS NOT NULL AND bank_id IS NULL)
    )
);
COMMENT ON TABLE xcentro.wallet_payment_method IS 'Links a wallet to a specific payment method (polymorphic).';

-- #############################################################################
-- # Auto Debit (Mandate) Tables
-- #############################################################################

CREATE TABLE xcentro.bank_auto_debit (
    id UUID PRIMARY KEY NOT NULL REFERENCES xcentro.bank(id) ON DELETE CASCADE,
    mandate JSONB NOT NULL,
    urmn TEXT,
    presentation JSONB
);
COMMENT ON TABLE xcentro.bank_auto_debit IS 'Stores bank mandate details for auto-debit.';

CREATE TABLE xcentro.card_auto_debit (
    id UUID PRIMARY KEY NOT NULL REFERENCES xcentro.card(id) ON DELETE CASCADE,
    mandate JSONB NOT NULL,
    urmn TEXT,
    presentation JSONB
);
COMMENT ON TABLE xcentro.card_auto_debit IS 'Stores card mandate details for auto-debit (tokenized).';

-- #############################################################################
-- # Payments and Financial Transaction Tables
-- #############################################################################

CREATE TABLE xcentro.payment_gateway (
    id UUID PRIMARY KEY DEFAULT generate_uuid_v7(),
    name TEXT NOT NULL,
    metadata JSONB
);

CREATE TABLE xcentro.payments (
    id UUID PRIMARY KEY DEFAULT generate_uuid_v7(),
    bid UUID REFERENCES xcentro.business(bid),
    comment TEXT,
    payment_source_id UUID,
    payment_type xcentro.payment_method_type,
    payment_schedule JSONB, -- CHANGED from TEXT for structured rules
    payment_schedule_date DATE,
    payment_date TIMESTAMPTZ,
    payment_status xcentro.payment_status_enum,
    payment_gateway_id UUID REFERENCES xcentro.payment_gateway(id),
    pg_ref TEXT,
    initiated_by UUID REFERENCES xcentro.user_table(id),
    initiated_date TIMESTAMPTZ DEFAULT NOW(),
    waiting_at UUID,
    approved_by UUID,
    settlement_state xcentro.settlement_state_enum,
    auto_debit_type xcentro.payment_method_type,
    bank_auto_debit_id UUID REFERENCES xcentro.bank_auto_debit(id),
    card_auto_debit_id UUID REFERENCES xcentro.card_auto_debit(id),
    CONSTRAINT chk_auto_debit CHECK (
        (auto_debit_type IS NULL AND bank_auto_debit_id IS NULL AND card_auto_debit_id IS NULL) OR
        (auto_debit_type = 'BANK' AND bank_auto_debit_id IS NOT NULL AND card_auto_debit_id IS NULL) OR
        (auto_debit_type = 'CARD' AND card_auto_debit_id IS NOT NULL AND bank_auto_debit_id IS NULL)
    )
);
COMMENT ON TABLE xcentro.payments IS 'Core table for all payment transactions. payment_schedule stores rules like {"type": "monthly", "day": 15}.';

CREATE TABLE xcentro.processing_fee (
    payment_id UUID PRIMARY KEY NOT NULL REFERENCES xcentro.payments(id) ON DELETE CASCADE,
    original_amount DECIMAL,
    reward_fee DECIMAL(5,2),
    reward_mul_type xcentro.fee_multiplier_type, -- CHANGED from TEXT
    percentage_rate DECIMAL(5,2),
    flat_rate DECIMAL,
    cgst DECIMAL,
    igst DECIMAL,
    sgst DECIMAL,
    final_amount DECIMAL
);

CREATE TABLE xcentro.settlement_table (
    id UUID PRIMARY KEY DEFAULT generate_uuid_v7(),
    payment_id UUID UNIQUE NOT NULL REFERENCES xcentro.payments(id),
    settlement_timestamp TIMESTAMPTZ NOT NULL,
    settlement_amount DECIMAL NOT NULL,
    currency_code VARCHAR(3) NOT NULL,
    settlement_date DATE NOT NULL
);
COMMENT ON TABLE xcentro.settlement_table IS 'Records the settlement details for payments.';

CREATE TABLE xcentro.payouts (
    id UUID PRIMARY KEY DEFAULT generate_uuid_v7(),
    payment_id UUID REFERENCES xcentro.payments(id),
    payout_status xcentro.payment_status_enum,
    payout_request JSONB,
    payout_fees DECIMAL,
    init_time TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE xcentro.refund_payouts (
    id UUID PRIMARY KEY DEFAULT generate_uuid_v7(),
    payment_id UUID REFERENCES xcentro.payments(id),
    payout_status xcentro.payment_status_enum,
    payout_request JSONB,
    payout_fees DECIMAL,
    init_time TIMESTAMPTZ DEFAULT NOW()
);

-- #############################################################################
-- # Payee (Polymorphic Recipient) Tables
-- #############################################################################

CREATE TABLE xcentro.payee (
    id UUID PRIMARY KEY DEFAULT generate_uuid_v7(),
    bid UUID NOT NULL REFERENCES xcentro.business(bid),
    payee_type xcentro.payee_type_enum NOT NULL,
    is_temporary BOOLEAN NOT NULL DEFAULT FALSE
);
COMMENT ON TABLE xcentro.payee IS 'Base table for different types of payment recipients (polymorphic).';

CREATE TABLE xcentro.gst_collector (
    id UUID PRIMARY KEY NOT NULL REFERENCES xcentro.payee(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    email VARCHAR(254),
    phone VARCHAR(20),
    bid UUID NOT NULL,
    bid_data JSONB,
    payment_source_id UUID NOT NULL REFERENCES xcentro.bank(id)
);

CREATE TABLE xcentro.standard_payee (
    id UUID PRIMARY KEY NOT NULL REFERENCES xcentro.payee(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    email VARCHAR(254),
    phone VARCHAR(20),
    bid UUID NOT NULL,
    bid_data JSONB,
    payment_source_id UUID NOT NULL REFERENCES xcentro.bank(id)
);

CREATE TABLE xcentro.payroll_payee (
    id UUID PRIMARY KEY NOT NULL REFERENCES xcentro.payee(id) ON DELETE CASCADE,
    employees_proof_doc TEXT,
    payment_cycle xcentro.payment_cycle_enum, -- CHANGED from TEXT
    payment_date DATE
);

CREATE TABLE xcentro.employee_metadata (
    id UUID PRIMARY KEY DEFAULT generate_uuid_v7(),
    payroll_id UUID NOT NULL REFERENCES xcentro.payroll_payee(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    email VARCHAR(254),
    phone VARCHAR(20),
    join_date DATE NOT NULL,
    salary_amount BIGINT NOT NULL,
    payment_source_id UUID NOT NULL REFERENCES xcentro.bank(id)
);

-- #############################################################################
-- # BBPS (Bharat Bill Payment System) Tables
-- #############################################################################

CREATE TABLE xcentro.bbps_categories (
    id BIGSERIAL PRIMARY KEY,
    name TEXT,
    biller_count BIGINT
);

CREATE TABLE xcentro.bbps_billers (
    id TEXT PRIMARY KEY NOT NULL,
    name TEXT NOT NULL,
    category BIGINT NOT NULL REFERENCES xcentro.bbps_categories(id),
    customers_params JSONB
);

CREATE TABLE xcentro.billers_metadata (
    id TEXT PRIMARY KEY NOT NULL REFERENCES xcentro.bbps_billers(id) ON DELETE CASCADE,
    metadata JSONB
);

CREATE TABLE xcentro.biller_payee (
    id UUID PRIMARY KEY NOT NULL REFERENCES xcentro.payee(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    email VARCHAR(254),
    phone VARCHAR(20) NOT NULL,
    biller_id TEXT NOT NULL REFERENCES xcentro.bbps_billers(id),
    customer_id TEXT,
    cou_payment_source_id UUID NOT NULL REFERENCES xcentro.bank(id)
);

CREATE TABLE xcentro.bbps_invoice (
    id UUID PRIMARY KEY DEFAULT generate_uuid_v7(),
    bbps_ref_id TEXT,
    biller_id TEXT REFERENCES xcentro.bbps_billers(id),
    bid UUID REFERENCES xcentro.business(bid),
    payee_id UUID REFERENCES xcentro.payee(id),
    payment_id UUID REFERENCES xcentro.payments(id),
    customer_params JSONB
);

CREATE TABLE xcentro.bbps_bill (
    bill_number TEXT PRIMARY KEY,
    customer_name TEXT,
    total_amount DECIMAL(20,2),
    bill_date DATE,
    bill_due_date DATE,
    bill_period JSONB, -- CHANGED from TEXT for structured data e.g. {"month": "JAN", "year": 2024}
    payment_options JSONB,
    invoice_id UUID UNIQUE REFERENCES xcentro.bbps_invoice(id)
);

-- #############################################################################
-- # Invoicing Tables
-- #############################################################################

CREATE TABLE xcentro.invoice (
    id UUID PRIMARY KEY DEFAULT generate_uuid_v7(),
    inv_id TEXT,
    type xcentro.invoice_type_enum,
    bid UUID REFERENCES xcentro.business(bid),
    payment_id UUID REFERENCES xcentro.payments(id),
    payee_id UUID REFERENCES xcentro.payee(id),
    inv_date DATE,
    inv_due_date DATE,
    cgst DECIMAL(10,2),
    igst DECIMAL(10,2),
    sgst DECIMAL(10,2),
    total_amount DECIMAL(20,2),
    currency VARCHAR(3),
    invoice_url TEXT,
    comments TEXT,
    document_id UUID REFERENCES xcentro.voucher_documents(id)
);

CREATE TABLE xcentro.recurring_invoice (
    id UUID PRIMARY KEY DEFAULT generate_uuid_v7(),
    inv_id TEXT,
    type xcentro.invoice_type_enum,
    bid UUID,
    auto_debit_type xcentro.payment_method_type NOT NULL,
    bank_auto_debit_id UUID REFERENCES xcentro.bank_auto_debit(id),
    card_auto_debit_id UUID REFERENCES xcentro.card_auto_debit(id),
    payment_id UUID REFERENCES xcentro.payments(id),
    payee_id UUID REFERENCES xcentro.payee(id),
    invoice_schedule JSONB, -- CHANGED from TEXT for structured rules
    invoice_payment_date DATE,
    cgst DECIMAL(10,2),
    igst DECIMAL(10,2),
    sgst DECIMAL(10,2),
    total_amount DECIMAL(20,2),
    currency VARCHAR(3),
    comments TEXT,
    CONSTRAINT chk_recurring_auto_debit CHECK (
        (auto_debit_type = 'BANK' AND bank_auto_debit_id IS NOT NULL AND card_auto_debit_id IS NULL) OR
        (auto_debit_type = 'CARD' AND card_auto_debit_id IS NOT NULL AND bank_auto_debit_id IS NULL)
    )
);
COMMENT ON TABLE xcentro.recurring_invoice IS 'invoice_schedule stores rules like {"type": "monthly", "day": "last"}.';

-- #############################################################################
-- # Rewards System Tables
-- #############################################################################

CREATE TABLE xcentro.subscription_status (
    bid UUID PRIMARY KEY NOT NULL REFERENCES xcentro.business(bid) ON DELETE CASCADE,
    subscription_type xcentro.subscription_type_enum NOT NULL,
    start_date DATE,
    expiry_date DATE
);

CREATE TABLE xcentro.rewards_ledger (
    bid UUID PRIMARY KEY NOT NULL REFERENCES xcentro.business(bid) ON DELETE CASCADE,
    reward_points BIGINT NOT NULL DEFAULT 0,
    actual_value DECIMAL NOT NULL DEFAULT 0.0,
    total_points_issued BIGINT NOT NULL DEFAULT 0,
    total_points_redeemed BIGINT NOT NULL DEFAULT 0,
    total_points_expired BIGINT NOT NULL DEFAULT 0
);
COMMENT ON TABLE xcentro.rewards_ledger IS 'Business reward points balance and history.';

CREATE TABLE xcentro.reward_goals (
    id BIGSERIAL PRIMARY KEY,
    bid UUID NOT NULL REFERENCES xcentro.business(bid),
    title TEXT,
    points_target BIGINT,
    desired_reward TEXT
);

CREATE TABLE xcentro.reward_partners (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    conversion_rate DECIMAL NOT NULL,
    metadata JSONB
);

CREATE TABLE xcentro.reward_spend_txn (
    id UUID PRIMARY KEY DEFAULT generate_uuid_v7(),
    from_bid UUID NOT NULL REFERENCES xcentro.business(bid),
    points_redeemed BIGINT NOT NULL,
    transfer_type xcentro.transfer_type_enum NOT NULL,
    partner_id BIGINT REFERENCES xcentro.reward_partners(id),
    partner_points_gained BIGINT,
    partner_transfer_status xcentro.transaction_status,
    partner_txn_id TEXT,
    to_bid UUID REFERENCES xcentro.business(bid),
    business_transfer_status xcentro.transaction_status,
    "timestamp" TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE xcentro.reward_earning_txn (
    id BIGSERIAL PRIMARY KEY,
    payment_id UUID NOT NULL REFERENCES xcentro.payments(id),
    bid UUID NOT NULL REFERENCES xcentro.business(bid),
    points_earned BIGINT NOT NULL DEFAULT 0,
    bonus_points_earned BIGINT NOT NULL DEFAULT 0,
    earned_date DATE NOT NULL,
    expiry_date DATE NOT NULL,
    status xcentro.transaction_status,
    processing_started_at TIMESTAMPTZ
);

-- #############################################################################
-- # Notifications and Auditing Tables
-- #############################################################################

CREATE TABLE xcentro.user_verification_codes (
    id TEXT PRIMARY KEY,
    user_id UUID REFERENCES xcentro.user_table(id) ON DELETE CASCADE,
    code TEXT,
    code_type xcentro.verification_type,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    used BOOLEAN DEFAULT FALSE,
    retry_attempts INTEGER DEFAULT 0
);

CREATE TABLE xcentro.notification_preferences (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID UNIQUE NOT NULL REFERENCES xcentro.user_table(id) ON DELETE CASCADE,
    email_notifications BOOLEAN NOT NULL DEFAULT TRUE,
    push_notifications BOOLEAN NOT NULL DEFAULT TRUE,
    categories JSONB
);

CREATE TABLE xcentro.notifications (
    id UUID PRIMARY KEY DEFAULT generate_uuid_v7(),
    user_id UUID NOT NULL REFERENCES xcentro.user_table(id) ON DELETE CASCADE,
    type xcentro.notification_type_enum NOT NULL,
    is_read BOOLEAN NOT NULL DEFAULT FALSE,
    "timestamp" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    data JSONB NOT NULL
);

CREATE TABLE xcentro.notifications_business (
    id UUID PRIMARY KEY DEFAULT generate_uuid_v7(),
    bid UUID NOT NULL REFERENCES xcentro.business(bid),
    type xcentro.notification_type_enum NOT NULL,
    "timestamp" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    data JSONB NOT NULL,
    status xcentro.process_status_enum NOT NULL,
    processing_started_at TIMESTAMPTZ
);

CREATE TABLE xcentro.notification_data_schema (
    id BIGSERIAL PRIMARY KEY,
    json_schema JSONB NOT NULL
);

CREATE TABLE xcentro.audit_log_table (
    id BIGSERIAL PRIMARY KEY,
    "timestamp" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    log_data JSONB NOT NULL,
    status xcentro.process_status_enum NOT NULL,
    processing_started_at TIMESTAMPTZ
);

CREATE TABLE xcentro.audit_log_data_schema (
    id BIGSERIAL PRIMARY KEY,
    json_schema JSONB NOT NULL
);

CREATE TABLE xcentro.payment_audit_trail (
    id BIGSERIAL PRIMARY KEY,
    payment_id UUID NOT NULL REFERENCES xcentro.payments(id) ON DELETE CASCADE,
    trail_data JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- #############################################################################
-- # Rules and Permissions Tables
-- #############################################################################

CREATE TABLE xcentro.reward_rule_engine (
    id BIGSERIAL PRIMARY KEY,
    rules JSONB NOT NULL
);

CREATE TABLE xcentro.rule_engine (
    id BIGSERIAL PRIMARY KEY,
    rule_data JSONB
);

CREATE TABLE xcentro.payout_rules (
    id BIGSERIAL PRIMARY KEY,
    rules JSONB
);

CREATE TABLE xcentro.ops_role_permission_map (
    idp_role_name TEXT NOT NULL,
    role_name TEXT UNIQUE NOT NULL,
    permission TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (idp_role_name, role_name)
);

CREATE TABLE xcentro.app_role_permission_map (
    role_name TEXT PRIMARY KEY,
    permission TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- #############################################################################
-- # Triggers for Automatic Timestamps
-- #############################################################################
CREATE TRIGGER set_timestamp
BEFORE UPDATE ON xcentro.user_table
FOR EACH ROW
EXECUTE PROCEDURE xcentro.trigger_set_timestamp();

CREATE TRIGGER set_timestamp
BEFORE UPDATE ON xcentro.business_user_relation
FOR EACH ROW
EXECUTE PROCEDURE xcentro.trigger_set_timestamp();

CREATE TRIGGER set_timestamp
BEFORE UPDATE ON xcentro.ops_role_permission_map
FOR EACH ROW
EXECUTE PROCEDURE xcentro.trigger_set_timestamp();

CREATE TRIGGER set_timestamp
BEFORE UPDATE ON xcentro.app_role_permission_map
FOR EACH ROW
EXECUTE PROCEDURE xcentro.trigger_set_timestamp();

-- ADDED trigger for business table
CREATE TRIGGER set_timestamp
BEFORE UPDATE ON xcentro.business
FOR EACH ROW
EXECUTE PROCEDURE xcentro.trigger_set_timestamp();


-- #############################################################################
-- # Indexes for Performance
-- #############################################################################
CREATE INDEX idx_business_user_relation_bid ON xcentro.business_user_relation(bid);
CREATE INDEX idx_payments_bid ON xcentro.payments(bid);
CREATE INDEX idx_payments_payment_gateway_id ON xcentro.payments(payment_gateway_id);
CREATE INDEX idx_payee_bid ON xcentro.payee(bid);
CREATE INDEX idx_invoice_bid ON xcentro.invoice(bid);
CREATE INDEX idx_invoice_payment_id ON xcentro.invoice(payment_id);
CREATE INDEX idx_invoice_payee_id ON xcentro.invoice(payee_id);
CREATE INDEX idx_notifications_user_id ON xcentro.notifications(user_id);
CREATE INDEX idx_notifications_business_bid ON xcentro.notifications_business(bid);
CREATE INDEX idx_reward_earning_txn_bid ON xcentro.reward_earning_txn(bid);
CREATE INDEX idx_reward_spend_txn_from_bid ON xcentro.reward_spend_txn(from_bid);
CREATE INDEX idx_reward_spend_txn_to_bid ON xcentro.reward_spend_txn(to_bid);
CREATE INDEX idx_user_verification_codes_user_id ON xcentro.user_verification_codes(user_id);
CREATE INDEX idx_business_itid ON xcentro.business(itid);
CREATE INDEX idx_bbps_invoice_bid ON xcentro.bbps_invoice(bid);
CREATE INDEX idx_reward_goals_bid ON xcentro.reward_goals(bid);

-- #############################################################################
-- # Extraction and Document Processing Tables
-- #############################################################################

CREATE TABLE xcentro.voucher_documents (
    id UUID PRIMARY KEY,
    s3_path TEXT NOT NULL,
    doc_type TEXT,
    status TEXT NOT NULL DEFAULT 'QUEUED',
    raw_extraction JSONB,
    draft_data JSONB,
    i2d_template TEXT,
    processed_by TEXT,
    validated_data JSONB
);
COMMENT ON TABLE xcentro.voucher_documents IS 'Tracks the lifecycle of uploaded documents from ingestion through extraction to validation.';

CREATE TABLE xcentro.purchase_orders (
    id UUID PRIMARY KEY DEFAULT generate_uuid_v7(),
    document_id UUID NOT NULL REFERENCES xcentro.voucher_documents(id),
    po_number TEXT,
    po_date TEXT,
    vendor_name TEXT,
    vendor_gstin TEXT,
    buyer_gstin TEXT,
    total_amount DECIMAL(20,2),
    discount DECIMAL(20,2),
    discount_type TEXT,
    line_items JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
COMMENT ON TABLE xcentro.purchase_orders IS 'Stores validated purchase order data extracted from voucher documents.';
