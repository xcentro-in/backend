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
			business_type, business_model, business_turnover, country_code, currency_code,
			has_completed_onboarding, has_added_payment_method, has_added_vendor, has_created_first_payment
		FROM xcentro.business
		WHERE bid = $1
	`

	var b business.Business
	var gstData sql.NullString
	var incomeTexID sql.NullString
	var kybStatusStr string // NOT NULL with DEFAULT
	var businessType, businessModel, businessTurnover sql.NullString
	var countryCode, currencyCode sql.NullString
	var hasCompletedOnboarding, hasAddedPaymentMethod, hasAddedVendor, hasCreatedFirstPayment sql.NullBool

	err := r.querier.QueryRow(ctx, query, bid).Scan(
		&b.BID, &b.Name, &b.GSTIN, &gstData, &incomeTexID, &kybStatusStr,
		&businessType, &businessModel, &businessTurnover, &countryCode, &currencyCode,
		&hasCompletedOnboarding, &hasAddedPaymentMethod, &hasAddedVendor, &hasCreatedFirstPayment,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("business not found")
		}
		return nil, fmt.Errorf("error finding business: %w", err)
	}

	b.GSTINData = gstData.String
	b.ITID = incomeTexID.String
	b.KybStatus = shared.KYBstatus(kybStatusStr)
	b.BusinessType = shared.BusinessType(businessType.String)
	b.BusinessModel = businessModel.String
	b.BusinessTurnover = businessTurnover.String
	b.CountryCode = countryCode.String
	b.CurrencyCode = currencyCode.String
	b.HasCompletedOnboarding = hasCompletedOnboarding.Bool
	b.HasAddedPaymentMethod = hasAddedPaymentMethod.Bool
	b.HasAddedVendor = hasAddedVendor.Bool
	b.HasCreatedFirstPayment = hasCreatedFirstPayment.Bool

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
	var kybStatusStr string // NOT NULL with DEFAULT
	var businessType sql.NullString

	err := r.querier.QueryRow(ctx, query, gstin).Scan(
		&b.BID, &b.Name, &b.GSTIN, &gstData, &incomeTexID, &kybStatusStr,
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
	b.KybStatus = shared.KYBstatus(kybStatusStr)
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
		SET kyb_status = $2, business_type = $3, income_tax_id = $4,
		    business_model = CASE WHEN $5 != '' THEN $5 ELSE business_model END,
		    business_turnover = CASE WHEN $6 != '' THEN $6 ELSE business_turnover END
		WHERE bid = $1
	`

	result, err := r.querier.Exec(ctx, query,
		bus.BID, bus.KybStatus, bus.BusinessType, bus.ITID,
		bus.BusinessModel, bus.BusinessTurnover,
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
		VALUES ($1, $2::jsonb, $3, $4::jsonb, $5)
		ON CONFLICT (income_tax_id)
		DO UPDATE SET
			income_tax_id_data = $2::jsonb,
			company_id = $3,
			company_id_data = $4::jsonb,
			business_deed = $5
		RETURNING income_tax_id
	`

	// Convert empty strings to NULL for JSONB columns
	var itidDataParam, cidDataParam interface{}
	if kybData.ITIDData == "" {
		itidDataParam = nil
	} else {
		itidDataParam = kybData.ITIDData
	}

	if kybData.CIDData == "" {
		cidDataParam = nil
	} else {
		cidDataParam = kybData.CIDData
	}

	err := r.querier.QueryRow(ctx, kybQuery,
		kybData.ITID, itidDataParam, kybData.CID, cidDataParam, kybData.DocUrl,
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

// GetKYBDataByPAN retrieves KYB data by PAN (income_tax_id)
func (r *PgBusinessRepository) GetKYBDataByPAN(ctx context.Context, pan string) (*business.BusinessKYBData, error) {
	query := `
		SELECT income_tax_id, income_tax_id_data, company_id, company_id_data, business_deed
		FROM xcentro.kyb_data
		WHERE income_tax_id = $1
	`

	var kybData business.BusinessKYBData
	var itidData, cid, cidData, docUrl sql.NullString

	err := r.querier.QueryRow(ctx, query, pan).Scan(
		&kybData.ITID, &itidData, &cid, &cidData, &docUrl,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // PAN not found
		}
		return nil, fmt.Errorf("error querying KYB data by PAN: %w", err)
	}

	kybData.ITIDData = itidData.String
	kybData.CID = cid.String
	kybData.CIDData = cidData.String
	kybData.DocUrl = docUrl.String

	return &kybData, nil
}

// GetBusinessWithKYBData retrieves both business and KYB data in a single query
func (r *PgBusinessRepository) GetBusinessWithKYBData(ctx context.Context, bid string) (*business.Business, *business.BusinessKYBData, error) {
	query := `
		SELECT
			b.bid, b.name, b.gstin, b.gst_data, b.income_tax_id,
			b.kyb_status, b.business_type, b.business_model, b.business_turnover,
			b.country_code, b.currency_code,
			k.income_tax_id, k.income_tax_id_data, k.company_id, k.company_id_data, k.business_deed
		FROM xcentro.business b
		LEFT JOIN xcentro.kyb_data k ON b.income_tax_id = k.income_tax_id
		WHERE b.bid = $1
	`

	var b business.Business
	var kybData business.BusinessKYBData
	var gstData, incomeTexID sql.NullString
	var kybStatusStr string // NOT NULL with DEFAULT
	var businessType, businessModel, businessTurnover sql.NullString
	var countryCode, currencyCode sql.NullString
	var kybITID, kybITIDData, kybCID, kybCIDData, kybDocUrl sql.NullString

	err := r.querier.QueryRow(ctx, query, bid).Scan(
		&b.BID, &b.Name, &b.GSTIN, &gstData, &incomeTexID,
		&kybStatusStr, &businessType, &businessModel, &businessTurnover,
		&countryCode, &currencyCode,
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
	b.KybStatus = shared.KYBstatus(kybStatusStr)
	b.BusinessType = shared.BusinessType(businessType.String)
	b.BusinessModel = businessModel.String
	b.BusinessTurnover = businessTurnover.String
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

// UpdateOnboardingFlag updates a specific onboarding flag for a business.
func (r *PgBusinessRepository) UpdateOnboardingFlag(ctx context.Context, bid string, flagName string, value bool) error {
	query := `
		UPDATE xcentro.business
		SET
			has_completed_onboarding = CASE WHEN $2 = 'has_completed_onboarding' THEN $3 ELSE has_completed_onboarding END,
			has_added_payment_method = CASE WHEN $2 = 'has_added_payment_method' THEN $3 ELSE has_added_payment_method END,
			has_added_vendor = CASE WHEN $2 = 'has_added_vendor' THEN $3 ELSE has_added_vendor END,
			has_created_first_payment = CASE WHEN $2 = 'has_created_first_payment' THEN $3 ELSE has_created_first_payment END
		WHERE bid = $1
	`

	result, err := r.querier.Exec(ctx, query, bid, flagName, value)
	if err != nil {
		return fmt.Errorf("error updating onboarding flag '%s' for business %s: %w", flagName, bid, err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("business %s not found or flag '%s' not updated", bid, flagName)
	}

	return nil
}

// ListBusinessesPaginated retrieves paginated businesses sorted by KYB status
func (r *PgBusinessRepository) ListBusinessesPaginated(ctx context.Context, page int, limit int) ([]*business.OpsBusinessListItem, int, error) {
	offset := (page - 1) * limit

	// Get total count
	countQuery := `SELECT COUNT(*) FROM xcentro.business`
	var totalCount int
	err := r.querier.QueryRow(ctx, countQuery).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("error counting businesses: %w", err)
	}

	// Get paginated results with custom sort
	query := `
		SELECT
			bid,
			name,
			gstin,
			kyb_status,
			business_type,
			COALESCE(updated_at::text, '') as updated_at
		FROM
			xcentro.business
		ORDER BY
			CASE kyb_status
				WHEN 'kyc_completed' THEN 1
				WHEN 'kyc_inprogress' THEN 2
				WHEN 'kyb_verified' THEN 99
				ELSE 3
			END ASC,
			updated_at DESC
		LIMIT $1
		OFFSET $2
	`

	rows, err := r.querier.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("error querying businesses: %w", err)
	}
	defer rows.Close()

	var businesses []*business.OpsBusinessListItem
	for rows.Next() {
		var item business.OpsBusinessListItem
		var kybStatusStr string
		var businessType sql.NullString

		err := rows.Scan(
			&item.BID,
			&item.Name,
			&item.GSTIN,
			&kybStatusStr,
			&businessType,
			&item.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("error scanning business row: %w", err)
		}

		item.KybStatus = shared.KYBstatus(kybStatusStr)
		item.BusinessType = shared.BusinessType(businessType.String)
		businesses = append(businesses, &item)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating business rows: %w", err)
	}

	return businesses, totalCount, nil
}

// SearchBusinesses searches businesses by name or GSTIN
func (r *PgBusinessRepository) SearchBusinesses(ctx context.Context, searchTerm string) ([]*business.OpsBusinessSearchResult, error) {
	query := `
		SELECT
			bid,
			name,
			gstin
		FROM
			xcentro.business
		WHERE
			name ILIKE $1 OR
			gstin ILIKE $1
		LIMIT 10
	`

	searchPattern := searchTerm + "%"
	rows, err := r.querier.Query(ctx, query, searchPattern)
	if err != nil {
		return nil, fmt.Errorf("error searching businesses: %w", err)
	}
	defer rows.Close()

	var results []*business.OpsBusinessSearchResult
	for rows.Next() {
		var result business.OpsBusinessSearchResult
		err := rows.Scan(&result.BID, &result.Name, &result.GSTIN)
		if err != nil {
			return nil, fmt.Errorf("error scanning search result: %w", err)
		}
		results = append(results, &result)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating search results: %w", err)
	}

	return results, nil
}

// GetBusinessDetailsForOps retrieves complete business details with KYB data and users
func (r *PgBusinessRepository) GetBusinessDetailsForOps(ctx context.Context, bid string) (*business.OpsBusinessDetails, error) {
	query := `
		SELECT
			b.bid, b.name, b.gstin, COALESCE(b.gst_data::text, ''), b.income_tax_id,
			b.kyb_status, b.business_type, b.business_model, b.business_turnover,
			b.country_code, b.currency_code, COALESCE(b.updated_at::text, ''),
			COALESCE(k.income_tax_id_data::text, ''), COALESCE(k.company_id, ''),
			COALESCE(k.company_id_data::text, ''), COALESCE(k.business_deed, '')
		FROM xcentro.business b
		LEFT JOIN xcentro.kyb_data k ON b.income_tax_id = k.income_tax_id
		WHERE b.bid = $1
	`

	var details business.OpsBusinessDetails
	var kybStatusStr string
	var businessType, businessModel, businessTurnover, countryCode, currencyCode sql.NullString

	err := r.querier.QueryRow(ctx, query, bid).Scan(
		&details.BID, &details.Name, &details.GSTIN, &details.GSTData, &details.ITID,
		&kybStatusStr, &businessType, &businessModel, &businessTurnover,
		&countryCode, &currencyCode, &details.UpdatedAt,
		&details.ITIDData, &details.CID, &details.CIDData, &details.DocUrl,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("business not found")
		}
		return nil, fmt.Errorf("error fetching business details: %w", err)
	}

	details.KybStatus = shared.KYBstatus(kybStatusStr)
	details.BusinessType = shared.BusinessType(businessType.String)
	details.BusinessModel = businessModel.String
	details.BusinessTurnover = businessTurnover.String
	details.CountryCode = countryCode.String
	details.CurrencyCode = currencyCode.String

	// Fetch users related to this business
	usersQuery := `
		SELECT
			u.id, u.email, u.name, u.mobile,
			bur.role, bur.role_claim, u.kyc_status, bur.kyc_needed,
			COALESCE(ui.tax_id, ''), COALESCE(ui.national_id, ''),
			u.email_verified, u.mobile_verified,
			bur.added_at::text, COALESCE(bur.updated_at::text, '')
		FROM xcentro.business_user_relation bur
		JOIN xcentro.user_table u ON bur.userid = u.id
		LEFT JOIN xcentro.user_identity ui ON u.tax_id = ui.tax_id
		WHERE bur.bid = $1
		ORDER BY bur.added_at DESC
	`

	rows, err := r.querier.Query(ctx, usersQuery, bid)
	if err != nil {
		return nil, fmt.Errorf("error fetching business users: %w", err)
	}
	defer rows.Close()

	var users []*business.OpsUserDetails
	for rows.Next() {
		var user business.OpsUserDetails
		var roleStr, roleClaimStr, kycStatusStr string

		err := rows.Scan(
			&user.UserID, &user.Email, &user.Name, &user.Mobile,
			&roleStr, &roleClaimStr, &kycStatusStr, &user.KycNeeded,
			&user.TaxID, &user.NationalID,
			&user.EmailVerified, &user.MobileVerified,
			&user.AddedAt, &user.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning user row: %w", err)
		}

		user.Role = shared.UserRole(roleStr)
		user.RoleClaim = shared.UserRole(roleClaimStr)
		user.KycStatus = shared.KYCStatus(kycStatusStr)
		users = append(users, &user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating user rows: %w", err)
	}

	details.Users = users
	return &details, nil
}
