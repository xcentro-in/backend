package payment

import (
	"context"
	"fmt"

	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/payment"
)

// PgBankAccountRepository implements payment.BankAccountRepository using PostgreSQL
type PgBankAccountRepository struct {
	querier client.Querier
}

// NewPgBankAccountRepository creates a new PostgreSQL bank account repository
func NewPgBankAccountRepository(db client.DB) *PgBankAccountRepository {
	return &PgBankAccountRepository{
		querier: db,
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgBankAccountRepository) WithTx(tx interface{}) payment.BankAccountRepository {
	return &PgBankAccountRepository{
		querier: tx.(client.Tx),
	}
}

// CreateBankAccount creates a new bank account record
func (r *PgBankAccountRepository) CreateBankAccount(ctx context.Context, account *payment.BankAccount) error {
	fmt.Printf("[PgBankAccountRepository] CreateBankAccount called - BusinessID: %s, IFSC: %s, HolderName: %s\n",
		account.BusinessID, account.IFSCCode, account.AccountHolderName)
	// TODO-PHASE-4.4: Implement SQL INSERT with encryption
	// - Compute SHA256 hash of account_number for account_number_hash
	// - Encrypt account_number for account_number_encrypted (AES-256-GCM)
	// - INSERT INTO xcentro.bank_accounts (...) VALUES (...) RETURNING account_id
	// - account_id generated via uuidv7()
	// - Set account.AccountID with returned UUID
	// - Serialize metadata to JSONB
	return nil
}

// GetBankAccountByID retrieves a bank account by ID
func (r *PgBankAccountRepository) GetBankAccountByID(ctx context.Context, accountID string) (*payment.BankAccount, error) {
	fmt.Printf("[PgBankAccountRepository] GetBankAccountByID called - AccountID: %s\n", accountID)
	// TODO-PHASE-4.4: Implement SQL SELECT with decryption
	// - SELECT * FROM xcentro.bank_accounts WHERE account_id = $1
	// - Decrypt account_number_encrypted to account_number
	// - Deserialize JSONB metadata
	return nil, nil
}

// GetBankAccountsByBusinessID retrieves all verified active accounts for a business
func (r *PgBankAccountRepository) GetBankAccountsByBusinessID(ctx context.Context, businessID string) ([]*payment.BankAccount, error) {
	fmt.Printf("[PgBankAccountRepository] GetBankAccountsByBusinessID called - BusinessID: %s\n", businessID)
	// TODO-PHASE-4.4: Implement SQL SELECT WHERE is_active AND is_verified
	// - SELECT * FROM xcentro.bank_accounts WHERE business_id = $1 AND is_active = true AND is_verified = true ORDER BY added_at DESC
	// - Decrypt account_number for each result
	// - Return slice of BankAccount pointers
	return nil, nil
}

// UpdateVerificationStatus updates verification status and metadata
func (r *PgBankAccountRepository) UpdateVerificationStatus(ctx context.Context, accountID string, isVerified bool, metadata map[string]any) error {
	fmt.Printf("[PgBankAccountRepository] UpdateVerificationStatus called - AccountID: %s, IsVerified: %v\n", accountID, isVerified)
	// TODO-PHASE-4.4: Implement SQL UPDATE
	// - UPDATE xcentro.bank_accounts SET is_verified = $1, metadata = $2 WHERE account_id = $3
	// - Merge new metadata with existing metadata
	return nil
}

// DeactivateAccount marks an account as inactive (soft delete)
func (r *PgBankAccountRepository) DeactivateAccount(ctx context.Context, accountID string) error {
	fmt.Printf("[PgBankAccountRepository] DeactivateAccount called - AccountID: %s\n", accountID)
	// TODO-PHASE-4.4: Implement SQL UPDATE SET is_active = false
	// - UPDATE xcentro.bank_accounts SET is_active = false WHERE account_id = $1
	return nil
}

// CheckDuplicateAccount checks if account already exists via hash
func (r *PgBankAccountRepository) CheckDuplicateAccount(ctx context.Context, businessID, accountNumberHash string) (bool, error) {
	fmt.Printf("[PgBankAccountRepository] CheckDuplicateAccount called - BusinessID: %s, AccountNumberHash: %s\n", businessID, accountNumberHash)
	// TODO-PHASE-4.4: Implement SQL SELECT EXISTS with unique index check
	// - SELECT EXISTS(SELECT 1 FROM xcentro.bank_accounts WHERE business_id = $1 AND account_number_hash = $2)
	// - Use unique index: idx_bank_accounts_unique ON (business_id, account_number_hash)
	return false, nil
}