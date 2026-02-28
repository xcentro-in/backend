// This file contains TDD tests for GetUserRoles behavior change (AUTH-07).
// GREEN state — GetUserRoles now calls businessUserRelRepo.FindByUser directly.
// No middleware injection required. BusinessRolesMiddleware dependency removed.
//
// Run: go test ./internal/handlers/http/user/... -run TestGetUserRoles -v
// Expected: PASS — handler returns roles from direct repo call.

package user

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	dbclient "xcentro.in/internal/adapters/database/postgres/client"
	coreBusiness "xcentro.in/internal/core/business"
	"xcentro.in/internal/core/shared"
)

// ---------------------------------------------------------------------------
// stubBizRelRepo satisfies coreBusiness.BusinessUserRelationRepository.
// Only FindByUser returns data; all other methods return zero values.
// ---------------------------------------------------------------------------

type stubBizRelRepo struct {
	relations []*coreBusiness.BusinessUserRelation
	err       error
}

func (s *stubBizRelRepo) FindByUser(_ context.Context, _ string) ([]*coreBusiness.BusinessUserRelation, error) {
	return s.relations, s.err
}

// All other methods return zero values to satisfy the interface.
func (s *stubBizRelRepo) WithTx(_ dbclient.Tx) coreBusiness.BusinessUserRelationRepository {
	return s
}
func (s *stubBizRelRepo) GetKYBInitiatedStatus(_ context.Context, _ string) (*coreBusiness.BusinessKYBData, *coreBusiness.Business, error) {
	return nil, nil, nil
}
func (s *stubBizRelRepo) FindByBusiness(_ context.Context, _ string) ([]*coreBusiness.BusinessUserRelation, error) {
	return nil, nil
}
func (s *stubBizRelRepo) GetBusinessUsersWithDetails(_ context.Context, _ string) ([]*coreBusiness.BusinessUserDTO, error) {
	return nil, nil
}
func (s *stubBizRelRepo) AddUserToBusiness(_ context.Context, _ *coreBusiness.BusinessUserRelation) error {
	return nil
}
func (s *stubBizRelRepo) UpdateUserRole(_ context.Context, _, _ string, _ shared.UserRole) error {
	return nil
}
func (s *stubBizRelRepo) UpdateRoleClaim(_ context.Context, _, _ string, _ shared.UserRole) error {
	return nil
}
func (s *stubBizRelRepo) UpdateKYBInitiated(_ context.Context, _, _ string, _ bool) error {
	return nil
}
func (s *stubBizRelRepo) UpdateKycNeeded(_ context.Context, _, _ string, _ bool) error {
	return nil
}
func (s *stubBizRelRepo) RemoveUserFromBusiness(_ context.Context, _, _ string) error {
	return nil
}
func (s *stubBizRelRepo) SetOwnerRoleToNull(_ context.Context, _ string) error {
	return nil
}
func (s *stubBizRelRepo) SetOwnerAndKYBInitiatorRoleToNull(_ context.Context, _ string) error {
	return nil
}
func (s *stubBizRelRepo) SetNonOwnerRolesToNone(_ context.Context, _ string) error {
	return nil
}
func (s *stubBizRelRepo) ResetAllUserRelationsForRejectedKYC(_ context.Context, _ string) error {
	return nil
}
func (s *stubBizRelRepo) OpsUpdateUserRole(_ context.Context, _, _ string, _ shared.UserRole) error {
	return nil
}
func (s *stubBizRelRepo) FindByUserAndBusiness(_ context.Context, _, _ string) (*coreBusiness.BusinessUserRelation, error) {
	return nil, nil
}
func (s *stubBizRelRepo) GetBusinessOwnerUserID(_ context.Context, _ string) (string, error) {
	return "", nil
}

// ---------------------------------------------------------------------------
// TestGetUserRoles_ReturnsRolesFromService
//
// RED: GetUserRoles reads c.Get("userRoles") — returns empty array when key
// is absent (userRoles not injected into context by BusinessRolesMiddleware).
// After Plan 02 adds businessUserRelRepo field to UserHandler and calls
// FindByUser directly, this test must pass (GREEN).
//
// Design note: This test constructs a UserHandler in the same package (white-box)
// to bypass middleware. It sets userId in context but NOT userRoles.
// The stubBizRelRepo is wired in after the struct is extended in Plan 02.
// For RED state: the test verifies that without userRoles in context, the
// handler returns empty data — proving the injection path is still the only path.
// ---------------------------------------------------------------------------

func TestGetUserRoles_ReturnsRolesFromService(t *testing.T) {
	const userID = "user-roles-test-id"
	const businessID = "biz-roles-test-id"

	// Build the stub repo and wire it into UserHandler.businessUserRelRepo (Plan 02).
	stub := &stubBizRelRepo{
		relations: []*coreBusiness.BusinessUserRelation{
			{
				UserID:    userID,
				BID:       businessID,
				Role:      shared.UserRole("owner"),
				KycNeeded: false,
			},
		},
		err: nil,
	}

	// Build handler with businessUserRelRepo wired in (Plan 02 adds this field).
	h := &UserHandler{
		businessUserRelRepo: stub,
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/user/roles", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Simulate JWTMiddleware: set userId in context (required by GetUserRoles).
	c.Set("userId", userID)

	// DO NOT set "userRoles" in context — simulating post-middleware-deletion state.
	// Current handler reads c.Get("userRoles"), finds nil, returns empty array.

	err := h.GetUserRoles(c)
	if err != nil {
		t.Fatalf("GetUserRoles returned unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	// Parse response body.
	var resp struct {
		Message string `json:"message"`
		Data    []struct {
			UserID     string `json:"userId"`
			BusinessID string `json:"businessId"`
			Role       string `json:"role"`
			KycNeeded  bool   `json:"kycNeeded"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// GREEN assertion: handler calls FindByUser directly via businessUserRelRepo.
	// Stub returns 1 relation; response data must contain 1 entry.
	if len(resp.Data) == 0 {
		t.Errorf("GetUserRoles returned empty array: expected roles from direct repo call (FindByUser) but got %d entries", len(resp.Data))
	}
}
