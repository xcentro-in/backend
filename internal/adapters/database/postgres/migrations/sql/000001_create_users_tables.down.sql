-- #############################################################################
-- # Subscription Entity
-- #############################################################################
DROP TABLE IF EXISTS xcentro.subscription_status;


-- #############################################################################
-- # Business Entity
-- #############################################################################
DROP TRIGGER IF EXISTS set_timestamp ON xcentro.business_user_relation;
DROP TRIGGER IF EXISTS set_timestamp ON xcentro.business;
DROP INDEX IF EXISTS xcentro.idx_business_user_relation_bid;
DROP INDEX IF EXISTS xcentro.idx_business_itid; -- legacy index name
DROP INDEX IF EXISTS xcentro.idx_business_income_tax_id; -- current index name
DROP TABLE IF EXISTS xcentro.business_user_relation;
DROP TABLE IF EXISTS xcentro.business;
DROP TABLE IF EXISTS xcentro.kyb_data;


-- #############################################################################
-- # User Entity
-- #############################################################################
DROP TRIGGER IF EXISTS set_timestamp ON xcentro.user_table;
DROP INDEX IF EXISTS xcentro.idx_user_verification_codes_user_id;
DROP TABLE IF EXISTS xcentro.user_verification_codes;
DROP TABLE IF EXISTS xcentro.user_table;


-- #############################################################################
-- # General Functions and Types
-- #############################################################################
DROP FUNCTION IF EXISTS xcentro.trigger_set_timestamp();
-- DROP FUNCTION IF EXISTS xcentro.generate_uuid_v7();
-- DROP FUNCTION IF EXISTS generate_uuid_v7(); -- legacy unqualified version
DROP TYPE IF EXISTS xcentro.verification_type;
DROP TYPE IF EXISTS xcentro.kyc_status_enum;
DROP TYPE IF EXISTS xcentro.user_role_enum;
DROP TYPE IF EXISTS xcentro.business_type_enum;
DROP TYPE IF EXISTS xcentro.subscription_type_enum;
