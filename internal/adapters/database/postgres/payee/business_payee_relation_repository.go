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

// PgBusinessPayeeRelationRepository implements payee.BusinessPayeeRelationRepository using PostgreSQL
type PgBusinessPayeeRelationRepository struct {
	querier client.Querier
}

// NewPgBusinessPayeeRelationRepository creates a new PostgreSQL business-payee relation repository
func NewPgBusinessPayeeRelationRepository(db client.DB) payee.BusinessPayeeRelationRepository {
	return &PgBusinessPayeeRelationRepository{
		querier: db,
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgBusinessPayeeRelationRepository) WithTx(tx client.Tx) payee.BusinessPayeeRelationRepository {
	return &PgBusinessPayeeRelationRepository{
		querier: tx,
	}
}

// CreateRelation creates a new business-payee relation
func (r *PgBusinessPayeeRelationRepository) CreateRelation(ctx context.Context, relation *payee.BusinessPayeeRelation) error {
	fmt.Printf("[PgBusinessPayeeRelationRepository] CreateRelation called - BusinessID: %s, PayeeID: %s, AccountID: %s\n",
		relation.BusinessID, relation.PayeeID, relation.AccountID)

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
		return fmt.Errorf("failed to create business-payee relation: %w", err)
	}

	fmt.Printf("[PgBusinessPayeeRelationRepository] Relation created at: %s\n", relation.AddedAt)
	return nil
}

// FindRelationByID retrieves a business-payee relation by its relation_id (surrogate key)
func (r *PgBusinessPayeeRelationRepository) FindRelationByID(ctx context.Context, relationID string) (*payee.BusinessPayeeRelation, error) {
	fmt.Printf("[PgBusinessPayeeRelationRepository] FindRelationByID called - RelationID: %s\n", relationID)

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
			return nil, fmt.Errorf("business-payee relation not found")
		}
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
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	fmt.Printf("[PgBusinessPayeeRelationRepository] Relation found - PayeeID: %s, AccountID: %s\n",
		relation.PayeeID, relation.AccountID)
	return &relation, nil
}

// FindRelation retrieves a specific business-payee relation
func (r *PgBusinessPayeeRelationRepository) FindRelation(ctx context.Context, businessID, payeeID, accountID string) (*payee.BusinessPayeeRelation, error) {
	fmt.Printf("[PgBusinessPayeeRelationRepository] FindRelation called - BusinessID: %s, PayeeID: %s, AccountID: %s\n",
		businessID, payeeID, accountID)

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
			return nil, fmt.Errorf("business-payee relation not found")
		}
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
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &relation, nil
}

// FindByBusiness retrieves all payee relations for a business with filters
func (r *PgBusinessPayeeRelationRepository) FindByBusiness(ctx context.Context, businessID string, filters payee.RelationSearchFilters) ([]*payee.BusinessPayeeRelation, error) {
	fmt.Printf("[PgBusinessPayeeRelationRepository] FindByBusiness called - BusinessID: %s\n", businessID)

	query := `
		SELECT r.relation_id, r.business_id, r.payee_id, r.account_id,
		       r.display_name, r.contact_email, r.contact_phone, r.notes,
		       r.is_active, r.is_favorite, r.tags,
		       r.added_at, r.added_by, r.last_used_at, r.updated_at, r.metadata,
			   m.payee_type
		FROM xcentro.business_payee_relation r
		INNER JOIN xcentro.payee_master m ON r.payee_id = m.payee_id
	`

	// Join with payee_master if filtering by payee_type
	if filters.PayeeType != "" {
		query += `
		INNER JOIN xcentro.payee_master m ON r.payee_id = m.payee_id
		`
	}

	query += `WHERE r.business_id = $1`

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
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		relations = append(relations, &relation)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating business-payee relations: %w", err)
	}

	return relations, nil
}

// UpdateRelation updates a business-payee relation
func (r *PgBusinessPayeeRelationRepository) UpdateRelation(ctx context.Context, relation *payee.BusinessPayeeRelation) error {
	fmt.Printf("[PgBusinessPayeeRelationRepository] UpdateRelation called - BusinessID: %s, PayeeID: %s, AccountID: %s\n",
		relation.BusinessID, relation.PayeeID, relation.AccountID)

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
		return fmt.Errorf("failed to update business-payee relation: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("business-payee relation not found")
	}

	return nil
}

