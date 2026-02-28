-- Migration: 000062_add_active_bid_to_user_table.down.sql
-- Reverses: 000062_add_active_bid_to_user_table.up.sql

ALTER TABLE xcentro.user_table
DROP COLUMN IF EXISTS active_bid;
