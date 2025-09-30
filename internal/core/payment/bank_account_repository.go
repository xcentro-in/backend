package payment

import "context"

// BankAccountRepository defines database operations for bank accounts
type BankAccountRepository interface {
	// Core CRUD
	CreateBankAccount(ctx context.Context, account *BankAccount) error
	GetBankAccountByID(ctx context.Context, accountID string) (*BankAccount, error)
	GetBankAccountsByBusinessID(ctx context.Context, businessID string) ([]*BankAccount, error)

	// Account lifecycle management
	UpdateVerificationStatus(ctx context.Context, accountID string, isVerified bool, metadata map[string]any) error
	DeactivateAccount(ctx context.Context, accountID string) error

	// Duplicate detection
	CheckDuplicateAccount(ctx context.Context, businessID, accountNumberHash string) (bool, error)

	// Transaction support
	WithTx(tx interface{}) BankAccountRepository
}