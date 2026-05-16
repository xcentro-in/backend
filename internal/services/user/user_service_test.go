// TDD tests for AUTH-09: RevokeAllUserTokens wiring in user service.
//
// RED state — TestRejectKYC_RevokesTokens FAILS because UserServiceImpl.RejectKYC
// does NOT yet call authService.RevokeAllUserTokens.
//
// Plan 05-07 turns it GREEN by adding the revocation call after the transaction.
//
// Approach: export_test.go exposes NewUserServiceWithAuth constructor.
// authService field was scaffolded on UserServiceImpl in Plan 05-01.
//
// Run: go test ./internal/services/user/... -run TestRejectKYC_RevokesTokens -v

package user_test

import (
	"context"
	"testing"

	dbclient "xcentro.in/internal/adapters/database/postgres/client"
	coreAuth "xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/shared"
	"xcentro.in/internal/core/user"
	usersvc "xcentro.in/internal/services/user"

	redisClient "xcentro.in/internal/adapters/cache/redis/client"
)

// ---------------------------------------------------------------------------
// mockAuthService: tracks RevokeAllUserTokens calls
// ---------------------------------------------------------------------------

type mockAuthService struct {
	revokeCalled        bool
	revokeCalledForUser string
}

func (m *mockAuthService) RevokeAllUserTokens(ctx context.Context, userID string) error {
	m.revokeCalled = true
	m.revokeCalledForUser = userID
	return nil
}

// --- unused interface methods (satisfy auth.AuthService) ---

