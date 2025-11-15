package business

import (
	"context"
	"database/sql"
	"fmt"

	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/shared"
)

// PgBusinessUserRelationRepository implements business.BusinessUserRelationRepository using PostgreSQL
type PgBusinessUserRelationRepository struct {
	querier client.Querier
}

// NewPgBusinessUserRelationRepository creates a new PostgreSQL business user relation repository
func NewPgBusinessUserRelationRepository(db client.DB) business.BusinessUserRelationRepository {
	return &PgBusinessUserRelationRepository{
		querier: db,
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgBusinessUserRelationRepository) WithTx(tx client.Tx) business.BusinessUserRelationRepository {
	return &PgBusinessUserRelationRepository{
		querier: tx,
	}
}

func (r *PgBusinessUserRelationRepository) UpdateKYBInitiated(ctx context.Context, uid string, bid string, initiated bool) error {
	query := `
		UPDATE xcentro.business_user_relation
		SET kyb_initiated = $3
		WHERE user_id = $1 AND bid = $2
	`

	result, err := r.querier.Exec(ctx, query, uid, bid, initiated)
	if err != nil {
		return fmt.Errorf("error updating KYB initiated status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("user-business relation not found")
	}

	return nil
}

// UpdateKycNeeded updates the kyc_needed flag for a user-business relation
func (r *PgBusinessUserRelationRepository) UpdateKycNeeded(ctx context.Context, userID string, bid string, kycNeeded bool) error {
	query := `
		UPDATE xcentro.business_user_relation
		SET kyc_needed = $3
		WHERE user_id = $1 AND bid = $2
	`

	result, err := r.querier.Exec(ctx, query, userID, bid, kycNeeded)
	if err != nil {
		return fmt.Errorf("error updating kyc_needed status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("user-business relation not found")
	}

	return nil
}

// FindByUser finds all business relations for a user
func (r *PgBusinessUserRelationRepository) FindByUser(ctx context.Context, userID string) ([]*business.BusinessUserRelation, error) {
	query := `
		SELECT
			user_id, bid, role, role_claim, kyb_initiated, kyc_needed
		FROM xcentro.business_user_relation
		WHERE user_id = $1
	`

	rows, err := r.querier.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("error finding business relations for user: %w", err)
	}
	defer rows.Close()

	var relations []*business.BusinessUserRelation

	for rows.Next() {
		var rel business.BusinessUserRelation
		var role, roleClaim sql.NullString

		err := rows.Scan(
			&rel.UserID, &rel.BID, &role, &roleClaim, &rel.KybInitiated, &rel.KycNeeded,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning business relation: %w", err)
		}

		rel.Role = shared.UserRole(role.String)
		rel.RoleClaim = shared.UserRole(roleClaim.String)
		relations = append(relations, &rel)
	}

	return relations, nil
}

// FindByBusiness finds all user relations for a business
func (r *PgBusinessUserRelationRepository) FindByBusiness(ctx context.Context, bid string) ([]*business.BusinessUserRelation, error) {
	query := `
		SELECT
			user_id, bid, role, role_claim, added_at::text, kyb_initiated, kyc_needed, COALESCE(updated_at::text, '')
		FROM xcentro.business_user_relation
		WHERE bid = $1
	`

	rows, err := r.querier.Query(ctx, query, bid)
	if err != nil {
		return nil, fmt.Errorf("error finding user relations for business: %w", err)
	}
	defer rows.Close()

	var relations []*business.BusinessUserRelation

	for rows.Next() {
		var rel business.BusinessUserRelation
		var role, roleClaim sql.NullString

		err := rows.Scan(
			&rel.UserID, &rel.BID, &role, &roleClaim,
			&rel.AddedAt, &rel.KybInitiated, &rel.KycNeeded, &rel.Timestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning business relation: %w", err)
		}

		rel.Role = shared.UserRole(role.String)
		rel.RoleClaim = shared.UserRole(roleClaim.String)
		relations = append(relations, &rel)
	}

	return relations, nil
}

// GetBusinessUsersWithDetails retrieves all users for a business with their detailed information
func (r *PgBusinessUserRelationRepository) GetBusinessUsersWithDetails(ctx context.Context, bid string) ([]*business.BusinessUserDTO, error) {
	query := `
		SELECT
			bur.user_id,
			u.email,
			u.name,
			bur.role,
			bur.role_claim,
			bur.kyc_needed,
			bur.added_at::text,
			COALESCE(bur.updated_at::text, '')
		FROM xcentro.business_user_relation bur
		INNER JOIN xcentro.user_table u ON bur.user_id = u.id
		WHERE bur.bid = $1
		ORDER BY bur.added_at DESC
	`

	rows, err := r.querier.Query(ctx, query, bid)
	if err != nil {
		return nil, fmt.Errorf("error finding users for business: %w", err)
	}
	defer rows.Close()

	var users []*business.BusinessUserDTO

	for rows.Next() {
		var user business.BusinessUserDTO
		var role, roleClaim sql.NullString

		err := rows.Scan(
			&user.UserID,
			&user.Email,
			&user.Name,
			&role,
			&roleClaim,
			&user.KycNeeded,
			&user.AddedAt,
			&user.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning business user: %w", err)
		}

		user.Role = shared.UserRole(role.String)
		user.RoleClaim = shared.UserRole(roleClaim.String)
		users = append(users, &user)
	}

	return users, nil
}

// AddUserToBusiness adds a user to a business with a specific role
func (r *PgBusinessUserRelationRepository) AddUserToBusiness(ctx context.Context, relation *business.BusinessUserRelation) error {
	query := `
		INSERT INTO xcentro.business_user_relation
			(user_id, bid, role, role_claim, kyb_initiated, kyc_needed)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	// Convert empty strings to NULL for enum columns
	var roleParam, roleClaimParam interface{}
	if relation.Role == "" {
		roleParam = nil
	} else {
		roleParam = relation.Role
	}

	if relation.RoleClaim == "" {
		roleClaimParam = nil
	} else {
		roleClaimParam = relation.RoleClaim
	}

	_, err := r.querier.Exec(ctx, query,
		relation.UserID, relation.BID, roleParam,
		roleClaimParam, relation.KybInitiated, relation.KycNeeded,
	)

	if err != nil {
		return fmt.Errorf("error adding user to business: %w", err)
	}

	return nil
}

// UpdateUserRole updates a user's role in a business
func (r *PgBusinessUserRelationRepository) UpdateUserRole(ctx context.Context, userID string, bid string, newRole shared.UserRole) error {
	query := `
		UPDATE xcentro.business_user_relation
		SET role = $3
		WHERE user_id = $1 AND bid = $2
	`

	// Convert empty string to NULL for enum column
	var roleParam interface{}
	if newRole == "" {
		roleParam = nil
	} else {
		roleParam = newRole
	}

	result, err := r.querier.Exec(ctx, query, userID, bid, roleParam)
	if err != nil {
		return fmt.Errorf("error updating user role: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("user-business relation not found")
	}

	return nil
}

func (r *PgBusinessUserRelationRepository) UpdateRoleClaim(ctx context.Context, userID string, bid string, roleClaim shared.UserRole) error {
	query := `
		UPDATE xcentro.business_user_relation
		SET role_claim = $3
		WHERE user_id = $1 AND bid = $2
	`

	// Convert empty string to NULL for enum column
	var roleClaimParam interface{}
	if roleClaim == "" {
		roleClaimParam = nil
	} else {
		roleClaimParam = roleClaim
	}

	result, err := r.querier.Exec(ctx, query, userID, bid, roleClaimParam)
	if err != nil {
		return fmt.Errorf("error updating role claim: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("user-business relation not found")
	}

	return nil
}

// RemoveUserFromBusiness removes a user from a business
func (r *PgBusinessUserRelationRepository) RemoveUserFromBusiness(ctx context.Context, userID string, bid string) error {
	query := `
		DELETE FROM xcentro.business_user_relation 
		WHERE user_id = $1 AND bid = $2
	`

	result, err := r.querier.Exec(ctx, query, userID, bid)
	if err != nil {
		return fmt.Errorf("error removing user from business: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("user-business relation not found")
	}

	return nil
}

func (r *PgBusinessUserRelationRepository) GetKYBInitiatedStatus(ctx context.Context, uid string) (*business.BusinessKYBData, *business.Business, error) {
	var kybData business.BusinessKYBData
	var bus business.Business

	// Use sql.NullString for nullable columns only
	var (
		nsBusIncomeTax sql.NullString
		nsBusGstData   sql.NullString
		nsCid          sql.NullString
		nsCidData      sql.NullString
		nsDocUrl       sql.NullString
		nsBid          sql.NullString
		nsName         sql.NullString
		nsGstin        sql.NullString
		kybStatusStr   string // NOT NULL with DEFAULT
		nsBusinessType sql.NullString
		nsCountryCode  sql.NullString
		nsCurrencyCode sql.NullString
	)

	query := `
	SELECT
		bus.income_tax_id AS bus_income_tax_id,
		bus.gst_data      AS bus_gst_data,
		kd.company_id,
		kd.company_id_data,
		kd.business_deed,
		bus.bid,
		bus.name,
		bus.gstin,
		bus.kyb_status,
		bus.business_type,
		bus.country_code,
		bus.currency_code
	FROM xcentro.business_user_relation bur
	INNER JOIN xcentro.business bus ON bur.bid = bus.bid
	LEFT JOIN xcentro.kyb_data kd ON bus.income_tax_id = kd.income_tax_id
	WHERE bur.user_id = $1 AND bur.kyb_initiated = true
	ORDER BY bur.updated_at DESC
	LIMIT 1
	`

	err := r.querier.QueryRow(ctx, query, uid).Scan(
		&nsBusIncomeTax, &nsBusGstData, &nsCid, &nsCidData, &nsDocUrl,
		&nsBid, &nsName, &nsGstin, &kybStatusStr, &nsBusinessType, &nsCountryCode, &nsCurrencyCode,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("error fetching KYB initiated status: %w", err)
	}

	// Map NullString values to your structs (empty string when NULL)
	kybData.ITID = nsBusIncomeTax.String
	kybData.ITIDData = nsBusGstData.String
	kybData.CID = nsCid.String
	kybData.CIDData = nsCidData.String
	kybData.DocUrl = nsDocUrl.String

	bus.BID = nsBid.String
	bus.Name = nsName.String
	bus.GSTIN = nsGstin.String
	bus.KybStatus = shared.KYBstatus(kybStatusStr)
	bus.BusinessType = shared.BusinessType(nsBusinessType.String)
	bus.CountryCode = nsCountryCode.String
	bus.CurrencyCode = nsCurrencyCode.String

	// If KYB is explicitly "not started", return nils like before
	if bus.KybStatus == shared.KYBNotStarted {
		return nil, nil, nil
	}

	return &kybData, &bus, nil
}

// SetOwnerRoleToNull sets all user roles to NULL for a business (used when KYB is rejected)
func (r *PgBusinessUserRelationRepository) SetOwnerRoleToNull(ctx context.Context, bid string) error {
	query := `UPDATE xcentro.business_user_relation SET role = NULL WHERE bid = $1`
	_, err := r.querier.Exec(ctx, query, bid)
	if err != nil {
		return fmt.Errorf("failed to set roles to NULL: %w", err)
	}
	return nil
}

// ResetAllUserRelationsForRejectedKYC resets all business relations for a user (role='none', kyc_needed=true)
func (r *PgBusinessUserRelationRepository) ResetAllUserRelationsForRejectedKYC(ctx context.Context, userID string) error {
	query := `UPDATE xcentro.business_user_relation SET role = 'none', kyc_needed = true WHERE user_id = $1`
	_, err := r.querier.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to reset user relations for rejected KYC: %w", err)
	}
	return nil
}

// OpsUpdateUserRole updates a user's role in a business (ops endpoint - simple update with WHERE check)
func (r *PgBusinessUserRelationRepository) OpsUpdateUserRole(ctx context.Context, bid string, userID string, role shared.UserRole) error {
	query := `UPDATE xcentro.business_user_relation SET role = $1 WHERE bid = $2 AND user_id = $3`
	result, err := r.querier.Exec(ctx, query, role, bid, userID)
	if err != nil {
		return fmt.Errorf("failed to update user role: %w", err)
	}

	// Check if any row was updated
	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("business-user relation not found")
	}

	return nil
}
