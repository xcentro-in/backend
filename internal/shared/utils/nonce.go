package utils

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// GenerateNonce generates a cryptographically secure random nonce for CSP
// Returns a base64-encoded string suitable for use in Content-Security-Policy headers
func GenerateNonce() (string, error) {
	// Generate 16 bytes of random data (128 bits)
	nonceBytes := make([]byte, 16)
	_, err := rand.Read(nonceBytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random nonce: %w", err)
	}

	// Encode to base64 for use in CSP headers
	nonce := base64.StdEncoding.EncodeToString(nonceBytes)
	return nonce, nil
}
