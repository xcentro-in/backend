// This file contains TDD tests for SwitchBusiness (AUTH-04).
// GREEN state — SwitchBusiness is implemented in Plan 03-02.
// Run: go test ./internal/services/auth/... -run TestSwitchBusiness -v

package auth_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"

	authservice "xcentro.in/internal/services/auth"

	dbclient "xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/shared"
	"xcentro.in/internal/core/user"
	"xcentro.in/internal/shared/config"
)

// ---------------------------------------------------------------------------
// Test constants
// ---------------------------------------------------------------------------

const testSecretSwitch = "test-secret-key-for-switch-business"

// ---------------------------------------------------------------------------
// Test helper: newTestSwitchService builds a minimal AuthService for SwitchBusiness tests.
// ---------------------------------------------------------------------------

func newTestSwitchService(
	userRepo user.UserRepository,
	relRepo business.BusinessUserRelationRepository,
	bizSvc business.BusinessService,
) *authservice.AuthService {
	return authservice.NewAuthServiceForSwitchBusiness(
		userRepo, relRepo, bizSvc,
		&config.JWTSecretConfig{
			SecretKey:          testSecretSwitch,
			AccessTokenExpiry:  15 * time.Minute,
			RefreshTokenExpiry: 7 * 24 * time.Hour,
		},
	)
}

// ---------------------------------------------------------------------------
// Test helper: makeTestAccessToken builds a real JWT access token signed with
// testSecretSwitch. Used to pass a valid access token into SwitchBusiness.
// The token carries all claims that SwitchBusiness will re-parse.
// ---------------------------------------------------------------------------

func makeTestAccessToken(
	t *testing.T,
	secretKey, userID, sessionID, deviceType string,
	deviceVersion int,
	bid, role string,
) string {
	t.Helper()
	claims := &auth.CustomClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
		SessionID:     sessionID,
		DeviceType:    deviceType,
		DeviceVersion: deviceVersion,
		Bid:           bid,
		Role:          role,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secretKey))
	if err != nil {
		t.Fatalf("makeTestAccessToken: %v", err)
	}
	return signed
}

// ---------------------------------------------------------------------------
// Inline mocks — no mockery/gomock
// ---------------------------------------------------------------------------

// mockSwitchUserRepo tracks calls to SetActiveBid; stubs all other methods.
type mockSwitchUserRepo struct {
	user               *user.User
	setActiveBidErr    error
	setActiveBidCalled bool
	setActiveBidArg    string
}

func (m *mockSwitchUserRepo) GetUserByID(_ context.Context, _ string) (*user.User, error) {
	return m.user, nil
}

func (m *mockSwitchUserRepo) SetActiveBid(_ context.Context, _ string, bid string) error {
	m.setActiveBidCalled = true
	m.setActiveBidArg = bid
	return m.setActiveBidErr
}

