-- Drop payout_configuration table (over-engineered, moving to code-based logic)
-- Transfer mode now calculated based on amount (NEFT ≤ ₹2L, RTGS > ₹2L)
-- Retry/worker settings moved to environment variables

DROP TABLE IF EXISTS xcentro.payout_configuration;
