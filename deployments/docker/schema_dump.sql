--
-- PostgreSQL database dump
--

\restrict Egde31RWpFfEEGERRaOkCufxCJqRgKiqbs4sfh0Q93uIoRjqQfC4z5vKjPUukMf

-- Dumped from database version 18.1 (Debian 18.1-1.pgdg13+2)
-- Dumped by pg_dump version 18.1 (Debian 18.1-1.pgdg13+2)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: xcentro; Type: SCHEMA; Schema: -; Owner: -
--

CREATE SCHEMA xcentro;


--
-- Name: approval_action_enum; Type: TYPE; Schema: xcentro; Owner: -
--

CREATE TYPE xcentro.approval_action_enum AS ENUM (
    'pending',
    'approved',
    'rejected',
    'request_changes',
    'edited'
);


--
-- Name: TYPE approval_action_enum; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TYPE xcentro.approval_action_enum IS 'Approval actions: pending, approved, rejected, request_changes, edited';


--
-- Name: bank_account_type_enum; Type: TYPE; Schema: xcentro; Owner: -
--

CREATE TYPE xcentro.bank_account_type_enum AS ENUM (
    'savings',
    'current',
    'cc',
    'od'
);


--
-- Name: business_type_enum; Type: TYPE; Schema: xcentro; Owner: -
--

CREATE TYPE xcentro.business_type_enum AS ENUM (
    'sole_proprietorship',
    'partnership',
    'limited_liability_partnership',
    'private_limited_company',
    'one_person_company',
    'section_8_non_profit',
    'trust',
    'ngo',
    'society',
    'cooperative_society',
    'other'
);


--
-- Name: card_category_enum; Type: TYPE; Schema: xcentro; Owner: -
--

CREATE TYPE xcentro.card_category_enum AS ENUM (
    'credit',
    'debit',
    'corporate'
);


--
-- Name: card_type_enum; Type: TYPE; Schema: xcentro; Owner: -
--

CREATE TYPE xcentro.card_type_enum AS ENUM (
    'visa',
    'mastercard',
    'rupay',
    'amex',
    'diners'
);


--
-- Name: currency_code_enum; Type: TYPE; Schema: xcentro; Owner: -
--

CREATE TYPE xcentro.currency_code_enum AS ENUM (
    'INR',
    'USD',
    'EUR',
    'GBP',
    'AED'
);


--
-- Name: device_type_enum; Type: TYPE; Schema: xcentro; Owner: -
--

CREATE TYPE xcentro.device_type_enum AS ENUM (
    'web',
    'mobile'
);


--
-- Name: fee_type_enum; Type: TYPE; Schema: xcentro; Owner: -
--

CREATE TYPE xcentro.fee_type_enum AS ENUM (
    'percentage',
    'fixed',
    'hybrid'
);


--
-- Name: filing_status_enum; Type: TYPE; Schema: xcentro; Owner: -
--

CREATE TYPE xcentro.filing_status_enum AS ENUM (
    'not_filed',
    'filed',
    'failed'
);


--
-- Name: gateway_type_enum; Type: TYPE; Schema: xcentro; Owner: -
--

CREATE TYPE xcentro.gateway_type_enum AS ENUM (
    'jiopay',
    'easebuzz',
    'mockpg'
);


--
-- Name: irn_status_enum; Type: TYPE; Schema: xcentro; Owner: -
--

CREATE TYPE xcentro.irn_status_enum AS ENUM (
    'pending',
    'irn_generated',
    'failed',
    'cancelled'
);


--
-- Name: kyc_status_enum; Type: TYPE; Schema: xcentro; Owner: -
--

CREATE TYPE xcentro.kyc_status_enum AS ENUM (
    'kyc_not_started',
    'kyc_in_progress',
    'kyc_completed',
    'kyc_verified',
    'kyc_needs_update',
    'kyc_rejected'
);


--
-- Name: payee_type_enum; Type: TYPE; Schema: xcentro; Owner: -
--

CREATE TYPE xcentro.payee_type_enum AS ENUM (
    'vendor',
    'gst_payment',
    'biller',
    'employee'
);


--
-- Name: payment_action_enum; Type: TYPE; Schema: xcentro; Owner: -
--

CREATE TYPE xcentro.payment_action_enum AS ENUM (
    'payment_initiated',
    'payment_processing',
    'payment_success',
    'payment_failed',
    'payment_refunded',
    'settlement_pending',
    'settlement_received',
    'settlement_failed',
    'payout_initiated',
    'payout_processing',
    'payout_completed',
    'payout_failed',
    'payout_timeout',
    'payout_cancelled'
);


--
-- Name: TYPE payment_action_enum; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TYPE xcentro.payment_action_enum IS 'Payment and payout lifecycle actions for audit trail';


--
-- Name: payment_actor_enum; Type: TYPE; Schema: xcentro; Owner: -
--

CREATE TYPE xcentro.payment_actor_enum AS ENUM (
    'user',
    'system',
    'webhook',
    'api'
);


--
-- Name: TYPE payment_actor_enum; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TYPE xcentro.payment_actor_enum IS 'Actor type that triggered the payment state change';


--
-- Name: payment_method_enum; Type: TYPE; Schema: xcentro; Owner: -
--

CREATE TYPE xcentro.payment_method_enum AS ENUM (
    'card',
    'token_card',
    'net_banking',
    'upi',
    'wallet'
);


--
-- Name: payment_source_type_enum; Type: TYPE; Schema: xcentro; Owner: -
--

CREATE TYPE xcentro.payment_source_type_enum AS ENUM (
    'card',
    'bank_account',
    'upi',
    'credit_line'
);


--
-- Name: payment_status_enum; Type: TYPE; Schema: xcentro; Owner: -
--

CREATE TYPE xcentro.payment_status_enum AS ENUM (
    'pending',
    'success',
    'failed',
    'cancelled'
);


--
-- Name: payment_type; Type: TYPE; Schema: xcentro; Owner: -
--

CREATE TYPE xcentro.payment_type AS ENUM (
    'payment',
    'tokenization',
    'refund'
);


--
-- Name: payout_provider_enum; Type: TYPE; Schema: xcentro; Owner: -
--

CREATE TYPE xcentro.payout_provider_enum AS ENUM (
    'castler',
    'razorpay',
    'bank_api'
);


--
-- Name: payout_transfer_status_enum; Type: TYPE; Schema: xcentro; Owner: -
--

CREATE TYPE xcentro.payout_transfer_status_enum AS ENUM (
    'initiated',
    'processing',
    'completed',
    'failed',
    'timeout'
);


--
-- Name: settlement_status_enum; Type: TYPE; Schema: xcentro; Owner: -
--

CREATE TYPE xcentro.settlement_status_enum AS ENUM (
    'pending',
    'settled',
    'failed',
    'refunded'
);


--
-- Name: subscription_type_enum; Type: TYPE; Schema: xcentro; Owner: -
--

CREATE TYPE xcentro.subscription_type_enum AS ENUM (
    'free_plan',
    'basic_plan',
    'premium_plan',
    'enterprise_plan'
);


--
-- Name: user_role_enum; Type: TYPE; Schema: xcentro; Owner: -
--

CREATE TYPE xcentro.user_role_enum AS ENUM (
    'owner',
    'admin',
    'manager',
    'auditor',
    'none'
);


--
-- Name: verification_status_enum; Type: TYPE; Schema: xcentro; Owner: -
--

CREATE TYPE xcentro.verification_status_enum AS ENUM (
    'pending',
    'in_progress',
    'success',
    'failed'
);


--
-- Name: verification_type; Type: TYPE; Schema: xcentro; Owner: -
--

CREATE TYPE xcentro.verification_type AS ENUM (
    'email_verification',
    'mobile_verification',
    'password_reset'
);


--
-- Name: voucher_status; Type: TYPE; Schema: xcentro; Owner: -
--

CREATE TYPE xcentro.voucher_status AS ENUM (
    'draft',
    'finalized',
    'posted',
    'synced'
);


--
-- Name: TYPE voucher_status; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TYPE xcentro.voucher_status IS 'Enum type for voucher lifecycle states in the payment_vouchers table';


--
-- Name: cleanup_failed_tokenization_cards(); Type: FUNCTION; Schema: xcentro; Owner: -
--

CREATE FUNCTION xcentro.cleanup_failed_tokenization_cards() RETURNS void
    LANGUAGE plpgsql
    AS $$
DECLARE
    deleted_cards_count INT;
    deleted_payments_count INT;
BEGIN
    -- Delete ALL tokenization payments (regardless of status) for failed cards
    -- Rationale: If card tokenization failed, payment status is irrelevant
    --            (e.g., payment may succeed but tokenization server fails)
    -- Must be done BEFORE deleting cards to avoid orphaned references
    WITH deleted_payments AS (
        DELETE FROM xcentro.payments
        WHERE payment_type = 'tokenization'
          AND created_at < NOW() - INTERVAL '2 days'
          AND payment_source_type = 'card'
          AND payment_source_id IN (
              SELECT card_id
              FROM xcentro.stored_cards
              WHERE is_failed_tokenization = TRUE
                AND added_at < NOW() - INTERVAL '2 days'
          )
        RETURNING payment_id
    )
    SELECT COUNT(*) INTO deleted_payments_count FROM deleted_payments;

    -- Delete failed tokenization cards older than 2 days
    WITH deleted_cards AS (
        DELETE FROM xcentro.stored_cards
        WHERE is_failed_tokenization = TRUE
          AND added_at < NOW() - INTERVAL '2 days'
        RETURNING card_id
    )
    SELECT COUNT(*) INTO deleted_cards_count FROM deleted_cards;

    -- Log the cleanup results (will appear in PostgreSQL logs)
    RAISE NOTICE 'Failed card cleanup: deleted % cards and % payments',
                 deleted_cards_count, deleted_payments_count;
END;
$$;


--
-- Name: cleanup_stale_pending_payments(); Type: FUNCTION; Schema: xcentro; Owner: -
--

CREATE FUNCTION xcentro.cleanup_stale_pending_payments() RETURNS void
    LANGUAGE plpgsql
    AS $$
DECLARE
    updated_payments_count INT;
BEGIN
    -- Update pending payments older than 2 hours to failed status
    -- Excludes tokenization payments (handled by separate cleanup job)
    -- Rationale: If callback URL is missed or webhook not received within 2 hours,
    --            payment should be marked as failed to prevent indefinite pending state
    WITH updated_payments AS (
        UPDATE xcentro.payments
        SET
            status = 'failed',
            updated_at = NOW(),
            metadata = COALESCE(metadata, '{}'::jsonb) ||
                      jsonb_build_object(
                          'auto_failed_at', NOW()::text,
                          'auto_failed_reason', 'Payment timed out after 2 hours in pending state',
                          'original_status', 'pending'
                      )
        WHERE status = 'pending'
          AND payment_type = 'payment'  -- Exclude tokenization and refund payments
          AND created_at < NOW() - INTERVAL '2 hours'
        RETURNING payment_id
    )
    SELECT COUNT(*) INTO updated_payments_count FROM updated_payments;

    -- Log the cleanup results (will appear in PostgreSQL logs)
    IF updated_payments_count > 0 THEN
        RAISE NOTICE 'Pending payment cleanup: marked % payment(s) as failed', updated_payments_count;
    END IF;
END;
$$;


--
-- Name: trigger_set_timestamp(); Type: FUNCTION; Schema: xcentro; Owner: -
--

CREATE FUNCTION xcentro.trigger_set_timestamp() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$;


--
-- Name: FUNCTION trigger_set_timestamp(); Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON FUNCTION xcentro.trigger_set_timestamp() IS 'This trigger function automatically sets the updated_at column to the current timestamp whenever a row is updated.';


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: bank_accounts; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.bank_accounts (
    account_id uuid DEFAULT uuidv7() NOT NULL,
    business_id uuid NOT NULL,
    account_number_hash character varying(64) NOT NULL,
    account_number_encrypted text NOT NULL,
    ifsc_code character varying(11) NOT NULL,
    account_holder_name text NOT NULL,
    bank_name text,
    branch_name text,
    account_type character varying(20),
    is_verified boolean DEFAULT false,
    is_active boolean DEFAULT true,
    added_at timestamp with time zone DEFAULT now(),
    metadata jsonb
);


--
-- Name: TABLE bank_accounts; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.bank_accounts IS 'Bank accounts with encrypted account numbers for payment sources';


--
-- Name: COLUMN bank_accounts.account_number_hash; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.bank_accounts.account_number_hash IS 'SHA-256 hash for O(1) duplicate detection';


--
-- Name: COLUMN bank_accounts.account_number_encrypted; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.bank_accounts.account_number_encrypted IS 'AES-256-GCM encrypted account number';


--
-- Name: COLUMN bank_accounts.metadata; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.bank_accounts.metadata IS 'Stores penny drop verification details (status, date, amount, reference)';


