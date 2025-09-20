package user_repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"

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
	userID := uuid.New().String()

	query := `
		INSERT INTO users (id, email, mobile, pan, kyc_status, kyc_data, email_verified, mobile_verified, role)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, email, mobile, pan, kyc_status, kyc_data, email_verified, mobile_verified, role
	`

	var u user.User
	var kycStatusInt int
	var roleInt int

	err := r.querier.QueryRow(ctx, query,
		userID,
		authPayload.Email,
		"", // mobile - empty initially
		"", // pan - empty initially
		int(shared.KYCNotStarted),
		"",                // kyc_data - empty initially
		false,             // email_verified
		false,             // mobile_verified
		int(shared.Owner), // default role
	).Scan(
		&u.Id,
		&u.Email,
		&u.Mobile,
		&u.PAN,
		&kycStatusInt,
		&u.KycData,
		&u.EmailVerified,
		&u.MobileVerified,
		&roleInt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	u.KycStatus = shared.KYCStatus(kycStatusInt)
	u.Role = shared.UserRole(roleInt)

	return &u, nil
}

// GetUserByID retrieves a user by ID
func (r *PgUserRepository) GetUserByID(ctx context.Context, userID string) (*user.User, error) {
	query := `
		SELECT id, email, mobile, pan, kyc_status, kyc_data, email_verified, mobile_verified, role
		FROM users
		WHERE id = $1
	`

	var u user.User
	var kycStatusInt int
	var roleInt int

	err := r.querier.QueryRow(ctx, query, userID).Scan(
		&u.Id,
		&u.Email,
		&u.Mobile,
		&u.PAN,
		&kycStatusInt,
		&u.KycData,
		&u.EmailVerified,
		&u.MobileVerified,
		&roleInt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get user by ID: %w", err)
	}

	u.KycStatus = shared.KYCStatus(kycStatusInt)
	u.Role = shared.UserRole(roleInt)

	return &u, nil
}

// GetUserByEmail retrieves a user by email
func (r *PgUserRepository) GetUserByEmail(ctx context.Context, email string) (*user.User, error) {
	query := `
		SELECT id, email, mobile, pan, kyc_status, kyc_data, email_verified, mobile_verified, role
		FROM users
		WHERE email = $1
	`

	var u user.User
	var kycStatusInt int
	var roleInt int

	err := r.querier.QueryRow(ctx, query, email).Scan(
		&u.Id,
		&u.Email,
		&u.Mobile,
		&u.PAN,
		&kycStatusInt,
		&u.KycData,
		&u.EmailVerified,
		&u.MobileVerified,
		&roleInt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	u.KycStatus = shared.KYCStatus(kycStatusInt)
	u.Role = shared.UserRole(roleInt)

	return &u, nil
}

// UpdateUserMobile updates user's mobile number
func (r *PgUserRepository) UpdateUserMobile(ctx context.Context, userID, newMobile string) error {
	query := `UPDATE users SET mobile = $1 WHERE id = $2`

	_, err := r.querier.Exec(ctx, query, newMobile, userID)
	if err != nil {
		return fmt.Errorf("failed to update user mobile: %w", err)
	}

	return nil
}

// UpdateUserPAN updates user's PAN
func (r *PgUserRepository) UpdateUserPAN(ctx context.Context, userID, newPAN string) error {
	query := `UPDATE users SET pan = $1 WHERE id = $2`

	_, err := r.querier.Exec(ctx, query, newPAN, userID)
	if err != nil {
		return fmt.Errorf("failed to update user PAN: %w", err)
	}

	return nil
}

// UpdateUserKYCStatus updates user's KYC status
func (r *PgUserRepository) UpdateUserKYCStatus(ctx context.Context, userID string, newStatus shared.KYCStatus) error {
	query := `UPDATE users SET kyc_status = $1 WHERE id = $2`

	_, err := r.querier.Exec(ctx, query, int(newStatus), userID)
	if err != nil {
		return fmt.Errorf("failed to update user KYC status: %w", err)
	}

	return nil
}

// UpdateUserKYCData updates user's KYC data
func (r *PgUserRepository) UpdateUserKYCData(ctx context.Context, userID, kycData string) error {
	query := `UPDATE users SET kyc_data = $1 WHERE id = $2`

	_, err := r.querier.Exec(ctx, query, kycData, userID)
	if err != nil {
		return fmt.Errorf("failed to update user KYC data: %w", err)
	}

	return nil
}

// CreateVerificationCode creates a new verification code
func (r *PgUserRepository) CreateVerificationCode(ctx context.Context, code *user.UserVerificationCodes) error {
	query := `
		INSERT INTO user_verification_codes (id, user_id, code, code_type, expires_at, created_at, used, retry_attempts)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	codeID := uuid.New().String()
	_, err := r.querier.Exec(ctx, query,
		codeID,
		code.UserID,
		code.Code,
		int(code.CodeType),
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
		SELECT id FROM user_verification_codes
		WHERE user_id = $1 AND code = $2 AND code_type = $3 AND used = false AND expires_at > NOW()
	`

	var codeID string
	err := r.querier.QueryRow(ctx, checkQuery, userID, code, int(codeType)).Scan(&codeID)
	if err != nil {
		return false, nil // Code not found or invalid
	}

	// Mark the code as used
	updateQuery := `UPDATE user_verification_codes SET used = true WHERE id = $1`
	_, err = r.querier.Exec(ctx, updateQuery, codeID)
	if err != nil {
		return false, fmt.Errorf("failed to mark verification code as used: %w", err)
	}

	// Update user verification status
	var userUpdateQuery string
	switch codeType {
	case shared.EmailVerification:
		userUpdateQuery = `UPDATE users SET email_verified = true WHERE id = $1`
	case shared.MobileVerification:
		userUpdateQuery = `UPDATE users SET mobile_verified = true WHERE id = $1`
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
	query := `SELECT email_verified FROM users WHERE id = $1`

	var verified bool
	err := r.querier.QueryRow(ctx, query, userID).Scan(&verified)
	if err != nil {
		return false, fmt.Errorf("failed to check email verification: %w", err)
	}

	return verified, nil
}

// CheckUserMobileVerified checks if user's mobile is verified
func (r *PgUserRepository) CheckUserMobileVerified(ctx context.Context, userID string) (bool, error) {
	query := `SELECT mobile_verified FROM users WHERE id = $1`

	var verified bool
	err := r.querier.QueryRow(ctx, query, userID).Scan(&verified)
	if err != nil {
		return false, fmt.Errorf("failed to check mobile verification: %w", err)
	}

	return verified, nil
}

// CheckUserEmailMobileVerified checks if both email and mobile are verified
func (r *PgUserRepository) CheckUserEmailMobileVerified(ctx context.Context, userID string) (bool, error) {
	query := `SELECT email_verified AND mobile_verified FROM users WHERE id = $1`

	var verified bool
	err := r.querier.QueryRow(ctx, query, userID).Scan(&verified)
	if err != nil {
		return false, fmt.Errorf("failed to check email and mobile verification: %w", err)
	}

	return verified, nil
}
