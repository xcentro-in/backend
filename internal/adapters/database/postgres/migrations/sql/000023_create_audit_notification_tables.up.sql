-- #############################################################################
-- # AUDIT & NOTIFICATION SYSTEM TABLES
-- #############################################################################
-- This migration creates the PostgreSQL-only (Phase 1) implementation
-- of the audit and notification queuing system.
--
-- Architecture:
-- - audit_events (queue) -> audit_trail (permanent log)
-- - notification_events (queue) -> user_notifications (user inbox)
--
-- All tables are in the xcentro schema to match existing project structure.

-- #############################################################################
-- # 1. AUDIT SYSTEM
-- #############################################################################

-- TABLE 1: audit_events (The Queue)
-- Temporary, append-only log of work to be done.
-- Business logic writes here FAST, then async processor moves to audit_trail.
CREATE TABLE xcentro.audit_events (
    id BIGSERIAL PRIMARY KEY,
    processed_at TIMESTAMPTZ DEFAULT NULL,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,     -- Full event details (actor, entities, change_log)
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for fast lookup of pending events
CREATE INDEX idx_audit_events_pending
ON xcentro.audit_events (created_at)
WHERE processed_at IS NULL;

COMMENT ON TABLE xcentro.audit_events IS 'Temporary queue of audit events to be processed';
COMMENT ON COLUMN xcentro.audit_events.payload IS 'JSON schema: {timestamp (RFC3339), event_type, actor, entities, note, change_log}';


-- TABLE 2: audit_trail (The Destination - Normalized Event Storage)
-- Permanent, queryable, append-only table optimized for SELECT by UI.
-- Stores the event ONCE, links to entities via audit_trail_entities table.
CREATE TABLE xcentro.audit_trail (
    id BIGSERIAL PRIMARY KEY,
    business_id UUID NOT NULL REFERENCES xcentro.business(bid) ON DELETE CASCADE,

    -- The Event Data
    event_timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    event_type TEXT NOT NULL,        -- Human-readable title (e.g., "Transaction Approved")
    actor_type TEXT NOT NULL,        -- 'user' or 'system'
    actor_id TEXT NOT NULL,          -- user_id UUID as text or 'system'
    actor_name TEXT NOT NULL,        -- "Michael Chen (Financial Controller)"
    note TEXT DEFAULT NULL,          -- Optional note (e.g., "Funds transferred")
    details JSONB DEFAULT NULL,      -- Extra data (IP, change_log, etc.)

    -- Link back to queue for idempotency
    source_event_id BIGINT NOT NULL UNIQUE REFERENCES xcentro.audit_events(id) ON DELETE CASCADE
);

-- Indexes for fast lookups
CREATE INDEX idx_audit_trail_business_id
ON xcentro.audit_trail (business_id, event_timestamp DESC);

CREATE INDEX idx_audit_trail_event_timestamp
ON xcentro.audit_trail (event_timestamp DESC);

COMMENT ON TABLE xcentro.audit_trail IS 'Permanent normalized audit log - stores event once, links via audit_trail_entities';
COMMENT ON COLUMN xcentro.audit_trail.event_type IS 'Human-readable event title for display';
COMMENT ON COLUMN xcentro.audit_trail.details IS 'Extra data: {ip_address, change_log, etc.}';
COMMENT ON COLUMN xcentro.audit_trail.source_event_id IS 'FK to audit_events.id (ensures no duplicate processing)';


-- TABLE 2B: audit_trail_entities (The Links)
-- Links a single audit trail event to multiple entities (normalized many-to-many).
-- This solves the bulk payment/voucher problem efficiently.
CREATE TABLE xcentro.audit_trail_entities (
    id BIGSERIAL PRIMARY KEY,
    business_id UUID NOT NULL REFERENCES xcentro.business(bid) ON DELETE CASCADE,

    -- Link to the event
    audit_trail_id BIGINT NOT NULL REFERENCES xcentro.audit_trail(id) ON DELETE CASCADE,

    -- Link to the entity
    entity_type TEXT NOT NULL,  -- 'payment', 'voucher', 'payee', 'user', 'wallet', 'payout'
    entity_id UUID NOT NULL     -- The ID of the payment, voucher, payee, etc.
);

-- Most important index for reading data (entity activity feeds)
CREATE INDEX idx_audit_trail_entities_lookup
ON xcentro.audit_trail_entities (entity_type, entity_id);

-- Index for reverse lookup (all entities for an event)
CREATE INDEX idx_audit_trail_entities_audit_id
ON xcentro.audit_trail_entities (audit_trail_id);

-- Composite index for business-scoped entity lookups
CREATE INDEX idx_audit_trail_entities_business_entity
ON xcentro.audit_trail_entities (business_id, entity_type, entity_id);

COMMENT ON TABLE xcentro.audit_trail_entities IS 'Many-to-many links between audit events and entities (normalized design)';
COMMENT ON COLUMN xcentro.audit_trail_entities.entity_type IS 'Type of entity: payment, voucher, payee, user, wallet, payout';
COMMENT ON COLUMN xcentro.audit_trail_entities.entity_id IS 'UUID of the entity (payment_id, voucher_id, etc.)';


-- #############################################################################
-- # 2. NOTIFICATION SYSTEM
-- #############################################################################

-- TABLE 3: notification_events (The Queue)
-- Temporary queue of business events to be "fanned out" to users.
CREATE TABLE xcentro.notification_events (
    id BIGSERIAL PRIMARY KEY,
    processed_at TIMESTAMPTZ DEFAULT NULL,
    event_type TEXT NOT NULL,      -- e.g., 'payment.failed', 'report.ready'
    payload JSONB NOT NULL,        -- Full message data (target, entities, message, actions)
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for fast lookup of pending events
CREATE INDEX idx_notification_events_pending
ON xcentro.notification_events (created_at)
WHERE processed_at IS NULL;

-- Index for event type analytics
CREATE INDEX idx_notification_events_type
ON xcentro.notification_events (event_type);

COMMENT ON TABLE xcentro.notification_events IS 'Temporary queue of notification events to be fanned out';
COMMENT ON COLUMN xcentro.notification_events.payload IS 'JSON schema: {timestamp (RFC3339), event_type, target, entities, message, actions}';


-- TABLE 4: user_notifications (The Destination)
-- Persistent "bell icon" inbox for each user.
CREATE TABLE xcentro.user_notifications (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES xcentro.user_table(id) ON DELETE CASCADE,
    business_id UUID DEFAULT NULL REFERENCES xcentro.business(bid) ON DELETE CASCADE,
    event_id BIGINT NOT NULL REFERENCES xcentro.notification_events(id) ON DELETE CASCADE,
    channel TEXT NOT NULL,             -- 'email', 'push', 'web'
    status TEXT NOT NULL DEFAULT 'pending', -- 'pending', 'sent', 'failed', 'read'
    content JSONB NOT NULL,            -- {title, body, actions, entities}
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at TIMESTAMPTZ DEFAULT NULL,
    read_at TIMESTAMPTZ DEFAULT NULL,

    -- Prevent duplicate notifications per user per event per channel
    CONSTRAINT uq_user_notifications_event UNIQUE (event_id, user_id, channel)
);

-- Indexes for fast lookups
CREATE INDEX idx_user_notifications_user_id
ON xcentro.user_notifications (user_id, created_at DESC);

CREATE INDEX idx_user_notifications_business_id
ON xcentro.user_notifications (business_id)
WHERE business_id IS NOT NULL;

CREATE INDEX idx_user_notifications_status
ON xcentro.user_notifications (status, created_at DESC)
WHERE status IN ('pending', 'sent');

-- Composite index for common query pattern: user + unread
CREATE INDEX idx_user_notifications_user_unread
ON xcentro.user_notifications (user_id, created_at DESC)
WHERE read_at IS NULL;

COMMENT ON TABLE xcentro.user_notifications IS 'Persistent notification inbox for each user';
COMMENT ON COLUMN xcentro.user_notifications.channel IS 'Delivery channel: email, push, web';
COMMENT ON COLUMN xcentro.user_notifications.status IS 'Lifecycle: pending -> sent -> read (or failed)';
COMMENT ON COLUMN xcentro.user_notifications.content IS 'Rendered message: {title, body, actions, entities}';


-- #############################################################################
-- # 3. NOTIFICATION PREFERENCES
-- #############################################################################

-- TABLE 5: notification_preferences (User Preferences)
-- Stores per-user notification preferences globally across all businesses.
-- Channel switches (email, push, messages) as boolean columns for easy querying.
-- Category preferences (announcements, rewards, payments, etc.) in JSONB for flexibility.
CREATE TABLE xcentro.notification_preferences (
    user_id UUID PRIMARY KEY REFERENCES xcentro.user_table(id) ON DELETE CASCADE,

    -- Channel-level switches (global on/off for each channel)
    email_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    push_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    messages_enabled BOOLEAN NOT NULL DEFAULT TRUE,

    -- Category preferences (flexible JSONB for easy extension)
    -- Structure: {"announcements": true, "rewards": true, "payments": true, "payees": true, "approvals": true, "security": true, "insights": true}
    category_preferences JSONB NOT NULL DEFAULT '{"announcements": true, "rewards": true, "payments": true, "payees": true, "approvals": true, "security": true, "insights": true}'::jsonb,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NULL
);

-- Trigger for automatic updated_at
CREATE TRIGGER set_timestamp
BEFORE UPDATE ON xcentro.notification_preferences
FOR EACH ROW
EXECUTE PROCEDURE xcentro.trigger_set_timestamp();

COMMENT ON TABLE xcentro.notification_preferences IS 'User notification preferences (global, not per-business)';
COMMENT ON COLUMN xcentro.notification_preferences.email_enabled IS 'Global switch for email notifications';
COMMENT ON COLUMN xcentro.notification_preferences.push_enabled IS 'Global switch for push notifications';
COMMENT ON COLUMN xcentro.notification_preferences.messages_enabled IS 'Global switch for in-app messages/web notifications';
COMMENT ON COLUMN xcentro.notification_preferences.category_preferences IS 'Category-level preferences: {announcements, rewards, payments, payees, approvals, security, insights}';