func (m *mockAuthService) CreateTokenPair(_ context.Context, _, _, _, _ string) (string, string, error) {
	return "", "", nil
}
func (m *mockAuthService) RefreshAccessToken(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (m *mockAuthService) RevokeRefreshToken(_ context.Context, _, _ string) error { return nil }
func (m *mockAuthService) ValidateAccessToken(_ string) (*coreAuth.CustomClaims, error) {
	return nil, nil
}
func (m *mockAuthService) ValidateAccessTokenWithVersion(_ context.Context, _ string) (*coreAuth.CustomClaims, error) {
	return nil, nil
}
func (m *mockAuthService) HashPassword(_ string) (string, error)        { return "", nil }
func (m *mockAuthService) ComparePassword(_, _ string) error            { return nil }
func (m *mockAuthService) Signup(_ context.Context, _, _ string) (*coreAuth.SignupResponse, error) {
	return nil, nil
}
func (m *mockAuthService) Login(_ context.Context, _, _, _, _ string) (*coreAuth.UnifiedLoginResponse, error) {
	return nil, nil
}
func (m *mockAuthService) BuildUnifiedLoginResponse(_ context.Context, _ string) (*coreAuth.UnifiedLoginResponse, error) {
	return nil, nil
}
func (m *mockAuthService) SwitchBusiness(_ context.Context, _, _ string) (*coreAuth.SwitchBusinessResponse, string, error) {
	return nil, "", nil
}
func (m *mockAuthService) SendPasswordResetEmail(_ context.Context, _, _ string) error { return nil }
func (m *mockAuthService) ResendInviteEmail(_ context.Context, _ string) error         { return nil }
func (m *mockAuthService) ResetPasswordWithToken(_ context.Context, _, _, _, _ string) error {
	return nil
}
func (m *mockAuthService) ChangePassword(_ context.Context, _, _, _, _ string) error { return nil }
func (m *mockAuthService) SendEmailVerification(_ context.Context, _ string) error   { return nil }
func (m *mockAuthService) ResendEmailVerification(_ context.Context, _ string) error { return nil }
func (m *mockAuthService) SendMobileVerificationOTP(_ context.Context, _, _ string) error {
	return nil
}
func (m *mockAuthService) GenerateMobileOTP(_ context.Context, _ string) error          { return nil }
func (m *mockAuthService) SendInviteOTP(_ context.Context, _, _, _ string) error        { return nil }
func (m *mockAuthService) VerifyEmailWithMobile(_ context.Context, _, _, _, _ string) error {
	return nil
}
func (m *mockAuthService) VerifyMobileOTP(_ context.Context, _, _ string) error { return nil }
func (m *mockAuthService) ResetPasswordWithMobileOTP(_ context.Context, _, _, _, _, _ string) error {
	return nil
}
func (m *mockAuthService) VerifyAndUpdateMobile(_ context.Context, _, _, _ string) error {
	return nil
}
func (m *mockAuthService) ResendMobileOTP(_ context.Context, _ string) error          { return nil }
func (m *mockAuthService) ResendInviteOTP(_ context.Context, _, _, _ string) error    { return nil }
func (m *mockAuthService) GetRedisClient() *redisClient.RedisClient                   { return nil }
func (m *mockAuthService) VerifySession(_ context.Context, _ string) (*coreAuth.VerifySessionResponse, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// mockTxManager: executes fn inline (no real DB transaction)
// ---------------------------------------------------------------------------

type mockTxManager struct{}

func (m *mockTxManager) WithTx(ctx context.Context, fn func(context.Context, dbclient.Tx) error) error {
	return fn(ctx, nil)
}

// ---------------------------------------------------------------------------
// mockUserRepoForRejectKYC: returns a user with TaxID set so the full
// RejectKYC path (including ClearUserIdentityData) executes.
// ---------------------------------------------------------------------------

type mockUserRepoForRejectKYC struct {
	targetUserID string
}

func (m *mockUserRepoForRejectKYC) WithTx(_ dbclient.Tx) user.UserRepository { return m }

func (m *mockUserRepoForRejectKYC) GetUserByID(_ context.Context, _ string) (*user.User, error) {
	return &user.User{
		Id:    m.targetUserID,
		Email: "test@example.com",
		TaxID: "ABCDE1234F", // non-empty TaxID ensures ClearUserIdentityData path runs
	}, nil
}

func (m *mockUserRepoForRejectKYC) SetKYCStatusRejected(_ context.Context, _ string) error  { return nil }
func (m *mockUserRepoForRejectKYC) ClearUserIdentityData(_ context.Context, _ string) error { return nil }

// --- remaining stubs (compile-time satisfaction of user.UserRepository) ---

func (m *mockUserRepoForRejectKYC) GetUserByEmail(_ context.Context, _ string) (*user.User, error) {
	return nil, nil
}
func (m *mockUserRepoForRejectKYC) CreateUser(_ context.Context, _ coreAuth.AuthPayload) (*user.User, error) {
	return nil, nil
}
func (m *mockUserRepoForRejectKYC) SetActiveBid(_ context.Context, _, _ string) error { return nil }
func (m *mockUserRepoForRejectKYC) CreateInvitedUser(_ context.Context, _, _, _, _ string, _ bool) (*user.User, error) {
	return nil, nil
}
func (m *mockUserRepoForRejectKYC) UpdateUserMobile(_ context.Context, _, _ string) error { return nil }
func (m *mockUserRepoForRejectKYC) UpdateUserKYCStatus(_ context.Context, _ string, _ shared.KYCStatus) error {
	return nil
}
func (m *mockUserRepoForRejectKYC) UpsertUserIdentity(_ context.Context, _ *user.UserIdentity) error {
	return nil
}
func (m *mockUserRepoForRejectKYC) UpdateUserNameAndDOB(_ context.Context, _, _, _ string) error {
	return nil
}
func (m *mockUserRepoForRejectKYC) UpdateUserTaxID(_ context.Context, _, _ string) error { return nil }
func (m *mockUserRepoForRejectKYC) UpdateAadhaarTransactionID(_ context.Context, _, _ string) error {
	return nil
}
func (m *mockUserRepoForRejectKYC) GetAadhaarTransactionID(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (m *mockUserRepoForRejectKYC) GetUserKYCStatus(_ context.Context, _ string) (*user.User, *user.UserIdentity, error) {
	return nil, nil, nil
}
func (m *mockUserRepoForRejectKYC) GetUserIdentity(_ context.Context, _ string) (*user.UserIdentity, error) {
	return nil, nil
}
func (m *mockUserRepoForRejectKYC) GetUserIdentityByTaxID(_ context.Context, _ string) (*user.UserIdentity, error) {
	return nil, nil
}
func (m *mockUserRepoForRejectKYC) GetUserIdentityByTxnID(_ context.Context, _ string) (*user.UserIdentity, error) {
	return nil, nil
}
func (m *mockUserRepoForRejectKYC) GetUserByTaxID(_ context.Context, _ string) (*user.User, error) {
	return nil, nil
}
func (m *mockUserRepoForRejectKYC) UpdateNationalIDData(_ context.Context, _, _ string) error {
	return nil
}
func (m *mockUserRepoForRejectKYC) UpdateTaxIDDataPartial(_ context.Context, _, _, _ string) error {
	return nil
}
func (m *mockUserRepoForRejectKYC) SetKYCStatusVerified(_ context.Context, _ string) error { return nil }
func (m *mockUserRepoForRejectKYC) SearchUsersByBusinesses(_ context.Context, _ []string, _ string) ([]*user.OpsUserSearchResult, error) {
	return nil, nil
}
func (m *mockUserRepoForRejectKYC) GetUserDetailsForOps(_ context.Context, _, _ string) (*user.OpsUserDetails, error) {
	return nil, nil
}
func (m *mockUserRepoForRejectKYC) CreateVerificationCode(_ context.Context, _ *user.UserVerificationCodes) error {
	return nil
}
func (m *mockUserRepoForRejectKYC) VerifyVerificationCode(_ context.Context, _, _ string, _ shared.VerificationType) (bool, error) {
	return false, nil
}
func (m *mockUserRepoForRejectKYC) GetVerificationCodeByCode(_ context.Context, _ string, _ shared.VerificationType) (*user.UserVerificationCodes, error) {
	return nil, nil
}
func (m *mockUserRepoForRejectKYC) GetVerificationCodeByUserIDAndCode(_ context.Context, _, _ string, _ shared.VerificationType) (*user.UserVerificationCodes, error) {
	return nil, nil
}
func (m *mockUserRepoForRejectKYC) MarkVerificationCodeAsUsed(_ context.Context, _ string) error {
	return nil
}
func (m *mockUserRepoForRejectKYC) UpdateUserEmailVerification(_ context.Context, _ string, _ bool) error {
	return nil
}
func (m *mockUserRepoForRejectKYC) UpdateUserMobileVerification(_ context.Context, _ string, _ bool) error {
	return nil
}
func (m *mockUserRepoForRejectKYC) CheckUserEmailVerified(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (m *mockUserRepoForRejectKYC) CheckUserMobileVerified(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (m *mockUserRepoForRejectKYC) CheckUserEmailMobileVerified(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (m *mockUserRepoForRejectKYC) InvalidatePreviousVerificationCodes(_ context.Context, _ string, _ shared.VerificationType) error {
	return nil
}
func (m *mockUserRepoForRejectKYC) IncrementResendAttempts(_ context.Context, _ string, _ shared.VerificationType) error {
	return nil
}
func (m *mockUserRepoForRejectKYC) GetResendAttemptsCount(_ context.Context, _ string, _ shared.VerificationType) (int, error) {
	return 0, nil
}
func (m *mockUserRepoForRejectKYC) CreateVerificationCodeWithInvalidation(_ context.Context, _ *user.UserVerificationCodes) error {
	return nil
}
func (m *mockUserRepoForRejectKYC) UpdateUserPasswordHash(_ context.Context, _, _ string) error {
	return nil
}
func (m *mockUserRepoForRejectKYC) CountBusinessRelations(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (m *mockUserRepoForRejectKYC) GetUserBasicInfo(_ context.Context, _ string) (string, string, string, error) {
	return "", "", "", nil
}
func (m *mockUserRepoForRejectKYC) GetUserMobileByID(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (m *mockUserRepoForRejectKYC) UpdateTaxIDData(_ context.Context, _, _ string) error { return nil }

// ---------------------------------------------------------------------------
// mockBusinessUserRelRepoForRejectKYC: stub for ResetAllUserRelationsForRejectedKYC
// ---------------------------------------------------------------------------

type mockBusinessUserRelRepoForRejectKYC struct{}

func (m *mockBusinessUserRelRepoForRejectKYC) WithTx(_ dbclient.Tx) business.BusinessUserRelationRepository {
	return m
}
func (m *mockBusinessUserRelRepoForRejectKYC) ResetAllUserRelationsForRejectedKYC(_ context.Context, _ string) error {
	return nil
}

// --- remaining stubs ---

func (m *mockBusinessUserRelRepoForRejectKYC) GetKYBInitiatedStatus(_ context.Context, _ string) (*business.BusinessKYBData, *business.Business, error) {
	return nil, nil, nil
}
func (m *mockBusinessUserRelRepoForRejectKYC) FindByUser(_ context.Context, _ string) ([]*business.BusinessUserRelation, error) {
	return nil, nil
}
func (m *mockBusinessUserRelRepoForRejectKYC) FindByBusiness(_ context.Context, _ string) ([]*business.BusinessUserRelation, error) {
	return nil, nil
}
func (m *mockBusinessUserRelRepoForRejectKYC) GetBusinessUsersWithDetails(_ context.Context, _ string) ([]*business.BusinessUserDTO, error) {
	return nil, nil
}
func (m *mockBusinessUserRelRepoForRejectKYC) AddUserToBusiness(_ context.Context, _ *business.BusinessUserRelation) error {
	return nil
}
func (m *mockBusinessUserRelRepoForRejectKYC) UpdateUserRole(_ context.Context, _, _ string, _ shared.UserRole) error {
	return nil
}
func (m *mockBusinessUserRelRepoForRejectKYC) UpdateRoleClaim(_ context.Context, _, _ string, _ shared.UserRole) error {
	return nil
}
func (m *mockBusinessUserRelRepoForRejectKYC) UpdateKYBInitiated(_ context.Context, _, _ string, _ bool) error {
	return nil
}
func (m *mockBusinessUserRelRepoForRejectKYC) UpdateKycNeeded(_ context.Context, _, _ string, _ bool) error {
	return nil
}
func (m *mockBusinessUserRelRepoForRejectKYC) RemoveUserFromBusiness(_ context.Context, _, _ string) error {
	return nil
}
func (m *mockBusinessUserRelRepoForRejectKYC) SetOwnerRoleToNull(_ context.Context, _ string) error {
	return nil
}
func (m *mockBusinessUserRelRepoForRejectKYC) SetOwnerAndKYBInitiatorRoleToNull(_ context.Context, _ string) error {
	return nil
}
func (m *mockBusinessUserRelRepoForRejectKYC) SetNonOwnerRolesToNone(_ context.Context, _ string) error {
	return nil
}
func (m *mockBusinessUserRelRepoForRejectKYC) OpsUpdateUserRole(_ context.Context, _, _ string, _ shared.UserRole) error {
	return nil
}
func (m *mockBusinessUserRelRepoForRejectKYC) FindByUserAndBusiness(_ context.Context, _, _ string) (*business.BusinessUserRelation, error) {
	return nil, nil
}
func (m *mockBusinessUserRelRepoForRejectKYC) GetBusinessOwnerUserID(_ context.Context, _ string) (string, error) {
	return "", nil
}

// ---------------------------------------------------------------------------
// Test (RED state)
// ---------------------------------------------------------------------------

const testUserID = "user-uuid-kyc-reject"

// TestRejectKYC_RevokesTokens asserts that RejectKYC calls RevokeAllUserTokens
// for the target user after successfully completing the DB transaction.
//
// RED: fails because UserServiceImpl.RejectKYC does not yet call
// authService.RevokeAllUserTokens (Plan 05-07 adds it).
func TestRejectKYC_RevokesTokens(t *testing.T) {
	mockAuth := &mockAuthService{}
	svc := usersvc.NewUserServiceWithAuth(
		&mockTxManager{},
		&mockUserRepoForRejectKYC{targetUserID: testUserID},
		&mockBusinessUserRelRepoForRejectKYC{},
		mockAuth,
	)

	err := svc.RejectKYC(context.Background(), testUserID, "rejected for test")
	if err != nil {
		t.Fatalf("RejectKYC returned unexpected error: %v", err)
	}

	if !mockAuth.revokeCalled {
		t.Errorf("expected RevokeAllUserTokens to be called for user %s, but it was not", testUserID)
	}
	if mockAuth.revokeCalledForUser != testUserID {
		t.Errorf("expected RevokeAllUserTokens called for %s, got %s", testUserID, mockAuth.revokeCalledForUser)
	}
}
