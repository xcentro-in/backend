-- Migration 000009 Rollback: Remove payout transfer requests and configuration

-- Drop tables (cascade will drop dependent objects)
DROP TABLE IF EXISTS xcentro.payout_transfer_requests CASCADE;
DROP TABLE IF EXISTS xcentro.payout_configuration CASCADE;

-- Drop enums
DROP TYPE IF EXISTS xcentro.payout_transfer_status_enum CASCADE;
