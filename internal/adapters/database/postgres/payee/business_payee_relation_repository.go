package payee

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/payee"
	"xcentro.in/internal/shared/crypto"
	"xcentro.in/internal/shared/logger"
)

// PgBusinessPayeeRelationRepository implements payee.BusinessPayeeRelationRepository using PostgreSQL
type PgBusinessPayeeRelationRepository struct {
	querier       client.Querier
	logger        *logger.Logger
	encryptionKey string
}

// NewPgBusinessPayeeRelationRepository creates a new PostgreSQL business-payee relation repository
func NewPgBusinessPayeeRelationRepository(db client.DB, encryptionKey string) payee.BusinessPayeeRelationRepository {
	return &PgBusinessPayeeRelationRepository{
		querier:       db,
		logger:        logger.GetDefault(),
		encryptionKey: encryptionKey,
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgBusinessPayeeRelationRepository) WithTx(tx client.Tx) payee.BusinessPayeeRelationRepository {
	return &PgBusinessPayeeRelationRepository{
		querier:       tx,
		logger:        r.logger,
		encryptionKey: r.encryptionKey,
	}
}

// CreateRelation creates a new business-payee relation
func (r *PgBusinessPayeeRelationRepository) CreateRelation(ctx context.Context, relation *payee.BusinessPayeeRelation) error {
	query := `
		INSERT INTO xcentro.business_payee_relation (
			business_id, payee_id, account_id,
			display_name, contact_email, contact_phone, notes,
			is_active, is_favorite, tags,
			added_by, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING relation_id, added_at
	`

	var metadataJSON []byte
	var err error

	if relation.Metadata != nil {
		metadataJSON, err = json.Marshal(relation.Metadata)
		if err != nil {
			r.logger.ErrorWith("failed to marshal metadata",
				"error", err,
				"business_id", relation.BusinessID,
				"payee_id", relation.PayeeID,
				"account_id", relation.AccountID)
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	// Handle empty AddedBy as NULL (UUID field)
	var addedBy interface{}
	if relation.AddedBy != "" {
		addedBy = relation.AddedBy
	} else {
		addedBy = nil
	}

	err = r.querier.QueryRow(ctx, query,
		relation.BusinessID,
		relation.PayeeID,
		relation.AccountID,
		relation.DisplayName,
		relation.ContactEmail,
		relation.ContactPhone,
		relation.Notes,
		relation.IsActive,
		relation.IsFavorite,
		relation.Tags, // PostgreSQL array (pgx/v5 handles []string natively)
		addedBy,
		metadataJSON,
	).Scan(&relation.RelationID, &relation.AddedAt)

	if err != nil {
		r.logger.ErrorWith("failed to create business-payee relation",
			"error", err,
			"business_id", relation.BusinessID,
			"payee_id", relation.PayeeID,
			"account_id", relation.AccountID)
		return fmt.Errorf("failed to create business-payee relation: %w", err)
	}

	r.logger.InfoWith("business-payee relation created",
		"relation_id", relation.RelationID,
		"business_id", relation.BusinessID,
		"payee_id", relation.PayeeID,
		"account_id", relation.AccountID)
	return nil
}

// FindRelationByID retrieves a business-payee relation by its relation_id (surrogate key)
func (r *PgBusinessPayeeRelationRepository) FindRelationByID(ctx context.Context, relationID string) (*payee.BusinessPayeeRelation, error) {
	query := `
		SELECT relation_id, business_id, payee_id, account_id,
		       display_name, contact_email, contact_phone, notes,
		       is_active, is_favorite, tags,
		       added_at, added_by, last_used_at, updated_at, metadata
		FROM xcentro.business_payee_relation
		WHERE relation_id = $1
	`

	var relation payee.BusinessPayeeRelation
	var contactEmail, contactPhone, notes sql.NullString
	var addedBy sql.NullString
	var lastUsedAt, updatedAt sql.NullTime
	var metadataJSON []byte

	err := r.querier.QueryRow(ctx, query, relationID).Scan(
		&relation.RelationID,
		&relation.BusinessID,
		&relation.PayeeID,
		&relation.AccountID,
		&relation.DisplayName,
		&contactEmail,
		&contactPhone,
		&notes,
		&relation.IsActive,
		&relation.IsFavorite,
		&relation.Tags,
		&relation.AddedAt,
		&addedBy,
		&lastUsedAt,
		&updatedAt,
		&metadataJSON,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			r.logger.DebugWith("business-payee relation not found",
				"relation_id", relationID)
			return nil, fmt.Errorf("business-payee relation not found")
		}
		r.logger.ErrorWith("failed to find business-payee relation",
			"error", err,
			"relation_id", relationID)
		return nil, fmt.Errorf("failed to find business-payee relation: %w", err)
	}

	// Handle nullable fields
	relation.ContactEmail = contactEmail.String
	relation.ContactPhone = contactPhone.String
	relation.Notes = notes.String
	relation.AddedBy = addedBy.String
	if lastUsedAt.Valid {
		relation.LastUsedAt = &lastUsedAt.Time
	}
	if updatedAt.Valid {
		relation.UpdatedAt = updatedAt.Time
	}

	// Deserialize JSONB
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &relation.Metadata); err != nil {
			r.logger.ErrorWith("failed to unmarshal metadata",
				"error", err,
				"relation_id", relationID)
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &relation, nil
}

// FindRelation retrieves a specific business-payee relation
func (r *PgBusinessPayeeRelationRepository) FindRelation(ctx context.Context, businessID, payeeID, accountID string) (*payee.BusinessPayeeRelation, error) {
	query := `
		SELECT relation_id, business_id, payee_id, account_id,
		       display_name, contact_email, contact_phone, notes,
		       is_active, is_favorite, tags,
		       added_at, added_by, last_used_at, updated_at, metadata
		FROM xcentro.business_payee_relation
		WHERE business_id = $1 AND payee_id = $2 AND account_id = $3
	`

	var relation payee.BusinessPayeeRelation
	var contactEmail, contactPhone, notes sql.NullString
	var addedBy sql.NullString
	var lastUsedAt, updatedAt sql.NullTime
	var metadataJSON []byte

	err := r.querier.QueryRow(ctx, query, businessID, payeeID, accountID).Scan(
		&relation.RelationID,
		&relation.BusinessID,
		&relation.PayeeID,
		&relation.AccountID,
		&relation.DisplayName,
		&contactEmail,
		&contactPhone,
		&notes,
		&relation.IsActive,
		&relation.IsFavorite,
		&relation.Tags,
		&relation.AddedAt,
		&addedBy,
		&lastUsedAt,
		&updatedAt,
		&metadataJSON,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			r.logger.DebugWith("business-payee relation not found",
				"business_id", businessID,
				"payee_id", payeeID,
				"account_id", accountID)
			return nil, fmt.Errorf("business-payee relation not found")
		}
		r.logger.ErrorWith("failed to find business-payee relation",
			"error", err,
			"business_id", businessID,
			"payee_id", payeeID,
			"account_id", accountID)
		return nil, fmt.Errorf("failed to find business-payee relation: %w", err)
	}

	// Handle nullable fields
	relation.ContactEmail = contactEmail.String
	relation.ContactPhone = contactPhone.String
	relation.Notes = notes.String
	relation.AddedBy = addedBy.String
	if lastUsedAt.Valid {
		relation.LastUsedAt = &lastUsedAt.Time
	}
	if updatedAt.Valid {
		relation.UpdatedAt = updatedAt.Time
	}

	// Deserialize JSONB
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &relation.Metadata); err != nil {
			r.logger.ErrorWith("failed to unmarshal metadata",
				"error", err,
				"business_id", businessID,
				"payee_id", payeeID,
				"account_id", accountID)
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &relation, nil
}

// FindByBusiness retrieves all payee relations for a business with filters
func (r *PgBusinessPayeeRelationRepository) FindByBusiness(ctx context.Context, businessID string, filters payee.RelationSearchFilters) ([]*payee.BusinessPayeeRelation, error) {

	query := `
		SELECT r.relation_id, r.business_id, r.payee_id, r.account_id,
		       r.display_name, r.contact_email, r.contact_phone, r.notes,
		       r.is_active, r.is_favorite, r.tags,
		       r.added_at, r.added_by, r.last_used_at, r.updated_at, r.metadata,
			   m.payee_type
		FROM xcentro.business_payee_relation r
		INNER JOIN xcentro.payee_master m ON r.payee_id = m.payee_id
		WHERE r.business_id = $1
	`

	args := []interface{}{businessID}
	argIdx := 2

	// Add filters
	if filters.DisplayName != "" {
		query += fmt.Sprintf(" AND r.display_name ILIKE $%d", argIdx)
		args = append(args, "%"+filters.DisplayName+"%")
		argIdx++
	}

	if filters.PayeeType != "" {
		query += fmt.Sprintf(" AND m.payee_type = $%d", argIdx)
		args = append(args, filters.PayeeType)
		argIdx++
	}

	if filters.IsActive != nil {
		query += fmt.Sprintf(" AND r.is_active = $%d", argIdx)
		args = append(args, *filters.IsActive)
		argIdx++
	}

	if filters.IsFavorite != nil {
		query += fmt.Sprintf(" AND r.is_favorite = $%d", argIdx)
		args = append(args, *filters.IsFavorite)
		argIdx++
	}

	if len(filters.Tags) > 0 {
		query += fmt.Sprintf(" AND r.tags && $%d", argIdx) // Array overlap operator
		args = append(args, filters.Tags)
		argIdx++
	}

	// Add ordering and pagination
	query += " ORDER BY r.is_favorite DESC, r.display_name ASC"

	if filters.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIdx)
		args = append(args, filters.Limit)
		argIdx++
	}

	if filters.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argIdx)
		args = append(args, filters.Offset)
	}

	rows, err := r.querier.Query(ctx, query, args...)
	if err != nil {
		r.logger.ErrorWith("failed to find business-payee relations",
			"error", err,
			"business_id", businessID)
		return nil, fmt.Errorf("failed to find business-payee relations: %w", err)
	}
	defer rows.Close()

	var relations []*payee.BusinessPayeeRelation

	for rows.Next() {
		var relation payee.BusinessPayeeRelation
		var contactEmail, contactPhone, notes sql.NullString
		var addedBy sql.NullString
		var lastUsedAt, updatedAt sql.NullTime
		var metadataJSON []byte

		err := rows.Scan(
			&relation.RelationID,
			&relation.BusinessID,
			&relation.PayeeID,
			&relation.AccountID,
			&relation.DisplayName,
			&contactEmail,
			&contactPhone,
			&notes,
			&relation.IsActive,
			&relation.IsFavorite,
			&relation.Tags,
			&relation.AddedAt,
			&addedBy,
			&lastUsedAt,
			&updatedAt,
			&metadataJSON,
			&relation.PayeeType, // Scan payee_type from joined payee_master table
		)

		if err != nil {
			r.logger.ErrorWith("failed to scan business-payee relation",
				"error", err,
				"business_id", businessID)
			return nil, fmt.Errorf("failed to scan business-payee relation: %w", err)
		}

		// Handle nullable fields
		relation.ContactEmail = contactEmail.String
		relation.ContactPhone = contactPhone.String
		relation.Notes = notes.String
		relation.AddedBy = addedBy.String
		if lastUsedAt.Valid {
			relation.LastUsedAt = &lastUsedAt.Time
		}
		if updatedAt.Valid {
			relation.UpdatedAt = updatedAt.Time
		}

		// Deserialize JSONB
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &relation.Metadata); err != nil {
				r.logger.ErrorWith("failed to unmarshal metadata",
					"error", err,
					"business_id", businessID)
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		relations = append(relations, &relation)
	}

	if err := rows.Err(); err != nil {
		r.logger.ErrorWith("error iterating business-payee relations",
			"error", err,
			"business_id", businessID)
		return nil, fmt.Errorf("error iterating business-payee relations: %w", err)
	}

	return relations, nil
}

