package payee

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/payee"
)

// PgPayeeGSTRepository implements payee.PayeeGSTRepository using PostgreSQL
type PgPayeeGSTRepository struct {
	querier client.Querier
}

// NewPgPayeeGSTRepository creates a new PostgreSQL payee GST repository
func NewPgPayeeGSTRepository(db client.DB) payee.PayeeGSTRepository {
	return &PgPayeeGSTRepository{
		querier: db,
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgPayeeGSTRepository) WithTx(tx client.Tx) payee.PayeeGSTRepository {
	return &PgPayeeGSTRepository{
		querier: tx,
	}
}

// GetGovernmentPayee retrieves the government GST payee (singleton)
func (r *PgPayeeGSTRepository) GetGovernmentPayee(ctx context.Context) (*payee.PayeeGST, error) {
	query := `
		SELECT payee_id, government_entity_name, gst_portal_url, created_at, updated_at, metadata
		FROM xcentro.payee_gst
		LIMIT 1
	`

	var gst payee.PayeeGST
	var updatedAt sql.NullTime
	var metadataJSON []byte

	err := r.querier.QueryRow(ctx, query).Scan(
		&gst.PayeeID,
		&gst.GovernmentEntityName,
		&gst.GSTPortalURL,
		&gst.CreatedAt,
		&updatedAt,
		&metadataJSON,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("government GST payee not found")
		}
		return nil, fmt.Errorf("failed to get government GST payee: %w", err)
	}

	if updatedAt.Valid {
		gst.UpdatedAt = updatedAt.Time
	}

	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &gst.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &gst, nil
}

