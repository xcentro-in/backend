// This file contains TDD tests for JWTMiddleware bid/role injection (AUTH-06).
// RED state — JWTMiddleware does not yet set "bid" or "role" in context.
// Run: go test ./internal/middleware/... -run TestJWTMiddleware -v
// Expected: FAIL — "bid" and "role" context values are nil after middleware runs.

package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	authcore "xcentro.in/internal/core/auth"
	redis "xcentro.in/internal/adapters/cache/redis/client"
	"xcentro.in/internal/middleware"
)

// ---------------------------------------------------------------------------
// noopAuthService satisfies auth.AuthService with zero-value stubs for all
// methods except ValidateAccessTokenWithVersion, which is the only method
// called by JWTMiddleware.
// ---------------------------------------------------------------------------

type noopAuthService struct {
	claims *authcore.CustomClaims
	err    error
}

func (m *noopAuthService) ValidateAccessTokenWithVersion(_ context.Context, _ string) (*authcore.CustomClaims, error) {
	return m.claims, m.err
}

// All other AuthService methods return zero values — not needed by JWTMiddleware.
func (m *noopAuthService) CreateTokenPair(_ context.Context, _, _, _, _ string) (string, string, error) {
	return "", "", nil
}
func (m *noopAuthService) RefreshAccessToken(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (m *noopAuthService) RevokeRefreshToken(_ context.Context, _, _ string) error { return nil }
func (m *noopAuthService) RevokeAllUserTokens(_ context.Context, _ string) error   { return nil }
func (m *noopAuthService) ValidateAccessToken(_ string) (*authcore.CustomClaims, error) {
	return nil, nil
}
func (m *noopAuthService) HashPassword(_ string) (string, error)        { return "", nil }
func (m *noopAuthService) ComparePassword(_, _ string) error            { return nil }
func (m *noopAuthService) Signup(_ context.Context, _, _ string) (*authcore.SignupResponse, error) {
	return nil, nil
}
func (m *noopAuthService) Login(_ context.Context, _, _, _, _ string) (*authcore.UnifiedLoginResponse, error) {
	return nil, nil
}
func (m *noopAuthService) BuildUnifiedLoginResponse(_ context.Context, _ string) (*authcore.UnifiedLoginResponse, error) {
	return nil, nil
}
func (m *noopAuthService) SwitchBusiness(_ context.Context, _, _ string) (*authcore.SwitchBusinessResponse, string, error) {
	return nil, "", nil
}
func (m *noopAuthService) SendPasswordResetEmail(_ context.Context, _, _ string) error  { return nil }
func (m *noopAuthService) ResendInviteEmail(_ context.Context, _ string) error           { return nil }
func (m *noopAuthService) ResetPasswordWithToken(_ context.Context, _, _, _, _ string) error {
	return nil
}
func (m *noopAuthService) ChangePassword(_ context.Context, _, _, _, _ string) error { return nil }
func (m *noopAuthService) SendEmailVerification(_ context.Context, _ string) error   { return nil }
func (m *noopAuthService) ResendEmailVerification(_ context.Context, _ string) error { return nil }
func (m *noopAuthService) SendMobileVerificationOTP(_ context.Context, _, _ string) error {
	return nil
}
func (m *noopAuthService) GenerateMobileOTP(_ context.Context, _ string) error { return nil }
func (m *noopAuthService) SendInviteOTP(_ context.Context, _, _, _ string) error { return nil }
func (m *noopAuthService) VerifyEmailWithMobile(_ context.Context, _, _, _, _ string) error {
	return nil
}
func (m *noopAuthService) VerifyMobileOTP(_ context.Context, _, _ string) error          { return nil }
func (m *noopAuthService) ResetPasswordWithMobileOTP(_ context.Context, _, _, _, _, _ string) error {
	return nil
}
func (m *noopAuthService) VerifyAndUpdateMobile(_ context.Context, _, _, _ string) error { return nil }
func (m *noopAuthService) ResendMobileOTP(_ context.Context, _ string) error             { return nil }
func (m *noopAuthService) ResendInviteOTP(_ context.Context, _, _, _ string) error       { return nil }
func (m *noopAuthService) GetRedisClient() *redis.RedisClient                            { return nil }
func (m *noopAuthService) VerifySession(_ context.Context, _ string) (*authcore.VerifySessionResponse, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// makeSignedJWT builds a valid JWT signed with the given secret.
// ---------------------------------------------------------------------------

const testJWTSecret = "test-jwt-secret-for-middleware-tests"

func makeSignedJWT(t *testing.T, bid, role string) string {
	t.Helper()
	claims := &authcore.CustomClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-test-id",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
		SessionID:     "session-test-id",
		DeviceType:    "web",
		DeviceVersion: 1,
		Bid:           bid,
		Role:          role,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("makeSignedJWT: failed to sign token: %v", err)
	}
	return signed
}

// ---------------------------------------------------------------------------
// TestJWTMiddleware_InjectsBidAndRole
//
// RED: JWTMiddleware does not yet call c.Set("bid", ...) or c.Set("role", ...).
// After implementation in Plan 02, these assertions should pass (GREEN).
// ---------------------------------------------------------------------------

func TestJWTMiddleware_InjectsBidAndRole(t *testing.T) {
	const (
		expectedBid  = "bid-xyz"
		expectedRole = "owner"
	)

	tokenString := makeSignedJWT(t, expectedBid, expectedRole)

	authSvc := &noopAuthService{
		claims: &authcore.CustomClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: "user-test-id",
			},
			SessionID:     "session-test-id",
			DeviceType:    "web",
			DeviceVersion: 1,
			Bid:           expectedBid,
			Role:          expectedRole,
		},
		err: nil,
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  "access_token",
		Value: tokenString,
	})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var capturedBid, capturedRole interface{}
	var nextCalled bool

	next := func(c echo.Context) error {
		nextCalled = true
		capturedBid = c.Get("bid")
		capturedRole = c.Get("role")
		return nil
	}

	mw := middleware.JWTMiddleware(authSvc)
	err := mw(next)(c)
	if err != nil {
		t.Fatalf("middleware returned unexpected error: %v", err)
	}
	if !nextCalled {
		t.Fatal("next handler was not called — middleware blocked the request unexpectedly")
	}

	// RED assertions: these FAIL until Plan 02 implements bid/role injection.
	if capturedBid != expectedBid {
		t.Errorf("c.Get(\"bid\"): expected %q, got %v", expectedBid, capturedBid)
	}
	if capturedRole != expectedRole {
		t.Errorf("c.Get(\"role\"): expected %q, got %v", expectedRole, capturedRole)
	}
}

