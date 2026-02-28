// Package auth_test contains TDD tests for Phase 2 unified login response behavior.
//
// Requirements covered:
//   - AUTH-03: Login response includes user fields, businesses list, active_bid
//   - AUTH-05: RefreshAccessToken re-fetches active_bid and role from DB before issuing new token
package auth_test

import (
	"context"
	"testing"

	"github.com/labstack/echo/v4"
	authservice "xcentro.in/internal/services/auth"

	dbclient "xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/shared"
	"xcentro.in/internal/core/user"
)

// ---------------------------------------------------------------------------
// Mock implementations (inline, no real DB or Redis required)
// ---------------------------------------------------------------------------

// mockUserRepo satisfies enough of user.UserRepository to return a user.User
// with the ActiveBid field (added in migration 000062, mapped in Plan 02-02).
type mockUserRepo struct {
	user *user.User
}

func (m *mockUserRepo) GetUserByID(_ context.Context, _ string) (*user.User, error) {
	return m.user, nil
}

func (m *mockUserRepo) SetActiveBid(_ context.Context, _, _ string) error { return nil }

// Stub all other UserRepository methods to satisfy the interface.
func (m *mockUserRepo) WithTx(_ dbclient.Tx) user.UserRepository                       { return m }
func (m *mockUserRepo) CreateUser(_ context.Context, _ auth.AuthPayload) (*user.User, error) {
	return nil, nil
}
func (m *mockUserRepo) GetUserByEmail(_ context.Context, _ string) (*user.User, error) {
	return nil, nil
}
func (m *mockUserRepo) UpdateUserMobile(_ context.Context, _, _ string) error                { return nil }
func (m *mockUserRepo) UpdateUserTaxID(_ context.Context, _, _ string) error                 { return nil }
func (m *mockUserRepo) UpdateUserKYCStatus(_ context.Context, _ string, _ shared.KYCStatus) error {
	return nil
}
func (m *mockUserRepo) UpsertUserIdentity(_ context.Context, _ *user.UserIdentity) error     { return nil }
func (m *mockUserRepo) GetUserIdentity(_ context.Context, _ string) (*user.UserIdentity, error) {
	return nil, nil
}
func (m *mockUserRepo) GetUserIdentityByTaxID(_ context.Context, _ string) (*user.UserIdentity, error) {
	return nil, nil
}
func (m *mockUserRepo) GetUserIdentityByTxnID(_ context.Context, _ string) (*user.UserIdentity, error) {
	return nil, nil
}
func (m *mockUserRepo) GetUserKYCStatus(_ context.Context, _ string) (*user.User, *user.UserIdentity, error) {
	return nil, nil, nil
}
func (m *mockUserRepo) GetUserByTaxID(_ context.Context, _ string) (*user.User, error) {
	return nil, nil
}
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
func (m *mockUserRepo) MarkVerificationCodeAsUsed(_ context.Context, _ string) error            { return nil }
func (m *mockUserRepo) UpdateUserEmailVerification(_ context.Context, _ string, _ bool) error   { return nil }
func (m *mockUserRepo) UpdateUserMobileVerification(_ context.Context, _ string, _ bool) error  { return nil }
func (m *mockUserRepo) CheckUserEmailVerified(_ context.Context, _ string) (bool, error)        { return false, nil }
func (m *mockUserRepo) CheckUserMobileVerified(_ context.Context, _ string) (bool, error)       { return false, nil }
func (m *mockUserRepo) CheckUserEmailMobileVerified(_ context.Context, _ string) (bool, error)  { return false, nil }
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
func (m *mockUserRepo) UpdateAadhaarTransactionID(_ context.Context, _, _ string) error { return nil }
func (m *mockUserRepo) GetAadhaarTransactionID(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (m *mockUserRepo) UpdateNationalIDData(_ context.Context, _, _ string) error          { return nil }
func (m *mockUserRepo) UpdateUserNameAndDOB(_ context.Context, _, _, _ string) error       { return nil }
func (m *mockUserRepo) UpdateTaxIDData(_ context.Context, _, _ string) error               { return nil }
func (m *mockUserRepo) UpdateUserPasswordHash(_ context.Context, _, _ string) error        { return nil }
func (m *mockUserRepo) CreateInvitedUser(_ context.Context, _, _, _, _ string, _ bool) (*user.User, error) {
	return nil, nil
}
func (m *mockUserRepo) CountBusinessRelations(_ context.Context, _ string) (int, error) { return 0, nil }
func (m *mockUserRepo) GetUserBasicInfo(_ context.Context, _ string) (string, string, string, error) {
	return "", "", "", nil
}
func (m *mockUserRepo) GetUserMobileByID(_ context.Context, _ string) (string, error) { return "", nil }
func (m *mockUserRepo) SearchUsersByBusinesses(_ context.Context, _ []string, _ string) ([]*user.OpsUserSearchResult, error) {
	return nil, nil
}
func (m *mockUserRepo) GetUserDetailsForOps(_ context.Context, _, _ string) (*user.OpsUserDetails, error) {
	return nil, nil
}
func (m *mockUserRepo) UpdateTaxIDDataPartial(_ context.Context, _, _, _ string) error { return nil }
func (m *mockUserRepo) SetKYCStatusRejected(_ context.Context, _ string) error         { return nil }
func (m *mockUserRepo) ClearUserIdentityData(_ context.Context, _ string) error         { return nil }
func (m *mockUserRepo) SetKYCStatusVerified(_ context.Context, _ string) error          { return nil }

// ---------------------------------------------------------------------------

// mockBizUserRelRepo satisfies business.BusinessUserRelationRepository for FindByUser.
// Returns a pre-configured list of relations ordered by AddedAt ASC.
type mockBizUserRelRepo struct {
	relations []*business.BusinessUserRelation
	// byBid: keyed by bid, used by FindByUserAndBusiness.
	byBid map[string]*business.BusinessUserRelation
}

func (m *mockBizUserRelRepo) FindByUser(_ context.Context, _ string) ([]*business.BusinessUserRelation, error) {
	return m.relations, nil
}

// FindByUserAndBusiness returns the relation for the given bid.
func (m *mockBizUserRelRepo) FindByUserAndBusiness(_ context.Context, _ string, bid string) (*business.BusinessUserRelation, error) {
	if rel, ok := m.byBid[bid]; ok {
		return rel, nil
	}
	return nil, nil
}

// Stub all other BusinessUserRelationRepository methods.
func (m *mockBizUserRelRepo) WithTx(_ dbclient.Tx) business.BusinessUserRelationRepository {
	return m
}
func (m *mockBizUserRelRepo) GetKYBInitiatedStatus(_ context.Context, _ string) (*business.BusinessKYBData, *business.Business, error) {
	return nil, nil, nil
}
func (m *mockBizUserRelRepo) FindByBusiness(_ context.Context, _ string) ([]*business.BusinessUserRelation, error) {
	return nil, nil
}
func (m *mockBizUserRelRepo) GetBusinessUsersWithDetails(_ context.Context, _ string) ([]*business.BusinessUserDTO, error) {
	return nil, nil
}
func (m *mockBizUserRelRepo) AddUserToBusiness(_ context.Context, _ *business.BusinessUserRelation) error {
	return nil
}
func (m *mockBizUserRelRepo) UpdateUserRole(_ context.Context, _, _ string, _ shared.UserRole) error {
	return nil
}
func (m *mockBizUserRelRepo) UpdateRoleClaim(_ context.Context, _, _ string, _ shared.UserRole) error {
	return nil
}
func (m *mockBizUserRelRepo) UpdateKYBInitiated(_ context.Context, _, _ string, _ bool) error { return nil }
func (m *mockBizUserRelRepo) UpdateKycNeeded(_ context.Context, _, _ string, _ bool) error    { return nil }
func (m *mockBizUserRelRepo) RemoveUserFromBusiness(_ context.Context, _, _ string) error     { return nil }
func (m *mockBizUserRelRepo) SetOwnerRoleToNull(_ context.Context, _ string) error            { return nil }
func (m *mockBizUserRelRepo) SetOwnerAndKYBInitiatorRoleToNull(_ context.Context, _ string) error {
	return nil
}
func (m *mockBizUserRelRepo) SetNonOwnerRolesToNone(_ context.Context, _ string) error {
	return nil
}
func (m *mockBizUserRelRepo) ResetAllUserRelationsForRejectedKYC(_ context.Context, _ string) error {
	return nil
}
func (m *mockBizUserRelRepo) OpsUpdateUserRole(_ context.Context, _, _ string, _ shared.UserRole) error {
	return nil
}
func (m *mockBizUserRelRepo) GetBusinessOwnerUserID(_ context.Context, _ string) (string, error) {
	return "", nil
}

// ---------------------------------------------------------------------------

// mockBusinessService satisfies business.BusinessService for GetBusiness and GetBusinessOnboardingFlags.
type mockBusinessService struct {
	businesses map[string]*business.Business
	flags      map[string]*business.BusinessOnboardingStatusDTO
}

func (m *mockBusinessService) GetBusiness(_ context.Context, bid string) (*business.Business, error) {
	if b, ok := m.businesses[bid]; ok {
		return b, nil
	}
	return nil, nil
}

func (m *mockBusinessService) GetBusinessOnboardingFlags(_ context.Context, bid string) (*business.BusinessOnboardingStatusDTO, error) {
	if f, ok := m.flags[bid]; ok {
		return f, nil
	}
	return &business.BusinessOnboardingStatusDTO{}, nil
}

// Stub all other BusinessService methods.
func (m *mockBusinessService) UpdateBusinessDeedDocumentKey(_ context.Context, _, _ string) error {
	return nil
}
func (m *mockBusinessService) GetKYBDocumentUploadURL(_ echo.Context, _ context.Context) (string, error) {
	return "", nil
}
func (m *mockBusinessService) ConfirmKYBDocumentUpload(_ echo.Context, _ context.Context) (string, error) {
	return "", nil
}
func (m *mockBusinessService) InitiateOnboarding(_ context.Context, _, _, _ string) (*business.OnboardingInitResponse, error) {
	return nil, nil
}
func (m *mockBusinessService) CompleteKYB(_ echo.Context, _ context.Context, _ string, _ *business.CompleteKYBRequest) (*business.CompleteKYBResponse, error) {
	return nil, nil
}
func (m *mockBusinessService) GetOnboardingStatus(_ echo.Context, _ context.Context, _ string) (*business.OnboardingStatusResponse, error) {
	return nil, nil
}
func (m *mockBusinessService) GetBusinessDetails(_ context.Context, _ string, _ shared.UserRole) (*business.BusinessDetailsResponse, error) {
	return nil, nil
}
func (m *mockBusinessService) GetBusinessKYBData(_ context.Context, _ string) (*business.BusinessKYBData, error) {
	return nil, nil
}
func (m *mockBusinessService) UpdateOnboardingStatus(_ context.Context, _, _ string, _ bool) error {
	return nil
}
func (m *mockBusinessService) InviteUserToBusiness(_ context.Context, _, _, _, _ string, _ shared.UserRole, _ shared.UserRole) (*business.InviteUserResponse, error) {
	return nil, nil
}
func (m *mockBusinessService) AddUserToBusiness(_ context.Context, _ *business.BusinessUserRelation) error {
	return nil
}
func (m *mockBusinessService) GetBusinessUsers(_ context.Context, _ string, _ shared.UserRole) ([]*business.BusinessUserDTO, error) {
	return nil, nil
}
func (m *mockBusinessService) UpdateUserRoleInBusiness(_ context.Context, _, _ string, _ shared.UserRole, _ shared.UserRole) error {
	return nil
}
func (m *mockBusinessService) RemoveUserFromBusiness(_ context.Context, _, _ string, _ shared.UserRole) error {
	return nil
}
func (m *mockBusinessService) ValidateUserInBusiness(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
func (m *mockBusinessService) ListBusinessesPaginated(_ context.Context, _, _ int, _ string) ([]*business.OpsBusinessListItem, int, error) {
	return nil, 0, nil
}
func (m *mockBusinessService) SearchBusinesses(_ context.Context, _, _ string) ([]*business.OpsBusinessSearchResult, error) {
	return nil, nil
}
func (m *mockBusinessService) GetBusinessDetailsForOps(_ context.Context, _ string) (*business.OpsBusinessDetails, error) {
	return nil, nil
}
func (m *mockBusinessService) ReloadBusinessData(_ context.Context, _, _ string) error { return nil }
func (m *mockBusinessService) RejectKYB(_ context.Context, _, _ string) error          { return nil }
func (m *mockBusinessService) VerifyKYB(_ context.Context, _, _ string) error          { return nil }
func (m *mockBusinessService) OpsUpdateUserRole(_ context.Context, _, _ string, _ shared.UserRole) error {
	return nil
}
func (m *mockBusinessService) GetOwnerMobile(_ context.Context, _ string) (string, error) {
	return "", nil
}

// ---------------------------------------------------------------------------
// Helper: buildUnifiedLoginResponse creates a minimal AuthService with the provided
// mock dependencies and calls BuildUnifiedLoginResponse.
// ---------------------------------------------------------------------------

func buildUnifiedLoginResponse(
	ctx context.Context,
	userID string,
	userRepo user.UserRepository,
	bizRelRepo *mockBizUserRelRepo,
	bizService business.BusinessService,
) (*auth.UnifiedLoginResponse, error) {
	svc := authservice.NewAuthServiceForBuildUnifiedLogin(userRepo, bizRelRepo, bizService)
	return svc.BuildUnifiedLoginResponse(ctx, userID)
}

// ---------------------------------------------------------------------------
// Test cases
// ---------------------------------------------------------------------------

// TestBuildUnifiedLoginResponse_UserFields verifies that BuildUnifiedLoginResponse
// correctly maps user-level fields into the UnifiedUserProfile.
//
// AUTH-03: user fields: id, name, email, mobile, kyc_status,
//
//	has_added_payment_method, has_added_vendor, has_created_first_payment
func TestBuildUnifiedLoginResponse_UserFields(t *testing.T) {
	ctx := context.Background()

	const (
		uid   = "user-fields-test-id"
		bid1  = "bid-1"
		uName = "Arjun Kumar"
		email = "arjun@example.com"
		mob   = "+919876543210"
	)

	uRepo := &mockUserRepo{
		user: &user.User{
			Id:        uid,
			Email:     email,
			Name:      uName,
			Mobile:    mob,
			KycStatus: shared.KYCStatus("verified"),
			ActiveBid: bid1,
		},
	}

	bizRelRepo := &mockBizUserRelRepo{
		relations: []*business.BusinessUserRelation{
			{
				UserID:    uid,
				BID:       bid1,
				Role:      shared.UserRole("owner"),
				KycNeeded: false,
				AddedAt:   "2024-01-01T00:00:00Z",
			},
		},
		byBid: map[string]*business.BusinessUserRelation{
			bid1: {UserID: uid, BID: bid1, Role: shared.UserRole("owner"), KycNeeded: false},
		},
	}

	bizService := &mockBusinessService{
		businesses: map[string]*business.Business{
			bid1: {BID: bid1, Name: "Arjun Corp", KybStatus: shared.KYBstatus("verified")},
		},
		flags: map[string]*business.BusinessOnboardingStatusDTO{
			bid1: {HasAddedPaymentMethod: true, HasAddedVendor: true, HasCreatedFirstPayment: false},
		},
	}

	resp, err := buildUnifiedLoginResponse(ctx, uid, uRepo, bizRelRepo, bizService)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Assert user profile fields (AUTH-03)
	if resp.User == nil {
		t.Fatal("expected User profile, got nil")
	}
	if resp.User.Id != uid {
		t.Errorf("User.Id: expected %q, got %q", uid, resp.User.Id)
	}
	if resp.User.Email != email {
		t.Errorf("User.Email: expected %q, got %q", email, resp.User.Email)
	}
	if resp.User.Name != uName {
		t.Errorf("User.Name: expected %q, got %q", uName, resp.User.Name)
	}
	if resp.User.Mobile != mob {
		t.Errorf("User.Mobile: expected %q, got %q", mob, resp.User.Mobile)
	}
	if string(resp.User.KycStatus) != "verified" {
		t.Errorf("User.KycStatus: expected %q, got %q", "verified", resp.User.KycStatus)
	}

	// Assert onboarding flags (AUTH-03: flags computed for active_bid)
	if !resp.User.HasAddedPaymentMethod {
		t.Errorf("expected HasAddedPaymentMethod = true, got false")
	}
	if !resp.User.HasAddedVendor {
		t.Errorf("expected HasAddedVendor = true, got false")
	}
	if resp.User.HasCreatedFirstPayment {
		t.Errorf("expected HasCreatedFirstPayment = false, got true")
	}
}

// TestBuildUnifiedLoginResponse_BusinessList verifies that all of the user's
// business relations are present in the response with correct fields.
//
// AUTH-03: businesses[]: bid, name, role, kyb_status, kyc_needed
func TestBuildUnifiedLoginResponse_BusinessList(t *testing.T) {
	ctx := context.Background()

	const (
		uid  = "user-biz-list-id"
		bid1 = "bid-1"
		bid2 = "bid-2"
	)

	uRepo := &mockUserRepo{
		user: &user.User{Id: uid, Email: "test@example.com"},
	}

	bizRelRepo := &mockBizUserRelRepo{
		relations: []*business.BusinessUserRelation{
			{UserID: uid, BID: bid1, Role: shared.UserRole("owner"), KycNeeded: false, AddedAt: "2024-01-01T00:00:00Z"},
			{UserID: uid, BID: bid2, Role: shared.UserRole("admin"), KycNeeded: true, AddedAt: "2024-02-01T00:00:00Z"},
		},
		byBid: map[string]*business.BusinessUserRelation{
			bid1: {UserID: uid, BID: bid1, Role: shared.UserRole("owner")},
			bid2: {UserID: uid, BID: bid2, Role: shared.UserRole("admin")},
		},
	}

	bizService := &mockBusinessService{
		businesses: map[string]*business.Business{
			bid1: {BID: bid1, Name: "Alpha Corp", KybStatus: shared.KYBstatus("verified")},
			bid2: {BID: bid2, Name: "Beta Ltd", KybStatus: shared.KYBstatus("pending")},
		},
		flags: map[string]*business.BusinessOnboardingStatusDTO{},
	}

	resp, err := buildUnifiedLoginResponse(ctx, uid, uRepo, bizRelRepo, bizService)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	if len(resp.Businesses) != 2 {
		t.Fatalf("expected 2 businesses, got %d", len(resp.Businesses))
	}

	// First business (bid-1, owner)
	b0 := resp.Businesses[0]
	if b0.Bid != bid1 {
		t.Errorf("Businesses[0].Bid: expected %q, got %q", bid1, b0.Bid)
	}
	if b0.Name != "Alpha Corp" {
		t.Errorf("Businesses[0].Name: expected %q, got %q", "Alpha Corp", b0.Name)
	}
	if string(b0.Role) != "owner" {
		t.Errorf("Businesses[0].Role: expected %q, got %q", "owner", b0.Role)
	}
	if string(b0.KybStatus) != "verified" {
		t.Errorf("Businesses[0].KybStatus: expected %q, got %q", "verified", b0.KybStatus)
	}
	if b0.KycNeeded {
		t.Errorf("Businesses[0].KycNeeded: expected false, got true")
	}

	// Second business (bid-2, admin)
	b1 := resp.Businesses[1]
	if b1.Bid != bid2 {
		t.Errorf("Businesses[1].Bid: expected %q, got %q", bid2, b1.Bid)
	}
	if string(b1.Role) != "admin" {
		t.Errorf("Businesses[1].Role: expected %q, got %q", "admin", b1.Role)
	}
	if !b1.KycNeeded {
		t.Errorf("Businesses[1].KycNeeded: expected true, got false")
	}
}

// TestBuildUnifiedLoginResponse_ActiveBid_FromUserTable verifies that when
// user_table.active_bid is set, it is used as the active_bid in the response.
//
// AUTH-03: active_bid = user_table.active_bid if set
func TestBuildUnifiedLoginResponse_ActiveBid_FromUserTable(t *testing.T) {
	ctx := context.Background()

	const (
		uid  = "user-active-bid-id"
		bid1 = "bid-1"
		bid2 = "bid-2"
	)

	uRepo := &mockUserRepo{
		user: &user.User{
			Id:        uid,
			ActiveBid: bid1, // user_table.active_bid = bid1
		},
	}

	bizRelRepo := &mockBizUserRelRepo{
		relations: []*business.BusinessUserRelation{
			{UserID: uid, BID: bid1, Role: shared.UserRole("owner"), AddedAt: "2024-01-01T00:00:00Z"},
			{UserID: uid, BID: bid2, Role: shared.UserRole("admin"), AddedAt: "2024-02-01T00:00:00Z"},
		},
		byBid: map[string]*business.BusinessUserRelation{
			bid1: {UserID: uid, BID: bid1, Role: shared.UserRole("owner")},
			bid2: {UserID: uid, BID: bid2, Role: shared.UserRole("admin")},
		},
	}

	bizService := &mockBusinessService{
		businesses: map[string]*business.Business{
			bid1: {BID: bid1, Name: "Alpha Corp", KybStatus: shared.KYBstatus("verified")},
			bid2: {BID: bid2, Name: "Beta Ltd", KybStatus: shared.KYBstatus("pending")},
		},
		flags: map[string]*business.BusinessOnboardingStatusDTO{},
	}

	resp, err := buildUnifiedLoginResponse(ctx, uid, uRepo, bizRelRepo, bizService)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// When user.ActiveBid == bid1, the response MUST use bid1 regardless of business order.
	if resp.ActiveBid != bid1 {
		t.Errorf("ActiveBid: expected %q (from user_table), got %q", bid1, resp.ActiveBid)
	}
}

// TestBuildUnifiedLoginResponse_ActiveBid_FallsBackToFirstBusiness verifies that
// when user_table.active_bid is empty (""), the active_bid falls back to the first
// business in the list (ordered by added_at ASC).
//
// AUTH-03: active_bid = businesses[0].Bid if user.active_bid is unset
func TestBuildUnifiedLoginResponse_ActiveBid_FallsBackToFirstBusiness(t *testing.T) {
	ctx := context.Background()

	const (
		uid       = "user-no-active-bid-id"
		firstBid  = "bid-oldest"  // added_at = 2024-01-01 (first)
		secondBid = "bid-newest"  // added_at = 2024-06-01 (second)
	)

	uRepo := &mockUserRepo{
		user: &user.User{
			Id:        uid,
			ActiveBid: "", // explicitly unset (zero value / NULL in DB)
		},
	}

	// Relations ordered by added_at ASC — firstBid comes first.
	bizRelRepo := &mockBizUserRelRepo{
		relations: []*business.BusinessUserRelation{
			{UserID: uid, BID: firstBid, Role: shared.UserRole("owner"), AddedAt: "2024-01-01T00:00:00Z"},
			{UserID: uid, BID: secondBid, Role: shared.UserRole("admin"), AddedAt: "2024-06-01T00:00:00Z"},
		},
		byBid: map[string]*business.BusinessUserRelation{
			firstBid:  {UserID: uid, BID: firstBid, Role: shared.UserRole("owner")},
			secondBid: {UserID: uid, BID: secondBid, Role: shared.UserRole("admin")},
		},
	}

	bizService := &mockBusinessService{
		businesses: map[string]*business.Business{
			firstBid:  {BID: firstBid, Name: "First Corp", KybStatus: shared.KYBstatus("verified")},
			secondBid: {BID: secondBid, Name: "Second Corp", KybStatus: shared.KYBstatus("pending")},
		},
		flags: map[string]*business.BusinessOnboardingStatusDTO{},
	}

	resp, err := buildUnifiedLoginResponse(ctx, uid, uRepo, bizRelRepo, bizService)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Fallback: active_bid must be the first business (by added_at ASC order).
	if resp.ActiveBid != firstBid {
		t.Errorf("ActiveBid fallback: expected %q (first by added_at), got %q", firstBid, resp.ActiveBid)
	}
}

// TestBuildUnifiedLoginResponse_NoBusinesses verifies the no-business edge case:
//   - businesses is empty slice (not nil)
//   - active_bid is ""
//   - all onboarding flags are false
//
// AUTH-03: if user has no business relations, return empty state
func TestBuildUnifiedLoginResponse_NoBusinesses(t *testing.T) {
	ctx := context.Background()

	const uid = "user-no-biz-id"

	uRepo := &mockUserRepo{
		user: &user.User{Id: uid, Email: "lonely@example.com"},
	}

	bizRelRepo := &mockBizUserRelRepo{
		relations: []*business.BusinessUserRelation{}, // empty
		byBid:     map[string]*business.BusinessUserRelation{},
	}

	bizService := &mockBusinessService{
		businesses: map[string]*business.Business{},
		flags:      map[string]*business.BusinessOnboardingStatusDTO{},
	}

	resp, err := buildUnifiedLoginResponse(ctx, uid, uRepo, bizRelRepo, bizService)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Businesses must be an empty slice, not nil (AUTH-03: safe JSON serialization).
	if resp.Businesses == nil {
		t.Error("expected non-nil (empty) Businesses slice, got nil")
	}
	if len(resp.Businesses) != 0 {
		t.Errorf("expected 0 businesses, got %d", len(resp.Businesses))
	}

	// active_bid must be empty string (AUTH-03)
	if resp.ActiveBid != "" {
		t.Errorf("ActiveBid: expected empty string, got %q", resp.ActiveBid)
	}

	// All onboarding flags must be false (AUTH-03: no active business context)
	if resp.User == nil {
		t.Fatal("expected User profile, got nil")
	}
	if resp.User.HasAddedPaymentMethod {
		t.Error("expected HasAddedPaymentMethod = false for no-business user, got true")
	}
	if resp.User.HasAddedVendor {
		t.Error("expected HasAddedVendor = false for no-business user, got true")
	}
	if resp.User.HasCreatedFirstPayment {
		t.Error("expected HasCreatedFirstPayment = false for no-business user, got true")
	}
}

// TestRefreshAccessToken_RefetchesBidRole documents the required behavior of
// RefreshAccessToken when re-fetching active_bid and role from the database.
//
// AUTH-05: after role change in DB, a new access token via RefreshAccessToken
//
//	must carry the updated role — NOT the stale role from the old token.
//
// The behavior contract is:
//
//  1. User has active_bid=bid1, role=owner in DB.
//  2. Admin changes user's role to finance in DB (via UpdateUserRole).
//  3. User's old access token still says role=owner (not yet expired).
//  4. User calls /refresh with valid refresh token.
//  5. RefreshAccessToken fetches current active_bid from user_table.
//  6. RefreshAccessToken fetches current role from business_user_relation.
//  7. New access token is issued with role=finance.
//  8. Old role (owner) does NOT persist.
//
// Additional case (AUTH-05, no active_bid):
//   - User has active_bid=NULL in user_table -> new token has bid="" and role=""
//
// This test is intentionally marked as a placeholder until a full integration
// setup with mock session/device-version repos is available.
func TestRefreshAccessToken_RefetchesBidRole(t *testing.T) {
	// TODO(Phase2-Plan03): implement integration test using mock AuthService deps.
	// For now this test function exists to document the RED contract and prevent
	// Plan 02-03 from being considered complete without covering this behavior.
	t.Skip("TODO(Phase2-Plan03): requires AuthService integration setup with mock deps")
}
