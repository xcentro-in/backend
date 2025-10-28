-- #############################################################################
-- # NOTIFICATION SYSTEM TABLES
-- #############################################################################
-- This migration creates the PostgreSQL-only (Phase 1) implementation
-- of the notification queuing system.
--
-- Architecture:
-- - notification_events (queue) -> user_notifications (user inbox)
--
-- All tables are in the xcentro schema to match existing project structure.
--
-- NOTE: Audit system tables were previously in this migration but have been
-- removed. See docs/AUDIT_SYSTEM_ARCHITECTURE.md for the complete audit
-- system implementation archive.

-- #############################################################################
-- # 1. NOTIFICATION SYSTEM
-- #############################################################################

-- TABLE 1: notification_events (The Queue)
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


-- TABLE 2: user_notifications (The Destination)
-- Persistent "bell icon" inbox for each user.
CREATE TABLE xcentro.user_notifications (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES xcentro.user_table(id) ON DELETE CASCADE,
    business_id UUID DEFAULT NULL REFERENCES xcentro.business(bid) ON DELETE CASCADE,
    event_id BIGINT NOT NULL REFERENCES xcentro.notification_events(id) ON DELETE CASCADE,
    channel TEXT NOT NULL,             -- 'email', 'push', 'web'
    status TEXT NOT NULL DEFAULT 'pending', -- 'pending', 'processing', 'sent', 'failed', 'read'
    content JSONB NOT NULL,            -- {title, body, actions, entities}
    retry_count INTEGER NOT NULL DEFAULT 0, -- Number of retry attempts for failed jobs
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at TIMESTAMPTZ DEFAULT NULL,
    read_at TIMESTAMPTZ DEFAULT NULL,

    -- Prevent duplicate notifications per user per event per channel
    CONSTRAINT uq_user_notifications_event UNIQUE (event_id, user_id, channel)
);

-- Indexes for fast lookups

-- Index 1: User inbox queries (GetUserNotifications)
-- Covers: WHERE user_id = X ORDER BY created_at DESC
CREATE INDEX idx_user_notifications_user_id
ON xcentro.user_notifications (user_id, created_at DESC);

-- Index 2: Unread count queries (GetUnreadCount)
-- Covers: WHERE user_id = X AND read_at IS NULL
CREATE INDEX idx_user_notifications_user_unread
ON xcentro.user_notifications (user_id, created_at DESC)
WHERE read_at IS NULL AND channel = 'web';

-- Index 3: Worker job fetching (FetchAndMarkJobsAsProcessing)
-- Covers: WHERE channel = X AND status IN ('pending', 'failed') AND retry_count < max ORDER BY created_at
CREATE INDEX idx_user_notifications_worker_jobs
ON xcentro.user_notifications (channel, status, retry_count, created_at)
WHERE status IN ('pending', 'failed');

-- Index 4: Recovery worker - stuck processing jobs (RecoverStuckProcessingJobs)
-- Covers: WHERE status = 'processing' AND created_at < timeout
CREATE INDEX idx_user_notifications_stuck_processing
ON xcentro.user_notifications (created_at)
WHERE status = 'processing';

-- Index 5 (Optional): Business-scoped queries (if needed for analytics)
-- Covers: WHERE business_id = X
-- Note: Only create if you have queries filtering by business_id
-- CREATE INDEX idx_user_notifications_business_id
-- ON xcentro.user_notifications (business_id, created_at DESC)
-- WHERE business_id IS NOT NULL;

COMMENT ON TABLE xcentro.user_notifications IS 'Persistent notification inbox for each user';
COMMENT ON COLUMN xcentro.user_notifications.channel IS 'Delivery channel: email, push, web';
COMMENT ON COLUMN xcentro.user_notifications.status IS 'Lifecycle: pending -> processing -> sent/failed (with retries) -> read (web only)';
COMMENT ON COLUMN xcentro.user_notifications.content IS 'Rendered message: {title, body, actions, entities}';
COMMENT ON COLUMN xcentro.user_notifications.retry_count IS 'Number of retry attempts for failed jobs (auto-incremented on retry)';


-- #############################################################################
-- # 2. NOTIFICATION PREFERENCES
-- #############################################################################

-- TABLE 3: notification_preferences (User Preferences)
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