// Stub all other UserRepository methods to satisfy the interface.
func (m *mockSwitchUserRepo) WithTx(_ dbclient.Tx) user.UserRepository { return m }
func (m *mockSwitchUserRepo) CreateUser(_ context.Context, _ auth.AuthPayload) (*user.User, error) {
	return nil, nil
}
func (m *mockSwitchUserRepo) GetUserByEmail(_ context.Context, _ string) (*user.User, error) {
	return nil, nil
}
func (m *mockSwitchUserRepo) UpdateUserMobile(_ context.Context, _, _ string) error                { return nil }
func (m *mockSwitchUserRepo) UpdateUserTaxID(_ context.Context, _, _ string) error                 { return nil }
func (m *mockSwitchUserRepo) UpdateUserKYCStatus(_ context.Context, _ string, _ shared.KYCStatus) error {
	return nil
}
func (m *mockSwitchUserRepo) UpsertUserIdentity(_ context.Context, _ *user.UserIdentity) error     { return nil }
func (m *mockSwitchUserRepo) GetUserIdentity(_ context.Context, _ string) (*user.UserIdentity, error) {
	return nil, nil
}
func (m *mockSwitchUserRepo) GetUserIdentityByTaxID(_ context.Context, _ string) (*user.UserIdentity, error) {
	return nil, nil
}
func (m *mockSwitchUserRepo) GetUserIdentityByTxnID(_ context.Context, _ string) (*user.UserIdentity, error) {
	return nil, nil
}
func (m *mockSwitchUserRepo) GetUserKYCStatus(_ context.Context, _ string) (*user.User, *user.UserIdentity, error) {
	return nil, nil, nil
}
func (m *mockSwitchUserRepo) GetUserByTaxID(_ context.Context, _ string) (*user.User, error) {
	return nil, nil
}
func (m *mockSwitchUserRepo) CreateVerificationCode(_ context.Context, _ *user.UserVerificationCodes) error {
	return nil
}
func (m *mockSwitchUserRepo) VerifyVerificationCode(_ context.Context, _, _ string, _ shared.VerificationType) (bool, error) {
	return false, nil
}
func (m *mockSwitchUserRepo) GetVerificationCodeByCode(_ context.Context, _ string, _ shared.VerificationType) (*user.UserVerificationCodes, error) {
	return nil, nil
}
func (m *mockSwitchUserRepo) GetVerificationCodeByUserIDAndCode(_ context.Context, _, _ string, _ shared.VerificationType) (*user.UserVerificationCodes, error) {
	return nil, nil
}
func (m *mockSwitchUserRepo) MarkVerificationCodeAsUsed(_ context.Context, _ string) error            { return nil }
func (m *mockSwitchUserRepo) UpdateUserEmailVerification(_ context.Context, _ string, _ bool) error   { return nil }
func (m *mockSwitchUserRepo) UpdateUserMobileVerification(_ context.Context, _ string, _ bool) error  { return nil }
func (m *mockSwitchUserRepo) CheckUserEmailVerified(_ context.Context, _ string) (bool, error)        { return false, nil }
func (m *mockSwitchUserRepo) CheckUserMobileVerified(_ context.Context, _ string) (bool, error)       { return false, nil }
func (m *mockSwitchUserRepo) CheckUserEmailMobileVerified(_ context.Context, _ string) (bool, error)  { return false, nil }
func (m *mockSwitchUserRepo) InvalidatePreviousVerificationCodes(_ context.Context, _ string, _ shared.VerificationType) error {
	return nil
}
func (m *mockSwitchUserRepo) IncrementResendAttempts(_ context.Context, _ string, _ shared.VerificationType) error {
	return nil
}
func (m *mockSwitchUserRepo) GetResendAttemptsCount(_ context.Context, _ string, _ shared.VerificationType) (int, error) {
	return 0, nil
}
func (m *mockSwitchUserRepo) CreateVerificationCodeWithInvalidation(_ context.Context, _ *user.UserVerificationCodes) error {
	return nil
}
func (m *mockSwitchUserRepo) UpdateAadhaarTransactionID(_ context.Context, _, _ string) error { return nil }
func (m *mockSwitchUserRepo) GetAadhaarTransactionID(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (m *mockSwitchUserRepo) UpdateNationalIDData(_ context.Context, _, _ string) error          { return nil }
func (m *mockSwitchUserRepo) UpdateUserNameAndDOB(_ context.Context, _, _, _ string) error       { return nil }
func (m *mockSwitchUserRepo) UpdateTaxIDData(_ context.Context, _, _ string) error               { return nil }
func (m *mockSwitchUserRepo) UpdateUserPasswordHash(_ context.Context, _, _ string) error        { return nil }
func (m *mockSwitchUserRepo) CreateInvitedUser(_ context.Context, _, _, _, _ string, _ bool) (*user.User, error) {
	return nil, nil
}
func (m *mockSwitchUserRepo) CountBusinessRelations(_ context.Context, _ string) (int, error) { return 0, nil }
func (m *mockSwitchUserRepo) GetUserBasicInfo(_ context.Context, _ string) (string, string, string, error) {
	return "", "", "", nil
}
func (m *mockSwitchUserRepo) GetUserMobileByID(_ context.Context, _ string) (string, error) { return "", nil }
func (m *mockSwitchUserRepo) SearchUsersByBusinesses(_ context.Context, _ []string, _ string) ([]*user.OpsUserSearchResult, error) {
	return nil, nil
}
func (m *mockSwitchUserRepo) GetUserDetailsForOps(_ context.Context, _, _ string) (*user.OpsUserDetails, error) {
	return nil, nil
}
func (m *mockSwitchUserRepo) UpdateTaxIDDataPartial(_ context.Context, _, _, _ string) error { return nil }
func (m *mockSwitchUserRepo) SetKYCStatusRejected(_ context.Context, _ string) error         { return nil }
func (m *mockSwitchUserRepo) ClearUserIdentityData(_ context.Context, _ string) error         { return nil }
func (m *mockSwitchUserRepo) SetKYCStatusVerified(_ context.Context, _ string) error          { return nil }

// ---------------------------------------------------------------------------

// mockSwitchBizRelRepo returns a single relation (or error) from FindByUserAndBusiness.
// All other methods are stubbed.
type mockSwitchBizRelRepo struct {
	rel *business.BusinessUserRelation
	err error
}

func (m *mockSwitchBizRelRepo) FindByUserAndBusiness(_ context.Context, _ string, _ string) (*business.BusinessUserRelation, error) {
	return m.rel, m.err
}

func (m *mockSwitchBizRelRepo) FindByUser(_ context.Context, _ string) ([]*business.BusinessUserRelation, error) {
	return nil, nil
}

// Stub all other BusinessUserRelationRepository methods.
func (m *mockSwitchBizRelRepo) WithTx(_ dbclient.Tx) business.BusinessUserRelationRepository {
	return m
}
func (m *mockSwitchBizRelRepo) GetKYBInitiatedStatus(_ context.Context, _ string) (*business.BusinessKYBData, *business.Business, error) {
	return nil, nil, nil
}
func (m *mockSwitchBizRelRepo) FindByBusiness(_ context.Context, _ string) ([]*business.BusinessUserRelation, error) {
	return nil, nil
}
func (m *mockSwitchBizRelRepo) GetBusinessUsersWithDetails(_ context.Context, _ string) ([]*business.BusinessUserDTO, error) {
	return nil, nil
}
func (m *mockSwitchBizRelRepo) AddUserToBusiness(_ context.Context, _ *business.BusinessUserRelation) error {
	return nil
}
func (m *mockSwitchBizRelRepo) UpdateUserRole(_ context.Context, _, _ string, _ shared.UserRole) error {
	return nil
}
func (m *mockSwitchBizRelRepo) UpdateRoleClaim(_ context.Context, _, _ string, _ shared.UserRole) error {
	return nil
}
func (m *mockSwitchBizRelRepo) UpdateKYBInitiated(_ context.Context, _, _ string, _ bool) error { return nil }
func (m *mockSwitchBizRelRepo) UpdateKycNeeded(_ context.Context, _, _ string, _ bool) error    { return nil }
func (m *mockSwitchBizRelRepo) RemoveUserFromBusiness(_ context.Context, _, _ string) error     { return nil }
func (m *mockSwitchBizRelRepo) SetOwnerRoleToNull(_ context.Context, _ string) error            { return nil }
func (m *mockSwitchBizRelRepo) SetOwnerAndKYBInitiatorRoleToNull(_ context.Context, _ string) error {
	return nil
}
func (m *mockSwitchBizRelRepo) SetNonOwnerRolesToNone(_ context.Context, _ string) error {
	return nil
}
func (m *mockSwitchBizRelRepo) ResetAllUserRelationsForRejectedKYC(_ context.Context, _ string) error {
	return nil
}
func (m *mockSwitchBizRelRepo) OpsUpdateUserRole(_ context.Context, _, _ string, _ shared.UserRole) error {
	return nil
}
func (m *mockSwitchBizRelRepo) GetBusinessOwnerUserID(_ context.Context, _ string) (string, error) {
	return "", nil
}

// ---------------------------------------------------------------------------

// mockSwitchBizService returns a single business (or error) from GetBusiness.
// All other methods are stubbed.
type mockSwitchBizService struct {
	biz *business.Business
	err error
}

func (m *mockSwitchBizService) GetBusiness(_ context.Context, _ string) (*business.Business, error) {
	return m.biz, m.err
}

func (m *mockSwitchBizService) GetBusinessOnboardingFlags(_ context.Context, _ string) (*business.BusinessOnboardingStatusDTO, error) {
	return &business.BusinessOnboardingStatusDTO{}, nil
}

// Stub all other BusinessService methods.
func (m *mockSwitchBizService) UpdateBusinessDeedDocumentKey(_ context.Context, _, _ string) error {
	return nil
}
func (m *mockSwitchBizService) GetKYBDocumentUploadURL(_ echo.Context, _ context.Context) (string, error) {
	return "", nil
}
func (m *mockSwitchBizService) ConfirmKYBDocumentUpload(_ echo.Context, _ context.Context) (string, error) {
	return "", nil
}
func (m *mockSwitchBizService) InitiateOnboarding(_ context.Context, _, _, _ string) (*business.OnboardingInitResponse, error) {
	return nil, nil
}
func (m *mockSwitchBizService) CompleteKYB(_ echo.Context, _ context.Context, _ string, _ *business.CompleteKYBRequest) (*business.CompleteKYBResponse, error) {
	return nil, nil
}
func (m *mockSwitchBizService) GetOnboardingStatus(_ echo.Context, _ context.Context, _ string) (*business.OnboardingStatusResponse, error) {
	return nil, nil
}
func (m *mockSwitchBizService) GetBusinessDetails(_ context.Context, _ string, _ shared.UserRole) (*business.BusinessDetailsResponse, error) {
	return nil, nil
}
func (m *mockSwitchBizService) GetBusinessKYBData(_ context.Context, _ string) (*business.BusinessKYBData, error) {
	return nil, nil
}
func (m *mockSwitchBizService) UpdateOnboardingStatus(_ context.Context, _, _ string, _ bool) error {
	return nil
}
func (m *mockSwitchBizService) InviteUserToBusiness(_ context.Context, _, _, _, _ string, _ shared.UserRole, _ shared.UserRole) (*business.InviteUserResponse, error) {
	return nil, nil
}
func (m *mockSwitchBizService) AddUserToBusiness(_ context.Context, _ *business.BusinessUserRelation) error {
	return nil
}
func (m *mockSwitchBizService) GetBusinessUsers(_ context.Context, _ string, _ shared.UserRole) ([]*business.BusinessUserDTO, error) {
	return nil, nil
}
func (m *mockSwitchBizService) UpdateUserRoleInBusiness(_ context.Context, _, _ string, _ shared.UserRole, _ shared.UserRole) error {
	return nil
}
func (m *mockSwitchBizService) RemoveUserFromBusiness(_ context.Context, _, _ string, _ shared.UserRole) error {
	return nil
}
func (m *mockSwitchBizService) ValidateUserInBusiness(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
func (m *mockSwitchBizService) ListBusinessesPaginated(_ context.Context, _, _ int, _ string) ([]*business.OpsBusinessListItem, int, error) {
	return nil, 0, nil
}
func (m *mockSwitchBizService) SearchBusinesses(_ context.Context, _, _ string) ([]*business.OpsBusinessSearchResult, error) {
	return nil, nil
}
func (m *mockSwitchBizService) GetBusinessDetailsForOps(_ context.Context, _ string) (*business.OpsBusinessDetails, error) {
	return nil, nil
}
func (m *mockSwitchBizService) ReloadBusinessData(_ context.Context, _, _ string) error { return nil }
func (m *mockSwitchBizService) RejectKYB(_ context.Context, _, _ string) error          { return nil }
func (m *mockSwitchBizService) VerifyKYB(_ context.Context, _, _ string) error          { return nil }
func (m *mockSwitchBizService) OpsUpdateUserRole(_ context.Context, _, _ string, _ shared.UserRole) error {
	return nil
}
func (m *mockSwitchBizService) GetOwnerMobile(_ context.Context, _ string) (string, error) {
	return "", nil
}

// ---------------------------------------------------------------------------
// Test cases — AUTH-04
// ---------------------------------------------------------------------------

// TestSwitchBusiness_ValidSwitch verifies that a valid member of a business can
// switch to it: the service returns a non-nil SwitchBusinessResponse with the
// correct bid/role/kyb_status, a non-empty access token string, and nil error.
//
// AUTH-04: valid member + valid bid → new token issued, response populated.
func TestSwitchBusiness_ValidSwitch(t *testing.T) {
	ctx := context.Background()

	const (
		userID    = "user-switch-valid-id"
		sessionID = "session-abc-123"
		newBid    = "bid-target-1"
		role      = "admin"
		kybStatus = "verified"
	)

	accessToken := makeTestAccessToken(t, testSecretSwitch, userID, sessionID, "web", 1, "bid-old", "owner")

	userRepo := &mockSwitchUserRepo{}
	relRepo := &mockSwitchBizRelRepo{
		rel: &business.BusinessUserRelation{
			UserID:    userID,
			BID:       newBid,
			Role:      shared.UserRole(role),
			KycNeeded: false,
		},
	}
	bizSvc := &mockSwitchBizService{
		biz: &business.Business{
			BID:       newBid,
			Name:      "Target Corp",
			KybStatus: shared.KYBstatus(kybStatus),
		},
	}

	svc := newTestSwitchService(userRepo, relRepo, bizSvc)
	resp, token, err := svc.SwitchBusiness(ctx, accessToken, newBid)

	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil SwitchBusinessResponse, got nil")
	}
	if token == "" {
		t.Error("expected non-empty access token string, got empty")
	}
	if resp.Bid != newBid {
		t.Errorf("resp.Bid: expected %q, got %q", newBid, resp.Bid)
	}
}

// TestSwitchBusiness_Unauthorized verifies that when FindByUserAndBusiness returns
// an error (user is not a member), SwitchBusiness returns nil response, empty token,
// and a non-nil error. This maps to 403 in the handler.
//
// AUTH-04: non-member bid → returns error (maps to 403).
func TestSwitchBusiness_Unauthorized(t *testing.T) {
	ctx := context.Background()

	const (
		userID    = "user-switch-unauth-id"
		sessionID = "session-def-456"
		newBid    = "bid-unauthorized"
	)

	accessToken := makeTestAccessToken(t, testSecretSwitch, userID, sessionID, "web", 1, "bid-current", "owner")

	userRepo := &mockSwitchUserRepo{}
	relRepo := &mockSwitchBizRelRepo{
		rel: nil,
		err: fmt.Errorf("business relation not found for user %s bid %s", userID, newBid),
	}
	bizSvc := &mockSwitchBizService{}

	svc := newTestSwitchService(userRepo, relRepo, bizSvc)
	resp, token, err := svc.SwitchBusiness(ctx, accessToken, newBid)

	if err == nil {
		t.Error("expected non-nil error for unauthorized switch, got nil")
	}
	if resp != nil {
		t.Errorf("expected nil response for unauthorized switch, got: %+v", resp)
	}
	if token != "" {
		t.Errorf("expected empty token for unauthorized switch, got: %q", token)
	}
}

// TestSwitchBusiness_DBWriteFailure_NonFatal verifies that a SetActiveBid failure
// does NOT prevent token issuance. The service must still return a non-nil response,
// a valid token, and nil error — the DB write failure is logged but treated as non-fatal.
//
// AUTH-04: DB write failure is non-fatal — token still issued.
func TestSwitchBusiness_DBWriteFailure_NonFatal(t *testing.T) {
	ctx := context.Background()

	const (
		userID    = "user-switch-dbfail-id"
		sessionID = "session-ghi-789"
		newBid    = "bid-target-dbfail"
	)

	accessToken := makeTestAccessToken(t, testSecretSwitch, userID, sessionID, "web", 1, "bid-old", "owner")

	userRepo := &mockSwitchUserRepo{
		setActiveBidErr: fmt.Errorf("DB connection timeout"),
	}
	relRepo := &mockSwitchBizRelRepo{
		rel: &business.BusinessUserRelation{
			UserID:    userID,
			BID:       newBid,
			Role:      shared.UserRole("finance"),
			KycNeeded: true,
		},
	}
	bizSvc := &mockSwitchBizService{
		biz: &business.Business{
			BID:       newBid,
			Name:      "DB Fail Corp",
			KybStatus: shared.KYBstatus("pending"),
		},
	}

	svc := newTestSwitchService(userRepo, relRepo, bizSvc)
	resp, token, err := svc.SwitchBusiness(ctx, accessToken, newBid)

	if err != nil {
		t.Fatalf("expected nil error despite DB write failure, got: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response despite DB write failure, got nil")
	}
	if token == "" {
		t.Error("expected non-empty token despite DB write failure, got empty")
	}
}

// TestSwitchBusiness_PersistsActiveBid verifies that on a successful switch,
// SetActiveBid is called with the correct userID and newBid.
//
// AUTH-04: user_table.active_bid updated in DB on success.
func TestSwitchBusiness_PersistsActiveBid(t *testing.T) {
	ctx := context.Background()

	const (
		userID    = "user-switch-persist-id"
		sessionID = "session-jkl-000"
		newBid    = "bid-to-persist"
	)

	accessToken := makeTestAccessToken(t, testSecretSwitch, userID, sessionID, "mobile", 2, "bid-old", "owner")

	userRepo := &mockSwitchUserRepo{}
	relRepo := &mockSwitchBizRelRepo{
		rel: &business.BusinessUserRelation{
			UserID: userID,
			BID:    newBid,
			Role:   shared.UserRole("owner"),
		},
	}
	bizSvc := &mockSwitchBizService{
		biz: &business.Business{
			BID:       newBid,
			Name:      "Persist Corp",
			KybStatus: shared.KYBstatus("verified"),
		},
	}

	svc := newTestSwitchService(userRepo, relRepo, bizSvc)
	_, _, err := svc.SwitchBusiness(ctx, accessToken, newBid)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !userRepo.setActiveBidCalled {
		t.Error("expected SetActiveBid to be called, but it was not")
	}
	if userRepo.setActiveBidArg != newBid {
		t.Errorf("SetActiveBid called with bid %q, expected %q", userRepo.setActiveBidArg, newBid)
	}
}

// TestSwitchBusiness_ResponseShape verifies that the response fields
// bid, role, kyb_status, and kyc_needed correctly reflect the mocked business relation data.
//
// AUTH-04: response body {bid, role, kyb_status, kyc_needed} matches mocked data.
func TestSwitchBusiness_ResponseShape(t *testing.T) {
	ctx := context.Background()

	const (
		userID     = "user-switch-shape-id"
		sessionID  = "session-mno-111"
		newBid     = "bid-shape-target"
		expectRole = "finance"
		expectKyb  = "pending"
	)
	expectKycNeeded := true

	accessToken := makeTestAccessToken(t, testSecretSwitch, userID, sessionID, "web", 1, "bid-prev", "admin")

	userRepo := &mockSwitchUserRepo{}
	relRepo := &mockSwitchBizRelRepo{
		rel: &business.BusinessUserRelation{
			UserID:    userID,
			BID:       newBid,
			Role:      shared.UserRole(expectRole),
			KycNeeded: expectKycNeeded,
		},
	}
	bizSvc := &mockSwitchBizService{
		biz: &business.Business{
			BID:       newBid,
			Name:      "Shape Corp",
			KybStatus: shared.KYBstatus(expectKyb),
		},
	}

	svc := newTestSwitchService(userRepo, relRepo, bizSvc)
	resp, _, err := svc.SwitchBusiness(ctx, accessToken, newBid)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response, got nil")
	}

	if resp.Bid != newBid {
		t.Errorf("resp.Bid: expected %q, got %q", newBid, resp.Bid)
	}
	if resp.Role != expectRole {
		t.Errorf("resp.Role: expected %q, got %q", expectRole, resp.Role)
	}
	if resp.KybStatus != expectKyb {
		t.Errorf("resp.KybStatus: expected %q, got %q", expectKyb, resp.KybStatus)
	}
	if resp.KycNeeded != expectKycNeeded {
		t.Errorf("resp.KycNeeded: expected %v, got %v", expectKycNeeded, resp.KycNeeded)
	}
}

