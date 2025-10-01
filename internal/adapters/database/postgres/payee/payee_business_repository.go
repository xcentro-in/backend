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

// PgPayeeBusinessRepository implements payee.PayeeBusinessRepository using PostgreSQL
type PgPayeeBusinessRepository struct {
	querier client.Querier
}

// NewPgPayeeBusinessRepository creates a new PostgreSQL payee business repository
func NewPgPayeeBusinessRepository(db client.DB) payee.PayeeBusinessRepository {
	return &PgPayeeBusinessRepository{
		querier: db,
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgPayeeBusinessRepository) WithTx(tx client.Tx) payee.PayeeBusinessRepository {
	return &PgPayeeBusinessRepository{
		querier: tx,
	}
}

// CreatePayeeBusiness creates a new payee business record
func (r *PgPayeeBusinessRepository) CreatePayeeBusiness(ctx context.Context, p *payee.PayeeBusiness) error {
	fmt.Printf("[PgPayeeBusinessRepository] CreatePayeeBusiness called - GSTIN: %s, LegalName: %s\n",
		p.GSTIN, p.LegalName)

	query := `
		INSERT INTO xcentro.payee_business (
			gstin, legal_name, business_type, registration_date, gst_status,
			building_name, street, city, state, pincode,
			gst_verification_status, gst_verified_at, gst_data, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING payee_id, created_at
	`

	// Serialize JSONB fields
	var gstDataJSON, metadataJSON []byte
	var err error

	if p.GSTData != nil {
		gstDataJSON, err = json.Marshal(p.GSTData)
		if err != nil {
			return fmt.Errorf("failed to marshal gst_data: %w", err)
		}
	}

	if p.Metadata != nil {
		metadataJSON, err = json.Marshal(p.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	err = r.querier.QueryRow(ctx, query,
		p.GSTIN,
		p.LegalName,
		p.BusinessType,
		p.RegistrationDate,
		p.GSTStatus,
		p.PrimaryAddress.BuildingName,
		p.PrimaryAddress.Street,
		p.PrimaryAddress.City,
		p.PrimaryAddress.State,
		p.PrimaryAddress.Pincode,
		p.GSTVerificationStatus,
		p.GSTVerifiedAt,
		gstDataJSON,
		metadataJSON,
	).Scan(&p.PayeeID, &p.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create payee business: %w", err)
	}

	fmt.Printf("[PgPayeeBusinessRepository] Payee business created with ID: %s\n", p.PayeeID)
	return nil
}

// FindByID retrieves a payee business by its ID
func (r *PgPayeeBusinessRepository) FindByID(ctx context.Context, payeeID string) (*payee.PayeeBusiness, error) {
	fmt.Printf("[PgPayeeBusinessRepository] FindByID called - PayeeID: %s\n", payeeID)

	query := `
		SELECT payee_id, gstin, legal_name, business_type, registration_date, gst_status,
		       building_name, street, city, state, pincode,
		       gst_verification_status, gst_verified_at, gst_data, metadata, created_at, updated_at
		FROM xcentro.payee_business
		WHERE payee_id = $1
	`

	var p payee.PayeeBusiness
	var businessType sql.NullString
	var registrationDate sql.NullTime
	var gstVerifiedAt sql.NullTime
	var buildingName, street, city, state, pincode sql.NullString
	var gstDataJSON, metadataJSON []byte
	var updatedAt sql.NullTime

	err := r.querier.QueryRow(ctx, query, payeeID).Scan(
		&p.PayeeID,
		&p.GSTIN,
		&p.LegalName,
		&businessType,
		&registrationDate,
		&p.GSTStatus,
		&buildingName,
		&street,
		&city,
		&state,
		&pincode,
		&p.GSTVerificationStatus,
		&gstVerifiedAt,
		&gstDataJSON,
		&metadataJSON,
		&p.CreatedAt,
		&updatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("payee business not found")
		}
		return nil, fmt.Errorf("failed to find payee business: %w", err)
	}

	// Handle nullable fields
	p.BusinessType = businessType.String
	if registrationDate.Valid {
		p.RegistrationDate = &registrationDate.Time
	}
	if gstVerifiedAt.Valid {
		p.GSTVerifiedAt = &gstVerifiedAt.Time
	}
	if updatedAt.Valid {
		p.UpdatedAt = updatedAt.Time
	}

	// Build address
	p.PrimaryAddress = payee.Address{
		BuildingName: buildingName.String,
		Street:       street.String,
		City:         city.String,
		State:        state.String,
		Pincode:      pincode.String,
	}

	// Deserialize JSONB fields
	if len(gstDataJSON) > 0 {
		if err := json.Unmarshal(gstDataJSON, &p.GSTData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal gst_data: %w", err)
		}
	}
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &p.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &p, nil
}

// FindByGSTIN retrieves a payee business by GSTIN (most important for duplicate check)
func (r *PgPayeeBusinessRepository) FindByGSTIN(ctx context.Context, gstin string) (*payee.PayeeBusiness, error) {
	fmt.Printf("[PgPayeeBusinessRepository] FindByGSTIN called - GSTIN: %s\n", gstin)

	query := `
		SELECT payee_id, gstin, legal_name, business_type, registration_date, gst_status,
		       building_name, street, city, state, pincode,
		       gst_verification_status, gst_verified_at, gst_data, metadata, created_at, updated_at
		FROM xcentro.payee_business
		WHERE gstin = $1
	`

	var p payee.PayeeBusiness
	var businessType sql.NullString
	var registrationDate sql.NullTime
	var gstVerifiedAt sql.NullTime
	var buildingName, street, city, state, pincode sql.NullString
	var gstDataJSON, metadataJSON []byte
	var updatedAt sql.NullTime

	err := r.querier.QueryRow(ctx, query, gstin).Scan(
		&p.PayeeID,
		&p.GSTIN,
		&p.LegalName,
		&businessType,
		&registrationDate,
		&p.GSTStatus,
		&buildingName,
		&street,
		&city,
		&state,
		&pincode,
		&p.GSTVerificationStatus,
		&gstVerifiedAt,
		&gstDataJSON,
		&metadataJSON,
		&p.CreatedAt,
		&updatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Return nil without error if not found (normal case)
		}
		return nil, fmt.Errorf("failed to find payee business by GSTIN: %w", err)
	}

	// Handle nullable fields
	p.BusinessType = businessType.String
	if registrationDate.Valid {
		p.RegistrationDate = &registrationDate.Time
	}
	if gstVerifiedAt.Valid {
		p.GSTVerifiedAt = &gstVerifiedAt.Time
	}
	if updatedAt.Valid {
		p.UpdatedAt = updatedAt.Time
	}

	// Build address
	p.PrimaryAddress = payee.Address{
		BuildingName: buildingName.String,
		Street:       street.String,
		City:         city.String,
		State:        state.String,
		Pincode:      pincode.String,
	}

	// Deserialize JSONB fields
	if len(gstDataJSON) > 0 {
		if err := json.Unmarshal(gstDataJSON, &p.GSTData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal gst_data: %w", err)
		}
	}
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &p.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &p, nil
}

// UpdatePayeeBusiness updates a payee business record
func (r *PgPayeeBusinessRepository) UpdatePayeeBusiness(ctx context.Context, p *payee.PayeeBusiness) error {
	fmt.Printf("[PgPayeeBusinessRepository] UpdatePayeeBusiness called - PayeeID: %s\n", p.PayeeID)

	query := `
		UPDATE xcentro.payee_business
		SET legal_name = $2, business_type = $3, registration_date = $4, gst_status = $5,
		    building_name = $6, street = $7, city = $8, state = $9, pincode = $10,
		    gst_verification_status = $11, gst_verified_at = $12, gst_data = $13, metadata = $14
		WHERE payee_id = $1
	`

	// Serialize JSONB fields
	var gstDataJSON, metadataJSON []byte
	var err error

	if p.GSTData != nil {
		gstDataJSON, err = json.Marshal(p.GSTData)
		if err != nil {
			return fmt.Errorf("failed to marshal gst_data: %w", err)
		}
	}

	if p.Metadata != nil {
		metadataJSON, err = json.Marshal(p.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	result, err := r.querier.Exec(ctx, query,
		p.PayeeID,
		p.LegalName,
		p.BusinessType,
		p.RegistrationDate,
		p.GSTStatus,
		p.PrimaryAddress.BuildingName,
		p.PrimaryAddress.Street,
		p.PrimaryAddress.City,
		p.PrimaryAddress.State,
		p.PrimaryAddress.Pincode,
		p.GSTVerificationStatus,
		p.GSTVerifiedAt,
		gstDataJSON,
		metadataJSON,
	)

	if err != nil {
		return fmt.Errorf("failed to update payee business: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("payee business not found")
	}

	return nil
}

// SearchPayees searches for payee businesses with filters
func (r *PgPayeeBusinessRepository) SearchPayees(ctx context.Context, filters payee.PayeeBusinessSearchFilters) ([]*payee.PayeeBusiness, error) {
	fmt.Printf("[PgPayeeBusinessRepository] SearchPayees called\n")

	query := `
		SELECT payee_id, gstin, legal_name, business_type, registration_date, gst_status,
		       building_name, street, city, state, pincode,
		       gst_verification_status, gst_verified_at, gst_data, metadata, created_at, updated_at
		FROM xcentro.payee_business
		WHERE 1=1
	`

	args := []interface{}{}
	argIdx := 1

	// Add filters
	if filters.LegalName != "" {
		query += fmt.Sprintf(" AND legal_name ILIKE $%d", argIdx)
		args = append(args, "%"+filters.LegalName+"%")
		argIdx++
	}

	if filters.GSTIN != "" {
		query += fmt.Sprintf(" AND gstin = $%d", argIdx)
		args = append(args, filters.GSTIN)
		argIdx++
	}

	if filters.GSTStatus != "" {
		query += fmt.Sprintf(" AND gst_status = $%d", argIdx)
		args = append(args, filters.GSTStatus)
		argIdx++
	}

	// Add ordering and pagination
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

	rows, err := r.querier.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search payee businesses: %w", err)
	}
	defer rows.Close()

	var payees []*payee.PayeeBusiness

	for rows.Next() {
		var p payee.PayeeBusiness
		var businessType sql.NullString
		var registrationDate sql.NullTime
		var gstVerifiedAt sql.NullTime
		var buildingName, street, city, state, pincode sql.NullString
		var gstDataJSON, metadataJSON []byte
		var updatedAt sql.NullTime

		err := rows.Scan(
			&p.PayeeID,
			&p.GSTIN,
			&p.LegalName,
			&businessType,
			&registrationDate,
			&p.GSTStatus,
			&buildingName,
			&street,
			&city,
			&state,
			&pincode,
			&p.GSTVerificationStatus,
			&gstVerifiedAt,
			&gstDataJSON,
			&metadataJSON,
			&p.CreatedAt,
			&updatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan payee business: %w", err)
		}

		// Handle nullable fields
		p.BusinessType = businessType.String
		if registrationDate.Valid {
			p.RegistrationDate = &registrationDate.Time
		}
		if gstVerifiedAt.Valid {
			p.GSTVerifiedAt = &gstVerifiedAt.Time
		}
		if updatedAt.Valid {
			p.UpdatedAt = updatedAt.Time
		}

		// Build address
		p.PrimaryAddress = payee.Address{
			BuildingName: buildingName.String,
			Street:       street.String,
			City:         city.String,
			State:        state.String,
			Pincode:      pincode.String,
		}

		// Deserialize JSONB fields
		if len(gstDataJSON) > 0 {
			if err := json.Unmarshal(gstDataJSON, &p.GSTData); err != nil {
				return nil, fmt.Errorf("failed to unmarshal gst_data: %w", err)
			}
		}
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &p.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		payees = append(payees, &p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating payee businesses: %w", err)
	}

	return payees, nil
}