// Update updates the government GST payee (singleton)
func (r *PgPayeeGSTRepository) Update(ctx context.Context, gst *payee.PayeeGST) error {
	query := `
		UPDATE xcentro.payee_gst
		SET government_entity_name = $2, gst_portal_url = $3, metadata = $4
		WHERE payee_id = $1
	`

	var metadataJSON []byte
	var err error

	if gst.Metadata != nil {
		metadataJSON, err = json.Marshal(gst.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	result, err := r.querier.Exec(ctx, query,
		gst.PayeeID,
		gst.GovernmentEntityName,
		gst.GSTPortalURL,
		metadataJSON,
	)

	if err != nil {
		return fmt.Errorf("failed to update government GST payee: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("government GST payee not found")
	}

	return nil
}

// PgPayeeGSTVirtualAccountRepository implements payee.PayeeGSTVirtualAccountRepository using PostgreSQL
type PgPayeeGSTVirtualAccountRepository struct {
	querier client.Querier
}

// NewPgPayeeGSTVirtualAccountRepository creates a new PostgreSQL GST virtual account repository
func NewPgPayeeGSTVirtualAccountRepository(db client.DB) payee.PayeeGSTVirtualAccountRepository {
	return &PgPayeeGSTVirtualAccountRepository{
		querier: db,
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgPayeeGSTVirtualAccountRepository) WithTx(tx client.Tx) payee.PayeeGSTVirtualAccountRepository {
	return &PgPayeeGSTVirtualAccountRepository{
		querier: tx,
	}
}

// Create creates a new GST virtual account
func (r *PgPayeeGSTVirtualAccountRepository) Create(ctx context.Context, account *payee.PayeeGSTVirtualAccount) error {
	query := `
		INSERT INTO xcentro.payee_gst_virtual_accounts (
			virtual_account_id, payee_id, virtual_account_number, ifsc_code, bank_name,
			tax_period, financial_year, tax_type, valid_from, valid_until, is_active,
			challan_document_url, uploaded_at, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING created_at
	`

	var metadataJSON []byte
	var err error

	if account.Metadata != nil {
		metadataJSON, err = json.Marshal(account.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	err = r.querier.QueryRow(ctx, query,
		account.VirtualAccountID,
		account.PayeeID,
		account.VirtualAccountNumber,
		account.IFSCCode,
		account.BankName,
		account.TaxPeriod,
		account.FinancialYear,
		account.TaxType,
		account.ValidFrom,
		account.ValidUntil,
		account.IsActive,
		account.ChallanDocumentURL,
		account.UploadedAt,
		metadataJSON,
	).Scan(&account.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create GST virtual account: %w", err)
	}

	return nil
}

// FindByID retrieves a GST virtual account by ID
func (r *PgPayeeGSTVirtualAccountRepository) FindByID(ctx context.Context, virtualAccountID string) (*payee.PayeeGSTVirtualAccount, error) {
	query := `
		SELECT virtual_account_id, payee_id, virtual_account_number, ifsc_code, bank_name,
		       tax_period, financial_year, tax_type, valid_from, valid_until, is_active,
		       challan_document_url, uploaded_at, created_at, updated_at, metadata
		FROM xcentro.payee_gst_virtual_accounts
		WHERE virtual_account_id = $1
	`

	return r.scanVirtualAccount(r.querier.QueryRow(ctx, query, virtualAccountID))
}

// FindByVirtualAccountNumber retrieves a GST virtual account by virtual account number
func (r *PgPayeeGSTVirtualAccountRepository) FindByVirtualAccountNumber(ctx context.Context, virtualAccountNumber string) (*payee.PayeeGSTVirtualAccount, error) {
	query := `
		SELECT virtual_account_id, payee_id, virtual_account_number, ifsc_code, bank_name,
		       tax_period, financial_year, tax_type, valid_from, valid_until, is_active,
		       challan_document_url, uploaded_at, created_at, updated_at, metadata
		FROM xcentro.payee_gst_virtual_accounts
		WHERE virtual_account_number = $1
	`

	return r.scanVirtualAccount(r.querier.QueryRow(ctx, query, virtualAccountNumber))
}

// Update updates a GST virtual account
func (r *PgPayeeGSTVirtualAccountRepository) Update(ctx context.Context, account *payee.PayeeGSTVirtualAccount) error {
	query := `
		UPDATE xcentro.payee_gst_virtual_accounts
		SET tax_period = $2, financial_year = $3, tax_type = $4, valid_from = $5, valid_until = $6,
		    is_active = $7, challan_document_url = $8, uploaded_at = $9, metadata = $10
		WHERE virtual_account_id = $1
	`

	var metadataJSON []byte
	var err error

	if account.Metadata != nil {
		metadataJSON, err = json.Marshal(account.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	result, err := r.querier.Exec(ctx, query,
		account.VirtualAccountID,
		account.TaxPeriod,
		account.FinancialYear,
		account.TaxType,
		account.ValidFrom,
		account.ValidUntil,
		account.IsActive,
		account.ChallanDocumentURL,
		account.UploadedAt,
		metadataJSON,
	)

	if err != nil {
		return fmt.Errorf("failed to update GST virtual account: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("GST virtual account not found")
	}

	return nil
}

// FindByPayeeID retrieves all GST virtual accounts for a payee
func (r *PgPayeeGSTVirtualAccountRepository) FindByPayeeID(ctx context.Context, payeeID string) ([]*payee.PayeeGSTVirtualAccount, error) {
	query := `
		SELECT virtual_account_id, payee_id, virtual_account_number, ifsc_code, bank_name,
		       tax_period, financial_year, tax_type, valid_from, valid_until, is_active,
		       challan_document_url, uploaded_at, created_at, updated_at, metadata
		FROM xcentro.payee_gst_virtual_accounts
		WHERE payee_id = $1
		ORDER BY created_at DESC
	`

	return r.scanVirtualAccounts(r.querier.Query(ctx, query, payeeID))
}

// FindByTaxPeriod retrieves a GST virtual account by tax period
func (r *PgPayeeGSTVirtualAccountRepository) FindByTaxPeriod(ctx context.Context, payeeID, taxPeriod string) (*payee.PayeeGSTVirtualAccount, error) {
	query := `
		SELECT virtual_account_id, payee_id, virtual_account_number, ifsc_code, bank_name,
		       tax_period, financial_year, tax_type, valid_from, valid_until, is_active,
		       challan_document_url, uploaded_at, created_at, updated_at, metadata
		FROM xcentro.payee_gst_virtual_accounts
		WHERE payee_id = $1 AND tax_period = $2
		LIMIT 1
	`

	return r.scanVirtualAccount(r.querier.QueryRow(ctx, query, payeeID, taxPeriod))
}

// FindByFinancialYear retrieves all GST virtual accounts for a financial year
func (r *PgPayeeGSTVirtualAccountRepository) FindByFinancialYear(ctx context.Context, payeeID, financialYear string) ([]*payee.PayeeGSTVirtualAccount, error) {
	query := `
		SELECT virtual_account_id, payee_id, virtual_account_number, ifsc_code, bank_name,
		       tax_period, financial_year, tax_type, valid_from, valid_until, is_active,
		       challan_document_url, uploaded_at, created_at, updated_at, metadata
		FROM xcentro.payee_gst_virtual_accounts
		WHERE payee_id = $1 AND financial_year = $2
		ORDER BY created_at DESC
	`

	return r.scanVirtualAccounts(r.querier.Query(ctx, query, payeeID, financialYear))
}

// FindActiveAccounts retrieves all active GST virtual accounts for a payee
func (r *PgPayeeGSTVirtualAccountRepository) FindActiveAccounts(ctx context.Context, payeeID string) ([]*payee.PayeeGSTVirtualAccount, error) {
	query := `
		SELECT virtual_account_id, payee_id, virtual_account_number, ifsc_code, bank_name,
		       tax_period, financial_year, tax_type, valid_from, valid_until, is_active,
		       challan_document_url, uploaded_at, created_at, updated_at, metadata
		FROM xcentro.payee_gst_virtual_accounts
		WHERE payee_id = $1 AND is_active = TRUE
		ORDER BY created_at DESC
	`

	return r.scanVirtualAccounts(r.querier.Query(ctx, query, payeeID))
}

// CountByFinancialYear counts GST virtual accounts by financial year
func (r *PgPayeeGSTVirtualAccountRepository) CountByFinancialYear(ctx context.Context, financialYear string) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM xcentro.payee_gst_virtual_accounts
		WHERE financial_year = $1
	`

	var count int
	err := r.querier.QueryRow(ctx, query, financialYear).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count GST virtual accounts: %w", err)
	}

	return count, nil
}

// Helper function to scan a single virtual account
func (r *PgPayeeGSTVirtualAccountRepository) scanVirtualAccount(row pgx.Row) (*payee.PayeeGSTVirtualAccount, error) {
	var account payee.PayeeGSTVirtualAccount
	var taxType sql.NullString
	var challanDocumentURL sql.NullString
	var uploadedAt sql.NullTime
	var updatedAt sql.NullTime
	var metadataJSON []byte

	err := row.Scan(
		&account.VirtualAccountID,
		&account.PayeeID,
		&account.VirtualAccountNumber,
		&account.IFSCCode,
		&account.BankName,
		&account.TaxPeriod,
		&account.FinancialYear,
		&taxType,
		&account.ValidFrom,
		&account.ValidUntil,
		&account.IsActive,
		&challanDocumentURL,
		&uploadedAt,
		&account.CreatedAt,
		&updatedAt,
		&metadataJSON,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("GST virtual account not found")
		}
		return nil, fmt.Errorf("failed to scan GST virtual account: %w", err)
	}

	account.TaxType = taxType.String
	account.ChallanDocumentURL = challanDocumentURL.String
	if uploadedAt.Valid {
		account.UploadedAt = &uploadedAt.Time
	}
	if updatedAt.Valid {
		account.UpdatedAt = updatedAt.Time
	}

	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &account.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &account, nil
}

// Helper function to scan multiple virtual accounts
func (r *PgPayeeGSTVirtualAccountRepository) scanVirtualAccounts(rows pgx.Rows, err error) ([]*payee.PayeeGSTVirtualAccount, error) {
	if err != nil {
		return nil, fmt.Errorf("failed to query GST virtual accounts: %w", err)
	}
	defer rows.Close()

	var accounts []*payee.PayeeGSTVirtualAccount

	for rows.Next() {
		var account payee.PayeeGSTVirtualAccount
		var taxType sql.NullString
		var challanDocumentURL sql.NullString
		var uploadedAt sql.NullTime
		var updatedAt sql.NullTime
		var metadataJSON []byte

		err := rows.Scan(
			&account.VirtualAccountID,
			&account.PayeeID,
			&account.VirtualAccountNumber,
			&account.IFSCCode,
			&account.BankName,
			&account.TaxPeriod,
			&account.FinancialYear,
			&taxType,
			&account.ValidFrom,
			&account.ValidUntil,
			&account.IsActive,
			&challanDocumentURL,
			&uploadedAt,
			&account.CreatedAt,
			&updatedAt,
			&metadataJSON,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan GST virtual account: %w", err)
		}

		account.TaxType = taxType.String
		account.ChallanDocumentURL = challanDocumentURL.String
		if uploadedAt.Valid {
			account.UploadedAt = &uploadedAt.Time
		}
		if updatedAt.Valid {
			account.UpdatedAt = updatedAt.Time
		}

		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &account.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		accounts = append(accounts, &account)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating GST virtual accounts: %w", err)
	}

	return accounts, nil
}
