// This file contains TDD tests for UpdateCardStatus JWT-context role enforcement (AUTH-07).
// GREEN state — UpdateCardStatus reads c.Get("role") string injected by JWTMiddleware
// and c.Get("bid") for business context. No userRoles slice or X-Business-ID header needed.
//
// Run: go test ./internal/handlers/http/payment/... -run TestUpdateCardStatus -v
// Expected: PASS — handler passes role check via JWT context values.

package payment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/payment"
	"xcentro.in/internal/shared/logger"
)

// ---------------------------------------------------------------------------
// stubStoredCardRepo satisfies payment.StoredCardRepository.
// Returns a pre-configured card for GetStoredCardByID; stubs all other methods.
// ---------------------------------------------------------------------------

type stubStoredCardRepo struct {
	card *payment.StoredCard
	err  error
}

func (s *stubStoredCardRepo) GetStoredCardByID(_ context.Context, _ string) (*payment.StoredCard, error) {
	return s.card, s.err
}

func (s *stubStoredCardRepo) CreateStoredCard(_ context.Context, _ *payment.StoredCard) error {
	return nil
}

func (s *stubStoredCardRepo) GetStoredCardsByBusinessID(_ context.Context, _ string, _ bool) ([]*payment.StoredCard, error) {
	return nil, nil
}

func (s *stubStoredCardRepo) UpdateStoredCard(_ context.Context, _ *payment.StoredCard) error {
	return nil
}

func (s *stubStoredCardRepo) DeactivateCard(_ context.Context, _ string) error {
	return nil
}

func (s *stubStoredCardRepo) UpdateLastUsedAt(_ context.Context, _ string, _ time.Time) error {
	return nil
}

func (s *stubStoredCardRepo) UpdateCardStatus(_ context.Context, _ string, _ bool, _ string) error {
	return nil
}

func (s *stubStoredCardRepo) WithTx(_ interface{}) payment.StoredCardRepository {
	return s
}

// ---------------------------------------------------------------------------
// TestUpdateCardStatus_OwnerRoleFromJWTContext
//
// GREEN: UpdateCardStatus reads c.Get("role") == "owner" and c.Get("bid") from
// JWT context injected by JWTMiddleware — passes the role check without
// userRoles slice or X-Business-ID header.
//
// The test sets bid="biz-card-test-id" and role="owner" in context (simulating
// JWTMiddleware injection). No "userRoles" context key is needed.
// ---------------------------------------------------------------------------

func TestUpdateCardStatus_OwnerRoleFromJWTContext(t *testing.T) {
	const (
		userID     = "user-card-test-id"
		businessID = "biz-card-test-id"
		cardID     = "card-test-id-123"
	)

	// Stub card belongs to the correct business; has a valid token (can be toggled).
	cardStub := &stubStoredCardRepo{
		card: &payment.StoredCard{
			CardID:     cardID,
			BusinessID: businessID,
			Token:      "tok_test_abc123", // non-empty = tokenized, can be toggled
			IsActive:   true,
		},
		err: nil,
	}

	// Build PaymentHandler with only storedCardRepo and logger set.
	// All other fields are zero (nil) — only UpdateCardStatus's code path is tested.
	h := &PaymentHandler{
		storedCardRepo: cardStub,
		logger:         logger.GetDefault(),
	}

	e := echo.New()
	body := strings.NewReader(`{"isActive": false}`)
	req := httptest.NewRequest(http.MethodPatch, "/v1/wallet/cards/"+cardID+"/status", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("card_id")
	c.SetParamValues(cardID)

	// Simulate JWTMiddleware: inject userId, bid, and role into context.
	c.Set("userId", userID)
	c.Set("bid", businessID) // bid injected by JWTMiddleware
	c.Set("role", "owner")   // role injected by JWTMiddleware

	err := h.UpdateCardStatus(c)
	if err != nil {
		t.Fatalf("UpdateCardStatus returned unexpected error: %v", err)
	}

	// GREEN assertion: handler reads c.Get("role") == "owner" from JWT context
	// and passes the role check without userRoles slice.
	if rec.Code == http.StatusForbidden {
		t.Errorf("UpdateCardStatus returned 403 Forbidden: expected handler to read c.Get(\"role\") == \"owner\" and pass the role check, got status %d", rec.Code)
	}
}
