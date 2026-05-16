// TDD tests for AUTH-09: RevokeAllUserTokens wiring in business service.
//
// RED state — these tests FAIL because UpdateUserRoleInBusiness, RemoveUserFromBusiness,
// and OpsUpdateUserRole do NOT yet call authService.RevokeAllUserTokens.
//
// Plan 05-07 turns them GREEN by adding the revocation calls.
//
// Approach: export_test.go exposes NewBusinessServiceWithAuth constructor.
// authService field was scaffolded on BusinessServiceImpl in Plan 05-01.
//
// Run: go test ./internal/services/business/... -run TestUpdateUserRoleInBusiness_RevokesTokens -v
//      go test ./internal/services/business/... -run TestRemoveUserFromBusiness_RevokesTokens -v
//      go test ./internal/services/business/... -run TestOpsUpdateUserRole_RevokesTokens -v

package business_test

import (
	"context"
	"testing"

	dbclient "xcentro.in/internal/adapters/database/postgres/client"
	coreAuth "xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/shared"
	"xcentro.in/internal/core/user"
	businesssvc "xcentro.in/internal/services/business"

	redis "github.com/redis/go-redis/v9"
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
func (m *mockAuthService) GenerateMobileOTP(_ context.Context, _ string) error { return nil }
func (m *mockAuthService) SendInviteOTP(_ context.Context, _, _, _ string) error { return nil }
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
func (m *mockAuthService) ResendMobileOTP(_ context.Context, _ string) error { return nil }
func (m *mockAuthService) ResendInviteOTP(_ context.Context, _, _, _ string) error { return nil }
func (m *mockAuthService) GetRedisClient() *redisClient.RedisClient              { return nil }
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
// mockBusinessUserRelRepo: minimal stub for UpdateUserRoleInBusiness and
// RemoveUserFromBusiness paths.
// ---------------------------------------------------------------------------

type mockBusinessUserRelRepo struct {
	findByBusinessResult []*business.BusinessUserRelation
}

func (m *mockBusinessUserRelRepo) WithTx(_ dbclient.Tx) business.BusinessUserRelationRepository {
	return m
}
func (m *mockBusinessUserRelRepo) FindByBusiness(_ context.Context, _ string) ([]*business.BusinessUserRelation, error) {
	return m.findByBusinessResult, nil
}
func (m *mockBusinessUserRelRepo) UpdateUserRole(_ context.Context, _, _ string, _ shared.UserRole) error {
	return nil
}
func (m *mockBusinessUserRelRepo) RemoveUserFromBusiness(_ context.Context, _, _ string) error {
	return nil
}
func (m *mockBusinessUserRelRepo) OpsUpdateUserRole(_ context.Context, _, _ string, _ shared.UserRole) error {
	return nil
}

// --- remaining interface stubs ---

func (m *mockBusinessUserRelRepo) GetKYBInitiatedStatus(_ context.Context, _ string) (*business.BusinessKYBData, *business.Business, error) {
	return nil, nil, nil
}
func (m *mockBusinessUserRelRepo) FindByUser(_ context.Context, _ string) ([]*business.BusinessUserRelation, error) {
	return nil, nil
}
func (m *mockBusinessUserRelRepo) GetBusinessUsersWithDetails(_ context.Context, _ string) ([]*business.BusinessUserDTO, error) {
	return nil, nil
}
func (m *mockBusinessUserRelRepo) AddUserToBusiness(_ context.Context, _ *business.BusinessUserRelation) error {
	return nil
}
func (m *mockBusinessUserRelRepo) UpdateRoleClaim(_ context.Context, _, _ string, _ shared.UserRole) error {
	return nil
}
func (m *mockBusinessUserRelRepo) UpdateKYBInitiated(_ context.Context, _, _ string, _ bool) error {
	return nil
}
func (m *mockBusinessUserRelRepo) UpdateKycNeeded(_ context.Context, _, _ string, _ bool) error {
	return nil
}
func (m *mockBusinessUserRelRepo) SetOwnerRoleToNull(_ context.Context, _ string) error { return nil }
func (m *mockBusinessUserRelRepo) SetOwnerAndKYBInitiatorRoleToNull(_ context.Context, _ string) error {
	return nil
}
func (m *mockBusinessUserRelRepo) SetNonOwnerRolesToNone(_ context.Context, _ string) error {
	return nil
}
func (m *mockBusinessUserRelRepo) ResetAllUserRelationsForRejectedKYC(_ context.Context, _ string) error {
	return nil
}
func (m *mockBusinessUserRelRepo) FindByUserAndBusiness(_ context.Context, _, _ string) (*business.BusinessUserRelation, error) {
	return nil, nil
}
func (m *mockBusinessUserRelRepo) GetBusinessOwnerUserID(_ context.Context, _ string) (string, error) {
	return "", nil
}

// ---------------------------------------------------------------------------
// mockUserRepo: minimal stub satisfying user.UserRepository for OpsUpdateUserRole
// (which calls s.userRepo.GetUserByID in a background goroutine for notification)
// ---------------------------------------------------------------------------

type mockUserRepo struct{}

func (m *mockUserRepo) WithTx(_ dbclient.Tx) user.UserRepository { return m }
func (m *mockUserRepo) GetUserByID(_ context.Context, _ string) (*user.User, error) {
	return &user.User{Id: "uid", Email: "test@example.com"}, nil
}

// --- remaining stubs (compile-time satisfaction of user.UserRepository) ---

func (m *mockUserRepo) GetUserByEmail(_ context.Context, _ string) (*user.User, error) {
	return nil, nil
}
func (m *mockUserRepo) CreateUser(_ context.Context, _ coreAuth.AuthPayload) (*user.User, error) {
	return nil, nil
}
func (m *mockUserRepo) SetActiveBid(_ context.Context, _, _ string) error { return nil }
func (m *mockUserRepo) CreateInvitedUser(_ context.Context, _, _, _, _ string, _ bool) (*user.User, error) {
	return nil, nil
}
func (m *mockUserRepo) UpdateUserMobile(_ context.Context, _, _ string) error { return nil }
func (m *mockUserRepo) UpdateUserKYCStatus(_ context.Context, _ string, _ shared.KYCStatus) error {
	return nil
}
func (m *mockUserRepo) UpsertUserIdentity(_ context.Context, _ *user.UserIdentity) error {
	return nil
}
func (m *mockUserRepo) UpdateUserNameAndDOB(_ context.Context, _, _, _ string) error { return nil }
func (m *mockUserRepo) UpdateUserTaxID(_ context.Context, _, _ string) error         { return nil }
func (m *mockUserRepo) UpdateAadhaarTransactionID(_ context.Context, _, _ string) error {
	return nil
}
func (m *mockUserRepo) GetAadhaarTransactionID(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (m *mockUserRepo) GetUserKYCStatus(_ context.Context, _ string) (*user.User, *user.UserIdentity, error) {
	return nil, nil, nil
}
func (m *mockUserRepo) GetUserIdentity(_ context.Context, _ string) (*user.UserIdentity, error) {
	return nil, nil
}
func (m *mockUserRepo) GetUserIdentityByTaxID(_ context.Context, _ string) (*user.UserIdentity, error) {
	return nil, nil
}
func (m *mockUserRepo) GetUserIdentityByTxnID(_ context.Context, _ string) (*user.UserIdentity, error) {
	return nil, nil
}
func (m *mockUserRepo) GetUserByTaxID(_ context.Context, _ string) (*user.User, error) {
	return nil, nil
}
func (m *mockUserRepo) UpdateNationalIDData(_ context.Context, _, _ string) error      { return nil }
func (m *mockUserRepo) UpdateTaxIDDataPartial(_ context.Context, _, _, _ string) error { return nil }
func (m *mockUserRepo) SetKYCStatusRejected(_ context.Context, _ string) error         { return nil }
func (m *mockUserRepo) ClearUserIdentityData(_ context.Context, _ string) error        { return nil }
func (m *mockUserRepo) SetKYCStatusVerified(_ context.Context, _ string) error         { return nil }
func (m *mockUserRepo) SearchUsersByBusinesses(_ context.Context, _ []string, _ string) ([]*user.OpsUserSearchResult, error) {
	return nil, nil
}
func (m *mockUserRepo) GetUserDetailsForOps(_ context.Context, _, _ string) (*user.OpsUserDetails, error) {
	return nil, nil
}

// verification code methods
func (m *mockUserRepo) CreateVerificationCode(_ context.Context, _ *user.UserVerificationCodes) error {
	return nil
}
func (m *mockUserRepo) VerifyVerificationCode(_ context.Context, _, _ string, _ shared.VerificationType) (bool, error) {
	return false, nil
}
func (m *mockUserRepo) GetVerificationCodeByCode(_ context.Context, _ string, _ shared.VerificationType) (*user.UserVerificationCodes, error) {
	return nil, nil
}
func (m *mockUserRepo) GetVerificationCodeByUserIDAndCode(_ context.Context, _, _ string, _ shared.VerificationType) (*user.UserVerificationCodes, error) {
	return nil, nil
}
func (m *mockUserRepo) MarkVerificationCodeAsUsed(_ context.Context, _ string) error { return nil }
func (m *mockUserRepo) UpdateUserEmailVerification(_ context.Context, _ string, _ bool) error {
	return nil
}
func (m *mockUserRepo) UpdateUserMobileVerification(_ context.Context, _ string, _ bool) error {
	return nil
}
func (m *mockUserRepo) CheckUserEmailVerified(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (m *mockUserRepo) CheckUserMobileVerified(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (m *mockUserRepo) CheckUserEmailMobileVerified(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (m *mockUserRepo) InvalidatePreviousVerificationCodes(_ context.Context, _ string, _ shared.VerificationType) error {
	return nil
}
func (m *mockUserRepo) IncrementResendAttempts(_ context.Context, _ string, _ shared.VerificationType) error {
	return nil
}
func (m *mockUserRepo) GetResendAttemptsCount(_ context.Context, _ string, _ shared.VerificationType) (int, error) {
	return 0, nil
}
func (m *mockUserRepo) CreateVerificationCodeWithInvalidation(_ context.Context, _ *user.UserVerificationCodes) error {
	return nil
}
func (m *mockUserRepo) UpdateUserPasswordHash(_ context.Context, _, _ string) error { return nil }
func (m *mockUserRepo) CountBusinessRelations(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (m *mockUserRepo) GetUserBasicInfo(_ context.Context, _ string) (string, string, string, error) {
	return "", "", "", nil
}
func (m *mockUserRepo) GetUserMobileByID(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (m *mockUserRepo) UpdateTaxIDData(_ context.Context, _, _ string) error { return nil }

// ---------------------------------------------------------------------------
// noopRedisClient is a Redis client pointed at an unreachable addr, used to
// satisfy the *redisClient.RedisClient type. The service skips cache
// invalidation when rclient==nil (added in Plan 05-01 nil-safe guard), so
// this is not needed; we pass nil directly.
// ---------------------------------------------------------------------------

// Helper: newTestRedisNoop creates a *redisClient.RedisClient that wraps a
// disconnected redis.Client. Goroutine Delete calls will fail silently.
// Not used: nil is passed instead; the nil-safe guard skips the goroutine.
func newTestRedisNoop() *redisClient.RedisClient {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	return &redisClient.RedisClient{Client: rdb}
}

// ---------------------------------------------------------------------------
// Tests (RED state)
// ---------------------------------------------------------------------------

const (
	testBID          = "business-uuid-001"
	testTargetUserID = "user-uuid-target"
)

// TestUpdateUserRoleInBusiness_RevokesTokens asserts that UpdateUserRoleInBusiness
// calls RevokeAllUserTokens for the target user after successfully updating the role.
//
// RED: fails because BusinessServiceImpl.UpdateUserRoleInBusiness does not yet
// call authService.RevokeAllUserTokens (Plan 05-07 adds it).
func TestUpdateUserRoleInBusiness_RevokesTokens(t *testing.T) {
	mockAuth := &mockAuthService{}
	relRepo := &mockBusinessUserRelRepo{
		findByBusinessResult: []*business.BusinessUserRelation{
			{UserID: testTargetUserID, BID: testBID, Role: shared.Admin},
		},
	}
	svc := businesssvc.NewBusinessServiceWithAuth(
		&mockTxManager{},
		relRepo,
		&mockUserRepo{},
		nil, // rclient — nil-safe guard skips cache goroutine
		mockAuth,
	)

	err := svc.UpdateUserRoleInBusiness(context.Background(), testBID, testTargetUserID, shared.Owner, shared.Admin)
	if err != nil {
		t.Fatalf("UpdateUserRoleInBusiness returned unexpected error: %v", err)
	}

	if !mockAuth.revokeCalled {
		t.Errorf("expected RevokeAllUserTokens to be called for user %s, but it was not", testTargetUserID)
	}
	if mockAuth.revokeCalledForUser != testTargetUserID {
		t.Errorf("expected RevokeAllUserTokens called for %s, got %s", testTargetUserID, mockAuth.revokeCalledForUser)
	}
}

// TestRemoveUserFromBusiness_RevokesTokens asserts that RemoveUserFromBusiness
// calls RevokeAllUserTokens for the removed user.
//
// RED: fails because BusinessServiceImpl.RemoveUserFromBusiness does not yet
// call authService.RevokeAllUserTokens (Plan 05-07 adds it).
func TestRemoveUserFromBusiness_RevokesTokens(t *testing.T) {
	mockAuth := &mockAuthService{}
	relRepo := &mockBusinessUserRelRepo{
		findByBusinessResult: []*business.BusinessUserRelation{
			{UserID: testTargetUserID, BID: testBID, Role: shared.Admin},
		},
	}
	svc := businesssvc.NewBusinessServiceWithAuth(
		&mockTxManager{},
		relRepo,
		&mockUserRepo{},
		nil, // rclient — nil-safe guard skips cache goroutine
		mockAuth,
	)

	err := svc.RemoveUserFromBusiness(context.Background(), testBID, testTargetUserID, shared.Owner)
	if err != nil {
		t.Fatalf("RemoveUserFromBusiness returned unexpected error: %v", err)
	}

	if !mockAuth.revokeCalled {
		t.Errorf("expected RevokeAllUserTokens to be called for user %s, but it was not", testTargetUserID)
	}
	if mockAuth.revokeCalledForUser != testTargetUserID {
		t.Errorf("expected RevokeAllUserTokens called for %s, got %s", testTargetUserID, mockAuth.revokeCalledForUser)
	}
}

// TestOpsUpdateUserRole_RevokesTokens asserts that OpsUpdateUserRole
// calls RevokeAllUserTokens for the target user after updating the role.
//
// RED: fails because BusinessServiceImpl.OpsUpdateUserRole does not yet
// call authService.RevokeAllUserTokens (Plan 05-07 adds it).
func TestOpsUpdateUserRole_RevokesTokens(t *testing.T) {
	mockAuth := &mockAuthService{}
	relRepo := &mockBusinessUserRelRepo{}
	svc := businesssvc.NewBusinessServiceWithAuth(
		&mockTxManager{},
		relRepo,
		&mockUserRepo{},
		nil, // rclient — nil-safe guard skips cache goroutine
		mockAuth,
	)

	err := svc.OpsUpdateUserRole(context.Background(), testBID, testTargetUserID, shared.Admin)
	if err != nil {
		t.Fatalf("OpsUpdateUserRole returned unexpected error: %v", err)
	}

	if !mockAuth.revokeCalled {
		t.Errorf("expected RevokeAllUserTokens to be called for user %s, but it was not", testTargetUserID)
	}
	if mockAuth.revokeCalledForUser != testTargetUserID {
		t.Errorf("expected RevokeAllUserTokens called for %s, got %s", testTargetUserID, mockAuth.revokeCalledForUser)
	}
}
