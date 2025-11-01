package payment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/payment"
	"xcentro.in/internal/shared/crypto"
)

// PgBankAccountRepository implements payment.BankAccountRepository using PostgreSQL
type PgBankAccountRepository struct {
	querier       client.Querier
	encryptionKey string
}

// NewPgBankAccountRepository creates a new PostgreSQL bank account repository
func NewPgBankAccountRepository(db client.DB, encryptionKey string) *PgBankAccountRepository {
	return &PgBankAccountRepository{
		querier:       db,
		encryptionKey: encryptionKey,
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgBankAccountRepository) WithTx(tx interface{}) payment.BankAccountRepository {
	return &PgBankAccountRepository{
		querier:       tx.(client.Tx),
		encryptionKey: r.encryptionKey,
	}
}

// CreateBankAccount creates a new bank account record
func (r *PgBankAccountRepository) CreateBankAccount(ctx context.Context, account *payment.BankAccount) error {
	fmt.Printf("[PgBankAccountRepository] CreateBankAccount called - BusinessID: %s, IFSC: %s, HolderName: %s\n",
		account.BusinessID, account.IFSCCode, account.AccountHolderName)

	// Compute SHA-256 hash for duplicate detection
	hash := sha256.Sum256([]byte(account.AccountNumber))
	accountNumberHash := hex.EncodeToString(hash[:])

	// Encrypt account number using AES-256-GCM
	encryptedAccountNumber, err := crypto.EncryptBankAccountNumber(account.AccountNumber, r.encryptionKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt account number: %w", err)
	}

	query := `
		INSERT INTO xcentro.bank_accounts (
			business_id, account_number_hash, account_number_encrypted,
			ifsc_code, account_holder_name, bank_name, branch_name, account_type,
			is_verified, is_active, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING account_id, added_at
	`

	var metadataJSON []byte
	if account.Metadata != nil {
		var err error
		metadataJSON, err = json.Marshal(account.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	err = r.querier.QueryRow(ctx, query,
		account.BusinessID,
		accountNumberHash,
		encryptedAccountNumber,
		account.IFSCCode,
		account.AccountHolderName,
		account.BankName,
		account.BranchName,
		account.AccountType,
		account.IsVerified,
		account.IsActive,
		metadataJSON,
	).Scan(&account.AccountID, &account.AddedAt)

	if err != nil {
		return fmt.Errorf("failed to create bank account: %w", err)
	}

	return nil
}

// GetBankAccountByID retrieves a bank account by ID
func (r *PgBankAccountRepository) GetBankAccountByID(ctx context.Context, accountID string) (*payment.BankAccount, error) {
	fmt.Printf("[PgBankAccountRepository] GetBankAccountByID called - AccountID: %s\n", accountID)

	query := `
		SELECT account_id, business_id, account_number_encrypted, ifsc_code,
		       account_holder_name, bank_name, branch_name, account_type,
		       is_verified, is_active, added_at, metadata
		FROM xcentro.bank_accounts
		WHERE account_id = $1 AND is_active = TRUE
	`

	var account payment.BankAccount
	var bankName, branchName, accountType *string
	var metadataJSON []byte
	var encryptedAccountNumber string

	err := r.querier.QueryRow(ctx, query, accountID).Scan(
		&account.AccountID,
		&account.BusinessID,
		&encryptedAccountNumber,
		&account.IFSCCode,
		&account.AccountHolderName,
		&bankName,
		&branchName,
		&accountType,
		&account.IsVerified,
		&account.IsActive,
		&account.AddedAt,
		&metadataJSON,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get bank account: %w", err)
	}

	// Decrypt account number
	decryptedAccountNumber, err := crypto.DecryptBankAccountNumber(encryptedAccountNumber, r.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt account number for account %s: %w", account.AccountID, err)
	}
	account.AccountNumber = decryptedAccountNumber

	// Handle NULL fields
	if bankName != nil {
		account.BankName = *bankName
	}
	if branchName != nil {
		account.BranchName = *branchName
	}
	if accountType != nil {
		account.AccountType = payment.BankAccountType(*accountType)
	}

	// Deserialize JSONB metadata
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &account.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	// Extract last five digits for display
	account.LastFiveDigits = crypto.ExtractLastDigits(account.AccountNumber, 5)

	return &account, nil
}

// GetBankAccountsByBusinessID retrieves all verified active accounts for a business
func (r *PgBankAccountRepository) GetBankAccountsByBusinessID(ctx context.Context, businessID string) ([]*payment.BankAccount, error) {
	fmt.Printf("[PgBankAccountRepository] GetBankAccountsByBusinessID called - BusinessID: %s\n", businessID)

	query := `
		SELECT account_id, business_id, account_number_encrypted, ifsc_code,
		       account_holder_name, bank_name, branch_name, account_type,
		       is_verified, is_active, added_at, metadata
		FROM xcentro.bank_accounts
		WHERE business_id = $1 AND is_active = TRUE AND is_verified = TRUE
		ORDER BY added_at DESC
	`

	rows, err := r.querier.Query(ctx, query, businessID)
	if err != nil {
		return nil, fmt.Errorf("failed to query bank accounts: %w", err)
	}
	defer rows.Close()

	var accounts []*payment.BankAccount
	for rows.Next() {
		var account payment.BankAccount
		var bankName, branchName, accountType *string
		var metadataJSON []byte
		var encryptedAccountNumber string

		err := rows.Scan(
			&account.AccountID,
			&account.BusinessID,
			&encryptedAccountNumber,
			&account.IFSCCode,
			&account.AccountHolderName,
			&bankName,
			&branchName,
			&accountType,
			&account.IsVerified,
			&account.IsActive,
			&account.AddedAt,
			&metadataJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan bank account: %w", err)
		}

		// Decrypt account number
		decryptedAccountNumber, err := crypto.DecryptBankAccountNumber(encryptedAccountNumber, r.encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt account number for account %s: %w", account.AccountID, err)
		}
		account.AccountNumber = decryptedAccountNumber

		// Handle NULL fields
		if bankName != nil {
			account.BankName = *bankName
		}
		if branchName != nil {
			account.BranchName = *branchName
		}
		if accountType != nil {
			account.AccountType = payment.BankAccountType(*accountType)
		}

		// Deserialize JSONB metadata
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &account.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		// Extract last five digits for display
		account.LastFiveDigits = crypto.ExtractLastDigits(account.AccountNumber, 5)

		accounts = append(accounts, &account)
	}

	return accounts, nil
}

// UpdateVerificationStatus updates verification status and metadata
func (r *PgBankAccountRepository) UpdateVerificationStatus(ctx context.Context, accountID string, isVerified bool, metadata map[string]any) error {
	fmt.Printf("[PgBankAccountRepository] UpdateVerificationStatus called - AccountID: %s, IsVerified: %v\n", accountID, isVerified)

	query := `
		UPDATE xcentro.bank_accounts
		SET is_verified = $1, metadata = COALESCE(metadata, '{}'::jsonb) || $2::jsonb
		WHERE account_id = $3
	`

	// Serialize metadata to JSONB
	var metadataJSON []byte
	if metadata != nil {
		var err error
		metadataJSON, err = json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	} else {
		metadataJSON = []byte("{}")
	}

	_, err := r.querier.Exec(ctx, query, isVerified, metadataJSON, accountID)
	if err != nil {
		return fmt.Errorf("failed to update verification status: %w", err)
	}

	return nil
}

// DeactivateAccount marks an account as inactive (soft delete)
func (r *PgBankAccountRepository) DeactivateAccount(ctx context.Context, accountID string) error {
	fmt.Printf("[PgBankAccountRepository] DeactivateAccount called - AccountID: %s\n", accountID)

	query := `UPDATE xcentro.bank_accounts SET is_active = FALSE WHERE account_id = $1`
	_, err := r.querier.Exec(ctx, query, accountID)
	if err != nil {
		return fmt.Errorf("failed to deactivate account: %w", err)
	}

	return nil
}

// CheckDuplicateAccount checks if account already exists via hash
func (r *PgBankAccountRepository) CheckDuplicateAccount(ctx context.Context, businessID, accountNumberHash string) (bool, error) {
	fmt.Printf("[PgBankAccountRepository] CheckDuplicateAccount called - BusinessID: %s, AccountNumberHash: %s\n", businessID, accountNumberHash)

	query := `
		SELECT EXISTS(
			SELECT 1 FROM xcentro.bank_accounts
			WHERE business_id = $1 AND account_number_hash = $2
		)
	`

	var exists bool
	err := r.querier.QueryRow(ctx, query, businessID, accountNumberHash).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check duplicate account: %w", err)
	}

	return exists, nil
}
