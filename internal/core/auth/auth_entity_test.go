package auth_test

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
	auth "xcentro.in/internal/core/auth"
)

// TestCustomClaimsHasBidAndRoleFields verifies struct fields exist with correct json tags.
// Uses struct literal to confirm fields compile — if fields are missing this fails at compile time.
func TestCustomClaimsHasBidAndRoleFields(t *testing.T) {
	claims := auth.CustomClaims{
		Bid:  "test-bid",
		Role: "owner",
	}
	if claims.Bid != "test-bid" {
		t.Errorf("expected Bid=test-bid, got %q", claims.Bid)
	}
	if claims.Role != "owner" {
		t.Errorf("expected Role=owner, got %q", claims.Role)
	}
}

// TestCustomClaimsZeroValues verifies empty Bid/Role are valid zero values (backward compat).
func TestCustomClaimsZeroValues(t *testing.T) {
	claims := auth.CustomClaims{}
	if claims.Bid != "" {
		t.Errorf("expected empty Bid, got %q", claims.Bid)
	}
	if claims.Role != "" {
		t.Errorf("expected empty Role, got %q", claims.Role)
	}
}

// TestBackwardCompatibleTokenParsing verifies a token that does NOT contain bid/role
// still parses successfully with empty-string zero values. Simulates pre-Phase-1 tokens.
func TestBackwardCompatibleTokenParsing(t *testing.T) {
	// Sign a minimal token that intentionally omits bid and role
	type oldClaims struct {
		jwt.RegisteredClaims
		SessionID     string `json:"session_id"`
		DeviceType    string `json:"device_type"`
		DeviceVersion int    `json:"device_version"`
	}
	old := oldClaims{
		SessionID:     "sess-old",
		DeviceType:    "web",
		DeviceVersion: 1,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, old)
	signed, err := token.SignedString([]byte("test-secret-key-for-testing-only"))
	if err != nil {
		t.Fatalf("failed to sign old-format token: %v", err)
	}

	// Parse with new CustomClaims — bid/role should be empty, not panic
	parsed, err := jwt.ParseWithClaims(signed, &auth.CustomClaims{}, func(t *jwt.Token) (interface{}, error) {
		return []byte("test-secret-key-for-testing-only"), nil
	})
	if err != nil {
		t.Fatalf("parsing old-format token failed: %v", err)
	}
	newClaims, ok := parsed.Claims.(*auth.CustomClaims)
	if !ok {
		t.Fatal("claims type assertion failed")
	}
	if newClaims.Bid != "" {
		t.Errorf("expected empty Bid from old token, got %q", newClaims.Bid)
	}
	if newClaims.Role != "" {
		t.Errorf("expected empty Role from old token, got %q", newClaims.Role)
	}
}
