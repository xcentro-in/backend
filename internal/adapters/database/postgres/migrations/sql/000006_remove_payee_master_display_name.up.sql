-- Migration: Remove redundant display_name from payee_master
-- Created: 2025-10-04
-- Purpose: Clean up redundant display_name column (canonical names live in type-specific tables)

-- Remove display_name column from payee_master
-- Rationale:
--   - Vendor canonical name: payee_vendor.legal_name
--   - Employee canonical name: payee_employee.full_name
--   - Biller canonical name: payee_biller.biller_name
--   - GST canonical name: payee_gst.government_entity_name
--   - Customer-specific name: business_payee_relation.display_name
ALTER TABLE xcentro.payee_master DROP COLUMN display_name;
