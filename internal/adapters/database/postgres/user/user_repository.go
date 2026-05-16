package user

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/shared"
	"xcentro.in/internal/core/user"
)

// PgUserRepository implements user.UserRepository interface using PostgreSQL
type PgUserRepository struct {
	querier client.Querier // Use Querier interface instead of DB
}

// NewPgUserRepository creates a new PostgreSQL user repository
func NewPgUserRepository(db client.DB) *PgUserRepository {
	return &PgUserRepository{
		querier: db,
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgUserRepository) WithTx(tx client.Tx) user.UserRepository {
	return &PgUserRepository{
		querier: tx,
	}
}

// CreateUser creates a new user from auth payload
func (r *PgUserRepository) CreateUser(ctx context.Context, authPayload auth.AuthPayload) (*user.User, error) {
	query := `
		INSERT INTO xcentro.user_table (email, pwd_hash, mobile, tax_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id, email;
	`

	var u user.User

	err := r.querier.QueryRow(ctx, query,
		authPayload.Email,
		authPayload.Password,
		"",  // mobile - empty initially
		nil, // tax_id - null initially
	).Scan(
		&u.Id,
		&u.Email,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return &u, nil
}

// GetUserByID retrieves a user by ID
func (r *PgUserRepository) GetUserByID(ctx context.Context, userID string) (*user.User, error) {
	query := `
		SELECT id, email, pwd_hash, mobile, name, dob, tax_id, kyc_status, kyc_needed, email_verified, mobile_verified,
		       COALESCE(active_bid::text, '') as active_bid
		FROM xcentro.user_table
		WHERE id = $1
	`

	var u user.User
	var kycStatusStr string
	var mobile, taxID, name, dob sql.NullString

	err := r.querier.QueryRow(ctx, query, userID).Scan(
		&u.Id,
		&u.Email,
		&u.PwdHash,
		&mobile,
		&name,
		&dob,
		&taxID,
		&kycStatusStr,
		&u.KycNeeded,
		&u.EmailVerified,
		&u.MobileVerified,
		&u.ActiveBid,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get user by ID: %w", err)
	}

	u.Mobile = mobile.String
	u.TaxID = taxID.String
	u.Name = name.String
	u.DOB = dob.String
	u.KycStatus = parseKYCStatus(kycStatusStr)

	return &u, nil
}

// SetActiveBid updates the active_bid column for a user.
// Pass an empty string to set the column to NULL.
func (r *PgUserRepository) SetActiveBid(ctx context.Context, userID string, activeBid string) error {
	if activeBid == "" {
		// Set to NULL for empty string input
		query := `UPDATE xcentro.user_table SET active_bid = NULL WHERE id = $1`
		_, err := r.querier.Exec(ctx, query, userID)
		return err
	}
	query := `
		UPDATE xcentro.user_table
		SET active_bid = $2::uuid
		WHERE id = $1
	`
	_, err := r.querier.Exec(ctx, query, userID, activeBid)
	return err
}

// GetUserMobileByID retrieves only the mobile number for a user
// Returns the mobile number, or empty string if not set
func (r *PgUserRepository) GetUserMobileByID(ctx context.Context, userID string) (string, error) {
	query := `
		SELECT mobile
		FROM xcentro.user_table
		WHERE id = $1
	`

	var mobile sql.NullString
	err := r.querier.QueryRow(ctx, query, userID).Scan(&mobile)
	if err != nil {
		return "", fmt.Errorf("failed to get user mobile: %w", err)
	}

	return mobile.String, nil
}

// parseKYCStatus converts string enum to KYCStatus
func parseKYCStatus(status string) shared.KYCStatus {
	switch status {
	case "kyc_not_started":
		return shared.KYCNotStarted
	case "kyc_in_progress":
		return shared.KYCInProgress
	case "kyc_completed":
		return shared.KYCCompleted
	case "kyc_verified":
		return shared.KYCVerified
	case "kyc_needs_update":
		return shared.KYCNeedsUpdate
	case "kyc_rejected":
		return shared.KYCRejected
	default:
		return shared.KYCNotStarted
	}
}

// GetUserByEmail retrieves a user by email
func (r *PgUserRepository) GetUserByEmail(ctx context.Context, email string) (*user.User, error) {
	query := `
		SELECT id, email, pwd_hash, mobile, tax_id, kyc_status, kyc_needed, email_verified, mobile_verified
		FROM xcentro.user_table
		WHERE email = $1
	`

	var u user.User
	var mobile, taxID sql.NullString

	err := r.querier.QueryRow(ctx, query, email).Scan(
		&u.Id,
		&u.Email,
		&u.PwdHash,
		&mobile,
		&taxID,
		&u.KycStatus,
		&u.KycNeeded,
		&u.EmailVerified,
		&u.MobileVerified,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	// Handle nullable fields
	if mobile.Valid {
		u.Mobile = mobile.String
	}
	if taxID.Valid {
		u.TaxID = taxID.String
	}

	return &u, nil
}

// UpdateUserMobile updates user's mobile number
func (r *PgUserRepository) UpdateUserMobile(ctx context.Context, userID, newMobile string) error {
	query := `UPDATE xcentro.user_table SET mobile = $1 WHERE id = $2`

	_, err := r.querier.Exec(ctx, query, newMobile, userID)
	if err != nil {
		return fmt.Errorf("failed to update user mobile: %w", err)
	}

	return nil
}

// UpdateUserTaxID updates user's tax ID reference
func (r *PgUserRepository) UpdateUserTaxID(ctx context.Context, userID, newTaxID string) error {
	query := `UPDATE xcentro.user_table SET tax_id = $1 WHERE id = $2`

	_, err := r.querier.Exec(ctx, query, newTaxID, userID)
	if err != nil {
		return fmt.Errorf("failed to update user tax ID: %w", err)
	}

	return nil
}

// UpdateUserKYCStatus updates user's KYC status
func (r *PgUserRepository) UpdateUserKYCStatus(ctx context.Context, userID string, newStatus shared.KYCStatus) error {
	statusStr := kycStatusToString(newStatus)
	query := `UPDATE xcentro.user_table SET kyc_status = $1 WHERE id = $2`

	_, err := r.querier.Exec(ctx, query, statusStr, userID)
	if err != nil {
		return fmt.Errorf("failed to update user KYC status: %w", err)
	}

	return nil
}

// kycStatusToString converts KYCStatus to string enum
func kycStatusToString(status shared.KYCStatus) string {
	switch status {
	case shared.KYCNotStarted:
		return "kyc_not_started"
	case shared.KYCInProgress:
		return "kyc_in_progress"
	case shared.KYCCompleted:
		return "kyc_completed"
	case shared.KYCVerified:
		return "kyc_verified"
	case shared.KYCNeedsUpdate:
		return "kyc_needs_update"
	case shared.KYCRejected:
		return "kyc_rejected"
	default:
		return "kyc_not_started"
	}
}

// UpsertUserIdentity creates or updates user identity data
func (r *PgUserRepository) UpsertUserIdentity(ctx context.Context, identity *user.UserIdentity) error {
	query := `
		INSERT INTO xcentro.user_identity (tax_id, tax_id_data, national_id, national_id_data, n_txn_id)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (tax_id)
		DO UPDATE SET 
			tax_id_data = $2,
			national_id = $3,
			national_id_data = $4,
			n_txn_id = $5
	`

	_, err := r.querier.Exec(ctx, query,
		identity.TaxID, identity.TaxIDData, identity.NationalID, identity.NationalIDData, identity.NTxnID,
	)

	if err != nil {
		return fmt.Errorf("failed to upsert person identity: %w", err)
	}

	return nil
}

// GetUserIdentity retrieves user identity by tax ID
func (r *PgUserRepository) GetUserIdentity(ctx context.Context, taxID string) (*user.UserIdentity, error) {
	query := `
		SELECT tax_id, tax_id_data, national_id, national_id_data, n_txn_id
		FROM xcentro.user_identity
		WHERE tax_id = $1
	`

	var identity user.UserIdentity
	var nationalID, taxIDData, nationalIDData, nTxnID sql.NullString

	err := r.querier.QueryRow(ctx, query, taxID).Scan(
		&identity.TaxID, &taxIDData, &nationalID, &nationalIDData, &nTxnID,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get person identity: %w", err)
	}

	identity.TaxIDData = taxIDData.String
	identity.NationalID = nationalID.String
	identity.NationalIDData = nationalIDData.String
	identity.NTxnID = nTxnID.String

	return &identity, nil
}

// GetUserKYCStatus retrieves user and their identity data for KYC status display
func (r *PgUserRepository) GetUserKYCStatus(ctx context.Context, userID string) (*user.User, *user.UserIdentity, error) {
	query := `
		SELECT
			u.id, u.email, u.mobile, u.name, u.dob, u.tax_id, u.kyc_status, u.email_verified, u.mobile_verified,
			ui.tax_id, ui.tax_id_data, ui.national_id, ui.national_id_data, ui.n_txn_id
		FROM xcentro.user_table u
		LEFT JOIN xcentro.user_identity ui ON u.tax_id = ui.tax_id
		WHERE u.id = $1
	`

	var u user.User
	var userMobile, userTaxID, userName, userDOB, piTaxID, piTaxIDData, piNationalID, piNationalIDData, piNTxnID sql.NullString
	var kycStatusStr string

	err := r.querier.QueryRow(ctx, query, userID).Scan(
		&u.Id, &u.Email, &userMobile, &userName, &userDOB, &userTaxID, &kycStatusStr, &u.EmailVerified, &u.MobileVerified,
		&piTaxID, &piTaxIDData, &piNationalID, &piNationalIDData, &piNTxnID,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, fmt.Errorf("user not found")
		}
		return nil, nil, fmt.Errorf("failed to get user KYC status: %w", err)
	}

	u.Mobile = userMobile.String
	u.TaxID = userTaxID.String
	u.Name = userName.String
	u.DOB = userDOB.String
	u.KycStatus = parseKYCStatus(kycStatusStr)

	var userIdentity *user.UserIdentity
	if piTaxID.Valid {
		userIdentity = &user.UserIdentity{
			TaxID:          piTaxID.String,
			TaxIDData:      piTaxIDData.String,
			NationalID:     piNationalID.String,
			NationalIDData: piNationalIDData.String,
			NTxnID:         piNTxnID.String,
		}
	}

	return &u, userIdentity, nil
}

// GetUserIdentityByTaxID retrieves user identity by tax ID (alias for GetUserIdentity)
func (r *PgUserRepository) GetUserIdentityByTaxID(ctx context.Context, taxID string) (*user.UserIdentity, error) {
	return r.GetUserIdentity(ctx, taxID)
}

// GetUserIdentityByTxnID retrieves user identity by Aadhaar transaction ID
func (r *PgUserRepository) GetUserIdentityByTxnID(ctx context.Context, txnID string) (*user.UserIdentity, error) {
	query := `
		SELECT tax_id, tax_id_data, national_id, national_id_data, n_txn_id
		FROM xcentro.user_identity
		WHERE n_txn_id = $1
	`

	var identity user.UserIdentity
	var nationalID, taxIDData, nationalIDData, nTxnID sql.NullString

	err := r.querier.QueryRow(ctx, query, txnID).Scan(
		&identity.TaxID, &taxIDData, &nationalID, &nationalIDData, &nTxnID,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("user identity not found for transaction ID: %s", txnID)
		}
		return nil, fmt.Errorf("failed to get user identity by txn_id: %w", err)
	}

	identity.TaxIDData = taxIDData.String
	identity.NationalID = nationalID.String
	identity.NationalIDData = nationalIDData.String
	identity.NTxnID = nTxnID.String

	return &identity, nil
}

// GetUserByTaxID retrieves user by tax ID
func (r *PgUserRepository) GetUserByTaxID(ctx context.Context, taxID string) (*user.User, error) {
	query := `
		SELECT id, email, mobile, name, dob, tax_id, kyc_status, email_verified, mobile_verified
		FROM xcentro.user_table
		WHERE tax_id = $1
	`

	var u user.User
	var userMobile, userTaxID, userName, userDOB sql.NullString
	var kycStatusStr string

	err := r.querier.QueryRow(ctx, query, taxID).Scan(
		&u.Id, &u.Email, &userMobile, &userName, &userDOB, &userTaxID, &kycStatusStr, &u.EmailVerified, &u.MobileVerified,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("user not found for tax_id: %s", taxID)
		}
		return nil, fmt.Errorf("failed to get user by tax_id: %w", err)
	}

	u.Mobile = userMobile.String
	u.TaxID = userTaxID.String
	u.Name = userName.String
	u.DOB = userDOB.String
	u.KycStatus = parseKYCStatus(kycStatusStr)

	return &u, nil
}