// UpdateRelation updates a business-payee relation
func (r *PgBusinessPayeeRelationRepository) UpdateRelation(ctx context.Context, relation *payee.BusinessPayeeRelation) error {
	query := `
		UPDATE xcentro.business_payee_relation
		SET display_name = $4, contact_email = $5, contact_phone = $6, notes = $7,
		    is_active = $8, is_favorite = $9, tags = $10, metadata = $11
		WHERE business_id = $1 AND payee_id = $2 AND account_id = $3
	`

	var metadataJSON []byte
	var err error

	if relation.Metadata != nil {
		metadataJSON, err = json.Marshal(relation.Metadata)
		if err != nil {
			r.logger.ErrorWith("failed to marshal metadata",
				"error", err,
				"business_id", relation.BusinessID,
				"payee_id", relation.PayeeID,
				"account_id", relation.AccountID)
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	result, err := r.querier.Exec(ctx, query,
		relation.BusinessID,
		relation.PayeeID,
		relation.AccountID,
		relation.DisplayName,
		relation.ContactEmail,
		relation.ContactPhone,
		relation.Notes,
		relation.IsActive,
		relation.IsFavorite,
		relation.Tags,
		metadataJSON,
	)

	if err != nil {
		r.logger.ErrorWith("failed to update business-payee relation",
			"error", err,
			"business_id", relation.BusinessID,
			"payee_id", relation.PayeeID,
			"account_id", relation.AccountID)
		return fmt.Errorf("failed to update business-payee relation: %w", err)
	}

	if result.RowsAffected() == 0 {
		r.logger.DebugWith("business-payee relation not found for update",
			"business_id", relation.BusinessID,
			"payee_id", relation.PayeeID,
			"account_id", relation.AccountID)
		return fmt.Errorf("business-payee relation not found")
	}

	r.logger.InfoWith("business-payee relation updated",
		"business_id", relation.BusinessID,
		"payee_id", relation.PayeeID,
		"account_id", relation.AccountID)
	return nil
}

// DeleteRelation deletes (soft delete) a business-payee relation
func (r *PgBusinessPayeeRelationRepository) DeleteRelation(ctx context.Context, businessID, payeeID, accountID string) error {
	// Soft delete: set is_active = FALSE
	query := `
		UPDATE xcentro.business_payee_relation
		SET is_active = FALSE
		WHERE business_id = $1 AND payee_id = $2 AND account_id = $3
	`

	result, err := r.querier.Exec(ctx, query, businessID, payeeID, accountID)
	if err != nil {
		r.logger.ErrorWith("failed to delete business-payee relation",
			"error", err,
			"business_id", businessID,
			"payee_id", payeeID,
			"account_id", accountID)
		return fmt.Errorf("failed to delete business-payee relation: %w", err)
	}

	if result.RowsAffected() == 0 {
		r.logger.DebugWith("business-payee relation not found for deletion",
			"business_id", businessID,
			"payee_id", payeeID,
			"account_id", accountID)
		return fmt.Errorf("business-payee relation not found")
	}

	r.logger.InfoWith("business-payee relation deleted (soft delete)",
		"business_id", businessID,
		"payee_id", payeeID,
		"account_id", accountID)
	return nil
}

// RelationExists checks if a business-payee relation exists
func (r *PgBusinessPayeeRelationRepository) RelationExists(ctx context.Context, businessID, payeeID, accountID string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM xcentro.business_payee_relation
			WHERE business_id = $1 AND payee_id = $2 AND account_id = $3
		)
	`

	var exists bool
	err := r.querier.QueryRow(ctx, query, businessID, payeeID, accountID).Scan(&exists)
	if err != nil {
		r.logger.ErrorWith("failed to check business-payee relation existence",
			"error", err,
			"business_id", businessID,
			"payee_id", payeeID,
			"account_id", accountID)
		return false, fmt.Errorf("failed to check business-payee relation existence: %w", err)
	}

	return exists, nil
}

// GetPayeeCount returns the count of active payees for a business
func (r *PgBusinessPayeeRelationRepository) GetPayeeCount(ctx context.Context, businessID string) (int, error) {
	query := `
		SELECT COUNT(DISTINCT (payee_id, account_id))
		FROM xcentro.business_payee_relation
		WHERE business_id = $1 AND is_active = TRUE
	`

	var count int
	err := r.querier.QueryRow(ctx, query, businessID).Scan(&count)
	if err != nil {
		r.logger.ErrorWith("failed to get payee count",
			"error", err,
			"business_id", businessID)
		return 0, fmt.Errorf("failed to get payee count: %w", err)
	}

	return count, nil
}

// GetPayeeCountByType returns the count of active payees of a specific type for a business
func (r *PgBusinessPayeeRelationRepository) GetPayeeCountByType(ctx context.Context, businessID string, payeeType payee.PayeeType) (int, error) {
	query := `
		SELECT COUNT(DISTINCT (r.payee_id, r.account_id))
		FROM xcentro.business_payee_relation r
		INNER JOIN xcentro.payee_master m ON r.payee_id = m.payee_id
		WHERE r.business_id = $1 AND r.is_active = TRUE AND m.payee_type = $2
	`

	var count int
	err := r.querier.QueryRow(ctx, query, businessID, payeeType).Scan(&count)
	if err != nil {
		r.logger.ErrorWith("failed to get payee count by type",
			"error", err,
			"business_id", businessID,
			"payee_type", payeeType)
		return 0, fmt.Errorf("failed to get payee count by type: %w", err)
	}

	return count, nil
}

// CountByBusiness returns the total count of payee relations for a business with filters (excluding pagination)
func (r *PgBusinessPayeeRelationRepository) CountByBusiness(ctx context.Context, businessID string, filters payee.RelationSearchFilters) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM xcentro.business_payee_relation r
		INNER JOIN xcentro.payee_master m ON r.payee_id = m.payee_id
		WHERE r.business_id = $1
	`

	args := []interface{}{businessID}
	argIdx := 2

	// Add filters (same as FindByBusiness, but excluding Limit and Offset)
	if filters.DisplayName != "" {
		query += fmt.Sprintf(" AND r.display_name ILIKE $%d", argIdx)
		args = append(args, "%"+filters.DisplayName+"%")
		argIdx++
	}

	if filters.PayeeType != "" {
		query += fmt.Sprintf(" AND m.payee_type = $%d", argIdx)
		args = append(args, filters.PayeeType)
		argIdx++
	}

	if filters.IsActive != nil {
		query += fmt.Sprintf(" AND r.is_active = $%d", argIdx)
		args = append(args, *filters.IsActive)
		argIdx++
	}

	if filters.IsFavorite != nil {
		query += fmt.Sprintf(" AND r.is_favorite = $%d", argIdx)
		args = append(args, *filters.IsFavorite)
		argIdx++
	}

	if len(filters.Tags) > 0 {
		query += fmt.Sprintf(" AND r.tags && $%d", argIdx)
		args = append(args, filters.Tags)
	}

	var count int
	err := r.querier.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		r.logger.ErrorWith("failed to count payee relations",
			"error", err,
			"business_id", businessID)
		return 0, fmt.Errorf("failed to count payee relations: %w", err)
	}

	return count, nil
}