// DeleteRelation deletes (soft delete) a business-payee relation
func (r *PgBusinessPayeeRelationRepository) DeleteRelation(ctx context.Context, businessID, payeeID, accountID string) error {
	fmt.Printf("[PgBusinessPayeeRelationRepository] DeleteRelation called - BusinessID: %s, PayeeID: %s, AccountID: %s\n",
		businessID, payeeID, accountID)

	// Soft delete: set is_active = FALSE
	query := `
		UPDATE xcentro.business_payee_relation
		SET is_active = FALSE
		WHERE business_id = $1 AND payee_id = $2 AND account_id = $3
	`

	result, err := r.querier.Exec(ctx, query, businessID, payeeID, accountID)
	if err != nil {
		return fmt.Errorf("failed to delete business-payee relation: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("business-payee relation not found")
	}

	return nil
}

// RelationExists checks if a business-payee relation exists
func (r *PgBusinessPayeeRelationRepository) RelationExists(ctx context.Context, businessID, payeeID, accountID string) (bool, error) {
	fmt.Printf("[PgBusinessPayeeRelationRepository] RelationExists called - BusinessID: %s, PayeeID: %s, AccountID: %s\n",
		businessID, payeeID, accountID)

	query := `
		SELECT EXISTS(
			SELECT 1 FROM xcentro.business_payee_relation
			WHERE business_id = $1 AND payee_id = $2 AND account_id = $3
		)
	`

	var exists bool
	err := r.querier.QueryRow(ctx, query, businessID, payeeID, accountID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check business-payee relation existence: %w", err)
	}

	return exists, nil
}

// GetPayeeCount returns the count of active payees for a business
func (r *PgBusinessPayeeRelationRepository) GetPayeeCount(ctx context.Context, businessID string) (int, error) {
	fmt.Printf("[PgBusinessPayeeRelationRepository] GetPayeeCount called - BusinessID: %s\n", businessID)

	query := `
		SELECT COUNT(DISTINCT (payee_id, account_id))
		FROM xcentro.business_payee_relation
		WHERE business_id = $1 AND is_active = TRUE
	`

	var count int
	err := r.querier.QueryRow(ctx, query, businessID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get payee count: %w", err)
	}

	return count, nil
}

// GetPayeeCountByType returns the count of active payees of a specific type for a business
func (r *PgBusinessPayeeRelationRepository) GetPayeeCountByType(ctx context.Context, businessID string, payeeType payee.PayeeType) (int, error) {
	fmt.Printf("[PgBusinessPayeeRelationRepository] GetPayeeCountByType called - BusinessID: %s, PayeeType: %s\n", businessID, payeeType)

	query := `
		SELECT COUNT(DISTINCT (r.payee_id, r.account_id))
		FROM xcentro.business_payee_relation r
		INNER JOIN xcentro.payee_master m ON r.payee_id = m.payee_id
		WHERE r.business_id = $1 AND r.is_active = TRUE AND m.payee_type = $2
	`

	var count int
	err := r.querier.QueryRow(ctx, query, businessID, payeeType).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get payee count by type: %w", err)
	}

	return count, nil
}

// GetVendorsForDropdown retrieves lightweight vendor list for dropdown (voucher creation)
// Returns only vendors (not employees, billers, or GST payees) with minimal fields
func (r *PgBusinessPayeeRelationRepository) GetVendorsForDropdown(ctx context.Context, businessID string) ([]*payee.VendorDropdownItem, error) {
	fmt.Printf("[PgBusinessPayeeRelationRepository] GetVendorsForDropdown - BusinessID: %s\n", businessID)

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
			return nil, fmt.Errorf("failed to scan vendor dropdown item: %w", err)
		}
		vendors = append(vendors, &vendor)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating vendor rows: %w", err)
	}

	fmt.Printf("[PgBusinessPayeeRelationRepository] Found %d vendors for dropdown\n", len(vendors))
	return vendors, nil
}

