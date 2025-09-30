-- #############################################################################
-- # Rollback Payment System Database Schema
-- # Phase 4.3: Drop payment tables and types
-- #############################################################################

-- Drop tables in reverse order of creation (respecting foreign key dependencies)
DROP TABLE IF EXISTS xcentro.payments CASCADE;
DROP TABLE IF EXISTS xcentro.bank_accounts CASCADE;
DROP TABLE IF EXISTS xcentro.stored_cards CASCADE;

-- Drop ENUM types in reverse order of creation
DROP TYPE IF EXISTS xcentro.currency_code_enum;
DROP TYPE IF EXISTS xcentro.payment_source_type_enum;
DROP TYPE IF EXISTS xcentro.card_category_enum;
DROP TYPE IF EXISTS xcentro.card_type_enum;
DROP TYPE IF EXISTS xcentro.payment_method_enum;
DROP TYPE IF EXISTS xcentro.gateway_type_enum;
DROP TYPE IF EXISTS xcentro.payment_status_enum;