// GetVendorsForDropdown retrieves lightweight vendor list for dropdown (voucher creation)
// Returns only vendors (not employees, billers, or GST payees) with minimal fields
func (r *PgBusinessPayeeRelationRepository) GetVendorsForDropdown(ctx context.Context, businessID string) ([]*payee.VendorDropdownItem, error) {
	query := `
		SELECT
			bpr.relation_id,
			bpr.display_name,
			COALESCE(bpr.contact_email, '') as contact_email,
			COALESCE(bpr.contact_phone, '') as contact_phone
		FROM xcentro.business_payee_relation bpr
		INNER JOIN xcentro.payee_master pm ON bpr.payee_id = pm.payee_id
		WHERE bpr.business_id = $1
		  AND pm.payee_type = 'vendor'
		  AND bpr.is_active = TRUE
		  AND pm.is_active = TRUE
		ORDER BY bpr.display_name ASC
	`

	rows, err := r.querier.Query(ctx, query, businessID)
	if err != nil {
		r.logger.ErrorWith("failed to query vendors for dropdown",
			"error", err,
			"business_id", businessID)
		return nil, fmt.Errorf("failed to query vendors for dropdown: %w", err)
	}
	defer rows.Close()

	var vendors []*payee.VendorDropdownItem
	for rows.Next() {
		var vendor payee.VendorDropdownItem
		err := rows.Scan(
			&vendor.RelationID,
			&vendor.DisplayName,
			&vendor.ContactEmail,
			&vendor.ContactPhone,
		)
		if err != nil {
			r.logger.ErrorWith("failed to scan vendor dropdown item",
				"error", err,
				"business_id", businessID)
			return nil, fmt.Errorf("failed to scan vendor dropdown item: %w", err)
		}
		vendors = append(vendors, &vendor)
	}

	if err = rows.Err(); err != nil {
		r.logger.ErrorWith("error iterating vendor rows",
			"error", err,
			"business_id", businessID)
		return nil, fmt.Errorf("error iterating vendor rows: %w", err)
	}

	return vendors, nil
}

