package auth_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	authcore "xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/shared"
)

const testJWTSecret = "test-secret-key-32-chars-minimum!!"

// signTestToken is a helper that creates a signed token with CustomClaims directly,
// bypassing AuthService (which requires DB/Redis). Tests generateToken behavior
// by verifying the claims struct round-trips correctly through JWT sign+parse.
func signTestToken(bid, role string) (string, error) {
	claims := authcore.CustomClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-123",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		SessionID:     "sess-123",
		DeviceType:    "web",
		DeviceVersion: 1,
		Bid:           bid,
		Role:          role,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(testJWTSecret))
}

func parseTestToken(signed string) (*authcore.CustomClaims, error) {
	parsed, err := jwt.ParseWithClaims(signed, &authcore.CustomClaims{}, func(t *jwt.Token) (interface{}, error) {
		return []byte(testJWTSecret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*authcore.CustomClaims)
	if !ok {
		return nil, fmt.Errorf("type assertion failed")
	}
	return claims, nil
}

// TestJWTRoundTripWithBidAndRole verifies bid/role survive sign->parse round-trip.
func TestJWTRoundTripWithBidAndRole(t *testing.T) {
	signed, err := signTestToken("business-abc-123", "owner")
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	claims, err := parseTestToken(signed)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if claims.Bid != "business-abc-123" {
		t.Errorf("Bid round-trip: expected %q, got %q", "business-abc-123", claims.Bid)
	}
	if claims.Role != string(shared.Owner) {
		t.Errorf("Role round-trip: expected %q, got %q", shared.Owner, claims.Role)
	}
}

// TestJWTRoundTripEmptyBidRole verifies Phase 1 default (empty bid/role) round-trips correctly.
func TestJWTRoundTripEmptyBidRole(t *testing.T) {
	signed, err := signTestToken("", "")
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	claims, err := parseTestToken(signed)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if claims.Bid != "" {
		t.Errorf("expected empty Bid, got %q", claims.Bid)
	}
	if claims.Role != "" {
		t.Errorf("expected empty Role, got %q", claims.Role)
	}
}

// TestJWTRoleValues verifies all valid role strings survive round-trip.
func TestJWTRoleValues(t *testing.T) {
	roles := []string{"owner", "admin", "finance", "support", "none", ""}
	for _, role := range roles {
		t.Run("role="+role, func(t *testing.T) {
			signed, err := signTestToken("bid-xyz", role)
			if err != nil {
				t.Fatalf("sign failed for role %q: %v", role, err)
			}
			claims, err := parseTestToken(signed)
			if err != nil {
				t.Fatalf("parse failed for role %q: %v", role, err)
			}
			if claims.Role != role {
				t.Errorf("role round-trip: expected %q, got %q", role, claims.Role)
			}
		})
	}
}