// ValidateRelationForBusiness validates that a relation_id belongs to a business and returns payee/account IDs
// This is used by voucher service to resolve relation_id without circular dependencies
func (r *PgBusinessPayeeRelationRepository) ValidateRelationForBusiness(ctx context.Context, businessID, relationID string) (payeeID, accountID string, err error) {
	fmt.Printf("[PgBusinessPayeeRelationRepository] ValidateRelationForBusiness - BusinessID: %s, RelationID: %s\n", businessID, relationID)

	query := `
		SELECT payee_id, account_id
		FROM xcentro.business_payee_relation
		WHERE business_id = $1 AND relation_id = $2 AND is_active = TRUE
	`

	err = r.querier.QueryRow(ctx, query, businessID, relationID).Scan(&payeeID, &accountID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", "", fmt.Errorf("relation not found or does not belong to business")
		}
		return "", "", fmt.Errorf("failed to validate relation: %w", err)
	}

	fmt.Printf("[PgBusinessPayeeRelationRepository] Relation validated - PayeeID: %s, AccountID: %s\n", payeeID, accountID)
	return payeeID, accountID, nil
}

// GetPayeeFullDetailsByRelation fetches complete payee details by relation_id (polymorphic)
// Joins: business_payee_relation → payee_master → type-specific table → bank/biller account
// Validates that the relation belongs to the specified business
func (r *PgBusinessPayeeRelationRepository) GetPayeeFullDetailsByRelation(ctx context.Context, businessID, relationID string) (*payee.PayeeFullDetails, error) {
	fmt.Printf("[PgBusinessPayeeRelationRepository] GetPayeeFullDetailsByRelation - BusinessID: %s, RelationID: %s\n", businessID, relationID)

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
		  AND bpr.is_active = TRUE
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
			return nil, fmt.Errorf("payee relation not found or does not belong to this business")
		}
		return nil, fmt.Errorf("failed to fetch payee relation: %w", err)
	}

	details.PayeeType = payeeType

	fmt.Printf("[PgBusinessPayeeRelationRepository] Relation validated for business - PayeeType: %s\n", payeeType)

	// Route to type-specific query based on payee_type
	switch payeeType {
	case "vendor":
		return r.fetchVendorDetails(ctx, &details, payeeID, accountID)
	case "employee":
		return r.fetchEmployeeDetails(ctx, &details, payeeID, accountID)
	case "biller":
		return r.fetchBillerDetails(ctx, &details, payeeID, accountID)
	default:
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
			is_verified,
			is_primary
		FROM xcentro.payee_bank_accounts
		WHERE account_id = $1
	`

	var bankDetails payee.PayeeBankDetails
	err = r.querier.QueryRow(ctx, bankQuery, accountID).Scan(
		&bankDetails.AccountNumber,
		&bankDetails.IFSCCode,
		&bankDetails.BankName,
		&bankDetails.BranchName,
		&bankDetails.AccountType,
		&bankDetails.AccountHolderName,
		&bankDetails.IsVerified,
		&bankDetails.IsPrimary,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to fetch bank details: %w", err)
	}

	details.BankDetails = &bankDetails
	return details, nil
}

// fetchEmployeeDetails populates employee-specific fields and bank details
func (r *PgBusinessPayeeRelationRepository) fetchEmployeeDetails(ctx context.Context, details *payee.PayeeFullDetails, payeeID, accountID string) (*payee.PayeeFullDetails, error) {
	// Get employee details
	employeeQuery := `
		SELECT
			pan,
			full_name,
			COALESCE(employee_code, '') as employee_code,
			COALESCE(designation, '') as designation,
			COALESCE(department, '') as department,
			COALESCE(employment_type, '') as employment_type
		FROM xcentro.payee_employee
		WHERE payee_id = $1
	`

	err := r.querier.QueryRow(ctx, employeeQuery, payeeID).Scan(
		&details.PAN,
		&details.FullName,
		&details.EmployeeCode,
		&details.Designation,
		&details.Department,
		&details.EmploymentType,
	)

	if err != nil {
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
			is_verified,
			is_primary
		FROM xcentro.payee_bank_accounts
		WHERE account_id = $1
	`

	var bankDetails payee.PayeeBankDetails
	err = r.querier.QueryRow(ctx, bankQuery, accountID).Scan(
		&bankDetails.AccountNumber,
		&bankDetails.IFSCCode,
		&bankDetails.BankName,
		&bankDetails.BranchName,
		&bankDetails.AccountType,
		&bankDetails.AccountHolderName,
		&bankDetails.IsVerified,
		&bankDetails.IsPrimary,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to fetch bank details: %w", err)
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
		return nil, fmt.Errorf("failed to fetch biller account details: %w", err)
	}

	// Convert paise to rupees
	details.LastBillAmount = float64(lastBillAmountPaise) / 100.0

	return details, nil
}