// TestSwitchBusiness_TokenClaims parses the returned access token and verifies
// that the Bid and Role claims in the new token match the switched business context.
//
// AUTH-04: new access token carries updated bid/role JWT claims.
func TestSwitchBusiness_TokenClaims(t *testing.T) {
	ctx := context.Background()

	const (
		userID    = "user-switch-claims-id"
		sessionID = "session-pqr-222"
		newBid    = "bid-claims-target"
		newRole   = "admin"
	)

	accessToken := makeTestAccessToken(t, testSecretSwitch, userID, sessionID, "web", 1, "bid-old", "owner")

	userRepo := &mockSwitchUserRepo{}
	relRepo := &mockSwitchBizRelRepo{
		rel: &business.BusinessUserRelation{
			UserID: userID,
			BID:    newBid,
			Role:   shared.UserRole(newRole),
		},
	}
	bizSvc := &mockSwitchBizService{
		biz: &business.Business{
			BID:       newBid,
			Name:      "Claims Corp",
			KybStatus: shared.KYBstatus("verified"),
		},
	}

	svc := newTestSwitchService(userRepo, relRepo, bizSvc)
	_, newToken, err := svc.SwitchBusiness(ctx, accessToken, newBid)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if newToken == "" {
		t.Fatal("expected non-empty token, got empty string")
	}

	// Parse the issued token and assert bid/role claims match the switched business.
	parsed, parseErr := jwt.ParseWithClaims(newToken, &auth.CustomClaims{}, func(_ *jwt.Token) (any, error) {
		return []byte(testSecretSwitch), nil
	})
	if parseErr != nil {
		t.Fatalf("failed to parse returned token: %v", parseErr)
	}

	claims, ok := parsed.Claims.(*auth.CustomClaims)
	if !ok || !parsed.Valid {
		t.Fatal("returned token has invalid claims")
	}

	if claims.Bid != newBid {
		t.Errorf("token claims.Bid: expected %q, got %q", newBid, claims.Bid)
	}
	if claims.Role != newRole {
		t.Errorf("token claims.Role: expected %q, got %q", newRole, claims.Role)
	}
	if claims.Subject != userID {
		t.Errorf("token claims.Subject: expected %q, got %q", userID, claims.Subject)
	}
}