// CreateVerificationCode creates a new verification code
func (r *PgUserRepository) CreateVerificationCode(ctx context.Context, code *user.UserVerificationCodes) error {
	query := `
		INSERT INTO xcentro.user_verification_codes (id, user_id, code, code_type, expires_at, created_at, used, retry_attempts)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	codeID := uuid.Must(uuid.NewV7()).String()
	_, err := r.querier.Exec(ctx, query,
		codeID,
		code.UserID,
		code.Code,
		code.CodeType,
		code.ExpiresAt,
		code.CreatedAt,
		code.Used,
		code.RetryAttempts,
	)

	if err != nil {
		return fmt.Errorf("failed to create verification code: %w", err)
	}

	code.ID = codeID
	return nil
}

// VerifyVerificationCode verifies a verification code
func (r *PgUserRepository) VerifyVerificationCode(ctx context.Context, userID, code string, codeType shared.VerificationType) (bool, error) {

	// First, check if the code exists and is valid
	checkQuery := `
		SELECT id FROM xcentro.user_verification_codes
		WHERE user_id = $1 AND code = $2 AND code_type = $3 AND used = false AND expires_at > NOW()
	`

	var codeID string
	err := r.querier.QueryRow(ctx, checkQuery, userID, code, codeType).Scan(&codeID)
	if err != nil {
		return false, nil // Code not found or invalid
	}

	// Mark the code as used
	updateQuery := `UPDATE xcentro.user_verification_codes SET used = true WHERE id = $1`
	_, err = r.querier.Exec(ctx, updateQuery, codeID)
	if err != nil {
		return false, fmt.Errorf("failed to mark verification code as used: %w", err)
	}

	// Update user verification status
	var userUpdateQuery string
	switch codeType {
	case shared.EmailVerification:
		userUpdateQuery = `UPDATE xcentro.user_table SET email_verified = true WHERE id = $1`
	case shared.MobileVerification:
		userUpdateQuery = `UPDATE xcentro.user_table SET mobile_verified = true WHERE id = $1`
	}

	if userUpdateQuery != "" {
		_, err = r.querier.Exec(ctx, userUpdateQuery, userID)
		if err != nil {
			return false, fmt.Errorf("failed to update user verification status: %w", err)
		}
	}

	return true, nil
}

// CheckUserEmailVerified checks if user's email is verified
func (r *PgUserRepository) CheckUserEmailVerified(ctx context.Context, userID string) (bool, error) {
	query := `SELECT email_verified FROM xcentro.user_table WHERE id = $1`

	var verified bool
	err := r.querier.QueryRow(ctx, query, userID).Scan(&verified)
	if err != nil {
		return false, fmt.Errorf("failed to check email verification: %w", err)
	}

	return verified, nil
}

// CheckUserMobileVerified checks if user's mobile is verified
func (r *PgUserRepository) CheckUserMobileVerified(ctx context.Context, userID string) (bool, error) {
	query := `SELECT mobile_verified FROM xcentro.user_table WHERE id = $1`

	var verified bool
	err := r.querier.QueryRow(ctx, query, userID).Scan(&verified)
	if err != nil {
		return false, fmt.Errorf("failed to check mobile verification: %w", err)
	}

	return verified, nil
}

// CheckUserEmailMobileVerified checks if both email and mobile are verified
func (r *PgUserRepository) CheckUserEmailMobileVerified(ctx context.Context, userID string) (bool, error) {
	query := `SELECT email_verified AND mobile_verified FROM xcentro.user_table WHERE id = $1`

	var verified bool
	err := r.querier.QueryRow(ctx, query, userID).Scan(&verified)
	if err != nil {
		return false, fmt.Errorf("failed to check email and mobile verification: %w", err)
	}

	return verified, nil
}

// UpdateAadhaarTransactionID updates the Aadhaar transaction ID for a user's identity
func (r *PgUserRepository) UpdateAadhaarTransactionID(ctx context.Context, taxID, txnID string) error {
	query := `
		UPDATE xcentro.user_identity 
		SET n_txn_id = $1, updated_at = NOW()
		WHERE tax_id = $2
	`

	result, err := r.querier.Exec(ctx, query, txnID, taxID)
	if err != nil {
		return fmt.Errorf("failed to update Aadhaar transaction ID: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("no user identity found with tax ID: %s", taxID)
	}

	return nil
}

// GetAadhaarTransactionID retrieves the stored Aadhaar transaction ID by user ID
func (r *PgUserRepository) GetAadhaarTransactionID(ctx context.Context, userID string) (string, error) {
	query := `
		SELECT ui.n_txn_id
		FROM xcentro.user_table u
		JOIN xcentro.user_identity ui ON u.tax_id = ui.tax_id
		WHERE u.id = $1 AND ui.n_txn_id IS NOT NULL AND ui.n_txn_id != ''
	`

	var txnID sql.NullString
	err := r.querier.QueryRow(ctx, query, userID).Scan(&txnID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("no Aadhaar transaction ID found for user")
		}
		return "", fmt.Errorf("failed to get Aadhaar transaction ID: %w", err)
	}

	if !txnID.Valid || txnID.String == "" {
		return "", fmt.Errorf("no Aadhaar transaction ID found for user")
	}

	return txnID.String, nil
}

// UpdateNationalIDData updates the national ID data (Aadhaar verification result)
func (r *PgUserRepository) UpdateNationalIDData(ctx context.Context, taxID, nationalIDData string) error {
	query := `
		UPDATE xcentro.user_identity 
		SET national_id_data = $1, updated_at = NOW()
		WHERE tax_id = $2
	`

	result, err := r.querier.Exec(ctx, query, nationalIDData, taxID)
	if err != nil {
		return fmt.Errorf("failed to update national ID data: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("no user identity found with tax ID: %s", taxID)
	}

	return nil
}

// GetVerificationCodeByCode retrieves verification code by hashed code and type
func (r *PgUserRepository) GetVerificationCodeByCode(ctx context.Context, hashedCode string, codeType shared.VerificationType) (*user.UserVerificationCodes, error) {
	query := `
		SELECT id, user_id, code, code_type, expires_at, created_at, used, retry_attempts
		FROM xcentro.user_verification_codes
		WHERE code = $1 AND code_type = $2 AND used = false
		ORDER BY created_at DESC
		LIMIT 1
	`

	var vc user.UserVerificationCodes

	err := r.querier.QueryRow(ctx, query, hashedCode, codeType).Scan(
		&vc.ID,
		&vc.UserID,
		&vc.Code,
		&vc.CodeType,
		&vc.ExpiresAt,
		&vc.CreatedAt,
		&vc.Used,
		&vc.RetryAttempts,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("verification code not found")
		}
		return nil, fmt.Errorf("failed to get verification code: %w", err)
	}

	return &vc, nil
}

// MarkVerificationCodeAsUsed marks a verification code as used
func (r *PgUserRepository) MarkVerificationCodeAsUsed(ctx context.Context, codeID string) error {
	query := `
		UPDATE xcentro.user_verification_codes
		SET used = true
		WHERE id = $1
	`

	result, err := r.querier.Exec(ctx, query, codeID)
	if err != nil {
		return fmt.Errorf("failed to mark verification code as used: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("verification code not found")
	}

	return nil
}

// GetVerificationCodeByUserIDAndCode retrieves a verification code for a specific user (faster lookup with userID)
func (r *PgUserRepository) GetVerificationCodeByUserIDAndCode(ctx context.Context, userID, hashedCode string, codeType shared.VerificationType) (*user.UserVerificationCodes, error) {
	query := `
		SELECT id, user_id, code, code_type, expires_at, created_at, used, retry_attempts
		FROM xcentro.user_verification_codes
		WHERE user_id = $1 AND code = $2 AND code_type = $3 AND used = false
		ORDER BY created_at DESC
		LIMIT 1
	`

	var vc user.UserVerificationCodes

	err := r.querier.QueryRow(ctx, query, userID, hashedCode, codeType).Scan(
		&vc.ID,
		&vc.UserID,
		&vc.Code,
		&vc.CodeType,
		&vc.ExpiresAt,
		&vc.CreatedAt,
		&vc.Used,
		&vc.RetryAttempts,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("verification code not found")
		}
		return nil, fmt.Errorf("failed to get verification code: %w", err)
	}

	return &vc, nil
}

// UpdateUserEmailVerification updates user's email verification status
func (r *PgUserRepository) UpdateUserEmailVerification(ctx context.Context, userID string, verified bool) error {
	query := `
		UPDATE xcentro.user_table
		SET email_verified = $1, updated_at = NOW()
		WHERE id = $2
	`

	result, err := r.querier.Exec(ctx, query, verified, userID)
	if err != nil {
		return fmt.Errorf("failed to update email verification status: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// UpdateUserMobileVerification updates user's mobile verification status
func (r *PgUserRepository) UpdateUserMobileVerification(ctx context.Context, userID string, verified bool) error {
	query := `
		UPDATE xcentro.user_table
		SET mobile_verified = $1, updated_at = NOW()
		WHERE id = $2
	`

	result, err := r.querier.Exec(ctx, query, verified, userID)
	if err != nil {
		return fmt.Errorf("failed to update mobile verification status: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// InvalidatePreviousVerificationCodes marks all unused verification codes of a specific type as used
// This ensures only the latest code is valid (security best practice)
func (r *PgUserRepository) InvalidatePreviousVerificationCodes(ctx context.Context, userID string, codeType shared.VerificationType) error {
	query := `
		UPDATE xcentro.user_verification_codes
		SET used = TRUE
		WHERE user_id = $1 AND code_type = $2 AND used = FALSE AND expires_at > NOW()
	`

	_, err := r.querier.Exec(ctx, query, userID, codeType)
	if err != nil {
		return fmt.Errorf("failed to invalidate previous verification codes: %w", err)
	}

	return nil
}

// IncrementResendAttempts increments the retry_attempts counter for the latest verification code
// Used for analytics and fraud detection
func (r *PgUserRepository) IncrementResendAttempts(ctx context.Context, userID string, codeType shared.VerificationType) error {
	query := `
		UPDATE xcentro.user_verification_codes
		SET retry_attempts = retry_attempts + 1
		WHERE id = (
			SELECT id FROM xcentro.user_verification_codes
			WHERE user_id = $1 AND code_type = $2 AND used = FALSE
			ORDER BY created_at DESC
			LIMIT 1
		)
	`

	result, err := r.querier.Exec(ctx, query, userID, codeType)
	if err != nil {
		return fmt.Errorf("failed to increment resend attempts: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("no active verification code found")
	}

	return nil
}

// GetResendAttemptsCount returns the total number of resend attempts in the last 24 hours
// Used for analytics dashboards and fraud pattern detection
func (r *PgUserRepository) GetResendAttemptsCount(ctx context.Context, userID string, codeType shared.VerificationType) (int, error) {
	query := `
		SELECT COALESCE(SUM(retry_attempts), 0) as total_attempts
		FROM xcentro.user_verification_codes
		WHERE user_id = $1 AND code_type = $2 AND created_at > NOW() - INTERVAL '24 hours'
	`

	var count int
	err := r.querier.QueryRow(ctx, query, userID, codeType).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get resend attempts count: %w", err)
	}

	return count, nil
}

// CreateVerificationCodeWithInvalidation atomically invalidates old codes and creates a new one
// This prevents race conditions where multiple valid codes could exist simultaneously
// Uses a transaction to ensure atomicity - either both operations succeed or both fail
func (r *PgUserRepository) CreateVerificationCodeWithInvalidation(ctx context.Context, code *user.UserVerificationCodes) error {
	// Type assert to check if querier is a DB (has BeginTx capability)
	db, isDB := r.querier.(client.DB)

	// If we're already in a transaction, just execute the operations sequentially
	if !isDB {
		// We're already in a transaction, execute directly
		if err := r.invalidateAndCreate(ctx, r.querier, code); err != nil {
			return err
		}
		return nil
	}

	// Start a new transaction
	tx, err := db.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Ensure rollback on error
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	// Execute the operations in transaction
	if err = r.invalidateAndCreate(ctx, tx, code); err != nil {
		return err
	}

	// Commit transaction
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// invalidateAndCreate is a helper that performs the actual invalidate + create operations
func (r *PgUserRepository) invalidateAndCreate(ctx context.Context, q client.Querier, code *user.UserVerificationCodes) error {
	// Step 1: Invalidate old codes
	invalidateQuery := `
		UPDATE xcentro.user_verification_codes
		SET used = TRUE
		WHERE user_id = $1 AND code_type = $2 AND used = FALSE AND expires_at > NOW()
	`

	_, err := q.Exec(ctx, invalidateQuery, code.UserID, code.CodeType)
	if err != nil {
		return fmt.Errorf("failed to invalidate previous verification codes: %w", err)
	}

	// Step 2: Create new code
	createQuery := `
		INSERT INTO xcentro.user_verification_codes (id, user_id, code, code_type, expires_at, created_at, used, retry_attempts)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	codeID := uuid.Must(uuid.NewV7()).String()
	_, err = q.Exec(ctx, createQuery,
		codeID,
		code.UserID,
		code.Code,
		code.CodeType,
		code.ExpiresAt,
		code.CreatedAt,
		code.Used,
		code.RetryAttempts,
	)

	if err != nil {
		return fmt.Errorf("failed to create verification code: %w", err)
	}

	return nil
}

