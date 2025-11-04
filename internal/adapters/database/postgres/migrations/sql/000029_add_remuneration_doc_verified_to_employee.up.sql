-- Add remuneration_doc_verified column to track document verification status
ALTER TABLE xcentro.payee_employee
ADD COLUMN remuneration_doc_verified BOOLEAN DEFAULT FALSE NOT NULL;

-- Add comment for documentation
COMMENT ON COLUMN xcentro.payee_employee.remuneration_doc_verified IS 'Indicates whether the employee remuneration document has been uploaded and verified';