// GetEmployeesForList retrieves lightweight employee list for tabulated display
// Returns only employees with minimal fields including bank account info
func (r *PgBusinessPayeeRelationRepository) GetEmployeesForList(ctx context.Context, businessID string) ([]*payee.EmployeeLightweight, error) {
	query := `
		SELECT
			bpr.relation_id,
			pe.full_name,
			COALESCE(pe.email, '') as email,
			COALESCE(pe.phone, '') as phone,
			COALESCE(pba.account_number_encrypted, '') as account_number_encrypted,
			COALESCE(pba.ifsc_code, '') as ifsc_code,
			pe.remuneration_doc_verified
		FROM xcentro.business_payee_relation bpr
		INNER JOIN xcentro.payee_master pm ON bpr.payee_id = pm.payee_id
		INNER JOIN xcentro.payee_employee pe ON bpr.payee_id = pe.payee_id
		LEFT JOIN xcentro.payee_bank_accounts pba ON bpr.account_id = pba.account_id
		WHERE bpr.business_id = $1
		  AND pm.payee_type = 'employee'
		  AND bpr.is_active = TRUE
		  AND pm.is_active = TRUE
		ORDER BY pe.full_name ASC
	`

	rows, err := r.querier.Query(ctx, query, businessID)
	if err != nil {
		r.logger.ErrorWith("failed to query employees for list",
			"error", err,
			"business_id", businessID)
		return nil, fmt.Errorf("failed to query employees for list: %w", err)
	}
	defer rows.Close()

	var employees []*payee.EmployeeLightweight
	for rows.Next() {
		var employee payee.EmployeeLightweight
		var encryptedAccountNumber string

		err := rows.Scan(
			&employee.RelationID,
			&employee.FullName,
			&employee.Email,
			&employee.Phone,
			&encryptedAccountNumber,
			&employee.IFSCCode,
			&employee.DocVerified,
		)
		if err != nil {
			r.logger.ErrorWith("failed to scan employee lightweight item",
				"error", err,
				"business_id", businessID)
			return nil, fmt.Errorf("failed to scan employee lightweight item: %w", err)
		}

		// Decrypt account number and get last 4 digits
		if encryptedAccountNumber != "" {
			decryptedAccountNumber, err := crypto.DecryptBankAccountNumber(encryptedAccountNumber, r.encryptionKey)
			if err != nil {
				r.logger.ErrorWith("failed to decrypt account number",
					"error", err,
					"relation_id", employee.RelationID)
				// Don't fail the entire query, just leave account number empty
				employee.AccountNumberLast4 = ""
			} else {
				// Get last 4 digits
				if len(decryptedAccountNumber) >= 4 {
					employee.AccountNumberLast4 = decryptedAccountNumber[len(decryptedAccountNumber)-4:]
				} else {
					employee.AccountNumberLast4 = decryptedAccountNumber
				}
			}
		}

		employees = append(employees, &employee)
	}

	if err = rows.Err(); err != nil {
		r.logger.ErrorWith("error iterating employee rows",
			"error", err,
			"business_id", businessID)
		return nil, fmt.Errorf("error iterating employee rows: %w", err)
	}

	return employees, nil
}