// UpdateUserNameAndDOB updates user's name and date of birth
func (r *PgUserRepository) UpdateUserNameAndDOB(ctx context.Context, userID, name, dob string) error {
	query := `
		UPDATE xcentro.user_table
		SET name = $1, dob = $2, updated_at = NOW()
		WHERE id = $3
	`

	result, err := r.querier.Exec(ctx, query, name, dob, userID)
	if err != nil {
		return fmt.Errorf("failed to update user name and DOB: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// UpdateTaxIDData updates the tax ID verification data (PAN_COM + PAN_AADHAAR combined)
func (r *PgUserRepository) UpdateTaxIDData(ctx context.Context, taxID, taxIDData string) error {
	query := `
		UPDATE xcentro.user_identity
		SET tax_id_data = $1, updated_at = NOW()
		WHERE tax_id = $2
	`

	result, err := r.querier.Exec(ctx, query, taxIDData, taxID)
	if err != nil {
		return fmt.Errorf("failed to update tax ID data: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("no user identity found with tax ID: %s", taxID)
	}

	return nil
}

// UpdateUserPasswordHash updates user's password hash
func (r *PgUserRepository) UpdateUserPasswordHash(ctx context.Context, userID, newPasswordHash string) error {
	query := `
		UPDATE xcentro.user_table
		SET pwd_hash = $1, updated_at = NOW()
		WHERE id = $2
	`

	result, err := r.querier.Exec(ctx, query, newPasswordHash, userID)
	if err != nil {
		return fmt.Errorf("failed to update password hash: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// CreateInvitedUser creates a new user for business invitation with all required fields
func (r *PgUserRepository) CreateInvitedUser(ctx context.Context, email, passwordHash, name, mobile string, kycNeeded bool) (*user.User, error) {
	query := `
		INSERT INTO xcentro.user_table (email, pwd_hash, name, mobile, kyc_needed, email_verified)
		VALUES ($1, $2, $3, $4, $5, true)
		RETURNING id, email, name, mobile, kyc_needed, kyc_status, email_verified, mobile_verified
	`

	var u user.User
	var kycStatusStr string

	err := r.querier.QueryRow(ctx, query,
		email,
		passwordHash,
		name,
		mobile,
		kycNeeded,
	).Scan(
		&u.Id,
		&u.Email,
		&u.Name,
		&u.Mobile,
		&u.KycNeeded,
		&kycStatusStr,
		&u.EmailVerified,
		&u.MobileVerified,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create invited user: %w", err)
	}

	u.KycStatus = parseKYCStatus(kycStatusStr)

	return &u, nil
}

// CountBusinessRelations counts the number of business relations for a user (optimized with COUNT)
func (r *PgUserRepository) CountBusinessRelations(ctx context.Context, userID string) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM xcentro.business_user_relation
		WHERE user_id = $1
	`

	var count int
	err := r.querier.QueryRow(ctx, query, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count business relations: %w", err)
	}

	return count, nil
}

// GetUserBasicInfo retrieves only id, email, and name (optimized for notifications)
func (r *PgUserRepository) GetUserBasicInfo(ctx context.Context, userID string) (id, email, name string, err error) {
	query := `
		SELECT id, email, COALESCE(name, '') as name
		FROM xcentro.user_table
		WHERE id = $1
	`

	err = r.querier.QueryRow(ctx, query, userID).Scan(&id, &email, &name)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to get user basic info: %w", err)
	}

	return id, email, name, nil
}

// SearchUsersByBusinesses searches users by name or email within specified businesses
func (r *PgUserRepository) SearchUsersByBusinesses(ctx context.Context, businessIDs []string, searchTerm string) ([]*user.OpsUserSearchResult, error) {
	query := `
		SELECT DISTINCT u.id, COALESCE(u.name, '') as name, bur.bid as business_id
		FROM xcentro.user_table u
		JOIN xcentro.business_user_relation bur ON u.id = bur.user_id
		WHERE bur.bid = ANY($1)
		AND (u.name ILIKE $2 OR u.email ILIKE $2)
		ORDER BY name
		LIMIT 50
	`

	searchPattern := "%" + searchTerm + "%"
	rows, err := r.querier.Query(ctx, query, businessIDs, searchPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to search users by businesses: %w", err)
	}
	defer rows.Close()

	var results []*user.OpsUserSearchResult
	for rows.Next() {
		var result user.OpsUserSearchResult
		if err := rows.Scan(&result.ID, &result.Name, &result.BusinessID); err != nil {
			return nil, fmt.Errorf("failed to scan user search result: %w", err)
		}
		results = append(results, &result)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating user search results: %w", err)
	}

	return results, nil
}

// GetUserDetailsForOps retrieves detailed user information with identity data for ops dashboard
// If businessID is provided, also fetches role information for that business
func (r *PgUserRepository) GetUserDetailsForOps(ctx context.Context, userID string, businessID string) (*user.OpsUserDetails, error) {
	var query string
	var args []interface{}

	if businessID != "" {
		// Query with business role data
		query = `
			SELECT
				u.id, u.email, COALESCE(u.mobile, '') as mobile, COALESCE(u.name, '') as name,
				COALESCE(u.dob::text, '') as dob, COALESCE(u.tax_id, '') as tax_id,
				u.kyc_status, u.kyc_needed, u.email_verified, u.mobile_verified,
				u.created_at::text, COALESCE(u.updated_at::text, '') as updated_at,
				COALESCE(ui.tax_id_data::text, '') as tax_id_data, COALESCE(ui.national_id, '') as national_id,
				COALESCE(ui.national_id_data::text, '') as national_id_data, COALESCE(ui.n_txn_id, '') as n_txn_id,
				bur.role, bur.role_claim
			FROM xcentro.user_table u
			LEFT JOIN xcentro.user_identity ui ON u.tax_id = ui.tax_id
			LEFT JOIN xcentro.business_user_relation bur ON u.id = bur.user_id AND bur.bid = $2
			WHERE u.id = $1
		`
		args = []interface{}{userID, businessID}
	} else {
		// Query without business role data
		query = `
			SELECT
				u.id, u.email, COALESCE(u.mobile, '') as mobile, COALESCE(u.name, '') as name,
				COALESCE(u.dob::text, '') as dob, COALESCE(u.tax_id, '') as tax_id,
				u.kyc_status, u.kyc_needed, u.email_verified, u.mobile_verified,
				u.created_at::text, COALESCE(u.updated_at::text, '') as updated_at,
				COALESCE(ui.tax_id_data::text, '') as tax_id_data, COALESCE(ui.national_id, '') as national_id,
				COALESCE(ui.national_id_data::text, '') as national_id_data, COALESCE(ui.n_txn_id, '') as n_txn_id
			FROM xcentro.user_table u
			LEFT JOIN xcentro.user_identity ui ON u.tax_id = ui.tax_id
			WHERE u.id = $1
		`
		args = []interface{}{userID}
	}

	var details user.OpsUserDetails
	var kycStatusStr string
	var roleStr, roleClaimStr sql.NullString

	if businessID != "" {
		err := r.querier.QueryRow(ctx, query, args...).Scan(
			&details.ID, &details.Email, &details.Mobile, &details.Name,
			&details.DOB, &details.TaxID, &kycStatusStr, &details.KycNeeded,
			&details.EmailVerified, &details.MobileVerified,
			&details.CreatedAt, &details.UpdatedAt,
			&details.TaxIDData, &details.NationalID, &details.NationalIDData, &details.NTxnID,
			&roleStr, &roleClaimStr,
		)

		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, fmt.Errorf("user not found")
			}
			return nil, fmt.Errorf("failed to get user details: %w", err)
		}

		// Parse role and role_claim if present
		if roleStr.Valid && roleStr.String != "" {
			role := parseUserRole(roleStr.String)
			details.Role = &role
		}
		if roleClaimStr.Valid && roleClaimStr.String != "" {
			roleClaim := parseUserRole(roleClaimStr.String)
			details.RoleClaim = &roleClaim
		}
	} else {
		err := r.querier.QueryRow(ctx, query, args...).Scan(
			&details.ID, &details.Email, &details.Mobile, &details.Name,
			&details.DOB, &details.TaxID, &kycStatusStr, &details.KycNeeded,
			&details.EmailVerified, &details.MobileVerified,
			&details.CreatedAt, &details.UpdatedAt,
			&details.TaxIDData, &details.NationalID, &details.NationalIDData, &details.NTxnID,
		)

		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, fmt.Errorf("user not found")
			}
			return nil, fmt.Errorf("failed to get user details: %w", err)
		}
	}

	// Parse KYC status
	details.KycStatus = parseKYCStatus(kycStatusStr)

	return &details, nil
}

// parseUserRole converts string to UserRole
func parseUserRole(role string) shared.UserRole {
	switch role {
	case "owner":
		return shared.Owner
	case "admin":
		return shared.Admin
	case "manager":
		return shared.Manager
	case "auditor":
		return shared.Auditor
	case "none":
		return shared.None
	default:
		return shared.None
	}
}

// UpdateTaxIDDataPartial updates a specific field in the tax_id_data JSON
func (r *PgUserRepository) UpdateTaxIDDataPartial(ctx context.Context, taxID string, field string, value string) error {
	query := `
		UPDATE xcentro.user_identity
		SET tax_id_data = jsonb_set(tax_id_data, $1, $2::jsonb)
		WHERE tax_id = $3
	`
	fieldPath := fmt.Sprintf("{%s}", field)
	_, err := r.querier.Exec(ctx, query, fieldPath, value, taxID)
	if err != nil {
		return fmt.Errorf("failed to update tax ID data field: %w", err)
	}
	return nil
}

// SetKYCStatusRejected sets user KYC status to rejected
func (r *PgUserRepository) SetKYCStatusRejected(ctx context.Context, userID string) error {
	query := `UPDATE xcentro.user_table SET kyc_status = 'kyc_rejected' WHERE id = $1`
	_, err := r.querier.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to set KYC status to rejected: %w", err)
	}
	return nil
}

// ClearUserIdentityData clears all identity data fields to empty JSON
func (r *PgUserRepository) ClearUserIdentityData(ctx context.Context, taxID string) error {
	query := `
		UPDATE xcentro.user_identity
		SET tax_id_data = '{}'::jsonb,
		    national_id_data = '{}'::jsonb
		WHERE tax_id = $1
	`
	_, err := r.querier.Exec(ctx, query, taxID)
	if err != nil {
		return fmt.Errorf("failed to clear user identity data: %w", err)
	}
	return nil
}

// SetKYCStatusVerified sets user KYC status to verified
func (r *PgUserRepository) SetKYCStatusVerified(ctx context.Context, userID string) error {
	query := `UPDATE xcentro.user_table SET kyc_status = 'kyc_verified' WHERE id = $1`
	_, err := r.querier.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to set KYC status to verified: %w", err)
	}
	return nil
}
