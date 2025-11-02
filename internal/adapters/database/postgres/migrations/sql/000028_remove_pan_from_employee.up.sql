-- Remove PAN-related fields from payee_employee table
-- Reason: Not a full-fledged payroll management system, PAN verification not needed

-- Drop the index on PAN
DROP INDEX IF EXISTS xcentro.idx_payee_employee_pan;

-- Remove PAN verification fields
ALTER TABLE xcentro.payee_employee
DROP COLUMN IF EXISTS pan_verification_status,
DROP COLUMN IF EXISTS pan_verified_at,
DROP COLUMN IF EXISTS pan;

-- Remove comments referencing PAN
COMMENT ON TABLE xcentro.payee_employee IS 'Employee payees for payroll (individuals, not businesses)';
