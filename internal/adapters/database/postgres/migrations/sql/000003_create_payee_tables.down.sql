-- #############################################################################
-- # Rollback Polymorphic Payee Management System
-- # Phase 5.X: Drop all payee tables and types
-- #############################################################################

-- Drop tables in reverse dependency order

DROP TABLE IF EXISTS xcentro.business_payee_relation CASCADE;
DROP TABLE IF EXISTS xcentro.payee_bank_accounts CASCADE;
DROP TABLE IF EXISTS xcentro.payee_employee CASCADE;
DROP TABLE IF EXISTS xcentro.payee_biller_accounts CASCADE;
DROP TABLE IF EXISTS xcentro.payee_biller CASCADE;
DROP TABLE IF EXISTS xcentro.payee_gst_virtual_accounts CASCADE;
DROP TABLE IF EXISTS xcentro.payee_gst CASCADE;
DROP TABLE IF EXISTS xcentro.payee_vendor CASCADE;
DROP TABLE IF EXISTS xcentro.payee_master CASCADE;

-- Drop custom types
DROP TYPE IF EXISTS xcentro.verification_status_enum CASCADE;
DROP TYPE IF EXISTS xcentro.payee_type_enum CASCADE;