// ValidateRelationForBusiness validates that a relation_id belongs to a business and returns payee/account IDs
// This is used by voucher service to resolve relation_id without circular dependencies
func (r *PgBusinessPayeeRelationRepository) ValidateRelationForBusiness(ctx context.Context, businessID, relationID string) (payeeID, accountID string, err error) {
	query := `
		SELECT payee_id, account_id
		FROM xcentro.business_payee_relation
		WHERE business_id = $1 AND relation_id = $2 AND is_active = TRUE
	`

	err = r.querier.QueryRow(ctx, query, businessID, relationID).Scan(&payeeID, &accountID)
	if err != nil {
		if err == pgx.ErrNoRows {
			r.logger.DebugWith("relation not found or does not belong to business",
				"business_id", businessID,
				"relation_id", relationID)
			return "", "", fmt.Errorf("relation not found or does not belong to business")
		}
		r.logger.ErrorWith("failed to validate relation",
			"error", err,
			"business_id", businessID,
			"relation_id", relationID)
		return "", "", fmt.Errorf("failed to validate relation: %w", err)
	}

	return payeeID, accountID, nil
}

// GetPayeeFullDetailsByRelation fetches complete payee details by relation_id (polymorphic)
// Joins: business_payee_relation → payee_master → type-specific table → bank/biller account
// Validates that the relation belongs to the specified business
// isRestrictive: if true, only returns active payees; if false, returns all payees (for audit/history access)
func (r *PgBusinessPayeeRelationRepository) GetPayeeFullDetailsByRelation(ctx context.Context, businessID, relationID string, isRestrictive bool) (*payee.PayeeFullDetails, error) {
	// First, get the relation and determine payee type
	// IMPORTANT: WHERE clause includes business_id validation for security
	query := `
		SELECT
			bpr.relation_id,
			bpr.display_name,
			COALESCE(bpr.contact_email, '') as contact_email,
			COALESCE(bpr.contact_phone, '') as contact_phone,
			bpr.tags,
			bpr.is_favorite,
			bpr.payee_id,
			bpr.account_id,
			pm.payee_type
		FROM xcentro.business_payee_relation bpr
		INNER JOIN xcentro.payee_master pm ON bpr.payee_id = pm.payee_id
		WHERE bpr.relation_id = $1
		  AND bpr.business_id = $2
	`

	// Conditionally add is_active filter based on isRestrictive flag
	if isRestrictive {
		query += `  AND bpr.is_active = TRUE`
	}

	query += `
	`

	var details payee.PayeeFullDetails
	var payeeID, accountID, payeeType string

	err := r.querier.QueryRow(ctx, query, relationID, businessID).Scan(
		&details.RelationID,
		&details.DisplayName,
		&details.ContactEmail,
		&details.ContactPhone,
		&details.Tags,
		&details.IsFavorite,
		&payeeID,
		&accountID,
		&payeeType,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			r.logger.DebugWith("payee relation not found or does not belong to this business",
				"business_id", businessID,
				"relation_id", relationID)
			return nil, fmt.Errorf("payee relation not found or does not belong to this business")
		}
		r.logger.ErrorWith("failed to fetch payee relation",
			"error", err,
			"business_id", businessID,
			"relation_id", relationID)
		return nil, fmt.Errorf("failed to fetch payee relation: %w", err)
	}

	details.PayeeType = payeeType

	// Route to type-specific query based on payee_type
	switch payeeType {
	case "vendor":
		return r.fetchVendorDetails(ctx, &details, payeeID, accountID)
	case "employee":
		return r.fetchEmployeeDetails(ctx, &details, payeeID, accountID)
	case "biller":
		return r.fetchBillerDetails(ctx, &details, payeeID, accountID)
	default:
		r.logger.ErrorWith("unknown payee type",
			"payee_type", payeeType,
			"business_id", businessID,
			"relation_id", relationID)
		return nil, fmt.Errorf("unknown payee type: %s", payeeType)
	}
}