// ---------------------------------------------------------------------------
// TestJWTMiddleware_EmptyBidRoleInContext
//
// RED: JWTMiddleware does not set "bid" or "role" in context at all.
// After Plan 02, it must set empty strings (not nil) for pre-Phase-3 tokens.
// ---------------------------------------------------------------------------

func TestJWTMiddleware_EmptyBidRoleInContext(t *testing.T) {
	tokenString := makeSignedJWT(t, "", "")

	authSvc := &noopAuthService{
		claims: &authcore.CustomClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: "user-test-id",
			},
			SessionID:     "session-test-id",
			DeviceType:    "web",
			DeviceVersion: 1,
			Bid:           "",
			Role:          "",
		},
		err: nil,
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  "access_token",
		Value: tokenString,
	})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var capturedBid, capturedRole interface{}
	var nextCalled bool

	next := func(c echo.Context) error {
		nextCalled = true
		capturedBid = c.Get("bid")
		capturedRole = c.Get("role")
		return nil
	}

	mw := middleware.JWTMiddleware(authSvc)
	err := mw(next)(c)
	if err != nil {
		t.Fatalf("middleware returned unexpected error: %v", err)
	}
	if !nextCalled {
		t.Fatal("next handler was not called — middleware blocked the request unexpectedly")
	}

	// RED assertions: these FAIL because "bid" and "role" are nil (not set), not empty string.
	// After Plan 02: c.Set("bid", "") makes capturedBid == "" (not nil).
	if capturedBid == nil {
		t.Errorf("c.Get(\"bid\"): expected empty string \"\", got nil — key not set in context")
	} else if capturedBid != "" {
		t.Errorf("c.Get(\"bid\"): expected empty string \"\", got %v", capturedBid)
	}
	if capturedRole == nil {
		t.Errorf("c.Get(\"role\"): expected empty string \"\", got nil — key not set in context")
	} else if capturedRole != "" {
		t.Errorf("c.Get(\"role\"): expected empty string \"\", got %v", capturedRole)
	}
}
