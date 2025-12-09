package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// GenerateSecureToken generates a cryptographically secure random token
// Returns a 64-character hex string (32 random bytes = 256 bits)
func GenerateSecureToken() (string, error) {
	// Generate 32 random bytes (256 bits)
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return hex.EncodeToString(randomBytes), nil
}

// HashToken creates a SHA-256 hash of a token for secure database storage
// This prevents token compromise if the database is breached
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// HashTokenWithPepper creates a SHA-256 hash of a token with pepper for secure storage
// Pepper should come from application config (e.g., bcrypt pepper)
func HashTokenWithPepper(token, pepper string) string {
	pepperedToken := fmt.Sprintf("%s_%s_%s", pepper, token, "xcentro")
	hash := sha256.Sum256([]byte(pepperedToken))
	return hex.EncodeToString(hash[:])
}

// GenerateHMACSignature generates an HMAC-SHA256 signature for userid:token
// This prevents tampering with verification URLs by ensuring integrity
// secretKey should be the JWT secret key from application config
func GenerateHMACSignature(userID, token, secretKey string) string {
	data := fmt.Sprintf("%s:%s", userID, token)
	h := hmac.New(sha256.New, []byte(secretKey))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

// ValidateHMACSignature validates an HMAC signature against userid:token
// Returns true if signature is valid, false otherwise
func ValidateHMACSignature(userID, token, signature, secretKey string) bool {
	expectedSignature := GenerateHMACSignature(userID, token, secretKey)
	return hmac.Equal([]byte(expectedSignature), []byte(signature))
}