// fetchVendorDetails populates vendor-specific fields and bank details
func (r *PgBusinessPayeeRelationRepository) fetchVendorDetails(ctx context.Context, details *payee.PayeeFullDetails, payeeID, accountID string) (*payee.PayeeFullDetails, error) {
	// Get vendor details
	vendorQuery := `
		SELECT
			gstin,
			legal_name,
			gst_status,
			COALESCE(city, '') as city,
			COALESCE(state, '') as state
		FROM xcentro.payee_vendor
		WHERE payee_id = $1
	`

	err := r.querier.QueryRow(ctx, vendorQuery, payeeID).Scan(
		&details.GSTIN,
		&details.LegalName,
		&details.GSTStatus,
		&details.City,
		&details.State,
	)

	if err != nil {
		r.logger.ErrorWith("failed to fetch vendor details",
			"error", err,
			"payee_id", payeeID)
		return nil, fmt.Errorf("failed to fetch vendor details: %w", err)
	}

	// Get bank account details
	bankQuery := `
		SELECT
			account_number_encrypted,
			ifsc_code,
			bank_name,
			COALESCE(branch_name, '') as branch_name,
			account_type,
			account_holder_name,
			is_verified
		FROM xcentro.payee_bank_accounts
		WHERE account_id = $1
	`

	var bankDetails payee.PayeeBankDetails
	var encryptedAccountNumber string
	err = r.querier.QueryRow(ctx, bankQuery, accountID).Scan(
		&encryptedAccountNumber,
		&bankDetails.IFSCCode,
		&bankDetails.BankName,
		&bankDetails.BranchName,
		&bankDetails.AccountType,
		&bankDetails.AccountHolderName,
		&bankDetails.IsVerified,
	)

	if err != nil {
		r.logger.ErrorWith("failed to fetch bank details",
			"error", err,
			"payee_id", payeeID,
			"account_id", accountID)
		return nil, fmt.Errorf("failed to fetch bank details: %w", err)
	}

	// Decrypt account number
	bankDetails.AccountNumber, err = crypto.DecryptBankAccountNumber(encryptedAccountNumber, r.encryptionKey)
	if err != nil {
		r.logger.ErrorWith("Failed to decrypt account number",
			"payee_id", payeeID,
			"account_id", accountID,
			"error", err)
		return nil, fmt.Errorf("failed to decrypt account number: %w", err)
	}

	details.BankDetails = &bankDetails
	return details, nil
}

