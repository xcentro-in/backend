-- #############################################################################
-- # Add mockpg to gateway_type_enum for testing
-- # Migration: 000036
-- #############################################################################

-- Add mockpg to gateway_type_enum
ALTER TYPE xcentro.gateway_type_enum ADD VALUE IF NOT EXISTS 'mockpg';
