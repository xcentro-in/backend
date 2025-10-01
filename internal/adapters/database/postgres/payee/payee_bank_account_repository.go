package payee

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/payee"
)

// PgPayeeBankAccountRepository implements payee.PayeeBankAccountRepository using PostgreSQL
type PgPayeeBankAccountRepository struct {
	querier client.Querier
}

// NewPgPayeeBankAccountRepository creates a new PostgreSQL payee bank account repository
func NewPgPayeeBankAccountRepository(db client.DB) payee.PayeeBankAccountRepository {
	return &PgPayeeBankAccountRepository{
		querier: db,
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgPayeeBankAccountRepository) WithTx(tx client.Tx) payee.PayeeBankAccountRepository {
	return &PgPayeeBankAccountRepository{
		querier: tx,
	}
}

// CreateBankAccount creates a new payee bank account record
func (r *PgPayeeBankAccountRepository) CreateBankAccount(ctx context.Context, account *payee.PayeeBankAccount) error {
	fmt.Printf("[PgPayeeBankAccountRepository] CreateBankAccount called - PayeeID: %s, IFSC: %s, HolderName: %s\n",
		account.PayeeID, account.IFSCCode, account.AccountHolderName)

	// Compute SHA-256 hash for duplicate detection
	hash := sha256.Sum256([]byte(account.AccountNumber))
	accountNumberHash := hex.EncodeToString(hash[:])

	// Phase 5.3: Store plaintext in encrypted field (encryption in Phase 5.5)
	accountNumberPlaintext := account.AccountNumber

	query := `
		INSERT INTO xcentro.payee_bank_accounts (
			payee_id, account_number_encrypted, account_number_hash,
			ifsc_code, bank_name, branch_name, account_type, account_holder_name,
			is_verified, verified_at, bank_verification_status, bank_verification_data,
			cross_verification_status, name_match_score,
			is_active, is_primary, account_label, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		RETURNING account_id, added_at
	`

	var bankVerificationDataJSON, metadataJSON []byte
	var err error

	if account.VerificationData != nil {
		bankVerificationDataJSON, err = json.Marshal(account.VerificationData)
		if err != nil {
			return fmt.Errorf("failed to marshal bank_verification_data: %w", err)
		}
	}

	if account.Metadata != nil {
		metadataJSON, err = json.Marshal(account.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	err = r.querier.QueryRow(ctx, query,
		account.PayeeID,
		accountNumberPlaintext, // TODO-PHASE-5.5: Replace with encryption
		accountNumberHash,
		account.IFSCCode,
		account.BankName,
		account.BranchName,
		account.AccountType,
		account.AccountHolderName,
		account.IsVerified,
		account.VerifiedAt,
		account.BankVerificationStatus,
		bankVerificationDataJSON,
		account.CrossVerificationStatus,
		account.NameMatchScore,
		account.IsActive,
		account.IsPrimary,
		account.AccountLabel,
		metadataJSON,
	).Scan(&account.AccountID, &account.AddedAt)

	if err != nil {
		return fmt.Errorf("failed to create payee bank account: %w", err)
	}

	// Store the hash in the entity for future reference
	account.AccountNumberHash = accountNumberHash

	fmt.Printf("[PgPayeeBankAccountRepository] Bank account created with ID: %s\n", account.AccountID)
	return nil
}

// FindByID retrieves a payee bank account by ID
func (r *PgPayeeBankAccountRepository) FindByID(ctx context.Context, accountID string) (*payee.PayeeBankAccount, error) {
	fmt.Printf("[PgPayeeBankAccountRepository] FindByID called - AccountID: %s\n", accountID)

	query := `
		SELECT account_id, payee_id, account_number_encrypted, account_number_hash,
		       ifsc_code, bank_name, branch_name, account_type, account_holder_name,
		       is_verified, verified_at, bank_verification_status, bank_verification_data,
		       cross_verification_status, name_match_score,
		       is_active, is_primary, account_label, metadata, added_at, updated_at
		FROM xcentro.payee_bank_accounts
		WHERE account_id = $1
	`

	var account payee.PayeeBankAccount
	var bankName, branchName, accountLabel sql.NullString
	var verifiedAt sql.NullTime
	var nameMatchScore sql.NullFloat64
	var bankVerificationDataJSON, metadataJSON []byte
	var updatedAt sql.NullTime

	err := r.querier.QueryRow(ctx, query, accountID).Scan(
		&account.AccountID,
		&account.PayeeID,
		&account.AccountNumber,
		&account.AccountNumberHash,
		&account.IFSCCode,
		&bankName,
		&branchName,
		&account.AccountType,
		&account.AccountHolderName,
		&account.IsVerified,
		&verifiedAt,
		&account.BankVerificationStatus,
		&bankVerificationDataJSON,
		&account.CrossVerificationStatus,
		&nameMatchScore,
		&account.IsActive,
		&account.IsPrimary,
		&accountLabel,
		&metadataJSON,
		&account.AddedAt,
		&updatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("payee bank account not found")
		}
		return nil, fmt.Errorf("failed to find payee bank account: %w", err)
	}

	// Handle nullable fields
	account.BankName = bankName.String
	account.BranchName = branchName.String
	account.AccountLabel = accountLabel.String
	account.NameMatchScore = nameMatchScore.Float64
	if verifiedAt.Valid {
		account.VerifiedAt = &verifiedAt.Time
	}
	if updatedAt.Valid {
		account.UpdatedAt = updatedAt.Time
	}

	// Deserialize JSONB fields
	if len(bankVerificationDataJSON) > 0 {
		if err := json.Unmarshal(bankVerificationDataJSON, &account.VerificationData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal bank_verification_data: %w", err)
		}
	}
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &account.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &account, nil
}

// FindByPayeeID retrieves all bank accounts for a payee
func (r *PgPayeeBankAccountRepository) FindByPayeeID(ctx context.Context, payeeID string) ([]*payee.PayeeBankAccount, error) {
	fmt.Printf("[PgPayeeBankAccountRepository] FindByPayeeID called - PayeeID: %s\n", payeeID)

	query := `
		SELECT account_id, payee_id, account_number_encrypted, account_number_hash,
		       ifsc_code, bank_name, branch_name, account_type, account_holder_name,
		       is_verified, verified_at, bank_verification_status, bank_verification_data,
		       cross_verification_status, name_match_score,
		       is_active, is_primary, account_label, metadata, added_at, updated_at
		FROM xcentro.payee_bank_accounts
		WHERE payee_id = $1 AND is_active = TRUE
		ORDER BY is_primary DESC, added_at DESC
	`

	rows, err := r.querier.Query(ctx, query, payeeID)
	if err != nil {
		return nil, fmt.Errorf("failed to find payee bank accounts: %w", err)
	}
	defer rows.Close()

	var accounts []*payee.PayeeBankAccount

	for rows.Next() {
		var account payee.PayeeBankAccount
		var bankName, branchName, accountLabel sql.NullString
		var verifiedAt sql.NullTime
		var nameMatchScore sql.NullFloat64
		var bankVerificationDataJSON, metadataJSON []byte
		var updatedAt sql.NullTime

		err := rows.Scan(
			&account.AccountID,
			&account.PayeeID,
			&account.AccountNumber,
			&account.AccountNumberHash,
			&account.IFSCCode,
			&bankName,
			&branchName,
			&account.AccountType,
			&account.AccountHolderName,
			&account.IsVerified,
			&verifiedAt,
			&account.BankVerificationStatus,
			&bankVerificationDataJSON,
			&account.CrossVerificationStatus,
			&nameMatchScore,
			&account.IsActive,
			&account.IsPrimary,
			&accountLabel,
			&metadataJSON,
			&account.AddedAt,
			&updatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan payee bank account: %w", err)
		}

		// Handle nullable fields
		account.BankName = bankName.String
		account.BranchName = branchName.String
		account.AccountLabel = accountLabel.String
		account.NameMatchScore = nameMatchScore.Float64
		if verifiedAt.Valid {
			account.VerifiedAt = &verifiedAt.Time
		}
		if updatedAt.Valid {
			account.UpdatedAt = updatedAt.Time
		}

		// Deserialize JSONB fields
		if len(bankVerificationDataJSON) > 0 {
			if err := json.Unmarshal(bankVerificationDataJSON, &account.VerificationData); err != nil {
				return nil, fmt.Errorf("failed to unmarshal bank_verification_data: %w", err)
			}
		}
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &account.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		accounts = append(accounts, &account)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating payee bank accounts: %w", err)
	}

	return accounts, nil
}

// UpdateBankAccount updates a payee bank account record
func (r *PgPayeeBankAccountRepository) UpdateBankAccount(ctx context.Context, account *payee.PayeeBankAccount) error {
	fmt.Printf("[PgPayeeBankAccountRepository] UpdateBankAccount called - AccountID: %s\n", account.AccountID)

	query := `
		UPDATE xcentro.payee_bank_accounts
		SET is_verified = $2, verified_at = $3, bank_verification_status = $4, bank_verification_data = $5,
		    cross_verification_status = $6, name_match_score = $7,
		    is_active = $8, is_primary = $9, account_label = $10, metadata = $11
		WHERE account_id = $1
	`

	var bankVerificationDataJSON, metadataJSON []byte
	var err error

	if account.VerificationData != nil {
		bankVerificationDataJSON, err = json.Marshal(account.VerificationData)
		if err != nil {
			return fmt.Errorf("failed to marshal bank_verification_data: %w", err)
		}
	}

	if account.Metadata != nil {
		metadataJSON, err = json.Marshal(account.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	result, err := r.querier.Exec(ctx, query,
		account.AccountID,
		account.IsVerified,
		account.VerifiedAt,
		account.BankVerificationStatus,
		bankVerificationDataJSON,
		account.CrossVerificationStatus,
		account.NameMatchScore,
		account.IsActive,
		account.IsPrimary,
		account.AccountLabel,
		metadataJSON,
	)

	if err != nil {
		return fmt.Errorf("failed to update payee bank account: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("payee bank account not found")
	}

	return nil
}

// CheckDuplicateAccount checks if account with same hash already exists for this payee
func (r *PgPayeeBankAccountRepository) CheckDuplicateAccount(ctx context.Context, payeeID, accountHash string) (bool, error) {
	fmt.Printf("[PgPayeeBankAccountRepository] CheckDuplicateAccount called - PayeeID: %s\n", payeeID)

	query := `
		SELECT EXISTS(
			SELECT 1 FROM xcentro.payee_bank_accounts
			WHERE payee_id = $1 AND account_number_hash = $2
		)
	`

	var exists bool
	err := r.querier.QueryRow(ctx, query, payeeID, accountHash).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check duplicate payee bank account: %w", err)
	}

	return exists, nil
}

// GetPrimaryAccount retrieves the primary bank account for a payee
func (r *PgPayeeBankAccountRepository) GetPrimaryAccount(ctx context.Context, payeeID string) (*payee.PayeeBankAccount, error) {
	fmt.Printf("[PgPayeeBankAccountRepository] GetPrimaryAccount called - PayeeID: %s\n", payeeID)

	query := `
		SELECT account_id, payee_id, account_number_encrypted, account_number_hash,
		       ifsc_code, bank_name, branch_name, account_type, account_holder_name,
		       is_verified, verified_at, bank_verification_status, bank_verification_data,
		       cross_verification_status, name_match_score,
		       is_active, is_primary, account_label, metadata, added_at, updated_at
		FROM xcentro.payee_bank_accounts
		WHERE payee_id = $1 AND is_primary = TRUE AND is_active = TRUE
	`

	var account payee.PayeeBankAccount
	var bankName, branchName, accountLabel sql.NullString
	var verifiedAt sql.NullTime
	var nameMatchScore sql.NullFloat64
	var bankVerificationDataJSON, metadataJSON []byte
	var updatedAt sql.NullTime

	err := r.querier.QueryRow(ctx, query, payeeID).Scan(
		&account.AccountID,
		&account.PayeeID,
		&account.AccountNumber,
		&account.AccountNumberHash,
		&account.IFSCCode,
		&bankName,
		&branchName,
		&account.AccountType,
		&account.AccountHolderName,
		&account.IsVerified,
		&verifiedAt,
		&account.BankVerificationStatus,
		&bankVerificationDataJSON,
		&account.CrossVerificationStatus,
		&nameMatchScore,
		&account.IsActive,
		&account.IsPrimary,
		&accountLabel,
		&metadataJSON,
		&account.AddedAt,
		&updatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // No primary account (normal case)
		}
		return nil, fmt.Errorf("failed to find primary payee bank account: %w", err)
	}

	// Handle nullable fields
	account.BankName = bankName.String
	account.BranchName = branchName.String
	account.AccountLabel = accountLabel.String
	account.NameMatchScore = nameMatchScore.Float64
	if verifiedAt.Valid {
		account.VerifiedAt = &verifiedAt.Time
	}
	if updatedAt.Valid {
		account.UpdatedAt = updatedAt.Time
	}

	// Deserialize JSONB fields
	if len(bankVerificationDataJSON) > 0 {
		if err := json.Unmarshal(bankVerificationDataJSON, &account.VerificationData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal bank_verification_data: %w", err)
		}
	}
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &account.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &account, nil
}

// SetPrimaryAccount sets an account as primary (and unsets all others for this payee)
// Should be called within a transaction for atomicity
func (r *PgPayeeBankAccountRepository) SetPrimaryAccount(ctx context.Context, accountID string) error {
	fmt.Printf("[PgPayeeBankAccountRepository] SetPrimaryAccount called - AccountID: %s\n", accountID)

	// First, get the payee_id for this account
	var payeeID string
	err := r.querier.QueryRow(ctx, `
		SELECT payee_id FROM xcentro.payee_bank_accounts WHERE account_id = $1
	`, accountID).Scan(&payeeID)

	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("payee bank account not found")
		}
		return fmt.Errorf("failed to find payee bank account: %w", err)
	}

	// Unset all primary flags for this payee
	_, err = r.querier.Exec(ctx, `
		UPDATE xcentro.payee_bank_accounts
		SET is_primary = FALSE
		WHERE payee_id = $1
	`, payeeID)

	if err != nil {
		return fmt.Errorf("failed to unset primary flags: %w", err)
	}

	// Set the specified account as primary
	result, err := r.querier.Exec(ctx, `
		UPDATE xcentro.payee_bank_accounts
		SET is_primary = TRUE
		WHERE account_id = $1
	`, accountID)

	if err != nil {
		return fmt.Errorf("failed to set primary account: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("payee bank account not found")
	}

	return nil
}
