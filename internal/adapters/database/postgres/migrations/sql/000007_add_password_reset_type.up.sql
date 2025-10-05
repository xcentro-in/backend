-- Add password_reset to verification_type enum
ALTER TYPE xcentro.verification_type ADD VALUE 'password_reset';

-- Add kyc_needed column to user_table
ALTER TABLE xcentro.user_table ADD COLUMN kyc_needed BOOLEAN DEFAULT FALSE;