// fetchEmployeeDetails populates employee-specific fields and bank details
func (r *PgBusinessPayeeRelationRepository) fetchEmployeeDetails(ctx context.Context, details *payee.PayeeFullDetails, payeeID, accountID string) (*payee.PayeeFullDetails, error) {
	// Get employee details
	employeeQuery := `
		SELECT
			full_name,
			COALESCE(employee_code, '') as employee_code,
			COALESCE(designation, '') as designation,
			COALESCE(department, '') as department,
			COALESCE(employment_type, '') as employment_type
		FROM xcentro.payee_employee
		WHERE payee_id = $1
	`

	err := r.querier.QueryRow(ctx, employeeQuery, payeeID).Scan(
		&details.FullName,
		&details.EmployeeCode,
		&details.Designation,
		&details.Department,
		&details.EmploymentType,
	)

	if err != nil {
		r.logger.ErrorWith("failed to fetch employee details",
			"error", err,
			"payee_id", payeeID)
		return nil, fmt.Errorf("failed to fetch employee details: %w", err)
	}

	// Get bank account details
	bankQuery := `
		SELECT
			account_number_encrypted,
			ifsc_code,
			bank_name,
			COALESCE(branch_name, '') as branch_name,
			account_type,
			account_holder_name,
			is_verified
		FROM xcentro.payee_bank_accounts
		WHERE account_id = $1
	`

	var bankDetails payee.PayeeBankDetails
	var encryptedAccountNumber string
	err = r.querier.QueryRow(ctx, bankQuery, accountID).Scan(
		&encryptedAccountNumber,
		&bankDetails.IFSCCode,
		&bankDetails.BankName,
		&bankDetails.BranchName,
		&bankDetails.AccountType,
		&bankDetails.AccountHolderName,
		&bankDetails.IsVerified,
	)

	if err != nil {
		r.logger.ErrorWith("failed to fetch bank details",
			"error", err,
			"payee_id", payeeID,
			"account_id", accountID)
		return nil, fmt.Errorf("failed to fetch bank details: %w", err)
	}

	// Decrypt account number
	bankDetails.AccountNumber, err = crypto.DecryptBankAccountNumber(encryptedAccountNumber, r.encryptionKey)
	if err != nil {
		r.logger.ErrorWith("Failed to decrypt account number",
			"payee_id", payeeID,
			"account_id", accountID,
			"error", err)
		return nil, fmt.Errorf("failed to decrypt account number: %w", err)
	}

	details.BankDetails = &bankDetails
	return details, nil
}