--
-- Name: batch_payment_items; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.batch_payment_items (
    item_id uuid DEFAULT uuidv7() NOT NULL,
    batch_id uuid NOT NULL,
    row_number integer NOT NULL,
    status character varying(20) NOT NULL,
    csv_payee_name character varying(255) NOT NULL,
    csv_account_last_4 character varying(4),
    csv_amount bigint NOT NULL,
    payment_context character varying(50) NOT NULL,
    description text,
    expense_category character varying(100),
    gst_amount bigint DEFAULT 0,
    tds_amount bigint DEFAULT 0,
    invoice_number character varying(100),
    matched_relation_id uuid,
    match_confidence numeric(3,2),
    voucher_id uuid,
    error_message text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    processed_at timestamp with time zone,
    CONSTRAINT batch_payment_items_status_check CHECK (((status)::text = ANY ((ARRAY['matched'::character varying, 'ambiguous'::character varying, 'not_found'::character varying, 'pending'::character varying, 'processing'::character varying, 'success'::character varying, 'failed'::character varying, 'skipped'::character varying])::text[])))
);


--
-- Name: batch_payments; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.batch_payments (
    batch_id uuid DEFAULT uuidv7() NOT NULL,
    business_id uuid NOT NULL,
    uploaded_by uuid NOT NULL,
    file_name character varying(255),
    status character varying(20) NOT NULL,
    total_rows integer DEFAULT 0 NOT NULL,
    total_amount bigint DEFAULT 0 NOT NULL,
    processed_rows integer DEFAULT 0 NOT NULL,
    success_rows integer DEFAULT 0 NOT NULL,
    failed_rows integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    processed_at timestamp with time zone,
    completed_at timestamp with time zone,
    CONSTRAINT batch_payments_status_check CHECK (((status)::text = ANY ((ARRAY['pending'::character varying, 'processing'::character varying, 'completed'::character varying, 'failed'::character varying, 'cancelled'::character varying])::text[])))
);


--
-- Name: business; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.business (
    bid uuid DEFAULT uuidv7() NOT NULL,
    name text,
    gstin character varying(15),
    gst_data jsonb,
    income_tax_id text,
    kyb_status xcentro.kyc_status_enum DEFAULT 'kyc_not_started'::xcentro.kyc_status_enum,
    business_type xcentro.business_type_enum,
    business_model text,
    business_turnover text,
    country_code character varying(3),
    currency_code character varying(3),
    updated_at timestamp with time zone,
    has_added_payment_method boolean DEFAULT false NOT NULL,
    has_added_vendor boolean DEFAULT false NOT NULL,
    has_created_first_payment boolean DEFAULT false NOT NULL
);


--
-- Name: TABLE business; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.business IS 'Business entities and their KYB information.';


--
-- Name: business_payee_relation; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.business_payee_relation (
    business_id uuid NOT NULL,
    payee_id uuid NOT NULL,
    account_id uuid NOT NULL,
    display_name character varying(255) NOT NULL,
    contact_email character varying(254),
    contact_phone character varying(20),
    notes text,
    is_active boolean DEFAULT true,
    is_favorite boolean DEFAULT false,
    tags text[],
    type_specific_data jsonb,
    added_at timestamp with time zone DEFAULT now(),
    added_by uuid NOT NULL,
    last_used_at timestamp with time zone,
    updated_at timestamp with time zone,
    metadata jsonb,
    relation_id uuid DEFAULT gen_random_uuid() NOT NULL
);


--
-- Name: TABLE business_payee_relation; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.business_payee_relation IS 'Customer-payee relationships (polymorphic - supports all payee types)';


--
-- Name: COLUMN business_payee_relation.account_id; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.business_payee_relation.account_id IS 'References bank_account, virtual_account, or biller_account based on payee type';


--
-- Name: COLUMN business_payee_relation.display_name; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.business_payee_relation.display_name IS 'Customer-specific name (can differ from legal name)';


--
-- Name: COLUMN business_payee_relation.type_specific_data; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.business_payee_relation.type_specific_data IS 'Type-specific metadata (e.g., GST: tax liability, Biller: customer ID)';


--
-- Name: COLUMN business_payee_relation.added_by; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.business_payee_relation.added_by IS 'User who added this payee relation (required - extracted from JWT)';


--
-- Name: COLUMN business_payee_relation.last_used_at; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.business_payee_relation.last_used_at IS 'Last payment sent to this payee (updated by payment system)';


--
-- Name: COLUMN business_payee_relation.relation_id; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.business_payee_relation.relation_id IS 'Surrogate key for simplified frontend API. Single UUID identifying business-payee-account relationship.';


--
-- Name: business_user_relation; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.business_user_relation (
    user_id uuid NOT NULL,
    bid uuid NOT NULL,
    role xcentro.user_role_enum,
    role_claim xcentro.user_role_enum,
    kyb_initiated boolean DEFAULT false,
    kyc_needed boolean DEFAULT true,
    added_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone
);


--
-- Name: TABLE business_user_relation; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.business_user_relation IS 'Maps users to businesses with specific roles.';


--
-- Name: erp_sync_queue; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.erp_sync_queue (
    queue_id uuid DEFAULT uuidv7() NOT NULL,
    voucher_id uuid NOT NULL,
    sync_direction character varying(20) NOT NULL,
    queue_status character varying(50) DEFAULT 'pending'::character varying NOT NULL,
    retry_count integer DEFAULT 0,
    max_retries integer DEFAULT 3,
    next_retry_at timestamp with time zone,
    request_payload jsonb,
    response_payload jsonb,
    error_code character varying(100),
    error_message text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    CONSTRAINT erp_sync_queue_check CHECK ((retry_count <= max_retries))
);


--
-- Name: TABLE erp_sync_queue; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.erp_sync_queue IS 'Async queue for ERP synchronization workers. Supports retry logic with exponential backoff.';


