package payee

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/lib/pq"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/payee"
)

// ############################################################################
// # Biller Payee Repository
// ############################################################################

// PgPayeeBillerRepository implements payee.PayeeBillerRepository using PostgreSQL
type PgPayeeBillerRepository struct {
	querier client.Querier
}

// NewPgPayeeBillerRepository creates a new PostgreSQL payee biller repository
func NewPgPayeeBillerRepository(db client.DB) payee.PayeeBillerRepository {
	return &PgPayeeBillerRepository{
		querier: db,
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgPayeeBillerRepository) WithTx(tx client.Tx) payee.PayeeBillerRepository {
	return &PgPayeeBillerRepository{
		querier: tx,
	}
}

// Create creates a new biller payee
func (r *PgPayeeBillerRepository) Create(ctx context.Context, biller *payee.PayeeBiller) error {
	query := `
		INSERT INTO xcentro.payee_biller (
			payee_id, biller_id, biller_name, biller_category, biller_description,
			bbps_biller_code, npci_registered, supports_adhoc, supports_recurring,
			payment_modes, avg_processing_time_hours, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING created_at, updated_at
	`

	var metadataJSON []byte
	var err error

	if biller.Metadata != nil {
		metadataJSON, err = json.Marshal(biller.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	err = r.querier.QueryRow(ctx, query,
		biller.PayeeID,
		biller.BillerID,
		biller.BillerName,
		biller.BillerCategory,
		biller.BillerDescription,
		biller.BBPSBillerCode,
		biller.NPCIRegistered,
		biller.SupportsAdhoc,
		biller.SupportsRecurring,
		pq.Array(biller.PaymentModes),
		biller.AvgProcessingTimeHours,
		metadataJSON,
	).Scan(&biller.CreatedAt, &biller.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create biller payee: %w", err)
	}

	return nil
}

// FindByID retrieves a biller payee by payee ID
func (r *PgPayeeBillerRepository) FindByID(ctx context.Context, payeeID string) (*payee.PayeeBiller, error) {
	query := `
		SELECT payee_id, biller_id, biller_name, biller_category, biller_description,
		       bbps_biller_code, npci_registered, supports_adhoc, supports_recurring,
		       payment_modes, avg_processing_time_hours, created_at, updated_at, metadata
		FROM xcentro.payee_biller
		WHERE payee_id = $1
	`

	return r.scanBiller(r.querier.QueryRow(ctx, query, payeeID))
}

// FindByBillerID retrieves a biller payee by biller ID
func (r *PgPayeeBillerRepository) FindByBillerID(ctx context.Context, billerID string) (*payee.PayeeBiller, error) {
	query := `
		SELECT payee_id, biller_id, biller_name, biller_category, biller_description,
		       bbps_biller_code, npci_registered, supports_adhoc, supports_recurring,
		       payment_modes, avg_processing_time_hours, created_at, updated_at, metadata
		FROM xcentro.payee_biller
		WHERE biller_id = $1
	`

	return r.scanBiller(r.querier.QueryRow(ctx, query, billerID))
}

// Update updates a biller payee
func (r *PgPayeeBillerRepository) Update(ctx context.Context, biller *payee.PayeeBiller) error {
	query := `
		UPDATE xcentro.payee_biller
		SET biller_name = $2, biller_category = $3, biller_description = $4,
		    bbps_biller_code = $5, npci_registered = $6, supports_adhoc = $7,
		    supports_recurring = $8, payment_modes = $9, avg_processing_time_hours = $10, metadata = $11
		WHERE payee_id = $1
	`

	var metadataJSON []byte
	var err error

	if biller.Metadata != nil {
		metadataJSON, err = json.Marshal(biller.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	result, err := r.querier.Exec(ctx, query,
		biller.PayeeID,
		biller.BillerName,
		biller.BillerCategory,
		biller.BillerDescription,
		biller.BBPSBillerCode,
		biller.NPCIRegistered,
		biller.SupportsAdhoc,
		biller.SupportsRecurring,
		pq.Array(biller.PaymentModes),
		biller.AvgProcessingTimeHours,
		metadataJSON,
	)

	if err != nil {
		return fmt.Errorf("failed to update biller payee: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("biller payee not found")
	}

	return nil
}

// Search searches for biller payees with filters
func (r *PgPayeeBillerRepository) Search(ctx context.Context, filters payee.BillerSearchFilters) ([]*payee.PayeeBiller, error) {
	query := `
		SELECT payee_id, biller_id, biller_name, biller_category, biller_description,
		       bbps_biller_code, npci_registered, supports_adhoc, supports_recurring,
		       payment_modes, avg_processing_time_hours, created_at, updated_at, metadata
		FROM xcentro.payee_biller
		WHERE 1=1
	`

	args := []interface{}{}
	argIdx := 1

	if filters.BillerName != "" {
		query += fmt.Sprintf(" AND biller_name ILIKE $%d", argIdx)
		args = append(args, "%"+filters.BillerName+"%")
		argIdx++
	}

	if filters.BillerCategory != "" {
		query += fmt.Sprintf(" AND biller_category = $%d", argIdx)
		args = append(args, filters.BillerCategory)
		argIdx++
	}

	if filters.BillerID != "" {
		query += fmt.Sprintf(" AND biller_id = $%d", argIdx)
		args = append(args, filters.BillerID)
		argIdx++
	}

	query += " ORDER BY created_at DESC"

	if filters.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIdx)
		args = append(args, filters.Limit)
		argIdx++
	}

	if filters.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argIdx)
		args = append(args, filters.Offset)
	}

	return r.scanBillers(r.querier.Query(ctx, query, args...))
}

// FindByCategory retrieves all billers in a category
func (r *PgPayeeBillerRepository) FindByCategory(ctx context.Context, category string) ([]*payee.PayeeBiller, error) {
	query := `
		SELECT payee_id, biller_id, biller_name, biller_category, biller_description,
		       bbps_biller_code, npci_registered, supports_adhoc, supports_recurring,
		       payment_modes, avg_processing_time_hours, created_at, updated_at, metadata
		FROM xcentro.payee_biller
		WHERE biller_category = $1
		ORDER BY biller_name ASC
	`

	return r.scanBillers(r.querier.Query(ctx, query, category))
}

// Helper function to scan a single biller
func (r *PgPayeeBillerRepository) scanBiller(row pgx.Row) (*payee.PayeeBiller, error) {
	var biller payee.PayeeBiller
	var billerDescription sql.NullString
	var bbpsBillerCode sql.NullString
	var avgProcessingTimeHours sql.NullInt32
	var updatedAt sql.NullTime
	var metadataJSON []byte

	err := row.Scan(
		&biller.PayeeID,
		&biller.BillerID,
		&biller.BillerName,
		&biller.BillerCategory,
		&billerDescription,
		&bbpsBillerCode,
		&biller.NPCIRegistered,
		&biller.SupportsAdhoc,
		&biller.SupportsRecurring,
		pq.Array(&biller.PaymentModes),
		&avgProcessingTimeHours,
		&biller.CreatedAt,
		&updatedAt,
		&metadataJSON,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("biller payee not found")
		}
		return nil, fmt.Errorf("failed to scan biller payee: %w", err)
	}

	biller.BillerDescription = billerDescription.String
	biller.BBPSBillerCode = bbpsBillerCode.String
	if avgProcessingTimeHours.Valid {
		biller.AvgProcessingTimeHours = int(avgProcessingTimeHours.Int32)
	}
	if updatedAt.Valid {
		biller.UpdatedAt = updatedAt.Time
	}

	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &biller.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &biller, nil
}

// Helper function to scan multiple billers
func (r *PgPayeeBillerRepository) scanBillers(rows pgx.Rows, err error) ([]*payee.PayeeBiller, error) {
	if err != nil {
		return nil, fmt.Errorf("failed to query biller payees: %w", err)
	}
	defer rows.Close()

	var billers []*payee.PayeeBiller

	for rows.Next() {
		var biller payee.PayeeBiller
		var billerDescription sql.NullString
		var bbpsBillerCode sql.NullString
		var avgProcessingTimeHours sql.NullInt32
		var updatedAt sql.NullTime
		var metadataJSON []byte

		err := rows.Scan(
			&biller.PayeeID,
			&biller.BillerID,
			&biller.BillerName,
			&biller.BillerCategory,
			&billerDescription,
			&bbpsBillerCode,
			&biller.NPCIRegistered,
			&biller.SupportsAdhoc,
			&biller.SupportsRecurring,
			pq.Array(&biller.PaymentModes),
			&avgProcessingTimeHours,
			&biller.CreatedAt,
			&updatedAt,
			&metadataJSON,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan biller payee: %w", err)
		}

		biller.BillerDescription = billerDescription.String
		biller.BBPSBillerCode = bbpsBillerCode.String
		if avgProcessingTimeHours.Valid {
			biller.AvgProcessingTimeHours = int(avgProcessingTimeHours.Int32)
		}
		if updatedAt.Valid {
			biller.UpdatedAt = updatedAt.Time
		}

		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &biller.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		billers = append(billers, &biller)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating biller payees: %w", err)
	}

	return billers, nil
}

// ############################################################################
// # Biller Account Repository
// ############################################################################

// PgPayeeBillerAccountRepository implements payee.PayeeBillerAccountRepository using PostgreSQL
type PgPayeeBillerAccountRepository struct {
	querier client.Querier
}

// NewPgPayeeBillerAccountRepository creates a new PostgreSQL biller account repository
func NewPgPayeeBillerAccountRepository(db client.DB) payee.PayeeBillerAccountRepository {
	return &PgPayeeBillerAccountRepository{
		querier: db,
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgPayeeBillerAccountRepository) WithTx(tx client.Tx) payee.PayeeBillerAccountRepository {
	return &PgPayeeBillerAccountRepository{
		querier: tx,
	}
}

// Create creates a new biller account
func (r *PgPayeeBillerAccountRepository) Create(ctx context.Context, account *payee.PayeeBillerAccount) error {
	query := `
		INSERT INTO xcentro.payee_biller_accounts (
			biller_account_id, payee_id, business_id, mobile_number, customer_params,
			auto_fetch_enabled, fetch_schedule, preferred_fetch_day,
			last_bill_fetched_at, last_bill_amount, last_bill_due_date, is_active, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING created_at, updated_at
	`

	var customerParamsJSON, metadataJSON []byte
	var err error

	if account.CustomerParams != nil {
		customerParamsJSON, err = json.Marshal(account.CustomerParams)
		if err != nil {
			return fmt.Errorf("failed to marshal customer_params: %w", err)
		}
	}

	if account.Metadata != nil {
		metadataJSON, err = json.Marshal(account.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	err = r.querier.QueryRow(ctx, query,
		account.BillerAccountID,
		account.PayeeID,
		account.BusinessID,
		account.MobileNumber,
		customerParamsJSON,
		account.AutoFetchEnabled,
		account.FetchSchedule,
		account.PreferredFetchDay,
		account.LastBillFetchedAt,
		account.LastBillAmount,
		account.LastBillDueDate,
		account.IsActive,
		metadataJSON,
	).Scan(&account.CreatedAt, &account.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create biller account: %w", err)
	}

	return nil
}

// FindByID retrieves a biller account by ID
func (r *PgPayeeBillerAccountRepository) FindByID(ctx context.Context, billerAccountID string) (*payee.PayeeBillerAccount, error) {
	query := `
		SELECT biller_account_id, payee_id, business_id, mobile_number, customer_params,
		       auto_fetch_enabled, fetch_schedule, preferred_fetch_day,
		       last_bill_fetched_at, last_bill_amount, last_bill_due_date,
		       is_active, created_at, updated_at, metadata
		FROM xcentro.payee_biller_accounts
		WHERE biller_account_id = $1
	`

	return r.scanBillerAccount(r.querier.QueryRow(ctx, query, billerAccountID))
}

// Update updates a biller account
func (r *PgPayeeBillerAccountRepository) Update(ctx context.Context, account *payee.PayeeBillerAccount) error {
	query := `
		UPDATE xcentro.payee_biller_accounts
		SET mobile_number = $2, customer_params = $3, auto_fetch_enabled = $4,
		    fetch_schedule = $5, preferred_fetch_day = $6, last_bill_fetched_at = $7,
		    last_bill_amount = $8, last_bill_due_date = $9, is_active = $10, metadata = $11
		WHERE biller_account_id = $1
	`

	var customerParamsJSON, metadataJSON []byte
	var err error

	if account.CustomerParams != nil {
		customerParamsJSON, err = json.Marshal(account.CustomerParams)
		if err != nil {
			return fmt.Errorf("failed to marshal customer_params: %w", err)
		}
	}

	if account.Metadata != nil {
		metadataJSON, err = json.Marshal(account.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	result, err := r.querier.Exec(ctx, query,
		account.BillerAccountID,
		account.MobileNumber,
		customerParamsJSON,
		account.AutoFetchEnabled,
		account.FetchSchedule,
		account.PreferredFetchDay,
		account.LastBillFetchedAt,
		account.LastBillAmount,
		account.LastBillDueDate,
		account.IsActive,
		metadataJSON,
	)

	if err != nil {
		return fmt.Errorf("failed to update biller account: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("biller account not found")
	}

	return nil
}

// Delete deletes a biller account (soft delete)
func (r *PgPayeeBillerAccountRepository) Delete(ctx context.Context, billerAccountID string) error {
	query := `
		UPDATE xcentro.payee_biller_accounts
		SET is_active = false
		WHERE biller_account_id = $1
	`

	result, err := r.querier.Exec(ctx, query, billerAccountID)
	if err != nil {
		return fmt.Errorf("failed to delete biller account: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("biller account not found")
	}

	return nil
}

// FindByBusinessAndPayee retrieves all biller accounts for a business-payee combination
func (r *PgPayeeBillerAccountRepository) FindByBusinessAndPayee(ctx context.Context, businessID, payeeID string) ([]*payee.PayeeBillerAccount, error) {
	query := `
		SELECT biller_account_id, payee_id, business_id, mobile_number, customer_params,
		       auto_fetch_enabled, fetch_schedule, preferred_fetch_day,
		       last_bill_fetched_at, last_bill_amount, last_bill_due_date,
		       is_active, created_at, updated_at, metadata
		FROM xcentro.payee_biller_accounts
		WHERE business_id = $1 AND payee_id = $2
		ORDER BY created_at DESC
	`

	return r.scanBillerAccounts(r.querier.Query(ctx, query, businessID, payeeID))
}

// FindByBusiness retrieves all biller accounts for a business
func (r *PgPayeeBillerAccountRepository) FindByBusiness(ctx context.Context, businessID string) ([]*payee.PayeeBillerAccount, error) {
	query := `
		SELECT biller_account_id, payee_id, business_id, mobile_number, customer_params,
		       auto_fetch_enabled, fetch_schedule, preferred_fetch_day,
		       last_bill_fetched_at, last_bill_amount, last_bill_due_date,
		       is_active, created_at, updated_at, metadata
		FROM xcentro.payee_biller_accounts
		WHERE business_id = $1
		ORDER BY created_at DESC
	`

	return r.scanBillerAccounts(r.querier.Query(ctx, query, businessID))
}

// CheckDuplicate checks if a biller account already exists
func (r *PgPayeeBillerAccountRepository) CheckDuplicate(ctx context.Context, businessID, payeeID, mobileNumber string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM xcentro.payee_biller_accounts
			WHERE business_id = $1 AND payee_id = $2 AND mobile_number = $3
		)
	`

	var exists bool
	err := r.querier.QueryRow(ctx, query, businessID, payeeID, mobileNumber).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check duplicate biller account: %w", err)
	}

	return exists, nil
}

// FindAccountsForAutoFetch retrieves all biller accounts with auto-fetch enabled
func (r *PgPayeeBillerAccountRepository) FindAccountsForAutoFetch(ctx context.Context) ([]*payee.PayeeBillerAccount, error) {
	query := `
		SELECT biller_account_id, payee_id, business_id, mobile_number, customer_params,
		       auto_fetch_enabled, fetch_schedule, preferred_fetch_day,
		       last_bill_fetched_at, last_bill_amount, last_bill_due_date,
		       is_active, created_at, updated_at, metadata
		FROM xcentro.payee_biller_accounts
		WHERE auto_fetch_enabled = TRUE AND is_active = TRUE
		ORDER BY last_bill_fetched_at ASC NULLS FIRST
	`

	return r.scanBillerAccounts(r.querier.Query(ctx, query))
}

// Helper function to scan a single biller account
func (r *PgPayeeBillerAccountRepository) scanBillerAccount(row pgx.Row) (*payee.PayeeBillerAccount, error) {
	var account payee.PayeeBillerAccount
	var fetchSchedule sql.NullString
	var preferredFetchDay sql.NullInt32
	var lastBillFetchedAt sql.NullTime
	var lastBillAmount sql.NullInt64
	var lastBillDueDate sql.NullTime
	var updatedAt sql.NullTime
	var customerParamsJSON, metadataJSON []byte

	err := row.Scan(
		&account.BillerAccountID,
		&account.PayeeID,
		&account.BusinessID,
		&account.MobileNumber,
		&customerParamsJSON,
		&account.AutoFetchEnabled,
		&fetchSchedule,
		&preferredFetchDay,
		&lastBillFetchedAt,
		&lastBillAmount,
		&lastBillDueDate,
		&account.IsActive,
		&account.CreatedAt,
		&updatedAt,
		&metadataJSON,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("biller account not found")
		}
		return nil, fmt.Errorf("failed to scan biller account: %w", err)
	}

	account.FetchSchedule = fetchSchedule.String
	if preferredFetchDay.Valid {
		account.PreferredFetchDay = int(preferredFetchDay.Int32)
	}
	if lastBillFetchedAt.Valid {
		account.LastBillFetchedAt = &lastBillFetchedAt.Time
	}
	if lastBillAmount.Valid {
		account.LastBillAmount = lastBillAmount.Int64
	}
	if lastBillDueDate.Valid {
		account.LastBillDueDate = &lastBillDueDate.Time
	}
	if updatedAt.Valid {
		account.UpdatedAt = updatedAt.Time
	}

	if len(customerParamsJSON) > 0 {
		if err := json.Unmarshal(customerParamsJSON, &account.CustomerParams); err != nil {
			return nil, fmt.Errorf("failed to unmarshal customer_params: %w", err)
		}
	}

	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &account.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &account, nil
}

// Helper function to scan multiple biller accounts
func (r *PgPayeeBillerAccountRepository) scanBillerAccounts(rows pgx.Rows, err error) ([]*payee.PayeeBillerAccount, error) {
	if err != nil {
		return nil, fmt.Errorf("failed to query biller accounts: %w", err)
	}
	defer rows.Close()

	var accounts []*payee.PayeeBillerAccount

	for rows.Next() {
		var account payee.PayeeBillerAccount
		var fetchSchedule sql.NullString
		var preferredFetchDay sql.NullInt32
		var lastBillFetchedAt sql.NullTime
		var lastBillAmount sql.NullInt64
		var lastBillDueDate sql.NullTime
		var updatedAt sql.NullTime
		var customerParamsJSON, metadataJSON []byte

		err := rows.Scan(
			&account.BillerAccountID,
			&account.PayeeID,
			&account.BusinessID,
			&account.MobileNumber,
			&customerParamsJSON,
			&account.AutoFetchEnabled,
			&fetchSchedule,
			&preferredFetchDay,
			&lastBillFetchedAt,
			&lastBillAmount,
			&lastBillDueDate,
			&account.IsActive,
			&account.CreatedAt,
			&updatedAt,
			&metadataJSON,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan biller account: %w", err)
		}

		account.FetchSchedule = fetchSchedule.String
		if preferredFetchDay.Valid {
			account.PreferredFetchDay = int(preferredFetchDay.Int32)
		}
		if lastBillFetchedAt.Valid {
			account.LastBillFetchedAt = &lastBillFetchedAt.Time
		}
		if lastBillAmount.Valid {
			account.LastBillAmount = lastBillAmount.Int64
		}
		if lastBillDueDate.Valid {
			account.LastBillDueDate = &lastBillDueDate.Time
		}
		if updatedAt.Valid {
			account.UpdatedAt = updatedAt.Time
		}

		if len(customerParamsJSON) > 0 {
			if err := json.Unmarshal(customerParamsJSON, &account.CustomerParams); err != nil {
				return nil, fmt.Errorf("failed to unmarshal customer_params: %w", err)
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
		return nil, fmt.Errorf("error iterating biller accounts: %w", err)
	}

	return accounts, nil
}
