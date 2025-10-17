-- #############################################################################
-- # Rollback Payment Fee Rules Table
-- # Phase 10: Remove fee rules configuration
-- #############################################################################

-- Drop table first (cascades to related triggers)
DROP TABLE IF EXISTS xcentro.payment_fee_rules;

-- Drop enum type
DROP TYPE IF EXISTS xcentro.fee_type_enum;
