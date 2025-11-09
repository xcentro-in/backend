-- #############################################################################
-- # Rollback: Remove mockpg from gateway_type_enum
-- # Migration: 000036
-- #############################################################################

-- NOTE: PostgreSQL does not support removing enum values directly.
-- To rollback, you would need to:
-- 1. Ensure no data uses 'mockpg' value
-- 2. Recreate the enum without 'mockpg'
-- This is intentionally left as a no-op for safety.
-- If rollback is absolutely necessary, manually recreate the enum.

-- No-op rollback
SELECT 1;
