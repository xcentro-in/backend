-- #############################################################################
-- # ROLLBACK: AUDIT & NOTIFICATION SYSTEM TABLES
-- #############################################################################
-- This migration rolls back the audit and notification system tables.
-- Drop order: child tables first (due to foreign keys)

-- Drop notification preferences table
DROP TABLE IF EXISTS xcentro.notification_preferences;

-- Drop notification system tables
DROP TABLE IF EXISTS xcentro.user_notifications;
DROP TABLE IF EXISTS xcentro.notification_events;

-- Drop audit system tables
DROP TABLE IF EXISTS xcentro.audit_trail;
DROP TABLE IF EXISTS xcentro.audit_events;
