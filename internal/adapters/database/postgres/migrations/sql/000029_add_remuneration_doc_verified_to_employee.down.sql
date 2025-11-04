-- Rollback: Remove remuneration_doc_verified column
ALTER TABLE xcentro.payee_employee
DROP COLUMN remuneration_doc_verified;