// fetchBillerDetails populates biller-specific fields (no bank details for billers)
func (r *PgBusinessPayeeRelationRepository) fetchBillerDetails(ctx context.Context, details *payee.PayeeFullDetails, payeeID, accountID string) (*payee.PayeeFullDetails, error) {
	// Get biller details
	billerQuery := `
		SELECT
			biller_name,
			biller_category
		FROM xcentro.payee_biller
		WHERE payee_id = $1
	`

	err := r.querier.QueryRow(ctx, billerQuery, payeeID).Scan(
		&details.BillerName,
		&details.BillerCategory,
	)

	if err != nil {
		r.logger.ErrorWith("failed to fetch biller details",
			"error", err,
			"payee_id", payeeID)
		return nil, fmt.Errorf("failed to fetch biller details: %w", err)
	}

	// Get biller account configuration
	billerAccountQuery := `
		SELECT
			mobile_number,
			COALESCE(last_bill_amount, 0) as last_bill_amount,
			COALESCE(last_bill_due_date::text, '') as last_bill_due_date
		FROM xcentro.payee_biller_accounts
		WHERE biller_account_id = $1
	`

	var lastBillAmountPaise int64
	err = r.querier.QueryRow(ctx, billerAccountQuery, accountID).Scan(
		&details.MobileNumber,
		&lastBillAmountPaise,
		&details.LastBillDueDate,
	)

	if err != nil {
		r.logger.ErrorWith("failed to fetch biller account details",
			"error", err,
			"payee_id", payeeID,
			"account_id", accountID)
		return nil, fmt.Errorf("failed to fetch biller account details: %w", err)
	}

	// Convert paise to rupees
	details.LastBillAmount = float64(lastBillAmountPaise) / 100.0

	return details, nil
}
