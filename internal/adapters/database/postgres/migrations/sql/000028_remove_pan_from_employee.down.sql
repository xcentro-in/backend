-- Rollback: Re-add PAN-related fields to payee_employee table

-- Add PAN column back
ALTER TABLE xcentro.payee_employee
ADD COLUMN pan VARCHAR(10),
ADD COLUMN pan_verification_status xcentro.verification_status_enum DEFAULT 'pending',
ADD COLUMN pan_verified_at TIMESTAMPTZ;

-- Recreate index on PAN
CREATE INDEX idx_payee_employee_pan ON xcentro.payee_employee(pan);

-- Restore original comment
COMMENT ON TABLE xcentro.payee_employee IS 'Employee payees for payroll (individuals, not businesses)';
COMMENT ON COLUMN xcentro.payee_employee.pan IS 'Permanent Account Number (individual tax ID)';