--
-- Name: feature_flags; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.feature_flags (
    flag_id uuid DEFAULT uuidv7() NOT NULL,
    flag_name character varying(100) NOT NULL,
    flag_value jsonb DEFAULT 'false'::jsonb NOT NULL,
    description text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: TABLE feature_flags; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.feature_flags IS 'Global/universal feature flags for enabling/disabling features across all businesses';


--
-- Name: COLUMN feature_flags.flag_name; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.feature_flags.flag_name IS 'Name of the feature flag (e.g., REWARD_POINT_SYSTEM) - unique across system';


--
-- Name: COLUMN feature_flags.flag_value; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.feature_flags.flag_value IS 'Value of the flag - can be boolean, string, array, or object stored as JSONB';


--
-- Name: kyb_data; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.kyb_data (
    income_tax_id text NOT NULL,
    income_tax_id_data jsonb,
    company_id text,
    company_id_data jsonb,
    business_deed text
);


--
-- Name: TABLE kyb_data; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.kyb_data IS 'Stores Know Your Business (KYB) identity and corporate data.';


--
-- Name: notification_events; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.notification_events (
    id bigint NOT NULL,
    event_type text NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: TABLE notification_events; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.notification_events IS 'Temporary queue of notification events to be fanned out and deleted';


--
-- Name: COLUMN notification_events.payload; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.notification_events.payload IS 'JSON schema: {timestamp (RFC3339), event_type, target, entities, message, actions}';


--
-- Name: notification_events_id_seq; Type: SEQUENCE; Schema: xcentro; Owner: -
--

CREATE SEQUENCE xcentro.notification_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: notification_events_id_seq; Type: SEQUENCE OWNED BY; Schema: xcentro; Owner: -
--

ALTER SEQUENCE xcentro.notification_events_id_seq OWNED BY xcentro.notification_events.id;


--
-- Name: notification_preferences; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.notification_preferences (
    user_id uuid NOT NULL,
    email_enabled boolean DEFAULT true NOT NULL,
    push_enabled boolean DEFAULT true NOT NULL,
    messages_enabled boolean DEFAULT true NOT NULL,
    category_preferences jsonb DEFAULT '{"others": true, "payees": true, "rewards": true, "insights": true, "payments": true, "security": true, "approvals": true, "announcements": true}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone
);


--
-- Name: TABLE notification_preferences; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.notification_preferences IS 'User notification preferences (global, not per-business)';


--
-- Name: COLUMN notification_preferences.email_enabled; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.notification_preferences.email_enabled IS 'Global switch for email notifications';


--
-- Name: COLUMN notification_preferences.push_enabled; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.notification_preferences.push_enabled IS 'Global switch for push notifications';


--
-- Name: COLUMN notification_preferences.messages_enabled; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.notification_preferences.messages_enabled IS 'Global switch for in-app messages/web notifications';


--
-- Name: COLUMN notification_preferences.category_preferences; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.notification_preferences.category_preferences IS 'Category-level preferences: {announcements, rewards, payments, payees, approvals, security, insights, others}';


--
-- Name: ops_session_table; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.ops_session_table (
    session_id uuid DEFAULT uuidv7() NOT NULL,
    ops_user_id uuid NOT NULL,
    refresh_token_hash character varying(255) NOT NULL,
    oidc_refresh_token text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    user_agent text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    last_used_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: TABLE ops_session_table; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.ops_session_table IS 'Stores active sessions for ops dashboard users';


--
-- Name: COLUMN ops_session_table.refresh_token_hash; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.ops_session_table.refresh_token_hash IS 'SHA-256 hash of the refresh token sent to client for session validation';


--
-- Name: COLUMN ops_session_table.oidc_refresh_token; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.ops_session_table.oidc_refresh_token IS 'Encrypted OIDC refresh token used to communicate with OIDC Provider - MUST be encrypted at application level';


--
-- Name: COLUMN ops_session_table.last_used_at; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.ops_session_table.last_used_at IS 'Timestamp of last session activity for tracking active sessions';


--
-- Name: ops_user_table; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.ops_user_table (
    id uuid DEFAULT uuidv7() NOT NULL,
    name character varying(255) NOT NULL,
    email character varying(255) NOT NULL,
    roles jsonb DEFAULT '[]'::jsonb NOT NULL,
    status character varying(20) DEFAULT 'active'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ops_user_table_status_check CHECK (((status)::text = ANY ((ARRAY['active'::character varying, 'inactive'::character varying, 'suspended'::character varying])::text[])))
);


--
-- Name: TABLE ops_user_table; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.ops_user_table IS 'Stores ops dashboard user accounts with OIDC authentication';


--
-- Name: COLUMN ops_user_table.id; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.ops_user_table.id IS 'OID from Entra ID access token (immutable user identifier)';


--
-- Name: COLUMN ops_user_table.email; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.ops_user_table.email IS 'User email from Entra ID (can change, use OID as primary identifier)';


--
-- Name: COLUMN ops_user_table.roles; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.ops_user_table.roles IS 'Array of role strings extracted from OIDC token claims, e.g., ["admin", "viewer"]';


--
-- Name: payee_bank_accounts; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.payee_bank_accounts (
    account_id uuid DEFAULT uuidv7() NOT NULL,
    account_number_encrypted text NOT NULL,
    account_number_hash character varying(64) NOT NULL,
    ifsc_code character varying(11) NOT NULL,
    bank_name character varying(255),
    branch_name character varying(255),
    account_type xcentro.bank_account_type_enum NOT NULL,
    account_holder_name character varying(255) NOT NULL,
    is_verified boolean DEFAULT false,
    verified_at timestamp with time zone,
    bank_verification_status xcentro.verification_status_enum DEFAULT 'pending'::xcentro.verification_status_enum,
    bank_verification_data jsonb,
    cross_verification_status xcentro.verification_status_enum DEFAULT 'pending'::xcentro.verification_status_enum,
    is_active boolean DEFAULT true,
    account_label character varying(100),
    added_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone,
    metadata jsonb,
    castler_beneficiary_id character varying(255)
);


--
-- Name: TABLE payee_bank_accounts; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.payee_bank_accounts IS 'Bank accounts for vendor and employee payees (NOT for GST/Biller)';


--
-- Name: COLUMN payee_bank_accounts.account_number_encrypted; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payee_bank_accounts.account_number_encrypted IS 'Encrypted account number (plaintext in Phase 5.3, encrypted in Phase 5.5)';


--
-- Name: COLUMN payee_bank_accounts.account_number_hash; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payee_bank_accounts.account_number_hash IS 'SHA-256 hash for duplicate detection per payee';


--
-- Name: COLUMN payee_bank_accounts.castler_beneficiary_id; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payee_bank_accounts.castler_beneficiary_id IS 'Castler provider beneficiary ID (result from /api/v1/payee API) - used for payouts';


--
-- Name: payee_biller; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.payee_biller (
    payee_id uuid NOT NULL,
    biller_id character varying(100) NOT NULL,
    biller_name character varying(500) NOT NULL,
    biller_category character varying(100) NOT NULL,
    biller_description text,
    bbps_biller_code character varying(50),
    npci_registered boolean DEFAULT true,
    supports_adhoc boolean DEFAULT true,
    supports_recurring boolean DEFAULT false,
    payment_modes text[],
    avg_processing_time_hours integer,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone,
    metadata jsonb
);


--
-- Name: TABLE payee_biller; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.payee_biller IS 'BBPS system billers (electricity, telecom, water, etc.)';


--
-- Name: COLUMN payee_biller.biller_id; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payee_biller.biller_id IS 'Unique biller ID from BBPS registry';


--
-- Name: COLUMN payee_biller.biller_category; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payee_biller.biller_category IS 'Category: Electricity, Telecom, Water, Gas, etc.';


--
-- Name: payee_biller_accounts; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.payee_biller_accounts (
    biller_account_id uuid DEFAULT uuidv7() NOT NULL,
    payee_id uuid NOT NULL,
    business_id uuid NOT NULL,
    mobile_number character varying(20) NOT NULL,
    customer_params jsonb NOT NULL,
    auto_fetch_enabled boolean DEFAULT false,
    fetch_schedule character varying(50),
    preferred_fetch_day integer,
    last_bill_fetched_at timestamp with time zone,
    last_bill_amount bigint,
    last_bill_due_date date,
    is_active boolean DEFAULT true,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone,
    metadata jsonb
);


--
-- Name: TABLE payee_biller_accounts; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.payee_biller_accounts IS 'Customer-biller configurations (pseudo-account for BBPS payments)';


--
-- Name: COLUMN payee_biller_accounts.mobile_number; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payee_biller_accounts.mobile_number IS 'Registered mobile number at biller';


--
-- Name: COLUMN payee_biller_accounts.customer_params; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payee_biller_accounts.customer_params IS 'JSONB array of biller-specific parameters (e.g., [{"key": "CA_Number", "value": "123"}])';


--
-- Name: payee_employee; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.payee_employee (
    payee_id uuid NOT NULL,
    employee_code character varying(100),
    full_name character varying(500) NOT NULL,
    email character varying(254),
    phone character varying(20),
    designation character varying(200),
    department character varying(200),
    date_of_joining date,
    employment_type character varying(50),
    remuneration_doc_url text,
    remuneration_doc_uploaded_at timestamp with time zone,
    is_sensitive boolean DEFAULT true,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone,
    metadata jsonb,
    remuneration_doc_verified boolean DEFAULT false NOT NULL
);


--
-- Name: TABLE payee_employee; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.payee_employee IS 'Employee payees for payroll (individuals, not businesses)';


--
-- Name: COLUMN payee_employee.remuneration_doc_url; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payee_employee.remuneration_doc_url IS 'S3 URL of remuneration document (versioned at S3 level)';


--
-- Name: COLUMN payee_employee.is_sensitive; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payee_employee.is_sensitive IS 'Always true - employee data is sensitive';


--
-- Name: COLUMN payee_employee.remuneration_doc_verified; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payee_employee.remuneration_doc_verified IS 'Indicates whether the employee remuneration document has been uploaded and verified';


--
-- Name: payee_gst; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.payee_gst (
    payee_id uuid NOT NULL,
    government_entity_name character varying(255) DEFAULT 'Government of India - GST'::character varying NOT NULL,
    gst_portal_url character varying(500) DEFAULT 'https://www.gst.gov.in'::character varying,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone,
    metadata jsonb
);


--
-- Name: TABLE payee_gst; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.payee_gst IS 'Government GST payment entity (singleton - only one row)';


--
-- Name: COLUMN payee_gst.government_entity_name; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payee_gst.government_entity_name IS 'Fixed entity: Government of India';


--
-- Name: payee_gst_virtual_accounts; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.payee_gst_virtual_accounts (
    virtual_account_id uuid DEFAULT uuidv7() NOT NULL,
    payee_id uuid NOT NULL,
    virtual_account_number character varying(50) NOT NULL,
    ifsc_code character varying(11) NOT NULL,
    bank_name character varying(255) DEFAULT 'Government GST Collection Account'::character varying,
    tax_period character varying(20) NOT NULL,
    financial_year character varying(10) NOT NULL,
    tax_type character varying(50),
    valid_from timestamp with time zone NOT NULL,
    valid_until timestamp with time zone NOT NULL,
    is_active boolean DEFAULT true,
    challan_document_url text,
    uploaded_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone,
    metadata jsonb
);


--
-- Name: TABLE payee_gst_virtual_accounts; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.payee_gst_virtual_accounts IS 'Virtual bank accounts for GST payments (generated monthly/quarterly)';


--
-- Name: COLUMN payee_gst_virtual_accounts.virtual_account_number; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payee_gst_virtual_accounts.virtual_account_number IS 'Temporary virtual account number from GST portal';


--
-- Name: COLUMN payee_gst_virtual_accounts.tax_period; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payee_gst_virtual_accounts.tax_period IS 'Tax period (e.g., Jan-2024, Q1-2024)';


--
-- Name: COLUMN payee_gst_virtual_accounts.challan_document_url; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payee_gst_virtual_accounts.challan_document_url IS 'S3 URL of tax challan document containing virtual ID';


--
-- Name: payee_master; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.payee_master (
    payee_id uuid DEFAULT uuidv7() NOT NULL,
    payee_type xcentro.payee_type_enum NOT NULL,
    is_active boolean DEFAULT true,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone
);


--
-- Name: TABLE payee_master; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.payee_master IS 'Master routing table for all payee types (vendor, GST, biller, employee)';


--
-- Name: COLUMN payee_master.payee_type; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payee_master.payee_type IS 'Discriminator for routing to type-specific tables';


--
-- Name: payee_vendor; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.payee_vendor (
    payee_id uuid NOT NULL,
    gstin character varying(15) NOT NULL,
    legal_name character varying(500) NOT NULL,
    business_type character varying(100),
    registration_date date,
    gst_status character varying(50) NOT NULL,
    building_name character varying(255),
    street character varying(255),
    city character varying(100),
    state character varying(100),
    pincode character varying(10),
    gst_verification_status xcentro.verification_status_enum DEFAULT 'pending'::xcentro.verification_status_enum,
    gst_verified_at timestamp with time zone,
    gst_data jsonb,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone,
    metadata jsonb
);


--
-- Name: TABLE payee_vendor; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.payee_vendor IS 'Vendor payees identified by GSTIN (businesses we pay)';


--
-- Name: COLUMN payee_vendor.gstin; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payee_vendor.gstin IS 'Unique 15-character GST identification number';


--
-- Name: COLUMN payee_vendor.gst_data; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payee_vendor.gst_data IS 'Full GST verification API response including directors';


--
-- Name: payment_fee_rules; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.payment_fee_rules (
    rule_id uuid DEFAULT uuidv7() NOT NULL,
    payment_method xcentro.payment_method_enum NOT NULL,
    card_type xcentro.card_type_enum,
    card_category xcentro.card_category_enum,
    fee_type xcentro.fee_type_enum NOT NULL,
    percentage_fee numeric(5,2),
    fixed_fee bigint DEFAULT 0,
    gst_percentage numeric(5,2) DEFAULT 18.00,
    gst_applicable boolean DEFAULT true,
    is_active boolean DEFAULT true,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now()
);


--
-- Name: TABLE payment_fee_rules; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.payment_fee_rules IS 'Simplified fee rules with explicit rules only - no NULL fallback complexity';


--
-- Name: COLUMN payment_fee_rules.card_type; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_fee_rules.card_type IS 'Card type (NULL only for non-card payment methods like net_banking, upi)';


--
-- Name: COLUMN payment_fee_rules.card_category; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_fee_rules.card_category IS 'Card category (NULL only for non-card payment methods)';


--
-- Name: COLUMN payment_fee_rules.fee_type; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_fee_rules.fee_type IS 'Calculation method: percentage, fixed, or hybrid (both)';


--
-- Name: COLUMN payment_fee_rules.percentage_fee; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_fee_rules.percentage_fee IS 'Percentage fee (e.g., 1.90 = 1.9%)';


--
-- Name: COLUMN payment_fee_rules.fixed_fee; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_fee_rules.fixed_fee IS 'Fixed fee in paise (e.g., 2800 = ₹28.00)';


--
-- Name: payment_invoices; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.payment_invoices (
    invoice_id integer NOT NULL,
    payment_id uuid NOT NULL,
    invoice_number character varying(50) NOT NULL,
    gstin_supplier character varying(15) DEFAULT '27AATCM5865F1ZU'::character varying NOT NULL,
    gstin_recipient character varying(15) NOT NULL,
    invoice_date date NOT NULL,
    irn character(64),
    irn_ack_no character varying(20),
    irn_ack_date timestamp with time zone,
    irn_status xcentro.irn_status_enum DEFAULT 'pending'::xcentro.irn_status_enum NOT NULL,
    qr_code text,
    filing_status xcentro.filing_status_enum DEFAULT 'not_filed'::xcentro.filing_status_enum NOT NULL,
    description text DEFAULT 'Convenience Fee'::text NOT NULL,
    sac_code character varying(10) DEFAULT '997159'::character varying NOT NULL,
    taxable_value numeric(12,2) NOT NULL,
    gst_rate numeric(5,2) DEFAULT 18.0 NOT NULL,
    gst_amount numeric(12,2) NOT NULL,
    total_amount numeric(12,2) NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);


--
-- Name: TABLE payment_invoices; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.payment_invoices IS 'GST invoices for successful payments with IRN generation';


--
-- Name: COLUMN payment_invoices.invoice_number; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_invoices.invoice_number IS 'Invoice reference number format: INV-<invoice_id>';


--
-- Name: COLUMN payment_invoices.irn; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_invoices.irn IS 'Invoice Reference Number - SHA256 hash from GSTIN|DocType|DocNumber|FY';


--
-- Name: COLUMN payment_invoices.irn_ack_no; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_invoices.irn_ack_no IS 'Acknowledgement number received from GST IRP portal';


--
-- Name: COLUMN payment_invoices.qr_code; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_invoices.qr_code IS 'Base64 encoded QR code PNG image from IRP';


--
-- Name: COLUMN payment_invoices.taxable_value; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_invoices.taxable_value IS 'Taxable amount (before GST)';


--
-- Name: COLUMN payment_invoices.gst_amount; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_invoices.gst_amount IS 'GST amount (taxable_value * gst_rate / 100)';


--
-- Name: COLUMN payment_invoices.total_amount; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_invoices.total_amount IS 'Total invoice amount (taxable_value + gst_amount)';


--
-- Name: payment_invoices_invoice_id_seq; Type: SEQUENCE; Schema: xcentro; Owner: -
--

CREATE SEQUENCE xcentro.payment_invoices_invoice_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: payment_invoices_invoice_id_seq; Type: SEQUENCE OWNED BY; Schema: xcentro; Owner: -
--

ALTER SEQUENCE xcentro.payment_invoices_invoice_id_seq OWNED BY xcentro.payment_invoices.invoice_id;


--
-- Name: payment_state_changes; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.payment_state_changes (
    state_change_id uuid DEFAULT uuidv7() NOT NULL,
    payment_id uuid NOT NULL,
    transfer_request_id uuid,
    action xcentro.payment_action_enum NOT NULL,
    action_timestamp timestamp with time zone DEFAULT now() NOT NULL,
    actor_type xcentro.payment_actor_enum DEFAULT 'system'::xcentro.payment_actor_enum NOT NULL,
    actor_user_id uuid,
    actor_name character varying(100),
    previous_state character varying(50),
    new_state character varying(50),
    amount bigint,
    currency character varying(3) DEFAULT 'INR'::character varying,
    utr character varying(100),
    provider character varying(50),
    metadata jsonb,
    remarks text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: TABLE payment_state_changes; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.payment_state_changes IS 'Audit trail for payment and payout state transitions. Tracks who initiated payments and when funds settled to payee accounts. Uses payment_id as primary relationship to avoid duplicates in multi-bill payments.';


--
-- Name: COLUMN payment_state_changes.payment_id; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_state_changes.payment_id IS 'Payment record (multiple vouchers can share same payment in multi-bill scenarios)';


--
-- Name: COLUMN payment_state_changes.transfer_request_id; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_state_changes.transfer_request_id IS 'Payout transfer request (nullable for payment-only events)';


--
-- Name: COLUMN payment_state_changes.action; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_state_changes.action IS 'Type of state change event';


--
-- Name: COLUMN payment_state_changes.actor_type; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_state_changes.actor_type IS 'Who/what triggered this change (user, system, webhook, api)';


--
-- Name: COLUMN payment_state_changes.actor_user_id; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_state_changes.actor_user_id IS 'User who initiated action (if actor_type = user)';


--
-- Name: COLUMN payment_state_changes.actor_name; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_state_changes.actor_name IS 'Denormalized actor name for system/webhook actors';


--
-- Name: COLUMN payment_state_changes.utr; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_state_changes.utr IS 'Bank UTR from payout completion webhook';


--
-- Name: COLUMN payment_state_changes.metadata; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_state_changes.metadata IS 'Additional context: gateway responses, error codes, webhook payloads';


--
-- Name: payment_voucher_pending; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.payment_voucher_pending (
    payment_id uuid NOT NULL,
    voucher_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: TABLE payment_voucher_pending; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.payment_voucher_pending IS 'Temporary table storing voucher-payment relationships during payment initiation. Records are deleted when payment succeeds or can be cleaned up if payment is abandoned.';


--
-- Name: COLUMN payment_voucher_pending.payment_id; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_voucher_pending.payment_id IS 'Payment ID that was initiated with these vouchers';


--
-- Name: COLUMN payment_voucher_pending.voucher_id; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_voucher_pending.voucher_id IS 'Voucher ID that is part of this payment';


--
-- Name: COLUMN payment_voucher_pending.created_at; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_voucher_pending.created_at IS 'Timestamp when this pending relationship was created (used for cleanup of abandoned payments)';


--
-- Name: payment_vouchers; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.payment_vouchers (
    voucher_id uuid DEFAULT uuidv7() NOT NULL,
    voucher_number character varying(50) NOT NULL,
    voucher_date date DEFAULT CURRENT_DATE NOT NULL,
    fiscal_year character varying(10) NOT NULL,
    accounting_period character varying(7) NOT NULL,
    voucher_type character varying(50) DEFAULT 'payment'::character varying NOT NULL,
    business_id uuid NOT NULL,
    payee_id uuid NOT NULL,
    account_id uuid NOT NULL,
    invoice_number character varying(100),
    invoice_date date,
    invoice_due_date date,
    gross_amount bigint NOT NULL,
    net_amount bigint NOT NULL,
    currency character varying(3) DEFAULT 'INR'::character varying NOT NULL,
    gst_amount bigint DEFAULT 0,
    gst_rate numeric(5,2),
    cgst_amount bigint DEFAULT 0,
    sgst_amount bigint DEFAULT 0,
    igst_amount bigint DEFAULT 0,
    tds_amount bigint DEFAULT 0,
    tds_rate numeric(5,2),
    tds_section character varying(20),
    fee_amount bigint DEFAULT 0,
    discount_amount bigint DEFAULT 0,
    financial_details jsonb,
    attachments jsonb,
    expense_category character varying(100),
    description text,
    payment_id uuid,
    erp_sync_status character varying(50) DEFAULT 'not_synced'::character varying NOT NULL,
    erp_reference_id character varying(255),
    erp_posting_date date,
    erp_last_sync_at timestamp with time zone,
    erp_sync_attempts integer DEFAULT 0,
    gl_expense_account character varying(50),
    gl_bank_account character varying(50),
    cost_center character varying(50),
    erp_metadata jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    created_by_user_id uuid,
    updated_at timestamp with time zone,
    metadata jsonb,
    payment_terms character varying(50),
    status xcentro.voucher_status DEFAULT 'draft'::xcentro.voucher_status NOT NULL
);


--
-- Name: TABLE payment_vouchers; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.payment_vouchers IS 'Payment vouchers serve as the accounting document. All payout tracking moved to payout_transfer_requests table.';


--
-- Name: COLUMN payment_vouchers.voucher_number; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_vouchers.voucher_number IS 'Unique voucher identifier in format PV-YYYY-NNNNN. Auto-generated based on fiscal year sequence.';


--
-- Name: COLUMN payment_vouchers.financial_details; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_vouchers.financial_details IS 'JSONB field for flexible financial data: line items, HSN codes, tax breakdowns, custom fields';


--
-- Name: COLUMN payment_vouchers.erp_sync_status; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_vouchers.erp_sync_status IS 'ERP synchronization state: not_synced → pending (in queue) → synced (success) → failed (retry needed)';


--
-- Name: COLUMN payment_vouchers.erp_metadata; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_vouchers.erp_metadata IS 'Vendor-specific ERP fields (Zoho Books, Tally, SAP, etc.) stored as JSONB for flexibility';


--
-- Name: COLUMN payment_vouchers.payment_terms; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_vouchers.payment_terms IS 'Payment terms from invoice (e.g., "Net 30", "Net 45", "Due on Receipt", "2/10 Net 30")';


--
-- Name: COLUMN payment_vouchers.status; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payment_vouchers.status IS 'Voucher lifecycle status (ENUM): draft (created on Page 1→2 transition) → finalized (after payment) → posted (accounting complete) → synced (ERP synchronized). This is the voucher state, not approval or payment status.';


--
-- Name: payments; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.payments (
    payment_id uuid NOT NULL,
    user_id uuid,
    business_id uuid NOT NULL,
    amount bigint NOT NULL,
    currency xcentro.currency_code_enum DEFAULT 'INR'::xcentro.currency_code_enum NOT NULL,
    payment_method xcentro.payment_method_enum NOT NULL,
    status xcentro.payment_status_enum DEFAULT 'pending'::xcentro.payment_status_enum NOT NULL,
    description text,
    gateway_used xcentro.gateway_type_enum,
    gateway_payment_id text,
    callback_url text,
    redirect_url text,
    payment_source_id uuid,
    payment_source_type xcentro.payment_source_type_enum,
    fee_amount bigint DEFAULT 0,
    total_amount bigint NOT NULL,
    metadata jsonb,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone,
    settlement_status xcentro.settlement_status_enum DEFAULT 'pending'::xcentro.settlement_status_enum,
    settled_at timestamp with time zone,
    settlement_reference_id character varying(255),
    settlement_utr character varying(100),
    settlement_metadata jsonb,
    settlement_amount bigint,
    mdr_amount bigint DEFAULT 0,
    net_settlement_amount bigint,
    payment_type xcentro.payment_type DEFAULT 'payment'::xcentro.payment_type NOT NULL,
    retry_of_payment_id uuid,
    retry_count integer DEFAULT 0 NOT NULL,
    convenience_fee bigint DEFAULT 0,
    convenience_fee_gst bigint DEFAULT 0,
    CONSTRAINT chk_net_settlement_calculation CHECK (((net_settlement_amount IS NULL) OR (settlement_amount IS NULL) OR (net_settlement_amount = (settlement_amount - mdr_amount)))),
    CONSTRAINT chk_settlement_amount_valid CHECK (((settlement_amount IS NULL) OR (settlement_amount <= total_amount))),
    CONSTRAINT chk_token_card_requires_source CHECK (((payment_method <> 'token_card'::xcentro.payment_method_enum) OR ((payment_source_id IS NOT NULL) AND (payment_source_type IS NOT NULL)))),
    CONSTRAINT payments_amount_check CHECK ((amount > 0)),
    CONSTRAINT payments_check CHECK ((total_amount >= amount)),
    CONSTRAINT payments_convenience_fee_check CHECK ((convenience_fee >= 0)),
    CONSTRAINT payments_convenience_fee_gst_check CHECK ((convenience_fee_gst >= 0)),
    CONSTRAINT payments_fee_amount_check CHECK ((fee_amount >= 0)),
    CONSTRAINT payments_mdr_amount_check CHECK ((mdr_amount >= 0)),
    CONSTRAINT payments_net_settlement_amount_check CHECK ((net_settlement_amount >= 0)),
    CONSTRAINT payments_settlement_amount_check CHECK ((settlement_amount >= 0))
);


--
-- Name: TABLE payments; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.payments IS 'Main payment transaction records with gateway routing, fee tracking, and settlement reconciliation';


--
-- Name: COLUMN payments.payment_id; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payments.payment_id IS 'Application-generated UUID (must be provided by application layer)';


--
-- Name: COLUMN payments.amount; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payments.amount IS 'Payment amount in smallest currency unit (paise for INR)';


--
-- Name: COLUMN payments.gateway_payment_id; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payments.gateway_payment_id IS 'Gateway transaction ID for webhook payment lookup';


--
-- Name: COLUMN payments.payment_source_id; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payments.payment_source_id IS 'Polymorphic reference to stored_cards.card_id or bank_accounts.account_id';


--
-- Name: COLUMN payments.payment_source_type; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payments.payment_source_type IS 'Discriminator for payment_source_id polymorphic relationship';


--
-- Name: COLUMN payments.fee_amount; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payments.fee_amount IS 'Total fee amount (convenience_fee + convenience_fee_gst) - kept for backward compatibility';


--
-- Name: COLUMN payments.metadata; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payments.metadata IS 'Stores gateway responses, payment attempts, failure reasons';


--
-- Name: COLUMN payments.settlement_status; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payments.settlement_status IS 'Settlement lifecycle: pending (T+0) → settled (T+1 to T+2) → ready for payout';


--
-- Name: COLUMN payments.settled_at; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payments.settled_at IS 'Timestamp when payment gateway funds actually settled into XCentro escrow account';


--
-- Name: COLUMN payments.settlement_utr; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payments.settlement_utr IS 'Bank UTR for escrow deposit (from gateway settlement report or webhook)';


--
-- Name: COLUMN payments.settlement_amount; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payments.settlement_amount IS 'Actual amount settled by gateway (may differ from total_amount due to chargebacks/adjustments)';


--
-- Name: COLUMN payments.mdr_amount; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payments.mdr_amount IS 'Merchant Discount Rate - gateway fees deducted from settlement (MDR %)';


--
-- Name: COLUMN payments.net_settlement_amount; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payments.net_settlement_amount IS 'Final amount received after all deductions: settlement_amount - mdr_amount';


--
-- Name: COLUMN payments.payment_type; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payments.payment_type IS 'Type of payment: payment (regular), tokenization (₹1 card tokenization), refund (refund transaction)';


--
-- Name: COLUMN payments.retry_of_payment_id; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payments.retry_of_payment_id IS 'References the original payment that this payment is retrying. NULL for original payments.';


--
-- Name: COLUMN payments.retry_count; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payments.retry_count IS 'Number of times this payment has been retried. 0 for first attempt.';


--
-- Name: COLUMN payments.convenience_fee; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payments.convenience_fee IS 'Base convenience fee before GST (in paise)';


--
-- Name: COLUMN payments.convenience_fee_gst; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payments.convenience_fee_gst IS 'GST on convenience fee - 18% (in paise)';


--
-- Name: CONSTRAINT chk_token_card_requires_source ON payments; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON CONSTRAINT chk_token_card_requires_source ON xcentro.payments IS 'Ensures tokenized card payments always have payment_source_id and payment_source_type set for proper tracking and reporting';


--
-- Name: payout_transfer_requests; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.payout_transfer_requests (
    transfer_request_id uuid DEFAULT uuidv7() NOT NULL,
    voucher_id uuid NOT NULL,
    provider character varying(50) NOT NULL,
    provider_beneficiary_id character varying(255) NOT NULL,
    provider_transfer_id character varying(255),
    amount bigint NOT NULL,
    currency character varying(3) DEFAULT 'INR'::character varying NOT NULL,
    purpose character varying(255),
    reference character varying(255),
    narration text,
    transfer_mode character varying(50),
    status xcentro.payout_transfer_status_enum DEFAULT 'initiated'::xcentro.payout_transfer_status_enum NOT NULL,
    initiated_at timestamp with time zone,
    processing_at timestamp with time zone,
    completed_at timestamp with time zone,
    failed_at timestamp with time zone,
    failure_reason text,
    utr character varying(100),
    retry_count integer DEFAULT 0 NOT NULL,
    max_retries integer DEFAULT 3 NOT NULL,
    next_retry_at timestamp with time zone,
    last_retry_at timestamp with time zone,
    last_webhook_at timestamp with time zone,
    webhook_timeout_at timestamp with time zone,
    webhook_history jsonb[] DEFAULT ARRAY[]::jsonb[],
    provider_metadata jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now(),
    payment_id uuid NOT NULL,
    escrow_id character varying(255),
    is_deleted boolean DEFAULT false NOT NULL,
    deleted_at timestamp with time zone,
    deleted_reason text,
    CONSTRAINT check_amount_positive CHECK ((amount > 0))
);


--
-- Name: TABLE payout_transfer_requests; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.payout_transfer_requests IS 'Payout transfer requests with soft delete support for retry mechanism. Each voucher can have multiple attempts (only one active). Failed/timeout transfers are soft deleted when retry is initiated.';


--
-- Name: COLUMN payout_transfer_requests.status; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payout_transfer_requests.status IS 'Transfer status: initiated (sent to Castler) → processing (Castler webhook) → completed (with UTR) / failed / timeout';


--
-- Name: COLUMN payout_transfer_requests.retry_count; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payout_transfer_requests.retry_count IS 'Number of retry attempts (placeholder for future retry strategy)';


--
-- Name: COLUMN payout_transfer_requests.webhook_timeout_at; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payout_transfer_requests.webhook_timeout_at IS 'Timestamp when transfer will be flagged as timeout if no webhook received';


--
-- Name: COLUMN payout_transfer_requests.webhook_history; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payout_transfer_requests.webhook_history IS 'Array of all webhook payloads received from provider';


--
-- Name: COLUMN payout_transfer_requests.payment_id; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payout_transfer_requests.payment_id IS 'Direct reference to the payment that triggered this payout';


--
-- Name: COLUMN payout_transfer_requests.escrow_id; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payout_transfer_requests.escrow_id IS 'Castler escrow account ID (source account for transfer)';


--
-- Name: COLUMN payout_transfer_requests.is_deleted; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payout_transfer_requests.is_deleted IS 'Soft delete flag for retry mechanism. When retry is initiated, old failed/timeout attempts are marked deleted and new attempt is created.';


--
-- Name: COLUMN payout_transfer_requests.deleted_at; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payout_transfer_requests.deleted_at IS 'Timestamp when this transfer was soft deleted (marked as obsolete by a retry or manual action).';


--
-- Name: COLUMN payout_transfer_requests.deleted_reason; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.payout_transfer_requests.deleted_reason IS 'Reason for soft deletion: "retry" (user initiated retry), "superseded" (new attempt created), "manual_cleanup" (ops action), etc.';


--
-- Name: schema_migrations; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.schema_migrations (
    version bigint NOT NULL,
    dirty boolean NOT NULL
);


--
-- Name: stored_cards; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.stored_cards (
    card_id uuid DEFAULT uuidv7() NOT NULL,
    business_id uuid NOT NULL,
    token text NOT NULL,
    last_four_digits character varying(4) NOT NULL,
    card_type xcentro.card_type_enum NOT NULL,
    card_category xcentro.card_category_enum NOT NULL,
    expiry_month character varying(2) NOT NULL,
    expiry_year character varying(4) NOT NULL,
    issuer_bank text,
    card_holder_name text,
    is_active boolean DEFAULT true,
    gateway_used xcentro.gateway_type_enum,
    added_at timestamp with time zone DEFAULT now(),
    last_used_at timestamp with time zone,
    metadata jsonb,
    failure_reason text,
    is_failed_tokenization boolean DEFAULT false NOT NULL
);


--
-- Name: TABLE stored_cards; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.stored_cards IS 'Tokenized card storage for recurring payments (PCI DSS compliant). Uniqueness enforced by card_id PRIMARY KEY.';


--
-- Name: COLUMN stored_cards.token; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.stored_cards.token IS 'Gateway-provided token - NOT actual card number';


--
-- Name: COLUMN stored_cards.last_four_digits; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.stored_cards.last_four_digits IS 'Last 4 digits for display purposes only';


--
-- Name: COLUMN stored_cards.gateway_used; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.stored_cards.gateway_used IS 'Gateway that tokenized this card. NULL during Phase 1 (pending tokenization), set in Phase 2 after gateway selection.';


--
-- Name: COLUMN stored_cards.failure_reason; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.stored_cards.failure_reason IS 'Reason for tokenization failure (populated when is_active = false)';


--
-- Name: COLUMN stored_cards.is_failed_tokenization; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.stored_cards.is_failed_tokenization IS 'True if tokenization payment failed (vs user deactivation). Failed cards shown to user for 7 days.';


--
-- Name: subscription_status; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.subscription_status (
    bid uuid NOT NULL,
    subscription_type xcentro.subscription_type_enum NOT NULL,
    start_date date,
    expiry_date date
);


--
-- Name: TABLE subscription_status; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.subscription_status IS 'Tracks the subscription status of businesses.';


--
-- Name: user_device_versions; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.user_device_versions (
    user_id uuid NOT NULL,
    device_type xcentro.device_type_enum NOT NULL,
    current_version integer DEFAULT 1 NOT NULL,
    updated_at timestamp with time zone DEFAULT now()
);


--
-- Name: TABLE user_device_versions; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.user_device_versions IS 'Tracks JWT version number per user per device type for session invalidation';


--
-- Name: COLUMN user_device_versions.current_version; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.user_device_versions.current_version IS 'Incremental version number - incremented when max sessions reached for device type';


--
-- Name: user_identity; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.user_identity (
    tax_id text NOT NULL,
    tax_id_data jsonb,
    national_id text,
    national_id_data jsonb,
    n_txn_id text,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone
);


--
-- Name: TABLE user_identity; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.user_identity IS 'Externalized personal identity & compliance artifacts (tax_id, national_id, with associated metadata).';


--
-- Name: user_notifications; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.user_notifications (
    id bigint NOT NULL,
    user_id uuid NOT NULL,
    business_id uuid,
    event_id bigint NOT NULL,
    channel text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    content jsonb NOT NULL,
    retry_count integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    sent_at timestamp with time zone,
    read_at timestamp with time zone
);


--
-- Name: TABLE user_notifications; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.user_notifications IS 'Persistent notification inbox for each user';


--
-- Name: COLUMN user_notifications.channel; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.user_notifications.channel IS 'Delivery channel: email, push, web';


--
-- Name: COLUMN user_notifications.status; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.user_notifications.status IS 'Lifecycle: pending -> processing -> sent/failed (with retries) -> read (web only)';


--
-- Name: COLUMN user_notifications.content; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.user_notifications.content IS 'Rendered message: {title, body, actions, entities}';


--
-- Name: COLUMN user_notifications.retry_count; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.user_notifications.retry_count IS 'Number of retry attempts for failed jobs (auto-incremented on retry)';


--
-- Name: user_notifications_id_seq; Type: SEQUENCE; Schema: xcentro; Owner: -
--

CREATE SEQUENCE xcentro.user_notifications_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_notifications_id_seq; Type: SEQUENCE OWNED BY; Schema: xcentro; Owner: -
--

ALTER SEQUENCE xcentro.user_notifications_id_seq OWNED BY xcentro.user_notifications.id;


--
-- Name: user_table; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.user_table (
    id uuid DEFAULT uuidv7() NOT NULL,
    name text,
    dob date,
    email character varying(254) NOT NULL,
    pwd_hash text NOT NULL,
    mobile character varying(20),
    tax_id text,
    kyc_status xcentro.kyc_status_enum DEFAULT 'kyc_not_started'::xcentro.kyc_status_enum NOT NULL,
    kyc_needed boolean DEFAULT true,
    email_verified boolean DEFAULT false,
    mobile_verified boolean DEFAULT false,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone
);


--
-- Name: TABLE user_table; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.user_table IS 'User accounts, authentication, and KYC data.';


--
-- Name: COLUMN user_table.name; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.user_table.name IS 'User full name collected during onboarding';


--
-- Name: COLUMN user_table.dob; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.user_table.dob IS 'User date of birth collected during onboarding';


--
-- Name: COLUMN user_table.kyc_needed; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.user_table.kyc_needed IS 'Whether KYC is required for this user (default TRUE for new users, can be set to FALSE for invited users based on business requirements)';


--
-- Name: user_tokens; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.user_tokens (
    session_id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    refresh_token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    last_used_at timestamp with time zone DEFAULT now(),
    user_agent text,
    device_type xcentro.device_type_enum DEFAULT 'web'::xcentro.device_type_enum NOT NULL,
    device_version integer DEFAULT 1 NOT NULL
);


--
-- Name: COLUMN user_tokens.device_type; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.user_tokens.device_type IS 'Device type for this session (web/mobile)';


--
-- Name: COLUMN user_tokens.device_version; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.user_tokens.device_version IS 'JWT version number at time of session creation - used for bulk invalidation';


--
-- Name: user_verification_codes; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.user_verification_codes (
    id uuid DEFAULT uuidv7() NOT NULL,
    user_id uuid,
    code text,
    code_type xcentro.verification_type,
    expires_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now(),
    used boolean DEFAULT false,
    retry_attempts integer DEFAULT 0
);


--
-- Name: voucher_approvals; Type: TABLE; Schema: xcentro; Owner: -
--

CREATE TABLE xcentro.voucher_approvals (
    approval_id uuid DEFAULT uuidv7() NOT NULL,
    voucher_id uuid NOT NULL,
    requester_user_id uuid NOT NULL,
    approver_user_id uuid NOT NULL,
    action xcentro.approval_action_enum DEFAULT 'pending'::xcentro.approval_action_enum NOT NULL,
    remarks text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: TABLE voucher_approvals; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON TABLE xcentro.voucher_approvals IS 'Approval workflow audit log. Each row represents an approval action/event. Query latest status with ORDER BY created_at DESC.';


--
-- Name: COLUMN voucher_approvals.requester_user_id; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.voucher_approvals.requester_user_id IS 'User who initiated the payment requiring approval';


--
-- Name: COLUMN voucher_approvals.approver_user_id; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.voucher_approvals.approver_user_id IS 'User who approved/rejected the payment (role fetched from user table)';


--
-- Name: COLUMN voucher_approvals.action; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.voucher_approvals.action IS 'Approval action taken: pending (requested), approved, rejected, request_changes. Each action creates a new row.';


--
-- Name: COLUMN voucher_approvals.remarks; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.voucher_approvals.remarks IS 'Approver notes explaining their decision';


--
-- Name: COLUMN voucher_approvals.created_at; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON COLUMN xcentro.voucher_approvals.created_at IS 'When this approval action was taken (chronological order for audit trail)';


--
-- Name: notification_events id; Type: DEFAULT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.notification_events ALTER COLUMN id SET DEFAULT nextval('xcentro.notification_events_id_seq'::regclass);


--
-- Name: payment_invoices invoice_id; Type: DEFAULT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payment_invoices ALTER COLUMN invoice_id SET DEFAULT nextval('xcentro.payment_invoices_invoice_id_seq'::regclass);


--
-- Name: user_notifications id; Type: DEFAULT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.user_notifications ALTER COLUMN id SET DEFAULT nextval('xcentro.user_notifications_id_seq'::regclass);


--
-- Name: bank_accounts bank_accounts_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.bank_accounts
    ADD CONSTRAINT bank_accounts_pkey PRIMARY KEY (account_id);


--
-- Name: batch_payment_items batch_payment_items_batch_id_row_number_key; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.batch_payment_items
    ADD CONSTRAINT batch_payment_items_batch_id_row_number_key UNIQUE (batch_id, row_number);


--
-- Name: batch_payment_items batch_payment_items_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.batch_payment_items
    ADD CONSTRAINT batch_payment_items_pkey PRIMARY KEY (item_id);


--
-- Name: batch_payments batch_payments_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.batch_payments
    ADD CONSTRAINT batch_payments_pkey PRIMARY KEY (batch_id);


--
-- Name: business business_gstin_key; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.business
    ADD CONSTRAINT business_gstin_key UNIQUE (gstin);


--
-- Name: business_payee_relation business_payee_relation_business_payee_account_key; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.business_payee_relation
    ADD CONSTRAINT business_payee_relation_business_payee_account_key UNIQUE (business_id, payee_id, account_id);


--
-- Name: business_payee_relation business_payee_relation_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.business_payee_relation
    ADD CONSTRAINT business_payee_relation_pkey PRIMARY KEY (relation_id);


--
-- Name: CONSTRAINT business_payee_relation_pkey ON business_payee_relation; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON CONSTRAINT business_payee_relation_pkey ON xcentro.business_payee_relation IS 'Surrogate key (relation_id) as primary key for performance. Natural composite key enforced via UNIQUE constraint.';


--
-- Name: business business_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.business
    ADD CONSTRAINT business_pkey PRIMARY KEY (bid);


--
-- Name: business_user_relation business_user_relation_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.business_user_relation
    ADD CONSTRAINT business_user_relation_pkey PRIMARY KEY (user_id, bid);


--
-- Name: erp_sync_queue erp_sync_queue_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.erp_sync_queue
    ADD CONSTRAINT erp_sync_queue_pkey PRIMARY KEY (queue_id);


--
-- Name: feature_flags feature_flags_flag_name_key; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.feature_flags
    ADD CONSTRAINT feature_flags_flag_name_key UNIQUE (flag_name);


--
-- Name: feature_flags feature_flags_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.feature_flags
    ADD CONSTRAINT feature_flags_pkey PRIMARY KEY (flag_id);


--
-- Name: kyb_data kyb_data_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.kyb_data
    ADD CONSTRAINT kyb_data_pkey PRIMARY KEY (income_tax_id);


--
-- Name: notification_events notification_events_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.notification_events
    ADD CONSTRAINT notification_events_pkey PRIMARY KEY (id);


--
-- Name: notification_preferences notification_preferences_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.notification_preferences
    ADD CONSTRAINT notification_preferences_pkey PRIMARY KEY (user_id);


--
-- Name: ops_session_table ops_session_table_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.ops_session_table
    ADD CONSTRAINT ops_session_table_pkey PRIMARY KEY (session_id);


--
-- Name: ops_user_table ops_user_table_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.ops_user_table
    ADD CONSTRAINT ops_user_table_pkey PRIMARY KEY (id);


--
-- Name: payee_bank_accounts payee_bank_accounts_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payee_bank_accounts
    ADD CONSTRAINT payee_bank_accounts_pkey PRIMARY KEY (account_id);


--
-- Name: payee_biller_accounts payee_biller_accounts_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payee_biller_accounts
    ADD CONSTRAINT payee_biller_accounts_pkey PRIMARY KEY (biller_account_id);


--
-- Name: payee_biller payee_biller_biller_id_key; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payee_biller
    ADD CONSTRAINT payee_biller_biller_id_key UNIQUE (biller_id);


--
-- Name: payee_biller payee_biller_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payee_biller
    ADD CONSTRAINT payee_biller_pkey PRIMARY KEY (payee_id);


--
-- Name: payee_employee payee_employee_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payee_employee
    ADD CONSTRAINT payee_employee_pkey PRIMARY KEY (payee_id);


--
-- Name: payee_gst payee_gst_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payee_gst
    ADD CONSTRAINT payee_gst_pkey PRIMARY KEY (payee_id);


--
-- Name: payee_gst_virtual_accounts payee_gst_virtual_accounts_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payee_gst_virtual_accounts
    ADD CONSTRAINT payee_gst_virtual_accounts_pkey PRIMARY KEY (virtual_account_id);


--
-- Name: payee_gst_virtual_accounts payee_gst_virtual_accounts_virtual_account_number_key; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payee_gst_virtual_accounts
    ADD CONSTRAINT payee_gst_virtual_accounts_virtual_account_number_key UNIQUE (virtual_account_number);


--
-- Name: payee_master payee_master_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payee_master
    ADD CONSTRAINT payee_master_pkey PRIMARY KEY (payee_id);


--
-- Name: payee_vendor payee_vendor_gstin_key; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payee_vendor
    ADD CONSTRAINT payee_vendor_gstin_key UNIQUE (gstin);


--
-- Name: payee_vendor payee_vendor_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payee_vendor
    ADD CONSTRAINT payee_vendor_pkey PRIMARY KEY (payee_id);


--
-- Name: payment_fee_rules payment_fee_rules_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payment_fee_rules
    ADD CONSTRAINT payment_fee_rules_pkey PRIMARY KEY (rule_id);


--
-- Name: payment_invoices payment_invoices_irn_key; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payment_invoices
    ADD CONSTRAINT payment_invoices_irn_key UNIQUE (irn);


--
-- Name: payment_invoices payment_invoices_payment_id_key; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payment_invoices
    ADD CONSTRAINT payment_invoices_payment_id_key UNIQUE (payment_id);


--
-- Name: payment_invoices payment_invoices_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payment_invoices
    ADD CONSTRAINT payment_invoices_pkey PRIMARY KEY (invoice_id);


--
-- Name: payment_state_changes payment_state_changes_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payment_state_changes
    ADD CONSTRAINT payment_state_changes_pkey PRIMARY KEY (state_change_id);


--
-- Name: payment_voucher_pending payment_voucher_pending_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payment_voucher_pending
    ADD CONSTRAINT payment_voucher_pending_pkey PRIMARY KEY (payment_id, voucher_id);


--
-- Name: payment_vouchers payment_vouchers_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payment_vouchers
    ADD CONSTRAINT payment_vouchers_pkey PRIMARY KEY (voucher_id);


--
-- Name: payment_vouchers payment_vouchers_voucher_number_key; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payment_vouchers
    ADD CONSTRAINT payment_vouchers_voucher_number_key UNIQUE (voucher_number);


--
-- Name: payments payments_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payments
    ADD CONSTRAINT payments_pkey PRIMARY KEY (payment_id);


--
-- Name: payout_transfer_requests payout_transfer_requests_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payout_transfer_requests
    ADD CONSTRAINT payout_transfer_requests_pkey PRIMARY KEY (transfer_request_id);


--
-- Name: schema_migrations schema_migrations_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.schema_migrations
    ADD CONSTRAINT schema_migrations_pkey PRIMARY KEY (version);


--
-- Name: stored_cards stored_cards_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.stored_cards
    ADD CONSTRAINT stored_cards_pkey PRIMARY KEY (card_id);


--
-- Name: subscription_status subscription_status_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.subscription_status
    ADD CONSTRAINT subscription_status_pkey PRIMARY KEY (bid);


--
-- Name: payee_bank_accounts unique_account_hash; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payee_bank_accounts
    ADD CONSTRAINT unique_account_hash UNIQUE (account_number_hash);


--
-- Name: payee_biller_accounts unique_business_biller; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payee_biller_accounts
    ADD CONSTRAINT unique_business_biller UNIQUE (business_id, payee_id, mobile_number);


--
-- Name: user_notifications uq_user_notifications_event; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.user_notifications
    ADD CONSTRAINT uq_user_notifications_event UNIQUE (event_id, user_id, channel);


--
-- Name: user_device_versions user_device_versions_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.user_device_versions
    ADD CONSTRAINT user_device_versions_pkey PRIMARY KEY (user_id, device_type);


--
-- Name: user_identity user_identity_national_id_key; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.user_identity
    ADD CONSTRAINT user_identity_national_id_key UNIQUE (national_id);


--
-- Name: user_identity user_identity_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.user_identity
    ADD CONSTRAINT user_identity_pkey PRIMARY KEY (tax_id);


--
-- Name: user_notifications user_notifications_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.user_notifications
    ADD CONSTRAINT user_notifications_pkey PRIMARY KEY (id);


--
-- Name: user_table user_table_email_key; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.user_table
    ADD CONSTRAINT user_table_email_key UNIQUE (email);


--
-- Name: user_table user_table_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.user_table
    ADD CONSTRAINT user_table_pkey PRIMARY KEY (id);


--
-- Name: user_tokens user_tokens_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.user_tokens
    ADD CONSTRAINT user_tokens_pkey PRIMARY KEY (session_id);


--
-- Name: user_verification_codes user_verification_codes_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.user_verification_codes
    ADD CONSTRAINT user_verification_codes_pkey PRIMARY KEY (id);


--
-- Name: voucher_approvals voucher_approvals_pkey; Type: CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.voucher_approvals
    ADD CONSTRAINT voucher_approvals_pkey PRIMARY KEY (approval_id);


--
-- Name: idx_bank_accounts_castler_onboarded; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_bank_accounts_castler_onboarded ON xcentro.payee_bank_accounts USING btree (castler_beneficiary_id) WHERE (castler_beneficiary_id IS NOT NULL);


--
-- Name: idx_bank_accounts_unique; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE UNIQUE INDEX idx_bank_accounts_unique ON xcentro.bank_accounts USING btree (business_id, account_number_hash);


--
-- Name: idx_batch_payment_items_batch_id; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_batch_payment_items_batch_id ON xcentro.batch_payment_items USING btree (batch_id);


--
-- Name: idx_batch_payment_items_status; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_batch_payment_items_status ON xcentro.batch_payment_items USING btree (status);


--
-- Name: idx_batch_payment_items_voucher_id; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_batch_payment_items_voucher_id ON xcentro.batch_payment_items USING btree (voucher_id);


--
-- Name: idx_batch_payments_business_id; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_batch_payments_business_id ON xcentro.batch_payments USING btree (business_id);


--
-- Name: idx_batch_payments_created_at; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_batch_payments_created_at ON xcentro.batch_payments USING btree (created_at);


--
-- Name: idx_batch_payments_status; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_batch_payments_status ON xcentro.batch_payments USING btree (status);


--
-- Name: idx_business_income_tax_id; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_business_income_tax_id ON xcentro.business USING btree (income_tax_id);


--
-- Name: idx_business_name_search; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_business_name_search ON xcentro.business USING btree (name text_pattern_ops);


--
-- Name: idx_business_payee_relation_account; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_business_payee_relation_account ON xcentro.business_payee_relation USING btree (account_id);


--
-- Name: idx_business_payee_relation_active; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_business_payee_relation_active ON xcentro.business_payee_relation USING btree (business_id, is_active) WHERE (is_active = true);


--
-- Name: idx_business_payee_relation_business; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_business_payee_relation_business ON xcentro.business_payee_relation USING btree (business_id);


--
-- Name: idx_business_payee_relation_business_id; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_business_payee_relation_business_id ON xcentro.business_payee_relation USING btree (business_id);


--
-- Name: idx_business_payee_relation_display_name; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_business_payee_relation_display_name ON xcentro.business_payee_relation USING btree (business_id, display_name);


--
-- Name: idx_business_payee_relation_favorite; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_business_payee_relation_favorite ON xcentro.business_payee_relation USING btree (business_id, is_favorite) WHERE (is_favorite = true);


--
-- Name: idx_business_payee_relation_payee; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_business_payee_relation_payee ON xcentro.business_payee_relation USING btree (payee_id);


--
-- Name: idx_business_payee_relation_payee_id; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_business_payee_relation_payee_id ON xcentro.business_payee_relation USING btree (payee_id);


--
-- Name: idx_business_user_relation_bid; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_business_user_relation_bid ON xcentro.business_user_relation USING btree (bid);


--
-- Name: idx_feature_flags_name; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_feature_flags_name ON xcentro.feature_flags USING btree (flag_name);


--
-- Name: idx_fee_rules_lookup; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_fee_rules_lookup ON xcentro.payment_fee_rules USING btree (payment_method, card_type, card_category) WHERE (is_active = true);


--
-- Name: idx_gst_virtual_active; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_gst_virtual_active ON xcentro.payee_gst_virtual_accounts USING btree (payee_id, is_active) WHERE (is_active = true);


--
-- Name: idx_gst_virtual_financial_year; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_gst_virtual_financial_year ON xcentro.payee_gst_virtual_accounts USING btree (financial_year);


--
-- Name: idx_gst_virtual_payee; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_gst_virtual_payee ON xcentro.payee_gst_virtual_accounts USING btree (payee_id);


--
-- Name: idx_gst_virtual_tax_period; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_gst_virtual_tax_period ON xcentro.payee_gst_virtual_accounts USING btree (tax_period);


--
-- Name: idx_ops_session_user_token; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_ops_session_user_token ON xcentro.ops_session_table USING btree (ops_user_id, refresh_token_hash);


--
-- Name: idx_payee_biller_accounts_auto_fetch; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payee_biller_accounts_auto_fetch ON xcentro.payee_biller_accounts USING btree (auto_fetch_enabled) WHERE (auto_fetch_enabled = true);


--
-- Name: idx_payee_biller_accounts_business; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payee_biller_accounts_business ON xcentro.payee_biller_accounts USING btree (business_id);


--
-- Name: idx_payee_biller_accounts_payee; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payee_biller_accounts_payee ON xcentro.payee_biller_accounts USING btree (payee_id);


--
-- Name: idx_payee_biller_biller_id; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payee_biller_biller_id ON xcentro.payee_biller USING btree (biller_id);


--
-- Name: idx_payee_biller_category; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payee_biller_category ON xcentro.payee_biller USING btree (biller_category);


--
-- Name: idx_payee_employee_full_name; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payee_employee_full_name ON xcentro.payee_employee USING btree (full_name);


--
-- Name: idx_payee_master_active; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payee_master_active ON xcentro.payee_master USING btree (is_active) WHERE (is_active = true);


--
-- Name: idx_payee_master_type; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payee_master_type ON xcentro.payee_master USING btree (payee_type);


--
-- Name: idx_payee_vendor_gst_status; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payee_vendor_gst_status ON xcentro.payee_vendor USING btree (gst_status);


--
-- Name: idx_payee_vendor_gstin; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payee_vendor_gstin ON xcentro.payee_vendor USING btree (gstin);


--
-- Name: idx_payee_vendor_legal_name; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payee_vendor_legal_name ON xcentro.payee_vendor USING btree (legal_name);


--
-- Name: idx_payment_invoices_created_at; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payment_invoices_created_at ON xcentro.payment_invoices USING btree (created_at DESC);


--
-- Name: idx_payment_invoices_date_range; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payment_invoices_date_range ON xcentro.payment_invoices USING btree (invoice_date, created_at);


--
-- Name: idx_payment_invoices_filing_status; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payment_invoices_filing_status ON xcentro.payment_invoices USING btree (filing_status);


--
-- Name: idx_payment_invoices_gstin_recipient; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payment_invoices_gstin_recipient ON xcentro.payment_invoices USING btree (gstin_recipient);


--
-- Name: idx_payment_invoices_invoice_date; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payment_invoices_invoice_date ON xcentro.payment_invoices USING btree (invoice_date DESC);


--
-- Name: idx_payment_invoices_invoice_number; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payment_invoices_invoice_number ON xcentro.payment_invoices USING btree (invoice_number);


--
-- Name: idx_payment_invoices_irn; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payment_invoices_irn ON xcentro.payment_invoices USING btree (irn) WHERE (irn IS NOT NULL);


--
-- Name: idx_payment_invoices_irn_status; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payment_invoices_irn_status ON xcentro.payment_invoices USING btree (irn_status);


--
-- Name: idx_payment_invoices_payment_id; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payment_invoices_payment_id ON xcentro.payment_invoices USING btree (payment_id);


--
-- Name: idx_payment_invoices_status_combo; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payment_invoices_status_combo ON xcentro.payment_invoices USING btree (irn_status, filing_status, created_at DESC);


--
-- Name: idx_payment_state_action; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payment_state_action ON xcentro.payment_state_changes USING btree (action, action_timestamp DESC);


--
-- Name: idx_payment_state_actor; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payment_state_actor ON xcentro.payment_state_changes USING btree (actor_user_id) WHERE (actor_user_id IS NOT NULL);


--
-- Name: idx_payment_state_payment; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payment_state_payment ON xcentro.payment_state_changes USING btree (payment_id, action_timestamp DESC);


--
-- Name: idx_payment_state_timestamp; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payment_state_timestamp ON xcentro.payment_state_changes USING btree (action_timestamp DESC);


--
-- Name: idx_payment_state_transfer; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payment_state_transfer ON xcentro.payment_state_changes USING btree (transfer_request_id, action_timestamp DESC);


--
-- Name: idx_payment_voucher_pending_created_at; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payment_voucher_pending_created_at ON xcentro.payment_voucher_pending USING btree (created_at);


--
-- Name: idx_payment_voucher_pending_payment_id; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payment_voucher_pending_payment_id ON xcentro.payment_voucher_pending USING btree (payment_id);


--
-- Name: idx_payment_voucher_pending_voucher_id; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payment_voucher_pending_voucher_id ON xcentro.payment_voucher_pending USING btree (voucher_id);


--
-- Name: idx_payments_business_month_method; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payments_business_month_method ON xcentro.payments USING btree (business_id, payment_method, created_at) WHERE (status = 'success'::xcentro.payment_status_enum);


--
-- Name: INDEX idx_payments_business_month_method; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON INDEX xcentro.idx_payments_business_month_method IS 'Dashboard optimization: expense distribution by payment source (pie chart)';


--
-- Name: idx_payments_business_status_created; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payments_business_status_created ON xcentro.payments USING btree (business_id, status, created_at DESC) WHERE (status = ANY (ARRAY['success'::xcentro.payment_status_enum, 'pending'::xcentro.payment_status_enum, 'failed'::xcentro.payment_status_enum]));


--
-- Name: INDEX idx_payments_business_status_created; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON INDEX xcentro.idx_payments_business_status_created IS 'Dashboard optimization: total spend this month, recent payments, payment trends';


--
-- Name: idx_payments_gateway_payment_id_unique; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE UNIQUE INDEX idx_payments_gateway_payment_id_unique ON xcentro.payments USING btree (gateway_payment_id) WHERE (gateway_payment_id IS NOT NULL);


--
-- Name: idx_payments_mdr_tracking; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payments_mdr_tracking ON xcentro.payments USING btree (gateway_used, settled_at, mdr_amount) WHERE ((mdr_amount > 0) AND (settled_at IS NOT NULL));


--
-- Name: idx_payments_payment_type; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payments_payment_type ON xcentro.payments USING btree (payment_type);


--
-- Name: idx_payments_retry_of; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payments_retry_of ON xcentro.payments USING btree (retry_of_payment_id) WHERE (retry_of_payment_id IS NOT NULL);


--
-- Name: idx_payments_settlement; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payments_settlement ON xcentro.payments USING btree (settlement_status, settled_at) WHERE (settlement_status = 'settled'::xcentro.settlement_status_enum);


--
-- Name: idx_payments_settlement_amounts; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payments_settlement_amounts ON xcentro.payments USING btree (business_id, settlement_status, settlement_amount, total_amount) WHERE ((settlement_status = 'settled'::xcentro.settlement_status_enum) AND (settlement_amount IS NOT NULL));


--
-- Name: idx_payout_initiated; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payout_initiated ON xcentro.payout_transfer_requests USING btree (status, initiated_at) WHERE (status = ANY (ARRAY['initiated'::xcentro.payout_transfer_status_enum, 'processing'::xcentro.payout_transfer_status_enum]));


--
-- Name: idx_payout_timeout_monitor; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payout_timeout_monitor ON xcentro.payout_transfer_requests USING btree (webhook_timeout_at, status) WHERE ((status = ANY (ARRAY['initiated'::xcentro.payout_transfer_status_enum, 'processing'::xcentro.payout_transfer_status_enum])) AND (webhook_timeout_at IS NOT NULL));


--
-- Name: idx_payout_transfers_deleted; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payout_transfers_deleted ON xcentro.payout_transfer_requests USING btree (deleted_at) WHERE (is_deleted = true);


--
-- Name: idx_payout_transfers_payment_id; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payout_transfers_payment_id ON xcentro.payout_transfer_requests USING btree (payment_id);


--
-- Name: idx_payout_transfers_payment_status; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payout_transfers_payment_status ON xcentro.payout_transfer_requests USING btree (payment_id, status);


--
-- Name: idx_payout_transfers_provider_transfer_id; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payout_transfers_provider_transfer_id ON xcentro.payout_transfer_requests USING btree (provider_transfer_id);


--
-- Name: idx_payout_transfers_status; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payout_transfers_status ON xcentro.payout_transfer_requests USING btree (status);


--
-- Name: idx_payout_transfers_voucher; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payout_transfers_voucher ON xcentro.payout_transfer_requests USING btree (voucher_id);


--
-- Name: idx_payout_transfers_voucher_all_attempts; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payout_transfers_voucher_all_attempts ON xcentro.payout_transfer_requests USING btree (voucher_id, created_at);


--
-- Name: idx_payout_transfers_voucher_chronological; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_payout_transfers_voucher_chronological ON xcentro.payout_transfer_requests USING btree (voucher_id, created_at DESC) WHERE (is_deleted = false);


--
-- Name: idx_person_identity_national_id; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_person_identity_national_id ON xcentro.user_identity USING btree (national_id);


--
-- Name: idx_user_notifications_stuck_processing; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_user_notifications_stuck_processing ON xcentro.user_notifications USING btree (created_at) WHERE (status = 'processing'::text);


--
-- Name: idx_user_notifications_user_web; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_user_notifications_user_web ON xcentro.user_notifications USING btree (user_id, read_at NULLS FIRST, created_at DESC) WHERE (channel = 'web'::text);


--
-- Name: idx_user_notifications_worker_jobs; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_user_notifications_worker_jobs ON xcentro.user_notifications USING btree (channel, status, retry_count, created_at) WHERE (status = ANY (ARRAY['pending'::text, 'failed'::text]));


--
-- Name: idx_user_tokens_expires_at; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_user_tokens_expires_at ON xcentro.user_tokens USING btree (expires_at);


--
-- Name: idx_user_tokens_user_device; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_user_tokens_user_device ON xcentro.user_tokens USING btree (user_id, device_type, created_at DESC);


--
-- Name: idx_user_tokens_user_device_token; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_user_tokens_user_device_token ON xcentro.user_tokens USING btree (user_id, device_type, refresh_token_hash);


--
-- Name: idx_user_verification_codes_user_id; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_user_verification_codes_user_id ON xcentro.user_verification_codes USING btree (user_id);


--
-- Name: idx_voucher_approvals_approver; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_voucher_approvals_approver ON xcentro.voucher_approvals USING btree (approver_user_id, action);


--
-- Name: idx_voucher_approvals_requester; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_voucher_approvals_requester ON xcentro.voucher_approvals USING btree (requester_user_id);


--
-- Name: idx_voucher_approvals_voucher; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_voucher_approvals_voucher ON xcentro.voucher_approvals USING btree (voucher_id);


--
-- Name: idx_vouchers_business_due_date_status; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_vouchers_business_due_date_status ON xcentro.payment_vouchers USING btree (business_id, invoice_due_date, status) WHERE (status = ANY (ARRAY['draft'::xcentro.voucher_status, 'finalized'::xcentro.voucher_status]));


--
-- Name: INDEX idx_vouchers_business_due_date_status; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON INDEX xcentro.idx_vouchers_business_due_date_status IS 'Dashboard optimization: overdue bills count and total value';


--
-- Name: idx_vouchers_business_upcoming_large; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE INDEX idx_vouchers_business_upcoming_large ON xcentro.payment_vouchers USING btree (business_id, status, net_amount DESC) WHERE (status = ANY (ARRAY['draft'::xcentro.voucher_status, 'finalized'::xcentro.voucher_status]));


--
-- Name: INDEX idx_vouchers_business_upcoming_large; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON INDEX xcentro.idx_vouchers_business_upcoming_large IS 'Dashboard optimization: upcoming large payments (top 3 by amount)';


--
-- Name: unique_account_per_business; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE UNIQUE INDEX unique_account_per_business ON xcentro.business_payee_relation USING btree (business_id, account_id);


--
-- Name: INDEX unique_account_per_business; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON INDEX xcentro.unique_account_per_business IS 'Ensures each business can only have one payee per bank account (prevents fraud while allowing cross-business account reuse for Castler efficiency)';


--
-- Name: unique_voucher_transfer_active; Type: INDEX; Schema: xcentro; Owner: -
--

CREATE UNIQUE INDEX unique_voucher_transfer_active ON xcentro.payout_transfer_requests USING btree (voucher_id) WHERE (is_deleted = false);


--
-- Name: INDEX unique_voucher_transfer_active; Type: COMMENT; Schema: xcentro; Owner: -
--

COMMENT ON INDEX xcentro.unique_voucher_transfer_active IS 'Ensures only ONE active (non-deleted) transfer exists per voucher at any time. Allows multiple soft-deleted historical attempts.';


--
-- Name: business set_timestamp; Type: TRIGGER; Schema: xcentro; Owner: -
--

CREATE TRIGGER set_timestamp BEFORE UPDATE ON xcentro.business FOR EACH ROW EXECUTE FUNCTION xcentro.trigger_set_timestamp();


--
-- Name: business_user_relation set_timestamp; Type: TRIGGER; Schema: xcentro; Owner: -
--

CREATE TRIGGER set_timestamp BEFORE UPDATE ON xcentro.business_user_relation FOR EACH ROW EXECUTE FUNCTION xcentro.trigger_set_timestamp();


--
-- Name: notification_preferences set_timestamp; Type: TRIGGER; Schema: xcentro; Owner: -
--

CREATE TRIGGER set_timestamp BEFORE UPDATE ON xcentro.notification_preferences FOR EACH ROW EXECUTE FUNCTION xcentro.trigger_set_timestamp();


--
-- Name: payment_fee_rules set_timestamp; Type: TRIGGER; Schema: xcentro; Owner: -
--

CREATE TRIGGER set_timestamp BEFORE UPDATE ON xcentro.payment_fee_rules FOR EACH ROW EXECUTE FUNCTION xcentro.trigger_set_timestamp();


--
-- Name: payment_invoices set_timestamp; Type: TRIGGER; Schema: xcentro; Owner: -
--

CREATE TRIGGER set_timestamp BEFORE UPDATE ON xcentro.payment_invoices FOR EACH ROW EXECUTE FUNCTION xcentro.trigger_set_timestamp();


--
-- Name: payments set_timestamp; Type: TRIGGER; Schema: xcentro; Owner: -
--

CREATE TRIGGER set_timestamp BEFORE UPDATE ON xcentro.payments FOR EACH ROW EXECUTE FUNCTION xcentro.trigger_set_timestamp();


--
-- Name: user_device_versions set_timestamp; Type: TRIGGER; Schema: xcentro; Owner: -
--

CREATE TRIGGER set_timestamp BEFORE UPDATE ON xcentro.user_device_versions FOR EACH ROW EXECUTE FUNCTION xcentro.trigger_set_timestamp();


--
-- Name: user_identity set_timestamp; Type: TRIGGER; Schema: xcentro; Owner: -
--

CREATE TRIGGER set_timestamp BEFORE UPDATE ON xcentro.user_identity FOR EACH ROW EXECUTE FUNCTION xcentro.trigger_set_timestamp();


--
-- Name: user_table set_timestamp; Type: TRIGGER; Schema: xcentro; Owner: -
--

CREATE TRIGGER set_timestamp BEFORE UPDATE ON xcentro.user_table FOR EACH ROW EXECUTE FUNCTION xcentro.trigger_set_timestamp();


--
-- Name: business_payee_relation set_timestamp_business_payee_relation; Type: TRIGGER; Schema: xcentro; Owner: -
--

CREATE TRIGGER set_timestamp_business_payee_relation BEFORE UPDATE ON xcentro.business_payee_relation FOR EACH ROW EXECUTE FUNCTION xcentro.trigger_set_timestamp();


--
-- Name: payee_bank_accounts set_timestamp_payee_bank_accounts; Type: TRIGGER; Schema: xcentro; Owner: -
--

CREATE TRIGGER set_timestamp_payee_bank_accounts BEFORE UPDATE ON xcentro.payee_bank_accounts FOR EACH ROW EXECUTE FUNCTION xcentro.trigger_set_timestamp();


--
-- Name: payee_biller set_timestamp_payee_biller; Type: TRIGGER; Schema: xcentro; Owner: -
--

CREATE TRIGGER set_timestamp_payee_biller BEFORE UPDATE ON xcentro.payee_biller FOR EACH ROW EXECUTE FUNCTION xcentro.trigger_set_timestamp();


--
-- Name: payee_biller_accounts set_timestamp_payee_biller_accounts; Type: TRIGGER; Schema: xcentro; Owner: -
--

CREATE TRIGGER set_timestamp_payee_biller_accounts BEFORE UPDATE ON xcentro.payee_biller_accounts FOR EACH ROW EXECUTE FUNCTION xcentro.trigger_set_timestamp();


--
-- Name: payee_employee set_timestamp_payee_employee; Type: TRIGGER; Schema: xcentro; Owner: -
--

CREATE TRIGGER set_timestamp_payee_employee BEFORE UPDATE ON xcentro.payee_employee FOR EACH ROW EXECUTE FUNCTION xcentro.trigger_set_timestamp();


--
-- Name: payee_gst set_timestamp_payee_gst; Type: TRIGGER; Schema: xcentro; Owner: -
--

CREATE TRIGGER set_timestamp_payee_gst BEFORE UPDATE ON xcentro.payee_gst FOR EACH ROW EXECUTE FUNCTION xcentro.trigger_set_timestamp();


--
-- Name: payee_gst_virtual_accounts set_timestamp_payee_gst_virtual_accounts; Type: TRIGGER; Schema: xcentro; Owner: -
--

CREATE TRIGGER set_timestamp_payee_gst_virtual_accounts BEFORE UPDATE ON xcentro.payee_gst_virtual_accounts FOR EACH ROW EXECUTE FUNCTION xcentro.trigger_set_timestamp();


--
-- Name: payee_master set_timestamp_payee_master; Type: TRIGGER; Schema: xcentro; Owner: -
--

CREATE TRIGGER set_timestamp_payee_master BEFORE UPDATE ON xcentro.payee_master FOR EACH ROW EXECUTE FUNCTION xcentro.trigger_set_timestamp();


--
-- Name: payee_vendor set_timestamp_payee_vendor; Type: TRIGGER; Schema: xcentro; Owner: -
--

CREATE TRIGGER set_timestamp_payee_vendor BEFORE UPDATE ON xcentro.payee_vendor FOR EACH ROW EXECUTE FUNCTION xcentro.trigger_set_timestamp();


--
-- Name: ops_user_table update_ops_user_updated_at; Type: TRIGGER; Schema: xcentro; Owner: -
--

CREATE TRIGGER update_ops_user_updated_at BEFORE UPDATE ON xcentro.ops_user_table FOR EACH ROW EXECUTE FUNCTION xcentro.trigger_set_timestamp();


--
-- Name: bank_accounts bank_accounts_business_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.bank_accounts
    ADD CONSTRAINT bank_accounts_business_id_fkey FOREIGN KEY (business_id) REFERENCES xcentro.business(bid) ON DELETE CASCADE;


--
-- Name: batch_payment_items batch_payment_items_batch_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.batch_payment_items
    ADD CONSTRAINT batch_payment_items_batch_id_fkey FOREIGN KEY (batch_id) REFERENCES xcentro.batch_payments(batch_id) ON DELETE CASCADE;


--
-- Name: batch_payments batch_payments_business_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.batch_payments
    ADD CONSTRAINT batch_payments_business_id_fkey FOREIGN KEY (business_id) REFERENCES xcentro.business(bid) ON DELETE CASCADE;


--
-- Name: batch_payments batch_payments_uploaded_by_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.batch_payments
    ADD CONSTRAINT batch_payments_uploaded_by_fkey FOREIGN KEY (uploaded_by) REFERENCES xcentro.user_table(id) ON DELETE RESTRICT;


--
-- Name: business business_income_tax_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.business
    ADD CONSTRAINT business_income_tax_id_fkey FOREIGN KEY (income_tax_id) REFERENCES xcentro.kyb_data(income_tax_id);


--
-- Name: business_payee_relation business_payee_relation_added_by_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.business_payee_relation
    ADD CONSTRAINT business_payee_relation_added_by_fkey FOREIGN KEY (added_by) REFERENCES xcentro.user_table(id) ON DELETE SET NULL;


--
-- Name: business_payee_relation business_payee_relation_business_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.business_payee_relation
    ADD CONSTRAINT business_payee_relation_business_id_fkey FOREIGN KEY (business_id) REFERENCES xcentro.business(bid) ON DELETE CASCADE;


--
-- Name: business_payee_relation business_payee_relation_payee_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.business_payee_relation
    ADD CONSTRAINT business_payee_relation_payee_id_fkey FOREIGN KEY (payee_id) REFERENCES xcentro.payee_master(payee_id) ON DELETE CASCADE;


--
-- Name: business_user_relation business_user_relation_bid_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.business_user_relation
    ADD CONSTRAINT business_user_relation_bid_fkey FOREIGN KEY (bid) REFERENCES xcentro.business(bid) ON DELETE CASCADE;


--
-- Name: business_user_relation business_user_relation_user_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.business_user_relation
    ADD CONSTRAINT business_user_relation_user_id_fkey FOREIGN KEY (user_id) REFERENCES xcentro.user_table(id) ON DELETE CASCADE;


--
-- Name: erp_sync_queue erp_sync_queue_voucher_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.erp_sync_queue
    ADD CONSTRAINT erp_sync_queue_voucher_id_fkey FOREIGN KEY (voucher_id) REFERENCES xcentro.payment_vouchers(voucher_id) ON DELETE CASCADE;


--
-- Name: payment_voucher_pending fk_payment_voucher_pending_payment; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payment_voucher_pending
    ADD CONSTRAINT fk_payment_voucher_pending_payment FOREIGN KEY (payment_id) REFERENCES xcentro.payments(payment_id) ON DELETE CASCADE;


--
-- Name: payment_voucher_pending fk_payment_voucher_pending_voucher; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payment_voucher_pending
    ADD CONSTRAINT fk_payment_voucher_pending_voucher FOREIGN KEY (voucher_id) REFERENCES xcentro.payment_vouchers(voucher_id) ON DELETE CASCADE;


--
-- Name: notification_preferences notification_preferences_user_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.notification_preferences
    ADD CONSTRAINT notification_preferences_user_id_fkey FOREIGN KEY (user_id) REFERENCES xcentro.user_table(id) ON DELETE CASCADE;


--
-- Name: ops_session_table ops_session_table_ops_user_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.ops_session_table
    ADD CONSTRAINT ops_session_table_ops_user_id_fkey FOREIGN KEY (ops_user_id) REFERENCES xcentro.ops_user_table(id) ON DELETE CASCADE;


--
-- Name: payee_biller_accounts payee_biller_accounts_business_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payee_biller_accounts
    ADD CONSTRAINT payee_biller_accounts_business_id_fkey FOREIGN KEY (business_id) REFERENCES xcentro.business(bid) ON DELETE CASCADE;


--
-- Name: payee_biller_accounts payee_biller_accounts_payee_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payee_biller_accounts
    ADD CONSTRAINT payee_biller_accounts_payee_id_fkey FOREIGN KEY (payee_id) REFERENCES xcentro.payee_master(payee_id) ON DELETE CASCADE;


--
-- Name: payee_biller payee_biller_payee_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payee_biller
    ADD CONSTRAINT payee_biller_payee_id_fkey FOREIGN KEY (payee_id) REFERENCES xcentro.payee_master(payee_id) ON DELETE CASCADE;


--
-- Name: payee_employee payee_employee_payee_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payee_employee
    ADD CONSTRAINT payee_employee_payee_id_fkey FOREIGN KEY (payee_id) REFERENCES xcentro.payee_master(payee_id) ON DELETE CASCADE;


--
-- Name: payee_gst payee_gst_payee_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payee_gst
    ADD CONSTRAINT payee_gst_payee_id_fkey FOREIGN KEY (payee_id) REFERENCES xcentro.payee_master(payee_id) ON DELETE CASCADE;


--
-- Name: payee_gst_virtual_accounts payee_gst_virtual_accounts_payee_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payee_gst_virtual_accounts
    ADD CONSTRAINT payee_gst_virtual_accounts_payee_id_fkey FOREIGN KEY (payee_id) REFERENCES xcentro.payee_master(payee_id) ON DELETE CASCADE;


--
-- Name: payee_vendor payee_vendor_payee_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payee_vendor
    ADD CONSTRAINT payee_vendor_payee_id_fkey FOREIGN KEY (payee_id) REFERENCES xcentro.payee_master(payee_id) ON DELETE CASCADE;


--
-- Name: payment_invoices payment_invoices_payment_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payment_invoices
    ADD CONSTRAINT payment_invoices_payment_id_fkey FOREIGN KEY (payment_id) REFERENCES xcentro.payments(payment_id) ON DELETE CASCADE;


--
-- Name: payment_state_changes payment_state_changes_actor_user_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payment_state_changes
    ADD CONSTRAINT payment_state_changes_actor_user_id_fkey FOREIGN KEY (actor_user_id) REFERENCES xcentro.user_table(id) ON DELETE SET NULL;


--
-- Name: payment_state_changes payment_state_changes_payment_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payment_state_changes
    ADD CONSTRAINT payment_state_changes_payment_id_fkey FOREIGN KEY (payment_id) REFERENCES xcentro.payments(payment_id) ON DELETE CASCADE;


--
-- Name: payment_state_changes payment_state_changes_transfer_request_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payment_state_changes
    ADD CONSTRAINT payment_state_changes_transfer_request_id_fkey FOREIGN KEY (transfer_request_id) REFERENCES xcentro.payout_transfer_requests(transfer_request_id) ON DELETE SET NULL;


--
-- Name: payment_vouchers payment_vouchers_business_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payment_vouchers
    ADD CONSTRAINT payment_vouchers_business_id_fkey FOREIGN KEY (business_id) REFERENCES xcentro.business(bid) ON DELETE CASCADE;


--
-- Name: payment_vouchers payment_vouchers_business_id_payee_id_account_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payment_vouchers
    ADD CONSTRAINT payment_vouchers_business_id_payee_id_account_id_fkey FOREIGN KEY (business_id, payee_id, account_id) REFERENCES xcentro.business_payee_relation(business_id, payee_id, account_id) ON DELETE RESTRICT;


--
-- Name: payment_vouchers payment_vouchers_created_by_user_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payment_vouchers
    ADD CONSTRAINT payment_vouchers_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES xcentro.user_table(id) ON DELETE SET NULL;


--
-- Name: payment_vouchers payment_vouchers_payment_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payment_vouchers
    ADD CONSTRAINT payment_vouchers_payment_id_fkey FOREIGN KEY (payment_id) REFERENCES xcentro.payments(payment_id) ON DELETE RESTRICT;


--
-- Name: payments payments_business_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payments
    ADD CONSTRAINT payments_business_id_fkey FOREIGN KEY (business_id) REFERENCES xcentro.business(bid) ON DELETE CASCADE;


--
-- Name: payments payments_retry_of_payment_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payments
    ADD CONSTRAINT payments_retry_of_payment_id_fkey FOREIGN KEY (retry_of_payment_id) REFERENCES xcentro.payments(payment_id) ON DELETE SET NULL;


--
-- Name: payments payments_user_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payments
    ADD CONSTRAINT payments_user_id_fkey FOREIGN KEY (user_id) REFERENCES xcentro.user_table(id) ON DELETE SET NULL;


--
-- Name: payout_transfer_requests payout_transfer_requests_payment_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payout_transfer_requests
    ADD CONSTRAINT payout_transfer_requests_payment_id_fkey FOREIGN KEY (payment_id) REFERENCES xcentro.payments(payment_id) ON DELETE RESTRICT;


--
-- Name: payout_transfer_requests payout_transfer_requests_voucher_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.payout_transfer_requests
    ADD CONSTRAINT payout_transfer_requests_voucher_id_fkey FOREIGN KEY (voucher_id) REFERENCES xcentro.payment_vouchers(voucher_id) ON DELETE RESTRICT;


--
-- Name: stored_cards stored_cards_business_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.stored_cards
    ADD CONSTRAINT stored_cards_business_id_fkey FOREIGN KEY (business_id) REFERENCES xcentro.business(bid) ON DELETE CASCADE;


--
-- Name: subscription_status subscription_status_bid_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.subscription_status
    ADD CONSTRAINT subscription_status_bid_fkey FOREIGN KEY (bid) REFERENCES xcentro.business(bid) ON DELETE CASCADE;


--
-- Name: user_device_versions user_device_versions_user_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.user_device_versions
    ADD CONSTRAINT user_device_versions_user_id_fkey FOREIGN KEY (user_id) REFERENCES xcentro.user_table(id) ON DELETE CASCADE;


--
-- Name: user_notifications user_notifications_business_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.user_notifications
    ADD CONSTRAINT user_notifications_business_id_fkey FOREIGN KEY (business_id) REFERENCES xcentro.business(bid) ON DELETE CASCADE;


--
-- Name: user_notifications user_notifications_user_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.user_notifications
    ADD CONSTRAINT user_notifications_user_id_fkey FOREIGN KEY (user_id) REFERENCES xcentro.user_table(id) ON DELETE CASCADE;


--
-- Name: user_table user_table_tax_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.user_table
    ADD CONSTRAINT user_table_tax_id_fkey FOREIGN KEY (tax_id) REFERENCES xcentro.user_identity(tax_id);


--
-- Name: user_tokens user_tokens_user_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.user_tokens
    ADD CONSTRAINT user_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES xcentro.user_table(id) ON DELETE CASCADE;


--
-- Name: user_verification_codes user_verification_codes_user_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.user_verification_codes
    ADD CONSTRAINT user_verification_codes_user_id_fkey FOREIGN KEY (user_id) REFERENCES xcentro.user_table(id) ON DELETE CASCADE;


--
-- Name: voucher_approvals voucher_approvals_approver_user_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.voucher_approvals
    ADD CONSTRAINT voucher_approvals_approver_user_id_fkey FOREIGN KEY (approver_user_id) REFERENCES xcentro.user_table(id) ON DELETE CASCADE;


--
-- Name: voucher_approvals voucher_approvals_requester_user_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.voucher_approvals
    ADD CONSTRAINT voucher_approvals_requester_user_id_fkey FOREIGN KEY (requester_user_id) REFERENCES xcentro.user_table(id) ON DELETE CASCADE;


--
-- Name: voucher_approvals voucher_approvals_voucher_id_fkey; Type: FK CONSTRAINT; Schema: xcentro; Owner: -
--

ALTER TABLE ONLY xcentro.voucher_approvals
    ADD CONSTRAINT voucher_approvals_voucher_id_fkey FOREIGN KEY (voucher_id) REFERENCES xcentro.payment_vouchers(voucher_id) ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--

\unrestrict Egde31RWpFfEEGERRaOkCufxCJqRgKiqbs4sfh0Q93uIoRjqQfC4z5vKjPUukMf

