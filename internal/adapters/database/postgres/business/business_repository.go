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
	var kybStatus, businessType sql.NullString

	err := r.querier.QueryRow(ctx, query, bid).Scan(
		&b.BID, &b.Name, &b.GSTIN, &gstData, &incomeTexID, &kybStatus,
		&businessType,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("business not found")
		}
		return nil, fmt.Errorf("error finding business: %w", err)
	}

	b.GSTINData = gstData.String
	b.ITID = incomeTexID.String
	b.KybStatus = shared.KYBstatus(kybStatus.String)
	b.BusinessType = shared.BusinessType(businessType.String)

	return &b, nil
}

// FindByGSTIN finds a business by its GSTIN
func (r *PgBusinessRepository) FindByGSTIN(ctx context.Context, gstin string) (*business.Business, error) {
	query := `
		SELECT 
			bid, name, gstin, gst_data, income_tax_id, kyb_status, 
			business_type
		FROM xcentro.business 
		WHERE gstin = $1
	`

	var b business.Business
	var gstData sql.NullString
	var incomeTexID sql.NullString
	var kybStatus sql.NullString
	var businessType sql.NullString

	err := r.querier.QueryRow(ctx, query, gstin).Scan(
		&b.BID, &b.Name, &b.GSTIN, &gstData, &incomeTexID, &kybStatus,
		&businessType,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("error finding business: %w", err)
	}

	b.GSTINData = gstData.String
	b.ITID = incomeTexID.String
	b.KybStatus = shared.KYBstatus(kybStatus.String)
	b.BusinessType = shared.BusinessType(businessType.String)

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

func (r *PgBusinessRepository) UpdateBusinessDeedDocumentKey(ctx context.Context, bid string, key string) error {
	query := `
		UPDATE xcentro.kyb_data
		SET business_deed = $2
		WHERE income_tax_id = (
			SELECT income_tax_id
			FROM xcentro.business
			WHERE bid = $1
		)
	`

	result, err := r.querier.Exec(ctx, query, bid, key)
	if err != nil {
		return fmt.Errorf("error updating business deed document key: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("business not found or KYB data not available")
	}

	return nil
}

// UpdateGSTData updates the GST data JSONB for a business
func (r *PgBusinessRepository) UpdateGSTData(ctx context.Context, bid string, gstDataJSON string) error {
	query := `
		UPDATE xcentro.business
		SET gst_data = $2::jsonb
		WHERE bid = $1
	`

	result, err := r.querier.Exec(ctx, query, bid, gstDataJSON)
	if err != nil {
		return fmt.Errorf("error updating GST data: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("business not found")
	}

	return nil
}

// GetBusinessWithKYBData retrieves both business and KYB data in a single query
func (r *PgBusinessRepository) GetBusinessWithKYBData(ctx context.Context, bid string) (*business.Business, *business.BusinessKYBData, error) {
	query := `
		SELECT
			b.bid, b.name, b.gstin, b.gst_data, b.income_tax_id,
			b.kyb_status, b.business_type, b.country_code, b.currency_code,
			k.income_tax_id, k.income_tax_id_data, k.company_id, k.company_id_data, k.business_deed
		FROM xcentro.business b
		LEFT JOIN xcentro.kyb_data k ON b.income_tax_id = k.income_tax_id
		WHERE b.bid = $1
	`

	var b business.Business
	var kybData business.BusinessKYBData
	var gstData, incomeTexID, kybStatus, businessType sql.NullString
	var countryCode, currencyCode sql.NullString
	var kybITID, kybITIDData, kybCID, kybCIDData, kybDocUrl sql.NullString

	err := r.querier.QueryRow(ctx, query, bid).Scan(
		&b.BID, &b.Name, &b.GSTIN, &gstData, &incomeTexID,
		&kybStatus, &businessType, &countryCode, &currencyCode,
		&kybITID, &kybITIDData, &kybCID, &kybCIDData, &kybDocUrl,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil, fmt.Errorf("business not found")
		}
		return nil, nil, fmt.Errorf("error finding business with KYB data: %w", err)
	}

	b.GSTINData = gstData.String
	b.ITID = incomeTexID.String
	b.KybStatus = shared.KYBstatus(kybStatus.String)
	b.BusinessType = shared.BusinessType(businessType.String)
	b.CountryCode = countryCode.String
	b.CurrencyCode = currencyCode.String

	// KYB data might be null if not yet created
	if kybITID.Valid {
		kybData.ITID = kybITID.String
		kybData.ITIDData = kybITIDData.String
		kybData.CID = kybCID.String
		kybData.CIDData = kybCIDData.String
		kybData.DocUrl = kybDocUrl.String
		return &b, &kybData, nil
	}

	return &b, nil, nil
}
