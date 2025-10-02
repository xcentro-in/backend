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

// PgPayeeVendorRepository implements payee.PayeeVendorRepository using PostgreSQL
type PgPayeeVendorRepository struct {
	querier client.Querier
}

// NewPgPayeeVendorRepository creates a new PostgreSQL payee vendor repository
func NewPgPayeeVendorRepository(db client.DB) payee.PayeeVendorRepository {
	return &PgPayeeVendorRepository{
		querier: db,
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgPayeeVendorRepository) WithTx(tx client.Tx) payee.PayeeVendorRepository {
	return &PgPayeeVendorRepository{
		querier: tx,
	}
}

// Create creates a new vendor payee record
func (r *PgPayeeVendorRepository) Create(ctx context.Context, vendor *payee.PayeeVendor) error {
	query := `
		INSERT INTO xcentro.payee_vendor (
			payee_id, gstin, legal_name, business_type, registration_date, gst_status,
			building_name, street, city, state, pincode,
			gst_verification_status, gst_verified_at, gst_data, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING created_at, updated_at
	`

	// Serialize JSONB fields
	var gstDataJSON, metadataJSON []byte
	var err error

	if vendor.GSTData != nil {
		gstDataJSON, err = json.Marshal(vendor.GSTData)
		if err != nil {
			return fmt.Errorf("failed to marshal gst_data: %w", err)
		}
	}

	if vendor.Metadata != nil {
		metadataJSON, err = json.Marshal(vendor.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	err = r.querier.QueryRow(ctx, query,
		vendor.PayeeID,
		vendor.GSTIN,
		vendor.LegalName,
		vendor.BusinessType,
		vendor.RegistrationDate,
		vendor.GSTStatus,
		vendor.PrimaryAddress.BuildingName,
		vendor.PrimaryAddress.Street,
		vendor.PrimaryAddress.City,
		vendor.PrimaryAddress.State,
		vendor.PrimaryAddress.Pincode,
		vendor.GSTVerificationStatus,
		vendor.GSTVerifiedAt,
		gstDataJSON,
		metadataJSON,
	).Scan(&vendor.CreatedAt, &vendor.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create vendor payee: %w", err)
	}

	return nil
}

// FindByID retrieves a vendor payee by its payee ID
func (r *PgPayeeVendorRepository) FindByID(ctx context.Context, payeeID string) (*payee.PayeeVendor, error) {
	query := `
		SELECT payee_id, gstin, legal_name, business_type, registration_date, gst_status,
		       building_name, street, city, state, pincode,
		       gst_verification_status, gst_verified_at, gst_data, metadata, created_at, updated_at
		FROM xcentro.payee_vendor
		WHERE payee_id = $1
	`

	var v payee.PayeeVendor
	var businessType sql.NullString
	var registrationDate sql.NullTime
	var gstVerifiedAt sql.NullTime
	var buildingName, street, city, state, pincode sql.NullString
	var gstDataJSON, metadataJSON []byte
	var updatedAt sql.NullTime

	err := r.querier.QueryRow(ctx, query, payeeID).Scan(
		&v.PayeeID,
		&v.GSTIN,
		&v.LegalName,
		&businessType,
		&registrationDate,
		&v.GSTStatus,
		&buildingName,
		&street,
		&city,
		&state,
		&pincode,
		&v.GSTVerificationStatus,
		&gstVerifiedAt,
		&gstDataJSON,
		&metadataJSON,
		&v.CreatedAt,
		&updatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("vendor payee not found")
		}
		return nil, fmt.Errorf("failed to find vendor payee: %w", err)
	}

	// Handle nullable fields
	v.BusinessType = businessType.String
	if registrationDate.Valid {
		v.RegistrationDate = &registrationDate.Time
	}
	if gstVerifiedAt.Valid {
		v.GSTVerifiedAt = &gstVerifiedAt.Time
	}
	if updatedAt.Valid {
		v.UpdatedAt = updatedAt.Time
	}

	// Build address
	v.PrimaryAddress = payee.Address{
		BuildingName: buildingName.String,
		Street:       street.String,
		City:         city.String,
		State:        state.String,
		Pincode:      pincode.String,
	}

	// Deserialize JSONB fields
	if len(gstDataJSON) > 0 {
		if err := json.Unmarshal(gstDataJSON, &v.GSTData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal gst_data: %w", err)
		}
	}
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &v.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &v, nil
}

// FindByGSTIN retrieves a vendor payee by GSTIN
func (r *PgPayeeVendorRepository) FindByGSTIN(ctx context.Context, gstin string) (*payee.PayeeVendor, error) {
	query := `
		SELECT payee_id, gstin, legal_name, business_type, registration_date, gst_status,
		       building_name, street, city, state, pincode,
		       gst_verification_status, gst_verified_at, gst_data, metadata, created_at, updated_at
		FROM xcentro.payee_vendor
		WHERE gstin = $1
	`

	var v payee.PayeeVendor
	var businessType sql.NullString
	var registrationDate sql.NullTime
	var gstVerifiedAt sql.NullTime
	var buildingName, street, city, state, pincode sql.NullString
	var gstDataJSON, metadataJSON []byte
	var updatedAt sql.NullTime

	err := r.querier.QueryRow(ctx, query, gstin).Scan(
		&v.PayeeID,
		&v.GSTIN,
		&v.LegalName,
		&businessType,
		&registrationDate,
		&v.GSTStatus,
		&buildingName,
		&street,
		&city,
		&state,
		&pincode,
		&v.GSTVerificationStatus,
		&gstVerifiedAt,
		&gstDataJSON,
		&metadataJSON,
		&v.CreatedAt,
		&updatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Return nil without error if not found (normal case)
		}
		return nil, fmt.Errorf("failed to find vendor payee by GSTIN: %w", err)
	}

	// Handle nullable fields
	v.BusinessType = businessType.String
	if registrationDate.Valid {
		v.RegistrationDate = &registrationDate.Time
	}
	if gstVerifiedAt.Valid {
		v.GSTVerifiedAt = &gstVerifiedAt.Time
	}
	if updatedAt.Valid {
		v.UpdatedAt = updatedAt.Time
	}

	// Build address
	v.PrimaryAddress = payee.Address{
		BuildingName: buildingName.String,
		Street:       street.String,
		City:         city.String,
		State:        state.String,
		Pincode:      pincode.String,
	}

	// Deserialize JSONB fields
	if len(gstDataJSON) > 0 {
		if err := json.Unmarshal(gstDataJSON, &v.GSTData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal gst_data: %w", err)
		}
	}
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &v.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &v, nil
}

// Update updates a vendor payee record
func (r *PgPayeeVendorRepository) Update(ctx context.Context, vendor *payee.PayeeVendor) error {
	query := `
		UPDATE xcentro.payee_vendor
		SET legal_name = $2, business_type = $3, registration_date = $4, gst_status = $5,
		    building_name = $6, street = $7, city = $8, state = $9, pincode = $10,
		    gst_verification_status = $11, gst_verified_at = $12, gst_data = $13, metadata = $14
		WHERE payee_id = $1
	`

	// Serialize JSONB fields
	var gstDataJSON, metadataJSON []byte
	var err error

	if vendor.GSTData != nil {
		gstDataJSON, err = json.Marshal(vendor.GSTData)
		if err != nil {
			return fmt.Errorf("failed to marshal gst_data: %w", err)
		}
	}

	if vendor.Metadata != nil {
		metadataJSON, err = json.Marshal(vendor.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	result, err := r.querier.Exec(ctx, query,
		vendor.PayeeID,
		vendor.LegalName,
		vendor.BusinessType,
		vendor.RegistrationDate,
		vendor.GSTStatus,
		vendor.PrimaryAddress.BuildingName,
		vendor.PrimaryAddress.Street,
		vendor.PrimaryAddress.City,
		vendor.PrimaryAddress.State,
		vendor.PrimaryAddress.Pincode,
		vendor.GSTVerificationStatus,
		vendor.GSTVerifiedAt,
		gstDataJSON,
		metadataJSON,
	)

	if err != nil {
		return fmt.Errorf("failed to update vendor payee: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("vendor payee not found")
	}

	return nil
}

// Search searches for vendor payees with filters
func (r *PgPayeeVendorRepository) Search(ctx context.Context, filters payee.VendorSearchFilters) ([]*payee.PayeeVendor, error) {
	query := `
		SELECT payee_id, gstin, legal_name, business_type, registration_date, gst_status,
		       building_name, street, city, state, pincode,
		       gst_verification_status, gst_verified_at, gst_data, metadata, created_at, updated_at
		FROM xcentro.payee_vendor
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
		return nil, fmt.Errorf("failed to search vendor payees: %w", err)
	}
	defer rows.Close()

	var vendors []*payee.PayeeVendor

	for rows.Next() {
		var v payee.PayeeVendor
		var businessType sql.NullString
		var registrationDate sql.NullTime
		var gstVerifiedAt sql.NullTime
		var buildingName, street, city, state, pincode sql.NullString
		var gstDataJSON, metadataJSON []byte
		var updatedAt sql.NullTime

		err := rows.Scan(
			&v.PayeeID,
			&v.GSTIN,
			&v.LegalName,
			&businessType,
			&registrationDate,
			&v.GSTStatus,
			&buildingName,
			&street,
			&city,
			&state,
			&pincode,
			&v.GSTVerificationStatus,
			&gstVerifiedAt,
			&gstDataJSON,
			&metadataJSON,
			&v.CreatedAt,
			&updatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan vendor payee: %w", err)
		}

		// Handle nullable fields
		v.BusinessType = businessType.String
		if registrationDate.Valid {
			v.RegistrationDate = &registrationDate.Time
		}
		if gstVerifiedAt.Valid {
			v.GSTVerifiedAt = &gstVerifiedAt.Time
		}
		if updatedAt.Valid {
			v.UpdatedAt = updatedAt.Time
		}

		// Build address
		v.PrimaryAddress = payee.Address{
			BuildingName: buildingName.String,
			Street:       street.String,
			City:         city.String,
			State:        state.String,
			Pincode:      pincode.String,
		}

		// Deserialize JSONB fields
		if len(gstDataJSON) > 0 {
			if err := json.Unmarshal(gstDataJSON, &v.GSTData); err != nil {
				return nil, fmt.Errorf("failed to unmarshal gst_data: %w", err)
			}
		}
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &v.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		vendors = append(vendors, &v)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating vendor payees: %w", err)
	}

	return vendors, nil
}
