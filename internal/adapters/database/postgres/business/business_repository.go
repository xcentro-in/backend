package business

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/shared"
)

// PgBusinessRepository implements business.BusinessRepository interface using PostgreSQL
type PgBusinessRepository struct {
	querier client.Querier
}

// NewPgBusinessRepository creates a new PostgreSQL business repository
func NewPgBusinessRepository(db client.DB) business.BusinessRepository {
	return &PgBusinessRepository{
		querier: db,
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgBusinessRepository) WithTx(tx client.Tx) business.BusinessRepository {
	return &PgBusinessRepository{
		querier: tx,
	}
}

// FindByID finds a business by its ID
func (r *PgBusinessRepository) FindByID(ctx context.Context, bid string) (*business.Business, error) {
	query := `
		SELECT 
			bid, name, gstin, gst_data, income_tax_id, kyb_status, 
			business_type, country_code, currency_code
		FROM xcentro.business 
		WHERE bid = $1
	`

	var b business.Business
	var gstData sql.NullString
	var incomeTexID sql.NullString
	var kybStatus, businessType string

	err := r.querier.QueryRow(ctx, query, bid).Scan(
		&b.BID, &b.Name, &b.GSTIN, &gstData, &incomeTexID, &kybStatus,
		&businessType, &b.CountryCode, &b.CurrencyCode,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("business not found")
		}
		return nil, fmt.Errorf("error finding business: %w", err)
	}

	b.GSTINData = gstData.String
	b.ITID = incomeTexID.String
	b.KybStatus = shared.KYBstatus(kybStatus)
	b.BusinessType = shared.BusinessType(businessType)

	return &b, nil
}

// FindByGSTIN finds a business by its GSTIN
func (r *PgBusinessRepository) FindByGSTIN(ctx context.Context, gstin string) (*business.Business, error) {
	query := `
		SELECT 
			bid, name, gstin, gst_data, income_tax_id, kyb_status, 
			business_type, country_code, currency_code
		FROM xcentro.business 
		WHERE gstin = $1
	`

	var b business.Business
	var gstData sql.NullString
	var incomeTexID sql.NullString
	var kybStatus, businessType string

	err := r.querier.QueryRow(ctx, query, gstin).Scan(
		&b.BID, &b.Name, &b.GSTIN, &gstData, &incomeTexID, &kybStatus,
		&businessType, &b.CountryCode, &b.CurrencyCode,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("business not found")
		}
		return nil, fmt.Errorf("error finding business: %w", err)
	}

	b.GSTINData = gstData.String
	b.ITID = incomeTexID.String
	b.KybStatus = shared.KYBstatus(kybStatus)
	b.BusinessType = shared.BusinessType(businessType)

	return &b, nil
}

func (r *PgBusinessRepository) CreateBusinessByNameAndGSTIN(ctx context.Context, name string, gstin string) (*business.Business, error) {
	query := `
		INSERT INTO xcentro.business 
			(name, gstin, kyb_status) 
		VALUES ($1, $2, $3)
		RETURNING bid, name, gstin
	`

	var createdBusiness business.Business

	err := r.querier.QueryRow(ctx, query, name, gstin, shared.KYBInProgress).Scan(
		&createdBusiness.BID, &createdBusiness.Name, &createdBusiness.GSTIN,
	)

	if err != nil {
		return nil, fmt.Errorf("error creating business: %w", err)
	}

	return &createdBusiness, nil
}

func (r *PgBusinessRepository) AddKYBSuccessData(ctx context.Context, bus *business.Business) error {
	query := `
		UPDATE xcentro.business 
		SET kyb_status = $2, business_type = $3, income_tax_id = $4
		WHERE bid = $1
	`

	result, err := r.querier.Exec(ctx, query,
		bus.BID, bus.KybStatus, bus.BusinessType, bus.ITID,
	)

	if err != nil {
		return fmt.Errorf("error updating business KYB success metadata: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("business not found")
	}

	return nil
}

// UpdateKYBData updates the KYB data for a business
func (r *PgBusinessRepository) UpdateKYBData(ctx context.Context, bid string, kybData *business.BusinessKYBData) (*business.BusinessKYBData, error) {
	// First, update or create KYB data
	kybQuery := `
		INSERT INTO xcentro.kyb_data (income_tax_id, income_tax_id_data, company_id, company_id_data, business_deed)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (income_tax_id) 
		DO UPDATE SET 
			income_tax_id_data = $2, 
			company_id = $3, 
			company_id_data = $4, 
			business_deed = $5
		RETURNING income_tax_id
	`

	err := r.querier.QueryRow(ctx, kybQuery,
		kybData.ITID, kybData.ITIDData, kybData.CID, kybData.CIDData, kybData.DocUrl,
	).Scan(&kybData.ITID)

	if err != nil {
		return nil, fmt.Errorf("error updating KYB data: %w", err)
	}

	// Update business to reference the KYB data
	businessQuery := `
		UPDATE xcentro.business 
		SET income_tax_id = $2 
		WHERE bid = $1
	`

	result, err := r.querier.Exec(ctx, businessQuery, bid, kybData.ITID)
	if err != nil {
		return nil, fmt.Errorf("error linking KYB data to business: %w", err)
	}

	if result.RowsAffected() == 0 {
		return nil, fmt.Errorf("business not found")
	}

	return kybData, nil
}
