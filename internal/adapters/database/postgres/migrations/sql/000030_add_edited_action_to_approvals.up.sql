-- Add 'edited' action to approval_action_enum for voucher edit audit trail
ALTER TYPE xcentro.approval_action_enum ADD VALUE 'edited';

COMMENT ON TYPE xcentro.approval_action_enum IS 'Approval actions: pending, approved, rejected, request_changes, edited';
