-- Migration: 000062_add_active_bid_to_user_table.up.sql
-- Purpose: Add active_bid column to user_table to track which business the user's JWT is scoped to.
-- This enables the unified auth milestone (v1.0) where JWT carries bid/role claims.
-- Part of: AUTH-02

ALTER TABLE xcentro.user_table
ADD COLUMN active_bid UUID REFERENCES xcentro.business(bid) ON DELETE SET NULL;

COMMENT ON COLUMN xcentro.user_table.active_bid IS
'Tracks which business the user''s current JWT is scoped to. NULL if user has no active business or has not yet selected one.';
