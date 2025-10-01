-- #############################################################################
-- # Payee Management System Rollback
-- # Phase 5.3: Drop payee tables and types
-- #############################################################################

-- Drop tables in reverse order (respecting foreign key dependencies)
DROP TABLE IF EXISTS xcentro.business_payee_relation;
DROP TABLE IF EXISTS xcentro.payee_bank_accounts;
DROP TABLE IF EXISTS xcentro.payee_business;

-- Drop custom types
DROP TYPE IF EXISTS xcentro.verification_status_enum;
