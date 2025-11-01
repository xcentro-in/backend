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
	"xcentro.in/internal/shared/crypto"
	"xcentro.in/internal/shared/logger"
)

// PgPayeeBankAccountRepository implements payee.PayeeBankAccountRepository using PostgreSQL
type PgPayeeBankAccountRepository struct {
	querier       client.Querier
	logger        *logger.Logger
	encryptionKey string
}

// NewPgPayeeBankAccountRepository creates a new PostgreSQL payee bank account repository
func NewPgPayeeBankAccountRepository(db client.DB, encryptionKey string) payee.PayeeBankAccountRepository {
	return &PgPayeeBankAccountRepository{
		querier:       db,
		logger:        logger.GetDefault(),
		encryptionKey: encryptionKey,
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgPayeeBankAccountRepository) WithTx(tx client.Tx) payee.PayeeBankAccountRepository {
	return &PgPayeeBankAccountRepository{
		querier:       tx,
		logger:        r.logger,
		encryptionKey: r.encryptionKey,
	}
}

// CreateBankAccount creates a new payee bank account record
func (r *PgPayeeBankAccountRepository) CreateBankAccount(ctx context.Context, account *payee.PayeeBankAccount) error {
	// Compute SHA-256 hash for duplicate detection
	hash := sha256.Sum256([]byte(account.AccountNumber))
	accountNumberHash := hex.EncodeToString(hash[:])

	// Encrypt account number using AES-256-GCM
	encryptedAccountNumber, err := crypto.EncryptBankAccountNumber(account.AccountNumber, r.encryptionKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt account number: %w", err)
	}

	query := `
		INSERT INTO xcentro.payee_bank_accounts (
			payee_id, account_number_encrypted, account_number_hash,
			ifsc_code, bank_name, branch_name, account_type, account_holder_name,
			is_verified, verified_at, bank_verification_status, bank_verification_data,
			cross_verification_status,
			castler_beneficiary_id,
			is_active, account_label, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		RETURNING account_id, added_at
	`

	var bankVerificationDataJSON, metadataJSON []byte

	if account.VerificationData != nil {
		bankVerificationDataJSON, err = json.Marshal(account.VerificationData)
		if err != nil {
			r.logger.ErrorWith("failed to marshal bank_verification_data",
				"error", err,
				"payee_id", account.PayeeID)
			return fmt.Errorf("failed to marshal bank_verification_data: %w", err)
		}
	}

	if account.Metadata != nil {
		metadataJSON, err = json.Marshal(account.Metadata)
		if err != nil {
			r.logger.ErrorWith("failed to marshal metadata",
				"error", err,
				"payee_id", account.PayeeID)
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	err = r.querier.QueryRow(ctx, query,
		account.PayeeID,
		encryptedAccountNumber,
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
		account.CastlerBeneficiaryID,
		account.IsActive,
		account.AccountLabel,
		metadataJSON,
	).Scan(&account.AccountID, &account.AddedAt)

	if err != nil {
		r.logger.ErrorWith("failed to create payee bank account",
			"error", err,
			"payee_id", account.PayeeID,
			"ifsc_code", account.IFSCCode)
		return fmt.Errorf("failed to create payee bank account: %w", err)
	}

	// Store the hash in the entity for future reference
	account.AccountNumberHash = accountNumberHash

	r.logger.InfoWith("payee bank account created",
		"account_id", account.AccountID,
		"payee_id", account.PayeeID,
		"ifsc_code", account.IFSCCode)
	return nil
}

// FindByID retrieves a payee bank account by ID
func (r *PgPayeeBankAccountRepository) FindByID(ctx context.Context, accountID string) (*payee.PayeeBankAccount, error) {
	query := `
		SELECT account_id, payee_id, account_number_encrypted, account_number_hash,
		       ifsc_code, bank_name, branch_name, account_type, account_holder_name,
		       is_verified, verified_at, bank_verification_status, bank_verification_data,
		       cross_verification_status,
		       castler_beneficiary_id,
		       is_active, account_label, metadata, added_at, updated_at
		FROM xcentro.payee_bank_accounts
		WHERE account_id = $1
	`

	var account payee.PayeeBankAccount
	var bankName, branchName, accountLabel sql.NullString
	var verifiedAt sql.NullTime
	var castlerBeneficiaryID sql.NullString
	var bankVerificationDataJSON, metadataJSON []byte
	var updatedAt sql.NullTime
	var encryptedAccountNumber string

	err := r.querier.QueryRow(ctx, query, accountID).Scan(
		&account.AccountID,
		&account.PayeeID,
		&encryptedAccountNumber,
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
		&castlerBeneficiaryID,
		&account.IsActive,
		&accountLabel,
		&metadataJSON,
		&account.AddedAt,
		&updatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			r.logger.DebugWith("payee bank account not found",
				"account_id", accountID)
			return nil, fmt.Errorf("payee bank account not found")
		}
		r.logger.ErrorWith("failed to find payee bank account",
			"error", err,
			"account_id", accountID)
		return nil, fmt.Errorf("failed to find payee bank account: %w", err)
	}

	// Decrypt account number
	decryptedAccountNumber, err := crypto.DecryptBankAccountNumber(encryptedAccountNumber, r.encryptionKey)
	if err != nil {
		r.logger.ErrorWith("Failed to decrypt account number",
			"account_id", accountID,
			"error", err)
		return nil, fmt.Errorf("failed to decrypt account number: %w", err)
	}
	account.AccountNumber = decryptedAccountNumber

	// Handle nullable fields
	account.BankName = bankName.String
	account.BranchName = branchName.String
	account.AccountLabel = accountLabel.String
	account.CastlerBeneficiaryID = castlerBeneficiaryID.String
	if verifiedAt.Valid {
		account.VerifiedAt = &verifiedAt.Time
	}
	if updatedAt.Valid {
		account.UpdatedAt = updatedAt.Time
	}

	// Deserialize JSONB fields
	if len(bankVerificationDataJSON) > 0 {
		if err := json.Unmarshal(bankVerificationDataJSON, &account.VerificationData); err != nil {
			r.logger.ErrorWith("failed to unmarshal bank_verification_data",
				"error", err,
				"account_id", accountID)
			return nil, fmt.Errorf("failed to unmarshal bank_verification_data: %w", err)
		}
	}
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &account.Metadata); err != nil {
			r.logger.ErrorWith("failed to unmarshal metadata",
				"error", err,
				"account_id", accountID)
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &account, nil
}

// FindByPayeeID retrieves all bank accounts for a payee
func (r *PgPayeeBankAccountRepository) FindByPayeeID(ctx context.Context, payeeID string) ([]*payee.PayeeBankAccount, error) {
	query := `
		SELECT account_id, payee_id, account_number_encrypted, account_number_hash,
		       ifsc_code, bank_name, branch_name, account_type, account_holder_name,
		       is_verified, verified_at, bank_verification_status, bank_verification_data,
		       cross_verification_status,
		       castler_beneficiary_id,
		       is_active, account_label, metadata, added_at, updated_at
		FROM xcentro.payee_bank_accounts
		WHERE payee_id = $1 AND is_active = TRUE
		ORDER BY added_at DESC
	`

	rows, err := r.querier.Query(ctx, query, payeeID)
	if err != nil {
		r.logger.ErrorWith("failed to find payee bank accounts",
			"error", err,
			"payee_id", payeeID)
		return nil, fmt.Errorf("failed to find payee bank accounts: %w", err)
	}
	defer rows.Close()

	var accounts []*payee.PayeeBankAccount

	for rows.Next() {
		var account payee.PayeeBankAccount
		var bankName, branchName, accountLabel sql.NullString
		var verifiedAt sql.NullTime
		var castlerBeneficiaryID sql.NullString
		var bankVerificationDataJSON, metadataJSON []byte
		var updatedAt sql.NullTime
		var encryptedAccountNumber string

		err := rows.Scan(
			&account.AccountID,
			&account.PayeeID,
			&encryptedAccountNumber,
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
			&castlerBeneficiaryID,
			&account.IsActive,
			&accountLabel,
			&metadataJSON,
			&account.AddedAt,
			&updatedAt,
		)

		if err != nil {
			r.logger.ErrorWith("failed to scan payee bank account",
				"error", err,
				"payee_id", payeeID)
			return nil, fmt.Errorf("failed to scan payee bank account: %w", err)
		}

		// Decrypt account number
		decryptedAccountNumber, err := crypto.DecryptBankAccountNumber(encryptedAccountNumber, r.encryptionKey)
		if err != nil {
			r.logger.ErrorWith("Failed to decrypt account number",
				"account_id", account.AccountID,
				"payee_id", payeeID,
				"error", err)
			return nil, fmt.Errorf("failed to decrypt account number for account %s: %w", account.AccountID, err)
		}
		account.AccountNumber = decryptedAccountNumber

		// Handle nullable fields
		account.BankName = bankName.String
		account.BranchName = branchName.String
		account.AccountLabel = accountLabel.String
		account.CastlerBeneficiaryID = castlerBeneficiaryID.String
		if verifiedAt.Valid {
			account.VerifiedAt = &verifiedAt.Time
		}
		if updatedAt.Valid {
			account.UpdatedAt = updatedAt.Time
		}

		// Deserialize JSONB fields
		if len(bankVerificationDataJSON) > 0 {
			if err := json.Unmarshal(bankVerificationDataJSON, &account.VerificationData); err != nil {
				r.logger.ErrorWith("failed to unmarshal bank_verification_data",
					"error", err,
					"payee_id", payeeID)
				return nil, fmt.Errorf("failed to unmarshal bank_verification_data: %w", err)
			}
		}
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &account.Metadata); err != nil {
				r.logger.ErrorWith("failed to unmarshal metadata",
					"error", err,
					"payee_id", payeeID)
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		accounts = append(accounts, &account)
	}

	if err := rows.Err(); err != nil {
		r.logger.ErrorWith("error iterating payee bank accounts",
			"error", err,
			"payee_id", payeeID)
		return nil, fmt.Errorf("error iterating payee bank accounts: %w", err)
	}

	return accounts, nil
}

// UpdateBankAccount updates a payee bank account record
func (r *PgPayeeBankAccountRepository) UpdateBankAccount(ctx context.Context, account *payee.PayeeBankAccount) error {
	query := `
		UPDATE xcentro.payee_bank_accounts
		SET is_verified = $2, verified_at = $3, bank_verification_status = $4, bank_verification_data = $5,
		    cross_verification_status = $6,
		    castler_beneficiary_id = $7,
		    is_active = $8, account_label = $9, metadata = $10
		WHERE account_id = $1
	`

	var bankVerificationDataJSON, metadataJSON []byte
	var err error

	if account.VerificationData != nil {
		bankVerificationDataJSON, err = json.Marshal(account.VerificationData)
		if err != nil {
			r.logger.ErrorWith("failed to marshal bank_verification_data",
				"error", err,
				"account_id", account.AccountID)
			return fmt.Errorf("failed to marshal bank_verification_data: %w", err)
		}
	}

	if account.Metadata != nil {
		metadataJSON, err = json.Marshal(account.Metadata)
		if err != nil {
			r.logger.ErrorWith("failed to marshal metadata",
				"error", err,
				"account_id", account.AccountID)
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
		account.CastlerBeneficiaryID,
		account.IsActive,
		account.AccountLabel,
		metadataJSON,
	)

	if err != nil {
		r.logger.ErrorWith("failed to update payee bank account",
			"error", err,
			"account_id", account.AccountID)
		return fmt.Errorf("failed to update payee bank account: %w", err)
	}

	if result.RowsAffected() == 0 {
		r.logger.DebugWith("payee bank account not found for update",
			"account_id", account.AccountID)
		return fmt.Errorf("payee bank account not found")
	}

	r.logger.InfoWith("payee bank account updated",
		"account_id", account.AccountID,
		"is_verified", account.IsVerified)
	return nil
}

// CheckDuplicateAccount checks if account with same hash already exists for this payee
func (r *PgPayeeBankAccountRepository) CheckDuplicateAccount(ctx context.Context, payeeID, accountHash string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM xcentro.payee_bank_accounts
			WHERE payee_id = $1 AND account_number_hash = $2
		)
	`

	var exists bool
	err := r.querier.QueryRow(ctx, query, payeeID, accountHash).Scan(&exists)
	if err != nil {
		r.logger.ErrorWith("failed to check duplicate payee bank account",
			"error", err,
			"payee_id", payeeID)
		return false, fmt.Errorf("failed to check duplicate payee bank account: %w", err)
	}

	return exists, nil
}
